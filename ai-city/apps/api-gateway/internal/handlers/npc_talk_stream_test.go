package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aicity/api-gateway/internal/npc"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// newStreamTestHandler returns a fresh NPCTalkHandler wired with a fakeRedis
// and a no-op zap logger, so individual tests don't need to repeat boilerplate.
func newStreamTestHandler(t *testing.T) *NPCTalkHandler {
	t.Helper()
	return NewNPCTalkHandler(map[string]*npc.Tree{}, &fakeRedis{}, zap.NewNop(), "aicity:npc_dialogue")
}

// TestForwardCrossCityStream_NoNpcIDReturns400 — npcID 为空时立即 400 + R_009，
// 不向 a2a-gateway 发任何请求。这是 plan T07 Step 1 的失败测试初始版本。
func TestForwardCrossCityStream_NoNpcIDReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	r.POST("/v1/npc/:id/talk", func(c *gin.Context) {
		h.forwardCrossCityStream(c, "", nil)
	})

	body := `{"player_id":"u","choice_id":"c"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/npc//talk", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "R_009") {
		t.Fatalf("body = %s, want R_009", w.Body.String())
	}
}

// TestForwardCrossCityStream_NoSessionIDReturns400 — body 缺 session_id
// 时拒绝（a2a-gateway SayStreamHandler 同样要求 session_id；这里前置校验，
// 避免白跑 HTTP 调用）。
func TestForwardCrossCityStream_NoSessionIDReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	r.POST("/x", func(c *gin.Context) {
		h.forwardCrossCityStream(c, "npc_b_wu", []byte(`{"npc_id":"npc_b_wu"}`))
	})

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "R_009") {
		t.Fatalf("body = %s, want R_009", w.Body.String())
	}
}

// TestForwardCrossCityStream_ForwardsBearerAuth — 设了 A2A_HTTP_API_KEY 时，
// 转发的 POST 必须带 `Authorization: Bearer <key>`，且 body 透传原 bytes。
func TestForwardCrossCityStream_ForwardsBearerAuth(t *testing.T) {
	var gotAuth string
	var gotBody string
	fakeA2A := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: {\"type\":\"npc_say_stream\",\"text\":\"hi\"}\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer fakeA2A.Close()

	t.Setenv("A2A_HUB_URL", fakeA2A.URL)
	t.Setenv("A2A_HTTP_API_KEY", "secret")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	body := []byte(`{"npc_id":"npc_b_wu","session_id":"s1","player_id":"u","player_input":"hi"}`)
	r.POST("/x", func(c *gin.Context) {
		h.forwardCrossCityStream(c, "npc_b_wu", body)
	})

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if gotAuth != "Bearer secret" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer secret")
	}
	if !strings.Contains(gotBody, `"session_id":"s1"`) {
		t.Fatalf("forwarded body = %q, want contains session_id s1", gotBody)
	}
	if !strings.Contains(w.Body.String(), `data: {"type":"npc_say_stream","text":"hi"}`) {
		t.Fatalf("response = %q, want SSE frame", w.Body.String())
	}
}

// TestForwardCrossCityStream_PipesSSEResponse — a2a-gateway 返回的多个 SSE
// 帧（含空行分隔）必须按原样写入 web 客户端。
func TestForwardCrossCityStream_PipesSSEResponse(t *testing.T) {
	fakeA2A := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		for _, line := range []string{
			`data: {"type":"npc_say_stream","sentence_idx":0,"text":"您好！","emotion":"happy"}`,
			``,
			`data: {"type":"npc_say_stream","sentence_idx":1,"text":"需要什么帮助？","emotion":"neutral"}`,
			``,
			`data: {"type":"npc_say_stream_done","sentence_count":2,"complete":true}`,
			``,
			`data: {"type":"end"}`,
			``,
		} {
			_, _ = w.Write([]byte(line + "\n"))
			flusher.Flush()
		}
	}))
	defer fakeA2A.Close()

	t.Setenv("A2A_HUB_URL", fakeA2A.URL)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	body := []byte(`{"npc_id":"npc_b_wu","session_id":"s1","player_input":"hi"}`)
	r.POST("/x", func(c *gin.Context) {
		h.forwardCrossCityStream(c, "npc_b_wu", body)
	})

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	resp := w.Body.String()
	for _, want := range []string{
		`data: {"type":"npc_say_stream","sentence_idx":0,"text":"您好！","emotion":"happy"}`,
		`data: {"type":"npc_say_stream","sentence_idx":1,"text":"需要什么帮助？","emotion":"neutral"}`,
		`data: {"type":"npc_say_stream_done","sentence_count":2,"complete":true}`,
		`data: {"type":"end"}`,
	} {
		if !strings.Contains(resp, want) {
			t.Fatalf("missing frame %q in response: %s", want, resp)
		}
	}
	// SSE headers must be set on the response writer.
	if got := w.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", got)
	}
	if got := w.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want no", got)
	}
}

// TestForwardCrossCityStream_ForwardsA2AGateway502 — a2a-gateway 自身返回
// 502 + JSON error body 时，本层必须按原状态码 + body 透传给 web 客户端（不
// 二次包装为 SSE）。
func TestForwardCrossCityStream_ForwardsA2AGateway502(t *testing.T) {
	fakeA2A := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"R_016","message":"upstream gRPC dial fail"}`))
	}))
	defer fakeA2A.Close()

	t.Setenv("A2A_HUB_URL", fakeA2A.URL)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	body := []byte(`{"npc_id":"npc_b_wu","session_id":"s1","player_input":"hi"}`)
	r.POST("/x", func(c *gin.Context) {
		h.forwardCrossCityStream(c, "npc_b_wu", body)
	})

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "R_016") {
		t.Fatalf("body = %q, want contains R_016", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "upstream gRPC dial fail") {
		t.Fatalf("body = %q, want contains upstream message", w.Body.String())
	}
}

