package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS 跨源资源共享。
//
// 为什么需要：web 在 :3000，api-gateway 在 :8080 —— 浏览器视为跨源。
// 没有 Access-Control-Allow-Origin 时浏览器会丢弃响应（curl 不受影响，
// 所以这个缺陷在命令行验收里完全看不出来）。
//
// 设计：
//   - 按 allowlist 回显 Origin，不用 "*"（"*" 与 credentials 互斥，
//     且将来上 cookie 会直接失效）
//   - 预检 OPTIONS 立即 204 短路，不落到业务路由（否则 404）
//   - 必须带 Vary: Origin，否则共享缓存会把某个源的响应喂给别的源
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		// 同源请求不带 Origin，无需任何 CORS 头
		if origin == "" {
			c.Next()
			return
		}

		// 无论是否命中 allowlist 都要发 Vary，避免缓存污染
		c.Writer.Header().Add("Vary", "Origin")

		if _, ok := allowed[origin]; !ok {
			// 不在 allowlist：不发 CORS 头，交由浏览器拦截。
			// 预检仍需短路，否则会 404 出一个误导性的错误。
			if c.Request.Method == http.MethodOptions {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Next()
			return
		}

		h := c.Writer.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Expose-Headers", "X-Trace-Id")

		if c.Request.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Trace-Id")
			h.Set("Access-Control-Max-Age", "600")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
