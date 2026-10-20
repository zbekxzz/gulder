package rules

import "github.com/zbekxzz/gulder/backend/internal/domain"

// Question — уточняющий вопрос с наибольшей ожидаемой пользой (FR-R04).
type Question struct {
	Attribute string  `json:"attribute"`
	Text      string  `json:"text"`
	Gain      float64 `json:"gain"` // сумма штрафов WARN, которые снимет ответ
}

// questionOrder задаёт и тексты, и порядок при равной пользе.
var questionOrder = []struct{ attr, text string }{
	{domain.AttrAllergies, "Есть ли у получателя аллергия на цветы, пыльцу или продукты?"},
	{domain.AttrPets, "Есть ли у получателя дома кошка, собака или другие животные?"},
	{domain.AttrAsthma, "Есть ли у получателя астма или чувствительность дыхательных путей?"},
	{domain.AttrScentSensitivity, "Как получатель относится к сильным запахам?"},
}

// QuestionText возвращает текст вопроса про атрибут.
func QuestionText(attr string) string {
	for _, q := range questionOrder {
		if q.attr == attr {
			return q.text
		}
	}
	return ""
}

// BestClarifyingQuestion для каждого атрибута из missing_info суммирует штрафы WARN по всем
// кандидатам и спрашивает про атрибут с максимальной суммой. Нет неизвестных → nil.
func BestClarifyingQuestion(results []Result) *Question {
	gain := map[string]float64{}
	for _, r := range results {
		for _, attr := range r.MissingInfo {
			for _, v := range r.Violations {
				if v.Decision == domain.Warn && v.Attribute == attr {
					gain[attr] += v.ConfidencePenalty
				}
			}
		}
	}
	var best *Question
	for _, q := range questionOrder {
		g, ok := gain[q.attr]
		if !ok {
			continue
		}
		if best == nil || g > best.Gain+1e-9 {
			best = &Question{Attribute: q.attr, Text: q.text, Gain: g}
		}
	}
	return best
}
