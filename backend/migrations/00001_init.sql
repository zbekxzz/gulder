-- Миграция 00001 — снимок схемы из Приложения A (TASK.md). Не править: схема меняется новыми миграциями.

-- +goose Up
-- Flowers — схема БД (PostgreSQL 14+)
-- Покрывает таблицы из разделов 9 и 12 ТЗ.
-- Соглашения: id — bigint identity, время — timestamptz, деньги — numeric(12,2) в тенге (KZT).


-- ─────────────────────────── Справочные типы ───────────────────────────

CREATE TYPE user_role        AS ENUM ('giver', 'florist', 'manager');
CREATE TYPE attr_source      AS ENUM ('giver', 'recipient', 'feedback', 'system');
-- «знаю» / «предполагаю» / «не знаю» (FR-P03). 'unknown' НИКОГДА не означает «нет ограничения».
CREATE TYPE attr_confidence  AS ENUM ('know', 'assume', 'unknown');
CREATE TYPE scent_level      AS ENUM ('none', 'low', 'medium', 'strong');
CREATE TYPE toxicity_level   AS ENUM ('none', 'mild', 'moderate', 'severe', 'unknown');
CREATE TYPE gift_category    AS ENUM ('card', 'sweets', 'soft_toy', 'fragrance', 'jewelry', 'candle', 'other');
CREATE TYPE substitution_status AS ENUM ('proposed', 'approved', 'blocked');

-- ─────────────────────────── Пользователи и магазины ───────────────────────────

