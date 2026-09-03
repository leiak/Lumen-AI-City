#!/usr/bin/env bash
# AI City - 种子数据初始化
# 用法: ./scripts/seed-data.sh

set -euo pipefail

echo "==> 初始化种子数据"

# 1. 创建数据库 schema
echo "==> 应用 PG schema"
psql "${DATABASE_URL:-postgresql://aicity:aicity_dev@localhost:5432/aicity}" \
  -f packages/proto/pg-schema.sql 2>/dev/null || echo "   (schema 已存在或未定义，跳过)"

# 2. 加载 NPC 模板
echo "==> 加载 NPC 模板"
for f in packages/npc-templates/*.yaml; do
  echo "  - $f"
  # TODO: 调用 admin-portal API 或直接 INSERT
done

# 3. 加载剧本
echo "==> 加载剧本"
for f in packages/storyline-catalog/*.json; do
  echo "  - $f"
  # TODO: INSERT 到 saga_definitions 表
done

# 4. 注册默认 Agent 到 Neo4j
echo "==> 初始化 Neo4j 图数据"
# TODO: 通过 neo4j-sync 初始化节点和边

echo "==> 种子数据初始化完成"
