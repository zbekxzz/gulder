# PROGRESS

Журнал вех (§17). Веха закрыта, когда `make test` и линтеры зелёные, запись здесь обновлена, стоит git-тег.

## M0. Каркас — закрыта (тег `m0`)

Сделано:
- Структура монорепозитория по §4; `TASK.md`, `CLAUDE.md` (выжимка §7 + §13).
- `backend`: Go-модуль, `cmd/{api,migrate,seed,eval}`, `/healthz`, `/readyz` (БД + миграции), JSON-логи slog
  с `request_id`, вырезание `lat`/`lon`/токенов из логируемого query, P1-эндпоинты → 501.
- Миграции goose: `00001_init.sql` = Приложение A (25 таблиц), `00002_catalog_codes.sql` (ADR 0004). Up/down/up проверено.
- Данные `data/`: справочник 16 родов (строго Приложение B, `verified=false`), 6 магазинов в Алматы,
  35 цветков (2 неопознанных для FR-C03), 43 букета, 32 подарка в 6 категориях, нумерология v1.
- `cmd/seed` идемпотентен и не затирает ручные правки (тест).
- `frontend`: Vite + React 18 + TS strict, Tailwind, ESLint + Prettier, vitest, Playwright smoke.
- `docker-compose.yml` (postgres, migrate, backend, frontend), `Makefile`, `.env.example`, CI, Dependabot.
- ADR 0001–0004.

Проверено локально: `go test -race`, golangci-lint (0 issues), vitest, `npm run build`, Playwright smoke
на локальном PostgreSQL 16.

Не проверено: `make dev` в Docker и CI на GitHub — на машине разработки нет Docker, у репозитория нет remote.

## Открытые вопросы
1. **Docker / CI.** Локально Docker не установлен, поэтому `docker compose` и GitHub Actions не запускались.
   Нужно запушить репозиторий на GitHub и убедиться, что CI зелёный.
2. **Эталонная реализация Rules Engine на Python** (§7) в репозитории отсутствует — порт сделан по спецификации §7.
3. **Справочник растений** — черновой (Приложение B, «по памяти»). Сверка с ASPCA — веха M7.
4. **Каталог** — демо-данные (вымышленные магазины на реальных улицах Алматы, правдоподобные цены).
   Реальный каталог — веха M7.
5. **Go 1.26**: свежие версии pgx/goose/testcontainers требуют Go ≥ 1.24; модуль на `go 1.26` (в задании «1.22+»).
