#!/usr/bin/env bash
# Восстановление из дампа (§18.6) в ПУСТУЮ базу и сверка ключевых таблиц.
#
#   TARGET_URL=postgres://.../gulder_restore  scripts/restore.sh /var/backups/gulder/gulder-20261020T000000Z.dump
#
# Базу-цель создайте заранее (createdb). Скрипт не трогает непустую базу: защита от восстановления поверх живых данных.
set -euo pipefail

dump="${1:?usage: restore.sh <dump-file>}"
: "${TARGET_URL:?set TARGET_URL to an empty database}"

if [[ -f "${dump}.sha256" ]]; then
  (cd "$(dirname "${dump}")" && (sha256sum -c "$(basename "${dump}").sha256" 2>/dev/null || shasum -a 256 -c "$(basename "${dump}").sha256"))
fi

tables="$(psql "${TARGET_URL}" -tAc "SELECT count(*) FROM information_schema.tables WHERE table_schema IN ('public','research')")"
if [[ "${tables}" != "0" ]]; then
  echo "restore: target database is not empty (${tables} tables) — refusing" >&2
  exit 1
fi

pg_restore --no-owner --exit-on-error --dbname="${TARGET_URL}" "${dump}"

echo "restore: row counts"
psql "${TARGET_URL}" -tA -F ' ' -c "
  SELECT 'users', count(*) FROM users UNION ALL
  SELECT 'recipients', count(*) FROM recipients UNION ALL
  SELECT 'recipient_attributes', count(*) FROM recipient_attributes UNION ALL
  SELECT 'bouquets', count(*) FROM bouquets UNION ALL
  SELECT 'recommendation_runs', count(*) FROM recommendation_runs UNION ALL
  SELECT 'audit_events', count(*) FROM audit_events UNION ALL
  SELECT 'goose_version', max(version_id) FROM goose_db_version"
