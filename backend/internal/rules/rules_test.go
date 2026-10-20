package rules_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

func hasCode(r rules.Result, code string) bool {
	for _, v := range r.Violations {
		if v.Code == code {
			return true
		}
	}
	return false
}

func ids(cands []rules.Candidate) []int64 {
	out := []int64{}
	for _, c := range cands {
		out = append(out, c.Bouquet.ID)
	}
	return out
}

// §7.7 №1, AC-1: профиль «ограничений нет» и «владелец кошки» дают разные множества допущенных.
func TestAC1_CatOwnerGetsDifferentAllowedSet(t *testing.T) {
	fx := newFixtures(t)
	catalog := []domain.Bouquet{
		bouquet(1, "Розы", 15000, item(fx.rose(), 11)),
		bouquet(2, "Лилии и розы", 20000, item(fx.lily(), 5), item(fx.rose(), 5)),
		bouquet(3, "Герберы", 9000, item(fx.gerbera(), 9)),
		bouquet(4, "Хризантемы", 8000, item(fx.chrysanthemum(), 7)),
		bouquet(5, "Орхидея", 30000, item(fx.orchid(), 1)),
	}
	free, freeExcluded := rules.FilterCandidates(catalog, noRestrictions())
	assert.ElementsMatch(t, []int64{1, 2, 3, 4, 5}, ids(free))
	assert.Empty(t, freeExcluded)

	catOwner := with(noRestrictions(), domain.AttrPets, a([]any{"cat"}, domain.Know))
	allowed, excluded := rules.FilterCandidates(catalog, catOwner)
	assert.ElementsMatch(t, []int64{1, 3, 5}, ids(allowed), "розы, герберы, орхидея допущены")
	excludedIDs := []int64{}
	for _, r := range excluded {
		excludedIDs = append(excludedIDs, r.ItemID)
		assert.True(t, hasCode(r, rules.CodePetToxic))
	}
	assert.ElementsMatch(t, []int64{2, 4}, excludedIDs, "лилии и хризантемы исключены")
	assert.NotEqual(t, ids(free), ids(allowed))
}

// §7.7 №2, AC-2: «не знаю» не считается отсутствием аллергии.
func TestAC2_UnknownAllergyIsNotNoAllergy(t *testing.T) {
	fx := newFixtures(t)
	lilies := bouquet(1, "Лилии", 20000, item(fx.lily(), 7))

	empty := rules.CheckBouquet(lilies, domain.Profile{})
	assert.Equal(t, domain.Warn, empty.Decision())
	assert.Contains(t, empty.MissingInfo, domain.AttrAllergies)

	known := rules.CheckBouquet(lilies, noRestrictions())
	assert.Equal(t, domain.Allow, known.Decision(), "аллергий нет (know) + остальное известно → ALLOW")
	assert.Less(t, empty.ConfidenceMultiplier(), known.ConfidenceMultiplier())

	nullUnknown := rules.CheckBouquet(lilies, with(noRestrictions(), domain.AttrAllergies, a(nil, domain.Unknown)))
	assert.Contains(t, nullUnknown.MissingInfo, domain.AttrAllergies, "null + unknown тоже missing")
	assert.True(t, hasCode(nullUnknown, rules.CodeAllergyUnknown))
	assert.Equal(t, domain.Warn, nullUnknown.Decision())

	// unknown со значением всё равно считается неизвестным
	valued := rules.CheckBouquet(lilies, with(noRestrictions(), domain.AttrAllergies, a([]any{}, domain.Unknown)))
	assert.Contains(t, valued.MissingInfo, domain.AttrAllergies)
}

// §7.7 №3: подарок с неизвестным составом (sweets).
func TestUnknownCompositionGift(t *testing.T) {
	candy := gift(10, "Конфеты весовые", domain.GiftSweets, 3000, nil, domain.LevelNone, boolPtr(false))

	r := rules.CheckGift(candy, domain.Profile{})
	assert.Equal(t, domain.Warn, r.Decision())
	assert.True(t, hasCode(r, rules.CodeUnknownComposition))
	assert.Contains(t, r.MissingInfo, domain.AttrAllergies)

	gluten := with(noRestrictions(), domain.AttrAllergies, a([]any{"food:gluten"}, domain.Know))
	assert.Equal(t, domain.Block, rules.CheckGift(candy, gluten).Decision())

	assert.Equal(t, domain.Allow, rules.CheckGift(candy, noRestrictions()).Decision(), "аллергий нет → ALLOW")
}

