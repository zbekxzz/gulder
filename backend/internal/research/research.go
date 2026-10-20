// Package research — исследовательский режим (RM-01–RM-08): согласия, псевдонимизированная связка
// research_id, экспорт без прямых идентификаторов и свободного текста, data dictionary и журнал оценок.
// Функциональность продукта от этого пакета не зависит (RM-08).
package research

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

// ConsentVersion — текущая версия текста согласия (RM-01). Меняется вместе с текстом.
const ConsentVersion = "research-consent-v1"

// ConsentText — текст информированного согласия.
const ConsentText = `Исследовательский режим помогает улучшать подбор букетов. Если вы согласны:
• ваши отзывы о рекомендациях (оценка, «нравится/не нравится»), тип повода, диапазон бюджета, стиль и формат выбранных букетов попадут в обезличенный исследовательский набор под случайным идентификатором;
• в набор не попадают email, имена получателей, комментарии, даты рождения, аллергии, питомцы и геолокация;
• группы меньше 5 записей скрываются при экспорте;
• ответы самих получателей в набор не входят без их собственного согласия.
Согласие можно отозвать одной кнопкой в любой момент — ваши данные исключаются из последующих экспортов. От согласия не зависит работа сервиса.`

// ErrWrongVersion — согласие дано на устаревший текст.
var ErrWrongVersion = errors.New("consent version is outdated")

// ConsentStatus — состояние согласия пользователя.
type ConsentStatus struct {
	Active         bool       `json:"active"`
	Version        *string    `json:"version"`
	GrantedAt      *time.Time `json:"granted_at"`
	CurrentVersion string     `json:"current_version"`
	Text           string     `json:"text"`
}

