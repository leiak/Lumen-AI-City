#!/bin/bash
# Saga 失败注入测试（强制 worker_c 失败）
set -e

ORCHESTRATOR_URL=${ORCHESTRATOR_URL:-http://localhost:9100}
SAGA_ID=${SAGA_ID:-welcome_3npc_fail}

echo "[e2e-fail] trigger saga $SAGA_ID with force_fail=worker_c"
curl -s -X POST "$ORCHESTRATOR_URL/v1/saga/trigger" \
  -H "Content-Type: application/json" \
  -d "{\"saga_id\":\"$SAGA_ID\",\"player_id\":\"demo_a\",\"force_fail\":\"worker_c\"}" | jq .

echo "[e2e-fail] wait 6s"
sleep 6

echo "[e2e-fail] check saga status"
STATUS=$(curl -s "$ORCHESTRATOR_URL/v1/saga/$SAGA_ID" | jq -r '.status')
echo "status = $STATUS"

echo "[e2e-fail] check compensation metric"
METRIC=$(curl -s "$ORCHESTRATOR_URL/metrics" | grep saga_compensation_total || echo "no metric")
echo "$METRIC"

if [ "$STATUS" = "COMPENSATED" ]; then
  echo "[e2e-fail] PASS"
  exit 0
else
  echo "[e2e-fail] FAIL: expected COMPENSATED, got $STATUS"
  exit 1
fi
