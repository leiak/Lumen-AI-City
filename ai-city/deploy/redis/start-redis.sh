#!/bin/sh
set -eu

: "${REDIS_ACL_PASSWORD_PROBE:?REDIS_ACL_PASSWORD_PROBE is required}"
: "${REDIS_ACL_PASSWORD_WORLD_ENGINE:?REDIS_ACL_PASSWORD_WORLD_ENGINE is required}"
: "${REDIS_ACL_PASSWORD_AGENT_OS:?REDIS_ACL_PASSWORD_AGENT_OS is required}"
: "${REDIS_ACL_PASSWORD_API_GATEWAY:?REDIS_ACL_PASSWORD_API_GATEWAY is required}"
: "${REDIS_ACL_PASSWORD_WS_GATEWAY:?REDIS_ACL_PASSWORD_WS_GATEWAY is required}"
: "${REDIS_ACL_PASSWORD_A2A_GATEWAY:?REDIS_ACL_PASSWORD_A2A_GATEWAY is required}"
: "${REDIS_ACL_PASSWORD_ECONOMY:?REDIS_ACL_PASSWORD_ECONOMY is required}"

mkdir -p /run/redis
cat > /run/redis/users.acl <<EOF
user default off
user probe on >${REDIS_ACL_PASSWORD_PROBE} &* +ping
user world_engine on >${REDIS_ACL_PASSWORD_WORLD_ENGINE} &aicity:* +ping +publish +subscribe
user agent_os on >${REDIS_ACL_PASSWORD_AGENT_OS} &aicity:* +ping +publish +subscribe
user api_gateway on >${REDIS_ACL_PASSWORD_API_GATEWAY} ~aicity:ratelimit:* &aicity:* +ping +incr +pexpire +eval +evalsha +script|load +publish +subscribe
user ws_gateway on >${REDIS_ACL_PASSWORD_WS_GATEWAY} &aicity:* +ping +subscribe
user a2a_gateway on >${REDIS_ACL_PASSWORD_A2A_GATEWAY} &aicity:* +ping +subscribe
user economy on >${REDIS_ACL_PASSWORD_ECONOMY} ~aicity:econ:* +ping +get +set
EOF
chmod 600 /run/redis/users.acl

exec redis-server /usr/local/etc/redis/redis.conf
