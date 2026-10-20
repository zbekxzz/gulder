package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/assistant"
	"github.com/zbekxzz/gulder/backend/internal/httpapi"
)

type offerItem struct {
	BouquetID     int64    `json:"bouquet_id"`
	Name          string   `json:"name"`
	Price         float64  `json:"price"`
	DiscountPrice *float64 `json:"discount_price"`
	PriceDiff     *float64 `json:"price_diff"`
	Score         float64  `json:"score"`
	Shop          struct {
		ID        int64
		Name      string
		DistanceM *float64 `json:"distance_m"`
		OpenNow   bool     `json:"open_now"`
	}
	Items []struct {
		ToxicFor []string `json:"toxic_for"`
	}
}

type offersResp struct {
	Items   []offerItem
	Gifts   []struct{ ID int64 }
	Message string
}

func (e *env) offers(token string, query string) offersResp {
	e.t.Helper()
	r := e.do(http.MethodGet, "/api/v1/offers?"+query, token, nil)
	require.Equal(e.t, http.StatusOK, r.Status, string(r.Body))
	var o offersResp
	r.Decode(e.t, &o)
	return o
}

// AC-6: «Рядом» сортирует по расстоянию; «Дешевле» — вариант дешевле; «Со скидкой» — только действующие.
func TestAC6_OfferFilters(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Мама", knownNoRestrictions()...)
	run, _ := e.recommend(giver, e.createOccasion(giver, id, 40000, nil))

	// точка рядом с магазином на Толе би
	near := e.offers(giver, fmt.Sprintf("filter=nearby&lat=43.2531&lon=76.9031&run_id=%d", run.RunID))
	require.NotEmpty(t, near.Items)
	assert.Equal(t, "Букетная на Толе би", near.Items[0].Shop.Name)
	for i := 1; i < len(near.Items); i++ {
		require.NotNil(t, near.Items[i].Shop.DistanceM)
		assert.LessOrEqual(t, *near.Items[i-1].Shop.DistanceM, *near.Items[i].Shop.DistanceM)
	}
	assert.Less(t, *near.Items[0].Shop.DistanceM, 50.0)

	current := run.Items[0]
	cheap := e.offers(giver, fmt.Sprintf("filter=cheaper&bouquet_id=%d&run_id=%d", current.BouquetID, run.RunID))
	require.Len(t, cheap.Items, 1)
	// сравнение по эффективной цене: со скидкой, если она действует (§10)
	effective := func(price float64, discount *float64) float64 {
		if discount != nil {
			return *discount
		}
		return price
	}
	cur, alt := effective(current.Price, current.DiscountPrice), effective(cheap.Items[0].Price, cheap.Items[0].DiscountPrice)
	assert.Less(t, alt, cur)
	require.NotNil(t, cheap.Items[0].PriceDiff)
	assert.InDelta(t, cur-alt, *cheap.Items[0].PriceDiff, 0.001)

	disc := e.offers(giver, fmt.Sprintf("filter=discount&run_id=%d", run.RunID))
	require.NotEmpty(t, disc.Items)
	for _, it := range disc.Items {
		require.NotNil(t, it.DiscountPrice, "только действующие скидки: %s", it.Name)
		assert.Less(t, *it.DiscountPrice, it.Price)
		assert.NotContains(t, []string{"Тюльпаны весенние", "Нарциссы", "Пионы белые"}, it.Name, "истёкшие скидки не показываются")
	}
	assert.NotEmpty(t, disc.Gifts)

	// фильтры комбинируются
	combo := e.offers(giver, fmt.Sprintf("filter=discount,nearby&lat=43.2531&lon=76.9031&run_id=%d", run.RunID))
	require.NotEmpty(t, combo.Items)
	for _, it := range combo.Items {
		assert.NotNil(t, it.DiscountPrice)
		assert.NotNil(t, it.Shop.DistanceM)
	}

	// ошибки параметров
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodGet, fmt.Sprintf("/api/v1/offers?filter=nearby&run_id=%d", run.RunID), giver, nil).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodGet, fmt.Sprintf("/api/v1/offers?filter=cheaper&run_id=%d", run.RunID), giver, nil).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodGet, fmt.Sprintf("/api/v1/offers?filter=teleport&run_id=%d", run.RunID), giver, nil).Status)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, fmt.Sprintf("/api/v1/offers?filter=discount&run_id=%d", run.RunID), e.login("giver2@gulder.local"), nil).Status)
}

