// acceptance_1_0 — 1.0 demo Definition of Done 二进制
//
// 验证 5 步闭环：login → walk → 接近 NPC → 主动 say → 玩家回复。
//
// 设计：docs/1.0-acceptance-design.md。
// - 跨 8 服务（postgres / redis / world-engine / api-gateway / ws-gateway /
//   web / a2a-gateway / agent-os）。
// - 状态复位保证确定性（-reset-state=true 默认）。
// - 失败 hint 表让 stakeholder 5min 内 fix。
// - 退出码 0=pass / 1=step fail / 2=setup fail / 3=bad CLI。
//
// 用法（bake 进 a2a-gateway 镜像）：
//
//	docker compose exec -T a2a-gateway /app/acceptance_1_0 [flags]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
)

// ----------------------- CLI flags -----------------------

type flags struct {
	APIURL    string
	WSURL     string
	User      string
	Password  string
	NPCID     string
	Tile      string
	Timeout   time.Duration
	Reset     bool
	Verbose   bool
	KeepAlive bool
	ShowVer   bool
}

func parseFlags() (*flags, error) {
	f := &flags{}
	flag.StringVar(&f.APIURL, "api-url", "http://api-gateway:8080", "api-gateway base URL")
	flag.StringVar(&f.WSURL, "ws-url", "ws://ws-gateway:8082", "ws-gateway base URL")
	flag.StringVar(&f.User, "user", "demo", "test username")
	flag.StringVar(&f.Password, "password", "demo123", "test password")
	flag.StringVar(&f.NPCID, "npc", "npc_wang_boss_001", "target NPC agent_id")
	flag.StringVar(&f.Tile, "tile", "tile_0_0", "target tile id (where the NPC lives)")
	flag.DurationVar(&f.Timeout, "timeout", 30*time.Second, "per-step timeout")
	flag.BoolVar(&f.Reset, "reset-state", true, "force-reset demo + NPC before run")
	flag.BoolVar(&f.Verbose, "verbose", false, "print full HTTP/WS traces")
	flag.BoolVar(&f.KeepAlive, "keep-alive", false, "don't close WS on success (for debug)")
	flag.BoolVar(&f.ShowVer, "v", false, "print binary version and exit 0")
	flag.Parse()
	if f.ShowVer {
		fmt.Println("acceptance_1_0 v1.0 (Sprint 12)")
		os.Exit(0)
	}
	if f.APIURL == "" || f.WSURL == "" || f.User == "" || f.NPCID == "" {
		return nil, errors.New("api-url / ws-url / user / npc must be non-empty")
	}
	return f, nil
}

// ----------------------- HTTP / WS helpers -----------------------

type httpClient struct {
	hc     *http.Client
	flags  *flags
}

func newHTTPClient(f *flags) *httpClient {
	return &httpClient{
		hc:    &http.Client{Timeout: f.Timeout},
		flags: f,
	}
}

type apiResp struct {
	status int
	body   []byte
	ctype  string
}

func (c *httpClient) do(ctx context.Context, method, path, token string, payload any) (*apiResp, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal payload: %w", err)
		}
		body = strings.NewReader(string(b))
	}
	req, err := http.NewRequestWithContext(ctx, method, c.flags.APIURL+path, body)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.flags.Verbose {
		fmt.Printf("    > %s %s\n", method, path)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(resp.Body)
	if c.flags.Verbose {
		fmt.Printf("    < HTTP %d (%s, %d bytes)\n", resp.StatusCode, resp.Header.Get("Content-Type"), len(rb))
	}
	return &apiResp{status: resp.StatusCode, body: rb, ctype: resp.Header.Get("Content-Type")}, nil
}

// wsEnvelope mirrors ws-gateway/internal/protocol.Envelope.
type wsEnvelope struct {
	Type    string          `json:"type"`
	TraceID string          `json:"trace_id"`
	TsMs    int64           `json:"ts_ms"`
	Payload json.RawMessage `json:"payload"`
}

// wsReader 持有一个 WS 连接 + envelope channel。
// run() 阻塞读 WS → envelope 投到 ch；main goroutine 用 select+timeout 消费。
type wsReader struct {
	conn    *websocket.Conn
	ch      chan wsEnvelope
	cancel  context.CancelFunc
	stopped chan struct{}
}

