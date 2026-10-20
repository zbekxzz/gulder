# API

Контракт — [`backend/api/openapi.yaml`](../backend/api/openapi.yaml), базовый путь `/api/v1`.
Ошибки: `{"error":{"code":"...","message":"...","details":{}}}`. Без `Authorization` → 401,
не та роль → 403, чужой ресурс → 404, эндпоинты релиза 2.0 → 501.

Разделы ниже дополняются по мере реализации вех.

## Аутентификация

`POST /auth/register` (только даритель), `POST /auth/login`, `POST /auth/refresh` → `{access_token, refresh_token, expires_in, user}`.
Access-токен передаётся в `Authorization: Bearer …` (по умолчанию живёт 15 минут), refresh — 30 дней.
Пароли хранятся как argon2id. Флористов и менеджеров заводит администратор; в dev их создаёт `make seed`
(пароль `gulder-dev-password`): `giver@`, `giver2@`, `florist@` (магазин «Абая»), `florist2@` («Достык»), `manager@gulder.local`.

## Профиль получателя

`PUT /recipients/{id}` принимает `{nickname?, attributes: [{key, value, source, confidence}]}` и обновляет
только перечисленные ключи. Правила:

- `confidence = unknown` → `value` не указывается («не знаю» ≠ «нет»). `know`/`assume` → значение обязательно.
- «Аллергий нет» — это `{"key":"allergies","value":[],"confidence":"know"}`.
- Даритель пишет только `source = giver`. Поля, заполненные получателем, в ответе приходят как
  `{"key":"allergies","hidden":true}` и не могут быть изменены дарителем (409).
- `GET /recipients/{id}` возвращает все 11 ключей; незаполненные — `{"key":…, "confidence":"unknown"}`.

| key | value |
|---|---|
| `allergies` | массив кодов: `genus:<род>`, `family:<семейство>`, `pollen`, `food:<группа>[:<подгруппа>]`, `fragrance`, `latex` |
| `asthma` | `true`/`false` |
| `pets` | массив видов латиницей: `cat`, `dog`, `horse`, `rabbit`, … |
| `scent_sensitivity` | `none`/`low`/`medium`/`strong` |
| `liked_flowers`, `disliked_flowers` | массив родов в нижнем регистре: `rosa`, `tulipa` |
| `liked_colors`, `disliked_colors` | массив цветов: `white`, `pink`, … |
| `style` | `classic`/`minimal`/`rustic`/`bright` |
| `composition_format` | `bouquet`/`box`/`basket`/`single` |
| `birth_date` | `YYYY-MM-DD` (только для нумерологии) |

## Импорт каталога флористом — `POST /catalog/import?kind=…`

`multipart/form-data`, поле `file` (CSV до 5 МБ, UTF-8, первая строка — заголовок, порядок колонок любой).
Импорт идёт в магазин флориста одной транзакцией; строки с ошибками пропускаются, остальные применяются.

Ответ: `{"imported": 2, "updated": 1, "errors": [{"line": 4, "field": "composition", "message": "…"}]}`
(`line` — номер строки файла, заголовок — строка 1; `field` пуст, если ошибка не в одном поле).

### kind=flowers
| Колонка | Обяз. | Описание |
|---|---|---|
| `code` | да | Код цветка, уникален. Коды общего справочника и других магазинов заняты |
| `trade_name` | да | Торговое название |
| `genus` | да | Род (латиница). **Неизвестный род → цветок создаётся неподтверждённым** (FR-C03) и блокируется, пока флорист не выберет таксон |
| `scientific_name` | | Уточняет таксон, если в справочнике несколько видов рода |
| `color` | да | Цвет латиницей: `white`, `pink`, … |
| `scent` | | `none`/`low`/`medium`/`strong`, по умолчанию `low` |
| `season_months` | | `1;2;3`, по умолчанию весь год |
| `care_notes`, `restrictions` | | Текст |
| `taxon_confirmed` | | `true` по умолчанию; `false` — оставить на подтверждение |

### kind=bouquets
| Колонка | Обяз. | Описание |
|---|---|---|
| `sku` | да | Артикул, уникален в магазине; существующий — обновляется вместе с составом |
| `name` | да | Название |
| `price` | да | Цена в тенге: `15000` или `15000.50` |
| `discount_price`, `discount_from`, `discount_until` | | Скидка: цена меньше `price`, `discount_until` обязателен; даты RFC3339 или `YYYY-MM-DD` |
| `composition` | да | `код_цветка:стебли;код_цветка:стебли` (коды общего справочника или своего магазина) |
| `packaging`, `style`, `composition_format`, `image_url` | | `style`: classic/minimal/rustic/bright; формат: bouquet/box/basket/single |
| `in_stock` | | `true` по умолчанию |

### kind=gifts
| Колонка | Обяз. | Описание |
|---|---|---|
| `sku`, `name` | да | |
| `category` | да | `card`, `sweets`, `soft_toy`, `fragrance`, `jewelry`, `candle`, `other` |
| `price` (+ скидка) | да | Как у букетов |
| `allergen_codes` | | **Пусто — состав неизвестен** (получателю с известными аллергиями такой подарок не предлагается); `-` — проверено, аллергенов нет; иначе коды через `;` |
| `composition` | | Текст состава |
| `scent` | | `none` по умолчанию; категория `fragrance` всегда считается `strong` |
| `pet_hazard` | | `true`/`false`; пусто — неизвестно |
| `in_stock` | | `true` по умолчанию |

## Прочие эндпоинты флориста

- `GET /catalog/bouquets`, `GET /catalog/gifts` — позиции своего магазина с наличием и текущей ценой.
- `GET /catalog/flowers?unconfirmed=true` — неподтверждённые растения; `PUT /catalog/flowers/{id}/taxon {taxon_id}` — подтвердить.
- `PUT /availability/{id}` — `{in_stock, quantity?, price?, discount_price?, discount_from?, discount_until?}`;
  новая цена добавляет запись в историю цен. Чужой магазин → 404.
