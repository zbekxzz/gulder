package ranking

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

// explainer считает факторы и одновременно собирает причины и конфликты (FR-R03).
// Предпочтения, указанные самим получателем, называются обобщённо: даритель не видит
// конкретных ответов получателя (§13).
type explainer struct {
	p         domain.Profile
	reasons   []string
	conflicts []string
}

func (e *explainer) reason(s string)   { e.reasons = append(e.reasons, s) }
func (e *explainer) conflict(s string) { e.conflicts = append(e.conflicts, s) }

func (e *explainer) fromRecipient(key string) bool {
	return e.p.SourceOf(key) == domain.SourceRecipient
}

func clip(x float64) float64 { return max(0, min(1, x)) }

func genusOf(f domain.Flower) string {
	if f.Taxon == nil {
		return ""
	}
	return strings.ToLower(f.Taxon.Genus)
}

// shareOf — доля стеблей, у которых key(item) ∈ set; и какие значения совпали.
func shareOf(b domain.Bouquet, set []string, key func(domain.BouquetItem) string) (float64, []string) {
	total := b.TotalStems()
	if total == 0 || len(set) == 0 {
		return 0, nil
	}
	hit := 0
	var matched []string
	for _, it := range b.Items {
		k := key(it)
		if slices.Contains(set, k) {
			hit += it.Stems
			if !slices.Contains(matched, k) {
				matched = append(matched, k)
			}
		}
	}
	return float64(hit) / float64(total), matched
}

// preferenceMatch — общая логика flower_match / color_match: доля любимых − 0.5 × доля нелюбимых,
// клип в [0,1]; любимые неизвестны → 0.5.
func (e *explainer) preferenceMatch(b domain.Bouquet, likedKey, dislikedKey string, key func(domain.BouquetItem) string,
	names map[string]string, likedTitle string,
) float64 {
	score := 0.5
	if liked, ok := e.p.StringList(likedKey); ok && len(liked) > 0 {
		share, matched := shareOf(b, liked, key)
		score = share
		if share >= 0.5 {
			if e.fromRecipient(likedKey) {
				e.reason("Совпадает с предпочтениями получателя")
			} else {
				e.reason(fmt.Sprintf("%s: %s", likedTitle, labels(names, matched)))
			}
		}
	}
	if disliked, ok := e.p.StringList(dislikedKey); ok {
		share, _ := shareOf(b, disliked, key)
		score -= 0.5 * share
	}
	return clip(score)
}

func (e *explainer) flowerMatch(b domain.Bouquet) float64 {
	return e.preferenceMatch(b, domain.AttrLikedFlowers, domain.AttrDislikedFlowers,
		func(it domain.BouquetItem) string { return genusOf(it.Flower) }, genusLabels, "Любимые цветы")
}

func (e *explainer) colorMatch(b domain.Bouquet) float64 {
	return e.preferenceMatch(b, domain.AttrLikedColors, domain.AttrDislikedColors,
		func(it domain.BouquetItem) string { return strings.ToLower(it.Flower.Color) }, colorLabels, "Любимые цвета")
}

// occasionPref — таблица соответствия «повод → стиль/цвета/формат» (§8.1, occasion_fit).
type occasionPref struct{ styles, colors, formats []string }

var occasionTable = map[string]occasionPref{
	"birthday":     {[]string{"bright", "classic"}, []string{"pink", "yellow", "orange", "red", "peach"}, []string{"bouquet", "box", "basket"}},
	"anniversary":  {[]string{"classic"}, []string{"red", "pink", "white"}, []string{"bouquet", "box"}},
	"march8":       {[]string{"classic", "bright"}, []string{"pink", "yellow", "white", "peach"}, []string{"bouquet", "box"}},
	"valentine":    {[]string{"classic"}, []string{"red", "pink"}, []string{"bouquet", "box", "single"}},
	"wedding":      {[]string{"classic", "minimal"}, []string{"white", "cream", "peach", "pink"}, []string{"bouquet", "basket"}},
	"apology":      {[]string{"classic", "minimal"}, []string{"white", "pink", "peach"}, []string{"bouquet", "single"}},
	"gratitude":    {[]string{"classic", "rustic"}, []string{"yellow", "orange", "pink", "white", "peach"}, []string{"bouquet", "basket"}},
	"congrats":     {[]string{"bright", "classic"}, []string{"yellow", "orange", "red", "pink"}, []string{"bouquet", "box", "basket"}},
	"sympathy":     {[]string{"minimal", "classic"}, []string{"white", "blue", "purple", "green"}, []string{"bouquet", "basket"}},
	"just_because": {[]string{"rustic", "minimal", "bright"}, []string{"pink", "yellow", "white", "peach", "orange"}, []string{"bouquet", "single"}},
}

var emotionColors = map[string][]string{
	"joy":       {"yellow", "orange", "pink"},
	"gratitude": {"pink", "yellow", "peach", "white"},
	"romance":   {"red", "pink"},
	"support":   {"white", "blue", "green", "yellow"},
	"respect":   {"white", "purple", "blue"},
	"apology":   {"white", "pink", "peach"},
}

