#!/bin/bash
# AI City - 证书自动轮换（到期前 7 天）
# 2.0 阶段 1: 跨城联邦 mTLS 基础设施
#
# 手动设置 cron（容器外宿主执行）：
#   0 3 * * * cd /d/work-ai/0401-town/ai-city && ./scripts/renew-certs.sh
#
# 用法: ./scripts/renew-certs.sh

set -euo pipefail
cd "$(dirname "$0")/.."
DAYS_WARN=7

for cert in certs/*.crt; do
    [[ "$cert" == *aicity-ca* ]] && continue
    service=$(basename "$cert" .crt)
    expiry=$(openssl x509 -in "$cert" -noout -enddate | cut -d= -f2)
    expiry_epoch=$(date -d "$expiry" +%s 2>/dev/null || echo 0)
    now_epoch=$(date +%s)
    days_left=$(( (expiry_epoch - now_epoch) / 86400 ))
    if [[ $days_left -lt $DAYS_WARN ]]; then
        echo "renewing $service (expires in $days_left days)"
        city=$(echo "$service" | grep -oE '(a|b)$' || echo "federation_hub")
        ./scripts/issue-cert.sh "$service" "$city"
    fi
done