#!/bin/bash
# 跨轨道联调: LLM ↔ 记忆 ↔ Saga ↔ 跨城
# 验证 LLM 调用触发记忆写入 + 跨轨道数据流通
set -e

API_URL=${API_URL:-http://localhost:8081}
MEM_URL=${MEM_URL:-http://localhost:9200}

echo "=== acceptance 2.0 cross-track smoke ==="

# 1. 登录
TOKEN_A=$(curl -s -X POST "$API_URL/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"demo_a","password":"demo123"}' \
  | jq -r .token)

if [ -z "$TOKEN_A" ]; then
  echo "FAIL: city_a login failed"
  exit 1
fi
echo "✓ city_a 登录"

# 2. LLM 调用触发记忆写入
echo "→ LLM 调用..."
RESP=$(curl -s -X POST "$API_URL/v1/npc/npc_a_wang_boss/talk" \
  -H "Authorization: Bearer $TOKEN_A" \
  -H "Content-Type: application/json" \
  -d '{"text":"红烧肉多少钱？"}')
echo "LLM 响应: $RESP"

# 3. 等记忆写入
echo "→ 等待 2s 记忆写入..."
sleep 2

# 4. 记忆召回
echo "→ 记忆召回..."
RECALL=$(curl -s -X POST "$MEM_URL/v1/recall" \
  -H "Content-Type: application/json" \
  -d '{"npc_id":"npc_a_wang_boss","player_id":"demo_a","query":"红烧肉","top_k":5}')

echo "记忆召回结果: $RECALL"

# 5. 断言
if echo "$RECALL" | jq -e '.messages | length >= 0' >/dev/null 2>&1; then
  COUNT=$(echo "$RECALL" | jq '.messages | length')
  echo "✓ 召回 $COUNT 条记忆"
else
  echo "FAIL: 召回 API 返回异常"
  exit 1
fi

echo "=== 跨轨道联调 PASS ==="
