package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/recipients"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

var (
	rosa   = &domain.PlantTaxon{Genus: "Rosa", Family: "Rosaceae", Pollen: domain.LevelLow, PetToxicity: map[string]domain.Toxicity{"cat": "none", "dog": "none", "horse": "none"}}
	gerb   = &domain.PlantTaxon{Genus: "Gerbera", Family: "Asteraceae", Pollen: domain.LevelLow, PetToxicity: map[string]domain.Toxicity{"cat": "none", "dog": "none", "horse": "none"}}
	orchid = &domain.PlantTaxon{Genus: "Phalaenopsis", Family: "Orchidaceae", Pollen: domain.LevelNone, PetToxicity: map[string]domain.Toxicity{"cat": "none", "dog": "none", "horse": "none"}}
	now    = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
)

func bouquet(id int64, price int64, t *domain.PlantTaxon) domain.Bouquet {
	f := domain.Flower{ID: id, TradeName: t.Genus, Color: "pink", Scent: domain.LevelLow, Taxon: t, TaxonConfirmed: true}
	return domain.Bouquet{
		ID: id, ShopID: id, Name: t.Genus, Price: domain.Tenge(price), Offer: domain.PriceOffer{Price: domain.Tenge(price)},
		Style: "classic", CompositionFormat: "bouquet", InStock: true, Items: []domain.BouquetItem{{Flower: f, Stems: 9}},
	}
}

func codes(c ...string) *[]string { return &c }

func gift(id int64, name, cat string, price int64, allergens *[]string) domain.GiftItem {
	f := false
	return domain.GiftItem{
		ID: id, Name: name, Category: cat, Price: domain.Tenge(price), AllergenCodes: allergens,
		Scent: domain.LevelNone, PetHazard: &f, InStock: true,
	}
}

func know(v any) domain.Attribute {
	return domain.Attribute{Value: v, Confidence: domain.Know, Source: domain.SourceGiver}
}

// nutAllergic — получатель с аллергией на орехи, остальное известно.
func nutAllergic() domain.Profile {
	return domain.Profile{
		domain.AttrAllergies: know([]any{"food:nuts"}), domain.AttrPets: know([]any{}),
		domain.AttrAsthma: know(false), domain.AttrScentSensitivity: know("none"),
	}
}

func testContext(t *testing.T, p domain.Profile) *recommend.Context {
	t.Helper()
	snap := catalog.Snapshot{
		Now: now,
		Bouquets: []domain.Bouquet{
			bouquet(1, 12000, rosa), bouquet(2, 9000, gerb), bouquet(3, 15000, orchid), bouquet(4, 30000, rosa),
		},
		Gifts: []domain.GiftItem{
			gift(101, "Открытка", domain.GiftCard, 800, codes()),
			gift(102, "Мишка", domain.GiftSoftToy, 6000, codes()),
			gift(103, "Пралине с фундуком", domain.GiftSweets, 4500, codes("food:nuts:hazelnut")),
			gift(104, "Мёд", domain.GiftSweets, 3000, codes("food:honey")),
			gift(105, "Свеча без запаха", domain.GiftCandle, 2500, codes()),
			gift(106, "Трюфели (состав неизвестен)", domain.GiftSweets, 2000, nil),
		},
		Shops: map[int64]catalog.Shop{},
	}
	c := &recommend.Context{
		Occasion: recipients.Occasion{Type: "birthday", BudgetMax: domain.Tenge(20000)},
		Profile:  p, Snapshot: snap, Now: now,
	}
	c.Allowed, c.Excluded = rules.FilterCandidates(snap.Bouquets, p)
	c.Scored = ranking.DefaultConfig().Score(ranking.Input{
		Candidates: c.Allowed, Profile: p,
		Occasion: recommend.RankingOccasion(c.Occasion, now), Now: now,
	})
	return c
}

// mockLLM возвращает заранее заданные ответы и запоминает запросы.
type mockLLM struct {
	answers []Proposal
	err     error
	prompts []Prompt
}

func (m *mockLLM) Propose(_ context.Context, p Prompt) (Proposal, error) {
	m.prompts = append(m.prompts, p)
	if m.err != nil {
		return Proposal{}, m.err
	}
	i := min(len(m.prompts)-1, len(m.answers)-1)
	return m.answers[i], nil
}

func setIDs(r Result) []int64 {
	var out []int64
	for _, s := range r.Sets {
		out = append(out, s.Bouquet.ID)
	}
	return out
}

