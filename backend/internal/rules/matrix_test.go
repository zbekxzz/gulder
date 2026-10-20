package rules_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

// S1–S7: полная матрица «чувствительность × запах» и «астма × запах/пыльца» (§7.4).
func TestScentAndAsthmaMatrix(t *testing.T) {
	lv := []domain.Level{domain.LevelNone, domain.LevelLow, domain.LevelMedium, domain.LevelStrong}
	// чувствительность → запах → ожидаемое решение и код
	sensitivity := map[string]map[domain.Level]string{
		"strong":  {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: rules.CodeScentTooStrong, domain.LevelStrong: rules.CodeScentTooStrong},
		"medium":  {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: rules.CodeScentCaution, domain.LevelStrong: rules.CodeScentTooStrong},
		"low":     {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: "", domain.LevelStrong: rules.CodeScentCaution},
		"none":    {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: "", domain.LevelStrong: ""},
		"unknown": {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: "", domain.LevelStrong: rules.CodeScentUnknown},
	}
	decisionOf := map[string]domain.Decision{
		"": domain.Allow, rules.CodeScentTooStrong: domain.Block, rules.CodeScentCaution: domain.Warn, rules.CodeScentUnknown: domain.Warn,
	}
	for sens, byScent := range sensitivity {
		for _, scent := range lv {
			t.Run(fmt.Sprintf("sens=%s/scent=%s", sens, scent), func(t *testing.T) {
				p := noRestrictions()
				if sens == "unknown" {
					p = without(p, domain.AttrScentSensitivity)
				} else {
					p[domain.AttrScentSensitivity] = a(sens, domain.Know)
				}
				p[domain.AttrAsthma] = a(false, domain.Know)
				candle := gift(1, "Свеча", domain.GiftCandle, 3000, codes(), scent, boolPtr(false))
				r := rules.CheckGift(candle, p)
				want := byScent[scent]
				assert.Equal(t, decisionOf[want], r.Decision())
				if want != "" {
					assert.True(t, hasCode(r, want), r.Codes())
				}
				if want == rules.CodeScentUnknown {
					assert.Contains(t, r.MissingInfo, domain.AttrScentSensitivity)
				}
			})
		}
	}

	asthma := map[string]map[domain.Level]string{
		"true":    {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: rules.CodeAsthmaCaution, domain.LevelStrong: rules.CodeAsthmaTrigger},
		"false":   {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: "", domain.LevelStrong: ""},
		"unknown": {domain.LevelNone: "", domain.LevelLow: "", domain.LevelMedium: "", domain.LevelStrong: rules.CodeAsthmaUnknown},
	}
	asthmaDecision := map[string]domain.Decision{
		"": domain.Allow, rules.CodeAsthmaCaution: domain.Warn, rules.CodeAsthmaTrigger: domain.Block, rules.CodeAsthmaUnknown: domain.Warn,
	}
	fx := newFixtures(t)
	for state, byPollen := range asthma {
		for _, pollen := range lv {
			t.Run(fmt.Sprintf("asthma=%s/pollen=%s", state, pollen), func(t *testing.T) {
				p := noRestrictions()
				switch state {
				case "true":
					p[domain.AttrAsthma] = a(true, domain.Know)
				case "false":
					p[domain.AttrAsthma] = a(false, domain.Know)
				default:
					p = without(p, domain.AttrAsthma)
				}
				// растение без запаха, без токсичности, вне рисковых семейств: проверяется только пыльца
				taxon := *fx.taxa["rosa"]
				taxon.Pollen = pollen
				f := domain.Flower{ID: 1, TradeName: "Тест", Color: "white", Scent: domain.LevelNone, Taxon: &taxon, TaxonConfirmed: true}
				r := rules.CheckBouquet(bouquet(1, "Тест", 1000, item(f, 3)), p)
				want := byPollen[pollen]
				assert.Equal(t, asthmaDecision[want], r.Decision(), r.Codes())
				if want != "" {
					assert.True(t, hasCode(r, want), r.Codes())
				}
			})
		}
	}
}