// §7.7 №4, AC-3: замены.
func TestAC3_Substitution(t *testing.T) {
	fx := newFixtures(t)
	rose, lily := fx.rose(), fx.lily()
	catOwner := with(noRestrictions(), domain.AttrPets, a([]any{"cat"}, domain.Know))

	roses := bouquet(1, "Розы", 15000, item(rose, 9))
	_, res, err := rules.ValidateSubstitution(roses, rose.ID, lily, catOwner)
	require.NoError(t, err)
	assert.Equal(t, domain.Block, res.Decision())
	assert.True(t, hasCode(res, rules.CodePetToxic))

	mixed := bouquet(2, "Лилии и розы", 20000, item(lily, 5), item(rose, 5))
	nb, res, err := rules.ValidateSubstitution(mixed, lily.ID, rose, catOwner)
	require.NoError(t, err)
	assert.Equal(t, domain.Allow, res.Decision())
	require.Len(t, nb.Items, 1)
	assert.Equal(t, rose.ID, nb.Items[0].Flower.ID)
	assert.Equal(t, 10, nb.Items[0].Stems)
	assert.Len(t, mixed.Items, 2, "исходный букет не меняется")

	unknownPlant := domain.Flower{ID: 999, TradeName: "Протея", Color: "pink", Scent: domain.LevelNone}
	_, res, err = rules.ValidateSubstitution(roses, rose.ID, unknownPlant, noRestrictions())
	require.NoError(t, err)
	assert.Equal(t, domain.Block, res.Decision())
	assert.True(t, hasCode(res, rules.CodeUnverifiedPlant))

	_, _, err = rules.ValidateSubstitution(roses, lily.ID, rose, noRestrictions())
	assert.ErrorIs(t, err, rules.ErrFlowerNotInBouquet)
}

// §7.7 №5, AC-4: сумма набора ≤ бюджета.
func TestAC4_GiftSetBudget(t *testing.T) {
	fx := newFixtures(t)
	roses := bouquet(1, "Розы", 12000, item(fx.rose(), 15))
	bear := gift(20, "Мишка", domain.GiftSoftToy, 6000, codes(), domain.LevelNone, boolPtr(false))
	card := gift(21, "Открытка", domain.GiftCard, 800, codes(), domain.LevelNone, boolPtr(false))
	budget := domain.Tenge(18000)

	over := rules.CheckGiftSet(roses, []domain.GiftItem{bear, card}, budget, noRestrictions())
	assert.Equal(t, domain.Block, over.Decision())
	assert.True(t, hasCode(over, rules.CodeOverBudget))
	assert.Equal(t, domain.Tenge(18800), rules.Total(roses, []domain.GiftItem{bear, card}))

	ok := rules.CheckGiftSet(roses, []domain.GiftItem{bear}, budget, noRestrictions())
	assert.Equal(t, domain.Allow, ok.Decision(), "ровно бюджет — допустимо")
}

// §7.7 №6, AC-5: аллергия food:nuts блокирует конфеты с food:nuts:hazelnut.
func TestAC5_NutAllergyBlocksHazelnutCandies(t *testing.T) {
	praline := gift(30, "Пралине", domain.GiftSweets, 4500, codes("food:nuts:hazelnut", "food:cocoa"), domain.LevelNone, boolPtr(false))
	p := with(noRestrictions(), domain.AttrAllergies, a([]any{"food:nuts"}, domain.Know))
	r := rules.CheckGift(praline, p)
	assert.Equal(t, domain.Block, r.Decision())
	require.True(t, hasCode(r, rules.CodeAllergenMatch))
	assert.Contains(t, r.Violations[0].Message, "food:nuts:hazelnut")

	// и в наборе тоже
	fx := newFixtures(t)
	set := rules.CheckGiftSet(bouquet(1, "Розы", 10000, item(fx.rose(), 9)), []domain.GiftItem{praline}, domain.Tenge(50000), p)
	assert.Equal(t, domain.Block, set.Decision())

	// food:nut не совпадает с food:nuts (совпадение по сегментам, а не по префиксу строки)
	p2 := with(noRestrictions(), domain.AttrAllergies, a([]any{"food:nut"}, domain.Know))
	assert.Equal(t, domain.Allow, rules.CheckGift(praline, p2).Decision())
}

// §7.7 №7: «предполагаю» + кошка → лилии BLOCK.
func TestAssumeIsTreatedAsKnow(t *testing.T) {
	fx := newFixtures(t)
	p := with(noRestrictions(), domain.AttrPets, a([]any{"cat"}, domain.Assume))
	r := rules.CheckBouquet(bouquet(1, "Лилии", 20000, item(fx.lily(), 5)), p)
	assert.Equal(t, domain.Block, r.Decision())
}

