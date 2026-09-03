// Package middleware 提供 HTTP 中间件
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit 限流中间件（令牌桶 / Redis 滑动窗口）
// 详细设计见 docs/04-API设计.md §18.7 + docs/09-架构优化v2.md §34
func RateLimit(redisURL string) gin.HandlerFunc {
	opt, _ := redis.ParseURL(redisURL)
	rdb := redis.NewClient(opt)
	ctx := context.Background()

	return func(c *gin.Context) {
		// 限流键：玩家 ID 或 IP
		key := c.ClientIP()
		if uid := c.GetHeader("X-Player-ID"); uid != "" {
			key = "rl:player:" + uid
		}

		// 滑动窗口：每分钟 60 次
		count, err := rdb.Incr(ctx, "rl:"+key).Result()
		if err == nil && count == 1 {
			rdb.Expire(ctx, "rl:"+key, 60*time.Second)
		}
		if err == nil && count > 60 {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate_limit_exceeded",
				"limit": 60,
				"window": "60s",
			})
			return
		}

		c.Next()
	}
}
