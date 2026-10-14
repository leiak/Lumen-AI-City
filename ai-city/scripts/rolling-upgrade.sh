#!/bin/sh
set -eu

REPLICAS="${REPLICAS:-2}"
EDGE_PORT="${EDGE_GATEWAY_HOST_PORT:-18090}"
COMPOSE_FILES="${COMPOSE_FILES:--f docker-compose.yml -f docker-compose.scale.yml}"
STATE_FILE="${ROLLOUT_STATE:-/tmp/aicity-rollout-images}"
SERVICE="${1:-}"
ACTION="${2:-apply}"

if [ "$ACTION" = "rollback" ]; then
    if [ -z "$SERVICE" ] || [ -z "${3:-}" ]; then
        echo "usage: $0 <service> rollback <image-id>" >&2
        exit 2
    fi
    PROJECT="${COMPOSE_PROJECT_NAME:-aitown}"
    IMAGE_NAME="$PROJECT-$SERVICE"
    IMAGE="$3"
    docker tag "$IMAGE" "$IMAGE_NAME:rollback"
    docker tag "$IMAGE_NAME:rollback" "$IMAGE_NAME"
    docker compose $COMPOSE_FILES up -d --no-deps --no-build --force-recreate --scale "$SERVICE=$REPLICAS" --wait
    echo "$SERVICE rolled back to image $IMAGE"
    exit 0
fi

case "$SERVICE" in
    api-gateway|web) ;;
    *) echo "usage: $0 <api-gateway|web> [apply]" >&2; exit 2 ;;
esac

compose() {
    docker compose $COMPOSE_FILES "$@"
}

service_container_names() {
    docker ps --filter "label=com.docker.compose.service=$SERVICE" \
        --format '{{.Names}}' | sort
}

compose config --quiet
compose up -d --build edge-gateway

current_image="$(docker inspect \
    --format '{{.Image}}' \
    "$(service_container_names | head -n 1)" 2>/dev/null || true)"
if [ -n "$current_image" ]; then
    printf '%s %s\n' "$SERVICE" "$current_image" >> "$STATE_FILE"
docker tag "$current_image" "${COMPOSE_PROJECT_NAME:-aitown}-$SERVICE:rollback"
fi

compose up -d --no-deps --scale "$SERVICE=$REPLICAS" --wait

for container in $(service_container_names); do
    echo "rolling replacement: $container"
    compose stop --timeout 30 "$container" >/dev/null 2>&1 || docker stop "$container"
    docker rm "$container"
    compose up -d --no-deps --scale "$SERVICE=$REPLICAS" --wait
done

ready=0
attempt=0
while [ "$attempt" -lt 30 ]; do
    if command -v curl >/dev/null 2>&1; then
        EDGE_HEALTH="$(curl -fsS "http://127.0.0.1:$EDGE_PORT/health" 2>/dev/null || true)"
    else
        EDGE_HEALTH="$(wget -qO- "http://127.0.0.1:$EDGE_PORT/health" 2>/dev/null || true)"
    fi
    if printf '%s' "$EDGE_HEALTH" | grep -q '"status":"ok"'; then
        ready=1
        break
    fi
    attempt=$((attempt + 1))
    sleep 2
done
if [ "$ready" -ne 1 ]; then
    echo "edge health check failed; roll back with the image ID from $STATE_FILE" >&2
    exit 1
fi

count="$(service_container_names | wc -l | tr -d ' ')"
echo "$SERVICE rolling upgrade complete: replicas=$count edge=http://127.0.0.1:$EDGE_PORT"
