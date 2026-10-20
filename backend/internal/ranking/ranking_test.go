package ranking

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

var (
	rosaTaxon   = &domain.PlantTaxon{Genus: "Rosa", Family: "Rosaceae", Pollen: domain.LevelLow, PetToxicity: map[string]domain.Toxicity{"cat": "none", "dog": "none", "horse": "none"}}
	liliumTaxon = &domain.PlantTaxon{Genus: "Lilium", Family: "Liliaceae", Pollen: domain.LevelStrong, PetToxicity: map[string]domain.Toxicity{"cat": "severe", "dog": "none", "horse": "none"}}
	orchidTaxon = &domain.PlantTaxon{Genus: "Phalaenopsis", Family: "Orchidaceae", Pollen: domain.LevelNone, PetToxicity: map[string]domain.Toxicity{"cat": "none", "dog": "none", "horse": "none"}}
	tulipTaxon  = &domain.PlantTaxon{Genus: "Tulipa", Family: "Liliaceae", Pollen: domain.LevelLow, PetToxicity: map[string]domain.Toxicity{"cat": "mild", "dog": "mild", "horse": "mild"}}
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func fl(id int64, t *domain.PlantTaxon, color string, scent domain.Level) domain.Flower {
	return domain.Flower{ID: id, TradeName: t.Genus, Color: color, Scent: scent, Taxon: t, TaxonConfirmed: true}
}

func bq(id, shop int64, price int64, style, format string, items ...domain.BouquetItem) domain.Bouquet {
	return domain.Bouquet{
		ID: id, ShopID: shop, Name: "B", Price: domain.Tenge(price), Offer: domain.PriceOffer{Price: domain.Tenge(price)},
		Style: style, CompositionFormat: format, InStock: true, Items: items,
	}
}

func it(f domain.Flower, stems int) domain.BouquetItem {
	return domain.BouquetItem{Flower: f, Stems: stems}
}

func know(v any) domain.Attribute {
	return domain.Attribute{Value: v, Confidence: domain.Know, Source: domain.SourceGiver}
}

func fullProfile() domain.Profile {
	return domain.Profile{
		domain.AttrAllergies: know([]any{}), domain.AttrPets: know([]any{}),
		domain.AttrAsthma: know(false), domain.AttrScentSensitivity: know("none"),
	}
}

func candidates(p domain.Profile, bouquets ...domain.Bouquet) []rules.Candidate {
	allowed, _ := rules.FilterCandidates(bouquets, p)
	return allowed
}

func TestNumerologyNumber_FRN02(t *testing.T) {
	d := func(s string) time.Time {
		v, err := time.Parse("2006-01-02", s)
		require.NoError(t, err)
		return v
	}
	assert.Equal(t, 1, NumerologyNumber(d("1990-03-15")), "28 → 10 → 1")
	assert.Equal(t, 11, NumerologyNumber(d("2000-01-08")), "сумма 11 не сводится")
	assert.Equal(t, 22, NumerologyNumber(d("2009-09-02")), "сумма 22 не сводится")
	assert.Equal(t, 33, NumerologyNumber(d("1950-09-09")), "сумма 33 не сводится")
	assert.Equal(t, 11, NumerologyNumber(d("1990-02-08")), "29 → 11 — мастер-число после сведения")
	assert.Equal(t, 3, NumerologyNumber(d("1999-01-01")), "30 → 3")
	for y := 1920; y < 2030; y++ {
		for _, md := range [][2]int{{1, 1}, {6, 15}, {12, 31}} {
			n := NumerologyNumber(time.Date(y, time.Month(md[0]), md[1], 0, 0, 0, 0, time.UTC))
			assert.True(t, (n >= 1 && n <= 9) || n == 11 || n == 22 || n == 33, n)
		}
	}
}

func TestDefaultConfigValid(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, cfg.Validate())
	assert.Equal(t, "baseline-0.1", cfg.ModelVersion)
	assert.LessOrEqual(t, cfg.NumerologyWeight, MaxNumerologyWeight)

	bad := cfg
	bad.NumerologyWeight = 0.2
	assert.Error(t, bad.Validate(), "FR-N04: вес нумерологии ≤ 15%")
	_, err := ParseConfig([]byte("model_version: x\nweights: {flower_match: 1}\n"))
	assert.Error(t, err)
}

