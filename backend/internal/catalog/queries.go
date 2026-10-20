package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Shop — магазин (FR-C05).
type Shop struct {
	ID           int64        `json:"id"`
	Code         string       `json:"code,omitempty"`
	Name         string       `json:"name"`
	Address      string       `json:"address"`
	Lat          float64      `json:"lat"`
	Lon          float64      `json:"lon"`
	Timezone     string       `json:"timezone"`
	OpeningHours OpeningHours `json:"opening_hours"`
}

// Location возвращает часовой пояс магазина (Asia/Almaty при ошибке).
func (s Shop) Location() *time.Location {
	if loc, err := time.LoadLocation(s.Timezone); err == nil {
		return loc
	}
	loc, _ := time.LoadLocation("Asia/Almaty")
	return loc
}

// ListShops возвращает все магазины.
func ListShops(ctx context.Context, q store.Querier) ([]Shop, error) {
	rows, err := q.Query(ctx, `SELECT id, coalesce(code,''), name, address, lat, lon, timezone, opening_hours FROM shops ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list shops: %w", err)
	}
	defer rows.Close()
	var out []Shop
	for rows.Next() {
		var s Shop
		var raw []byte
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.Address, &s.Lat, &s.Lon, &s.Timezone, &raw); err != nil {
			return nil, fmt.Errorf("scan shop: %w", err)
		}
		if err := json.Unmarshal(raw, &s.OpeningHours); err != nil {
			return nil, fmt.Errorf("decode hours of shop %d: %w", s.ID, err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Availability — наличие позиции.
type Availability struct {
	ID       int64 `json:"id"`
	InStock  bool  `json:"in_stock"`
	Quantity *int  `json:"quantity"`
}

// Offer — цена и скидка в ответах API.
type Offer struct {
	Price         domain.Money  `json:"price"`
	DiscountPrice *domain.Money `json:"discount_price"`
	DiscountFrom  *time.Time    `json:"discount_from"`
	DiscountUntil *time.Time    `json:"discount_until"`
}

// Domain переводит в доменный тип.
func (o Offer) Domain() domain.PriceOffer {
	return domain.PriceOffer{Price: o.Price, DiscountPrice: o.DiscountPrice, DiscountFrom: o.DiscountFrom, DiscountUntil: o.DiscountUntil}
}

// CompositionView — позиция состава в ответе.
type CompositionView struct {
	FlowerID       int64  `json:"flower_id"`
	Code           string `json:"code"`
	TradeName      string `json:"trade_name"`
	Color          string `json:"color"`
	Genus          string `json:"genus"`
	Stems          int    `json:"stems"`
	TaxonConfirmed bool   `json:"taxon_confirmed"`
}

// BouquetView — букет в кабинете флориста.
type BouquetView struct {
	ID                int64             `json:"id"`
	SKU               string            `json:"sku"`
	Name              string            `json:"name"`
	Packaging         string            `json:"packaging"`
	Style             string            `json:"style"`
	CompositionFormat string            `json:"composition_format"`
	ImageURL          string            `json:"image_url"`
	Active            bool              `json:"active"`
	Availability      Availability      `json:"availability"`
	Offer             Offer             `json:"offer"`
	Items             []CompositionView `json:"items"`
}

const offerJoin = `
	LEFT JOIN LATERAL (SELECT price, discount_price, discount_from, discount_until FROM price_offers p
	                    WHERE p.%s = %s ORDER BY p.created_at DESC, p.id DESC LIMIT 1) po ON true`

// ListShopBouquets возвращает букеты магазина с составом, наличием и ценой.
func ListShopBouquets(ctx context.Context, q store.Querier, shopID int64) ([]BouquetView, error) {
	rows, err := q.Query(ctx, `
		SELECT b.id, b.sku, b.name, coalesce(b.packaging,''), coalesce(b.style,''), coalesce(b.composition_format,''),
		       coalesce(b.image_url,''), b.active, a.id, a.in_stock, a.quantity,
		       po.price, po.discount_price, po.discount_from, po.discount_until
		  FROM bouquets b
		  JOIN availability a ON a.bouquet_id = b.id`+fmt.Sprintf(offerJoin, "bouquet_id", "b.id")+`
		 WHERE b.shop_id = $1 ORDER BY b.sku`, shopID)
	if err != nil {
		return nil, fmt.Errorf("list bouquets: %w", err)
	}
	var out []BouquetView
	index := map[int64]int{}
	for rows.Next() {
		var b BouquetView
		var price *domain.Money
		if err := rows.Scan(&b.ID, &b.SKU, &b.Name, &b.Packaging, &b.Style, &b.CompositionFormat, &b.ImageURL, &b.Active,
			&b.Availability.ID, &b.Availability.InStock, &b.Availability.Quantity,
			&price, &b.Offer.DiscountPrice, &b.Offer.DiscountFrom, &b.Offer.DiscountUntil); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan bouquet: %w", err)
		}
		if price != nil {
			b.Offer.Price = *price
		}
		b.Items = []CompositionView{}
		index[b.ID] = len(out)
		out = append(out, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bouquets: %w", err)
	}
	items, err := q.Query(ctx, `
		SELECT bi.bouquet_id, f.id, coalesce(f.code,''), f.trade_name, f.color, coalesce(t.genus,''), bi.stems, f.taxon_confirmed
		  FROM bouquet_items bi JOIN bouquets b ON b.id = bi.bouquet_id
		  JOIN flowers f ON f.id = bi.flower_id LEFT JOIN plant_taxa t ON t.id = f.taxon_id
		 WHERE b.shop_id = $1 ORDER BY bi.bouquet_id, bi.id`, shopID)
	if err != nil {
		return nil, fmt.Errorf("list composition: %w", err)
	}
	defer items.Close()
	for items.Next() {
		var bid int64
		var c CompositionView
		if err := items.Scan(&bid, &c.FlowerID, &c.Code, &c.TradeName, &c.Color, &c.Genus, &c.Stems, &c.TaxonConfirmed); err != nil {
			return nil, fmt.Errorf("scan composition: %w", err)
		}
		if i, ok := index[bid]; ok {
			out[i].Items = append(out[i].Items, c)
		}
	}
	return out, items.Err()
}

// GiftView — подарок в кабинете флориста.
type GiftView struct {
	ID            int64        `json:"id"`
	SKU           string       `json:"sku"`
	Name          string       `json:"name"`
	Category      string       `json:"category"`
	Composition   string       `json:"composition"`
	AllergenCodes *[]string    `json:"allergen_codes"` // null = состав неизвестен
	Scent         domain.Level `json:"scent"`
	PetHazard     *bool        `json:"pet_hazard"`
	Active        bool         `json:"active"`
	Availability  Availability `json:"availability"`
	Offer         Offer        `json:"offer"`
}

// ListShopGifts возвращает подарки магазина.
func ListShopGifts(ctx context.Context, q store.Querier, shopID int64) ([]GiftView, error) {
	rows, err := q.Query(ctx, `
		SELECT g.id, g.sku, g.name, g.category, coalesce(g.composition,''), g.allergen_codes, g.scent, g.pet_hazard, g.active,
		       a.id, a.in_stock, a.quantity, po.price, po.discount_price, po.discount_from, po.discount_until
		  FROM gift_items g
		  JOIN availability a ON a.gift_item_id = g.id`+fmt.Sprintf(offerJoin, "gift_item_id", "g.id")+`
		 WHERE g.shop_id = $1 ORDER BY g.sku`, shopID)
	if err != nil {
		return nil, fmt.Errorf("list gifts: %w", err)
	}
	defer rows.Close()
	var out []GiftView
	for rows.Next() {
		var g GiftView
		var price *domain.Money
		var codes []string
		if err := rows.Scan(&g.ID, &g.SKU, &g.Name, &g.Category, &g.Composition, &codes, &g.Scent, &g.PetHazard, &g.Active,
			&g.Availability.ID, &g.Availability.InStock, &g.Availability.Quantity,
			&price, &g.Offer.DiscountPrice, &g.Offer.DiscountFrom, &g.Offer.DiscountUntil); err != nil {
			return nil, fmt.Errorf("scan gift: %w", err)
		}
		if codes != nil {
			g.AllergenCodes = &codes
		}
		if price != nil {
			g.Offer.Price = *price
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// FlowerView — цветок для флориста (подтверждение таксона, замены).
type FlowerView struct {
	ID             int64        `json:"id"`
	Code           string       `json:"code"`
	TradeName      string       `json:"trade_name"`
	Color          string       `json:"color"`
	Scent          domain.Level `json:"scent"`
	TaxonID        *int64       `json:"taxon_id"`
	Genus          string       `json:"genus"`
	TaxonConfirmed bool         `json:"taxon_confirmed"`
	Shared         bool         `json:"shared"`
}

// ListFlowers возвращает общий справочник и цветы магазина. unconfirmedOnly — только неподтверждённые,
// используемые в букетах этого магазина или принадлежащие ему.
func ListFlowers(ctx context.Context, q store.Querier, shopID int64, unconfirmedOnly bool) ([]FlowerView, error) {
	rows, err := q.Query(ctx, `
		SELECT f.id, coalesce(f.code,''), f.trade_name, f.color, f.scent, f.taxon_id, coalesce(t.genus,''), f.taxon_confirmed, f.shop_id IS NULL
		  FROM flowers f LEFT JOIN plant_taxa t ON t.id = f.taxon_id
		 WHERE (f.shop_id IS NULL OR f.shop_id = $1)
		   AND (NOT $2 OR (NOT f.taxon_confirmed AND (f.shop_id = $1 OR EXISTS (
		        SELECT 1 FROM bouquet_items bi JOIN bouquets b ON b.id = bi.bouquet_id WHERE bi.flower_id = f.id AND b.shop_id = $1))))
		 ORDER BY f.trade_name`, shopID, unconfirmedOnly)
	if err != nil {
		return nil, fmt.Errorf("list flowers: %w", err)
	}
	defer rows.Close()
	var out []FlowerView
	for rows.Next() {
		var f FlowerView
		if err := rows.Scan(&f.ID, &f.Code, &f.TradeName, &f.Color, &f.Scent, &f.TaxonID, &f.Genus, &f.TaxonConfirmed, &f.Shared); err != nil {
			return nil, fmt.Errorf("scan flower: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// TaxonView — запись справочника растений.
type TaxonView struct {
	ID             int64                      `json:"id"`
	ScientificName string                     `json:"scientific_name"`
	Genus          string                     `json:"genus"`
	Family         string                     `json:"family"`
	PetToxicity    map[string]domain.Toxicity `json:"pet_toxicity"`
	Pollen         domain.Level               `json:"pollen"`
	AllergenCodes  []string                   `json:"allergen_codes"`
	ASPCAURL       *string                    `json:"aspca_url"`
	ImageURL       *string                    `json:"image_url"`
	ImageLicense   *string                    `json:"image_license"`
	ImageAuthor    *string                    `json:"image_author"`
	Verified       bool                       `json:"verified"`
}

// ListTaxa возвращает справочник растений.
func ListTaxa(ctx context.Context, q store.Querier) ([]TaxonView, error) {
	rows, err := q.Query(ctx, `SELECT id, scientific_name, genus, family, pet_toxicity, pollen_level, allergen_codes,
		aspca_url, image_url, image_license, image_author, toxicity_verified FROM plant_taxa ORDER BY genus`)
	if err != nil {
		return nil, fmt.Errorf("list taxa: %w", err)
	}
	defer rows.Close()
	var out []TaxonView
	for rows.Next() {
		var t TaxonView
		if err := rows.Scan(&t.ID, &t.ScientificName, &t.Genus, &t.Family, &t.PetToxicity, &t.Pollen, &t.AllergenCodes,
			&t.ASPCAURL, &t.ImageURL, &t.ImageLicense, &t.ImageAuthor, &t.Verified); err != nil {
			return nil, fmt.Errorf("scan taxon: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ConfirmTaxon — флорист подтверждает растение (FR-C03). Разрешено для цветов магазина
// и для общих цветов, используемых в его букетах.
func ConfirmTaxon(ctx context.Context, q store.Querier, shopID, flowerID, taxonID int64) error {
	tag, err := q.Exec(ctx, `
		UPDATE flowers f SET taxon_id = $3, taxon_confirmed = true
		 WHERE f.id = $1 AND EXISTS (SELECT 1 FROM plant_taxa WHERE id = $3)
		   AND (f.shop_id = $2 OR (f.shop_id IS NULL AND EXISTS (
		        SELECT 1 FROM bouquet_items bi JOIN bouquets b ON b.id = bi.bouquet_id WHERE bi.flower_id = f.id AND b.shop_id = $2)))`,
		flowerID, shopID, taxonID)
	if err != nil {
		return fmt.Errorf("confirm taxon: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

// AvailabilityUpdate — PUT /availability/{id}: наличие и (необязательно) новая цена/скидка.
type AvailabilityUpdate struct {
	InStock       bool          `json:"in_stock"`
	Quantity      *int          `json:"quantity"`
	Price         *domain.Money `json:"price"`
	DiscountPrice *domain.Money `json:"discount_price"`
	DiscountFrom  *time.Time    `json:"discount_from"`
	DiscountUntil *time.Time    `json:"discount_until"`
}

// Validate проверяет согласованность цены и скидки.
func (u AvailabilityUpdate) Validate() map[string]any {
	errs := map[string]any{}
	if u.Quantity != nil && *u.Quantity < 0 {
		errs["quantity"] = "не может быть отрицательным"
	}
	if u.Price == nil && (u.DiscountPrice != nil || u.DiscountUntil != nil) {
		errs["price"] = "укажите цену вместе со скидкой"
	}
	if u.Price != nil {
		if *u.Price <= 0 {
			errs["price"] = "должна быть положительной"
		}
		if u.DiscountPrice != nil {
			if *u.DiscountPrice <= 0 || *u.DiscountPrice >= *u.Price {
				errs["discount_price"] = "должна быть больше 0 и меньше цены"
			}
			if u.DiscountUntil == nil {
				errs["discount_until"] = "у скидки должен быть срок"
			}
		}
		if u.DiscountFrom != nil && u.DiscountUntil != nil && !u.DiscountFrom.Before(*u.DiscountUntil) {
			errs["discount_until"] = "срок должен быть позже начала"
		}
	}
	return errs
}

// UpdateAvailability обновляет наличие (и цену) позиции магазина. Возвращает store.ErrNotFound,
// если позиция принадлежит другому магазину (§13: флорист меняет только свой магазин).
func UpdateAvailability(ctx context.Context, q store.Querier, shopID, availabilityID int64, u AvailabilityUpdate) error {
	var bouquetID, giftID *int64
	err := q.QueryRow(ctx, `
		SELECT a.bouquet_id, a.gift_item_id FROM availability a
		  LEFT JOIN bouquets b ON b.id = a.bouquet_id LEFT JOIN gift_items g ON g.id = a.gift_item_id
		 WHERE a.id = $1 AND coalesce(b.shop_id, g.shop_id) = $2`, availabilityID, shopID).Scan(&bouquetID, &giftID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("load availability: %w", err)
	}
	if _, err := q.Exec(ctx, `UPDATE availability SET in_stock = $2, quantity = $3, updated_at = now() WHERE id = $1`,
		availabilityID, u.InStock, u.Quantity); err != nil {
		return fmt.Errorf("update availability: %w", err)
	}
	if u.Price != nil {
		if _, err := q.Exec(ctx, `INSERT INTO price_offers (bouquet_id, gift_item_id, price, discount_price, discount_from, discount_until)
			VALUES ($1,$2,$3,$4,$5,$6)`, bouquetID, giftID, *u.Price, u.DiscountPrice, u.DiscountFrom, u.DiscountUntil); err != nil {
			return fmt.Errorf("insert price: %w", err)
		}
	}
	return nil
}
