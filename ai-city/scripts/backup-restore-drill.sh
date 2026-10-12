#!/bin/sh
set -eu

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
CONTAINER="aicity-backup-drill-$$"
BACKUP_DIR="${BACKUP_DRILL_DIR:-$(mktemp -d)}"
BACKUP_FILE="$BACKUP_DIR/aicity-drill.dump"
CHECKSUM_FILE="$BACKUP_DIR/aicity-drill.dump.sha256"
IMAGE="${BACKUP_DRILL_IMAGE:-postgres:16-alpine}"
PGUSER="drill"
PGPASSWORD="drill_password"
PGDATABASE="drill"

cleanup() {
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
    if [ "${BACKUP_DRILL_KEEP:-0}" != "1" ]; then
        rm -f "$BACKUP_FILE" "$CHECKSUM_FILE"
        rmdir "$BACKUP_DIR" 2>/dev/null || true
    fi
}
trap cleanup EXIT INT TERM

echo "starting isolated backup/restore drill database"
docker run -d --name "$CONTAINER" \
    -e POSTGRES_USER="$PGUSER" \
    -e POSTGRES_PASSWORD="$PGPASSWORD" \
    -e POSTGRES_DB="$PGDATABASE" \
    "$IMAGE" >/dev/null

ready=false
for _ in $(seq 1 30); do
    if docker exec "$CONTAINER" pg_isready -U "$PGUSER" -d "$PGDATABASE" >/dev/null 2>&1; then
        ready=true
        break
    fi
    sleep 1
done
if [ "$ready" != true ]; then
    docker logs "$CONTAINER" >&2 || true
    echo "backup drill database failed to start" >&2
    exit 1
fi

echo "applying versioned migrations"
psql_exec() {
    docker exec -i "$CONTAINER" psql -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$PGDATABASE" "$@"
}

psql_exec <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
SQL

while IFS= read -r filename || [ -n "$filename" ]; do
    case "$filename" in
        ''|\#*) continue ;;
    esac
    migration="$ROOT/$filename"
    if [ ! -f "$migration" ]; then
        echo "migration missing: $filename" >&2
        exit 1
    fi
    checksum="$(sha256sum "$migration" | awk '{print $1}')"
    applied_checksum="$(psql_exec -Atqc "SELECT checksum FROM schema_migrations WHERE filename = '$filename'" </dev/null)"
    if [ -n "$applied_checksum" ]; then
        if [ "$applied_checksum" != "$checksum" ]; then
            echo "checksum changed for applied migration: $filename" >&2
            exit 1
        fi
        continue
    fi
    {
        echo 'BEGIN;'
        cat "$migration"
        echo 'COMMIT;'
    } | psql_exec >/dev/null
    psql_exec -c "INSERT INTO schema_migrations (filename, checksum) VALUES ('$filename', '$checksum')" </dev/null >/dev/null
done < "$ROOT/db/migrations/manifest.txt"

echo "creating drill marker row"
psql_exec <<'SQL'
CREATE TABLE backup_drill_audit (
    id BIGINT PRIMARY KEY,
    marker TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO backup_drill_audit (id, marker) VALUES (1, 'backup-restore-drill');
SQL

echo "creating custom-format backup"
docker exec "$CONTAINER" pg_dump \
    --username="$PGUSER" \
    --format=custom \
    --no-owner \
    "$PGDATABASE" > "$BACKUP_FILE"

if [ ! -s "$BACKUP_FILE" ]; then
    echo "pg_dump produced an empty file" >&2
    exit 1
fi
sha256sum "$BACKUP_FILE" | awk '{print $1 "  " FILENAME}' FILENAME="$(basename "$BACKUP_FILE")" > "$CHECKSUM_FILE"

echo "deleting marker data before restore"
docker exec "$CONTAINER" psql -v ON_ERROR_STOP=1 -U "$PGUSER" -d "$PGDATABASE" \
    -c 'DROP TABLE backup_drill_audit'

echo "restoring from backup"
docker exec -i "$CONTAINER" pg_restore \
    --username="$PGUSER" \
    --dbname="$PGDATABASE" \
    --clean \
    --if-exists \
    --no-owner \
    --exit-on-error \
    < "$BACKUP_FILE"

marker="$(docker exec "$CONTAINER" psql -v ON_ERROR_STOP=1 -Atqc \
    "SELECT marker FROM backup_drill_audit WHERE id = 1" -U "$PGUSER" -d "$PGDATABASE")"
if [ "$marker" != "backup-restore-drill" ]; then
    echo "restore verification failed: marker=$marker" >&2
    exit 1
fi

migration_count="$(docker exec "$CONTAINER" psql -v ON_ERROR_STOP=1 -Atqc \
    "SELECT COUNT(*) FROM schema_migrations" -U "$PGUSER" -d "$PGDATABASE")"
if [ "$migration_count" -le 0 ]; then
    echo "restore verification failed: schema_migrations is empty" >&2
    exit 1
fi

echo "backup/restore drill passed"
echo "migrations restored: $migration_count"
if [ "${BACKUP_DRILL_KEEP:-0}" = "1" ]; then
    echo "backup kept: $BACKUP_FILE"
fi
