#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$ROOT_DIR"

FORCE="${FORCE:-0}"
CITY_A_SERVICE="a2a-gateway-city-a"
CITY_B_SERVICE="a2a-gateway-city-b"
CA_SERVICE="aicity-ca"

ca_is_valid() {
  test -f "certs/$CA_SERVICE.crt" && test -f "certs/$CA_SERVICE.key" &&
    openssl x509 -in "certs/$CA_SERVICE.crt" -noout -checkend 86400 >/dev/null
}

service_is_valid() {
  test -f "certs/$1.crt" && test -f "certs/$1.key" &&
    openssl x509 -in "certs/$1.crt" -noout -checkend 86400 >/dev/null
}

if [[ "$FORCE" != "1" ]] && ca_is_valid; then
  echo "CA certificate is valid: certs/$CA_SERVICE.crt"
else
  ./scripts/gen-ca.sh "$CA_SERVICE"
fi

for service in "$CITY_A_SERVICE" "$CITY_B_SERVICE"; do
  if [[ "$FORCE" != "1" ]] && service_is_valid "$service"; then
    echo "Service certificate is valid: certs/$service.crt"
  else
    ./scripts/issue-cert.sh "$service" "cross-city"
  fi
done

for service in "$CITY_A_SERVICE" "$CITY_B_SERVICE"; do
  openssl verify -CAfile "certs/$CA_SERVICE.crt" "certs/$service.crt"
done
