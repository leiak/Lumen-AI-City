package httpgw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// FakeSayStreamClient is a SayStreamClient stub used by all tests in this file.
// It can either fail immediately (Err != nil) or pre-load a sequence of beats
// that are streamed through a goroutine respecting ctx cancellation.
type FakeSayStreamClient struct {
	Beats []*a2av1.SayBeat
	Err   error

	calls atomic.Int32
}

func (f *FakeSayStreamClient) SayStreamForward(
	ctx context.Context, _ *a2av1.SayRequestInit,
) (<-chan *a2av1.SayBeat, <-chan error, error) {
	f.calls.Add(1)
	if f.Err != nil {
		return nil, nil, f.Err
	}
	bch := make(chan *a2av1.SayBeat)
	ech := make(chan error, 1)
	go func() {
		defer close(bch)
		for _, b := range f.Beats {
			select {
			case <-ctx.Done():
				close(ech)
				return
			case bch <- b:
			}
		}
		if f.Err != nil {
			ech <- f.Err
		}
		close(ech)
	}()
	return bch, ech, nil
}

func strP(s string) *string { return &s }
func boolP(b bool) *bool    { return &b }

// --- Tests ---

// 1. Auth: missing API key set + wrong Bearer → 401.
func TestSayStreamRejectsUnauthorized(t *testing.T) {
	h := &SayStreamHandler{APIKey: "secret", Client: &FakeSayStreamClient{}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// 2. Auth: missing API key set + correct Bearer → reaches handler body (200
//    with no beats because the fake is empty but valid input parses OK).
func TestSayStreamAcceptsCorrectBearer(t *testing.T) {
	fc := &FakeSayStreamClient{}
	h := &SayStreamHandler{APIKey: "secret", Client: fc}
	body := `{"npc_id":"npc_b","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"type":"end"`) {
		t.Fatalf("expected end frame, got %s", rec.Body.String())
	}
	if fc.calls.Load() != 1 {
		t.Fatalf("client called %d times, want 1", fc.calls.Load())
	}
}

// 3. Validation: missing npc_id → 400 + R_009.
func TestSayStreamRejectsMissingNpcID(t *testing.T) {
	h := &SayStreamHandler{APIKey: "", Client: &FakeSayStreamClient{}}
	body := `{"session_id":"s1","player_input":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_009") {
		t.Fatalf("body=%s, want R_009", rec.Body.String())
	}
}

// 4. Validation: missing session_id → 400 + R_009 (T04 contract requires both).
func TestSayStreamRejectsMissingSessionID(t *testing.T) {
	h := &SayStreamHandler{APIKey: "", Client: &FakeSayStreamClient{}}
	body := `{"npc_id":"npc_b","player_input":"hi"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_009") {
		t.Fatalf("body=%s, want R_009", rec.Body.String())
	}
}

// 5. Validation: malformed JSON → 400 + R_009.
func TestSayStreamRejectsInvalidJSON(t *testing.T) {
	h := &SayStreamHandler{APIKey: "", Client: &FakeSayStreamClient{}}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_009") {
		t.Fatalf("body=%s, want R_009", rec.Body.String())
	}
}

