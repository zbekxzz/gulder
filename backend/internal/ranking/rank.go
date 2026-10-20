package ranking

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

// Occasion — повод в терминах ranker'а (FR-O01).
type Occasion struct {
	Type              string
	Emotion           string
	Place             string
	Formality         int // 0 — не указана
	Date              time.Time
	BudgetMin         domain.Money
	BudgetMax         domain.Money
	NumerologyEnabled bool
}

// Input — вход ranker'а: только допущенные Rules Engine кандидаты (FR-R01).
type Input struct {
	Candidates []rules.Candidate
	Profile    domain.Profile
	Occasion   Occasion
	// Numerology — строка таблицы для числа получателя; nil — фактор не применяется (FR-N01, FR-N06).
	Numerology *NumerologyEntry
	Now        time.Time
}

// Scored — кандидат с score, confidence и объяснениями.
type Scored struct {
	Bouquet    domain.Bouquet
	Result     rules.Result
	Score      float64
	Confidence float64
	Factors    map[string]float64
	Reasons    []string
	Conflicts  []string
	OverBudget bool
}

// safetyFields — поля безопасности, чьё отсутствие усиливает нейтральность (FR-R02).
var safetyFields = []string{domain.AttrAllergies, domain.AttrPets, domain.AttrAsthma, domain.AttrScentSensitivity}

// UnknownSafetyFields — число неизвестных полей безопасности.
func UnknownSafetyFields(p domain.Profile) int {
	n := 0
	for _, k := range safetyFields {
		if _, ok := p.Get(k); !ok {
			n++
		}
	}
	return n
}

// baseConfidenceWeights — вклад известных полей в базовую уверенность (§8.1); срез, а не map,
// чтобы порядок суммирования (и последний бит результата) был детерминирован.
var baseConfidenceWeights = []struct {
	key string
	w   float64
}{
	{domain.AttrAllergies, 0.25},
	{domain.AttrPets, 0.20},
	{domain.AttrAsthma, 0.15},
	{domain.AttrScentSensitivity, 0.15},
	{domain.AttrLikedFlowers, 0.10},
	{domain.AttrLikedColors, 0.10},
	{domain.AttrStyle, 0.05},
}

// BaseConfidence — 0.2 + 0.8 × взвешенная доля известных ключевых полей: даже при пустом профиле
// рекомендация опирается на повод и бюджет, поэтому уверенность не нулевая.
func BaseConfidence(p domain.Profile) float64 {
	known := 0.0
	for _, f := range baseConfidenceWeights {
		if _, ok := p.Get(f.key); ok {
			known += f.w
		}
	}
	return clip(0.2 + 0.8*known)
}

// Score оценивает всех кандидатов и сортирует по убыванию score
// (при равенстве — дешевле, затем меньший id: порядок детерминирован).
func (c Config) Score(in Input) []Scored {
	numerologyOn := in.Occasion.NumerologyEnabled && in.Numerology != nil
	weights := c.EffectiveWeights(UnknownSafetyFields(in.Profile), numerologyOn)
	base := BaseConfidence(in.Profile)
	date := in.Occasion.Date
	if date.IsZero() {
		date = in.Now
	}
	out := make([]Scored, 0, len(in.Candidates))
	for _, cand := range in.Candidates {
		b := cand.Bouquet
		e := explainer{p: in.Profile}
		f := map[string]float64{
			FactorFlowerMatch: e.flowerMatch(b),
			FactorColorMatch:  e.colorMatch(b),
			FactorOccasionFit: e.occasionFit(b, in.Occasion),
			FactorStyleFormat: e.styleFormat(b),
			FactorBudgetFit:   e.budgetFit(b, in.Occasion, in.Now),
			FactorSeasonAvail: e.seasonAvail(b, date),
			FactorNeutrality:  e.neutrality(b, UnknownSafetyFields(in.Profile) > 0),
		}
		if numerologyOn {
			f[FactorNumerology] = numerologyFactor(b, in.Numerology)
			if f[FactorNumerology] > 0 {
				e.reasons = append(e.reasons, fmt.Sprintf("Нумерология (%s): число %d", NumerologyLabel, in.Numerology.Number))
			}
		}
		score := 0.0
		for _, name := range append(slices.Clone(BaseFactors), FactorNumerology) { // фиксированный порядок суммы
			score += weights[name] * f[name]
		}
		e.conflictsFromRules(cand.Result)
		out = append(out, Scored{
			Bouquet: b, Result: cand.Result, Factors: f,
			Score:      score,
			Confidence: clip(c.Calibration.Apply(base * cand.Result.ConfidenceMultiplier())),
			Reasons:    nonNil(e.reasons), Conflicts: nonNil(e.conflicts),
			OverBudget: in.Occasion.BudgetMax > 0 && b.Price > in.Occasion.BudgetMax,
		})
	}
	slices.SortStableFunc(out, func(a, b Scored) int {
		if d := cmp.Compare(b.Score, a.Score); d != 0 {
			return d
		}
		if d := cmp.Compare(a.Bouquet.Price, b.Bouquet.Price); d != 0 {
			return d
		}
		return cmp.Compare(a.Bouquet.ID, b.Bouquet.ID)
	})
	return out
}

// Diversity-ограничения Top-5 (§8.2).
const (
	MaxPerShop          = 2
	MaxPerDominantGenus = 3
)

// DominantGenus — род с наибольшим числом стеблей.
func DominantGenus(b domain.Bouquet) string {
	stems := map[string]int{}
	best, bestN := "", -1
	for _, it := range b.Items {
		g := "?"
		if it.Flower.Taxon != nil {
			g = strings.ToLower(it.Flower.Taxon.Genus)
		}
		stems[g] += it.Stems
		if stems[g] > bestN || (stems[g] == bestN && g < best) {
			best, bestN = g, stems[g]
		}
	}
	return best
}

// SelectTop выбирает до n позиций: без превышения бюджета, не больше 2 из одного магазина
// и не больше 3 с одинаковым доминирующим родом.
func SelectTop(scored []Scored, n int) []Scored {
	perShop := map[int64]int{}
	perGenus := map[string]int{}
	out := []Scored{}
	for _, s := range scored {
		if len(out) == n {
			break
		}
		if s.OverBudget {
			continue
		}
		g := DominantGenus(s.Bouquet)
		if perShop[s.Bouquet.ShopID] >= MaxPerShop || perGenus[g] >= MaxPerDominantGenus {
			continue
		}
		perShop[s.Bouquet.ShopID]++
		perGenus[g]++
		out = append(out, s)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