// §7.7 №8: питомцы неизвестны — у лилий (severe для кошки) штраф больше, чем у хризантем (mild).
func TestPetsUnknownPenaltyBySeverity(t *testing.T) {
	fx := newFixtures(t)
	p := without(noRestrictions(), domain.AttrPets)
	lilies := rules.CheckBouquet(bouquet(1, "Лилии", 20000, item(fx.lily(), 5)), p)
	chrys := rules.CheckBouquet(bouquet(2, "Хризантемы", 8000, item(fx.chrysanthemum(), 5)), p)
	require.True(t, hasCode(lilies, rules.CodePetsUnknown))
	require.True(t, hasCode(chrys, rules.CodePetsUnknown))
	assert.Less(t, lilies.ConfidenceMultiplier(), chrys.ConfidenceMultiplier())
	assert.InDelta(t, 0.65, lilies.ConfidenceMultiplier(), 1e-9)
	assert.InDelta(t, 0.85, chrys.ConfidenceMultiplier(), 1e-9)
	assert.Contains(t, lilies.MissingInfo, domain.AttrPets)
}

// §7.7 №9: аллергия family:asteraceae блокирует герберы; genus:lilium — нет.
func TestFamilyAllergy(t *testing.T) {
	fx := newFixtures(t)
	gerberas := bouquet(1, "Герберы", 9000, item(fx.gerbera(), 9))
	asteraceae := with(noRestrictions(), domain.AttrAllergies, a([]any{"family:asteraceae"}, domain.Know))
	assert.Equal(t, domain.Block, rules.CheckBouquet(gerberas, asteraceae).Decision())
	lilium := with(noRestrictions(), domain.AttrAllergies, a([]any{"genus:lilium"}, domain.Know))
	assert.Equal(t, domain.Allow, rules.CheckBouquet(gerberas, lilium).Decision())
}

// §7.7 №10: астма блокирует лилии и парфюм; орхидея (без запаха и пыльцы) — ALLOW.
func TestAsthma(t *testing.T) {
	fx := newFixtures(t)
	p := with(noRestrictions(), domain.AttrAsthma, a(true, domain.Know))
	lilies := rules.CheckBouquet(bouquet(1, "Лилии", 20000, item(fx.lily(), 5)), p)
	assert.Equal(t, domain.Block, lilies.Decision())
	assert.True(t, hasCode(lilies, rules.CodeAsthmaTrigger))

	perfume := gift(40, "Парфюм", domain.GiftFragrance, 18000, codes("fragrance"), domain.LevelNone, nil)
	assert.Equal(t, domain.Block, rules.CheckGift(perfume, with(p, domain.AttrPets, a([]any{}, domain.Know))).Decision(),
		"категория fragrance всегда strong")

	orchid := rules.CheckBouquet(bouquet(2, "Орхидея", 30000, item(fx.orchid(), 1)), p)
	assert.Equal(t, domain.Allow, orchid.Decision())

	// S2: астма + пыльца medium (хризантема) → WARN
	chrys := rules.CheckBouquet(bouquet(3, "Хризантемы", 8000, item(fx.chrysanthemum(), 5)), p)
	assert.Equal(t, domain.Warn, chrys.Decision())
	assert.True(t, hasCode(chrys, rules.CodeAsthmaCaution))
}

// §7.7 №11: эвкалипт (запах medium): чувствительность strong → BLOCK, medium → WARN.
func TestEucalyptusScentSensitivity(t *testing.T) {
	fx := newFixtures(t)
	b := bouquet(1, "Эвкалипт", 7000, item(fx.eucalyptus(), 5))
	strong := rules.CheckBouquet(b, with(noRestrictions(), domain.AttrScentSensitivity, a("strong", domain.Know)))
	assert.Equal(t, domain.Block, strong.Decision())
	assert.True(t, hasCode(strong, rules.CodeScentTooStrong))
	medium := rules.CheckBouquet(b, with(noRestrictions(), domain.AttrScentSensitivity, a("medium", domain.Know)))
	assert.Equal(t, domain.Warn, medium.Decision())
	assert.True(t, hasCode(medium, rules.CodeScentCaution))
}

// §7.7 №12: конфеты с PetHazard=true + собака → BLOCK.
func TestPetHazardGiftWithDog(t *testing.T) {
	chocolate := gift(50, "Шоколад", domain.GiftSweets, 2200, codes("food:cocoa"), domain.LevelNone, boolPtr(true))
	p := with(noRestrictions(), domain.AttrPets, a([]any{"dog"}, domain.Know))
	r := rules.CheckGift(chocolate, p)
	assert.Equal(t, domain.Block, r.Decision())
	assert.True(t, hasCode(r, rules.CodePetHazard))
}

// §7.7 №13: кролик → WARN PET_NO_DATA.
func TestRabbitHasNoData(t *testing.T) {
	fx := newFixtures(t)
	p := with(noRestrictions(), domain.AttrPets, a([]any{"rabbit", "parrot"}, domain.Know))
	r := rules.CheckBouquet(bouquet(1, "Розы", 15000, item(fx.rose(), 9)), p)
	assert.Equal(t, domain.Warn, r.Decision())
	assert.True(t, hasCode(r, rules.CodePetNoData))
	assert.Len(t, r.Violations, 1, "дубликаты (code, item, attribute) схлопываются")
	assert.InDelta(t, 0.8, r.ConfidenceMultiplier(), 1e-9)
}

