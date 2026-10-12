#!/bin/sh
set -eu

usage() {
    echo "usage: $0 --backup-file FILE [--checksum-file FILE] [--yes]" >&2
}

BACKUP_FILE=""
CHECKSUM_FILE=""
CONFIRM=false

while [ "$#" -gt 0 ]; do
    case "$1" in
        --backup-file)
            BACKUP_FILE="${2:-}"
            shift 2
            ;;
        --checksum-file)
            CHECKSUM_FILE="${2:-}"
            shift 2
            ;;
        --yes)
            CONFIRM=true
            shift
            ;;
        *)
            usage
            exit 1
            ;;
    esac
done

if [ -z "$BACKUP_FILE" ] || [ ! -f "$BACKUP_FILE" ]; then
    usage
    exit 1
fi

ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$ROOT"
ENV_FILE="${ENV_FILE:-.env.production}"

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

compose() {
    docker compose --env-file "$ENV_FILE" \
        -f docker-compose.yml \
        -f docker-compose.prod.yml "$@"
}

if [ "$CONFIRM" != true ]; then
    echo "restore replaces the contents of database $POSTGRES_DB." >&2
    echo "re-run with --yes after stopping writes." >&2
    exit 1
fi

if [ -n "$CHECKSUM_FILE" ]; then
    if [ ! -f "$CHECKSUM_FILE" ]; then
        echo "checksum file not found: $CHECKSUM_FILE" >&2
        exit 1
    fi
    sha256sum --check "$CHECKSUM_FILE"
fi

container_backup="/tmp/aicity-restore.dump"
compose stop world-engine api-gateway economy-service admin-portal memory-service
compose cp "$BACKUP_FILE" "postgres:$container_backup"
compose exec -T postgres psql \
    --username="$POSTGRES_USER" \
    --dbname="$POSTGRES_DB" \
    --command="SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = current_database() AND pid <> pg_backend_pid()"
if ! compose exec -T postgres pg_restore \
    --username="$POSTGRES_USER" \
    --dbname="$POSTGRES_DB" \
    --clean \
    --if-exists \
    --no-owner \
    --exit-on-error \
    "$container_backup"; then
    compose exec -T postgres rm -f "$container_backup"
    echo "restore failed; inspect database state before retrying" >&2
    exit 1
fi

compose exec -T postgres rm -f "$container_backup"
compose start world-engine api-gateway economy-service admin-portal memory-service
echo "restore complete"
