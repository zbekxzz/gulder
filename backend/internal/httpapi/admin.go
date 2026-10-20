package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

type namedRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Shop string `json:"shop,omitempty"`
}

func queryRefs(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string,
) ([]namedRef, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("data quality query: %w", err)
	}
	defer rows.Close()
	out := []namedRef{}
	for rows.Next() {
		var r namedRef
		if err := rows.Scan(&r.ID, &r.Name, &r.Shop); err != nil {
			return nil, fmt.Errorf("scan data quality: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// dataQuality — GET /admin/data-quality: растения без ASPCA и непроверенные, подарки без состава,
// букеты без фото, неподтверждённые цветы (§15, экран менеджера).
func (s *Server) dataQuality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	queries := map[string]string{
		"taxa_without_aspca":         `SELECT id, scientific_name, '' FROM plant_taxa WHERE aspca_url IS NULL ORDER BY genus`,
		"taxa_unverified":            `SELECT id, scientific_name, '' FROM plant_taxa WHERE NOT toxicity_verified ORDER BY genus`,
		"taxa_image_without_license": `SELECT id, scientific_name, '' FROM plant_taxa WHERE image_url IS NOT NULL AND image_license IS NULL ORDER BY genus`,
		"gifts_without_composition":  `SELECT g.id, g.name, s.name FROM gift_items g JOIN shops s ON s.id = g.shop_id WHERE g.active AND g.allergen_codes IS NULL ORDER BY g.id`,
		"bouquets_without_image":     `SELECT b.id, b.name, s.name FROM bouquets b JOIN shops s ON s.id = b.shop_id WHERE b.active AND b.image_url IS NULL ORDER BY b.id`,
		"unconfirmed_flowers":        `SELECT id, trade_name, '' FROM flowers WHERE NOT taxon_confirmed ORDER BY id`,
	}
	out := map[string]any{}
	for key, sql := range queries {
		refs, err := queryRefs(ctx, s.store.Pool, sql)
		if err != nil {
			writeError(w, r, err)
			return
		}
		out[key] = refs
	}
	writeJSON(w, http.StatusOK, out)
}

type countByKey struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// analytics — GET /admin/analytics: запуски, feedback, источник наборов, коды исключений Rules Engine.
// Только агрегаты, без персональных данных.
func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var runs, numerologyRuns, feedbackN, sets, llmSets, blockedSubs int
	var avgRating *float64
	err := s.store.Pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM recommendation_runs),
		(SELECT count(*) FROM recommendation_runs WHERE numerology_used),
		(SELECT count(*) FROM feedback),
		(SELECT avg(rating)::float8 FROM feedback WHERE rating IS NOT NULL),
		(SELECT count(*) FROM gift_sets),
		(SELECT count(*) FROM gift_sets WHERE generator = 'llm'),
		(SELECT count(*) FROM substitutions WHERE status = 'blocked')`).
		Scan(&runs, &numerologyRuns, &feedbackN, &avgRating, &sets, &llmSets, &blockedSubs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	byDay, err := countRows(ctx, s.store.Pool, `SELECT to_char(created_at::date, 'YYYY-MM-DD'), count(*)::int FROM recommendation_runs
		WHERE created_at > now() - interval '30 days' GROUP BY 1 ORDER BY 1`)
	if err != nil {
		writeError(w, r, err)
		return
	}
	blockCodes, err := countRows(ctx, s.store.Pool, `SELECT code, count(*)::int FROM recommendation_runs rr,
		jsonb_array_elements(rr.excluded) e, jsonb_array_elements_text(e->'codes') code GROUP BY code ORDER BY 2 DESC`)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runs_total": runs, "numerology_runs": numerologyRuns, "feedback_total": feedbackN, "avg_rating": avgRating,
		"gift_sets_total": sets, "gift_sets_llm": llmSets, "substitutions_blocked": blockedSubs,
		"runs_by_day": byDay, "exclusion_codes": blockCodes,
	})
}

func countRows(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, sql string,
) ([]countByKey, error) {
	rows, err := q.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("analytics query: %w", err)
	}
	defer rows.Close()
	out := []countByKey{}
	for rows.Next() {
		var c countByKey
		if err := rows.Scan(&c.Key, &c.Count); err != nil {
			return nil, fmt.Errorf("scan analytics: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// rulesInfo — GET /admin/rules: версия Rules Engine и модели (экран «ограничения/правила»).
func (s *Server) rulesInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"rules_version": rules.Version, "model_version": s.ranking.ModelVersion,
		"weights": s.ranking.Weights, "numerology_weight": s.ranking.NumerologyWeight,
		"numerology_label": ranking.NumerologyLabel,
	})
}
