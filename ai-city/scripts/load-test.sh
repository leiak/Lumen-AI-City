#!/usr/bin/env bash
# AI City - 压测脚本（基于 k6）
# 用法: ./scripts/load-test.sh [scenario]

set -euo pipefail

SCENARIO="${1:-baseline}"

echo "==> 启动压测场景: $SCENARIO"

# 检查 k6
if ! command -v k6 >/dev/null; then
  echo "请先安装 k6: brew install k6"
  exit 1
fi

# 按场景运行
case "$SCENARIO" in
  baseline)
    echo "==> Baseline: 100 用户，5min"
    k6 run --vus 100 --duration 5m tests/load/baseline.js
    ;;
  lod)
    echo "==> LOD 24h 实测"
    k6 run --vus 50 --duration 24h tests/load/lod_24h.js
    ;;
  spike)
    echo "==> 突发流量：0→500 用户"
    k6 run --stages 30s:0,10s:500,1m:500,30s:0 tests/load/spike.js
    ;;
  *)
    echo "未知场景: $SCENARIO"
    echo "可选: baseline | lod | spike"
    exit 1
    ;;
esac
