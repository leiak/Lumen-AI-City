#!/bin/sh
set -eu

: "${REDIS_ACL_PASSWORD_PROBE:?REDIS_ACL_PASSWORD_PROBE is required}"
: "${REDIS_ACL_PASSWORD_METRICS:?REDIS_ACL_PASSWORD_METRICS is required}"
: "${REDIS_ACL_PASSWORD_WORLD_ENGINE:?REDIS_ACL_PASSWORD_WORLD_ENGINE is required}"
: "${REDIS_ACL_PASSWORD_AGENT_OS:?REDIS_ACL_PASSWORD_AGENT_OS is required}"
: "${REDIS_ACL_PASSWORD_API_GATEWAY:?REDIS_ACL_PASSWORD_API_GATEWAY is required}"
: "${REDIS_ACL_PASSWORD_WS_GATEWAY:?REDIS_ACL_PASSWORD_WS_GATEWAY is required}"
: "${REDIS_ACL_PASSWORD_A2A_GATEWAY:?REDIS_ACL_PASSWORD_A2A_GATEWAY is required}"
: "${REDIS_ACL_PASSWORD_ECONOMY:?REDIS_ACL_PASSWORD_ECONOMY is required}"
: "${REDIS_MAXMEMORY:=512mb}"
: "${REDIS_MAXMEMORY_POLICY:=allkeys-lru}"

case "${REDIS_MAXMEMORY_POLICY}" in
  volatile-lru|allkeys-lru|volatile-lfu|allkeys-lfu|volatile-random|allkeys-random|volatile-ttl|noeviction) ;;
  *)
    echo "unsupported REDIS_MAXMEMORY_POLICY: ${REDIS_MAXMEMORY_POLICY}" >&2
    exit 2
    ;;
esac

mkdir -p /run/redis
cat > /run/redis/redis.conf <<EOF
port 6379
bind 0.0.0.0
protected-mode yes
dir /data
aclfile /run/redis/users.acl
loglevel notice
timeout 0
tcp-keepalive 300
maxclients 10000
maxmemory ${REDIS_MAXMEMORY}
maxmemory-policy ${REDIS_MAXMEMORY_POLICY}
save 900 1
save 300 10
save 60 10000
stop-writes-on-bgsave-error yes
rdbcompression yes
rdbchecksum yes
appendonly yes
appendfsync everysec
no-appendfsync-on-rewrite yes
auto-aof-rewrite-percentage 100
auto-aof-rewrite-min-size 64mb
slowlog-log-slower-than 10000
slowlog-max-len 256
latency-monitor-threshold 100
enable-debug-command no
enable-module-command no
EOF
cat > /run/redis/users.acl <<EOF
user default off
user probe on >${REDIS_ACL_PASSWORD_PROBE} &* +ping
user metrics on >${REDIS_ACL_PASSWORD_METRICS} &* +ping +info
user world_engine on >${REDIS_ACL_PASSWORD_WORLD_ENGINE} &aicity:* +ping +publish +subscribe
user agent_os on >${REDIS_ACL_PASSWORD_AGENT_OS} &aicity:* +ping +publish +subscribe
user api_gateway on >${REDIS_ACL_PASSWORD_API_GATEWAY} ~aicity:ratelimit:* &aicity:* +ping +incr +pexpire +eval +evalsha +script|load +publish +subscribe
user ws_gateway on >${REDIS_ACL_PASSWORD_WS_GATEWAY} &aicity:* +ping +subscribe
user a2a_gateway on >${REDIS_ACL_PASSWORD_A2A_GATEWAY} &aicity:* +ping +subscribe
user economy on >${REDIS_ACL_PASSWORD_ECONOMY} ~aicity:econ:* +ping +get +set
EOF
chmod 600 /run/redis/users.acl

exec redis-server /run/redis/redis.conf