func (e *explainer) occasionFit(b domain.Bouquet, o Occasion) float64 {
	pref, ok := occasionTable[o.Type]
	if !ok {
		return 0.5
	}
	colors := append(slices.Clone(pref.colors), emotionColors[o.Emotion]...)
	colorShare, _ := shareOf(b, colors, func(it domain.BouquetItem) string { return strings.ToLower(it.Flower.Color) })
	parts := []float64{boolScore(slices.Contains(pref.styles, b.Style)), colorShare, boolScore(slices.Contains(pref.formats, b.CompositionFormat))}
	switch {
	case o.Formality >= 4:
		parts = append(parts, boolScore(b.Style == "classic" || b.Style == "minimal"))
	case o.Formality >= 1 && o.Formality <= 2:
		parts = append(parts, boolScore(b.Style != "classic"))
	}
	fit := mean(parts)
	if fit >= 0.6 {
		e.reason(fmt.Sprintf("Подходит к поводу «%s»", OccasionLabel(o.Type)))
	}
	return fit
}

func (e *explainer) styleFormat(b domain.Bouquet) float64 {
	parts := []float64{}
	if style, ok := e.p.String(domain.AttrStyle); ok {
		match := style == b.Style
		parts = append(parts, boolScore(match))
		switch {
		case match && e.fromRecipient(domain.AttrStyle):
			e.reason("Совпадает с предпочтениями получателя")
		case match:
			e.reason(fmt.Sprintf("Стиль «%s» — как любит получатель", label(styleLabels, style)))
		case !e.fromRecipient(domain.AttrStyle):
			e.conflict(fmt.Sprintf("Стиль «%s», а получатель предпочитает «%s»", label(styleLabels, b.Style), label(styleLabels, style)))
		}
	} else {
		parts = append(parts, 0.5)
	}
	if format, ok := e.p.String(domain.AttrCompositionFormat); ok {
		parts = append(parts, boolScore(format == b.CompositionFormat))
	} else {
		parts = append(parts, 0.5)
	}
	return mean(parts)
}

// budgetFit: 1 внутри [min,max], линейно убывает при цене < min; > max исключается из Top-5.
func (e *explainer) budgetFit(b domain.Bouquet, o Occasion, now time.Time) float64 {
	if b.Offer.DiscountActive(now) && b.Offer.DiscountUntil != nil {
		e.reason(fmt.Sprintf("Скидка до %s", b.Offer.DiscountUntil.In(now.Location()).Format("02.01")))
	}
	switch {
	case o.BudgetMax > 0 && b.Price > o.BudgetMax:
		return 0
	case b.Price < o.BudgetMin && o.BudgetMin > 0:
		e.conflict("Дешевле минимального бюджета")
		return clip(float64(b.Price) / float64(o.BudgetMin))
	default:
		e.reason("В пределах бюджета")
		return 1
	}
}

// seasonAvail — доля стеблей в сезоне на дату повода, если букет в наличии; иначе 0.
func (e *explainer) seasonAvail(b domain.Bouquet, date time.Time) float64 {
	if !b.InStock {
		e.conflict("Сейчас нет в наличии")
		return 0
	}
	total := b.TotalStems()
	if total == 0 {
		return 0
	}
	in := 0
	for _, it := range b.Items {
		if it.Flower.InSeason(date.Month()) {
			in += it.Stems
		}
	}
	share := float64(in) / float64(total)
	if share < 1 {
		e.conflict("Часть цветов не в сезоне на дату повода")
	}
	return share
}

// neutrality — бонус за отсутствие запаха, низкую пыльцу и отсутствие токсичных для кошек/собак растений.
func (e *explainer) neutrality(b domain.Bouquet, explain bool) float64 {
	scent, pollen := domain.LevelNone, domain.LevelNone
	toxic := false
	for _, it := range b.Items {
		scent = domain.MaxLevel(scent, it.Flower.Scent)
		if t := it.Flower.Taxon; t != nil {
			pollen = domain.MaxLevel(pollen, t.Pollen)
			for _, sp := range []string{"cat", "dog"} {
				if tox, ok := t.PetToxicity[sp]; !ok || tox != domain.ToxNone {
					toxic = true
				}
			}
		} else {
			toxic = true
		}
	}
	v := mean([]float64{1 - float64(scent.Rank())/3, 1 - float64(pollen.Rank())/3, boolScore(!toxic)})
	if explain && v >= 0.75 {
		e.reason("Нейтральный вариант: слабый запах, мало пыльцы, без токсичных для животных растений")
	}
	return v
}

// conflictsFromRules переносит «нелюбимые» предупреждения Rules Engine в конфликты,
// кроме вызванных ответами получателя — они видны дарителю только через GiverView.
func (e *explainer) conflictsFromRules(r rules.Result) {
	for _, v := range r.Violations {
		if (v.Code == rules.CodeDislikedFlower || v.Code == rules.CodeDislikedColor) && v.Source != domain.SourceRecipient {
			if !slices.Contains(e.conflicts, v.Message) {
				e.conflict(v.Message)
			}
		}
	}
}

func boolScore(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}
