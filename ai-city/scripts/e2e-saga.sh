#!/bin/bash
# Saga 端到端冒烟测试
# 用法: bash scripts/e2e-saga.sh

set -e

ORCHESTRATOR_URL=${ORCHESTRATOR_URL:-http://localhost:9100}
SAGA_ID=${SAGA_ID:-welcome_3npc}

echo "[e2e] trigger saga $SAGA_ID"
curl -s -X POST "$ORCHESTRATOR_URL/v1/saga/trigger" \
  -H "Content-Type: application/json" \
  -d "{\"saga_id\":\"$SAGA_ID\",\"player_id\":\"demo_a\"}" | jq .

echo "[e2e] wait 6s for saga execution"
sleep 6

echo "[e2e] check saga status"
STATUS=$(curl -s "$ORCHESTRATOR_URL/v1/saga/$SAGA_ID" | jq -r '.status')
echo "status = $STATUS"

if [ "$STATUS" = "SUCCESS" ]; then
  echo "[e2e] PASS"
  exit 0
else
  echo "[e2e] FAIL: expected SUCCESS, got $STATUS"
  exit 1
fi
