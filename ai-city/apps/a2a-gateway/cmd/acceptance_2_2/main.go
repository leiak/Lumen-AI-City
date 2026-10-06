// acceptance_2_2 —— B1 跨城流式 5 步 E2E 验证。
//
// 验证 B1 跨城流式 replay 链路：mint sid → POST /v1/federation/say_stream
// 读 ≥1 beat 后主动断流 → GET buffer 验证 mirror 已落地 → 轮询 buffer 直到
// mirror.Complete=true → 总结 5/5 PASS。
//
// 设计：docs/superpowers/plans/2026-10-06-2.0-stage3-cross-city-stream.md T09。
// - Step 1：A 城登录拿 token（api-gateway /v1/auth/login）
// - Step 2：mint sid + POST /v1/federation/say_stream，读 ≥1 beat 后断流
//           （主动断开模拟"客户端掉线"场景）
// - Step 3：GET /v1/federation/sessions/{sid}/buffer 验证 mirror 已缓存
//           ≥1 beat（即使上游 producer 还在跑，断流前收到的也要 replay）
// - Step 4：轮询 buffer 直到 complete=true（forwarder 流跑完后会 MarkDone）
// - Step 5：总结 5/5 PASS
//
// 用法（bake 进 a2a-gateway 镜像）：
//
//	docker compose exec -T a2a-gateway /app/acceptance_2_2
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// ExitOK / ExitStepFailed：仿 acceptance_2_0/2_1 风格。
	ExitOK         = 0
	ExitStepFailed = 1

	// 整体超时 90s：登录 + SSE 短读 + 轮询 complete 兜底。
	totalBudget = 90 * time.Second

	// step2SSEBudget 是 step 2 中等 ≥1 beat 的最长等待时间；超过即判失败。
	step2SSEBudget = 8 * time.Second

	// step4PollInterval / step4PollBudget 是 step 4 轮询 buffer 直到 complete=true
	// 的间隔与上限。
	step4PollInterval = 500 * time.Millisecond
	step4PollBudget   = 60 * time.Second
)

// npcID 选 B 城 NPC（与 stage 2 acceptance_2_1 的 5 NPC 不重叠；前向联邦路径）。
const npcID = "npc_b_grace_healer_001"

var (
	apiBase = getEnv("API_GATEWAY_URL", "http://api-gateway:8080")
	a2aBase = getEnv("A2A_HUB_URL", "http://a2a-gateway:8083")
	apiKey  = os.Getenv("A2A_HTTP_API_KEY") // 与 a2a-gateway A2A_HTTP_API_KEY 对齐
)

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("acceptance_2_2 start: api=%s a2a=%s api_key=%s",
		apiBase, a2aBase, redactKey(apiKey))

	ctx, cancel := context.WithTimeout(context.Background(), totalBudget)
	defer cancel()

	// Step 1: A 城登录。
	fmt.Println("Step 1: A 城登录")
	token, err := login(ctx)
	if err != nil {
		die("step 1", err)
	}
	fmt.Printf("  PASS login → token=%s...\n", token[:min(8, len(token))])

	// Step 2: mint sid + POST /v1/federation/say_stream，读 ≥1 beat 后断流。
	fmt.Println("Step 2: SSE 触发跨城 NPC 流式 + 主动断流")
	sid, beatCount, err := triggerSSEAndDisconnect(ctx, token)
	if err != nil {
		die("step 2", err)
	}
	fmt.Printf("  PASS sid=%s, 读到 %d beat 后断流\n", sid, beatCount)

	// Step 3: GET buffer 验证 mirror 至少落地了 1 beat。
	fmt.Println("Step 3: GET buffer 验证 mirror replay")
	beatsLen, err := fetchBufferCount(ctx, sid, 0)
	if err != nil {
		die("step 3", err)
	}
	if beatsLen == 0 {
		die("step 3", fmt.Errorf("buffer empty after disconnect, replay would lose data"))
	}
	fmt.Printf("  PASS replay buffer=%d beats\n", beatsLen)

	// Step 4: 轮询 buffer 直到 mirror.Complete=true。
	fmt.Println("Step 4: 轮询 buffer 直到 mirror.Complete=true")
	if err := waitComplete(ctx, sid); err != nil {
		die("step 4", err)
	}
	fmt.Println("  PASS mirror.Complete=true")

	// Step 5: 总结。
	fmt.Println("\n=== acceptance_2_2 5/5 PASS ===")
	os.Exit(ExitOK)
}

// login POST /v1/auth/login，返 token。失败 → err。
func login(ctx context.Context) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"username": "demo",
		"password": "demo123",
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+"/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("login decode: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("login empty token")
	}
	return out.Token, nil
}

// mintSID 生成 sess-<32hex>（16 随机字节 → 32 hex 字符）。
func mintSID() string {
	var buf [16]byte
	_, _ = rand.Read(buf[:])
	return "sess-" + hex.EncodeToString(buf[:])
}

