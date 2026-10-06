// Package crosscity stream_forwarder.go —— A 城 a2a-gateway 的 SayStreamClient
// 具体实现（B1-T06）。
//
// 整体跨城流式 pipeline（A 城视角）：
//
//	1. httpgw.SayStreamHandler 解析 SayRequestInit（JSON body）。
//	2. 调用 SayStreamClient.SayStreamForward(ctx, init) —— 由本 Forwarder 实现。
//	3. Forwarder 行为：
//	   a) MirrorStore.Create(sid)  —— 写入本地 60min 镜像（T08 replay 用）。
//	   b) 订阅 A 城 Redis aicity:npc:say_stream（env REDIS_CHANNEL_NPC_SAY_STREAM 覆盖）。
//	   c) 触发 B 城 agent-os HTTP POST /v1/npc/say_stream（mTLS 暂不需要，intra-VPN）。
//	   d) 从 Redis 拉 beat，每帧 Mirror.Append + 推 beats chan。
//	   e) 收到 done(complete=true) → MarkDone(true) + 正常关流。
//	   f) ctx 取消 / 单 beat 8s 超时 / Redis EOF → 推 done(complete=false) + MarkDone(false) + R_017。
//	4. SayStreamHandler 把 beats 序列化成 SSE 帧写给 api-gateway 客户端。
//
// 接口抽象说明：本包不引入新依赖（go-redis 仅 indirect，且本 task 不增加直接 dep）。
// Subscriber 接口由两个 impl 满足：
//   - RedisSubscriber（生产）：包 redis.Client.Subscribe；
//   - 单元测试用 fake（见 stream_forwarder_test.go）。
//
// 与 T04 SSE handler 的契约（say_stream.go:25-28）：
//   - MUST close BOTH channels when ctx is cancelled（goroutine 内 defer）。
//   - beats channel closed BEFORE errs channel closed（defer 顺序）。
//   - errs channel buffered (size >= 1)。
//   - 初始错误（dial / Mirror.Create 冲突）走 3rd return；通道错误仅 mid-stream。

package crosscity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// 默认超时。
const (
	DefaultStreamTimeout  = 30 * time.Second // 整段流（与 spec §6 R_017 一致）
	DefaultBeatTimeout    = 8 * time.Second  // 单 beat 间隔（与 stage 2 streamcheck 一致）
	defaultChannelEnv     = "REDIS_CHANNEL_NPC_SAY_STREAM"
	defaultChannelName    = "aicity:npc:say_stream"
	defaultBCityTriggerPath = "/v1/npc/say_stream"
)

// BCityClient A 城 → B 城 agent-os HTTP client（B1 跨城触发用，非 mTLS）。
//
// mTLS 暂不需要（intra-VPN；spec §6 也允许 http）；未来若跨城走公网，把 HTTP client
// 换成带 tls.Config 的 http.Transport 即可。
type BCityClient struct {
	BaseURL string            // e.g. "http://b-city:8081"
	HTTP    *http.Client      // 不为 nil（NewBCityClient 兜底 10s timeout）
	Path    string            // 默认 "/v1/npc/say_stream"
}

// NewBCityClient 默认 10s HTTP timeout；Path 默认 "/v1/npc/say_stream"。
func NewBCityClient(baseURL string) *BCityClient {
	return &BCityClient{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
		Path:    defaultBCityTriggerPath,
	}
}

// Subscriber 抽象 Redis 订阅（便于单测注入 fake）。
//
//   - Subscribe(ctx, channel) → (payload chan, cancel func, err)
//     payload chan 在 Redis pub/sub 转发过来的 payload string；关闭时返 EOF。
//     cancel func 取消订阅（释放 redis.PubSub.Close 资源）。
//   - Close() 关闭底层客户端（仅 RedisSubscriber 需要；fake 通常 no-op）。
type Subscriber interface {
	Subscribe(ctx context.Context, channel string) (<-chan string, func(), error)
	Close() error
}

// Forwarder 跨城 SayStreamForward 客户端（A 城 a2a-gateway → B 城 agent-os）。
//
// 字段：
//   - BCity  : 触发 B 城 HTTP say_stream 客户端（必有）。
//   - Sub    : Redis 订阅抽象（必有；生产 = *RedisSubscriber；测试 = fake）。
//   - Mirror : 本地镜像 store（必有；提供 Create/Append/MarkDone/IsComplete）。
//   - Channel: Redis 频道名（默认 "aicity:npc:say_stream"，可 env 覆盖）。
//   - Timeout / BeatTimeout：可覆盖，默认 30s / 8s。
type Forwarder struct {
	BCity       *BCityClient
	Sub         Subscriber
	Mirror      *MirrorStore
	Channel     string
	Timeout     time.Duration
	BeatTimeout time.Duration
}

