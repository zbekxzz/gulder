# Runbook

Эксплуатация Gulder на одном хосте (§18.6): Docker Compose + Caddy (TLS), PostgreSQL 16 в контейнере.

## Требования к хосту

- Linux с Docker Engine 24+ и плагином compose; открыты порты 80 и 443.
- DNS-запись `A`/`AAAA` для домена указывает на хост (Caddy получит сертификат Let's Encrypt сам).
- `pg_dump`/`pg_restore` 16 и `rclone` (для копии бэкапов вне хоста), cron.

## Первый запуск

```bash
git clone <repo> gulder && cd gulder
cp .env.example .env
# заполнить: DOMAIN, POSTGRES_PASSWORD, JWT_SECRET (openssl rand -base64 48), при желании ANTHROPIC_API_KEY
docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d postgres
docker compose -f docker-compose.prod.yml --profile migrate run --rm migrate      # миграции — отдельный шаг
docker compose -f docker-compose.prod.yml up -d backend web
docker compose -f docker-compose.prod.yml exec backend /app/seed -data /app/data  # справочник растений, каталог, нумерология
curl -fsS https://$DOMAIN/healthz
```

В `APP_ENV=prod` сид **не** создаёт dev-пользователей. Флористов и менеджеров создаёт администратор
(пока — SQL: `INSERT INTO users (email, role, shop_id, password_hash) …` с хэшем из `auth.HashPassword`; UI администрирования — в планах).

## Обновление версии

```bash
git pull && export VERSION=$(git describe --tags)
docker compose -f docker-compose.prod.yml build
scripts/backup.sh                                                                  # бэкап до миграций
docker compose -f docker-compose.prod.yml --profile migrate run --rm migrate
docker compose -f docker-compose.prod.yml up -d backend web
```

Откат: `docker compose … --profile migrate run --rm migrate /app/migrate down` (по одной миграции) и предыдущий `VERSION`.
Каждая миграция имеет рабочий `Down` — это проверяет CI.

## Бэкапы

Ежедневно в 03:15 (cron хоста):

```cron
15 3 * * * cd /opt/gulder && BACKUP_DIR=/var/backups/gulder BACKUP_REMOTE=s3:gulder-backups/prod scripts/backup.sh >> /var/log/gulder-backup.log 2>&1
```

`scripts/backup.sh`: `pg_dump -Fc`, проверка читаемости дампа (`pg_restore --list`), SHA-256, копия через `rclone`
(без `BACKUP_REMOTE` — предупреждение: копия только на хосте), ротация `BACKUP_KEEP_DAYS` (14 дней).

## Восстановление

```bash
createdb -h <host> -U <user> gulder_restore
TARGET_URL=postgres://<user>:<pass>@<host>/gulder_restore scripts/restore.sh /var/backups/gulder/gulder-<stamp>.dump
```

Скрипт проверяет контрольную сумму, отказывается восстанавливать в непустую базу и печатает число строк ключевых таблиц
и версию миграций. После проверки переключите `DATABASE_URL` (или переименуйте базы) и перезапустите `backend`.

### Журнал проверок восстановления

| Дата | Где | Дамп | Результат |
|---|---|---|---|
| 2026-10-20 | локально, PostgreSQL 16.10 | `gulder_dev` (88 КБ) → пустая `gulder_restore_check` | ✅ счётчики users/recipients/attributes/bouquets/runs/audit и `goose_version=5` совпали; повторный запуск в непустую базу отклонён |
| — | прод-хост | — | ⏳ выполнить после первого развёртывания и записать сюда |

## Наблюдаемость

- Логи: JSON (slog) в stdout контейнера, `request_id` в каждой записи; `docker compose logs backend`.
  В логи не пишутся аллергии, питомцы, даты рождения, геолокация и токены.
- `/healthz` — живость; `/readyz` — БД доступна и миграции применены.
- `/metrics` (Prometheus) — порт 9090 внутри сети compose, наружу не публикуется. Главное:
  - `rules_block_total{code}` — **резкое изменение = сломались данные или правила** (например, после импорта каталога);
  - `http_request_duration_seconds{route,status}` — латентность и ошибки по эндпоинтам;
  - `gift_sets_total{generator}` и `llm_errors_total` — доля LLM против fallback и сбои LLM.

## Частые ситуации

| Симптом | Что делать |
|---|---|
| `/readyz` → `migrations_pending` | Запустить шаг migrate этой версии |
| Рост `llm_errors_total`, все наборы `fallback` | Проверить `ANTHROPIC_API_KEY`, лимиты аккаунта; сервис работает и без LLM |
| Скачок `rules_block_total{code="UNVERIFIED_PLANT"}` | Флорист импортировал неизвестные роды — подтвердить растения на экране «Неподтверждённые» |
| 429 на `/auth/*` | Срабатывает лимит по IP (`RATE_LIMIT_AUTH`); при подборе пароля — так и задумано |
| Нужно убрать человека из исследовательских данных | Пользователь нажимает «Отозвать согласие» (или `SELECT research.revoke_consent(<user_id>)`) |