// FR-R02, FR-N04: эффективные веса нормированы; нейтральность растёт с числом неизвестных полей.
func TestEffectiveWeights(t *testing.T) {
	cfg := DefaultConfig()
	sum := func(w map[string]float64) float64 {
		s := 0.0
		for _, v := range w {
			s += v
		}
		return s
	}
	w0 := cfg.EffectiveWeights(0, false)
	w4 := cfg.EffectiveWeights(4, false)
	assert.InDelta(t, 1, sum(w0), 1e-9)
	assert.InDelta(t, 1, sum(w4), 1e-9)
	assert.InDelta(t, 0.05, w0[FactorNeutrality], 1e-9)
	assert.InDelta(t, 0.25/1.2, w4[FactorNeutrality], 1e-9)
	assert.Greater(t, w4[FactorNeutrality], w0[FactorNeutrality])

	wn := cfg.EffectiveWeights(0, true)
	assert.InDelta(t, 1, sum(wn), 1e-9)
	assert.InDelta(t, cfg.NumerologyWeight, wn[FactorNumerology], 1e-9)
	assert.LessOrEqual(t, wn[FactorNumerology], MaxNumerologyWeight)
	assert.NotContains(t, w0, FactorNumerology)
}

// AC-7: нумерология учитывает число, но безопасность не меняется; выключена — фактор не применяется.
func TestAC7_NumerologyAffectsScoreNotSafety(t *testing.T) {
	cfg := DefaultConfig()
	catOwner := fullProfile()
	catOwner[domain.AttrPets] = know([]any{"cat"})
	catOwner[domain.AttrBirthDate] = know("1990-03-15") // число 1

	bouquets := []domain.Bouquet{
		bq(1, 1, 15000, "classic", "bouquet", it(fl(1, rosaTaxon, "white", domain.LevelLow), 9)),
		bq(2, 2, 15000, "classic", "bouquet", it(fl(2, orchidTaxon, "white", domain.LevelNone), 1)),
		bq(3, 3, 15000, "classic", "bouquet", it(fl(3, liliumTaxon, "white", domain.LevelStrong), 5)),
	}
	entry := &NumerologyEntry{VersionID: 1, Number: 1, Flowers: []string{"phalaenopsis"}, Colors: []string{"yellow"}, Meaning: "Начало"}
	occ := Occasion{Type: "birthday", BudgetMax: domain.Tenge(20000), Date: now}

	allowedOff, excludedOff := rules.FilterCandidates(bouquets, catOwner)
	occOn := occ
	occOn.NumerologyEnabled = true
	// Rules Engine вызывается до ranker'а и не знает о нумерологии: множество excluded одно и то же
	allowedOn, excludedOn := rules.FilterCandidates(bouquets, catOwner)
	assert.Equal(t, excludedOff, excludedOn)
	require.Len(t, excludedOff, 1)
	assert.Equal(t, int64(3), excludedOff[0].ItemID, "лилии исключены при любом режиме")

	off := cfg.Score(Input{Candidates: allowedOff, Profile: catOwner, Occasion: occ, Numerology: entry, Now: now})
	on := cfg.Score(Input{Candidates: allowedOn, Profile: catOwner, Occasion: occOn, Numerology: entry, Now: now})

	for _, s := range off {
		assert.NotContains(t, s.Factors, FactorNumerology, "выключена — фактор не применяется")
	}
	byID := func(list []Scored, id int64) Scored {
		for _, s := range list {
			if s.Bouquet.ID == id {
				return s
			}
		}
		t.Fatalf("bouquet %d not scored", id)
		return Scored{}
	}
	assert.InDelta(t, 1, byID(on, 2).Factors[FactorNumerology], 1e-9, "орхидея — цветок числа 1")
	assert.InDelta(t, 0, byID(on, 1).Factors[FactorNumerology], 1e-9)
	gapOff := byID(off, 2).Score - byID(off, 1).Score
	gapOn := byID(on, 2).Score - byID(on, 1).Score
	assert.Greater(t, gapOn, gapOff, "фактор учитывает число получателя")
	assert.LessOrEqual(t, gapOn-gapOff, MaxNumerologyWeight+1e-9, "вклад не больше 15%")
	assert.Contains(t, byID(on, 2).Reasons, "Нумерология (развлекательная функция): число 1")

	// включена, но таблицы для числа нет — фактор не применяется
	none := cfg.Score(Input{Candidates: allowedOn, Profile: catOwner, Occasion: occOn, Numerology: nil, Now: now})
	for i := range none {
		assert.InDelta(t, off[i].Score, none[i].Score, 1e-12)
	}
}

