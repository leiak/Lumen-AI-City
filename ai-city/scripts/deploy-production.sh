#!/bin/sh
set -eu

ENV_FILE="${ENV_FILE:-.env.production}"
export AICITY_ENV="${AICITY_ENV:-production}"

if [ "$AICITY_ENV" != "production" ]; then
    echo "AICITY_ENV must be production" >&2
    exit 1
fi

if [ ! -f "$ENV_FILE" ]; then
    echo "missing production env file: $ENV_FILE" >&2
    exit 1
fi

compose() {
    docker compose --env-file "$ENV_FILE" \
        -f docker-compose.yml \
        -f "${SCALE_OVERLAY:-docker-compose.scale.yml}" \
        -f docker-compose.prod.yml "$@"
}

compose config --quiet
compose up -d --build
compose ps
echo "production core deployed"
