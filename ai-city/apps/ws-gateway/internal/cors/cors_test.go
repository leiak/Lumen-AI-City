package cors

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newHandler(origins []string) http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	return Middleware(origins, next)
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
			method:          http.MethodGet,
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
			method:          http.MethodGet,
			origin:          "http://evil.example",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "",
		},
		{
			name:            "无 Origin（同源/curl）不受影响",
			method:          http.MethodGet,
			origin:          "",
			wantStatus:      http.StatusOK,
			wantAllowOrigin: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(allowed)
			req := httptest.NewRequest(tc.method, "/healthz", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tc.wantAllowOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantAllowOrigin)
			}
		})
	}
}

// 带 Origin 的响应必须有 Vary: Origin，否则共享缓存会串源。
func TestCORS_VaryOriginAlwaysSetWhenOriginPresent(t *testing.T) {
	for _, origin := range []string{"http://localhost:3000", "http://evil.example"} {
		h := newHandler([]string{"http://localhost:3000"})
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if got := w.Header().Get("Vary"); got != "Origin" {
			t.Errorf("origin %s: Vary = %q, want %q", origin, got, "Origin")
		}
	}
}

// 预检必须放通 Authorization，且 Max-Age=600。
func TestCORS_PreflightHeaders(t *testing.T) {
	h := newHandler([]string{"http://localhost:3000"})
	req := httptest.NewRequest(http.MethodOptions, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Errorf("Allow-Headers = %q, 必须含 Authorization", got)
	}
	if got := w.Header().Get("Access-Control-Max-Age"); got != "600" {
		t.Errorf("Max-Age = %q, want 600", got)
	}
}

// HostPatterns 必须剥掉 scheme：nhooyr 用 url.Parse(origin).Host 匹配 pattern，
// 带 scheme 的 pattern 永远匹配不上（症状是 403，不是配置报错）。
func TestHostPatterns(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "剥掉 http/https scheme",
			in:   []string{"http://localhost:3000", "https://city.example.com"},
			want: []string{"localhost:3000", "city.example.com"},
		},
		{
			name: "裸 host 原样保留",
			in:   []string{"localhost:3000"},
			want: []string{"localhost:3000"},
		},
		{
			name: "空串 / 空白项丢掉",
			in:   []string{"http://localhost:3000", "", "   "},
			want: []string{"localhost:3000"},
		},
		{
			name: "通配 pattern 原样传给 nhooyr",
			in:   []string{"*.example.com"},
			want: []string{"*.example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HostPatterns(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("HostPatterns(%q) = %q, want %q", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("HostPatterns(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// allowlist 里的空串 / 空白项必须被丢掉，否则 CORS_ALLOWED_ORIGINS 末尾多个
// 逗号就会放通一个空 Origin 分支。
func TestCORS_BlankOriginsIgnored(t *testing.T) {
	h := newHandler([]string{"http://localhost:3000", "", "  "})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", " ")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty", got)
	}
}