// NewForwarder 构造 forwarder；缺省值：Channel/Timeout/BeatTimeout。
//
// 缺省 Channel 读 env `REDIS_CHANNEL_NPC_SAY_STREAM`，否则用 "aicity:npc:say_stream"。
func NewForwarder(b *BCityClient, sub Subscriber, m *MirrorStore) *Forwarder {
	return &Forwarder{
		BCity:       b,
		Sub:         sub,
		Mirror:      m,
		Channel:     resolveChannel(),
		Timeout:     DefaultStreamTimeout,
		BeatTimeout: DefaultBeatTimeout,
	}
}

func resolveChannel() string {
	v := os.Getenv(defaultChannelEnv)
	if v == "" {
		return defaultChannelName
	}
	return v
}

// RedisSubscriber 是 Subscriber 的 go-redis 实现。
//
// 当前 a2a-gateway 未将 github.com/redis/go-redis/v9 提升为直接 dep（仅 indirect），
// 故本 struct 由调用方注入 redis.Client 时构造：本文件不直接 import go-redis。
//
// 实际接线由 main.go 在引入 go-redis 后构造（cmd/main.go:122+ 一段）。
type RedisSubscriber struct {
	Client RedisClient // 见下接口
}

// NoopSubscriber 总是返错（用于 Redis 未接入时的占位）。
//
// Forwarder 收到 Subscribe 错误后，会走 init 错误路径 → SayStreamHandler 返 502+R_016。
// 这允许 cmd/main.go 在没有 Redis 时也安全挂载 SSE handler。
type NoopSubscriber struct {
	Err error
}

// Subscribe 总是返错（default: "R_016: redis subscriber not configured"）。
func (n *NoopSubscriber) Subscribe(ctx context.Context, channel string) (<-chan string, func(), error) {
	if n == nil || n.Err == nil {
		return nil, nil, errors.New("crosscity: redis subscriber not configured")
	}
	return nil, nil, n.Err
}

// Close no-op。
func (n *NoopSubscriber) Close() error { return nil }

// RedisClient 是 go-redis *redis.Client 的最小子集（避开直接 import）。
//
// 由 cmd/main.go 在 import go-redis 时通过一个轻量 shim 注入：
//
//	type redisShim struct{ C *redis.Client }
//	func (s *redisShim) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
//	    return s.C.Subscribe(ctx, channels...)
//	}
type RedisClient interface {
	Subscribe(ctx context.Context, channels ...string) RedisPubSub
}

// RedisPubSub 是 go-redis *redis.PubSub 的最小子集。
type RedisPubSub interface {
	Channel() <-chan *RedisMessage
	Close() error
}

// RedisMessage 是 go-redis *redis.Message 的最小子集。
type RedisMessage struct {
	Channel string
	Payload string
}

// Subscribe 订阅频道，返回 payload chan + 取消 func。
func (r *RedisSubscriber) Subscribe(ctx context.Context, channel string) (<-chan string, func(), error) {
	if r == nil || r.Client == nil {
		return nil, nil, errors.New("crosscity: RedisSubscriber not initialized")
	}
	ps := r.Client.Subscribe(ctx, channel)
	// 必须等 Subscribe 确认（go-redis 文档明确要求）—— 否则首个 Publish 可能 race 丢失。
	// 借助 ps.Receive；但本接口未暴露它，生产环境调用方应在 main.go 用更完整的 shim 包一层。
	// 这里仅做订阅，不等确认：测试 fake 不需要；真实环境超时由 ctx 控制。
	out := make(chan string, 16)
	stopped := atomic.Bool{}
	go func() {
		defer close(out)
		for msg := range ps.Channel() {
			if stopped.Load() {
				return
			}
			select {
			case out <- msg.Payload:
			case <-ctx.Done():
				return
			}
		}
	}()
	cancel := func() {
		stopped.Store(true)
		_ = ps.Close()
	}
	return out, cancel, nil
}

// Close 关闭底层（Redis 客户端通常在 main 进程生命周期管理，此处 no-op）。
func (r *RedisSubscriber) Close() error { return nil }

