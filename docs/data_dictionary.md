# Data dictionary

Описание исходных данных (`data/*.csv`) и, с вехи M6, полей исследовательского экспорта (RM-06).

## Исходные CSV (§12)

Общие соглашения: UTF-8, разделитель `,`, первая строка — заголовок. Списки внутри поля — через `;`.
Деньги — тенге, до 2 знаков после точки. Даты — RFC3339 или `YYYY-MM-DD`.

### plant_taxa.csv — справочник растений
| Поле | Тип | Описание |
|---|---|---|
| scientific_name | text | Научное название, уникально |
| genus, family | text | Род и семейство |
| tox_cat, tox_dog, tox_horse | none/mild/moderate/severe/unknown | Токсичность по ASPCA; пусто = unknown |
| pollen | none/low/medium/strong | Экспертная оценка пыльцы |
| allergen_codes | `;`-список | Дополнительные коды аллергенов (например `latex`) |
| aspca_url | url | Страница ASPCA, по которой сверена строка (M7) |
| gbif_key, image_url, image_license, image_author | | Таксономия и изображение GBIF; у изображения обязательна лицензия |
| verified | bool | Строка сверена с ASPCA. Сейчас все `false`: данные из Приложения B черновые |

### shops.csv — магазины (FR-C05)
`code` (ключ), `name`, `address`, `lat`, `lon`, `timezone` (IANA), `opening_hours` — JSON
`{"mon":[["09:00","21:00"]],"sun":[]}`; несколько интервалов в день, `24:00` — конец суток, нет дня = выходной.
Демо-данные: вымышленные магазины на реальных улицах Алматы.

### flowers.csv — цветы (FR-C01)
`code` (ключ), `trade_name`, `scientific_name` (для поиска таксона), `genus`, `color`, `scent` (none…strong),
`season_months` (`1;2;…`), `care_notes`, `restrictions`, `taxon_confirmed`.
Неизвестный род → `taxon_id = NULL`, `taxon_confirmed = false` → Rules Engine блокирует (FR-C03).

### bouquets.csv + bouquet_items.csv — букеты (FR-C02)
`shop_code`, `sku` (уникален в магазине), `name`, `packaging`, `style` (classic/minimal/rustic/bright),
`composition_format` (bouquet/box/basket/single), `price`, `discount_price`, `discount_from`, `discount_until`,
`in_stock`, `image_url`. Состав — `bouquet_items.csv`: `shop_code`, `sku`, `flower_code`, `stems`.

### gifts.csv — подарки (FR-C04)
`shop_code`, `sku`, `name`, `category` (card/sweets/soft_toy/fragrance/jewelry/candle/other), `composition`,
`allergen_codes` — **пусто = состав неизвестен**, `-` = проверено, аллергенов нет, иначе `;`-список кодов;
`scent`, `pet_hazard` (пусто = неизвестно), цены и скидки как у букетов, `in_stock`.

### numerology.csv — нумерология (FR-N03)
`number` (1–9, 11, 22, 33), `flowers` (роды, lowercase), `colors`, `meaning`, `note`.
Черновик, «подготовлено менеджером», импортируется как версия 1. Развлекательная функция.

### Скидки в демо-данных
Действующие: `2026-10-01 … 2027-06-30`, истёкшие: `2026-09-01 … 2026-09-30` (Asia/Almaty).