func openWS(ctx context.Context, wsURL, token string, flags *flags) (*wsReader, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("parse ws url: %w", err)
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()

	dialCtx, cancel := context.WithCancel(ctx)
	conn, _, err := websocket.Dial(dialCtx, u.String(), nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	r := &wsReader{
		conn:    conn,
		ch:      make(chan wsEnvelope, 64),
		cancel:  cancel,
		stopped: make(chan struct{}),
	}
	go r.run(dialCtx)
	return r, nil
}

func (r *wsReader) run(ctx context.Context) {
	defer close(r.stopped)
	for {
		msgType, data, err := r.conn.Read(ctx)
		if err != nil {
			if flags_v != nil && flags_v.Verbose {
				fmt.Printf("    ws read err (msgType=%d): %v\n", msgType, err)
			}
			return
		}
		var env wsEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			if flags_v != nil && flags_v.Verbose {
				fmt.Printf("    ws decode err: %v (data=%q)\n", err, string(data))
			}
			continue
		}
		if flags_v != nil && flags_v.Verbose {
			fmt.Printf("    ws recv: type=%s payload=%d bytes\n", env.Type, len(env.Payload))
		}
		select {
		case r.ch <- env:
		case <-ctx.Done():
			return
		}
	}
}

// flags_v is a package-level mirror of the last parsed flags so wsReader.run
// can use it without changing the helper signature. Set once in main() before
// openWS is called. Not thread-safe by design; assigned once at startup.
var flags_v *flags

func (r *wsReader) close() error {
	r.cancel()
	err := r.conn.Close(websocket.StatusNormalClosure, "bye")
	<-r.stopped
	return err
}

// waitFor 阻塞等 type 匹配的 envelope；timeout 后返 ErrStepTimeout。
// 一次性消费：找到就返；超时返错；其它类型 envelope 走 _ 跳过。
//
// ⚠️ 调用方责任：在每个 step 的 HTTP 动作之前 drainStale()，否则 setup 阶段
// 触发的 player_moved 会污染 step 2 / step 3 的断言（reset_player 移到 tile_-1_0
// 而非期望的 tile_1_0 / tile_0_0）。
func (r *wsReader) waitFor(ctx context.Context, wantType string, timeout time.Duration) (wsEnvelope, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case env := <-r.ch:
			if env.Type == wantType {
				return env, nil
			}
			// 其它类型跳过（player_moved/npc_moved 也常见）
		case <-deadline.C:
			return wsEnvelope{}, ErrStepTimeout
		case <-ctx.Done():
			return wsEnvelope{}, ctx.Err()
		}
	}
}

// drainStale 把已经在 channel 里堆着的 envelope 全部丢弃——典型场景：上一步
// 触发的 player_moved 已被 run() 推到 ch 但还没被消费（HTTP 200 已返、
// waitFor 还没被调，或被另一个 envelope 抢先）。
//
// 用一个很短的超时（50ms）从 ch 里 non-blocking 地抽空：没有 envelope 时
// 立刻返回；有时抽光为止。
func (r *wsReader) drainStale() {
	t := time.NewTimer(50 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-r.ch:
			// 丢
		case <-t.C:
			return
		}
	}
}

// ----------------------- Errors -----------------------

var (
	ErrSetupFail    = errors.New("setup failed")
	ErrStepTimeout  = errors.New("step timeout")
	ErrStepFail     = errors.New("step failed")
	ErrBadCLI       = errors.New("bad cli args")
	ErrWSReadFailed = errors.New("ws read failed")
)

// ----------------------- 5 steps -----------------------

type state struct {
	f       *flags
	http    *httpClient
	ws      *wsReader
	token   string
	pid     string
}

func runAcceptance(ctx context.Context, f *flags) error {
	flags_v = f
	s := &state{f: f, http: newHTTPClient(f)}

	// ----- 准备阶段：login + WS connect + reset state -----
	token, pid, err := step0Login(ctx, s)
	if err != nil {
		return err
	}
	s.token, s.pid = token, pid

	ws, err := openWS(ctx, f.WSURL+"/ws", token, f)
	if err != nil {
		fmt.Printf("[FAIL] setup ws_dial: %v\n", err)
		fmt.Println("   hint: ws-gateway 未起来或 REDIS_URL 配错；docker compose ps ws-gateway")
		return fmt.Errorf("%w: ws dial: %v", ErrSetupFail, err)
	}
	s.ws = ws
	if !f.KeepAlive {
		defer func() { _ = ws.close() }()
	}

	if f.Reset {
		// demo → tile_-1_0 (-50, 50)；王老板 → tile_0_0 (50, 50)
		if err := resetPlayer(ctx, s, -50, 50); err != nil {
			fmt.Printf("[FAIL] setup reset_player: %v\n", err)
			return fmt.Errorf("%w: reset_player: %v", ErrSetupFail, err)
		}
		if err := resetNPC(ctx, s, 50, 50); err != nil {
			fmt.Printf("[FAIL] setup reset_npc: %v\n", err)
			return fmt.Errorf("%w: reset_npc: %v", ErrSetupFail, err)
		}
		fmt.Println("[OK] setup: login + ws_dial + reset demo + reset NPC")
	}

	// ----- 5 steps -----
	steps := []struct {
		name string
		fn   func(context.Context, *state) error
	}{
		{"1 login", step1Login},
		{"2 walk", step2Walk},
		{"3 npc_approach", step3Approach},
		{"4 npc_say", step4Say},
		{"5 player_reply", step5Reply},
	}
	for _, st := range steps {
		if err := st.fn(ctx, s); err != nil {
			return fmt.Errorf("%w: step %s: %v", ErrStepFail, st.name, err)
		}
	}
	return nil
}

