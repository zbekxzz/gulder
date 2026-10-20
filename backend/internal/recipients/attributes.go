// Package recipients — получатели, профиль (EAV с value/source/confidence), поводы и feedback
// (FR-P01–P03, FR-O01, FR-F01, FR-N07).
package recipients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/domain"
)

// AttributeInput — поле профиля из запроса PUT /recipients/{id} (FR-P03).
type AttributeInput struct {
	Key        string            `json:"key"`
	Value      json.RawMessage   `json:"value"`
	Source     domain.Source     `json:"source"`
	Confidence domain.Confidence `json:"confidence"`
}

// Attribute — нормализованное поле, готовое к записи.
type Attribute struct {
	Key        string
	Value      any // nil только при confidence=unknown
	Source     domain.Source
	Confidence domain.Confidence
}

var (
	allergenRe = regexp.MustCompile(`^(genus:[a-z]+|family:[a-z]+|pollen|food:[a-z]+(:[a-z]+)?|fragrance|latex)$`)
	wordRe     = regexp.MustCompile(`^[a-z][a-z_]{1,39}$`)
)

// Допустимые значения строковых полей.
var (
	Styles             = []string{"classic", "minimal", "rustic", "bright"}
	CompositionFormats = []string{"bouquet", "box", "basket", "single"}
)

// ValidateAttribute проверяет и нормализует поле. Ошибка — человекочитаемое сообщение для details.
//
// AC-2: «не знаю» хранится как confidence=unknown с value=null и никогда не превращается
// в «аллергий нет» — для этого нужен явный [] с know/assume.
func ValidateAttribute(in AttributeInput, now time.Time) (Attribute, error) {
	a := Attribute{Key: in.Key, Source: in.Source, Confidence: in.Confidence}
	if !domain.IsAttributeKey(in.Key) {
		return a, fmt.Errorf("неизвестный ключ профиля")
	}
	if a.Source == "" {
		a.Source = domain.SourceGiver
	}
	if !a.Source.Valid() {
		return a, fmt.Errorf("source: giver/recipient/feedback/system")
	}
	if !a.Confidence.Valid() {
		return a, fmt.Errorf("confidence: know/assume/unknown")
	}
	isNull := len(bytes.TrimSpace(in.Value)) == 0 || string(bytes.TrimSpace(in.Value)) == "null"
	if a.Confidence == domain.Unknown {
		if !isNull {
			return a, fmt.Errorf("при «не знаю» значение не указывается")
		}
		return a, nil
	}
	if isNull {
		return a, fmt.Errorf("укажите значение или выберите «не знаю»")
	}
	v, err := normalizeValue(in.Key, in.Value, now)
	if err != nil {
		return a, err
	}
	a.Value = v
	return a, nil
}

func normalizeValue(key string, raw json.RawMessage, now time.Time) (any, error) {
	switch key {
	case domain.AttrAllergies:
		return stringList(raw, allergenRe, "коды аллергенов: genus:<род>, family:<семейство>, pollen, food:<группа>[:<подгруппа>], fragrance, latex")
	case domain.AttrPets, domain.AttrLikedFlowers, domain.AttrDislikedFlowers, domain.AttrLikedColors, domain.AttrDislikedColors:
		return stringList(raw, wordRe, "ожидается список слов латиницей в нижнем регистре")
	case domain.AttrAsthma:
		var b bool
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("ожидается true/false")
		}
		return b, nil
	case domain.AttrScentSensitivity:
		s, err := oneOf(raw, []string{"none", "low", "medium", "strong"})
		return s, err
	case domain.AttrStyle:
		return oneOf(raw, Styles)
	case domain.AttrCompositionFormat:
		return oneOf(raw, CompositionFormats)
	case domain.AttrBirthDate:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, fmt.Errorf("ожидается дата YYYY-MM-DD")
		}
		d, err := time.Parse("2006-01-02", s)
		if err != nil || d.After(now) || d.Year() < 1900 {
			return nil, fmt.Errorf("ожидается дата YYYY-MM-DD не позже сегодняшней")
		}
		return s, nil
	}
	return nil, fmt.Errorf("неизвестный ключ профиля")
}

func stringList(raw json.RawMessage, re *regexp.Regexp, msg string) (any, error) {
	var items []string
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("ожидается массив строк")
	}
	out := make([]any, 0, len(items))
	seen := map[string]bool{}
	for _, it := range items {
		it = strings.ToLower(strings.TrimSpace(it))
		if !re.MatchString(it) {
			return nil, fmt.Errorf("%s: %q", msg, it)
		}
		if !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	return out, nil
}

func oneOf(raw json.RawMessage, allowed []string) (any, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("ожидается строка")
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if !slices.Contains(allowed, s) {
		return nil, fmt.Errorf("допустимо: %s", strings.Join(allowed, ", "))
	}
	return s, nil
}