// AC-4: LLM нарочно превышает бюджет → набор отброшен; сумма каждого набора ≤ бюджета.
func TestAC4_LLMOverBudgetSetDropped(t *testing.T) {
	c := testContext(t, nutAllergic())
	llm := &mockLLM{answers: []Proposal{{Sets: []ProposedSet{
		{BouquetID: 3, GiftIDs: []int64{102, 105}, Title: "Слишком дорого"}, // 15000+6000+2500 > 20000
		{BouquetID: 1, GiftIDs: []int64{101, 102}, Title: "Классика"},       // 18800
		{BouquetID: 2, GiftIDs: []int64{105, 101}, Title: "Ярко"},           // 12300
	}}}}
	res, err := Assemble(context.Background(), llm, c, domain.Tenge(20000))
	require.NoError(t, err)
	assert.Equal(t, GeneratorLLM, res.Generator)
	assert.ElementsMatch(t, []int64{1, 2}, setIDs(res))
	for _, s := range res.Sets {
		assert.LessOrEqual(t, s.Total, domain.Tenge(20000))
		assert.Equal(t, domain.Tenge(20000)-s.Total, s.BudgetLeft, "остаток считает код")
	}
	assert.Len(t, res.Rejected, 1)
	assert.Len(t, llm.prompts, 1, "двух валидных наборов достаточно — повтора нет")
}

// AC-5: подарок с аллергеном получателя не попадает в набор, даже если LLM его предлагает.
func TestAC5_LLMForbiddenGiftDropped(t *testing.T) {
	c := testContext(t, nutAllergic())
	llm := &mockLLM{answers: []Proposal{{Sets: []ProposedSet{
		{BouquetID: 1, GiftIDs: []int64{103}, Title: "С пралине"},
		{BouquetID: 2, GiftIDs: []int64{106}, Title: "С трюфелями"}, // неизвестный состав при известной аллергии
	}}}}
	res, err := Assemble(context.Background(), llm, c, domain.Tenge(20000))
	require.NoError(t, err)
	for _, s := range res.Sets {
		for _, g := range s.Gifts {
			assert.NotEqual(t, int64(103), g.ID)
			assert.NotEqual(t, int64(106), g.ID)
		}
	}
	// запрещённые подарки даже не передаются LLM
	for _, g := range llm.prompts[0].Gifts {
		assert.NotContains(t, []int64{103, 106}, g.ID)
	}
	assert.Len(t, llm.prompts, 2, "после отбраковки — один повторный запрос с ошибками")
	assert.NotEmpty(t, llm.prompts[1].Errors)
	assert.GreaterOrEqual(t, len(res.Sets), MinSets, "fallback добирает недостающие наборы")
}

// FR-G02: несуществующий ID → набор отброшен.
func TestFRG02_UnknownIDDropped(t *testing.T) {
	c := testContext(t, nutAllergic())
	llm := &mockLLM{answers: []Proposal{
		{Sets: []ProposedSet{{BouquetID: 999, Title: "Чужой"}, {BouquetID: 1, GiftIDs: []int64{12345}, Title: "Чужой подарок"}}},
		{Sets: []ProposedSet{{BouquetID: 1, GiftIDs: []int64{101}, Title: "А"}, {BouquetID: 2, GiftIDs: []int64{102}, Title: "Б"}}},
	}}
	res, err := Assemble(context.Background(), llm, c, domain.Tenge(20000))
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{1, 2}, setIDs(res))
	assert.Equal(t, GeneratorLLM, res.Generator)
	require.Len(t, llm.prompts, 2)
	assert.Len(t, llm.prompts[1].Errors, 2)
	// букет дороже бюджета не предлагается LLM (§9.1)
	for _, b := range llm.prompts[0].Bouquets {
		assert.NotEqual(t, int64(4), b.ID)
	}
}

