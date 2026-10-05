#!/bin/bash
# scripts/check-cross-city.sh
# city_b 玩家 → city_a NPC 跨城连通测
set -e
echo "=== city_a → city_b 跨城连通测 ==="

# 1. 注册路由（手动 seed）
MSYS_NO_PATHCONV=1 docker compose exec -T postgres psql -U aicity -d aicity -c "
INSERT INTO a2a_npc_routes (npc_id, city, world_engine_addr, grpc_cert_fingerprint)
VALUES ('npc_a_grace_healer', 'city_a', 'world-engine-a:50051', '$(cat certs/world-engine-a.fingerprint 2>/dev/null || echo stub)')
ON CONFLICT (npc_id) DO UPDATE SET last_heartbeat_ms = EXTRACT(epoch FROM now())*1000;"

# 2. B 城玩家登录获取 token
TOKEN_B=$(curl -s -X POST http://localhost:8081/v1/auth/login \
  -d '{"username":"demo_b","password":"demo123"}' \
  -H "Content-Type: application/json" \
  | jq -r .token)

if [ -z "$TOKEN_B" ]; then
  echo "FAIL: B 城玩家登录失败"
  exit 1
fi

# 3. B 城玩家跨城调用 A 城 NPC
RESP=$(curl -s -X POST http://localhost:8081/v1/npc/npc_a_grace_healer/talk \
  -H "Authorization: Bearer $TOKEN_B" \
  -H "Content-Type: application/json" \
  -d '{"text":"医生，我失眠"}')

echo "跨城响应: $RESP"

# 4. 断言响应 text 非空
echo "$RESP" | jq -e '.text | length > 0' >/dev/null || (echo "FAIL: 跨城无响应"; exit 1)

echo "PASS"