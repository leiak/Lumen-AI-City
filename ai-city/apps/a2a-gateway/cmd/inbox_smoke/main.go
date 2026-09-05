// Package main a2a-gateway Sprint 7 inbox E2E smoke（6 项）。
//
// 用法：
//   1) 启动 server：DATABASE_URL=postgresql://... A2A_GRPC_ADDR=... A2A_HTTP_ADDR=... ./bin/a2a-gateway.exe
//   2) 跑 smoke：  ./bin/inbox_smoke.exe
//
// 退出码：0 = 全部 OK；非 0 = 失败。
//
// 检查清单（6 项）：
//   1) POST /v1/cards offline_bob（无公钥）         → 200 accepted:true
//   2) POST /v1/messages alice → offline_bob        → 200 delivered:true（实际写 inbox）
//   3) GET  /v1/inbox/offline_bob                   → 1 message（含 payload / signature）
//   4) GET  /v1/inbox/offline_bob?mark_read=true    → 1 message, read_at 已设
//   5) GET  /v1/inbox/offline_bob 再次              → 0 message（已读）
//   6) GET  /v1/inbox/ghost                          → 404 F_013
//
// 依赖：
//   - a2a-gateway 已启动并连接 PG（DATABASE_URL 已设）
//   - 本 smoke 不破坏现有 a2a_smoke 12 项 + http_smoke 8 项
//   - 若 server 端未启用 PG，inbox 写可能 fail；本 smoke 假定生产配置（PG 在线）
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// cardReq 是 HTTP 注册请求体（对齐 httpgw.cardDTO）。
type inboxCardReq struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
}

type inboxRegisterResp struct {
	Accepted bool   `json:"accepted"`
	CardID   string `json:"card_id"`
}

type inboxMsgReq struct {
	MessageID   string `json:"message_id"`
	FromAgentID string `json:"from_agent_id"`
	ToAgentID   string `json:"to_agent_id"`
	Type        string `json:"type"`
	Payload     string `json:"payload"`
	TsMs        int64  `json:"ts_ms"`
}

type inboxSendResp struct {
	Delivered bool   `json:"delivered"`
	Error     string `json:"error"`
}

type inboxMsgDTO struct {
	MessageID      string `json:"message_id"`
	FromAgentID    string `json:"from_agent_id"`
	ToAgentID      string `json:"to_agent_id"`
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	Payload        string `json:"payload"`
	TsMs           int64  `json:"ts_ms"`
	TraceID        string `json:"trace_id"`
	Signature      string `json:"signature"`
}

type inboxFetchResp struct {
	Messages   []inboxMsgDTO `json:"messages"`
	NextCursor string        `json:"next_cursor"`
	TraceID    string        `json:"trace_id"`
}

type inboxErrEnv struct {
	Error   string `json:"error"`
	Detail  string `json:"detail"`
	TraceID string `json:"trace_id"`
}

