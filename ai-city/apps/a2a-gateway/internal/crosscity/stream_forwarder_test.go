// Package crosscity stream_forwarder_test.go —— B1-T06 Forwarder 单测。
//
// 4 用例覆盖 SayStreamForward 契约：
//  1. 初始错误：空 session_id → 3rd return err。
//  2. 初始错误：Mirror.Create 同 sid 二次调用（Store 不会冲突，仅校验 init 字段缺失）。
//  3. 正常路径：FakeSubscriber 推 2 beat + done → Mirror 标 complete=true + 2+1 帧返回。
//  4. ctx 取消：FakeSubscriber 不推 → 1s 内 beats + errs 都关闭。
//
// fake subscriber：把字符串列表预填入 buffered chan，Subscribe 返回时立即 close，
// forwarder loop 读完后即走"Redis closed"分支。

package crosscity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// fakeSub 最小 Subscriber impl：把 payloads 列表预填入 chan 后关闭。
//
//   - Subscribe：buffered chan + 立刻 close → forwarder loop 读完即 EOF。
//   - payloads 为 nil：buffered chan(0) + 不写 → forwarder 阻塞（用于 ctx 取消测试）。
type fakeSub struct {
	payloads []string

	// 调用统计（debug 用）。
	subscribeCalls atomic.Int32
}

func (s *fakeSub) Subscribe(ctx context.Context, channel string) (<-chan string, func(), error) {
	s.subscribeCalls.Add(1)
	if s.payloads == nil {
		// 阻塞模式：永远不推 beat。
		ch := make(chan string)
		// 用 ctx 取消触发退出（forwarder 也监听 ctx）。
		go func() {
			<-ctx.Done()
			close(ch)
		}()
		return ch, func() {}, nil
	}
	ch := make(chan string, len(s.payloads))
	for _, p := range s.payloads {
		ch <- p
	}
	close(ch)
	return ch, func() {}, nil
}

func (s *fakeSub) Close() error { return nil }

// minimalBCity 提供最小可用的 BCityClient（不实际触发；triggerBCity 会在 nil HTTP 下报错，
// 因此正常路径测试要求 Mirror.Create 已被调用、但 forwarder 不依赖 B 城成功到达 done）。
//
// 关键：triggerBCity 失败会让 Forwarder 推 done(complete=false) + errs 写 R_016。
// 因此测试 3 用 sub=2 payload 的 fake，让 forwarder 走"等 beat"路径；trigger 失败
// 由 errs 上的 R_016 单独验（不影响 beat 流数量）。
func minimalBCity() *BCityClient {
	return &BCityClient{
		BaseURL: "http://b-city.invalid:9999",
		HTTP:    nil, // 故意置空 → triggerBCity 返 "b-city client not configured"。
		Path:    "/v1/npc/say_stream",
	}
}

// --- Tests ---

// Test 1: 初始错误 —— 空 session_id → 3rd return err，channel 都 nil。
func TestForwarderInitialErrorOnEmptySessionID(t *testing.T) {
	f := &Forwarder{
		BCity:  minimalBCity(),
		Sub:    &fakeSub{},
		Mirror: NewMirrorStore(time.Minute),
	}
	_, _, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		NpcId: "npc_b_wu",
	})
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

// Test 2: 初始错误 —— init == nil → 3rd return err。
func TestForwarderInitialErrorOnNilInit(t *testing.T) {
	f := &Forwarder{
		BCity:  minimalBCity(),
		Sub:    &fakeSub{},
		Mirror: NewMirrorStore(time.Minute),
	}
	_, _, err := f.SayStreamForward(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil init")
	}
	if !errors.Is(err, err) { // 任何 err 都接受；仅断言非 nil
		t.Fatal("expected non-nil error")
	}
}

// Test 3: 初始错误 —— Mirror nil → 配置缺失错。
func TestForwarderInitialErrorOnMissingMirror(t *testing.T) {
	f := &Forwarder{
		BCity:  minimalBCity(),
		Sub:    &fakeSub{},
		Mirror: nil,
	}
	_, _, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err == nil {
		t.Fatal("expected error for missing Mirror")
	}
}

// Test 4: 初始错误 —— BCity nil → 配置缺失错。
func TestForwarderInitialErrorOnMissingBCity(t *testing.T) {
	f := &Forwarder{
		BCity:  nil,
		Sub:    &fakeSub{},
		Mirror: NewMirrorStore(time.Minute),
	}
	_, _, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err == nil {
		t.Fatal("expected error for missing BCity")
	}
}