// step0Login: 准备阶段 — POST /v1/auth/login 拿 token + player_id。
func step0Login(ctx context.Context, s *state) (string, string, error) {
	r, err := s.http.do(ctx, "POST", "/v1/auth/login", "", map[string]string{
		"username": s.f.User,
		"password": s.f.Password,
	})
	if err != nil {
		fmt.Printf("[FAIL] setup login: %v\n", err)
		fmt.Println("   hint: api-gateway 未起来或 DB 不可达；docker compose ps")
		return "", "", fmt.Errorf("%w: login: %v", ErrSetupFail, err)
	}
	if r.status != 200 {
		fmt.Printf("[FAIL] setup login: HTTP %d body=%s\n", r.status, string(r.body))
		if r.status == 401 {
			fmt.Println("   hint: demo 用户未 seed；docker compose down -v 后 up 重建 initdb")
		} else if r.status >= 500 {
			fmt.Println("   hint: api-gateway 未起来或 DB 不可达；docker compose ps")
		}
		return "", "", fmt.Errorf("%w: login HTTP %d", ErrSetupFail, r.status)
	}
	var login struct {
		Token    string `json:"token"`
		PlayerID string `json:"player_id"`
	}
	if err := json.Unmarshal(r.body, &login); err != nil {
		return "", "", fmt.Errorf("decode login: %w", err)
	}
	if len(login.Token) < 100 {
		return "", "", fmt.Errorf("token too short (len=%d, expect HS256 3 段)", len(login.Token))
	}
	if len(login.PlayerID) < 32 {
		return "", "", fmt.Errorf("player_id not UUID-shaped: %q", login.PlayerID)
	}
	return login.Token, login.PlayerID, nil
}

// resetPlayer: POST /v1/world/move to (-50, 50) → tile_-1_0
func resetPlayer(ctx context.Context, s *state, x, y float32) error {
	_, err := s.http.do(ctx, "POST", "/v1/world/move", s.token, map[string]any{
		"player_id":   s.pid,
		"to_tile_id":  "tile_-1_0",
		"x":           x,
		"y":           y,
	})
	return err
}

// resetNPC: POST /v1/npc/:id/position → (50, 50)
func resetNPC(ctx context.Context, s *state, x, y float32) error {
	r, err := s.http.do(ctx, "POST", "/v1/npc/"+s.f.NPCID+"/position", s.token, map[string]float32{
		"x": x, "y": y,
	})
	if err != nil {
		return err
	}
	if r.status != 200 {
		return fmt.Errorf("HTTP %d body=%s", r.status, string(r.body))
	}
	return nil
}

// step1Login: 复用 0 阶段 token；此步骤仅 sanity check "我们已经在登录态"。
func step1Login(ctx context.Context, s *state) error {
	if s.token == "" || s.pid == "" {
		return fmt.Errorf("token or player_id empty (setup bug)")
	}
	fmt.Println("[OK] step 1 login: token valid + player_id UUID")
	return nil
}

