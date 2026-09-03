package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TraceID 为每个请求注入 trace_id（贯穿所有下游服务）
func TraceID() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.NewString()
		}
		c.Set("trace_id", traceID)
		c.Writer.Header().Set("X-Trace-Id", traceID)
		c.Next()
	}
}

// Recovery panic 恢复
func Recovery(logger *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, recovered any) {
		logger.Error("panic recovered",
			zap.Any("error", recovered),
			zap.String("path", c.Request.URL.Path),
			zap.String("trace_id", c.GetString("trace_id")),
		)
		c.AbortWithStatusJSON(500, gin.H{"error": "internal_error"})
	})
}

// Logging 请求日志
func Logging(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		logger.Info("request",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.String("trace_id", c.GetString("trace_id")),
			zap.String("ip", c.ClientIP()),
		)
	}
}

// AntiScrap 反爬虫（基础 IP 频控 + UA 校验）
// 详细见 docs/04-API设计.md §18.15
func AntiScrap() gin.HandlerFunc {
	return func(c *gin.Context) {
		ua := c.GetHeader("User-Agent")
		if ua == "" {
			c.AbortWithStatusJSON(403, gin.H{"error": "missing_ua"})
			return
		}
		c.Next()
	}
}
