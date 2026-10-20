package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SeedReport — сколько записей добавлено сидом.
type SeedReport struct {
	Taxa, Shops, Flowers, Bouquets, Gifts, NumerologyRows, Judgements int
}

// Seed загружает data/*.csv. Идемпотентен и не затирает ручные правки: существующие
// записи (по естественным ключам) пропускаются целиком (§18.3).
func Seed(ctx context.Context, tx pgx.Tx, dataDir string) (SeedReport, error) {
	var rep SeedReport
	var err error
	if rep.Taxa, err = seedTaxa(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	if rep.Shops, err = seedShops(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	if rep.Flowers, err = seedFlowers(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	if rep.Bouquets, err = seedBouquets(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	if rep.Gifts, err = seedGifts(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	if rep.NumerologyRows, err = seedNumerology(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	if rep.Judgements, err = seedJudgements(ctx, tx, dataDir); err != nil {
		return rep, err
	}
	return rep, nil
}

func parseFile[T any](dir, name string, parse func(io.Reader) ([]T, []RowError, error)) ([]T, error) {
	f, err := os.Open(filepath.Join(dir, name)) //nolint:gosec // путь задаёт оператор (флаг -data)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	rows, rowErrs, err := parse(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	if len(rowErrs) > 0 {
		msgs := make([]string, 0, len(rowErrs))
		for _, e := range rowErrs {
			msgs = append(msgs, fmt.Sprintf("line %d, %s: %s", e.Line, e.Field, e.Message))
		}
		return nil, fmt.Errorf("%s: %s", name, strings.Join(msgs, "; "))
	}
	return rows, nil
}

func seedTaxa(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	rows, err := parseFile(dir, "plant_taxa.csv", ParseTaxa)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		tag, err := tx.Exec(ctx, `
			INSERT INTO plant_taxa (gbif_key, scientific_name, genus, family, pet_toxicity, aspca_url,
			                        pollen_level, allergen_codes, image_url, image_license, image_author, toxicity_verified)
			VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),$7,$8,NULLIF($9,''),NULLIF($10,''),NULLIF($11,''),$12)
			ON CONFLICT (scientific_name) DO NOTHING`,
			r.GBIFKey, r.Taxon.ScientificName, r.Taxon.Genus, r.Taxon.Family, r.Taxon.PetToxicity, r.Taxon.ASPCAURL,
			string(r.Taxon.Pollen), nonNil(r.Taxon.AllergenCodes), r.Taxon.ImageURL, r.Taxon.ImageLicense, r.Taxon.ImageAuthor, r.Verified)
		if err != nil {
			return n, fmt.Errorf("insert taxon %s: %w", r.Taxon.ScientificName, err)
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

func seedShops(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	rows, err := parseFile(dir, "shops.csv", ParseShops)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		tag, err := tx.Exec(ctx, `
			INSERT INTO shops (code, name, address, lat, lon, opening_hours, timezone)
			VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (code) DO NOTHING`,
			r.Code, r.Name, r.Address, r.Lat, r.Lon, r.OpeningHours, r.TZ)
		if err != nil {
			return n, fmt.Errorf("insert shop %s: %w", r.Code, err)
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

// ResolveTaxon ищет таксон по научному названию, затем по роду (если он однозначен).
// Не найден → nil: цветок создаётся с taxon_id=NULL, taxon_confirmed=false (FR-C03).
func ResolveTaxon(ctx context.Context, q pgx.Tx, scientificName, genus string) (*int64, error) {
	var id int64
	if scientificName != "" {
		err := q.QueryRow(ctx, `SELECT id FROM plant_taxa WHERE lower(scientific_name) = lower($1)`, scientificName).Scan(&id)
		if err == nil {
			return &id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("resolve taxon: %w", err)
		}
	}
	rows, err := q.Query(ctx, `SELECT id FROM plant_taxa WHERE lower(genus) = lower($1) LIMIT 2`, genus)
	if err != nil {
		return nil, fmt.Errorf("resolve genus: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("resolve genus: %w", err)
	}
	if len(ids) == 1 {
		return &ids[0], nil
	}
	return nil, nil
}

func seedFlowers(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	rows, err := parseFile(dir, "flowers.csv", ParseFlowers)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		taxonID, err := ResolveTaxon(ctx, tx, r.ScientificName, r.Genus)
		if err != nil {
			return n, err
		}
		confirmed := r.TaxonConfirmed && taxonID != nil
		tag, err := tx.Exec(ctx, `
			INSERT INTO flowers (code, taxon_id, taxon_confirmed, trade_name, color, scent, season_months, care_notes, restrictions)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,'')) ON CONFLICT (code) DO NOTHING`,
			r.Code, taxonID, confirmed, r.TradeName, r.Color, string(r.Scent), r.SeasonMonths, r.CareNotes, r.Restrictions)
		if err != nil {
			return n, fmt.Errorf("insert flower %s: %w", r.Code, err)
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}

func shopIDs(ctx context.Context, tx pgx.Tx) (map[string]int64, error) {
	rows, err := tx.Query(ctx, `SELECT code, id FROM shops WHERE code IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("load shops: %w", err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var code string
		var id int64
		if err := rows.Scan(&code, &id); err != nil {
			return nil, fmt.Errorf("scan shop: %w", err)
		}
		out[code] = id
	}
	return out, rows.Err()
}

func seedBouquets(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	rows, err := parseFile(dir, "bouquets.csv", ParseBouquets)
	if err != nil {
		return 0, err
	}
	items, err := parseFile(dir, "bouquet_items.csv", ParseBouquetItems)
	if err != nil {
		return 0, err
	}
	comp := map[string][]CompositionItem{}
	for _, it := range items {
		key := it.ShopCode + "/" + it.SKU
		comp[key] = append(comp[key], it.Item)
	}
	shops, err := shopIDs(ctx, tx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		shopID, ok := shops[r.ShopCode]
		if !ok {
			return n, fmt.Errorf("bouquets.csv line %d: unknown shop_code %q", r.Line, r.ShopCode)
		}
		r.Composition = comp[r.ShopCode+"/"+r.SKU]
		if len(r.Composition) == 0 {
			return n, fmt.Errorf("bouquets.csv line %d: no composition in bouquet_items.csv", r.Line)
		}
		inserted, err := InsertBouquet(ctx, tx, shopID, r, false)
		if err != nil {
			return n, err
		}
		if inserted {
			n++
		}
	}
	return n, nil
}

// InsertBouquet создаёт букет с составом, наличием и ценой. update=false — существующий
// (shop_id, sku) не трогается; update=true — обновляет карточку, состав, наличие и цену.
// Возвращает true, если букет был создан.
func InsertBouquet(ctx context.Context, tx pgx.Tx, shopID int64, r BouquetRow, update bool) (bool, error) {
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO bouquets (shop_id, sku, name, packaging, style, composition_format, image_url)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''))
		ON CONFLICT (shop_id, sku) DO NOTHING RETURNING id`,
		shopID, r.SKU, r.Name, r.Packaging, r.Style, r.CompositionFormat, r.ImageURL).Scan(&id)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		if !update {
			return false, nil
		}
		err = tx.QueryRow(ctx, `
			UPDATE bouquets SET name=$3, packaging=NULLIF($4,''), style=NULLIF($5,''),
			       composition_format=NULLIF($6,''), image_url=NULLIF($7,''), active=true
			 WHERE shop_id=$1 AND sku=$2 RETURNING id`,
			shopID, r.SKU, r.Name, r.Packaging, r.Style, r.CompositionFormat, r.ImageURL).Scan(&id)
		if err != nil {
			return false, fmt.Errorf("update bouquet %s: %w", r.SKU, err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM bouquet_items WHERE bouquet_id=$1`, id); err != nil {
			return false, fmt.Errorf("reset composition %s: %w", r.SKU, err)
		}
	} else if err != nil {
		return false, fmt.Errorf("insert bouquet %s: %w", r.SKU, err)
	}
	for _, it := range r.Composition {
		tag, err := tx.Exec(ctx, `
			INSERT INTO bouquet_items (bouquet_id, flower_id, stems)
			SELECT $1, f.id, $3 FROM flowers f WHERE f.code = $2 AND (f.shop_id IS NULL OR f.shop_id = $4)
			ON CONFLICT (bouquet_id, flower_id) DO UPDATE SET stems = bouquet_items.stems + EXCLUDED.stems`,
			id, it.FlowerCode, it.Stems, shopID)
		if err != nil {
			return false, fmt.Errorf("insert composition %s: %w", r.SKU, err)
		}
		if tag.RowsAffected() == 0 {
			return false, &ImportError{Msg: fmt.Sprintf("неизвестный код цветка %q (доступны общий справочник и цветы вашего магазина)", it.FlowerCode)}
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO availability (bouquet_id, in_stock) VALUES ($1,$2)
		ON CONFLICT (bouquet_id) DO UPDATE SET in_stock = EXCLUDED.in_stock, updated_at = now()`, id, r.InStock); err != nil {
		return false, fmt.Errorf("availability %s: %w", r.SKU, err)
	}
	if err := insertOffer(ctx, tx, "bouquet_id", id, r.Offer, update); err != nil {
		return false, fmt.Errorf("price %s: %w", r.SKU, err)
	}
	return created, nil
}

// insertOffer добавляет запись цены. Без force — только если цен ещё нет (не затирать ручные правки).
func insertOffer(ctx context.Context, tx pgx.Tx, col string, id int64, o OfferFields, force bool) error {
	if col != "bouquet_id" && col != "gift_item_id" {
		return fmt.Errorf("bad offer column %q", col)
	}
	if !force {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM price_offers WHERE `+col+` = $1)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("check offer: %w", err)
		}
		if exists {
			return nil
		}
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO price_offers (`+col+`, price, discount_price, discount_from, discount_until)
		VALUES ($1,$2,$3,$4,$5)`, id, o.Price, o.DiscountPrice, o.DiscountFrom, o.DiscountUntil)
	if err != nil {
		return fmt.Errorf("insert offer: %w", err)
	}
	return nil
}

func seedGifts(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	rows, err := parseFile(dir, "gifts.csv", ParseGifts)
	if err != nil {
		return 0, err
	}
	shops, err := shopIDs(ctx, tx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		shopID, ok := shops[r.ShopCode]
		if !ok {
			return n, fmt.Errorf("gifts.csv line %d: unknown shop_code %q", r.Line, r.ShopCode)
		}
		created, err := InsertGift(ctx, tx, shopID, r, false)
		if err != nil {
			return n, err
		}
		if created {
			n++
		}
	}
	return n, nil
}

// InsertGift создаёт (или при update=true обновляет) подарок с наличием и ценой.
func InsertGift(ctx context.Context, tx pgx.Tx, shopID int64, r GiftRow, update bool) (bool, error) {
	var allergens any // NULL = состав неизвестен
	if r.AllergenCodes != nil {
		allergens = nonNil(*r.AllergenCodes)
	}
	var id int64
	err := tx.QueryRow(ctx, `
		INSERT INTO gift_items (shop_id, sku, name, category, composition, allergen_codes, scent, pet_hazard)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8)
		ON CONFLICT (shop_id, sku) DO NOTHING RETURNING id`,
		shopID, r.SKU, r.Name, r.Category, r.Composition, allergens, string(r.Scent), r.PetHazard).Scan(&id)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		if !update {
			return false, nil
		}
		err = tx.QueryRow(ctx, `
			UPDATE gift_items SET name=$3, category=$4, composition=NULLIF($5,''), allergen_codes=$6,
			       scent=$7, pet_hazard=$8, active=true
			 WHERE shop_id=$1 AND sku=$2 RETURNING id`,
			shopID, r.SKU, r.Name, r.Category, r.Composition, allergens, string(r.Scent), r.PetHazard).Scan(&id)
		if err != nil {
			return false, fmt.Errorf("update gift %s: %w", r.SKU, err)
		}
	} else if err != nil {
		return false, fmt.Errorf("insert gift %s: %w", r.SKU, err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO availability (gift_item_id, in_stock) VALUES ($1,$2)
		ON CONFLICT (gift_item_id) DO UPDATE SET in_stock = EXCLUDED.in_stock, updated_at = now()`, id, r.InStock); err != nil {
		return false, fmt.Errorf("availability %s: %w", r.SKU, err)
	}
	if err := insertOffer(ctx, tx, "gift_item_id", id, r.Offer, update); err != nil {
		return false, fmt.Errorf("price %s: %w", r.SKU, err)
	}
	return created, nil
}

func seedNumerology(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	rows, err := parseFile(dir, "numerology.csv", ParseNumerology)
	if err != nil {
		return 0, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM numerology_versions)`).Scan(&exists); err != nil {
		return 0, fmt.Errorf("check numerology: %w", err)
	}
	if exists {
		return 0, nil
	}
	var versionID int64
	if err := tx.QueryRow(ctx, `INSERT INTO numerology_versions (version, is_active) VALUES (1, true) RETURNING id`).Scan(&versionID); err != nil {
		return 0, fmt.Errorf("create numerology version: %w", err)
	}
	for _, r := range rows {
		if _, err := tx.Exec(ctx, `INSERT INTO numerology_table (version_id, number, flowers, colors, meaning) VALUES ($1,$2,$3,$4,$5)`,
			versionID, r.Number, nonNil(r.Flowers), nonNil(r.Colors), r.Meaning); err != nil {
			return 0, fmt.Errorf("insert numerology %d: %w", r.Number, err)
		}
	}
	return len(rows), nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// seedJudgements загружает экспертный набор, если файл есть. Существующие (версия, случай, букет) не трогаются.
func seedJudgements(ctx context.Context, tx pgx.Tx, dir string) (int, error) {
	if _, err := os.Stat(filepath.Join(dir, "expert_judgements.csv")); errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	rows, err := parseFile(dir, "expert_judgements.csv", ParseJudgements)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range rows {
		tag, err := tx.Exec(ctx, `
			INSERT INTO expert_judgements (dataset_version, case_id, recipient_snapshot, occasion_snapshot, bouquet_id, grade, is_unsafe, reason)
			SELECT $1, $2, $3, $4, b.id, $7, $8, $9 FROM bouquets b JOIN shops s ON s.id = b.shop_id
			 WHERE s.code = $5 AND b.sku = $6
			ON CONFLICT (dataset_version, case_id, bouquet_id) DO NOTHING`,
			r.DatasetVersion, r.CaseID, r.RecipientSnapshot, r.OccasionSnapshot, r.ShopCode, r.SKU, r.Grade, r.IsUnsafe, r.Reason)
		if err != nil {
			return n, fmt.Errorf("insert judgement line %d: %w", r.Line, err)
		}
		n += int(tag.RowsAffected())
	}
	return n, nil
}
