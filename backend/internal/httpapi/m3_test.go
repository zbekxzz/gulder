package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type card struct {
	BouquetID   int64    `json:"bouquet_id"`
	Rank        int      `json:"rank"`
	Name        string   `json:"name"`
	Score       float64  `json:"score"`
	Confidence  float64  `json:"confidence"`
	Reasons     []string `json:"reasons"`
	Conflicts   []string `json:"conflicts"`
	MissingInfo []string `json:"missing_info"`
	Warnings    []struct{ Code, Message string }
	Numerology  *struct {
		Number  int
		Meaning string
		Label   string
	}
	Shop struct {
		ID   int64
		Name string
	}
	Price         float64  `json:"price"`
	DiscountPrice *float64 `json:"discount_price"`
	Items         []struct {
		TradeName string   `json:"trade_name"`
		Genus     string   `json:"genus"`
		ToxicFor  []string `json:"toxic_for"`
		Stems     int
	}
}

type runResp struct {
	RunID              int64  `json:"run_id"`
	ModelVersion       string `json:"model_version"`
	RulesVersion       string `json:"rules_version"`
	NumerologyUsed     bool   `json:"numerology_used"`
	Items              []card `json:"items"`
	ExcludedCount      int    `json:"excluded_count"`
	ClarifyingQuestion *struct {
		Attribute string
		Text      string
	} `json:"clarifying_question"`
}

func (e *env) recommend(token string, occasionID int64) (runResp, resp) {
	e.t.Helper()
	r := e.do(http.MethodPost, "/api/v1/recommendations", token, map[string]any{"occasion_id": occasionID})
	require.Equal(e.t, http.StatusCreated, r.Status, string(r.Body))
	var run runResp
	r.Decode(e.t, &run)
	return run, r
}

func knownNoRestrictions() []map[string]any {
	return []map[string]any{
		attr("allergies", []string{}, "know"), attr("pets", []string{}, "know"),
		attr("asthma", false, "know"), attr("scent_sensitivity", "none", "know"),
	}
}

// AC-1 (HTTP): при одном бюджете два получателя получают разные рекомендации.
func TestAC1_TwoRecipientsSameBudget_HTTP(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	// одинаковые вкусы (любят лилии и белый цвет), разные ограничения
	tastes := []map[string]any{attr("liked_flowers", []string{"lilium"}, "know"), attr("liked_colors", []string{"white"}, "know")}
	free := e.createRecipient(giver, "Без ограничений", append(knownNoRestrictions(), tastes...)...)
	catAttrs := append([]map[string]any{
		attr("allergies", []string{}, "know"), attr("pets", []string{"cat"}, "know"),
		attr("asthma", true, "know"), attr("scent_sensitivity", "none", "know"),
	}, tastes...)
	cat := e.createRecipient(giver, "Кошатница-астматик", catAttrs...)

	runFree, _ := e.recommend(giver, e.createOccasion(giver, free, 30000, nil))
	runCat, _ := e.recommend(giver, e.createOccasion(giver, cat, 30000, nil))
	require.Len(t, runFree.Items, 5)
	require.NotEmpty(t, runCat.Items)

	set := func(r runResp) []int64 {
		out := []int64{}
		for _, c := range r.Items {
			out = append(out, c.BouquetID)
		}
		return out
	}
	assert.NotEqual(t, set(runFree), set(runCat))
	hasLily := false
	for _, c := range runFree.Items {
		for _, it := range c.Items {
			hasLily = hasLily || it.Genus == "Lilium"
		}
	}
	assert.True(t, hasLily, "без ограничений любимые лилии попадают в Top-5")
	assert.Greater(t, runCat.ExcludedCount, runFree.ExcludedCount)
	for _, c := range runCat.Items {
		for _, it := range c.Items {
			assert.NotContains(t, it.ToxicFor, "cat", "у кошатника нет токсичных для кошек растений: %s", c.Name)
			assert.NotEqual(t, "Lilium", it.Genus)
		}
	}
	assert.Equal(t, "baseline-0.1", runFree.ModelVersion)
	assert.Equal(t, "rules-1.0.0", runFree.RulesVersion)
}