// §8.2: не больше 2 букетов из магазина и 3 с одним доминирующим родом; дороже бюджета — не в Top-5.
func TestSelectTopDiversity(t *testing.T) {
	cfg := DefaultConfig()
	p := fullProfile()
	rose := fl(1, rosaTaxon, "pink", domain.LevelLow)
	tulip := fl(2, tulipTaxon, "pink", domain.LevelLow)
	var bouquets []domain.Bouquet
	id := int64(0)
	for shop := int64(1); shop <= 3; shop++ {
		for k := 0; k < 3; k++ {
			id++
			bouquets = append(bouquets, bq(id, shop, 10000, "bright", "bouquet", it(rose, 9)))
		}
	}
	bouquets = append(bouquets,
		bq(100, 4, 10000, "bright", "bouquet", it(tulip, 9)),
		bq(101, 5, 99000, "bright", "bouquet", it(tulip, 9)), // дороже бюджета
	)
	scored := cfg.Score(Input{
		Candidates: candidates(p, bouquets...), Profile: p,
		Occasion: Occasion{Type: "birthday", BudgetMax: domain.Tenge(20000)}, Now: now,
	})
	top := SelectTop(scored, 5)
	shops := map[int64]int{}
	genera := map[string]int{}
	for _, s := range top {
		shops[s.Bouquet.ShopID]++
		genera[DominantGenus(s.Bouquet)]++
		assert.NotEqual(t, int64(101), s.Bouquet.ID)
	}
	for _, n := range shops {
		assert.LessOrEqual(t, n, MaxPerShop)
	}
	assert.LessOrEqual(t, genera["rosa"], MaxPerDominantGenus)
	assert.Len(t, top, 4, "3 розы (лимит рода) + тюльпан")
}

// FR-R02: чем меньше известно, тем сильнее выигрывает нейтральный вариант.
func TestNeutralityGrowsWithUnknowns_FRR02(t *testing.T) {
	cfg := DefaultConfig()
	orchid := bq(1, 1, 15000, "classic", "bouquet", it(fl(1, orchidTaxon, "white", domain.LevelNone), 1))
	tulips := bq(2, 2, 15000, "classic", "bouquet", it(fl(2, tulipTaxon, "white", domain.LevelLow), 15))
	occ := Occasion{Type: "birthday", BudgetMax: domain.Tenge(20000)}
	gap := func(p domain.Profile) float64 {
		s := cfg.Score(Input{Candidates: candidates(p, orchid, tulips), Profile: p, Occasion: occ, Now: now})
		var a, b float64
		for _, x := range s {
			if x.Bouquet.ID == 1 {
				a = x.Score
			} else {
				b = x.Score
			}
		}
		return a - b
	}
	assert.Greater(t, gap(domain.Profile{}), gap(fullProfile()))
}

