package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/audit"
	"github.com/zbekxzz/gulder/backend/internal/research"
)

// getConsent — GET /research/consent: состояние, текущая версия и текст согласия (RM-01).
func (s *Server) getConsent(w http.ResponseWriter, r *http.Request) {
	st, err := research.Status(r.Context(), s.store.Pool, principal(r).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// grantConsent — POST /research/consent {consent_version} (RM-01).
func (s *Server) grantConsent(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConsentVersion string `json:"consent_version" validate:"required"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	user := principal(r).UserID
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		created, err := research.Grant(r.Context(), tx, user, req.ConsentVersion)
		if err != nil || !created {
			return err
		}
		return audit.Record(r.Context(), tx, user, audit.ConsentGranted, "user", user, map[string]any{"consent_version": req.ConsentVersion})
	})
	if errors.Is(err, research.ErrWrongVersion) {
		writeError(w, r, errConflict("consent_outdated", "Текст согласия обновился — ознакомьтесь с новой версией", map[string]any{"current_version": research.ConsentVersion}))
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.getConsent(w, r)
}

// revokeConsent — DELETE /research/consent: отзыв в один шаг (RM-02).
func (s *Server) revokeConsent(w http.ResponseWriter, r *http.Request) {
	user := principal(r).UserID
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := research.Revoke(r.Context(), tx, user); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, user, audit.ConsentRevoked, "user", user, nil)
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportResearch — POST /research/export: без прямых идентификаторов и свободного текста, группы < 5 скрыты (RM-06).
func (s *Server) exportResearch(w http.ResponseWriter, r *http.Request) {
	manager := principal(r).UserID
	var exp research.Export
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		if exp, err = research.BuildExport(r.Context(), tx, manager); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, manager, audit.ResearchExported, "research_export", exp.ExportID,
			map[string]any{"rows": len(exp.Rows), "suppressed_groups": exp.SuppressedGroups})
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, exp)
}

func (s *Server) dataDictionary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, research.DataDictionary())
}

// listEvaluations — список запусков оценки для dashboard (RM-05).
func (s *Server) listEvaluations(w http.ResponseWriter, r *http.Request) {
	runs, err := research.ListRuns(r.Context(), s.store.Pool)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(runs)})
}

// getEvaluation — GET /research/evaluation/{run_id}: запуск и сравнение на том же split (RM-05).
func (s *Server) getEvaluation(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "run_id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	run, same, err := research.RunWithComparison(r.Context(), s.store.Pool, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "same_split": nonNilSlice(same)})
}

type judgementDTO struct {
	ID                int64           `json:"id"`
	DatasetVersion    string          `json:"dataset_version"`
	CaseID            string          `json:"case_id"`
	RecipientSnapshot json.RawMessage `json:"recipient_snapshot"`
	OccasionSnapshot  json.RawMessage `json:"occasion_snapshot"`
	BouquetID         int64           `json:"bouquet_id"`
	BouquetName       string          `json:"bouquet_name"`
	Grade             int             `json:"grade"`
	IsUnsafe          bool            `json:"is_unsafe"`
	Reason            string          `json:"reason"`
	ExpertID          *int64          `json:"expert_id"`
	CreatedAt         time.Time       `json:"created_at"`
}

// listJudgements — GET /admin/expert-judgements?dataset_version=…: случаи для экспертной разметки.
func (s *Server) listJudgements(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("dataset_version")
	rows, err := s.store.Pool.Query(r.Context(), `
		SELECT ej.id, ej.dataset_version, ej.case_id, ej.recipient_snapshot, ej.occasion_snapshot, ej.bouquet_id, b.name,
		       ej.grade, ej.is_unsafe, ej.reason, ej.expert_id, ej.created_at
		  FROM expert_judgements ej JOIN bouquets b ON b.id = ej.bouquet_id
		 WHERE ej.dataset_version = $1 ORDER BY ej.case_id, ej.bouquet_id`, version)
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (judgementDTO, error) {
		var j judgementDTO
		err := row.Scan(&j.ID, &j.DatasetVersion, &j.CaseID, &j.RecipientSnapshot, &j.OccasionSnapshot, &j.BouquetID, &j.BouquetName,
			&j.Grade, &j.IsUnsafe, &j.Reason, &j.ExpertID, &j.CreatedAt)
		return j, err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	versions, err := countRows(r.Context(), s.store.Pool, `SELECT dataset_version, count(*)::int FROM expert_judgements GROUP BY 1 ORDER BY 1`)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": nonNilSlice(items), "versions": versions})
}

type judgementRequest struct {
	DatasetVersion    string          `json:"dataset_version" validate:"required,max=40"`
	CaseID            string          `json:"case_id" validate:"required,max=60"`
	RecipientSnapshot json.RawMessage `json:"recipient_snapshot" validate:"required"`
	OccasionSnapshot  json.RawMessage `json:"occasion_snapshot" validate:"required"`
	BouquetID         int64           `json:"bouquet_id" validate:"required,gt=0"`
	Grade             int             `json:"grade" validate:"min=0,max=3"`
	IsUnsafe          bool            `json:"is_unsafe"`
	Reason            string          `json:"reason" validate:"required,max=500"`
}

// saveJudgement — POST /admin/expert-judgements: оценка 0–3 + причина в проверенную версию набора (RM-07).
func (s *Server) saveJudgement(w http.ResponseWriter, r *http.Request) {
	var req judgementRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	details := map[string]any{}
	if strings.HasPrefix(req.DatasetVersion, "draft-") {
		details["dataset_version"] = "черновые версии генерируются, проверенная разметка пишется в новую версию (например, v1)"
	}
	if req.IsUnsafe && req.Grade != 0 {
		details["grade"] = "опасная пара (is_unsafe) размечается оценкой 0"
	}
	if !json.Valid(req.RecipientSnapshot) || !json.Valid(req.OccasionSnapshot) {
		details["snapshot"] = "ожидается JSON"
	}
	if len(details) > 0 {
		writeError(w, r, errValidation(details))
		return
	}
	expert := principal(r).UserID
	var id int64
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := tx.QueryRow(r.Context(), `
			INSERT INTO expert_judgements (dataset_version, case_id, recipient_snapshot, occasion_snapshot, bouquet_id, grade, is_unsafe, reason, expert_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (dataset_version, case_id, bouquet_id) DO UPDATE
			   SET grade = EXCLUDED.grade, is_unsafe = EXCLUDED.is_unsafe, reason = EXCLUDED.reason, expert_id = EXCLUDED.expert_id
			RETURNING id`, req.DatasetVersion, req.CaseID, req.RecipientSnapshot, req.OccasionSnapshot, req.BouquetID,
			req.Grade, req.IsUnsafe, strings.TrimSpace(req.Reason), expert).Scan(&id); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, expert, audit.ExpertJudgementStored, "expert_judgement", id,
			map[string]any{"dataset_version": req.DatasetVersion, "case_id": req.CaseID, "grade": req.Grade})
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

// deleteAccount — DELETE /me: удаляет аккаунт, получателей, атрибуты (включая даты рождения),
// поводы, запуски и research-связку каскадом (§18.5, RM-02, FR-N07).
func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	user := principal(r).UserID
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		if err := research.Revoke(r.Context(), tx, user); err != nil {
			return err
		}
		if _, err := tx.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, user); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, 0, audit.AccountDeleted, "user", user, nil)
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