// G1–G7: правила подарка (§7.5).
func TestGiftRules(t *testing.T) {
	known := noRestrictions()
	unknownAllergies := without(known, domain.AttrAllergies)
	nuts := with(known, domain.AttrAllergies, a([]any{"food:nuts"}, domain.Know))
	dog := with(known, domain.AttrPets, a([]any{"dog"}, domain.Know))
	unknownPets := without(known, domain.AttrPets)

	cases := []struct {
		name     string
		g        domain.GiftItem
		p        domain.Profile
		decision domain.Decision
		code     string
		missing  string
	}{
		{"G1 unknown composition + known allergies", gift(1, "Трюфели", domain.GiftSweets, 1, nil, domain.LevelNone, boolPtr(false)), nuts, domain.Block, rules.CodeUnknownComposition, ""},
		{"G2 unknown composition, unknown allergies, candle", gift(1, "Свеча", domain.GiftCandle, 1, nil, domain.LevelNone, boolPtr(false)), unknownAllergies, domain.Warn, rules.CodeUnknownComposition, domain.AttrAllergies},
		{"G2 not for soft toys", gift(1, "Мишка", domain.GiftSoftToy, 1, nil, domain.LevelNone, boolPtr(false)), unknownAllergies, domain.Allow, "", ""},
		{"G3 allergen match", gift(1, "Пряники", domain.GiftSweets, 1, codes("food:gluten", "food:nuts:almond"), domain.LevelNone, boolPtr(false)), nuts, domain.Block, rules.CodeAllergenMatch, ""},
		{"G3 no match", gift(1, "Мёд", domain.GiftSweets, 1, codes("food:honey"), domain.LevelNone, boolPtr(false)), nuts, domain.Allow, "", ""},
		{"G4 allergens + unknown allergies", gift(1, "Мёд", domain.GiftSweets, 1, codes("food:honey"), domain.LevelNone, boolPtr(false)), unknownAllergies, domain.Warn, rules.CodeAllergyUnknown, domain.AttrAllergies},
		{"G4 verified no allergens", gift(1, "Открытка", domain.GiftCard, 1, codes(), domain.LevelNone, boolPtr(false)), unknownAllergies, domain.Allow, "", ""},
		{"G5 pet hazard + dog", gift(1, "Шоколад", domain.GiftSweets, 1, codes(), domain.LevelNone, boolPtr(true)), dog, domain.Block, rules.CodePetHazard, ""},
		{"G5 pet hazard + no pets", gift(1, "Шоколад", domain.GiftSweets, 1, codes(), domain.LevelNone, boolPtr(true)), known, domain.Allow, "", ""},
		{"G6 unknown hazard + dog", gift(1, "Свеча", domain.GiftCandle, 1, codes(), domain.LevelNone, nil), dog, domain.Warn, rules.CodePetHazard, ""},
		{"G7 hazard + unknown pets", gift(1, "Шоколад", domain.GiftSweets, 1, codes(), domain.LevelNone, boolPtr(true)), unknownPets, domain.Warn, rules.CodePetsUnknown, domain.AttrPets},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := rules.CheckGift(c.g, c.p)
			assert.Equal(t, c.decision, r.Decision(), r.Codes())
			if c.code != "" {
				assert.True(t, hasCode(r, c.code), r.Codes())
			}
			if c.missing != "" {
				assert.Contains(t, r.MissingInfo, c.missing)
			}
		})
	}
}

func TestGiftSetDuplicatesAndMerge(t *testing.T) {
	fx := newFixtures(t)
	b := bouquet(1, "Лилии", 10000, item(fx.lily(), 5))
	card := gift(2, "Открытка", domain.GiftCard, 500, codes(), domain.LevelNone, boolPtr(false))
	honey := gift(3, "Мёд", domain.GiftSweets, 500, codes("food:honey"), domain.LevelNone, boolPtr(false))

	dup := rules.CheckGiftSet(b, []domain.GiftItem{card, card}, domain.Tenge(100000), noRestrictions())
	assert.Equal(t, domain.Block, dup.Decision())
	assert.True(t, hasCode(dup, rules.CodeDuplicateItem))
	assert.Equal(t, rules.KindGiftSet, dup.Kind)

	// missing_info букета и подарков объединяются как множество
	set := rules.CheckGiftSet(b, []domain.GiftItem{honey}, domain.Tenge(100000), without(noRestrictions(), domain.AttrAllergies))
	assert.Equal(t, []string{domain.AttrAllergies}, set.MissingInfo)
	assert.Equal(t, domain.Warn, set.Decision())
	assert.Len(t, set.Codes(), 2)
}

func TestPollenAllergyAndExtraAllergenCodes(t *testing.T) {
	fx := newFixtures(t)
	pollen := with(noRestrictions(), domain.AttrAllergies, a([]any{"pollen"}, domain.Know))
	assert.Equal(t, domain.Block, rules.CheckBouquet(bouquet(1, "Хризантемы", 1, item(fx.chrysanthemum(), 3)), pollen).Decision(),
		"пыльца medium даёт ключ pollen")
	assert.Equal(t, domain.Allow, rules.CheckBouquet(bouquet(2, "Розы", 1, item(fx.rose(), 3)), pollen).Decision(),
		"пыльца low — нет ключа pollen")

	taxon := *fx.taxa["rosa"]
	taxon.AllergenCodes = []string{"Latex"}
	f := domain.Flower{ID: 9, TradeName: "Роза в латексной обмотке", Color: "red", Scent: domain.LevelLow, Taxon: &taxon, TaxonConfirmed: true}
	latex := with(noRestrictions(), domain.AttrAllergies, a([]any{"latex"}, domain.Know))
	assert.Equal(t, domain.Block, rules.CheckBouquet(bouquet(3, "Латекс", 1, item(f, 1)), latex).Decision())
}

func TestSubstitutionIntoSamePositionAndConfidenceFloor(t *testing.T) {
	fx := newFixtures(t)
	rose, gerbera, orchid := fx.rose(), fx.gerbera(), fx.orchid()
	b := bouquet(1, "Микс", 1, item(rose, 3), item(gerbera, 4))
	nb, res, err := rules.ValidateSubstitution(b, rose.ID, orchid, noRestrictions())
	require.NoError(t, err)
	assert.Equal(t, domain.Allow, res.Decision())
	require.Len(t, nb.Items, 2)
	assert.Equal(t, orchid.ID, nb.Items[0].Flower.ID, "замена встаёт на место заменяемого")
	assert.Equal(t, 3, nb.Items[0].Stems)

	// множитель уверенности не опускается ниже 0.2
	many := domain.Profile{}
	items := []domain.BouquetItem{}
	for i := 0; i < 6; i++ {
		l := fx.lily()
		l.TradeName = fmt.Sprintf("Лилия %d", i)
		items = append(items, item(l, 1))
	}
	assert.InDelta(t, 0.2, rules.CheckBouquet(bouquet(2, "Много", 1, items...), many).ConfidenceMultiplier(), 1e-9)
}
