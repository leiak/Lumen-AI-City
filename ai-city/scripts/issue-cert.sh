#!/usr/bin/env bash
# AI City - 基于 CA 签发服务证书 (含 SAN)
# 用法: ./scripts/issue-cert.sh <service-name> <city-or-role>
# 2.0 阶段 1: 跨城联邦 mTLS 基础设施
# 输出: certs/<service>.{key,csr,cnf,crt,fingerprint} (90 天)

set -euo pipefail

SERVICE="${1:?usage: issue-cert.sh <service-name> <city-or-role>}"
ROLE="${2:?usage: issue-cert.sh <service-name> <city-or-role>}"
CA_CN="aicity-ca"
DIR="certs"
DAYS=90

mkdir -p "$DIR"

# 生成 key + CSR 配置文件
openssl genrsa -out "$DIR/$SERVICE.key" 2048
cat > "$DIR/$SERVICE.cnf" <<EOF
[req]
distinguished_name = req_distinguished_name
req_extensions = v3_req
prompt = no
[req_distinguished_name]
CN = $SERVICE
[v3_req]
keyUsage = critical, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth, clientAuth
subjectAltName = @alt_names
[alt_names]
DNS.1 = $SERVICE
DNS.2 = localhost
DNS.3 = host.docker.internal
IP.1 = 127.0.0.1
EOF

openssl req -new -key "$DIR/$SERVICE.key" \
  -out "$DIR/$SERVICE.csr" -config "$DIR/$SERVICE.cnf"

# 用 CA 签名
openssl x509 -req -in "$DIR/$SERVICE.csr" \
  -CA "$DIR/$CA_CN.crt" -CAkey "$DIR/$CA_CN.key" -CAcreateserial \
  -out "$DIR/$SERVICE.crt" -days $DAYS \
  -extensions v3_req -extfile "$DIR/$SERVICE.cnf"

# 打印 SHA256 fingerprint (去冒号)
openssl x509 -in "$DIR/$SERVICE.crt" -noout -fingerprint -sha256 | \
  awk -F'=' '{print $2}' | tr -d ':' > "$DIR/$SERVICE.fingerprint"

echo "Cert: $DIR/$SERVICE.crt (valid $DAYS days)"
echo "Fingerprint: $(cat $DIR/$SERVICE.fingerprint)"
