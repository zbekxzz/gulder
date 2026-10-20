// Package domain содержит типы предметной области, общие для Rules Engine, ranker,
// ассистента и слоя хранения. Пакет не зависит от БД, HTTP и LLM.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// Confidence — уверенность дарителя в значении поля профиля (FR-P03).
// "unknown" никогда не означает «ограничения нет» (§7.1).
type Confidence string

// Значения Confidence.
const (
	Know    Confidence = "know"
	Assume  Confidence = "assume"
	Unknown Confidence = "unknown"
)

// Valid сообщает, допустимо ли значение.
func (c Confidence) Valid() bool { return c == Know || c == Assume || c == Unknown }

// Source — кто сообщил значение поля профиля (FR-P03).
type Source string

// Значения Source.
const (
	SourceGiver     Source = "giver"
	SourceRecipient Source = "recipient"
	SourceFeedback  Source = "feedback"
	SourceSystem    Source = "system"
)

// Valid сообщает, допустимо ли значение.
func (s Source) Valid() bool {
	return s == SourceGiver || s == SourceRecipient || s == SourceFeedback || s == SourceSystem
}

// Level — шкала интенсивности запаха, пыльцы и чувствительности: none < low < medium < strong.
type Level string

// Значения Level по возрастанию.
const (
	LevelNone   Level = "none"
	LevelLow    Level = "low"
	LevelMedium Level = "medium"
	LevelStrong Level = "strong"
)

// Rank возвращает порядковый номер уровня 0..3. Неизвестное значение трактуется как strong:
// при ошибке в данных правила должны срабатывать, а не молчать.
func (l Level) Rank() int {
	switch l {
	case LevelNone:
		return 0
	case LevelLow:
		return 1
	case LevelMedium:
		return 2
	default:
		return 3
	}
}

// AtLeast сообщает, что уровень не ниже other.
func (l Level) AtLeast(other Level) bool { return l.Rank() >= other.Rank() }

// MaxLevel возвращает больший из уровней.
func MaxLevel(a, b Level) Level {
	if a.Rank() >= b.Rank() {
		return a
	}
	return b
}

// ParseLevel разбирает уровень; регистр не важен.
func ParseLevel(s string) (Level, error) {
	l := Level(strings.ToLower(strings.TrimSpace(s)))
	switch l {
	case LevelNone, LevelLow, LevelMedium, LevelStrong:
		return l, nil
	}
	return "", fmt.Errorf("unknown level %q", s)
}

// Toxicity — токсичность растения для вида животного (ASPCA).
type Toxicity string

// Значения Toxicity.
const (
	ToxNone     Toxicity = "none"
	ToxMild     Toxicity = "mild"
	ToxModerate Toxicity = "moderate"
	ToxSevere   Toxicity = "severe"
	ToxUnknown  Toxicity = "unknown"
)

// ParseToxicity разбирает уровень токсичности; пустая строка = unknown.
func ParseToxicity(s string) (Toxicity, error) {
	t := Toxicity(strings.ToLower(strings.TrimSpace(s)))
	switch t {
	case "":
		return ToxUnknown, nil
	case ToxNone, ToxMild, ToxModerate, ToxSevere, ToxUnknown:
		return t, nil
	}
	return "", fmt.Errorf("unknown toxicity %q", s)
}

// Decision — итог проверки Rules Engine.
type Decision string

// Значения Decision.
const (
	Allow Decision = "allow"
	Warn  Decision = "warn"
	Block Decision = "block"
)

// PetSpecies — виды, для которых в справочнике есть данные о токсичности (§7.3, F4).
var PetSpecies = []string{"cat", "dog", "horse"}

// Ключи профиля получателя (§6.1).
const (
	AttrAllergies         = "allergies"
	AttrAsthma            = "asthma"
	AttrPets              = "pets"
	AttrScentSensitivity  = "scent_sensitivity"
	AttrLikedFlowers      = "liked_flowers"
	AttrDislikedFlowers   = "disliked_flowers"
	AttrLikedColors       = "liked_colors"
	AttrDislikedColors    = "disliked_colors"
	AttrStyle             = "style"
	AttrCompositionFormat = "composition_format"
	AttrBirthDate         = "birth_date"
)

// AttributeKeys — все допустимые ключи профиля в каноническом порядке.
var AttributeKeys = []string{
	AttrAllergies, AttrAsthma, AttrPets, AttrScentSensitivity,
	AttrLikedFlowers, AttrDislikedFlowers, AttrLikedColors, AttrDislikedColors,
	AttrStyle, AttrCompositionFormat, AttrBirthDate,
}

// IsAttributeKey сообщает, известен ли ключ профиля.
func IsAttributeKey(k string) bool {
	for _, key := range AttributeKeys {
		if key == k {
			return true
		}
	}
	return false
}

// Attribute — одно поле профиля: значение, источник и уверенность (FR-P03).
type Attribute struct {
	Value      any
	Source     Source
	Confidence Confidence
	UpdatedAt  time.Time
}

// Known сообщает, что поле известно: confidence != unknown и value != null.
// «Предполагаю» для ограничений трактуется как «знаю» (§7.1, принцип 2).
func (a Attribute) Known() bool {
	return a.Confidence != Unknown && a.Confidence != "" && a.Value != nil
}

// Profile — профиль получателя. Отсутствие ключа = unknown.
type Profile map[string]Attribute