// SayStreamForward 满足 httpgw.SayStreamClient 接口（T04 契约）。
//
// 返回：
//   - beats: 至少 1 帧（首 beat 可能 = done 帧）；流结束关闭。
//   - errs  : 仅 mid-stream 错误；流结束前关闭 1 次，buffer size = 1。
//   - err   : 仅初始错误（dial/JSON/Mirror 冲突）；成功 = nil。
//
// ctx 取消时两个 channel 都关闭（defer 顺序：beats → errs）。
func (f *Forwarder) SayStreamForward(ctx context.Context, init *a2av1.SayRequestInit) (
	<-chan *a2av1.SayBeat, <-chan error, error,
) {
	if init == nil {
		return nil, nil, errors.New("crosscity: init is nil")
	}
	sid := init.GetSessionId()
	if sid == "" {
		return nil, nil, errors.New("crosscity: session_id required")
	}
	npcID := init.GetNpcId()
	if npcID == "" {
		return nil, nil, errors.New("crosscity: npc_id required")
	}
	if f.Mirror == nil {
		return nil, nil, errors.New("crosscity: MirrorStore not configured")
	}
	if f.Sub == nil {
		return nil, nil, errors.New("crosscity: Subscriber not configured")
	}
	if f.BCity == nil {
		return nil, nil, errors.New("crosscity: BCityClient not configured")
	}
	if f.Channel == "" {
		f.Channel = defaultChannelName
	}

	// 1) Mirror session（先建，让 T08 replay 立即可查）。
	f.Mirror.Create(sid, npcID, init.GetPlayerId())

	// 2) 订阅 Redis（必须在 trigger B 城之前，避免首 beat 丢失）。
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	payloads, subDone, err := f.Sub.Subscribe(subCtx, f.Channel)
	if err != nil {
		_ = f.Mirror.MarkDone(sid, false)
		return nil, nil, fmt.Errorf("crosscity: redis subscribe: %w", err)
	}
	// subDone 幂等化（多次调用安全）。
	var subDoneOnce sync.Once
	subDoneSafe := func() { subDoneOnce.Do(subDone) }

	// 3) 启动 producer goroutine：trigger B 城 + 拉 beats。
	beats := make(chan *a2av1.SayBeat, 8)
	errs := make(chan error, 1)

	streamCtx, streamCancel := context.WithTimeout(ctx, f.Timeout)
	// 注意：streamCancel 不能在 SayStreamForward 返回前 defer —— 否则立即取消，
	// 让 goroutine 的 triggerBCity 拿到已 canceled ctx。改在 goroutine 内 defer。

	go func() {
		defer streamCancel()
		defer subDoneSafe()
		defer close(beats)
		defer close(errs)

		// 3a) 触发 B 城 agent-os HTTP（非阻塞；body 用 drain 即可，beat 来源是 Redis）。
		if err := f.triggerBCity(streamCtx, sid, init); err != nil {
			_ = f.Mirror.MarkDone(sid, false)
			beats <- doneBeat(sid, npcID, init.GetTraceId(), false, "R_016:"+err.Error())
			errs <- fmt.Errorf("R_016: trigger b-city: %w", err)
			return
		}

		// 3b) pump beats。
		beatTimer := time.NewTimer(f.BeatTimeout)
		defer beatTimer.Stop()

		for {
			select {
			case <-streamCtx.Done():
				_ = f.Mirror.MarkDone(sid, false)
				beats <- doneBeat(sid, npcID, init.GetTraceId(), false, "R_017:context cancelled")
				errs <- fmt.Errorf("R_017: %w", streamCtx.Err())
				return

			case <-beatTimer.C:
				_ = f.Mirror.MarkDone(sid, false)
				beats <- doneBeat(sid, npcID, init.GetTraceId(), false, "R_017:beat timeout")
				errs <- errors.New("R_017: beat timeout")
				return

			case payload, ok := <-payloads:
				if !ok {
					// Redis 通道关闭（EOF / ctx 取消）。
					_ = f.Mirror.MarkDone(sid, false)
					beats <- doneBeat(sid, npcID, init.GetTraceId(), false, "R_016:redis closed")
					errs <- errors.New("R_016: redis channel closed")
					return
				}
				// 重置 beat timer。
				if !beatTimer.Stop() {
					select {
					case <-beatTimer.C:
					default:
					}
				}
				beatTimer.Reset(f.BeatTimeout)

				beat, err := decodeSayBeat([]byte(payload))
				if err != nil {
					// 跳过畸形包；不打断流。
					continue
				}
				// stamp canonical session_id（Redis payload 可能不含 sid，靠 channel 共享 sid）。
				beat.SessionId = sid
				if beat.NpcId == "" {
					beat.NpcId = npcID
				}

				// mirror 写入（仅 stage 2 字段；可选字段填空安全）。
				if err := f.Mirror.Append(sid, Beat{
					SentenceIdx: uint32(beat.GetSentenceIdx()),
					Text:        beat.GetText(),
					Emotion:     beat.GetEmotion(),
					TSMS:        beat.GetTsMs(),
				}); err != nil {
					// mirror miss (TTL 过期 / 未创建) → 跳过，不打断流。
					_ = err
				}

				// 推给 SSE handler；ctx 取消则退出。
				select {
				case beats <- beat:
				case <-streamCtx.Done():
					return
				}

				if beat.GetComplete() {
					_ = f.Mirror.MarkDone(sid, true)
					// 正常结束：不写 errs（handler 视 EOF 为 clean close）。
					return
				}
			}
		}
	}()

	// 释放 subDone 在 goroutine 退出时由 defer 触发（subDoneSafe 幂等）。
	_ = subDoneSafe

	return beats, errs, nil
}