// httpDo 一次 HTTP POST/GET。返 (status, body bytes)。
func inboxHTTPDo(method, url string, body any) (int, []byte) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		fmt.Printf("[FAIL] build req %s %s: %v\n", method, url, err)
		os.Exit(1)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[FAIL] %s %s: %v\n", method, url, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func main() {
	addr := os.Getenv("A2A_HTTP_ADDR")
	if addr == "" {
		addr = "http://127.0.0.1:8083"
	}
	apiKey := os.Getenv("A2A_HTTP_API_KEY")

	fmt.Printf("[inbox_smoke] dial %s (api_key=%s)\n", addr, apiKeyState(apiKey))

	// === 1) POST /v1/cards offline_bob ===
	{
		code, body := inboxHTTPDo(http.MethodPost, addr+"/v1/cards", inboxCardReq{
			AgentID: "offline_bob", Name: "Offline Bob",
		})
		if code != 200 {
			fmt.Printf("[FAIL] register offline_bob status=%d body=%s\n", code, body)
			os.Exit(2)
		}
		var r inboxRegisterResp
		_ = json.Unmarshal(body, &r)
		if !r.Accepted || r.CardID != "offline_bob" {
			fmt.Printf("[FAIL] register offline_bob resp=%+v\n", r)
			os.Exit(2)
		}
		fmt.Printf("[OK]   POST /v1/cards offline_bob → 200 accepted=true\n")
	}

	// === 2) POST /v1/messages alice → offline_bob → delivered:true ===
	// 假定 alice 已经由前面的 a2a_smoke / http_smoke 注册过；若没有，server 返 F_005（接受）
	{
		msg := inboxMsgReq{
			MessageID:   fmt.Sprintf("inbox_smoke_%d", time.Now().UnixNano()),
			FromAgentID: "alice",
			ToAgentID:   "offline_bob",
			Type:        "request",
			Payload:     "hello offline",
			TsMs:        time.Now().UnixMilli(),
		}
		code, body := inboxHTTPDo(http.MethodPost, addr+"/v1/messages", msg)
		if code != 200 {
			fmt.Printf("[FAIL] send alice→offline_bob status=%d body=%s\n", code, body)
			os.Exit(3)
		}
		var r inboxSendResp
		_ = json.Unmarshal(body, &r)
		if !r.Delivered {
			fmt.Printf("[FAIL] send alice→offline_bob delivered=false err=%q (sender alice may not be registered; run a2a_smoke first)\n", r.Error)
			os.Exit(3)
		}
		fmt.Printf("[OK]   POST /v1/messages alice→offline_bob → 200 delivered=true (queued in inbox)\n")
	}

	// === 3) GET /v1/inbox/offline_bob → 1 message ===
	{
		code, body := inboxHTTPDo(http.MethodGet, addr+"/v1/inbox/offline_bob?limit=10", nil)
		if code != 200 {
			fmt.Printf("[FAIL] fetch inbox status=%d body=%s\n", code, body)
			os.Exit(4)
		}
		var r inboxFetchResp
		_ = json.Unmarshal(body, &r)
		if len(r.Messages) != 1 {
			fmt.Printf("[FAIL] fetch inbox want 1 message, got %d: %+v\n", len(r.Messages), r.Messages)
			os.Exit(4)
		}
		m := r.Messages[0]
		if m.FromAgentID != "alice" || m.ToAgentID != "offline_bob" {
			fmt.Printf("[FAIL] inbox message from/to mismatch: from=%q to=%q\n", m.FromAgentID, m.ToAgentID)
			os.Exit(4)
		}
		if m.Payload != "hello offline" {
			fmt.Printf("[FAIL] inbox payload mismatch: got %q\n", m.Payload)
			os.Exit(4)
		}
		fmt.Printf("[OK]   GET /v1/inbox/offline_bob → 1 message (alice → offline_bob, payload preserved)\n")
	}

	// === 4) GET /v1/inbox/offline_bob?mark_read=true → 1 message, read_at 设了 ===
	{
		// 先注一条新消息（确保 step 3 没标已读）
		msg := inboxMsgReq{
			MessageID:   fmt.Sprintf("inbox_smoke_mark_%d", time.Now().UnixNano()),
			FromAgentID: "alice",
			ToAgentID:   "offline_bob",
			Type:        "request",
			Payload:     "second msg",
			TsMs:        time.Now().UnixMilli(),
		}
		code, _ := inboxHTTPDo(http.MethodPost, addr+"/v1/messages", msg)
		if code != 200 {
			fmt.Printf("[FAIL] pre-step-4 send status=%d\n", code)
			os.Exit(5)
		}

		code, body := inboxHTTPDo(http.MethodGet, addr+"/v1/inbox/offline_bob?limit=10&mark_read=true", nil)
		if code != 200 {
			fmt.Printf("[FAIL] fetch mark_read status=%d body=%s\n", code, body)
			os.Exit(5)
		}
		var r inboxFetchResp
		_ = json.Unmarshal(body, &r)
		if len(r.Messages) == 0 {
			fmt.Printf("[FAIL] mark_read fetch returned 0 messages (pre-step-4 send failed?)\n")
			os.Exit(5)
		}
		// 不强制要求恰好 1（可能之前的消息已重新被读），但至少 1
		fmt.Printf("[OK]   GET /v1/inbox/offline_bob?mark_read=true → %d messages (marked read)\n", len(r.Messages))
	}

	// === 5) GET /v1/inbox/offline_bob 再次 → 0 message（已读） ===
	{
		code, body := inboxHTTPDo(http.MethodGet, addr+"/v1/inbox/offline_bob?limit=10", nil)
		if code != 200 {
			fmt.Printf("[FAIL] fetch again status=%d body=%s\n", code, body)
			os.Exit(6)
		}
		var r inboxFetchResp
		_ = json.Unmarshal(body, &r)
		if len(r.Messages) != 0 {
			fmt.Printf("[FAIL] after mark_read, expected 0 unread, got %d\n", len(r.Messages))
			os.Exit(6)
		}
		fmt.Printf("[OK]   GET /v1/inbox/offline_bob (after mark_read) → 0 messages\n")
	}

	// === 6) GET /v1/inbox/ghost → 404 F_013 ===
	{
		code, body := inboxHTTPDo(http.MethodGet, addr+"/v1/inbox/ghost?limit=10", nil)
		if code != 404 {
			fmt.Printf("[FAIL] fetch ghost status=%d want 404 body=%s\n", code, body)
			os.Exit(7)
		}
		var e inboxErrEnv
		_ = json.Unmarshal(body, &e)
		if e.Error != "F_013" {
			fmt.Printf("[FAIL] ghost error=%q want F_013\n", e.Error)
			os.Exit(7)
		}
		fmt.Printf("[OK]   GET /v1/inbox/ghost → 404 F_013\n")
	}

	fmt.Printf("\n[OK] all 6 inbox_smoke checks passed against %s\n", addr)
}

func apiKeyState(k string) string {
	if k == "" {
		return "unset"
	}
	return "set"
}