// 6. Happy path: 2 beats → 2 SSE frames + final end frame.
func TestSayStreamStreamsBeatsAsSSE(t *testing.T) {
	idx := int32(0)
	complete := true
	beats := []*a2av1.SayBeat{
		{Type: "npc_say_stream", SessionId: "s1", NpcId: "npc_b_wu",
			SentenceIdx: &idx, Text: strP("hi"), Emotion: strP("happy"),
			TsMs: 1000, TraceId: "tr-1"},
		{Type: "npc_say_stream_done", SessionId: "s1", NpcId: "npc_b_wu",
			SentenceCount: &idx, Complete: &complete, TsMs: 1001, TraceId: "tr-1"},
	}
	h := &SayStreamHandler{APIKey: "", Client: &FakeSayStreamClient{Beats: beats}}
	body := `{"npc_id":"npc_b_wu","session_id":"s1","player_input":"hi","trace_id":"tr-1"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type=%q, want text/event-stream", ct)
	}

	out := rec.Body.String()
	// Each SSE frame is `data: <json>\n\n`. We expect 2 beat frames + 1 end frame.
	frameCount := strings.Count(out, "data: {")
	if frameCount != 3 {
		t.Fatalf("frame count=%d, want 3; body=%s", frameCount, out)
	}
	if !strings.Contains(out, `"type":"npc_say_stream"`) {
		t.Fatalf("missing npc_say_stream frame; body=%s", out)
	}
	if !strings.Contains(out, `"type":"npc_say_stream_done"`) {
		t.Fatalf("missing npc_say_stream_done frame; body=%s", out)
	}
	if !strings.Contains(out, `"type":"end"`) {
		t.Fatalf("missing end sentinel; body=%s", out)
	}
	if !strings.Contains(out, `"emotion":"happy"`) {
		t.Fatalf("missing emotion field; body=%s", out)
	}

	// Each non-empty `data:` line must be valid JSON.
	for _, line := range strings.Split(out, "\n") {
		const pfx = "data: "
		if !strings.HasPrefix(line, pfx) {
			continue
		}
		payload := strings.TrimPrefix(line, pfx)
		if payload == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			t.Fatalf("invalid JSON frame %q: %v", payload, err)
		}
	}
}

// 7. Disconnect: client cancels ctx before handler reads beats → handler
//    returns cleanly without panicking; the FakeSayStreamClient goroutine
//    also exits because it watches ctx.Done().
func TestSayStreamHandlesClientDisconnect(t *testing.T) {
	idx := int32(0)
	complete := true
	beats := []*a2av1.SayBeat{
		{Type: "npc_say_stream_done", SentenceCount: &idx, Complete: &complete},
	}
	fc := &FakeSayStreamClient{Beats: beats}
	h := &SayStreamHandler{APIKey: "", Client: fc}

	ctx, cancel := context.WithCancel(context.Background())
	body := `{"npc_id":"npc_b","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()

	cancel() // simulate client disconnect before handler even dispatches
	h.ServeHTTP(rec, req)

	// No assertions on body (handler may return before flushing). We verify
	// only that ServeHTTP returns without panicking and does not produce a
	// 5xx that would suggest a programming error. 200 with possibly empty
	// body is acceptable.
	if rec.Code >= 500 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// 8. Upstream error at dial time → 502 + R_016.
func TestSayStreamErrorsOnClientError(t *testing.T) {
	h := &SayStreamHandler{
		APIKey: "",
		Client: &FakeSayStreamClient{Err: errors.New("upstream fail")},
	}
	body := `{"npc_id":"npc_b","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status=%d, want 502; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "R_016") {
		t.Fatalf("body=%s, want R_016", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "upstream fail") {
		t.Fatalf("body should include upstream error message; got %s", rec.Body.String())
	}
}

// 9. SSE frame formatting: every frame is `data: {json}\n\n` (double newline
//    terminator, per SSE spec).
func TestSayStreamFramesUseSSEDataFormat(t *testing.T) {
	complete := true
	beats := []*a2av1.SayBeat{
		{Type: "npc_say_stream_done", SessionId: "s1", NpcId: "n",
			Complete: &complete, TsMs: 42},
	}
	h := &SayStreamHandler{APIKey: "", Client: &FakeSayStreamClient{Beats: beats}}
	body := `{"npc_id":"n","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := rec.Body.String()
	if !strings.Contains(out, "data: {") {
		t.Fatalf("no data frames: %s", out)
	}
	if !strings.Contains(out, "\n\n") {
		t.Fatalf("no SSE frame separator: %s", out)
	}
	// Sanity: X-Accel-Buffering header is set.
	if got := rec.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering=%q, want no", got)
	}
}