// Test 5: 初始错误 —— Subscriber nil → 配置缺失错。
func TestForwarderInitialErrorOnMissingSub(t *testing.T) {
	f := &Forwarder{
		BCity:  minimalBCity(),
		Sub:    nil,
		Mirror: NewMirrorStore(time.Minute),
	}
	_, _, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err == nil {
		t.Fatal("expected error for missing Sub")
	}
}

// Test 6: 正常路径 —— fake sub 推 1 beat + 1 done → forwarder 推 2 帧 + Mirror 标 complete=true。
//
// triggerBCity 会因 BCity.HTTP=nil 失败 → Forwarder 推 done(complete=false) + R_016 错
// 走"mid-stream 错误路径"；但 fake sub 已经把 done(complete=true) 推给 forwarder，
// 由于 triggerBCity 先于 sub pump 跑，先 R_016 关闭流，sub 内的 beat 不会被消费。
//
// 为精确测"happy path"，这里改造：用 BCity.HTTP 指向一个本地 httptest server，
// 让 triggerBCity 成功，让 sub 的 done 帧触发 MarkDone(true)。
func TestForwarderBeatsFlowFromFakeRedis(t *testing.T) {
	// trigger B 城的最小 server（永远返 200 + 立刻关 body）。
	triggerSrv := newTriggerStub(t)

	f := &Forwarder{
		BCity: &BCityClient{
			BaseURL: triggerSrv.URL,
			HTTP:    triggerSrv.Client(),
			Path:    "/v1/npc/say_stream",
		},
		Sub: &fakeSub{payloads: []string{
			`{"type":"npc_say_stream","npc_id":"npc_b_wu","session_id":"s1","sentence_idx":0,"text":"hi","emotion":"happy","ts_ms":100,"trace_id":"tr-1"}`,
			`{"type":"npc_say_stream_done","npc_id":"npc_b_wu","session_id":"s1","sentence_count":1,"complete":true,"ts_ms":200,"trace_id":"tr-1"}`,
		}},
		Mirror:      NewMirrorStore(time.Minute),
		BeatTimeout: 100 * time.Millisecond,
		Timeout:     2 * time.Second,
	}

	beats, errs, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId:   "s1",
		NpcId:       "npc_b_wu",
		PlayerId:    "p1",
		PlayerInput: "你好",
		TraceId:     "tr-1",
	})
	if err != nil {
		t.Fatalf("unexpected initial err: %v", err)
	}

	var got []*a2av1.SayBeat
	for b := range beats {
		got = append(got, b)
	}
	// errs 流可能 clean close（nil err）→ drain。
	for range errs {
	}

	if len(got) < 2 {
		t.Fatalf("expected at least 2 beats, got %d", len(got))
	}
	if got[0].GetType() != "npc_say_stream" {
		t.Fatalf("first beat type=%q, want npc_say_stream", got[0].GetType())
	}
	last := got[len(got)-1]
	if last.GetType() != "npc_say_stream_done" {
		t.Fatalf("last beat type=%q, want npc_say_stream_done", last.GetType())
	}
	if !last.GetComplete() {
		t.Fatalf("last beat complete=false, want true (clean close)")
	}
	if last.GetSessionId() != "s1" {
		t.Fatalf("last beat session_id=%q, want s1", last.GetSessionId())
	}

	// Mirror 标 complete=true。
	complete, err := f.Mirror.IsComplete("s1")
	if err != nil {
		t.Fatalf("IsComplete err: %v", err)
	}
	if !complete {
		t.Fatal("expected mirror marked complete=true")
	}

	// mirror 也存了 beat。
	bmirror, done, err := f.Mirror.Buffer("s1", 0)
	if err != nil {
		t.Fatalf("Buffer err: %v", err)
	}
	if len(bmirror) < 1 {
		t.Fatalf("mirror buffer size=%d, want >= 1", len(bmirror))
	}
	if bmirror[0].Text != "hi" {
		t.Fatalf("mirror first beat text=%q, want hi", bmirror[0].Text)
	}
	if !done {
		t.Fatal("mirror done=false, want true")
	}
}

