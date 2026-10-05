#!/usr/bin/env bash
# AI City - 生成 mTLS CA 根证书
# 用法: ./scripts/gen-ca.sh [ca-cn]
# 2.0 阶段 1: 跨城联邦 mTLS 基础设施
# 默认 CN=aicity-ca，输出到 certs/<cn>.crt 与 certs/<cn>.key (365 天)

set -euo pipefail

CN="${1:-aicity-ca}"
DIR="certs"
mkdir -p "$DIR"

openssl genrsa -out "$DIR/$CN.key" 2048
# MSYS_NO_PATHCONV=1: Git Bash 不会把 /CN=... 误转成 C:/Program Files/Git/CN=...
MSYS_NO_PATHCONV=1 openssl req -new -x509 -days 365 -key "$DIR/$CN.key" \
  -out "$DIR/$CN.crt" -subj "/CN=$CN/O=AI-City/C=CN"

echo "CA cert: $DIR/$CN.crt"
echo "CA key:  $DIR/$CN.key"
