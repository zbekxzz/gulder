#!/usr/bin/env bash
# Бэкап PostgreSQL (§18.6): pg_dump в custom-формате, ротация, копия вне хоста.
#
#   DATABASE_URL=...  BACKUP_DIR=/var/backups/gulder  BACKUP_KEEP_DAYS=14  BACKUP_REMOTE=s3:bucket/gulder  scripts/backup.sh
#
# Без DATABASE_URL дамп снимается из контейнера postgres прод-стека (docker compose).
# BACKUP_REMOTE — цель rclone (настраивается отдельно); без неё копия остаётся только на хосте — этого недостаточно.
set -euo pipefail

BACKUP_DIR="${BACKUP_DIR:-/var/backups/gulder}"
KEEP_DAYS="${BACKUP_KEEP_DAYS:-14}"
COMPOSE="${COMPOSE:-docker compose -f docker-compose.prod.yml}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
file="${BACKUP_DIR}/gulder-${stamp}.dump"

mkdir -p "${BACKUP_DIR}"
umask 077

if [[ -n "${DATABASE_URL:-}" ]]; then
  pg_dump --format=custom --no-owner --file="${file}" "${DATABASE_URL}"
else
  ${COMPOSE} exec -T postgres pg_dump -U gulder -d gulder --format=custom --no-owner > "${file}"
fi

# дамп должен читаться — иначе бэкап бесполезен
pg_restore --list "${file}" > /dev/null
sha256sum "${file}" > "${file}.sha256" 2>/dev/null || shasum -a 256 "${file}" > "${file}.sha256"
echo "backup: ${file} ($(du -h "${file}" | cut -f1))"

if [[ -n "${BACKUP_REMOTE:-}" ]]; then
  rclone copy "${file}" "${BACKUP_REMOTE}/" && rclone copy "${file}.sha256" "${BACKUP_REMOTE}/"
  echo "backup: copied to ${BACKUP_REMOTE}"
else
  echo "backup: WARNING BACKUP_REMOTE is not set — copy is stored on this host only" >&2
fi

find "${BACKUP_DIR}" -name 'gulder-*.dump*' -mtime +"${KEEP_DAYS}" -delete