// triggerSSEAndDisconnect mint sid → POST /v1/federation/say_stream →
// 用 bufio.Scanner 读 `data: ` 前缀行记 beat 数；读到 ≥1 beat 或超 step2SSEBudget
// 即 cancel ctx（释放 TCP + 取消 SSE 流），返 (sid, beatCount)。
//
// 主动断流是测 B1-T08 replay 的关键：客户端半路掉线，server 端 producer 还在跑
// （forwarder 持续推 beat 到 MirrorStore），但客户端已经收不到。后续 GET buffer
// 必须能拿到断流前已收到的 beat。
func triggerSSEAndDisconnect(ctx context.Context, token string) (string, int, error) {
	sid := mintSID()

	body, _ := json.Marshal(map[string]any{
		"session_id":   sid,
		"npc_id":       npcID,
		"player_id":    "demo",
		"player_input": "客官您来了",
		"trace_id":     "acc-2-2-" + sid,
	})

	// 用独立 ctx 控制断流时机：拿到 ≥1 beat 或超时即 cancel，触发 SSE handler
	// 的 ctx.Done() 路径 + 释放 TCP 连接。
	streamCtx, streamCancel := context.WithTimeout(ctx, step2SSEBudget)
	defer streamCancel()

	req, err := http.NewRequestWithContext(streamCtx, http.MethodPost,
		a2aBase+"/v1/federation/say_stream", bytes.NewReader(body))
	if err != nil {
		return sid, 0, fmt.Errorf("build SSE req: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// 优先用 a2a-gateway 的 APIKey；空 = dev 模式（a2a-gateway APIKey env 留空）。
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sid, 0, fmt.Errorf("SSE POST: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return sid, 0, fmt.Errorf("SSE status %d: %s", resp.StatusCode, b)
	}

	scanner := bufio.NewScanner(resp.Body)
	// 单帧可能较大；放宽 buffer 上限避免长 text 截断。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	beatCount := 0
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			continue
		}
		// end / error 不算 beat（producer 关闭或出错）。其余 type 一律记 1 beat。
		t, _ := payload["type"].(string)
		if t == "end" || t == "error" {
			continue
		}
		beatCount++
		// 读到 ≥1 beat 就主动断流——模拟客户端掉线。
		if beatCount >= 1 {
			break
		}
	}
	// 主动断流后再确认 scanner 没把"读到一半连接被关"误判为正常 EOF。
	if err := scanner.Err(); err != nil {
		log.Printf("scanner err after %d beats: %v", beatCount, err)
	}
	// 拿到 beat 后 streamCancel 会 defer 时跑；这里也显式触发一次以尽快释放 TCP。
	streamCancel()

	if beatCount == 0 {
		return sid, 0, fmt.Errorf("no beat in %s", step2SSEBudget)
	}
	return sid, beatCount, nil
}

// fetchBufferCount GET /v1/federation/sessions/{sid}/buffer?from_idx=N，
// 返 beats 长度。
func fetchBufferCount(ctx context.Context, sid string, fromIdx uint32) (int, error) {
	url := fmt.Sprintf("%s/v1/federation/sessions/%s/buffer?from_idx=%d",
		a2aBase, sid, fromIdx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("buffer GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("buffer status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Beats    []map[string]any `json:"beats"`
		Complete bool             `json:"complete"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("buffer decode: %w", err)
	}
	return len(out.Beats), nil
}

// waitComplete 轮询 buffer 直到 complete=true 或超时。
//
// 复用 fetchBufferCount；为减少一次重复请求这里直接读 complete 字段。
func waitComplete(ctx context.Context, sid string) error {
	deadline := time.Now().Add(step4PollBudget)
	pollCtx, pollCancel := context.WithTimeout(ctx, step4PollBudget)
	defer pollCancel()

	attempts := 0
	for {
		attempts++
		complete, beats, err := fetchBufferComplete(pollCtx, sid)
		if err != nil {
			return fmt.Errorf("poll attempt %d: %w", attempts, err)
		}
		if complete {
			fmt.Printf("  attempt %d: beats=%d, complete=true\n", attempts, beats)
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %d attempts, last beats=%d",
				attempts, beats)
		}
		time.Sleep(step4PollInterval)
	}
}

// fetchBufferComplete GET buffer 并返 (complete, beatsLen, err)。
func fetchBufferComplete(ctx context.Context, sid string) (bool, int, error) {
	url := fmt.Sprintf("%s/v1/federation/sessions/%s/buffer?from_idx=0", a2aBase, sid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, 0, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, 0, fmt.Errorf("buffer GET: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return false, 0, fmt.Errorf("buffer status %d: %s", resp.StatusCode, b)
	}
	var out struct {
		Beats    []map[string]any `json:"beats"`
		Complete bool             `json:"complete"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, 0, fmt.Errorf("buffer decode: %w", err)
	}
	return out.Complete, len(out.Beats), nil
}

// die 打印 FAIL + 退出码，仿 acceptance_2_1 风格。
func die(step string, err error) {
	fmt.Fprintf(os.Stderr, "FAIL %s: %v\n", step, err)
	os.Exit(ExitStepFailed)
}

// redactKey 把 api key 缩成 "set" / "unset"，避免日志泄露。
func redactKey(k string) string {
	if k == "" {
		return "unset"
	}
	return "set"
}

