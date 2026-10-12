#!/bin/sh
set -eu

ROOT="${MIGRATIONS_ROOT:-$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)}"
MANIFEST="${MIGRATIONS_MANIFEST:-$ROOT/db/migrations/manifest.txt}"
PGHOST="${PGHOST:-postgres}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-${POSTGRES_USER:-aicity}}"
PGPASSWORD="${PGPASSWORD:-${POSTGRES_PASSWORD:-aicity_dev}}"
PGDATABASE="${PGDATABASE:-${POSTGRES_DB:-aicity}}"
PGSSLMODE="${PGSSLMODE:-prefer}"
export PGHOST PGPORT PGUSER PGPASSWORD PGDATABASE PGSSLMODE

psql -v ON_ERROR_STOP=1 -c "CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    checksum TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)"

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
    applied_checksum="$(psql -v ON_ERROR_STOP=1 -Atqc \
        "SELECT checksum FROM schema_migrations WHERE filename = '$filename'")"

    if [ -n "$applied_checksum" ]; then
        if [ "$applied_checksum" != "$checksum" ]; then
            echo "checksum changed for applied migration: $filename" >&2
            exit 1
        fi
        echo "migration already applied: $filename"
        continue
    fi

    echo "applying migration: $filename"
    psql -v ON_ERROR_STOP=1 --single-transaction --file="$migration"
    psql -v ON_ERROR_STOP=1 -c \
        "INSERT INTO schema_migrations (filename, checksum) VALUES ('$filename', '$checksum')"
done < "$MANIFEST"

echo "migrations complete"