// FR-R01, FR-C03, §8.2: неподтверждённые растения и букеты дороже бюджета не попадают в Top-5;
// карточка объяснима: причины, конфликты, недостающие сведения.
func TestTop5ExplainableAndSafe_FRR03(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Пустой профиль")
	run, _ := e.recommend(giver, e.createOccasion(giver, id, 25000, map[string]any{"emotion_goal": "joy"}))
	require.NotEmpty(t, run.Items)
	shops := map[int64]int{}
	for i, c := range run.Items {
		assert.Equal(t, i+1, c.Rank)
		assert.NotEmpty(t, c.Reasons, "у карточки есть причины")
		assert.NotNil(t, c.Conflicts)
		assert.LessOrEqual(t, c.Price, 25000.0)
		assert.True(t, c.Confidence > 0 && c.Confidence <= 1)
		for _, it := range c.Items {
			assert.NotContains(t, it.TradeName, "уточняется", "FR-C03: неподтверждённое растение")
		}
		shops[c.Shop.ID]++
		if i > 0 {
			assert.GreaterOrEqual(t, run.Items[i-1].Score, c.Score)
		}
	}
	for _, n := range shops {
		assert.LessOrEqual(t, n, 2)
	}
	// FR-R04: при пустом профиле есть уточняющий вопрос
	require.NotNil(t, run.ClarifyingQuestion)
	assert.NotEmpty(t, run.ClarifyingQuestion.Text)
	assert.Contains(t, []string{"allergies", "pets", "asthma", "scent_sensitivity"}, run.ClarifyingQuestion.Attribute)

	// сохранённый запуск доступен владельцу и только ему
	got := e.do(http.MethodGet, fmt.Sprintf("/api/v1/recommendations/%d", run.RunID), giver, nil)
	require.Equal(t, http.StatusOK, got.Status)
	var saved runResp
	got.Decode(t, &saved)
	assert.Equal(t, run.Items, saved.Items)
	assert.Equal(t, run.ClarifyingQuestion, saved.ClarifyingQuestion)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, fmt.Sprintf("/api/v1/recommendations/%d", run.RunID), e.login("giver2@gulder.local"), nil).Status)

	hist := e.do(http.MethodGet, fmt.Sprintf("/api/v1/recipients/%d/recommendations", id), giver, nil).JSON(t)
	assert.Len(t, hist["items"], 1)

	// полный профиль — вопрос не задаётся
	full := e.createRecipient(giver, "Полный", knownNoRestrictions()...)
	runFull, _ := e.recommend(giver, e.createOccasion(giver, full, 25000, nil))
	assert.Nil(t, runFull.ClarifyingQuestion)
}

// §7.6: все ответы для роли giver проходят через GiverView — проверка на уровне HTTP.
func TestGiverViewOverHTTP(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Подруга", attr("pets", []string{}, "know"), attr("asthma", false, "know"), attr("scent_sensitivity", "none", "know"))
	_, err := e.store.Pool.Exec(context.Background(), `INSERT INTO recipient_attributes (recipient_id, key, value, source, confidence)
		VALUES ($1, 'allergies', '["genus:lilium","family:asteraceae"]', 'recipient', 'know'),
		       ($1, 'disliked_flowers', '["rosa"]', 'recipient', 'know')`, id)
	require.NoError(t, err)

	run, raw := e.recommend(giver, e.createOccasion(giver, id, 60000, nil))
	body := strings.ToLower(string(raw.Body))
	assert.NotContains(t, body, "lilium")
	assert.NotContains(t, body, "asteraceae")
	assert.NotContains(t, body, "genus:")
	assert.NotContains(t, body, "аллерги")
	assert.NotContains(t, body, "не любит")
	sawHidden := false
	for _, c := range run.Items {
		for _, w := range c.Warnings {
			assert.NotEqual(t, "ALLERGEN_MATCH", w.Code)
			if w.Code == "RECIPIENT_RESTRICTION" {
				sawHidden = true
				assert.Equal(t, "Не подходит по ограничениям получателя", w.Message)
			}
		}
	}
	assert.True(t, sawHidden, "нелюбимые розы получателя видны только как RECIPIENT_RESTRICTION")

	saved := e.do(http.MethodGet, fmt.Sprintf("/api/v1/recommendations/%d", run.RunID), giver, nil)
	assert.NotContains(t, strings.ToLower(string(saved.Body)), "lilium")
}