// step2Walk: 移到 tile_1_0 (150, 50)；等 WS player_moved envelope。
func step2Walk(ctx context.Context, s *state) error {
	s.ws.drainStale() // 丢掉 setup 阶段（reset_player）的 stale envelope
	r, err := s.http.do(ctx, "POST", "/v1/world/move", s.token, map[string]any{
		"player_id":  s.pid,
		"to_tile_id": "tile_1_0",
		"x":          150.0,
		"y":          50.0,
	})
	if err != nil {
		return err
	}
	if r.status != 200 {
		fmt.Printf("[FAIL] step 2 walk: HTTP %d body=%s\n", r.status, string(r.body))
		if r.status == 401 {
			fmt.Println("   hint: token 失效或 player_id 不匹配；重跑")
		}
		return fmt.Errorf("HTTP %d", r.status)
	}

	env, err := s.ws.waitFor(ctx, "player_moved", s.f.Timeout)
	if err != nil {
		fmt.Printf("[FAIL] step 2 walk: ws wait_for player_moved: %v\n", err)
		fmt.Println("   hint: ws-gateway 未订阅 aicity:player:moved；查 REDIS_CHANNELS env 或 ws-gateway 配置")
		return err
	}
	var p struct {
		PlayerID string  `json:"player_id"`
		TileID   string  `json:"tile_id"`
		X        float32 `json:"x"`
		Y        float32 `json:"y"`
	}
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		return fmt.Errorf("decode player_moved payload: %w", err)
	}
	if p.PlayerID != s.pid {
		return fmt.Errorf("player_id mismatch: ws=%q expect=%q", p.PlayerID, s.pid)
	}
	if p.TileID != "tile_1_0" {
		return fmt.Errorf("tile_id mismatch: ws=%q expect=tile_1_0", p.TileID)
	}
	if absDiff(p.X, 150.0) > 0.01 || absDiff(p.Y, 50.0) > 0.01 {
		return fmt.Errorf("coord mismatch: ws=(%v,%v) expect=(150,50)", p.X, p.Y)
	}
	fmt.Printf("[OK] step 2 walk: tile_1_0 + ws player_moved envelope (%v,%v)\n", p.X, p.Y)
	return nil
}

// step3Approach: 移到 tile_0_0 (50,50)；验 GET /v1/tiles 含 player + npc。
func step3Approach(ctx context.Context, s *state) error {
	r, err := s.http.do(ctx, "POST", "/v1/world/move", s.token, map[string]any{
		"player_id":  s.pid,
		"to_tile_id": "tile_0_0",
		"x":          50.0,
		"y":          50.0,
	})
	if err != nil {
		return err
	}
	if r.status != 200 {
		fmt.Printf("[FAIL] step 3 npc_approach: move HTTP %d body=%s\n", r.status, string(r.body))
		return fmt.Errorf("move HTTP %d", r.status)
	}

	// 等 agent-os tick（≤ 1s）+ Redis 写完，再查 tiles。
	time.Sleep(1100 * time.Millisecond)

	r, err = s.http.do(ctx, "GET", "/v1/tiles", s.token, nil)
	if err != nil {
		return err
	}
	if r.status != 200 {
		fmt.Printf("[FAIL] step 3 npc_approach: GET /v1/tiles HTTP %d body=%s\n", r.status, string(r.body))
		return fmt.Errorf("tiles HTTP %d", r.status)
	}
	var tiles []struct {
		ID        string   `json:"id"`
		PlayerIDs []string `json:"player_ids"`
		NPCIDs    []string `json:"npc_ids"`
	}
	if err := json.Unmarshal(r.body, &tiles); err != nil {
		return fmt.Errorf("decode tiles: %w", err)
	}
	var target *struct {
		ID        string   `json:"id"`
		PlayerIDs []string `json:"player_ids"`
		NPCIDs    []string `json:"npc_ids"`
	}
	for i := range tiles {
		if tiles[i].ID == "tile_0_0" {
			target = &tiles[i]
			break
		}
	}
	if target == nil {
		fmt.Printf("[FAIL] step 3 npc_approach: tile_0_0 不在 /v1/tiles 列表\n")
		return fmt.Errorf("tile_0_0 missing")
	}
	if !contains(target.PlayerIDs, s.pid) {
		fmt.Printf("[FAIL] step 3 npc_approach: tile_0_0.player_ids=%v 不含 player %s\n", target.PlayerIDs, s.pid)
		fmt.Println("   hint: demo 移到错误 tile；-verbose 看 HTTP body")
		return fmt.Errorf("player not in tile_0_0")
	}
	if !contains(target.NPCIDs, s.f.NPCID) {
		fmt.Printf("[FAIL] step 3 npc_approach: tile_0_0.npc_ids=%v 不含 %s\n", target.NPCIDs, s.f.NPCID)
		fmt.Println("   hint: 王老板没 seed；docker compose down -v 后 up")
		return fmt.Errorf("npc not in tile_0_0")
	}
	fmt.Printf("[OK] step 3 npc_approach: tile_0_0 含 player + %s\n", s.f.NPCID)
	return nil
}