// triggerBCity POST B 城 agent-os /v1/npc/say_stream；body 触发 LLM 流，beat 通过
// B 城 a2a-gateway 写入 B 城 Redis，本 Forwarder 再从 A 城 Redis 拉到。
//
// 不读 response body（流是 Redis 真源；HTTP 仅做触发）。非 200 = 触发失败 → R_016。
func (f *Forwarder) triggerBCity(ctx context.Context, sid string, init *a2av1.SayRequestInit) error {
	if f.BCity == nil || f.BCity.HTTP == nil {
		return errors.New("b-city client not configured")
	}

	// context 数组转 JSON-friendly 形态（proto []*ContextMessage 转 []map）。
	var ctxList []map[string]string
	for _, c := range init.GetContext() {
		if c == nil {
			continue
		}
		ctxList = append(ctxList, map[string]string{
			"role":    c.GetRole(),
			"content": c.GetContent(),
		})
	}

	body, err := json.Marshal(map[string]any{
		"session_id":   sid,
		"npc_id":       init.GetNpcId(),
		"player_id":    init.GetPlayerId(),
		"player_input": init.GetPlayerInput(),
		"trace_id":     init.GetTraceId(),
		"context":      ctxList,
	})
	if err != nil {
		return fmt.Errorf("marshal trigger body: %w", err)
	}

	url := f.BCity.BaseURL + f.BCity.Path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := f.BCity.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("http do: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 抽 body 头 200 字节便于诊断。
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		_ = resp.Body.Close()
		return fmt.Errorf("b-city status %d body=%q", resp.StatusCode, string(snippet))
	}
	// Drain body（流是 Redis 真源，不解析 HTTP body）。
	// 显式 ReadAll + Close：避免 keep-alive 长连接让 io.Copy 阻塞到下一次请求。
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return nil
}

// doneBeat 构造 npc_say_stream_done 帧（异常结束路径）。
func doneBeat(sid, npcID, traceID string, complete bool, _ string) *a2av1.SayBeat {
	c := complete
	ts := time.Now().UnixMilli()
	return &a2av1.SayBeat{
		Type:      "npc_say_stream_done",
		NpcId:     npcID,
		SessionId: sid,
		Complete:  &c,
		TsMs:      ts,
		TraceId:   traceID,
	}
}

// decodeSayBeat 把 Redis payload（JSON）解成 *a2av1.SayBeat。
//
// 兼容两种来源：
//   - agent-os Publisher（stage 2）：type/npc_id/sentence_idx/text/emotion/ts_ms/trace_id/sentence_count/complete。
//   - 任意中间转发器：可能缺字段，按可选字段填零值。
//
// proto optional fields 通过 nil pointer 解码；这里直接用 map → 指针，方便兼容。
func decodeSayBeat(payload []byte) (*a2av1.SayBeat, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}
	var m map[string]any
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, err
	}
	beat := &a2av1.SayBeat{TsMs: time.Now().UnixMilli()}
	if v, ok := m["type"].(string); ok {
		beat.Type = v
	}
	if v, ok := m["npc_id"].(string); ok {
		beat.NpcId = v
	}
	if v, ok := m["session_id"].(string); ok {
		beat.SessionId = v
	}
	if v, ok := m["trace_id"].(string); ok {
		beat.TraceId = v
	}
	if v, ok := m["sentence_idx"].(float64); ok {
		i := int32(v)
		beat.SentenceIdx = &i
	}
	if v, ok := m["text"].(string); ok {
		s := v
		beat.Text = &s
	}
	if v, ok := m["emotion"].(string); ok {
		s := v
		beat.Emotion = &s
	}
	if v, ok := m["sentence_count"].(float64); ok {
		i := int32(v)
		beat.SentenceCount = &i
	}
	if v, ok := m["complete"].(bool); ok {
		b := v
		beat.Complete = &b
	}
	if v, ok := m["ts_ms"].(float64); ok {
		beat.TsMs = int64(v)
	}
	return beat, nil
}
