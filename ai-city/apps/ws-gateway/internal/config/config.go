// Package config 环境变量加载（mirror apps/api-gateway/internal/config）。
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port     string
	RedisURL string
	// ChannelMoved 是 world-engine publish 的频道（与 REDIS_CHANNEL_MOVED 一致）
	ChannelMoved string
	// JWTSecret 必须与 api-gateway 的 JWT_SECRET 相同，否则签发的 token 验不过
	JWTSecret string
	// AllowAnon 为 true 时 ?token= 缺失也放行（仅 dev/调试）
	AllowAnon bool
	// VerifyOrigin 为 true 时 WS 升级按 CORSAllowedOrigins 校验 Origin。
	// dev 默认 false（Sprint 9 现状）；生产必须置 true，否则任何站点的
	// 页面都能建立 WS 连接（token 仍需合法，但可被 CSRF 式滥用）。
	VerifyOrigin bool
	// SendBuffer 是单个客户端写队列长度；满了即判定慢消费者并断开
	SendBuffer int
	// PingInterval WS 心跳间隔；0 = 关闭心跳
	PingInterval       time.Duration
	LogLevel           string
	ServiceName        string
	CORSAllowedOrigins []string
}

func Load() *Config {
	return &Config{
		Port:         getEnv("WS_GATEWAY_PORT", "8082"),
		RedisURL:     getEnv("REDIS_URL", "redis://localhost:6379/0"),
		ChannelMoved: getEnv("REDIS_CHANNEL_MOVED", "aicity:player:moved"),
		JWTSecret:    getEnv("JWT_SECRET", "dev-secret-change-me"),
		AllowAnon:    getEnv("WS_ALLOW_ANON", "false") == "true",
		VerifyOrigin: getEnv("WS_VERIFY_ORIGIN", "false") == "true",
		SendBuffer:   getEnvInt("WS_SEND_BUFFER", 16),
		PingInterval: time.Duration(getEnvInt("WS_PING_INTERVAL_SEC", 30)) * time.Second,
		LogLevel:     getEnv("LOG_LEVEL", "info"),
		ServiceName:  getEnv("SERVICE_NAME", "ws-gateway"),
		CORSAllowedOrigins: strings.Split(
			getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000"), ","),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}
