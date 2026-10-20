package catalog

import (
	"context"
	"fmt"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Snapshot — каталог на момент Now в доменных типах: вход Rules Engine, ranker'а и ассистента.
// Price позиций — эффективная цена (со скидкой, если она действует).
type Snapshot struct {
	Now      time.Time
	Bouquets []domain.Bouquet
	Gifts    []domain.GiftItem
	Shops    map[int64]Shop
}

// BouquetByID ищет букет.
func (s Snapshot) BouquetByID(id int64) (domain.Bouquet, bool) {
	for _, b := range s.Bouquets {
		if b.ID == id {
			return b, true
		}
	}
	return domain.Bouquet{}, false
}

// GiftByID ищет подарок.
func (s Snapshot) GiftByID(id int64) (domain.GiftItem, bool) {
	for _, g := range s.Gifts {
		if g.ID == id {
			return g, true
		}
	}
	return domain.GiftItem{}, false
}

// LoadTaxa загружает справочник растений.
func LoadTaxa(ctx context.Context, q store.Querier) (map[int64]*domain.PlantTaxon, error) {
	rows, err := q.Query(ctx, `SELECT id, scientific_name, genus, family, pet_toxicity, pollen_level, allergen_codes,
		coalesce(aspca_url,''), coalesce(image_url,''), coalesce(image_license,''), coalesce(image_author,'') FROM plant_taxa`)
	if err != nil {
		return nil, fmt.Errorf("load taxa: %w", err)
	}
	defer rows.Close()
	out := map[int64]*domain.PlantTaxon{}
	for rows.Next() {
		t := &domain.PlantTaxon{}
		var pollen string
		if err := rows.Scan(&t.ID, &t.ScientificName, &t.Genus, &t.Family, &t.PetToxicity, &pollen, &t.AllergenCodes,
			&t.ASPCAURL, &t.ImageURL, &t.ImageLicense, &t.ImageAuthor); err != nil {
			return nil, fmt.Errorf("scan taxon: %w", err)
		}
		t.Pollen = domain.Level(pollen)
		out[t.ID] = t
	}
	return out, rows.Err()
}

// LoadFlowers загружает все цветы с таксонами.
func LoadFlowers(ctx context.Context, q store.Querier, taxa map[int64]*domain.PlantTaxon) (map[int64]domain.Flower, error) {
	rows, err := q.Query(ctx, `SELECT id, coalesce(code,''), trade_name, color, scent, taxon_id, taxon_confirmed, season_months FROM flowers`)
	if err != nil {
		return nil, fmt.Errorf("load flowers: %w", err)
	}
	defer rows.Close()
	out := map[int64]domain.Flower{}
	for rows.Next() {
		var f domain.Flower
		var scent string
		var taxonID *int64
		var months []int16
		if err := rows.Scan(&f.ID, &f.Code, &f.TradeName, &f.Color, &scent, &taxonID, &f.TaxonConfirmed, &months); err != nil {
			return nil, fmt.Errorf("scan flower: %w", err)
		}
		f.Scent = domain.Level(scent)
		if taxonID != nil {
			f.Taxon = taxa[*taxonID] // nil, если таксон пропал — Rules Engine заблокирует (F1)
		}
		for _, m := range months {
			f.SeasonMonths = append(f.SeasonMonths, int(m))
		}
		out[f.ID] = f
	}
	return out, rows.Err()
}

// LoadSnapshot загружает активный каталог.
func LoadSnapshot(ctx context.Context, q store.Querier, now time.Time) (Snapshot, error) {
	snap := Snapshot{Now: now, Shops: map[int64]Shop{}}
	shops, err := ListShops(ctx, q)
	if err != nil {
		return snap, err
	}
	for _, s := range shops {
		snap.Shops[s.ID] = s
	}
	taxa, err := LoadTaxa(ctx, q)
	if err != nil {
		return snap, err
	}
	flowers, err := LoadFlowers(ctx, q, taxa)
	if err != nil {
		return snap, err
	}
	if snap.Bouquets, err = loadBouquets(ctx, q, flowers, now); err != nil {
		return snap, err
	}
	if snap.Gifts, err = loadGifts(ctx, q, now); err != nil {
		return snap, err
	}
	return snap, nil
}

func loadBouquets(ctx context.Context, q store.Querier, flowers map[int64]domain.Flower, now time.Time) ([]domain.Bouquet, error) {
	rows, err := q.Query(ctx, `
		SELECT b.id, b.shop_id, b.sku, b.name, coalesce(b.packaging,''), coalesce(b.style,''), coalesce(b.composition_format,''),
		       coalesce(b.image_url,''), coalesce(a.in_stock, false),
		       po.price, po.discount_price, po.discount_from, po.discount_until
		  FROM bouquets b
		  LEFT JOIN availability a ON a.bouquet_id = b.id`+fmt.Sprintf(offerJoin, "bouquet_id", "b.id")+`
		 WHERE b.active AND po.price IS NOT NULL
		 ORDER BY b.id`)
	if err != nil {
		return nil, fmt.Errorf("load bouquets: %w", err)
	}
	var out []domain.Bouquet
	index := map[int64]int{}
	for rows.Next() {
		var b domain.Bouquet
		if err := rows.Scan(&b.ID, &b.ShopID, &b.SKU, &b.Name, &b.Packaging, &b.Style, &b.CompositionFormat, &b.ImageURL, &b.InStock,
			&b.Offer.Price, &b.Offer.DiscountPrice, &b.Offer.DiscountFrom, &b.Offer.DiscountUntil); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan bouquet: %w", err)
		}
		b.Price = b.Offer.Effective(now)
		index[b.ID] = len(out)
		out = append(out, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load bouquets: %w", err)
	}
	items, err := q.Query(ctx, `SELECT bouquet_id, flower_id, stems FROM bouquet_items ORDER BY bouquet_id, id`)
	if err != nil {
		return nil, fmt.Errorf("load bouquet items: %w", err)
	}
	defer items.Close()
	for items.Next() {
		var bid, fid int64
		var stems int
		if err := items.Scan(&bid, &fid, &stems); err != nil {
			return nil, fmt.Errorf("scan bouquet item: %w", err)
		}
		i, ok := index[bid]
		if !ok {
			continue
		}
		out[i].Items = append(out[i].Items, domain.BouquetItem{Flower: flowers[fid], Stems: stems})
	}
	return out, items.Err()
}

func loadGifts(ctx context.Context, q store.Querier, now time.Time) ([]domain.GiftItem, error) {
	rows, err := q.Query(ctx, `
		SELECT g.id, g.shop_id, g.sku, g.name, g.category, coalesce(g.composition,''), g.allergen_codes, g.scent, g.pet_hazard,
		       coalesce(a.in_stock, false), po.price, po.discount_price, po.discount_from, po.discount_until
		  FROM gift_items g
		  LEFT JOIN availability a ON a.gift_item_id = g.id`+fmt.Sprintf(offerJoin, "gift_item_id", "g.id")+`
		 WHERE g.active AND po.price IS NOT NULL
		 ORDER BY g.id`)
	if err != nil {
		return nil, fmt.Errorf("load gifts: %w", err)
	}
	defer rows.Close()
	var out []domain.GiftItem
	for rows.Next() {
		var g domain.GiftItem
		var codes []string
		var scent string
		if err := rows.Scan(&g.ID, &g.ShopID, &g.SKU, &g.Name, &g.Category, &g.Composition, &codes, &scent, &g.PetHazard,
			&g.InStock, &g.Offer.Price, &g.Offer.DiscountPrice, &g.Offer.DiscountFrom, &g.Offer.DiscountUntil); err != nil {
			return nil, fmt.Errorf("scan gift: %w", err)
		}
		if codes != nil { // NULL = состав неизвестен; {} = проверено, аллергенов нет
			g.AllergenCodes = &codes
		}
		g.Scent = domain.Level(scent)
		g.Price = g.Offer.Effective(now)
		out = append(out, g)
	}
	return out, rows.Err()
}