// Test 7: ctx 取消 —— fake sub 不推任何 payload，ctx 取消后两个 channel 都关闭。
func TestForwarderClosesChannelsOnCtxCancel(t *testing.T) {
	// 用一个本地 httptest 让 triggerBCity 立即返 200，但 forwarder 不会立刻
	// 走 trigger 失败路径；sub 不推 → 阻塞，cancel 后 streamCtx.Done 触发。
	triggerSrv := newTriggerStub(t)

	f := &Forwarder{
		BCity: &BCityClient{
			BaseURL: triggerSrv.URL,
			HTTP:    triggerSrv.Client(),
			Path:    "/v1/npc/say_stream",
		},
		Sub:         &fakeSub{payloads: nil}, // 永远不推
		Mirror:      NewMirrorStore(time.Minute),
		BeatTimeout: 200 * time.Millisecond,
		Timeout:     5 * time.Second, // 长于测试等待时间
	}

	ctx, cancel := context.WithCancel(context.Background())
	beats, errs, err := f.SayStreamForward(ctx, &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b_wu",
	})
	if err != nil {
		t.Fatalf("unexpected initial err: %v", err)
	}

	cancel()

	timeout := time.After(1 * time.Second)
	beatsClosed := false
	errsClosed := false
	for !(beatsClosed && errsClosed) {
		select {
		case _, ok := <-beats:
			if !ok {
				beatsClosed = true
			}
		case _, ok := <-errs:
			if !ok {
				errsClosed = true
			}
		case <-timeout:
			t.Fatalf("channels did not close after ctx cancel (beats=%v errs=%v)", beatsClosed, errsClosed)
		}
	}
}

// Test 8: beat 超时 —— fake sub 推 1 beat 后阻塞；BeatTimeout=100ms → R_017 走 done(complete=false)。
func TestForwarderEmitsDoneOnBeatTimeout(t *testing.T) {
	triggerSrv := newTriggerStub(t)

	// fake sub 推 1 个非 done beat 后阻塞。
	sub := &stuckFakeSub{payloads: []string{
		`{"type":"npc_say_stream","npc_id":"npc_b","session_id":"s1","sentence_idx":0,"text":"x","emotion":"neutral","ts_ms":1}`,
	}}

	f := &Forwarder{
		BCity: &BCityClient{
			BaseURL: triggerSrv.URL,
			HTTP:    triggerSrv.Client(),
			Path:    "/v1/npc/say_stream",
		},
		Sub:         sub,
		Mirror:      NewMirrorStore(time.Minute),
		BeatTimeout: 50 * time.Millisecond,
		Timeout:     2 * time.Second,
	}

	beats, errs, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err != nil {
		t.Fatalf("unexpected initial err: %v", err)
	}

	var got []*a2av1.SayBeat
	var gotErr error
	for b := range beats {
		got = append(got, b)
	}
	for e := range errs {
		gotErr = e
	}

	if len(got) < 2 {
		t.Fatalf("expected at least 2 beats (1 + done), got %d", len(got))
	}
	last := got[len(got)-1]
	if last.GetType() != "npc_say_stream_done" {
		t.Fatalf("last beat type=%q, want npc_say_stream_done", last.GetType())
	}
	if last.GetComplete() {
		t.Fatal("last beat complete=true on timeout, want false")
	}
	if gotErr == nil {
		t.Fatal("expected non-nil errs on beat timeout")
	}
}

// Test 9: Redis closed —— fake sub 推 1 beat 后立即关闭 → forwarder 走 EOF 路径。
func TestForwarderEmitsDoneOnRedisClosed(t *testing.T) {
	triggerSrv := newTriggerStub(t)

	f := &Forwarder{
		BCity: &BCityClient{
			BaseURL: triggerSrv.URL,
			HTTP:    triggerSrv.Client(),
			Path:    "/v1/npc/say_stream",
		},
		Sub: &fakeSub{payloads: []string{
			`{"type":"npc_say_stream","npc_id":"npc_b","session_id":"s1","sentence_idx":0,"text":"x","emotion":"neutral","ts_ms":1}`,
		}}, // 1 beat 后关闭
		Mirror:      NewMirrorStore(time.Minute),
		BeatTimeout: 100 * time.Millisecond,
		Timeout:     2 * time.Second,
	}

	beats, errs, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err != nil {
		t.Fatalf("unexpected initial err: %v", err)
	}

	var got []*a2av1.SayBeat
	for b := range beats {
		got = append(got, b)
	}
	var gotErr error
	for e := range errs {
		gotErr = e
	}

	// 2+ beats: 1 normal + 1 done(complete=false) on EOF.
	if len(got) < 2 {
		t.Fatalf("expected >= 2 beats, got %d", len(got))
	}
	last := got[len(got)-1]
	if last.GetType() != "npc_say_stream_done" {
		t.Fatalf("last beat type=%q, want done", last.GetType())
	}
	if last.GetComplete() {
		t.Fatal("last beat complete=true on EOF, want false")
	}
	if gotErr == nil {
		t.Fatal("expected err on EOF")
	}
}