// Status возвращает состояние согласия.
func Status(ctx context.Context, q store.Querier, userID int64) (ConsentStatus, error) {
	s := ConsentStatus{CurrentVersion: ConsentVersion, Text: ConsentText}
	err := q.QueryRow(ctx, `SELECT consent_version, granted_at FROM research.research_consents
		WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&s.Version, &s.GrantedAt)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return s, nil
	case err != nil:
		return s, fmt.Errorf("consent status: %w", err)
	}
	s.Active = true
	return s, nil
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Grant фиксирует согласие с версией текста и создаёт research_id (RM-01, RM-03).
// Триггер в БД не даст создать связку без активного согласия (AC-10).
func Grant(ctx context.Context, q store.Querier, userID int64, version string) (bool, error) {
	if version != ConsentVersion {
		return false, ErrWrongVersion
	}
	tag, err := q.Exec(ctx, `INSERT INTO research.research_consents (user_id, consent_version) VALUES ($1, $2)
		ON CONFLICT (user_id) WHERE revoked_at IS NULL DO NOTHING`, userID, version)
	if err != nil {
		return false, fmt.Errorf("grant consent: %w", err)
	}
	if _, err := q.Exec(ctx, `INSERT INTO research.research_subjects (research_id, user_id) VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING`, newUUID(), userID); err != nil {
		return false, fmt.Errorf("create research subject: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// Revoke отзывает согласие в один шаг: связка удаляется, данные выпадают из будущих экспортов (RM-02).
func Revoke(ctx context.Context, q store.Querier, userID int64) error {
	if _, err := q.Exec(ctx, `SELECT research.revoke_consent($1)`, userID); err != nil {
		return fmt.Errorf("revoke consent: %w", err)
	}
	return nil
}

// DataDictionaryVersion — версия описания полей экспорта.
const DataDictionaryVersion = "dd-1"

// MinGroupSize — группы меньше скрываются (RM-06).
const MinGroupSize = 5

// Field — описание поля экспорта.
type Field struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// DataDictionary — словарь данных экспорта (RM-06). Дублируется в docs/data_dictionary.md.
func DataDictionary() map[string]any {
	return map[string]any{
		"version":        DataDictionaryVersion,
		"min_group_size": MinGroupSize,
		"grouping":       "группы по (occasion_type, budget_band); группы меньше min_group_size удаляются целиком",
		"excluded":       "email, имена, никнеймы получателей, комментарии, даты рождения, аллергии, питомцы, астма, геолокация, ответы получателей",
		"fields": []Field{
			{"research_id", "uuid", "Псевдоним дарителя; связь с аккаунтом хранится отдельно с ограниченным доступом (RM-03)"},
			{"event_month", "YYYY-MM", "Месяц отзыва (без точной даты)"},
			{"occasion_type", "enum", "Тип повода: birthday, march8, …"},
			{"budget_band", "enum", "Диапазон максимального бюджета: <15000, 15000-30000, >30000 (тенге)"},
			{"model_version", "string", "Версия ranker'а запуска"},
			{"rules_version", "string", "Версия Rules Engine запуска"},
			{"numerology_used", "bool", "Нумерологический фактор применялся (отдельный признак, RM-07)"},
			{"bouquet_style", "enum|null", "Стиль букета, о котором отзыв"},
			{"bouquet_format", "enum|null", "Формат композиции"},
			{"rating", "int 1–5|null", "Оценка"},
			{"liked", "bool|null", "Понравился ли букет"},
		},
	}
}

// ExportRow — строка экспорта.
type ExportRow struct {
	ResearchID     string  `json:"research_id"`
	EventMonth     string  `json:"event_month"`
	OccasionType   string  `json:"occasion_type"`
	BudgetBand     string  `json:"budget_band"`
	ModelVersion   string  `json:"model_version"`
	RulesVersion   string  `json:"rules_version"`
	NumerologyUsed bool    `json:"numerology_used"`
	BouquetStyle   *string `json:"bouquet_style"`
	BouquetFormat  *string `json:"bouquet_format"`
	Rating         *int    `json:"rating"`
	Liked          *bool   `json:"liked"`
}

// Export — результат экспорта.
type Export struct {
	ExportID              int64       `json:"export_id"`
	DataDictionaryVersion string      `json:"data_dictionary_version"`
	MinGroupSize          int         `json:"min_group_size"`
	SuppressedGroups      int         `json:"suppressed_groups"`
	Rows                  []ExportRow `json:"rows"`
}

// BuildExport собирает экспорт только по пользователям с активным согласием (RM-02, RM-06).
func BuildExport(ctx context.Context, q store.Querier, managerID int64) (Export, error) {
	rows, err := q.Query(ctx, `
		SELECT rs.research_id::text, to_char(f.created_at, 'YYYY-MM'), o.type,
		       CASE WHEN o.budget_max < 15000 THEN '<15000' WHEN o.budget_max <= 30000 THEN '15000-30000' ELSE '>30000' END,
		       rr.model_version, rr.rules_version, rr.numerology_used, b.style, b.composition_format, f.rating, f.liked
		  FROM feedback f
		  JOIN recommendation_runs rr ON rr.id = f.run_id
		  JOIN occasions o ON o.id = rr.occasion_id
		  JOIN recipients r ON r.id = f.recipient_id
		  JOIN research.research_subjects rs ON rs.user_id = r.giver_id
		  JOIN research.research_consents rc ON rc.user_id = r.giver_id AND rc.revoked_at IS NULL
		  LEFT JOIN bouquets b ON b.id = f.bouquet_id
		 ORDER BY rs.research_id, f.created_at, f.id`)
	if err != nil {
		return Export{}, fmt.Errorf("export query: %w", err)
	}
	all, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExportRow, error) {
		var e ExportRow
		err := r.Scan(&e.ResearchID, &e.EventMonth, &e.OccasionType, &e.BudgetBand, &e.ModelVersion, &e.RulesVersion,
			&e.NumerologyUsed, &e.BouquetStyle, &e.BouquetFormat, &e.Rating, &e.Liked)
		return e, err
	})
	if err != nil {
		return Export{}, fmt.Errorf("export rows: %w", err)
	}
	groups := map[string]int{}
	for _, r := range all {
		groups[r.OccasionType+"|"+r.BudgetBand]++
	}
	exp := Export{DataDictionaryVersion: DataDictionaryVersion, MinGroupSize: MinGroupSize, Rows: []ExportRow{}}
	for _, n := range groups {
		if n < MinGroupSize {
			exp.SuppressedGroups++
		}
	}
	for _, r := range all {
		if groups[r.OccasionType+"|"+r.BudgetBand] >= MinGroupSize {
			exp.Rows = append(exp.Rows, r)
		}
	}
	err = q.QueryRow(ctx, `INSERT INTO research.research_exports (requested_by, dataset_version, data_dictionary_version, min_group_size, row_count, file_uri)
		VALUES ($1, 'feedback-live', $2, $3, $4, 'inline') RETURNING id`, managerID, DataDictionaryVersion, MinGroupSize, len(exp.Rows)).Scan(&exp.ExportID)
	if err != nil {
		return Export{}, fmt.Errorf("record export: %w", err)
	}
	return exp, nil
}

// ExperimentRun — запись research.experiment_runs (RM-04).
type ExperimentRun struct {
	ID             int64           `json:"id"`
	ModelVersion   string          `json:"model_version"`
	DatasetVersion string          `json:"dataset_version"`
	SplitID        string          `json:"split_id"`
	Seed           int             `json:"seed"`
	Params         json.RawMessage `json:"params"`
	Metrics        json.RawMessage `json:"metrics"`
	CreatedAt      time.Time       `json:"created_at"`
}

const runCols = `id, model_version, dataset_version, split_id, seed, params, metrics, created_at`

func scanRuns(rows pgx.Rows) ([]ExperimentRun, error) {
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (ExperimentRun, error) {
		var e ExperimentRun
		err := r.Scan(&e.ID, &e.ModelVersion, &e.DatasetVersion, &e.SplitID, &e.Seed, &e.Params, &e.Metrics, &e.CreatedAt)
		return e, err
	})
	if err != nil {
		return nil, fmt.Errorf("scan experiment runs: %w", err)
	}
	return out, nil
}

// ListRuns — последние запуски оценки.
func ListRuns(ctx context.Context, q store.Querier) ([]ExperimentRun, error) {
	rows, err := q.Query(ctx, `SELECT `+runCols+` FROM research.experiment_runs ORDER BY created_at DESC, id DESC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("list experiment runs: %w", err)
	}
	return scanRuns(rows)
}

// RunWithComparison — запуск и все запуски на том же split (RM-05).
func RunWithComparison(ctx context.Context, q store.Querier, id int64) (ExperimentRun, []ExperimentRun, error) {
	rows, err := q.Query(ctx, `SELECT `+runCols+` FROM research.experiment_runs WHERE id = $1`, id)
	if err != nil {
		return ExperimentRun{}, nil, fmt.Errorf("get experiment run: %w", err)
	}
	found, err := scanRuns(rows)
	if err != nil {
		return ExperimentRun{}, nil, err
	}
	if len(found) == 0 {
		return ExperimentRun{}, nil, store.ErrNotFound
	}
	rows, err = q.Query(ctx, `SELECT `+runCols+` FROM research.experiment_runs WHERE split_id = $1 ORDER BY id`, found[0].SplitID)
	if err != nil {
		return ExperimentRun{}, nil, fmt.Errorf("same split runs: %w", err)
	}
	same, err := scanRuns(rows)
	return found[0], same, err
}