func TestReasonsConflictsAndConfidence_FRR03(t *testing.T) {
	cfg := DefaultConfig()
	p := fullProfile()
	p[domain.AttrLikedColors] = know([]any{"white"})
	p[domain.AttrLikedFlowers] = know([]any{"rosa"})
	p[domain.AttrDislikedColors] = know([]any{"red"})
	p[domain.AttrStyle] = know("minimal")
	b := bq(1, 1, 15000, "classic", "bouquet",
		it(fl(1, rosaTaxon, "white", domain.LevelLow), 9), it(fl(2, rosaTaxon, "red", domain.LevelLow), 2))
	s := cfg.Score(Input{Candidates: candidates(p, b), Profile: p, Occasion: Occasion{Type: "gratitude", BudgetMin: domain.Tenge(16000), BudgetMax: domain.Tenge(20000)}, Now: now})
	require.Len(t, s, 1)
	assert.Contains(t, s[0].Reasons, "Любимые цвета: белый")
	assert.Contains(t, s[0].Reasons, "Любимые цветы: розы")
	assert.Contains(t, s[0].Conflicts, "Дешевле минимального бюджета")
	assert.Contains(t, s[0].Conflicts, "Стиль «классика», а получатель предпочитает «минимализм»")
	assert.Contains(t, s[0].Conflicts, "Получатель не любит этот цвет: «Rosa»")
	for _, f := range s[0].Factors {
		assert.True(t, f >= 0 && f <= 1)
	}
	assert.InDelta(t, 15.0/16.0, s[0].Factors[FactorBudgetFit], 1e-9)

	// предпочтения, указанные получателем, объясняются обобщённо (§13)
	pr := fullProfile()
	pr[domain.AttrLikedColors] = domain.Attribute{Value: []any{"white"}, Confidence: domain.Know, Source: domain.SourceRecipient}
	s2 := cfg.Score(Input{Candidates: candidates(pr, b), Profile: pr, Occasion: Occasion{Type: "gratitude", BudgetMax: domain.Tenge(20000)}, Now: now})
	assert.Contains(t, s2[0].Reasons, "Совпадает с предпочтениями получателя")
	assert.NotContains(t, s2[0].Reasons, "Любимые цвета: белый")

	// confidence: известный профиль увереннее пустого
	empty := cfg.Score(Input{Candidates: candidates(domain.Profile{}, b), Profile: domain.Profile{}, Occasion: Occasion{Type: "gratitude", BudgetMax: domain.Tenge(20000)}, Now: now})
	assert.Greater(t, s[0].Confidence, empty[0].Confidence)
	assert.InDelta(t, 0.2, BaseConfidence(domain.Profile{}), 1e-9)
	assert.LessOrEqual(t, s[0].Confidence, 1.0)
}

func TestSeasonAndStock(t *testing.T) {
	cfg := DefaultConfig()
	tulip := fl(1, tulipTaxon, "red", domain.LevelLow)
	tulip.SeasonMonths = []int{1, 2, 3, 4, 5}
	b := bq(1, 1, 10000, "bright", "bouquet", it(tulip, 9))
	occ := Occasion{Type: "birthday", BudgetMax: domain.Tenge(20000), Date: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)}
	s := cfg.Score(Input{Candidates: candidates(fullProfile(), b), Profile: fullProfile(), Occasion: occ, Now: now})
	assert.InDelta(t, 0, s[0].Factors[FactorSeasonAvail], 1e-9)
	assert.Contains(t, s[0].Conflicts, "Часть цветов не в сезоне на дату повода")

	b.InStock = false
	occ.Date = time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	s = cfg.Score(Input{Candidates: candidates(fullProfile(), b), Profile: fullProfile(), Occasion: occ, Now: now})
	assert.InDelta(t, 0, s[0].Factors[FactorSeasonAvail], 1e-9)
	assert.Contains(t, s[0].Conflicts, "Сейчас нет в наличии")
}

func TestPlattCalibration(t *testing.T) {
	var p *Platt
	assert.InDelta(t, 0.3, p.Apply(0.3), 1e-12)
	p = &Platt{A: 2, B: -1}
	assert.InDelta(t, 1/(1+math.Exp(-(2*0.3-1))), p.Apply(0.3), 1e-12)
}

func TestScoreDeterministicOrder(t *testing.T) {
	cfg := DefaultConfig()
	rose := fl(1, rosaTaxon, "pink", domain.LevelLow)
	a := bq(2, 1, 10000, "bright", "bouquet", it(rose, 9))
	b := bq(1, 2, 10000, "bright", "bouquet", it(rose, 9))
	c := bq(3, 3, 9000, "bright", "bouquet", it(rose, 9))
	for i := 0; i < 20; i++ {
		s := cfg.Score(Input{Candidates: candidates(fullProfile(), a, b, c), Profile: fullProfile(), Occasion: Occasion{Type: "birthday", BudgetMax: domain.Tenge(20000)}, Now: now})
		assert.Equal(t, []int64{3, 1, 2}, []int64{s[0].Bouquet.ID, s[1].Bouquet.ID, s[2].Bouquet.ID})
	}
}
