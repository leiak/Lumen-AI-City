// Package cors 是 api-gateway/internal/middleware/cors.go 的 net/http 版本。
//
// 语义逐条对齐（差别只在 gin.HandlerFunc → http.Handler）：
//   - 按 allowlist 回显 Origin，绝不用 "*"（与 credentials 互斥）
//   - 带 Origin 的响应恒发 Vary: Origin，否则共享缓存会串源
//   - 预检 OPTIONS 短路：命中 allowlist → 204；未命中 → 403（而非落到 404）
//
// 注意：这里管的是普通 HTTP（/healthz、以及 WS handshake 的预检）。
// WebSocket upgrade 本身的 Origin 校验由 websocket.AcceptOptions 负责，
// 两者互不替代。
package cors

import (
	"net/http"
	"net/url"
	"strings"
)

// HostPatterns 把 origin allowlist（`http://localhost:3000`）转成
// websocket.AcceptOptions.OriginPatterns 要的形式（`localhost:3000`）。
//
// 必须转：nhooyr 的 authenticateOrigin 拿 `url.Parse(origin).Host` 去匹配
// pattern（accept.go:222），带 scheme 的 pattern 永远匹配不上，
// 于是所有跨源 WS 都会被拒 —— 且症状是 403 而不是配置报错。
func HostPatterns(allowedOrigins []string) []string {
	out := make([]string, 0, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if u, err := url.Parse(o); err == nil && u.Host != "" {
			out = append(out, u.Host)
			continue
		}
		// 已经是裸 host[:port]（url.Parse 对无 scheme 的串给不出 Host）
		out = append(out, o)
	}
	return out
}

// Middleware 返回一个包装 next 的 CORS handler。
func Middleware(allowedOrigins []string, next http.Handler) http.Handler {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		if o = strings.TrimSpace(o); o != "" {
			allowed[o] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// 同源请求不带 Origin，无需任何 CORS 头
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		h := w.Header()
		// 无论是否命中 allowlist 都要发 Vary，避免缓存污染
		h.Add("Vary", "Origin")

		if _, ok := allowed[origin]; !ok {
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Expose-Headers", "X-Trace-Id")

		if r.Method == http.MethodOptions {
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Trace-Id")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
