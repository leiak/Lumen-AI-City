// Package middleware 提供 HTTP 中间件
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"encoding/base64"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

type rateLimiterStore interface {
	Hit(ctx context.Context, key string, ttl time.Duration) (int64, error)
}

type redisRateLimiterStore struct {
	client redis.UniversalClient
}

func (store *redisRateLimiterStore) Hit(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return redis.NewScript(`
		local count = redis.call('INCR', KEYS[1])
		if count == 1 then
			redis.call('PEXPIRE', KEYS[1], ARGV[1])
		end
		return count
	`).Run(ctx, store.client, []string{key}, ttl.Milliseconds()).Int64()
}

var rateLimitRejectedTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{Name: "aicity_rate_limit_rejected_total"},
	[]string{"scope"},
)

type rateLimitScope struct {
	prefix string
	limit  int64
}

var criticalMoneyPrefixes = []string{
	"/v1/wallet/purchase",
	"/v1/wallet/transfer",
	"/v1/marketplace/purchase",
	"/v1/marketplace/revenue-withdraw",
}

func rateLimitScopeFor(path string) rateLimitScope {
	switch {
	case path == "/v1/auth/login" || path == "/v1/auth/register":
		return rateLimitScope{prefix: "auth", limit: 10}
	case containsPrefix(criticalMoneyPrefixes, path):
		return rateLimitScope{prefix: "money", limit: 20}
	default:
		return rateLimitScope{prefix: "global", limit: 120}
	}
}

func containsPrefix(prefixes []string, path string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func rateLimitIdentity(token string) string {
	if token == "" {
		return ""
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if subject, ok := claims["sub"].(string); ok {
		return subject
	}
	return ""
}

// RateLimit uses route-specific fixed windows keyed by the JWT subject.
// An unverified JWT is parsed only to read its sub claim; authorization still
// happens in middleware.Auth. Never trust a client-provided identity header.
func RateLimit(redisURL string) gin.HandlerFunc {
	opt, _ := redis.ParseURL(redisURL)
	client := redis.NewClient(opt)
	return rateLimitHandler(&redisRateLimiterStore{client: client})
}

func rateLimitHandler(store rateLimiterStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		scope := rateLimitScopeFor(c.Request.URL.Path)
		identity := rateLimitIdentity(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
		if identity == "" {
			identity = c.ClientIP()
		}

		key := "rl:" + scope.prefix + ":" + identity
		count, err := store.Hit(ctx, key, time.Minute)
		if err != nil || count > scope.limit {
			if err != nil {
				c.Next()
				return
			}
			rateLimitRejectedTotal.WithLabelValues(scope.prefix).Inc()
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":  "rate_limit_exceeded",
				"limit":  scope.limit,
				"window": "60s",
				"scope":  scope.prefix,
			})
			return
		}
		c.Next()
	}
}
