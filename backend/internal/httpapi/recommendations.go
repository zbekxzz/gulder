package httpapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/audit"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/recipients"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
)

type recommendRequest struct {
	OccasionID int64           `json:"occasion_id" validate:"required,gt=0"`
	Filters    map[string]bool `json:"filters"`
}

// createRecommendations — POST /recommendations: Top-5 с причинами, уточняющий вопрос, run_id (§8.2).
// Ответ собирается только из GiverView-представлений (§7.6).
func (s *Server) createRecommendations(w http.ResponseWriter, r *http.Request) {
	var req recommendRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	filters, err := recommend.NormalizeFilters(req.Filters)
	if err != nil {
		writeError(w, r, errValidation(map[string]any{"filters": err.Error()}))
		return
	}
	run, err := recommend.Recommend(r.Context(), s.store, s.ranking, principal(r).UserID, req.OccasionID, filters, s.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) getRecommendations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	run, err := recommend.GetRun(r.Context(), s.store.Pool, principal(r).UserID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

// listRecipientRecommendations — история рекомендаций получателя (§15).
func (s *Server) listRecipientRecommendations(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	if _, err := recipients.Get(r.Context(), s.store.Pool, giver, id); err != nil {
		writeError(w, r, err)
		return
	}
	runs, err := recommend.ListRuns(r.Context(), s.store.Pool, giver, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": runs})
}

type numerologyRow struct {
	Number  int      `json:"number"`
	Flowers []string `json:"flowers"`
	Colors  []string `json:"colors"`
	Meaning string   `json:"meaning"`
}

type numerologyResponse struct {
	Active   *numerologyVersionDTO       `json:"active"`
	Versions []ranking.NumerologyVersion `json:"versions"`
	Label    string                      `json:"label"`
}

type numerologyVersionDTO struct {
	ranking.NumerologyVersion
	Rows []numerologyRow `json:"rows"`
}

func toNumerologyDTO(v *ranking.NumerologyVersion) *numerologyVersionDTO {
	if v == nil {
		return nil
	}
	dto := &numerologyVersionDTO{NumerologyVersion: *v, Rows: []numerologyRow{}}
	dto.NumerologyVersion.Rows = nil
	for _, e := range v.Rows {
		dto.Rows = append(dto.Rows, numerologyRow{Number: e.Number, Flowers: e.Flowers, Colors: e.Colors, Meaning: e.Meaning})
	}
	return dto
}

// getNumerology — GET /admin/numerology: активная таблица и история версий (FR-N03).
func (s *Server) getNumerology(w http.ResponseWriter, r *http.Request) {
	active, err := ranking.ActiveNumerology(r.Context(), s.store.Pool)
	if err != nil {
		writeError(w, r, err)
		return
	}
	versions, err := ranking.ListNumerologyVersions(r.Context(), s.store.Pool)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, numerologyResponse{Active: toNumerologyDTO(active), Versions: nonNilSlice(versions), Label: ranking.NumerologyLabel})
}

var numerologyNumbers = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 22, 33}

// putNumerology — PUT /admin/numerology: новая версия таблицы становится активной (FR-N03).
func (s *Server) putNumerology(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Rows []numerologyRow `json:"rows"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	details := map[string]any{}
	seen := map[int]bool{}
	entries := make([]ranking.NumerologyEntry, 0, len(req.Rows))
	for _, row := range req.Rows {
		key := "number_" + strconv.Itoa(row.Number)
		switch {
		case !slices.Contains(numerologyNumbers, row.Number):
			details[key] = "допустимо 1–9, 11, 22, 33"
		case seen[row.Number]:
			details[key] = "число повторяется"
		case strings.TrimSpace(row.Meaning) == "":
			details[key] = "нужно значение"
		}
		seen[row.Number] = true
		entries = append(entries, ranking.NumerologyEntry{Number: row.Number, Flowers: lower(row.Flowers), Colors: lower(row.Colors), Meaning: strings.TrimSpace(row.Meaning)})
	}
	if len(req.Rows) == 0 {
		details["rows"] = "таблица не может быть пустой"
	}
	if len(details) > 0 {
		writeError(w, r, errValidation(details))
		return
	}
	var v ranking.NumerologyVersion
	err := s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		if v, err = ranking.CreateNumerologyVersion(r.Context(), tx, principal(r).UserID, entries); err != nil {
			return err
		}
		return audit.Record(r.Context(), tx, principal(r).UserID, audit.NumerologyUpdated, "numerology_version", v.ID,
			map[string]any{"version": v.Version, "rows": len(entries)})
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toNumerologyDTO(&v))
}

func lower(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			out = append(out, s)
		}
	}
	return out
}