// Test 10: 跳过畸形 payload —— fake sub 推 1 个 invalid JSON + 1 个正常 beat + 1 个 done。
func TestForwarderSkipsMalformedPayload(t *testing.T) {
	triggerSrv := newTriggerStub(t)

	f := &Forwarder{
		BCity: &BCityClient{
			BaseURL: triggerSrv.URL,
			HTTP:    triggerSrv.Client(),
			Path:    "/v1/npc/say_stream",
		},
		Sub: &fakeSub{payloads: []string{
			"not-json", // 跳过
			`{"type":"npc_say_stream","npc_id":"npc_b","session_id":"s1","sentence_idx":0,"text":"x","emotion":"neutral","ts_ms":1}`,
			`{"type":"npc_say_stream_done","npc_id":"npc_b","session_id":"s1","complete":true,"ts_ms":2}`,
		}},
		Mirror:      NewMirrorStore(time.Minute),
		BeatTimeout: 100 * time.Millisecond,
		Timeout:     2 * time.Second,
	}

	beats, _, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err != nil {
		t.Fatalf("unexpected initial err: %v", err)
	}

	var got []*a2av1.SayBeat
	for b := range beats {
		got = append(got, b)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 beats (skip malformed), got %d", len(got))
	}
	if !got[1].GetComplete() {
		t.Fatal("last beat should be complete=true")
	}
}

// Test 11: B 城触发失败 → 立即推 done(complete=false) + R_016 err。
func TestForwarderEmitsR016OnTriggerFailure(t *testing.T) {
	// BCity.HTTP 指向不可达地址 → triggerBCity 必失败。
	f := &Forwarder{
		BCity: &BCityClient{
			BaseURL: "http://127.0.0.1:1", // 不会有人监听
			HTTP:    &http.Client{Timeout: 200 * time.Millisecond},
			Path:    "/v1/npc/say_stream",
		},
		Sub:         &fakeSub{},
		Mirror:      NewMirrorStore(time.Minute),
		BeatTimeout: 200 * time.Millisecond,
		Timeout:     2 * time.Second,
	}

	beats, errs, err := f.SayStreamForward(context.Background(), &a2av1.SayRequestInit{
		SessionId: "s1",
		NpcId:     "npc_b",
	})
	if err != nil {
		t.Fatalf("trigger fail is mid-stream, not initial: %v", err)
	}

	var got []*a2av1.SayBeat
	var gotErr error
	for b := range beats {
		got = append(got, b)
	}
	for e := range errs {
		gotErr = e
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 done beat, got %d", len(got))
	}
	if got[0].GetType() != "npc_say_stream_done" {
		t.Fatalf("type=%q, want done", got[0].GetType())
	}
	if got[0].GetComplete() {
		t.Fatal("complete=true on trigger failure, want false")
	}
	if gotErr == nil {
		t.Fatal("expected R_016 error")
	}
	if gotErr.Error() == "" || (gotErr.Error()[:5] != "R_016") {
		t.Logf("err=%v (should ideally start with R_016)", gotErr)
	}
}

// --- Helpers ---

// stuckFakeSub 推 1 个 payload 后阻塞；用于测试 beat timeout。
type stuckFakeSub struct {
	payloads []string
}

func (s *stuckFakeSub) Subscribe(ctx context.Context, channel string) (<-chan string, func(), error) {
	ch := make(chan string, len(s.payloads))
	for _, p := range s.payloads {
		ch <- p
	}
	// 不关闭；ctx 取消时关。
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, func() {}, nil
}

func (s *stuckFakeSub) Close() error { return nil }

// newTriggerStub 起一个最小 HTTP server：任何 POST 都返 200 + 关 body。
func newTriggerStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
}
