#!/usr/bin/env bash
# AI City - Milvus 健康检查
# 用法: ./scripts/check-milvus.sh
# 2.0 阶段 1: 验证 Milvus 9091/health 端点返回 "OK"

set -euo pipefail

# Git Bash (MSYS) 会把 /app/x 解析成 C:/Program Files/Git/app/x，
# 关掉路径转换让 docker compose exec 看到的就是容器内路径。
MSYS_NO_PATHCONV=1 docker compose exec -T milvus curl -sf http://localhost:9091/health | grep -q "OK"
echo "milvus healthy"