// TestForwardCrossCityStream_A2AGatewayUnreachable — a2a-gateway 域名不可达
// 时回 502 + R_016（F_016 a2a-gateway 不可达）。
func TestForwardCrossCityStream_A2AGatewayUnreachable(t *testing.T) {
	// Use an unroutable address (RFC 5737 documentation IP).
	t.Setenv("A2A_HUB_URL", "http://192.0.2.1:1")
	t.Setenv("A2A_HTTP_API_KEY", "")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	body := []byte(`{"npc_id":"npc_b_wu","session_id":"s1","player_input":"hi"}`)
	r.POST("/x", func(c *gin.Context) {
		h.forwardCrossCityStream(c, "npc_b_wu", body)
	})

	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", w.Code)
	}
	if !strings.Contains(w.Body.String(), "R_016") {
		t.Fatalf("body = %q, want R_016", w.Body.String())
	}
}

// TestHandle_ByID_CrossCity_NpcID — 通过 router 直发，验证 HandleByID 收到
// 含 `_b_` 的 npc_id 时真的走 forwardCrossCityStream（这里用一个会失败的
// fake upstream 触发 502 R_016）。
func TestHandle_ByID_CrossCity_NpcID(t *testing.T) {
	fakeA2A := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"R_016","message":"upstream"}`))
	}))
	defer fakeA2A.Close()

	t.Setenv("A2A_HUB_URL", fakeA2A.URL)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := newStreamTestHandler(t)
	r.POST("/v1/npc/:id/talk", h.HandleByID)

	body := bytes.NewBufferString(`{"npc_id":"npc_b_wu","session_id":"s1","player_id":"u","choice_id":"x","player_input":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/npc/npc_b_wu/talk", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 (cross-city forwarding); body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "R_016") {
		t.Fatalf("body = %q, want R_016", w.Body.String())
	}
}