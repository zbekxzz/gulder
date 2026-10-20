-- Стабильные коды для идемпотентного сида и CSV-импорта (§12, §18.3; ADR 0004).
-- shops.code / flowers.code — естественные ключи из data/*.csv.
-- flowers.shop_id — владелец позиции: NULL = общий справочник, флорист правит только свои (§13).
-- plant_taxa.toxicity_verified — строка сверена с ASPCA (M7); до сверки данные черновые (Прил. B).

-- +goose Up
ALTER TABLE shops ADD COLUMN code text UNIQUE;
ALTER TABLE flowers ADD COLUMN code text UNIQUE;
ALTER TABLE flowers ADD COLUMN shop_id bigint REFERENCES shops(id) ON DELETE CASCADE;
CREATE INDEX ON bouquet_items (flower_id);
ALTER TABLE plant_taxa ADD COLUMN toxicity_verified boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE plant_taxa DROP COLUMN toxicity_verified;
DROP INDEX bouquet_items_flower_id_idx;
ALTER TABLE flowers DROP COLUMN shop_id;
ALTER TABLE flowers DROP COLUMN code;
ALTER TABLE shops DROP COLUMN code;
