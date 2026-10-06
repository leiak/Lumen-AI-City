package httpgw

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aicity/a2a-gateway/internal/crosscity"
)

// SessionReplayHandler serves GET /v1/federation/sessions/{sid}/buffer.
//
// 用途：跨城流式 (B1) SayStreamForward 断线 / 重连后，客户端可调用此端点
// 从 MirrorStore 拉取 from_idx 起的 beats 实现 replay。
//
// MirrorStore 由 cmd/main.go 注入（T02 实现），由 T06 forwarder 在 init 帧
// Create、每个 beat 帧 Append、defer MarkDone。
//
// 响应（200）：
//
//	{
//	  "session_id": "sess-...",
//	  "npc_id":     "npc_b_wu",    // MirrorStore.Get 暴露 NPCID，handler 填充
//	  "complete":   false,
//	  "from_idx":   2,
//	  "beats": [
//	    {"sentence_idx": 2, "text": "...", "emotion": "happy", "ts_ms": 1000},
//	    ...
//	  ]
//	}
//
// 错误：
//   - 401 + R_001：Bearer 鉴权失败（APIKey 非空时启用）
//   - 400 + R_009：from_idx 非法（非 uint32）；或 path 缺 sid
//   - 400 + R_015：session 不存在或已过期（ErrSessionNotFound）
//   - 503 + R_016：MirrorStore 不可用（h.Mirror == nil）
//   - 500 + R_011：其它 MirrorStore 错误
//
// 注意：路由解析 sid 的同时实现：
//
//   - 标准库 mux 用 `r.PathValue("sid")`（Go 1.22+ 路由 `{sid}`）
//   - gin v1.10 不支持 `{sid}` 语法且 gin.WrapH 不传播 PathValue，
//     因此 path 解析需手动 fallback（cmd/main.go 用 gin 挂载）
type SessionReplayHandler struct {
	APIKey string
	Mirror *crosscity.MirrorStore
}

// ReplayResponse 是客户端拿到的 JSON 形状。
type ReplayResponse struct {
	SessionID string      `json:"session_id"`
	NPCID     string      `json:"npc_id"`
	Complete  bool        `json:"complete"`
	FromIdx   uint32      `json:"from_idx"`
	Beats     []BeatFrame `json:"beats"`
}

// BeatFrame 镜像 crosscity.Beat 的字段，JSON tag 用 snake_case。
type BeatFrame struct {
	SentenceIdx uint32 `json:"sentence_idx"`
	Text        string `json:"text"`
	Emotion     string `json:"emotion"`
	TSMS        int64  `json:"ts_ms"`
}

// ServeHTTP 实现 http.Handler。
func (h *SessionReplayHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 1. Bearer 鉴权（constant-time）。空 APIKey 跳过（dev 模式）。
	if h.APIKey != "" {
		auth := r.Header.Get("Authorization")
		expected := "Bearer " + h.APIKey
		if subtle.ConstantTimeCompare([]byte(auth), []byte(expected)) != 1 {
			http.Error(w, `{"code":"R_001","message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	// 2. 路径参数：sid。
	//    标准库 mux 走 r.PathValue；gin 挂载时 fallback 到手动 URL 切片。
	sid := r.PathValue("sid")
	if sid == "" {
		sid = extractSidFromPath(r.URL.Path)
	}
	if sid == "" {
		http.Error(w, `{"code":"R_009","message":"session_id required"}`, http.StatusBadRequest)
		return
	}

	// 3. 查询参数：from_idx（缺省 0）。负数由 ParseUint 自动拒绝（非数字串）。
	fromIdx := uint32(0)
	if raw := r.URL.Query().Get("from_idx"); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 32)
		if err != nil {
			http.Error(w, `{"code":"R_009","message":"from_idx must be uint32"}`, http.StatusBadRequest)
			return
		}
		fromIdx = uint32(n)
	}

	// 4. Mirror 可用？
	if h.Mirror == nil {
		http.Error(w, `{"code":"R_016","message":"mirror store unavailable"}`, http.StatusServiceUnavailable)
		return
	}

	// 5. 取 buffer。
	beats, done, err := h.Mirror.Buffer(sid, fromIdx)
	if err != nil {
		if errors.Is(err, crosscity.ErrSessionNotFound) {
			http.Error(w, `{"code":"R_015","message":"session expired or not found"}`, http.StatusBadRequest)
			return
		}
		http.Error(w,
			fmt.Sprintf(`{"code":"R_011","message":"%s"}`, err),
			http.StatusInternalServerError)
		return
	}

	// 6. 构造响应。
	resp := ReplayResponse{
		SessionID: sid,
		// MirrorStore.Get 暴露 NPCID accessor；填充便于首次缓冲就拿到 npc_id。
		// 若 Get 失败（理论上 buffer 已成功则 Get 也会成功），保持空字符串。
		NPCID:    "",
		Complete: done,
		FromIdx:  fromIdx,
		Beats:    make([]BeatFrame, 0, len(beats)),
	}
	if sess, gErr := h.Mirror.Get(sid); gErr == nil && sess != nil {
		resp.NPCID = sess.NPCID
	}
	for _, b := range beats {
		resp.Beats = append(resp.Beats, BeatFrame{
			SentenceIdx: b.SentenceIdx,
			Text:        b.Text,
			Emotion:     b.Emotion,
			TSMS:        b.TSMS,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// extractSidFromPath 从 /v1/federation/sessions/{sid}/buffer 抽出 sid。
// 不匹配或 sid 为空返 ""。
//
// 为什么需要：gin v1.10 用 :name 而非 stdlib {name}；gin.WrapH 把 request
// 透传进 http.Handler 时不写 r.PathValue，需要手动从 URL.Path 切。
func extractSidFromPath(p string) string {
	const prefix = "/v1/federation/sessions/"
	const suffix = "/buffer"
	if !strings.HasPrefix(p, prefix) || !strings.HasSuffix(p, suffix) {
		return ""
	}
	mid := p[len(prefix) : len(p)-len(suffix)]
	if mid == "" || strings.Contains(mid, "/") {
		return ""
	}
	return mid
}