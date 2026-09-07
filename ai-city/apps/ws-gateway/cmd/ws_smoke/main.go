// Sprint 9 E2E smoke：登录 → 连 WS → 触发 move → 等 WS 推送。
//
// 验的是整条链路：
//
//	api-gateway /v1/world/move → world-engine gRPC → Redis pub
//	  → ws-gateway sub → WS 扇出 → 本进程收到 player_moved 信封
//
// 用法（容器内）：
//
//	docker compose exec -T ws-gateway /app/ws_smoke
//
// env：
//
//	SMOKE_API_URL   默认 http://api-gateway:8080
//	SMOKE_WS_URL    默认 ws://127.0.0.1:8082/ws
//	SMOKE_USERNAME  默认 demo
//	SMOKE_PASSWORD  默认 demo123
//
// 退出码 0 = 全过；非 0 = 失败（步号即退出码）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"nhooyr.io/websocket"
)

const (
	targetX = 50.0
	targetY = 50.0
	// 目标坐标落在 tile_0_0（tileIdAt = floor(x/100)）
	targetTile = "tile_0_0"
)

type loginResp struct {
	Token    string `json:"token"`
	PlayerID string `json:"player_id"`
	Username string `json:"username"`
}

type moveResp struct {
	PlayerID    string  `json:"player_id"`
	CurrentTile string  `json:"current_tile_id"`
	X           float32 `json:"x"`
	Y           float32 `json:"y"`
	Accepted    bool    `json:"accepted"`
}

type playerMoved struct {
	PlayerID string  `json:"player_id"`
	TileID   string  `json:"tile_id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	TsMs     int64   `json:"ts_ms"`
}

type envelope struct {
	Type    string      `json:"type"`
	TraceID string      `json:"trace_id"`
	TsMs    int64       `json:"ts_ms"`
	Payload playerMoved `json:"payload"`
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func failf(code int, format string, args ...any) {
	fmt.Printf("[FAIL] "+format+"\n", args...)
	os.Exit(code)
}

func main() {
	apiURL := env("SMOKE_API_URL", "http://api-gateway:8080")
	wsURL := env("SMOKE_WS_URL", "ws://127.0.0.1:8082/ws")
	username := env("SMOKE_USERNAME", "demo")
	password := env("SMOKE_PASSWORD", "demo123")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 10 * time.Second}

	// 1) 登录拿 token + player_id
	login, err := doLogin(ctx, client, apiURL, username, password)
	if err != nil {
		failf(1, "login: %v", err)
	}
	fmt.Printf("[OK]   1/6 login user=%s player_id=%s\n", login.Username, login.PlayerID)

	// 2) 连 WS（token 走 query string —— 浏览器 WebSocket 不支持自定义 header）
	conn, _, err := websocket.Dial(ctx, wsURL+"?token="+login.Token, nil)
	if err != nil {
		failf(2, "ws dial %s: %v", wsURL, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "smoke done")
	fmt.Printf("[OK]   2/6 ws connected %s\n", wsURL)

	// 3) 无 token 必须被拒（401）—— 鉴权真在生效，而不是恰好放行
	if _, _, err := websocket.Dial(ctx, wsURL, nil); err == nil {
		failf(3, "ws dial without token succeeded, want 401")
	}
	fmt.Printf("[OK]   3/6 ws without token rejected\n")

	// 4) 触发 move。先起接收 goroutine 再发请求，避免事件早于 Read 到达
	//    （WS 无缓冲，Read 之前到的帧留在 TCP 缓冲里其实也不会丢，
	//     但先起 reader 能让超时窗口只覆盖真正的传播延迟）。
	recv := make(chan envelope, 4)
	recvErr := make(chan error, 1)
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				recvErr <- err
				return
			}
			var e envelope
			if err := json.Unmarshal(data, &e); err != nil {
				fmt.Printf("[warn] non-JSON ws frame: %s\n", data)
				continue
			}
			recv <- e
		}
	}()

	mv, err := doMove(ctx, client, apiURL, login.Token, login.PlayerID)
	if err != nil {
		failf(4, "move: %v", err)
	}
	if !mv.Accepted {
		failf(4, "move accepted=false")
	}
	fmt.Printf("[OK]   4/6 move accepted tile=%s pos=(%.1f,%.1f)\n",
		mv.CurrentTile, mv.X, mv.Y)

	// 5) 等自己那条 player_moved（5s 超时）
	deadline := time.After(5 * time.Second)
	var got envelope
	for {
		select {
		case e := <-recv:
			if e.Type != "player_moved" {
				fmt.Printf("[warn] ignoring envelope type=%s\n", e.Type)
				continue
			}
			if e.Payload.PlayerID != login.PlayerID {
				fmt.Printf("[warn] ignoring other player %s\n", e.Payload.PlayerID)
				continue
			}
			got = e
		case err := <-recvErr:
			failf(5, "ws read: %v", err)
		case <-deadline:
			failf(5, "no player_moved envelope for %s within 5s", login.PlayerID)
		}
		break
	}
	fmt.Printf("[OK]   5/6 player_moved received trace_id=%s ts_ms=%d\n",
		got.TraceID, got.TsMs)

	// 6) 校验 payload 字段（player_id 而非 entity_id；坐标为服务端校正后的值）
	if got.TraceID == "" || got.TsMs <= 0 {
		failf(6, "envelope trace_id=%q ts_ms=%d", got.TraceID, got.TsMs)
	}
	if math.Abs(float64(got.Payload.X-targetX)) > 0.01 ||
		math.Abs(float64(got.Payload.Y-targetY)) > 0.01 {
		failf(6, "payload pos=(%.3f,%.3f), want (%.1f,%.1f)",
			got.Payload.X, got.Payload.Y, targetX, targetY)
	}
	if got.Payload.TileID != targetTile {
		failf(6, "payload tile_id=%q, want %q", got.Payload.TileID, targetTile)
	}
	if got.Payload.TsMs <= 0 {
		failf(6, "payload ts_ms=%d", got.Payload.TsMs)
	}
	fmt.Printf("[OK]   6/6 payload player_id=%s tile_id=%s pos=(%.1f,%.1f)\n",
		got.Payload.PlayerID, got.Payload.TileID, got.Payload.X, got.Payload.Y)

	fmt.Printf("\n[OK] all 6 ws_smoke checks passed (api=%s ws=%s)\n", apiURL, wsURL)
}

func doLogin(ctx context.Context, c *http.Client, apiURL, username, password string) (*loginResp, error) {
	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiURL+"/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, raw)
	}

	var out loginResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w (body=%s)", err, raw)
	}
	if out.Token == "" || out.PlayerID == "" {
		return nil, fmt.Errorf("empty token or player_id: %s", raw)
	}
	return &out, nil
}

func doMove(ctx context.Context, c *http.Client, apiURL, token, playerID string) (*moveResp, error) {
	body, _ := json.Marshal(map[string]any{
		"player_id":    playerID,
		"from_tile_id": targetTile,
		"to_tile_id":   targetTile,
		"x":            targetX,
		"y":            targetY,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiURL+"/v1/world/move", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, raw)
	}

	var out moveResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode: %w (body=%s)", err, raw)
	}
	return &out, nil
}
