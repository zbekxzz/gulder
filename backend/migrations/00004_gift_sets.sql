-- M4: наборы ассистента (§9). generator — источник набора (llm | fallback, §9.4); title — «характер»;
-- excluded_bouquet_ids — исключённые букеты (excluded_item_ids из Прил. A хранит исключённые подарки:
-- id букетов и подарков из разных таблиц и могут совпадать).

-- +goose Up
ALTER TABLE gift_sets ADD COLUMN generator text NOT NULL DEFAULT 'fallback' CHECK (generator IN ('llm', 'fallback'));
ALTER TABLE gift_sets ADD COLUMN title text;
ALTER TABLE gift_sets ADD COLUMN excluded_bouquet_ids bigint[] NOT NULL DEFAULT '{}';
CREATE INDEX ON gift_sets (run_id);

-- +goose Down
DROP INDEX gift_sets_run_id_idx;
ALTER TABLE gift_sets DROP COLUMN excluded_bouquet_ids;
ALTER TABLE gift_sets DROP COLUMN title;
ALTER TABLE gift_sets DROP COLUMN generator;