// §7.7 №14: нелюбимая роза → WARN DISLIKED_FLOWER, не BLOCK; штраф 0.
func TestDislikedFlowerIsWarnNotBlock(t *testing.T) {
	fx := newFixtures(t)
	p := with(noRestrictions(), domain.AttrDislikedFlowers, a([]any{"rosa"}, domain.Know))
	p = with(p, domain.AttrDislikedColors, a([]any{"red"}, domain.Know))
	r := rules.CheckBouquet(bouquet(1, "Розы", 15000, item(fx.rose(), 9)), p)
	assert.Equal(t, domain.Warn, r.Decision())
	assert.True(t, hasCode(r, rules.CodeDislikedFlower))
	assert.True(t, hasCode(r, rules.CodeDislikedColor))
	assert.InDelta(t, 1.0, r.ConfidenceMultiplier(), 1e-9)
}

// §7.7 №15: пустой букет → BLOCK.
func TestEmptyBouquetBlocked(t *testing.T) {
	r := rules.CheckBouquet(bouquet(1, "Пусто", 1000), noRestrictions())
	assert.Equal(t, domain.Block, r.Decision())
	assert.True(t, hasCode(r, rules.CodeEmptyComposition))
}

// §7.7 №16: GiverView скрывает ограничения, указанные получателем.
func TestGiverViewHidesRecipientRestrictions(t *testing.T) {
	fx := newFixtures(t)
	lilies := bouquet(1, "Лилии", 20000, item(fx.lily(), 7))

	byRecipient := with(noRestrictions(), domain.AttrAllergies,
		domain.Attribute{Value: []any{"genus:lilium"}, Source: domain.SourceRecipient, Confidence: domain.Know})
	gv := rules.CheckBouquet(lilies, byRecipient).GiverView()
	assert.False(t, gv.Suitable)
	b, err := json.Marshal(gv)
	require.NoError(t, err)
	js := strings.ToLower(string(b))
	assert.NotContains(t, js, "lilium")
	assert.NotContains(t, js, "аллерги")
	require.Len(t, gv.Reasons, 1)
	assert.Equal(t, rules.CodeRecipientRestriction, gv.Reasons[0].Code)

	byGiver := with(noRestrictions(), domain.AttrAllergies, a([]any{"genus:lilium"}, domain.Know))
	gv = rules.CheckBouquet(lilies, byGiver).GiverView()
	assert.False(t, gv.Suitable)
	assert.Equal(t, rules.CodeAllergenMatch, gv.Reasons[0].Code)

	// несколько скрытых нарушений схлопываются в одну причину
	both := with(byRecipient, domain.AttrPets, domain.Attribute{Value: []any{"cat"}, Source: domain.SourceRecipient, Confidence: domain.Know})
	gv = rules.CheckBouquet(lilies, both).GiverView()
	assert.Len(t, gv.Reasons, 1)
}

// §7.7 №17: уточняющий вопрос.
func TestBestClarifyingQuestion(t *testing.T) {
	fx := newFixtures(t)
	catalog := []domain.Bouquet{
		bouquet(1, "Лилии", 20000, item(fx.lily(), 5)),
		bouquet(2, "Хризантемы", 8000, item(fx.chrysanthemum(), 5)),
		bouquet(3, "Розы", 15000, item(fx.rose(), 9)),
	}
	results := func(p domain.Profile) []rules.Result {
		allowed, _ := rules.FilterCandidates(catalog, p)
		out := []rules.Result{}
		for _, c := range allowed {
			out = append(out, c.Result)
		}
		return out
	}
	q := rules.BestClarifyingQuestion(results(without(noRestrictions(), domain.AttrPets)))
	require.NotNil(t, q)
	assert.Equal(t, domain.AttrPets, q.Attribute)
	assert.Equal(t, "Есть ли у получателя дома кошка, собака или другие животные?", q.Text)
	assert.InDelta(t, 0.5, q.Gain, 1e-9)

	assert.Nil(t, rules.BestClarifyingQuestion(results(noRestrictions())))

	// пустой профиль: питомцы (0.35+0.15) выигрывают у аллергий (0.15+0.15) и астмы
	q = rules.BestClarifyingQuestion(results(domain.Profile{}))
	require.NotNil(t, q)
	assert.Equal(t, domain.AttrPets, q.Attribute)
	assert.Equal(t, rules.QuestionText(domain.AttrAllergies), "Есть ли у получателя аллергия на цветы, пыльцу или продукты?")
	assert.Empty(t, rules.QuestionText("style"))
}