CREATE TABLE users (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         text NOT NULL UNIQUE,
    display_name  text,
    role          user_role NOT NULL DEFAULT 'giver',
    shop_id       bigint,                         -- для флористов (FK ниже)
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shops (                              -- FR-C05
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          text NOT NULL,
    address       text NOT NULL,
    lat           double precision NOT NULL CHECK (lat BETWEEN -90 AND 90),
    lon           double precision NOT NULL CHECK (lon BETWEEN -180 AND 180),
    -- {"mon":[["09:00","21:00"]], "sun":[]} — несколько интервалов в день
    opening_hours jsonb NOT NULL DEFAULT '{}'::jsonb,
    timezone      text NOT NULL DEFAULT 'Asia/Almaty',
    created_at    timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE users ADD CONSTRAINT users_shop_fk FOREIGN KEY (shop_id) REFERENCES shops(id) ON DELETE SET NULL;

-- ─────────────────────────── Получатели ───────────────────────────

CREATE TABLE recipients (                         -- FR-P01
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    giver_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname      text NOT NULL,                  -- «Мама», «Айгерим»
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON recipients (giver_id);

-- EAV: одно поле профиля = одна строка с value/source/confidence (FR-P02, FR-P03).
-- Ключи: allergies, asthma, pets, scent_sensitivity, liked_flowers, disliked_flowers,
--        liked_colors, disliked_colors, style, composition_format, birth_date (FR-N07)
CREATE TABLE recipient_attributes (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipient_id  bigint NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    key           text NOT NULL,
    value         jsonb,                          -- NULL допустим только при confidence='unknown'
    source        attr_source NOT NULL,
    confidence    attr_confidence NOT NULL,
    -- FR-P05: получатель разрешил использовать ответ в будущем
    reusable      boolean NOT NULL DEFAULT true,
    -- раздел 10: ответ получателя скрыт от дарителя на уровне конкретного ограничения
    hidden_from_giver boolean GENERATED ALWAYS AS (source = 'recipient') STORED,
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (recipient_id, key),
    CHECK (value IS NOT NULL OR confidence = 'unknown')
);

CREATE TABLE recipient_invites (                  -- FR-P04 (P1)
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipient_id  bigint NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    token_hash    text NOT NULL UNIQUE,           -- храним только хэш токена
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz,                    -- одноразовость
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE occasions (                          -- FR-O01
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    recipient_id  bigint NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    type          text NOT NULL,                  -- birthday, apology, march8, wedding...
    relationship  text,                           -- partner, mother, colleague...
    date          date,
    budget_min    numeric(12,2) NOT NULL DEFAULT 0 CHECK (budget_min >= 0),
    budget_max    numeric(12,2) NOT NULL CHECK (budget_max > 0),
    place         text,                           -- home, office, restaurant...
    formality     smallint CHECK (formality BETWEEN 1 AND 5),
    emotion_goal  text,                           -- joy, gratitude, romance, support...
    numerology_enabled boolean NOT NULL DEFAULT false,  -- FR-N01
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (budget_min <= budget_max)
);

-- ─────────────────────────── Ботаника и каталог ───────────────────────────

CREATE TABLE plant_taxa (                         -- GBIF + ASPCA
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    gbif_key      bigint UNIQUE,
    scientific_name text NOT NULL UNIQUE,         -- 'Lilium'
    genus         text NOT NULL,
    family        text NOT NULL,                  -- 'Liliaceae'
    -- ASPCA: {"cat":"severe","dog":"none","horse":"none"}
    pet_toxicity  jsonb NOT NULL DEFAULT '{"cat":"unknown","dog":"unknown","horse":"unknown"}'::jsonb,
    aspca_url     text,
    pollen_level  scent_level NOT NULL DEFAULT 'medium',
    allergen_codes text[] NOT NULL DEFAULT '{}',  -- доп. коды, напр. 'latex'
    image_url     text,
    image_license text,                           -- раздел 8: лицензия на каждое изображение
    image_author  text
);

CREATE TABLE flowers (                            -- FR-C01: торговая позиция «цветок»
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    taxon_id      bigint REFERENCES plant_taxa(id),   -- NULL = не опознан → FR-C03
    taxon_confirmed boolean NOT NULL DEFAULT false,   -- подтверждено флористом
    trade_name    text NOT NULL,                  -- 'Роза Red Naomi'
    color         text NOT NULL,
    scent         scent_level NOT NULL DEFAULT 'low',
    season_months smallint[] NOT NULL DEFAULT '{1,2,3,4,5,6,7,8,9,10,11,12}',
    care_notes    text,
    restrictions  text
);

CREATE TABLE bouquets (                           -- FR-C02
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    shop_id       bigint NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    sku           text NOT NULL,
    name          text NOT NULL,
    packaging     text,
    style         text,                           -- classic, minimal, rustic...
    composition_format text,                      -- bouquet, box, basket, single
    image_url     text,
    active        boolean NOT NULL DEFAULT true,
    UNIQUE (shop_id, sku)
);

CREATE TABLE bouquet_items (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bouquet_id    bigint NOT NULL REFERENCES bouquets(id) ON DELETE CASCADE,
    flower_id     bigint NOT NULL REFERENCES flowers(id),
    stems         integer NOT NULL CHECK (stems > 0),
    UNIQUE (bouquet_id, flower_id)
);

CREATE TABLE gift_items (                         -- FR-C04
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    shop_id       bigint NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    sku           text NOT NULL,
    name          text NOT NULL,
    category      gift_category NOT NULL,
    composition   text,
    -- NULL = состав неизвестен (≠ «аллергенов нет»); '{}' = проверено, аллергенов нет
    allergen_codes text[],
    scent         scent_level NOT NULL DEFAULT 'none',
    pet_hazard    boolean,                        -- напр. шоколад/ксилит для собак; NULL = неизвестно
    image_url     text,
    active        boolean NOT NULL DEFAULT true,
    UNIQUE (shop_id, sku)
);

-- Доступность и цены отделены от карточки: флорист меняет их часто (PUT /availability/{id})
CREATE TABLE availability (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bouquet_id    bigint REFERENCES bouquets(id) ON DELETE CASCADE,
    gift_item_id  bigint REFERENCES gift_items(id) ON DELETE CASCADE,
    in_stock      boolean NOT NULL DEFAULT true,
    quantity      integer CHECK (quantity >= 0),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((bouquet_id IS NULL) <> (gift_item_id IS NULL)),
    UNIQUE (bouquet_id),
    UNIQUE (gift_item_id)
);

CREATE TABLE price_offers (                       -- цена, скидка и срок скидки (FR-C01, FR-L04)
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bouquet_id    bigint REFERENCES bouquets(id) ON DELETE CASCADE,
    gift_item_id  bigint REFERENCES gift_items(id) ON DELETE CASCADE,
    price         numeric(12,2) NOT NULL CHECK (price > 0),
    discount_price numeric(12,2) CHECK (discount_price > 0),
    discount_from timestamptz,
    discount_until timestamptz,
    currency      char(3) NOT NULL DEFAULT 'KZT',
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK ((bouquet_id IS NULL) <> (gift_item_id IS NULL)),
    CHECK (discount_price IS NULL OR discount_price < price),
    CHECK (discount_price IS NULL OR discount_until IS NOT NULL)
);
CREATE INDEX ON price_offers (bouquet_id);
CREATE INDEX ON price_offers (gift_item_id);

-- Действующая цена: удобна для фильтров «Дешевле» и «Со скидкой»
CREATE VIEW current_prices AS
SELECT DISTINCT ON (coalesce(bouquet_id, -gift_item_id))
       id AS offer_id, bouquet_id, gift_item_id, price,
       CASE WHEN discount_price IS NOT NULL
             AND now() >= coalesce(discount_from, '-infinity') AND now() < discount_until
            THEN discount_price END AS active_discount_price,
       discount_until
FROM price_offers
ORDER BY coalesce(bouquet_id, -gift_item_id), created_at DESC;

-- ─────────────────────────── Рекомендации ───────────────────────────

CREATE TABLE recommendation_runs (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occasion_id   bigint NOT NULL REFERENCES occasions(id) ON DELETE CASCADE,
    model_version text NOT NULL,                  -- 'baseline-0.1'
    rules_version text NOT NULL,
    numerology_used boolean NOT NULL DEFAULT false,  -- FR-N06: лог использования фактора
    numerology_version_id bigint,                 -- FK ниже
    filters       jsonb NOT NULL DEFAULT '{}'::jsonb,  -- {"nearby":true,"discount":false}
    -- [{bouquet_id, score, confidence, reasons[], conflicts[], missing[]}]
    results       jsonb NOT NULL,
    excluded      jsonb NOT NULL DEFAULT '[]'::jsonb,  -- что отсёк Rules Engine и почему
    clarifying_question text,                     -- FR-R04
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE gift_sets (                          -- FR-G01..G04
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id        bigint NOT NULL REFERENCES recommendation_runs(id) ON DELETE CASCADE,
    budget        numeric(12,2) NOT NULL CHECK (budget > 0),
    bouquet_id    bigint NOT NULL REFERENCES bouquets(id),
    gift_item_ids bigint[] NOT NULL DEFAULT '{}',
    total_price   numeric(12,2) NOT NULL,
    reasons       jsonb NOT NULL DEFAULT '[]'::jsonb,
    warnings      jsonb NOT NULL DEFAULT '[]'::jsonb,
    excluded_item_ids bigint[] NOT NULL DEFAULT '{}',  -- позиции, которые даритель исключил
    parent_set_id bigint REFERENCES gift_sets(id),      -- набор после замены
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (total_price <= budget)                 -- критерий приёмки 4 на уровне БД
);

CREATE TABLE substitutions (                      -- FR-S01
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    bouquet_id    bigint NOT NULL REFERENCES bouquets(id) ON DELETE CASCADE,
    from_flower_id bigint NOT NULL REFERENCES flowers(id),
    to_flower_id  bigint NOT NULL REFERENCES flowers(id),
    occasion_id   bigint REFERENCES occasions(id) ON DELETE CASCADE,
    proposed_by   bigint REFERENCES users(id),
    status        substitution_status NOT NULL,
    rule_violations jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE feedback (                           -- FR-F01: привязан к одному получателю
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id        bigint NOT NULL REFERENCES recommendation_runs(id) ON DELETE CASCADE,
    recipient_id  bigint NOT NULL REFERENCES recipients(id) ON DELETE CASCADE,
    bouquet_id    bigint REFERENCES bouquets(id),
    rating        smallint CHECK (rating BETWEEN 1 AND 5),
    liked         boolean,
    comment       text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- ─────────────────────────── Нумерология ───────────────────────────

CREATE TABLE numerology_versions (                -- FR-N03: версионирование
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version       integer NOT NULL UNIQUE,
    created_by    bigint REFERENCES users(id),
    is_active     boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX one_active_numerology ON numerology_versions (is_active) WHERE is_active;
ALTER TABLE recommendation_runs ADD CONSTRAINT runs_numerology_fk
    FOREIGN KEY (numerology_version_id) REFERENCES numerology_versions(id);

CREATE TABLE numerology_table (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    version_id    bigint NOT NULL REFERENCES numerology_versions(id) ON DELETE CASCADE,
    number        smallint NOT NULL CHECK (number BETWEEN 1 AND 9 OR number IN (11, 22, 33)),
    flowers       text[] NOT NULL DEFAULT '{}',   -- genus: {'Rosa','Tulipa'}
    colors        text[] NOT NULL DEFAULT '{}',
    meaning       text NOT NULL,
    UNIQUE (version_id, number)
);

-- ─────────────────────────── Экспертная разметка и аудит ───────────────────────────

CREATE TABLE expert_judgements (                  -- ≥100 случаев, оценка 0–3 (раздел 8)
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    dataset_version text NOT NULL,                -- RM-07: версионированный эталон
    case_id       text NOT NULL,                  -- для split по получателю
    recipient_snapshot jsonb NOT NULL,            -- обезличенный профиль
    occasion_snapshot  jsonb NOT NULL,
    bouquet_id    bigint NOT NULL REFERENCES bouquets(id),
    grade         smallint NOT NULL CHECK (grade BETWEEN 0 AND 3),
    is_unsafe     boolean NOT NULL DEFAULT false, -- для unsafe recommendation rate
    reason        text NOT NULL,
    expert_id     bigint REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (dataset_version, case_id, bouquet_id)
);

CREATE TABLE audit_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id      bigint REFERENCES users(id),
    action        text NOT NULL,                  -- 'substitution.blocked', 'numerology.updated'...
    entity        text NOT NULL,
    entity_id     bigint,
    payload       jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON audit_events (entity, entity_id);

-- ─────────────────────────── Исследовательский режим (раздел 12) ───────────────────────────
-- Отдельная схема: проще ограничить доступ (RM-03).

CREATE SCHEMA research;

CREATE TABLE research.research_consents (         -- RM-01, RM-02
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    consent_version text NOT NULL,
    granted_at    timestamptz NOT NULL DEFAULT now(),
    revoked_at    timestamptz
);
CREATE UNIQUE INDEX one_active_consent ON research.research_consents (user_id) WHERE revoked_at IS NULL;

-- Связка research_id ↔ user_id. Доступ — только роли research_admin (RM-03).
CREATE TABLE research.research_subjects (
    research_id   uuid PRIMARY KEY,
    user_id       bigint NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE research.experiment_runs (           -- RM-04, RM-05
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    model_version text NOT NULL,
    dataset_version text NOT NULL,
    split_id      text NOT NULL,                  -- baseline и модель сравниваются на одном split
    seed          integer NOT NULL,
    params        jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- {"ndcg@5":{"mean":0.71,"ci95":[0.66,0.75]}, "unsafe_rate":0, "coverage":0.42, "ece":0.08}
    metrics       jsonb NOT NULL,
    numerology_in_features boolean NOT NULL DEFAULT false,  -- RM-07, FR-N06
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE research.research_exports (          -- RM-06
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    requested_by  bigint REFERENCES users(id),
    dataset_version text NOT NULL,
    data_dictionary_version text NOT NULL,
    min_group_size integer NOT NULL DEFAULT 5 CHECK (min_group_size >= 5),
    row_count     integer,
    file_uri      text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Критерий приёмки 10: без согласия в research-таблицах нет записей.
-- +goose StatementBegin
CREATE FUNCTION research.require_consent() RETURNS trigger AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM research.research_consents c
                   WHERE c.user_id = NEW.user_id AND c.revoked_at IS NULL) THEN
        RAISE EXCEPTION 'research: no active consent for user %', NEW.user_id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER research_subjects_need_consent
    BEFORE INSERT ON research.research_subjects
    FOR EACH ROW EXECUTE FUNCTION research.require_consent();

-- RM-02: отзыв согласия в один шаг — удаляет связку, данные выпадают из будущих экспортов.
-- +goose StatementBegin
CREATE FUNCTION research.revoke_consent(p_user_id bigint) RETURNS void AS $$
BEGIN
    UPDATE research.research_consents SET revoked_at = now()
     WHERE user_id = p_user_id AND revoked_at IS NULL;
    DELETE FROM research.research_subjects WHERE user_id = p_user_id;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP SCHEMA research CASCADE;
DROP TABLE audit_events;
DROP TABLE expert_judgements;
ALTER TABLE recommendation_runs DROP CONSTRAINT runs_numerology_fk;
DROP TABLE numerology_table;
DROP TABLE numerology_versions;
DROP TABLE feedback;
DROP TABLE substitutions;
DROP TABLE gift_sets;
DROP TABLE recommendation_runs;
DROP VIEW current_prices;
DROP TABLE price_offers;
DROP TABLE availability;
DROP TABLE gift_items;
DROP TABLE bouquet_items;
DROP TABLE bouquets;
DROP TABLE flowers;
DROP TABLE plant_taxa;
DROP TABLE occasions;
DROP TABLE recipient_invites;
DROP TABLE recipient_attributes;
DROP TABLE recipients;
ALTER TABLE users DROP CONSTRAINT users_shop_fk;
DROP TABLE shops;
DROP TABLE users;
DROP TYPE substitution_status;
DROP TYPE gift_category;
DROP TYPE toxicity_level;
DROP TYPE scent_level;
DROP TYPE attr_confidence;
DROP TYPE attr_source;
DROP TYPE user_role;