// step4Say: 等 agent-os 主动 say 的 npc_dialogue envelope。
//
// agent-os SayScheduler 每 SAY_TICK_SECONDS（默认 5s）触发一次，对 enabled
// NPC 选 greeting → publish aicity:npc_dialogue。本步骤最长等 1 个 tick
// + 推送 50ms 容差 = 6s。
func step4Say(ctx context.Context, s *state) error {
	s.ws.drainStale() // 丢 step 3 触发的 player_moved 残留
	wait := s.f.Timeout
	if wait > 7*time.Second {
		wait = 7 * time.Second
	}
	env, err := s.ws.waitFor(ctx, "npc_dialogue", wait)
	if err != nil {
		fmt.Printf("[FAIL] step 4 npc_say: ws wait_for npc_dialogue: %v\n", err)
		if errors.Is(err, ErrStepTimeout) {
			fmt.Println("   hint: agent-os tick 正常但未触发 say；查 npc-templates 触发条件")
			fmt.Println("   hint: 或检查 ws-gateway 是否订阅了 aicity:npc_dialogue 频道（REDIS_CHANNELS env）")
			fmt.Println("   hint: 或检查 agent-os 是否真在订阅")
		}
		return err
	}
	var d struct {
		NpcID   string `json:"npc_id"`
		Say     string `json:"say"`
		Options []struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		} `json:"options"`
	}
	if err := json.Unmarshal(env.Payload, &d); err != nil {
		return fmt.Errorf("decode npc_dialogue: %w", err)
	}
	if d.NpcID != s.f.NPCID {
		return fmt.Errorf("npc_id mismatch: ws=%q expect=%q", d.NpcID, s.f.NPCID)
	}
	if d.Say == "" {
		return fmt.Errorf("npc_dialogue.say is empty")
	}
	fmt.Printf("[OK] step 4 npc_say: %s 说 %q（%d 选项）\n", d.NpcID, d.Say, len(d.Options))
	return nil
}

// step5Reply: POST /v1/npc/:id/talk 取 reply + options。
func step5Reply(ctx context.Context, s *state) error {
	choiceID := "ask_food" // 与 wang_boss.yaml talk_tree root.options 对齐
	r, err := s.http.do(ctx, "POST", "/v1/npc/"+s.f.NPCID+"/talk", s.token, map[string]string{
		"player_id": s.pid,
		"choice_id": choiceID,
	})
	if err != nil {
		return err
	}
	if r.status == 404 {
		fmt.Printf("[FAIL] step 5 player_reply: HTTP 404 — POST /v1/npc/.../talk endpoint not implemented (1.0 MUST)\n")
		return errors.New("endpoint not implemented")
	}
	if r.status != 200 {
		fmt.Printf("[FAIL] step 5 player_reply: HTTP %d body=%s\n", r.status, string(r.body))
		return fmt.Errorf("HTTP %d", r.status)
	}
	var reply struct {
		Say     string `json:"say"`
		Options []struct {
			ID string `json:"id"`
		} `json:"options"`
	}
	if err := json.Unmarshal(r.body, &reply); err != nil {
		return fmt.Errorf("decode reply: %w", err)
	}
	if reply.Say == "" {
		fmt.Println("[FAIL] step 5 player_reply: body.say 为空")
		fmt.Println("   hint: npc-templates 缺 talk_tree 字段或 choice 不匹配")
		return fmt.Errorf("empty reply")
	}
	fmt.Printf("[OK] step 5 player_reply: NPC 回 %q（%d 选项）\n", reply.Say, len(reply.Options))
	return nil
}

// ----------------------- Main -----------------------

func main() {
	f, err := parseFlags()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad cli args: %v\n", err)
		os.Exit(3)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	err = runAcceptance(ctx, f)
	dur := time.Since(start)

	switch {
	case err == nil:
		fmt.Printf("\n[OK] all 5 acceptance_1_0 checks passed in %s\n", dur.Round(time.Millisecond))
		os.Exit(0)
	case errors.Is(err, ErrSetupFail):
		fmt.Fprintf(os.Stderr, "\n[FAIL] setup failed in %s\n", dur.Round(time.Millisecond))
		os.Exit(2)
	case errors.Is(err, ErrBadCLI):
		fmt.Fprintf(os.Stderr, "\n[FAIL] bad cli args\n")
		os.Exit(3)
	default:
		fmt.Fprintf(os.Stderr, "\n[FAIL] step failed in %s\n", dur.Round(time.Millisecond))
		os.Exit(1)
	}
}

// ----------------------- utils -----------------------

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func absDiff(a, b float32) float32 {
	if a > b {
		return a - b
	}
	return b - a
}

// keep imports referenced even if some are only used in helpers above
var (
	_ = sync.Mutex{}
)