// Get возвращает поле, только если оно известно.
func (p Profile) Get(key string) (Attribute, bool) {
	a, ok := p[key]
	if !ok || !a.Known() {
		return Attribute{}, false
	}
	return a, true
}

// SourceOf возвращает источник поля или "" для отсутствующего.
func (p Profile) SourceOf(key string) Source { return p[key].Source }

// StringList возвращает известный список строк в нижнем регистре. Значение неверного
// типа считается неизвестным: правила трактуют его консервативно.
func (p Profile) StringList(key string) ([]string, bool) {
	a, ok := p.Get(key)
	if !ok {
		return nil, false
	}
	switch v := a.Value.(type) {
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			out = append(out, strings.ToLower(strings.TrimSpace(s)))
		}
		return out, true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, isStr := item.(string)
			if !isStr {
				return nil, false
			}
			out = append(out, strings.ToLower(strings.TrimSpace(s)))
		}
		return out, true
	}
	return nil, false
}

// Bool возвращает известное булево значение.
func (p Profile) Bool(key string) (value, known bool) {
	a, ok := p.Get(key)
	if !ok {
		return false, false
	}
	b, isBool := a.Value.(bool)
	return b, isBool
}

// String возвращает известное строковое значение.
func (p Profile) String(key string) (string, bool) {
	a, ok := p.Get(key)
	if !ok {
		return "", false
	}
	s, isStr := a.Value.(string)
	return strings.ToLower(strings.TrimSpace(s)), isStr
}

// Level возвращает известный уровень (scent_sensitivity).
func (p Profile) Level(key string) (Level, bool) {
	s, ok := p.String(key)
	if !ok {
		return "", false
	}
	l, err := ParseLevel(s)
	if err != nil {
		return "", false
	}
	return l, true
}

// PlantTaxon — запись справочника растений (Приложение B).
type PlantTaxon struct {
	ID             int64
	ScientificName string
	Genus          string
	Family         string
	PetToxicity    map[string]Toxicity // "cat","dog","horse"
	Pollen         Level
	AllergenCodes  []string
	ASPCAURL       string
	ImageURL       string
	ImageLicense   string
	ImageAuthor    string
}

// Flower — торговая позиция «цветок» (FR-C01).
type Flower struct {
	ID             int64
	Code           string
	TradeName      string
	Color          string
	Scent          Level
	Taxon          *PlantTaxon
	TaxonConfirmed bool
	SeasonMonths   []int
}

// InSeason сообщает, доступен ли цветок в указанном месяце.
func (f Flower) InSeason(month time.Month) bool {
	if len(f.SeasonMonths) == 0 {
		return true
	}
	for _, m := range f.SeasonMonths {
		if m == int(month) {
			return true
		}
	}
	return false
}

// BouquetItem — позиция состава букета.
type BouquetItem struct {
	Flower Flower
	Stems  int
}

// PriceOffer — цена и скидка позиции (FR-C01, FR-L04).
type PriceOffer struct {
	Price         Money
	DiscountPrice *Money
	DiscountFrom  *time.Time
	DiscountUntil *time.Time
}

// DiscountActive сообщает, действует ли скидка в момент now: now ∈ [from, until).
func (o PriceOffer) DiscountActive(now time.Time) bool {
	if o.DiscountPrice == nil || o.DiscountUntil == nil {
		return false
	}
	if o.DiscountFrom != nil && now.Before(*o.DiscountFrom) {
		return false
	}
	return now.Before(*o.DiscountUntil)
}

// Effective возвращает эффективную цену: со скидкой, если она действует (§10).
func (o PriceOffer) Effective(now time.Time) Money {
	if o.DiscountActive(now) {
		return *o.DiscountPrice
	}
	return o.Price
}

// Bouquet — букет (FR-C02). Price — эффективная цена на момент проверки;
// её использует Rules Engine для проверки бюджета.
type Bouquet struct {
	ID                int64
	ShopID            int64
	SKU               string
	Name              string
	Items             []BouquetItem
	Price             Money
	Offer             PriceOffer
	Packaging         string
	Style             string
	CompositionFormat string
	ImageURL          string
	InStock           bool
}

// TotalStems возвращает число стеблей в букете.
func (b Bouquet) TotalStems() int {
	n := 0
	for _, it := range b.Items {
		n += it.Stems
	}
	return n
}

// Gift categories (FR-C04).
const (
	GiftCard      = "card"
	GiftSweets    = "sweets"
	GiftSoftToy   = "soft_toy"
	GiftFragrance = "fragrance"
	GiftJewelry   = "jewelry"
	GiftCandle    = "candle"
	GiftOther     = "other"
)

// GiftCategories — допустимые категории подарков.
var GiftCategories = []string{GiftCard, GiftSweets, GiftSoftToy, GiftFragrance, GiftJewelry, GiftCandle, GiftOther}

// IsGiftCategory сообщает, известна ли категория.
func IsGiftCategory(c string) bool {
	for _, k := range GiftCategories {
		if k == c {
			return true
		}
	}
	return false
}

// GiftItem — дополнительный подарок (FR-C04).
type GiftItem struct {
	ID       int64
	ShopID   int64
	SKU      string
	Name     string
	Category string
	Price    Money
	Offer    PriceOffer
	// nil = состав неизвестен; &[]string{} = проверено, аллергенов нет.
	AllergenCodes *[]string
	Scent         Level
	// nil = неизвестно.
	PetHazard   *bool
	Composition string
	InStock     bool
}