// §9.4: без LLM (или при его ошибке) — детерминированный fallback; открытка в приоритете.
func TestFallbackDeterministic(t *testing.T) {
	c := testContext(t, nutAllergic())
	a, err := Assemble(context.Background(), nil, c, domain.Tenge(20000))
	require.NoError(t, err)
	b, err := Assemble(context.Background(), &mockLLM{err: errors.New("timeout")}, c, domain.Tenge(20000))
	require.NoError(t, err)
	assert.Equal(t, GeneratorFallback, a.Generator)
	assert.Equal(t, GeneratorFallback, b.Generator)
	require.Len(t, a.Sets, 3)
	assert.Equal(t, setIDs(a), setIDs(b))
	titles := map[string]bool{}
	for _, s := range a.Sets {
		titles[s.Title] = true
		assert.LessOrEqual(t, s.Total, domain.Tenge(20000))
		assert.Equal(t, int64(101), s.Gifts[0].ID, "открытка — первая")
		cats := map[string]bool{}
		for _, g := range s.Gifts {
			assert.False(t, cats[g.Category], "подарки из разных категорий")
			cats[g.Category] = true
			assert.NotContains(t, []int64{103, 106}, g.ID)
		}
		assert.Equal(t, GeneratorFallback, s.Generator)
	}
	assert.Len(t, titles, 3, "разные характеры")

	_, err = Assemble(context.Background(), nil, c, domain.Tenge(1000))
	assert.ErrorIs(t, err, ErrNoCandidates)
}

// §9.3: наборы отличаются букетом или минимум двумя подарками.
func TestDuplicateSetsRejected(t *testing.T) {
	c := testContext(t, nutAllergic())
	llm := &mockLLM{answers: []Proposal{{Sets: []ProposedSet{
		{BouquetID: 1, GiftIDs: []int64{101, 102}},
		{BouquetID: 1, GiftIDs: []int64{101}}, // отличается одним подарком
		{BouquetID: 1, GiftIDs: []int64{105}}, // отличается тремя
	}}}}
	res, err := Assemble(context.Background(), llm, c, domain.Tenge(20000))
	require.NoError(t, err)
	llmSets := 0
	for _, s := range res.Sets {
		if s.Generator == GeneratorLLM {
			llmSets++
		}
	}
	assert.Equal(t, 2, llmSets)
}

// FR-G04: исключение позиции — замена в пределах оставшегося бюджета с повторной проверкой.
func TestFRG04_ExcludeGift(t *testing.T) {
	c := testContext(t, nutAllergic())
	res, err := Exclude(c, ExcludeRequest{BouquetID: 1, GiftIDs: []int64{101, 102}, Budget: domain.Tenge(20000), ItemType: ItemGift, ItemID: 102})
	require.NoError(t, err)
	assert.Empty(t, res.Message)
	gifts := ids(res.Set.Gifts)
	assert.Contains(t, gifts, int64(101))
	assert.NotContains(t, gifts, int64(102))
	assert.Len(t, gifts, 2, "подобрана замена")
	assert.LessOrEqual(t, res.Set.Total, domain.Tenge(20000))
	assert.Equal(t, []int64{102}, res.ExcludedGifts)

	// исключённое не возвращается и в следующей итерации
	next, err := Exclude(c, ExcludeRequest{
		BouquetID: 1, GiftIDs: gifts, Budget: domain.Tenge(20000),
		ExcludedGifts: res.ExcludedGifts, ItemType: ItemGift, ItemID: gifts[1],
	})
	require.NoError(t, err)
	assert.NotContains(t, ids(next.Set.Gifts), int64(102))
	assert.ElementsMatch(t, []int64{102, gifts[1]}, next.ExcludedGifts)

	// замены нет — набор без позиции и сообщение
	tight, err := Exclude(c, ExcludeRequest{BouquetID: 1, GiftIDs: []int64{101}, Budget: domain.Tenge(13000), ItemType: ItemGift, ItemID: 101})
	require.NoError(t, err)
	assert.Empty(t, tight.Set.Gifts)
	assert.NotEmpty(t, tight.Message)
}

func TestFRG04_ExcludeBouquet(t *testing.T) {
	c := testContext(t, nutAllergic())
	res, err := Exclude(c, ExcludeRequest{BouquetID: 1, GiftIDs: []int64{101}, Budget: domain.Tenge(20000), ItemType: ItemBouquet, ItemID: 1})
	require.NoError(t, err)
	assert.NotEqual(t, int64(1), res.Set.Bouquet.ID)
	assert.Equal(t, []int64{101}, ids(res.Set.Gifts))
	assert.Equal(t, []int64{1}, res.ExcludedBouquets)

	_, err = Exclude(c, ExcludeRequest{BouquetID: 1, GiftIDs: []int64{101}, Budget: domain.Tenge(20000), ItemType: ItemGift, ItemID: 555})
	assert.ErrorIs(t, err, ErrItemNotInSet)
	_, err = Exclude(c, ExcludeRequest{BouquetID: 2, Budget: domain.Tenge(9500), ExcludedBouquets: []int64{1, 3}, ItemType: ItemBouquet, ItemID: 2})
	assert.ErrorIs(t, err, ErrNoBouquetReplacement)
}
