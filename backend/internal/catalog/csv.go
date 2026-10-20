// Package catalog — разбор CSV каталога (сид и импорт флориста), загрузка каталога из БД
// в доменные типы, доступность и цены (FR-C01–C05, §12).
package catalog

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/domain"
)

// RowError — ошибка в строке CSV (ответ импорта: {line, field, message}).
type RowError struct {
	Line    int    `json:"line"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// table — CSV с заголовком; строки адресуются по имени колонки.
type table struct {
	header map[string]int
	rows   [][]string
	lines  []int
}

func readTable(r io.Reader, required []string) (*table, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	t := &table{header: map[string]int{}}
	for i, h := range head {
		t.header[strings.TrimPrefix(strings.ToLower(strings.TrimSpace(h)), "\ufeff")] = i
	}
	var missing []string
	for _, col := range required {
		if _, ok := t.header[col]; !ok {
			missing = append(missing, col)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing columns: %s", strings.Join(missing, ", "))
	}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read csv: %w", err)
		}
		line, _ := cr.FieldPos(0)
		if isBlank(rec) {
			continue
		}
		t.rows = append(t.rows, rec)
		t.lines = append(t.lines, line)
	}
	return t, nil
}

func isBlank(rec []string) bool {
	for _, v := range rec {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// row — доступ к полям одной строки с накоплением ошибок.
type row struct {
	t    *table
	vals []string
	line int
	errs *[]RowError
}

func (t *table) each(errs *[]RowError, fn func(r *row)) {
	for i, vals := range t.rows {
		fn(&row{t: t, vals: vals, line: t.lines[i], errs: errs})
	}
}

func (r *row) fail(field, msg string) {
	*r.errs = append(*r.errs, RowError{Line: r.line, Field: field, Message: msg})
}

func (r *row) str(col string) string {
	i, ok := r.t.header[col]
	if !ok || i >= len(r.vals) {
		return ""
	}
	return strings.TrimSpace(r.vals[i])
}

func (r *row) required(col string) string {
	v := r.str(col)
	if v == "" {
		r.fail(col, "обязательное поле")
	}
	return v
}

func (r *row) money(col string, required bool) *domain.Money {
	v := r.str(col)
	if v == "" {
		if required {
			r.fail(col, "обязательное поле")
		}
		return nil
	}
	m, err := domain.ParseMoney(v)
	if err != nil || m <= 0 {
		r.fail(col, "ожидается положительная сумма в тенге, например 12000 или 12000.50")
		return nil
	}
	return &m
}

func (r *row) time(col string) *time.Time {
	v := r.str(col)
	if v == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if ts, err := time.Parse(layout, v); err == nil {
			return &ts
		}
	}
	r.fail(col, "ожидается дата RFC3339 или YYYY-MM-DD")
	return nil
}

func (r *row) boolean(col string, def *bool) *bool {
	v := strings.ToLower(r.str(col))
	switch v {
	case "":
		return def
	case "true", "1", "yes", "да":
		b := true
		return &b
	case "false", "0", "no", "нет":
		b := false
		return &b
	}
	r.fail(col, "ожидается true/false")
	return def
}

func (r *row) level(col string, def domain.Level) domain.Level {
	v := r.str(col)
	if v == "" {
		return def
	}
	l, err := domain.ParseLevel(v)
	if err != nil {
		r.fail(col, "ожидается none/low/medium/strong")
		return def
	}
	return l
}

func (r *row) toxicity(col string) domain.Toxicity {
	t, err := domain.ParseToxicity(r.str(col))
	if err != nil {
		r.fail(col, "ожидается none/mild/moderate/severe/unknown")
		return domain.ToxUnknown
	}
	return t
}

func (r *row) intVal(col string, min int) int {
	v, err := strconv.Atoi(r.str(col))
	if err != nil || v < min {
		r.fail(col, fmt.Sprintf("ожидается целое ≥ %d", min))
		return 0
	}
	return v
}

func (r *row) float(col string, min, max float64) float64 {
	v, err := strconv.ParseFloat(r.str(col), 64)
	if err != nil || v < min || v > max {
		r.fail(col, fmt.Sprintf("ожидается число в диапазоне [%g, %g]", min, max))
		return 0
	}
	return v
}

func splitCodes(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ";") {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseAllergenCodes разбирает коды аллергенов: пусто = состав неизвестен (nil), "-" = проверено, аллергенов нет (§12).
func ParseAllergenCodes(s string) *[]string {
	s = strings.TrimSpace(s)
	switch s {
	case "":
		return nil
	case "-":
		empty := []string{}
		return &empty
	}
	codes := splitCodes(s)
	return &codes
}

// TaxonRow — строка plant_taxa.csv.
type TaxonRow struct {
	Line     int
	Taxon    domain.PlantTaxon
	GBIFKey  *int64
	Verified bool
}

// ParseTaxa разбирает справочник растений (Приложение B + поля ASPCA/GBIF).
func ParseTaxa(r io.Reader) ([]TaxonRow, []RowError, error) {
	t, err := readTable(r, []string{"scientific_name", "genus", "family", "tox_cat", "tox_dog", "tox_horse", "pollen"})
	if err != nil {
		return nil, nil, err
	}
	var out []TaxonRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		tr := TaxonRow{Line: r.line, Taxon: domain.PlantTaxon{
			ScientificName: r.required("scientific_name"),
			Genus:          r.required("genus"),
			Family:         r.required("family"),
			PetToxicity: map[string]domain.Toxicity{
				"cat": r.toxicity("tox_cat"), "dog": r.toxicity("tox_dog"), "horse": r.toxicity("tox_horse"),
			},
			Pollen:        r.level("pollen", domain.LevelMedium),
			AllergenCodes: splitCodes(r.str("allergen_codes")),
			ASPCAURL:      r.str("aspca_url"),
			ImageURL:      r.str("image_url"),
			ImageLicense:  r.str("image_license"),
			ImageAuthor:   r.str("image_author"),
		}}
		if tr.Taxon.ImageURL != "" && tr.Taxon.ImageLicense == "" {
			r.fail("image_license", "у изображения должна быть лицензия")
		}
		if v := r.str("gbif_key"); v != "" {
			k, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				r.fail("gbif_key", "ожидается целое")
			} else {
				tr.GBIFKey = &k
			}
		}
		f := false
		tr.Verified = *r.boolean("verified", &f)
		out = append(out, tr)
	})
	return out, errs, nil
}

// ShopRow — строка shops.csv.
type ShopRow struct {
	Line                    int
	Code, Name, Address, TZ string
	Lat, Lon                float64
	OpeningHours            json.RawMessage
}

// ParseShops разбирает магазины (FR-C05).
func ParseShops(r io.Reader) ([]ShopRow, []RowError, error) {
	t, err := readTable(r, []string{"code", "name", "address", "lat", "lon", "opening_hours"})
	if err != nil {
		return nil, nil, err
	}
	var out []ShopRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		s := ShopRow{
			Line: r.line, Code: r.required("code"), Name: r.required("name"), Address: r.required("address"),
			Lat: r.float("lat", -90, 90), Lon: r.float("lon", -180, 180), TZ: r.str("timezone"),
		}
		if s.TZ == "" {
			s.TZ = "Asia/Almaty"
		}
		if _, err := time.LoadLocation(s.TZ); err != nil {
			r.fail("timezone", "неизвестный часовой пояс")
		}
		raw := r.str("opening_hours")
		if _, err := ParseOpeningHours([]byte(raw)); err != nil {
			r.fail("opening_hours", err.Error())
		}
		s.OpeningHours = json.RawMessage(raw)
		out = append(out, s)
	})
	return out, errs, nil
}

// FlowerRow — строка flowers.csv / импорта цветов.
type FlowerRow struct {
	Line           int
	Code           string
	TradeName      string
	ScientificName string
	Genus          string
	Color          string
	Scent          domain.Level
	SeasonMonths   []int
	CareNotes      string
	Restrictions   string
	TaxonConfirmed bool
}

// ParseFlowers разбирает цветы (FR-C01).
func ParseFlowers(r io.Reader) ([]FlowerRow, []RowError, error) {
	t, err := readTable(r, []string{"code", "trade_name", "genus", "color"})
	if err != nil {
		return nil, nil, err
	}
	var out []FlowerRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		f := FlowerRow{
			Line: r.line, Code: r.required("code"), TradeName: r.required("trade_name"),
			ScientificName: r.str("scientific_name"), Genus: r.required("genus"),
			Color: strings.ToLower(r.required("color")), Scent: r.level("scent", domain.LevelLow),
			CareNotes: r.str("care_notes"), Restrictions: r.str("restrictions"),
		}
		tr := true
		f.TaxonConfirmed = *r.boolean("taxon_confirmed", &tr)
		f.SeasonMonths = parseMonths(r, "season_months")
		out = append(out, f)
	})
	return out, errs, nil
}

func parseMonths(r *row, col string) []int {
	v := r.str(col)
	if v == "" {
		return []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	}
	var out []int
	for _, p := range strings.Split(v, ";") {
		m, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || m < 1 || m > 12 {
			r.fail(col, "ожидаются месяцы 1–12 через «;»")
			return nil
		}
		out = append(out, m)
	}
	return out
}

// OfferFields — цена и скидка в строке CSV.
type OfferFields struct {
	Price         domain.Money
	DiscountPrice *domain.Money
	DiscountFrom  *time.Time
	DiscountUntil *time.Time
}

func (r *row) offer() OfferFields {
	var o OfferFields
	if p := r.money("price", true); p != nil {
		o.Price = *p
	}
	o.DiscountPrice = r.money("discount_price", false)
	o.DiscountFrom = r.time("discount_from")
	o.DiscountUntil = r.time("discount_until")
	if o.DiscountPrice != nil {
		if *o.DiscountPrice >= o.Price {
			r.fail("discount_price", "цена со скидкой должна быть меньше цены")
		}
		if o.DiscountUntil == nil {
			r.fail("discount_until", "у скидки должен быть срок")
		}
	}
	if o.DiscountFrom != nil && o.DiscountUntil != nil && !o.DiscountFrom.Before(*o.DiscountUntil) {
		r.fail("discount_until", "срок скидки должен быть позже начала")
	}
	return o
}

// BouquetRow — строка bouquets.csv / импорта букетов.
type BouquetRow struct {
	Line              int
	ShopCode          string
	SKU, Name         string
	Packaging         string
	Style             string
	CompositionFormat string
	ImageURL          string
	InStock           bool
	Offer             OfferFields
	// Composition заполняется из колонки composition (импорт флориста) или bouquet_items.csv (сид).
	Composition []CompositionItem
}

// CompositionItem — позиция состава.
type CompositionItem struct {
	FlowerCode string
	Stems      int
}

// ParseBouquets разбирает букеты (FR-C02). Колонка composition — формат «code:stems;code:stems».
func ParseBouquets(r io.Reader) ([]BouquetRow, []RowError, error) {
	t, err := readTable(r, []string{"sku", "name", "price"})
	if err != nil {
		return nil, nil, err
	}
	var out []BouquetRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		tr := true
		b := BouquetRow{
			Line: r.line, ShopCode: r.str("shop_code"), SKU: r.required("sku"), Name: r.required("name"),
			Packaging: r.str("packaging"), Style: strings.ToLower(r.str("style")),
			CompositionFormat: strings.ToLower(r.str("composition_format")), ImageURL: r.str("image_url"),
			InStock: *r.boolean("in_stock", &tr), Offer: r.offer(),
		}
		if b.CompositionFormat == "" {
			b.CompositionFormat = "bouquet"
		}
		if c := r.str("composition"); c != "" {
			for _, part := range strings.Split(c, ";") {
				code, stems, ok := strings.Cut(strings.TrimSpace(part), ":")
				n, err := strconv.Atoi(strings.TrimSpace(stems))
				if !ok || err != nil || n <= 0 || strings.TrimSpace(code) == "" {
					r.fail("composition", "ожидается «код_цветка:стебли;…»")
					break
				}
				b.Composition = append(b.Composition, CompositionItem{FlowerCode: strings.TrimSpace(code), Stems: n})
			}
		}
		out = append(out, b)
	})
	return out, errs, nil
}

// BouquetItemRow — строка bouquet_items.csv.
type BouquetItemRow struct {
	Line          int
	ShopCode, SKU string
	Item          CompositionItem
}

// ParseBouquetItems разбирает составы букетов для сида.
func ParseBouquetItems(r io.Reader) ([]BouquetItemRow, []RowError, error) {
	t, err := readTable(r, []string{"shop_code", "sku", "flower_code", "stems"})
	if err != nil {
		return nil, nil, err
	}
	var out []BouquetItemRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		out = append(out, BouquetItemRow{
			Line: r.line, ShopCode: r.required("shop_code"), SKU: r.required("sku"),
			Item: CompositionItem{FlowerCode: r.required("flower_code"), Stems: r.intVal("stems", 1)},
		})
	})
	return out, errs, nil
}

// GiftRow — строка gifts.csv / импорта подарков.
type GiftRow struct {
	Line          int
	ShopCode      string
	SKU, Name     string
	Category      string
	Composition   string
	AllergenCodes *[]string
	Scent         domain.Level
	PetHazard     *bool
	InStock       bool
	Offer         OfferFields
}

// ParseGifts разбирает подарки (FR-C04).
func ParseGifts(r io.Reader) ([]GiftRow, []RowError, error) {
	t, err := readTable(r, []string{"sku", "name", "category", "price"})
	if err != nil {
		return nil, nil, err
	}
	var out []GiftRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		tr := true
		g := GiftRow{
			Line: r.line, ShopCode: r.str("shop_code"), SKU: r.required("sku"), Name: r.required("name"),
			Category: strings.ToLower(r.required("category")), Composition: r.str("composition"),
			AllergenCodes: ParseAllergenCodes(r.str("allergen_codes")),
			Scent:         r.level("scent", domain.LevelNone),
			PetHazard:     r.boolean("pet_hazard", nil),
			InStock:       *r.boolean("in_stock", &tr),
			Offer:         r.offer(),
		}
		if g.Category != "" && !domain.IsGiftCategory(g.Category) {
			r.fail("category", "допустимо: "+strings.Join(domain.GiftCategories, ", "))
		}
		out = append(out, g)
	})
	return out, errs, nil
}

// NumerologyRow — строка numerology.csv (FR-N03).
type NumerologyRow struct {
	Line    int
	Number  int
	Flowers []string
	Colors  []string
	Meaning string
}

// ParseNumerology разбирает таблицу нумерологии.
func ParseNumerology(r io.Reader) ([]NumerologyRow, []RowError, error) {
	t, err := readTable(r, []string{"number", "flowers", "colors", "meaning"})
	if err != nil {
		return nil, nil, err
	}
	var out []NumerologyRow
	var errs []RowError
	t.each(&errs, func(r *row) {
		n := r.intVal("number", 1)
		if (n < 1 || n > 9) && n != 11 && n != 22 && n != 33 {
			r.fail("number", "допустимо 1–9, 11, 22, 33")
		}
		out = append(out, NumerologyRow{
			Line: r.line, Number: n, Flowers: splitCodes(r.str("flowers")),
			Colors: splitCodes(r.str("colors")), Meaning: r.required("meaning"),
		})
	})
	return out, errs, nil
}
