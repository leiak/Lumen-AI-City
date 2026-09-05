// Package main A2A Gateway HTTP E2E smoke（Sprint 6 8 项）。
//
// 用法：
//   1) 启动 server： A2A_HTTP_ADDR=127.0.0.1:8083 ./bin/a2a-gateway.exe
//   2) 跑 smoke：   A2A_HTTP_ADDR=127.0.0.1:8083 ./bin/http_smoke.exe
//
// 退出码：0 = 全部 OK；非 0 = 失败。
//
// 检查清单（8 项）：
//   1) GET  /v1/healthz     → 200 {"ok":true}
//   2) POST /v1/cards       alice（无 auth） → 200 {"accepted":true,"card_id":"alice"}
//   3) POST /v1/cards       缺 name → 400 F_001
//   4) POST /v1/cards       alice 带 auth.ed25519=!!bad!! → 400 F_006
//   5) GET  /v1/discover?capability=chat → 1 card
//   6) GET  /v1/discover    （无 cap） → 400 F_003
//   7) POST /v1/messages    alice→bob 签过 → 200 {"delivered":true}
//   8) POST /v1/messages    alice→ghost → 200 {"delivered":false,"error":"F_004..."}
//
// 注意：HTTP smoke 不覆盖 outbound HTTP adapter 的端到端转发（用单测 httptest 已足够）。
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// cardReq 是 HTTP 注册请求体（对齐 httpgw.cardDTO）。
type cardReq struct {
	AgentID      string            `json:"agent_id"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Auth         map[string]string `json:"auth,omitempty"`
}

type registerResp struct {
	Accepted bool   `json:"accepted"`
	CardID   string `json:"card_id"`
}

type discoverResp struct {
	Cards []cardReq `json:"cards"`
}

type msgReq struct {
	MessageID   string `json:"message_id"`
	FromAgentID string `json:"from_agent_id"`
	ToAgentID   string `json:"to_agent_id"`
	Type        string `json:"type"`
	Payload     string `json:"payload"`
	TsMs        int64  `json:"ts_ms"`
	Signature   string `json:"signature,omitempty"`
}

type sendResp struct {
	Delivered bool   `json:"delivered"`
	Error     string `json:"error"`
}

type errEnv struct {
	Error   string `json:"error"`
	Detail  string `json:"detail"`
	TraceID string `json:"trace_id"`
}

// httpDo 一次 HTTP POST/GET。返 (status, body bytes)。
func httpDo(t, method, path, token string, body any) (int, []byte) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, t+path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("[FAIL] %s %s: %v\n", method, path, err)
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
	apiKey := os.Getenv("A2A_HTTP_API_KEY") // 留空 = 关闭鉴权（dev）

	fmt.Printf("[http_smoke] dial %s (api_key=%s)\n", addr, apiKeyState(apiKey))

	// === 1) GET /v1/healthz ===
	{
		code, body := httpDo(addr, http.MethodGet, "/v1/healthz", apiKey, nil)
		if code != 200 {
			fmt.Printf("[FAIL] healthz status=%d body=%s\n", code, body)
			os.Exit(1)
		}
		var r map[string]any
		_ = json.Unmarshal(body, &r)
		if r["ok"] != true {
			fmt.Printf("[FAIL] healthz ok=%v body=%s\n", r["ok"], body)
			os.Exit(1)
		}
		fmt.Printf("[OK]   GET /v1/healthz → 200 ok=true\n")
	}

	// === 2) POST /v1/cards alice ===
	{
		code, body := httpDo(addr, http.MethodPost, "/v1/cards", apiKey, cardReq{
			AgentID: "alice", Name: "Alice",
			Provider:     "aicity",
			Capabilities: []string{"chat"},
		})
		if code != 200 {
			fmt.Printf("[FAIL] register alice status=%d body=%s\n", code, body)
			os.Exit(2)
		}
		var r registerResp
		_ = json.Unmarshal(body, &r)
		if !r.Accepted || r.CardID != "alice" {
			fmt.Printf("[FAIL] register alice resp=%+v\n", r)
			os.Exit(2)
		}
		fmt.Printf("[OK]   POST /v1/cards alice → 200 accepted=true\n")
	}

	// === 3) POST /v1/cards 缺 name → 400 F_001 ===
	{
		code, body := httpDo(addr, http.MethodPost, "/v1/cards", apiKey, map[string]any{
			"agent_id": "alice_no_name",
		})
		if code != 400 {
			fmt.Printf("[FAIL] register no-name status=%d want 400 body=%s\n", code, body)
			os.Exit(3)
		}
		var e errEnv
		_ = json.Unmarshal(body, &e)
		if e.Error != "F_001" {
			fmt.Printf("[FAIL] register no-name error=%q want F_001\n", e.Error)
			os.Exit(3)
		}
		fmt.Printf("[OK]   POST /v1/cards (missing name) → 400 F_001\n")
	}

	// === 4) POST /v1/cards 带 bad ed25519 → 400 F_006 ===
	{
		code, body := httpDo(addr, http.MethodPost, "/v1/cards", apiKey, cardReq{
			AgentID: "evil", Name: "Evil",
			Auth: map[string]string{"ed25519": "!!bad!!"},
		})
		if code != 400 {
			fmt.Printf("[FAIL] register bad-pubkey status=%d want 400 body=%s\n", code, body)
			os.Exit(4)
		}
		var e errEnv
		_ = json.Unmarshal(body, &e)
		if e.Error != "F_006" {
			fmt.Printf("[FAIL] register bad-pubkey error=%q want F_006\n", e.Error)
			os.Exit(4)
		}
		fmt.Printf("[OK]   POST /v1/cards (bad ed25519) → 400 F_006\n")
	}

	// === 5) GET /v1/discover?capability=chat → 含 alice 的 card（不依赖数量） ===
	{
		code, body := httpDo(addr, http.MethodGet, "/v1/discover?capability=chat", apiKey, nil)
		if code != 200 {
			fmt.Printf("[FAIL] discover chat status=%d body=%s\n", code, body)
			os.Exit(5)
		}
		var r discoverResp
		_ = json.Unmarshal(body, &r)
		// 不要求 "恰好 1 card"——若 gRPC smoke 先跑，bob 也在；只要 alice 在列表里即 OK。
		foundAlice := false
		for _, c := range r.Cards {
			if c.AgentID == "alice" {
				foundAlice = true
				break
			}
		}
		if !foundAlice {
			fmt.Printf("[FAIL] discover chat should include alice, got %d cards: %+v\n", len(r.Cards), r.Cards)
			os.Exit(5)
		}
		fmt.Printf("[OK]   GET /v1/discover?capability=chat → alice in cards\n")
	}

	// === 6) GET /v1/discover (无 cap) → 400 F_003 ===
	{
		code, body := httpDo(addr, http.MethodGet, "/v1/discover", apiKey, nil)
		if code != 400 {
			fmt.Printf("[FAIL] discover no-cap status=%d want 400 body=%s\n", code, body)
			os.Exit(6)
		}
		var e errEnv
		_ = json.Unmarshal(body, &e)
		if e.Error != "F_003" {
			fmt.Printf("[FAIL] discover no-cap error=%q want F_003\n", e.Error)
			os.Exit(6)
		}
		fmt.Printf("[OK]   GET /v1/discover (no cap) → 400 F_003\n")
	}

	// === 7) POST /v1/messages alice→bob 签过 → delivered:true ===
	{
		// 先生成 bob + alice2（带 ed25519）
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fmt.Printf("[FAIL] GenerateKey: %v\n", err)
			os.Exit(7)
		}
		// bob 不带 key（opt-in 放行）
		httpDo(addr, http.MethodPost, "/v1/cards", apiKey, cardReq{
			AgentID: "bob", Name: "Bob", Provider: "aicity",
		})
		// alice2 带 key（必签）
		httpDo(addr, http.MethodPost, "/v1/cards", apiKey, cardReq{
			AgentID: "alice2", Name: "Alice2", Provider: "aicity",
			Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
		})

		now := time.Now().UnixMilli()
		m := msgReq{
			MessageID: "m_http_signed", FromAgentID: "alice2", ToAgentID: "bob",
			Type: "request", Payload: "aGk=", TsMs: now,
		}
		m.Signature = signHTTP(priv, m)

		code, body := httpDo(addr, http.MethodPost, "/v1/messages", apiKey, m)
		if code != 200 {
			fmt.Printf("[FAIL] send signed status=%d body=%s\n", code, body)
			os.Exit(7)
		}
		var r sendResp
		_ = json.Unmarshal(body, &r)
		if !r.Delivered || r.Error != "" {
			fmt.Printf("[FAIL] send signed delivered=%v err=%q\n", r.Delivered, r.Error)
			os.Exit(7)
		}
		fmt.Printf("[OK]   POST /v1/messages alice2→bob (signed) → 200 delivered=true\n")
	}

	// === 8) POST /v1/messages alice→ghost → delivered:false F_004 ===
	{
		m := msgReq{
			MessageID: "m_ghost", FromAgentID: "alice", ToAgentID: "ghost",
			Type: "request", Payload: "aGk=", TsMs: time.Now().UnixMilli(),
		}
		code, body := httpDo(addr, http.MethodPost, "/v1/messages", apiKey, m)
		if code != 200 {
			fmt.Printf("[FAIL] send ghost status=%d body=%s\n", code, body)
			os.Exit(8)
		}
		var r sendResp
		_ = json.Unmarshal(body, &r)
		if r.Delivered {
			fmt.Printf("[FAIL] ghost should not be delivered\n")
			os.Exit(8)
		}
		if !strings.HasPrefix(r.Error, "F_004") {
			fmt.Printf("[FAIL] ghost err=%q want F_004 prefix\n", r.Error)
			os.Exit(8)
		}
		fmt.Printf("[OK]   POST /v1/messages alice→ghost → 200 delivered=false F_004\n")
	}

	fmt.Printf("\n[OK] all 8 http_smoke checks passed against %s\n", addr)
}

// signHTTP 用 priv 对 m 做签，返 base64(stdEncoding)。
// 与 server a2asrv.canonicalEnvelope 字段顺序严格一致（mirror）。
func signHTTP(priv ed25519.PrivateKey, m msgReq) string {
	env := struct {
		MessageID      string `json:"message_id"`
		FromAgentID    string `json:"from_agent_id"`
		ToAgentID      string `json:"to_agent_id"`
		ConversationID string `json:"conversation_id"`
		Type           string `json:"type"`
		PayloadB64     string `json:"payload_b64"`
		TsMs           int64  `json:"ts_ms"`
		TraceID        string `json:"trace_id"`
	}{
		MessageID:      m.MessageID,
		FromAgentID:    m.FromAgentID,
		ToAgentID:      m.ToAgentID,
		ConversationID: "",
		Type:           m.Type,
		PayloadB64:     base64.RawStdEncoding.EncodeToString([]byte(m.Payload)),
		TsMs:           m.TsMs,
		TraceID:        "",
	}
	body, _ := json.Marshal(env)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, body))
}

func apiKeyState(k string) string {
	if k == "" {
		return "unset"
	}
	return "set"
}
