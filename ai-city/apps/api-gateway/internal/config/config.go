// Package config 负责环境变量加载与校验
package config

import (
	"os"
)

type Config struct {
	Port        string
	RedisURL    string
	JWTSecret   string
	ServiceName string
	LogLevel    string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("API_GATEWAY_PORT", "8080"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret"),
		ServiceName: getEnv("SERVICE_NAME", "api-gateway"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
