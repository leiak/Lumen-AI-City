package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newCORSEngine 装一个只挂 CORS 的最小 engine（业务 handler 回 200）。
func newCORSEngine(origins []string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CORS(origins))
	r.POST("/v1/auth/login", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestCORS(t *testing.T) {
	allowed := []string{"http://localhost:3000"}

	tests := []struct {
		name            string
		method          string
		origin          string
		wantStatus      int
		wantAllowOrigin string // "" = 必须不存在
	}{
		{
			name:            "允许的 origin 发预检 → 204 且回显 origin",
			method:          http.MethodOptions,
			origin:          "http://localhost:3000",
			wantStatus:      http.StatusNoContent,
			wantAllowOrigin: "http://localhost:3000",
		},
		{
			name:            "允许的 origin 发真实请求 → 200 且带 CORS 头",
			method:          http.MethodPost,
			origin:          "http://localhost:3000",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "http://localhost:3000",
		},
		{
			name:            "未授权 origin 的预检 → 403 且不授予",
			method:          http.MethodOptions,
			origin:          "http://evil.example",
			wantStatus:      http.StatusForbidden,
			wantAllowOrigin: "",
		},
		{
			// 关键安全断言：绝不能无脑回显 Origin
			name:            "未授权 origin 的真实请求不得拿到 Allow-Origin",
			method:          http.MethodPost,
			origin:          "http://evil.example",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "",
		},
		{
			name:            "无 Origin（同源/curl）不受影响",
			method:          http.MethodPost,
			origin:          "",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newCORSEngine(allowed)
			req := httptest.NewRequest(tc.method, "/v1/auth/login", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tc.wantAllowOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantAllowOrigin)
			}
		})
	}
}

// 带 Origin 的响应必须有 Vary: Origin，否则共享缓存会把
// 某个源的响应（含/不含 CORS 头）错喂给另一个源。
func TestCORS_VaryOriginAlwaysSetWhenOriginPresent(t *testing.T) {
	for _, origin := range []string{"http://localhost:3000", "http://evil.example"} {
		r := newCORSEngine([]string{"http://localhost:3000"})
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if got := w.Header().Get("Vary"); got != "Origin" {
			t.Errorf("origin %s: Vary = %q, want %q", origin, got, "Origin")
		}
	}
}

// 预检必须放通 Authorization，否则所有鉴权路由在浏览器里都调不通。
func TestCORS_PreflightAllowsAuthorizationHeader(t *testing.T) {
	r := newCORSEngine([]string{"http://localhost:3000"})
	req := httptest.NewRequest(http.MethodOptions, "/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Errorf("Allow-Headers = %q, 必须含 Authorization", got)
	}
}
