package httpgw

import (
	"log"
	"net/http"
	"strings"

	"github.com/aicity/a2a-gateway/internal/a2asrv"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Server 是 httpgw 的对外入口：持有 *a2asrv.Service + apiKey + gin.Engine。
// 路由 / handler 方法见 router.go（构造函数 + 中间件）与 handlers.go。
type Server struct {
	svc    *a2asrv.Service
	apiKey string
	engine *gin.Engine
}

// New 构造 HTTP gateway Server。
//   - svc: 必须非 nil（共享 gRPC server 的 *a2asrv.Service）
//   - apiKey: 空字符串 = 关闭 Bearer 鉴权（开发态）；非空 = 强制要求 Authorization: Bearer <apiKey>
func New(svc *a2asrv.Service, apiKey string) *Server {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New() // 不用 Default()，我们自己挂中间件

	s := &Server{
		svc:    svc,
		apiKey: apiKey,
		engine: engine,
	}

	// 中间件链：trace_id → recovery → logging → auth（可选）
	engine.Use(s.traceIDMiddleware())
	engine.Use(s.recoveryMiddleware())
	engine.Use(s.loggingMiddleware())
	if apiKey != "" {
		engine.Use(s.authMiddleware())
	}

	// 路由（4 个）
	engine.GET("/v1/healthz", s.Healthz)
	engine.POST("/v1/cards", s.RegisterCard)
	engine.GET("/v1/discover", s.Discover)
	engine.POST("/v1/messages", s.SendMessage)

	return s
}

// Handler 返 http.Handler（兼容 httptest + main.go）。
func (s *Server) Handler() http.Handler {
	return s.engine
}

// Engine 直接返 gin.Engine（给 main.go / 调试用）。
func (s *Server) Engine() *gin.Engine {
	return s.engine
}

// traceIDMiddleware 注入 / 透传 X-Trace-Id（贯穿下游）。
func (s *Server) traceIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("X-Trace-Id")
		if traceID == "" {
			traceID = uuid.NewString()
		}
		c.Set(traceIDKey, traceID)
		c.Writer.Header().Set("X-Trace-Id", traceID)
		c.Next()
	}
}

// recoveryMiddleware panic 恢复 → 500 + envelope。
func (s *Server) recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[httpgw] PANIC path=%s err=%v trace=%s",
					c.Request.URL.Path, rec, c.GetString(traceIDKey))
				c.AbortWithStatusJSON(http.StatusInternalServerError, errorEnvelope(
					"F_INTERNAL", "panic recovered", c.GetString(traceIDKey)))
			}
		}()
		c.Next()
	}
}

// loggingMiddleware 打印 method/path/status/trace_id/IP。
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Printf("[httpgw] %s %s %d trace=%s ip=%s",
			c.Request.Method, c.Request.URL.Path, c.Writer.Status(),
			c.GetString(traceIDKey), c.ClientIP())
	}
}

// authMiddleware 简单 Bearer token 校验：
//   - 跳过 /v1/healthz（公开）
//   - 校验 Authorization: Bearer <apiKey>，错 → 401 unauthorized
func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/v1/healthz") {
			c.Next()
			return
		}
		authz := c.GetHeader("Authorization")
		token := strings.TrimPrefix(authz, "Bearer ")
		token = strings.TrimSpace(token)
		if token == "" || token != s.apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorEnvelope(
				"F_AUTH", "invalid or missing api key", c.GetString(traceIDKey)))
			return
		}
		c.Next()
	}
}
