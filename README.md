# Gulder

Сервис, который помогает дарителю выбрать букет и подарки для конкретного человека с учётом
повода, бюджета, вкусов, аллергий, запахов, домашних животных, магазинов рядом и скидок —
и честно показывает, насколько он уверен и каких сведений не хватает.

Главное правило: система **никогда не выдаёт рекомендацию, нарушающую известное ограничение**,
а «не знаю» не считается «ограничения нет». Полное задание — [TASK.md](TASK.md), правила — [CLAUDE.md](CLAUDE.md),
ход работ — [PROGRESS.md](PROGRESS.md).

## Запуск за 3 команды

```bash
cp .env.example .env   # при желании впишите ANTHROPIC_API_KEY
make dev               # postgres + миграции + API (:8080) + фронтенд (:5173) в Docker
make seed              # загрузить демо-каталог из data/*.csv
```

Откройте http://localhost:5173.

Без Docker: поднимите PostgreSQL 16, задайте `DATABASE_URL`, затем `make dev-api` и `make dev-web`
в двух терминалах и `make seed`.

## Переменные окружения

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `APP_ENV` | `dev` | `dev` / `test` / `prod`. В `prod` обязателен `JWT_SECRET` |
| `DATABASE_URL` | — | Строка подключения к PostgreSQL 16 |
| `HTTP_ADDR` | `:8080` | Адрес API |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error`, логи в JSON |
| `JWT_SECRET` | dev-заглушка | Секрет подписи JWT, ≥ 32 байт |
| `ACCESS_TOKEN_TTL`, `REFRESH_TOKEN_TTL` | `15m`, `720h` | Время жизни токенов |
| `CORS_ORIGINS` | `http://localhost:5173` | Разрешённые origin через запятую |
| `ANTHROPIC_API_KEY` | пусто | Ключ LLM-ассистента; без него работает детерминированный fallback |
| `LLM_MODEL` | `claude-sonnet-5-5` | Модель ассистента |
| `LLM_TIMEOUT` | `20s` | Таймаут запроса к LLM |
| `DATA_DIR` | `../data` | Каталог CSV для сида |
| `RANKING_CONFIG` | встроенный | Путь к `ranking.yaml` с весами |
| `TEST_DATABASE_URL` | — | Для тестов: готовый PostgreSQL вместо testcontainers |
| `E2E_DATABASE_URL` / `E2E_BASE_URL` | — | Для Playwright: своя БД или уже поднятый стек |

## Команды

`make help` — полный список. Основные: `make test` (Go + vitest + Playwright), `make lint`,
`make vuln`, `make eval`, `make gen-api` (TS-типы из `backend/api/openapi.yaml`).

## Структура

```
backend/   Go 1.26: cmd/{api,migrate,seed,eval}, internal/{rules,ranking,assistant,offers,…}, migrations/
frontend/  React 18 + TypeScript + Vite, Tailwind, react-query, Leaflet
data/      исходные CSV (справочник растений, каталог, нумерология, экспертная разметка)
docs/      model card, data dictionary, API, walkthrough, runbook, ADR
```
