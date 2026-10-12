#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"

ENV_FILE="${ENV_FILE:-.env.production}"
BACKUP_ROOT="${BACKUP_ROOT:-backups/production}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"

if [ ! -f "$ENV_FILE" ]; then
    echo "missing production env file: $ENV_FILE" >&2
    exit 1
fi

set -a
. "$ENV_FILE"
set +a

: "${POSTGRES_USER:?set POSTGRES_USER}"
: "${POSTGRES_PASSWORD:?set POSTGRES_PASSWORD}"
: "${POSTGRES_DB:?set POSTGRES_DB}"

mkdir -p "$BACKUP_ROOT"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
backup_file="$BACKUP_ROOT/aicity-$timestamp.dump"
tmp_file="$backup_file.partial"

compose() {
    docker compose --env-file "$ENV_FILE" \
        -f docker-compose.yml \
        -f docker-compose.prod.yml "$@"
}

compose config --quiet
echo "creating database backup: $backup_file"
compose exec -T postgres pg_dump \
    --username="$POSTGRES_USER" \
    --format=custom \
    --no-owner \
    "$POSTGRES_DB" > "$tmp_file"

if [ ! -s "$tmp_file" ]; then
    rm -f "$tmp_file"
    echo "pg_dump produced an empty file" >&2
    exit 1
fi

mv "$tmp_file" "$backup_file"
checksum_file="$backup_file.sha256"
sha256sum "$backup_file" | awk '{print $1 "  " FILENAME}' FILENAME="$(basename "$backup_file")" > "$checksum_file"
find "$BACKUP_ROOT" -name 'aicity-*.dump' -type f -mtime "+$RETENTION_DAYS" -delete
find "$BACKUP_ROOT" -name 'aicity-*.dump.sha256' -type f -mtime "+$RETENTION_DAYS" -delete

echo "backup complete"
echo "file: $backup_file"
echo "checksum: $(cat "$checksum_file")"
