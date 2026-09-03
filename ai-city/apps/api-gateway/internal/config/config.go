// Package config 负责环境变量加载与校验
package config

import (
	"os"
	"time"
)

type Config struct {
	Port        string
	RedisURL    string
	DatabaseURL string
	JWTSecret   string
	JWTExpiry   time.Duration
	ServiceName string
	LogLevel    string
	WorldURL    string
}

func Load() *Config {
	return &Config{
		Port:        getEnv("API_GATEWAY_PORT", "8080"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		DatabaseURL: getEnv("DATABASE_URL", "postgresql://aicity:aicity_dev@localhost:5432/aicity"),
		JWTSecret:   getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTExpiry:   time.Hour,
		ServiceName: getEnv("SERVICE_NAME", "api-gateway"),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		WorldURL:    getEnv("WORLD_ENGINE_URL", "http://localhost:50052"),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}