// FR-L05: фильтры не отменяют ограничения; FR-L01: координаты не попадают в логи и БД.
func TestFiltersKeepRestrictionsAndDropGeo_FRL05_FRL01(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	attrs := append([]map[string]any{attr("allergies", []string{}, "know"), attr("pets", []string{"cat"}, "know")},
		attr("asthma", false, "know"), attr("scent_sensitivity", "none", "know"))
	id := e.createRecipient(giver, "Кошатник", attrs...)
	run, _ := e.recommend(giver, e.createOccasion(giver, id, 60000, nil))

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(httpapi.LogHandler{Handler: slog.NewJSONHandler(&buf, nil)}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	for _, f := range []string{"nearby", "discount", "nearby,discount"} {
		o := e.offers(giver, fmt.Sprintf("filter=%s&lat=43.238949&lon=76.889709&run_id=%d", f, run.RunID))
		for _, it := range o.Items {
			for _, fl := range it.Items {
				assert.NotContains(t, fl.ToxicFor, "cat", "фильтр %s вернул опасный для кошки букет %s", f, it.Name)
			}
		}
	}
	assert.NotContains(t, buf.String(), "43.238949")
	assert.NotContains(t, buf.String(), "76.889709")

	var hits int
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_events WHERE payload::text LIKE '%43.238949%')
		     + (SELECT count(*) FROM recommendation_runs WHERE filters::text LIKE '%43.238949%' OR results::text LIKE '%43.238949%')`).Scan(&hits))
	assert.Zero(t, hits)
}

// mockGenerator — LLM-мок, который получает кандидатов и возвращает заданные наборы.
type mockGenerator func(p assistant.Prompt) assistant.Proposal

func (m mockGenerator) Propose(_ context.Context, p assistant.Prompt) (assistant.Proposal, error) {
	return m(p), nil
}

type setsResp struct {
	Sets []struct {
		ID         int64
		Title      string
		Generator  string
		TotalPrice float64 `json:"total_price"`
		BudgetLeft float64 `json:"budget_left"`
		Bouquet    struct {
			BouquetID int64 `json:"bouquet_id"`
		}
		Gifts []struct {
			ID    int64
			Name  string
			Price float64
		}
		Reasons  []string
		Warnings []struct{ Code string }
	}
	Generator string
	Message   string
}

func giftID(t *testing.T, e *env, sku string) int64 {
	var id int64
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `SELECT id FROM gift_items WHERE sku=$1`, sku).Scan(&id))
	return id
}

// AC-4, AC-5, FR-G02 по HTTP: мок нарочно превышает бюджет, предлагает аллерген и чужой ID.
func TestAC4_AC5_GiftSetsWithMisbehavingLLM(t *testing.T) {
	var praline int64
	var lastPrompt assistant.Prompt
	gen := mockGenerator(func(p assistant.Prompt) assistant.Proposal {
		lastPrompt = p
		all := []int64{}
		for _, g := range p.Gifts {
			all = append(all, g.ID)
		}
		return assistant.Proposal{Sets: []assistant.ProposedSet{
			{BouquetID: p.Bouquets[0].ID, GiftIDs: all, Title: "Всё сразу"},              // превышает бюджет
			{BouquetID: p.Bouquets[1].ID, GiftIDs: []int64{praline}, Title: "С орехами"}, // аллерген получателя
			{BouquetID: 987654, Title: "Чужой"},                                          // не из каталога
			{BouquetID: p.Bouquets[2].ID, GiftIDs: []int64{}, Title: "Только букет"},     // валидный
		}}
	})
	e := newEnv(t, httpapi.WithGenerator(gen))
	praline = giftID(t, e, "G-AB-03")
	giver := e.login("giver@gulder.local")
	attrs := append([]map[string]any{attr("allergies", []string{"food:nuts"}, "know")}, knownNoRestrictions()[1:]...)
	id := e.createRecipient(giver, "Аллергик", attrs...)
	run, _ := e.recommend(giver, e.createOccasion(giver, id, 25000, nil))

	r := e.do(http.MethodPost, "/api/v1/gift-sets", giver, map[string]any{"run_id": run.RunID, "budget": 25000})
	require.Equal(t, http.StatusCreated, r.Status, string(r.Body))
	var sets setsResp
	r.Decode(t, &sets)
	require.GreaterOrEqual(t, len(sets.Sets), 2)
	assert.Equal(t, "llm", sets.Generator)
	llm := 0
	for _, s := range sets.Sets {
		assert.LessOrEqual(t, s.TotalPrice, 25000.0, "AC-4")
		assert.InDelta(t, 25000-s.TotalPrice, s.BudgetLeft, 0.001)
		for _, g := range s.Gifts {
			assert.NotEqual(t, praline, g.ID, "AC-5")
		}
		assert.NotEqual(t, "Чужой", s.Title, "FR-G02")
		if s.Generator == "llm" {
			llm++
			assert.Equal(t, "Только букет", s.Title)
		}
	}
	assert.Equal(t, 1, llm)
	for _, g := range lastPrompt.Gifts {
		assert.NotEqual(t, praline, g.ID, "запрещённый подарок даже не передаётся LLM")
	}
}

// AC-4 на уровне БД: CHECK (total_price <= budget).
func TestAC4_DBCheckBudget(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "X", knownNoRestrictions()...)
	run, _ := e.recommend(giver, e.createOccasion(giver, id, 20000, nil))
	_, err := e.store.Pool.Exec(context.Background(), `INSERT INTO gift_sets (run_id, budget, bouquet_id, total_price)
		VALUES ($1, 1000, $2, 1000.01)`, run.RunID, run.Items[0].BouquetID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "check")
}

// FR-G01, FR-G04 по HTTP: fallback без LLM, исключение позиции с заменой.
func TestGiftSetsFallbackAndExclude_FRG04(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Подруга", knownNoRestrictions()...)
	run, _ := e.recommend(giver, e.createOccasion(giver, id, 25000, nil))

	r := e.do(http.MethodPost, "/api/v1/gift-sets", giver, map[string]any{"run_id": run.RunID, "budget": 25000})
	require.Equal(t, http.StatusCreated, r.Status, string(r.Body))
	var sets setsResp
	r.Decode(t, &sets)
	assert.Equal(t, "fallback", sets.Generator)
	require.Len(t, sets.Sets, 3)
	set := sets.Sets[0]
	require.NotEmpty(t, set.Gifts)
	assert.NotEmpty(t, set.Reasons)

	removed := set.Gifts[0].ID
	r = e.do(http.MethodPost, fmt.Sprintf("/api/v1/gift-sets/%d/exclude", set.ID), giver, map[string]any{"item_type": "gift", "item_id": removed})
	require.Equal(t, http.StatusCreated, r.Status, string(r.Body))
	var ex struct {
		Set struct {
			ID              int64
			ParentSetID     *int64  `json:"parent_set_id"`
			TotalPrice      float64 `json:"total_price"`
			Gifts           []struct{ ID int64 }
			ExcludedGiftIDs []int64 `json:"excluded_gift_ids"`
		}
		Message string
	}
	r.Decode(t, &ex)
	require.NotNil(t, ex.Set.ParentSetID)
	assert.Equal(t, set.ID, *ex.Set.ParentSetID)
	assert.Equal(t, []int64{removed}, ex.Set.ExcludedGiftIDs)
	for _, g := range ex.Set.Gifts {
		assert.NotEqual(t, removed, g.ID)
	}
	assert.LessOrEqual(t, ex.Set.TotalPrice, 25000.0)

	r = e.do(http.MethodPost, fmt.Sprintf("/api/v1/gift-sets/%d/exclude", set.ID), giver, map[string]any{"item_type": "bouquet", "item_id": set.Bouquet.BouquetID})
	require.Equal(t, http.StatusCreated, r.Status, string(r.Body))

	assert.Equal(t, http.StatusUnprocessableEntity,
		e.do(http.MethodPost, fmt.Sprintf("/api/v1/gift-sets/%d/exclude", set.ID), giver, map[string]any{"item_type": "gift", "item_id": 999999}).Status)
	assert.Equal(t, http.StatusNotFound,
		e.do(http.MethodPost, fmt.Sprintf("/api/v1/gift-sets/%d/exclude", set.ID), e.login("giver2@gulder.local"), map[string]any{"item_type": "gift", "item_id": removed}).Status)

	r = e.do(http.MethodPost, "/api/v1/gift-sets", giver, map[string]any{"run_id": run.RunID, "budget": 100})
	require.Equal(t, http.StatusOK, r.Status)
	assert.NotEmpty(t, r.JSON(t)["message"])
}
