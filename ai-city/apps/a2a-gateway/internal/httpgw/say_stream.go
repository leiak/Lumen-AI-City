package httpgw

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// SayStreamClient is the A-city client interface to the remote B-city gRPC server.
//
// T04 declares this interface only; T06 provides the concrete implementation that
// dials B-city a2a-gateway over mTLS (see apps/a2a-gateway/internal/crosscity).
//
// Channels close semantics:
//   - beats: closed when the server closes the gRPC stream (EOF or RST).
//   - errs:  buffered (size 1); receives the terminal error if any, then is closed.
type SayStreamClient interface {
	SayStreamForward(ctx context.Context, init *a2av1.SayRequestInit) (
		beats <-chan *a2av1.SayBeat, errs <-chan error, err error)
}

// SayStreamHandler handles POST /v1/federation/say_stream.
//
// Body (JSON):
//
//	{
//	  "npc_id":       "npc_b_grace_healer_001",
//	  "player_input": "你好",
//	  "session_id":   "sess-...",   // optional but required by T04 contract
//	  "trace_id":     "tr-...",
//	  "player_id":    "...",
//	  "context":      [{"role":"user","content":"..."}]
//	}
//
// Response: text/event-stream. Each frame is `data: <json>\n\n` where <json>
// is an SSEFrame (see below). The stream terminates with a final `{"type":"end"}`
// frame on clean close, or an `{"type":"error","code":"R_016",...}` frame on
// upstream failure.
//
// Auth: if APIKey is non-empty, requires `Authorization: Bearer <APIKey>`.
//
// Status codes (pre-stream):
//   - 401: missing/wrong Bearer (when APIKey is set)
//   - 400 + R_009: invalid JSON, missing npc_id or session_id
//   - 502 + R_016: failed to open gRPC stream
type SayStreamHandler struct {
	APIKey string
	Client SayStreamClient
}

// SSEFrame is the wire format for one SSE event in the SayStreamForward stream.
// Field names mirror the proto SayBeat JSON so the same DTO can be parsed by
// api-gateway (T07) without a second translation layer.
type SSEFrame struct {
	Type          string `json:"type"` // "npc_say_stream" | "npc_say_stream_done" | "end" | "error"
	SessionID     string `json:"session_id,omitempty"`
	NPCID         string `json:"npc_id,omitempty"`
	SentenceIdx   *int32 `json:"sentence_idx,omitempty"`
	Text          string `json:"text,omitempty"`
	Emotion       string `json:"emotion,omitempty"`
	SentenceCount *int32 `json:"sentence_count,omitempty"`
	Complete      *bool  `json:"complete,omitempty"`
	TSMS          int64  `json:"ts_ms,omitempty"`
	TraceID       string `json:"trace_id,omitempty"`

	// Error fields (populated only when Type=={error}).
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// ServeHTTP implements http.Handler.
func (h *SayStreamHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Bearer auth (skipped when APIKey is empty — development mode).
	if h.APIKey != "" {
		if r.Header.Get("Authorization") != "Bearer "+h.APIKey {
			http.Error(w, `{"code":"R_001","message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	// 2. Parse SayRequestInit JSON body.
	var init a2av1.SayRequestInit
	if err := json.NewDecoder(r.Body).Decode(&init); err != nil {
		http.Error(w,
			fmt.Sprintf(`{"code":"R_009","message":"invalid body: %s"}`, err),
			http.StatusBadRequest)
		return
	}
	if init.GetNpcId() == "" || init.GetSessionId() == "" {
		http.Error(w,
			`{"code":"R_009","message":"npc_id and session_id required"}`,
			http.StatusBadRequest)
		return
	}

	// 3. SSE headers. X-Accel-Buffering=no disables nginx buffering so frames
	//    flush immediately to clients.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w,
			`{"code":"R_INTERNAL","message":"streaming unsupported"}`,
			http.StatusInternalServerError)
		return
	}

	// 4. Open gRPC stream. T06 concrete impl handles dial + TLS.
	beats, errs, err := h.Client.SayStreamForward(r.Context(), &init)
	if err != nil {
		http.Error(w,
			fmt.Sprintf(`{"code":"R_016","message":"%s"}`, err),
			http.StatusBadGateway)
		return
	}

	// 5. Pump SayBeat frames to SSE. Graceful exit on:
	//   - beats channel closed (server EOF) → emit `{"type":"end"}` sentinel
	//   - upstream error                       → emit `{"type":"error"}` + return
	//   - client disconnect (ctx.Done)         → just return
	for {
		select {
		case beat, ok := <-beats:
			if !ok {
				// Stream closed cleanly. Send sentinel so the client knows
				// the boundary; raw TCP fallback is also acceptable per SSE spec.
				writeSSEFrame(w, flusher, SSEFrame{Type: "end"})
				return
			}
			writeSSEFrame(w, flusher, beatToFrame(beat))
		case err := <-errs:
			if err == nil {
				// errs closed without a payload → treat as clean EOF.
				writeSSEFrame(w, flusher, SSEFrame{Type: "end"})
				return
			}
			data, _ := json.Marshal(SSEFrame{
				Type:    "error",
				Code:    "R_016",
				Message: err.Error(),
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
			return
		case <-r.Context().Done():
			// Client disconnected. Returning here cancels the gRPC stream
			// via context propagation in the concrete client (T06).
			return
		}
	}
}

// writeSSEFrame serialises a frame to `data: <json>\n\n` and flushes.
func writeSSEFrame(w http.ResponseWriter, flusher http.Flusher, f SSEFrame) {
	data, err := json.Marshal(f)
	if err != nil {
		// Should not happen for our DTO shape; fall back to an empty frame
		// so the client still sees a `data:` line.
		data = []byte(`{"type":"error","code":"R_INTERNAL","message":"marshal failed"}`)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// beatToFrame converts a proto SayBeat into our SSE wire format.
func beatToFrame(b *a2av1.SayBeat) SSEFrame {
	if b == nil {
		return SSEFrame{Type: "end"}
	}
	f := SSEFrame{
		Type:      b.GetType(),
		SessionID: b.GetSessionId(),
		NPCID:     b.GetNpcId(),
		TSMS:      b.GetTsMs(),
		TraceID:   b.GetTraceId(),
	}
	if b.SentenceIdx != nil {
		v := b.GetSentenceIdx()
		f.SentenceIdx = &v
	}
	if b.Text != nil {
		f.Text = b.GetText()
	}
	if b.Emotion != nil {
		f.Emotion = b.GetEmotion()
	}
	if b.SentenceCount != nil {
		v := b.GetSentenceCount()
		f.SentenceCount = &v
	}
	if b.Complete != nil {
		v := b.GetComplete()
		f.Complete = &v
	}
	return f
}