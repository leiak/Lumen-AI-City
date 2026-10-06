package httpgw

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aicity/a2a-gateway/internal/crosscity"
)

// newMirrorWithBeats 构造一个含 3 帧、MarkDone(true) 的 mirror 用于 replay 测试。
// 3 帧：sentence_idx 0/1/2；MarkDone 把 done 置 true 让测试能验证 Complete 字段。
func newMirrorWithBeats(t *testing.T) *crosscity.MirrorStore {
	t.Helper()
	m := crosscity.NewMirrorStore(5 * time.Minute)
	m.Create("sess-1", "npc_b_wu", "p1")
	if err := m.Append("sess-1", crosscity.Beat{SentenceIdx: 0, Text: "hi", Emotion: "happy", TSMS: 100}); err != nil {
		t.Fatal(err)
	}
	if err := m.Append("sess-1", crosscity.Beat{SentenceIdx: 1, Text: "there", Emotion: "neutral", TSMS: 200}); err != nil {
		t.Fatal(err)
	}
	if err := m.Append("sess-1", crosscity.Beat{SentenceIdx: 2, Text: "world", Emotion: "neutral", TSMS: 300}); err != nil {
		t.Fatal(err)
	}
	if err := m.MarkDone("sess-1", true); err != nil {
		t.Fatal(err)
	}
	return m
}

// 1. Happy path：from_idx 缺省 → 全部 3 帧。
func TestReplayReturnsAllBeatsFromZero(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type=%q, want application/json", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control=%q, want no-store", got)
	}

	var resp ReplayResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if resp.SessionID != "sess-1" {
		t.Fatalf("session_id=%q, want sess-1", resp.SessionID)
	}
	if !resp.Complete {
		t.Fatalf("complete=false, want true (MarkDone)")
	}
	if resp.FromIdx != 0 {
		t.Fatalf("from_idx=%d, want 0", resp.FromIdx)
	}
	if len(resp.Beats) != 3 {
		t.Fatalf("len(beats)=%d, want 3", len(resp.Beats))
	}
	if resp.Beats[0].Text != "hi" || resp.Beats[0].SentenceIdx != 0 || resp.Beats[0].Emotion != "happy" || resp.Beats[0].TSMS != 100 {
		t.Fatalf("beat[0]=%+v", resp.Beats[0])
	}
	if resp.Beats[2].Text != "world" || resp.Beats[2].SentenceIdx != 2 || resp.Beats[2].TSMS != 300 {
		t.Fatalf("beat[2]=%+v", resp.Beats[2])
	}
}

// 2. from_idx=1 → 截断到剩 2 帧。
func TestReplayWithFromIdx(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer?from_idx=1", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp ReplayResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if resp.FromIdx != 1 {
		t.Fatalf("from_idx=%d, want 1", resp.FromIdx)
	}
	if len(resp.Beats) != 2 {
		t.Fatalf("len(beats)=%d, want 2", len(resp.Beats))
	}
	if resp.Beats[0].SentenceIdx != 1 || resp.Beats[0].Text != "there" {
		t.Fatalf("first beat=%+v, want idx=1 text=there", resp.Beats[0])
	}
}

// 3. 不存在 session → 400 + R_015。
func TestReplaySessionNotFound(t *testing.T) {
	m := crosscity.NewMirrorStore(5 * time.Minute)
	h := &SessionReplayHandler{Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/missing/buffer", nil)
	req.SetPathValue("sid", "missing")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_015") {
		t.Fatalf("body=%s, want R_015", rec.Body.String())
	}
}

// 4. from_idx 非法（非数字）→ 400 + R_009。
func TestReplayInvalidFromIdx(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer?from_idx=abc", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for non-numeric from_idx; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_009") {
		t.Fatalf("body=%s, want R_009", rec.Body.String())
	}
}

// 5. from_idx 负数 → 400 + R_009（ParseUint 不接受负号）。
func TestReplayNegativeFromIdx(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer?from_idx=-1", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400 for negative from_idx; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_009") {
		t.Fatalf("body=%s, want R_009", rec.Body.String())
	}
}

// 6. Mirror=nil → 503 + R_016。
func TestReplayNilMirror(t *testing.T) {
	h := &SessionReplayHandler{Mirror: nil}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_016") {
		t.Fatalf("body=%s, want R_016", rec.Body.String())
	}
}

// 7. Bearer 鉴权：APIKey 非空 + 错误 Bearer → 401 + R_001。
func TestReplayRejectsWrongBearer(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{APIKey: "secret", Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer", nil)
	req.SetPathValue("sid", "sess-1")
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_001") {
		t.Fatalf("body=%s, want R_001", rec.Body.String())
	}
}

// 8. Bearer 鉴权：APIKey 非空 + 正确 Bearer → 200。
func TestReplayAcceptsCorrectBearer(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{APIKey: "secret", Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer", nil)
	req.SetPathValue("sid", "sess-1")
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// 9. Bearer 鉴权：APIKey 非空 + 缺失 Authorization → 401。
func TestReplayRejectsMissingBearer(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{APIKey: "secret", Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// 10. APIKey 空 + 任何 Bearer → 跳过鉴权到 handler body（dev mode）。
func TestReplaySkipsAuthWhenAPIKeyEmpty(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{APIKey: "", Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer", nil)
	req.SetPathValue("sid", "sess-1")
	req.Header.Set("Authorization", "Bearer whatever")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 in dev mode; body=%s", rec.Code, rec.Body.String())
	}
}

// 11. from_idx 越界（> 总帧数）→ 200 + 空 beats + done=true。
//     MirrorStore 行为：返空切片而非 ErrSessionNotFound，让客户端可放心"从最新拉"。
func TestReplayFromIdxOutOfRange(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer?from_idx=99", nil)
	req.SetPathValue("sid", "sess-1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp ReplayResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if len(resp.Beats) != 0 {
		t.Fatalf("len(beats)=%d, want 0", len(resp.Beats))
	}
	if !resp.Complete {
		t.Fatalf("complete=false, want true (session is MarkDone)")
	}
}

// 12. 手动 URL 切片 fallback（模拟 gin.WrapH 不写 PathValue 的场景）。
func TestReplayManualPathFallback(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	// 不调 SetPathValue，强制走 URL.Path 手动解析分支。
	req := httptest.NewRequest(http.MethodGet, "/v1/federation/sessions/sess-1/buffer?from_idx=2", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp ReplayResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
	if resp.SessionID != "sess-1" {
		t.Fatalf("session_id=%q, want sess-1 (from manual parse)", resp.SessionID)
	}
	if len(resp.Beats) != 1 || resp.Beats[0].SentenceIdx != 2 {
		t.Fatalf("beats=%+v, want 1 beat with idx=2", resp.Beats)
	}
}

// 13. 路径无法解析（path 不匹配预期模式）→ 400 + R_009。
func TestReplayBadPath(t *testing.T) {
	m := newMirrorWithBeats(t)
	h := &SessionReplayHandler{Mirror: m}

	// 完全无关的路径
	req := httptest.NewRequest(http.MethodGet, "/totally/unrelated", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_009") {
		t.Fatalf("body=%s, want R_009", rec.Body.String())
	}
}