// FR-N01–N07: нумерология включается на поводе, число и пометка на карточке, лог использования в запуске.
func TestNumerologyOnCards_FRN05(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Именинница", append(knownNoRestrictions(), attr("birth_date", "1990-03-15", "know"))...)

	off, _ := e.recommend(giver, e.createOccasion(giver, id, 30000, nil))
	assert.False(t, off.NumerologyUsed)
	for _, c := range off.Items {
		assert.Nil(t, c.Numerology)
	}

	on, _ := e.recommend(giver, e.createOccasion(giver, id, 30000, map[string]any{"numerology_enabled": true}))
	assert.True(t, on.NumerologyUsed)
	require.NotEmpty(t, on.Items)
	n := on.Items[0].Numerology
	require.NotNil(t, n)
	assert.Equal(t, 1, n.Number)
	assert.Equal(t, "развлекательная функция", n.Label)
	assert.NotEmpty(t, n.Meaning)
	assert.Equal(t, off.ExcludedCount, on.ExcludedCount, "AC-7: безопасность не меняется")

	var used bool
	var version *int64
	require.NoError(t, e.store.Pool.QueryRow(context.Background(),
		`SELECT numerology_used, numerology_version_id FROM recommendation_runs WHERE id = $1`, on.RunID).Scan(&used, &version))
	assert.True(t, used)
	assert.NotNil(t, version)

	// без даты рождения нумерология не применяется, даже если включена
	noBirth := e.createRecipient(giver, "Без даты", knownNoRestrictions()...)
	r, _ := e.recommend(giver, e.createOccasion(giver, noBirth, 30000, map[string]any{"numerology_enabled": true}))
	assert.False(t, r.NumerologyUsed)
}

// FR-N03: менеджер редактирует таблицу нумерологии, каждая правка — новая активная версия + audit.
func TestNumerologyAdminVersioning_FRN03(t *testing.T) {
	e := newEnv(t)
	manager, giver := e.login("manager@gulder.local"), e.login("giver@gulder.local")
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v1/admin/numerology", giver, nil).Status)

	r := e.do(http.MethodGet, "/api/v1/admin/numerology", manager, nil)
	require.Equal(t, http.StatusOK, r.Status)
	body := r.JSON(t)
	assert.EqualValues(t, 1, body["active"].(map[string]any)["version"])
	assert.Len(t, body["active"].(map[string]any)["rows"], 12)

	r = e.do(http.MethodPut, "/api/v1/admin/numerology", manager, map[string]any{"rows": []map[string]any{
		{"number": 1, "flowers": []string{"Rosa"}, "colors": []string{"red"}, "meaning": "Начало"},
		{"number": 10, "flowers": []string{}, "colors": []string{}, "meaning": "x"},
	}})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)

	r = e.do(http.MethodPut, "/api/v1/admin/numerology", manager, map[string]any{"rows": []map[string]any{
		{"number": 1, "flowers": []string{"Rosa"}, "colors": []string{"red"}, "meaning": "Начало"},
		{"number": 11, "flowers": []string{"phalaenopsis"}, "colors": []string{}, "meaning": "Вдохновение"},
	}})
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	v := r.JSON(t)
	assert.EqualValues(t, 2, v["version"])
	assert.Equal(t, []any{"rosa"}, v["rows"].([]any)[0].(map[string]any)["flowers"])

	body = e.do(http.MethodGet, "/api/v1/admin/numerology", manager, nil).JSON(t)
	assert.EqualValues(t, 2, body["active"].(map[string]any)["version"])
	assert.Len(t, body["versions"], 2)

	var n int
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE action='numerology.updated'`).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestRecommendationsValidation(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	r := e.do(http.MethodPost, "/api/v1/recommendations", giver, map[string]any{"occasion_id": 999999})
	assert.Equal(t, http.StatusNotFound, r.Status)
	id := e.createRecipient(giver, "X")
	occ := e.createOccasion(giver, id, 10000, nil)
	r = e.do(http.MethodPost, "/api/v1/recommendations", giver, map[string]any{"occasion_id": occ, "filters": map[string]bool{"teleport": true}})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	r = e.do(http.MethodPost, "/api/v1/recommendations", e.login("giver2@gulder.local"), map[string]any{"occasion_id": occ})
	assert.Equal(t, http.StatusNotFound, r.Status, "чужой повод")
}
