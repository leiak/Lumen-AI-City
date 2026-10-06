// Service.SayStreamForward 单元测试（B1-T03）—— 验证 B 城 a2a-gateway
// 收到非法 init 帧时正确返回 InvalidArgument；happy path 仅 smoke 校验
// （done 帧 + mirror 写入）；完整双工往返交给 T09 acceptance_2_2 e2e。
//
// 复用 service_test.go 的 newSignedService() 构造最小 Service；
// SayStreamForward 不依赖 reg/verifier/dispatcher/inbox/acl ——
// 任何 nil 都安全（实现里 nil-check 完整）。
package a2asrv

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aicity/a2a-gateway/internal/crosscity"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockBiDiStream 嵌入 A2AGateway_SayStreamForwardServer 接口，
// 只实现 Recv / Send / Context 三个方法即可走通测试驱动。
// 其它接口方法（SetHeader / SendHeader / SetTrailer / SendMsg / RecvMsg / XXX_NoUnkeyedLength）
// 由嵌入的 nil 接口零值兜底，调用即 panic —— 表明测试覆盖未到位。
type mockBiDiStream struct {
	a2av1.A2AGateway_SayStreamForwardServer
	in  chan *a2av1.SayStreamMessage
	out []*a2av1.SayBeat
	ctx context.Context
}

func newMockBiDiStream() *mockBiDiStream {
	return &mockBiDiStream{
		in:  make(chan *a2av1.SayStreamMessage, 4),
		out: nil,
		ctx: context.Background(),
	}
}

func (m *mockBiDiStream) Recv() (*a2av1.SayStreamMessage, error) {
	msg, ok := <-m.in
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

func (m *mockBiDiStream) Send(b *a2av1.SayBeat) error {
	m.out = append(m.out, b)
	return nil
}

func (m *mockBiDiStream) Context() context.Context { return m.ctx }

// ---------- 验证类 ----------

// TestSayStreamForward_RejectsNoNpcID —— 空 npc_id → InvalidArgument "R_009:..."。
func TestSayStreamForward_RejectsNoNpcID(t *testing.T) {
	svc, _ := newSignedService()
	stream := newMockBiDiStream()

	stream.in <- &a2av1.SayStreamMessage{
		Msg: &a2av1.SayStreamMessage_Init{
			Init: &a2av1.SayRequestInit{
				NpcId:       "", // 故意空
				PlayerInput: "hello",
				SessionId:   "sid-1",
				TraceId:     "trace-1",
			},
		},
	}
	close(stream.in)

	err := svc.SayStreamForward(stream)
	if err == nil {
		t.Fatal("expected error for empty npc_id, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected grpc status error, got %T: %v", err, err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %s, want InvalidArgument", st.Code())
	}
	if !strings.Contains(st.Message(), "R_009") {
		t.Errorf("message = %q, want substring R_009", st.Message())
	}
}

// TestSayStreamForward_RejectsNonInitFirstFrame —— 首帧是 heartbeat 而非 init → InvalidArgument。
func TestSayStreamForward_RejectsNonInitFirstFrame(t *testing.T) {
	svc, _ := newSignedService()
	stream := newMockBiDiStream()

	stream.in <- &a2av1.SayStreamMessage{
		Msg: &a2av1.SayStreamMessage_Heartbeat{
			Heartbeat: &a2av1.SayHeartbeat{TsMs: time.Now().UnixMilli()},
		},
	}
	close(stream.in)

	err := svc.SayStreamForward(stream)
	if err == nil {
		t.Fatal("expected error for non-init first frame, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %s, want InvalidArgument", st.Code())
	}
	if !strings.Contains(st.Message(), "R_001") {
		t.Errorf("message = %q, want substring R_001", st.Message())
	}
}

// TestSayStreamForward_RejectsClosedStream —— client 在 init 前关流 → InvalidArgument。
func TestSayStreamForward_RejectsClosedStream(t *testing.T) {
	svc, _ := newSignedService()
	stream := newMockBiDiStream()
	close(stream.in) // 直接 EOF

	err := svc.SayStreamForward(stream)
	if err == nil {
		t.Fatal("expected error for closed stream, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %s, want InvalidArgument", st.Code())
	}
}

// ---------- Happy path（骨架：只发 done 帧） ----------

// TestSayStreamForward_HappyPath_SendsDone —— 合法 init → 返一帧 done(complete=true)，
// 并写入 MirrorStore（Create + MarkDone）。
func TestSayStreamForward_HappyPath_SendsDone(t *testing.T) {
	svc, _ := newSignedService()
	store := crosscity.NewMirrorStore(60 * time.Minute)
	svc.SetMirrorStore(store)

	stream := newMockBiDiStream()
	stream.in <- &a2av1.SayStreamMessage{
		Msg: &a2av1.SayStreamMessage_Init{
			Init: &a2av1.SayRequestInit{
				NpcId:       "npc_b_wu",
				PlayerInput: "你好",
				SessionId:   "sid-happy-1",
				TraceId:     "trace-happy-1",
				PlayerId:    "player-1",
			},
		},
	}
	close(stream.in)

	if err := svc.SayStreamForward(stream); err != nil {
		t.Fatalf("SayStreamForward: %v", err)
	}
	if len(stream.out) != 1 {
		t.Fatalf("got %d beat frames, want 1", len(stream.out))
	}
	beat := stream.out[0]
	if beat.Type != "npc_say_stream_done" {
		t.Errorf("type = %q, want npc_say_stream_done", beat.Type)
	}
	if beat.NpcId != "npc_b_wu" {
		t.Errorf("npc_id = %q, want npc_b_wu", beat.NpcId)
	}
	if beat.SessionId != "sid-happy-1" {
		t.Errorf("session_id = %q, want sid-happy-1", beat.SessionId)
	}
	if beat.TraceId != "trace-happy-1" {
		t.Errorf("trace_id = %q, want trace-happy-1", beat.TraceId)
	}
	if beat.Complete == nil || !*beat.Complete {
		t.Errorf("complete = %v, want true", beat.Complete)
	}
	if beat.TsMs == 0 {
		t.Errorf("ts_ms should be set")
	}

	// mirror 状态：MarkDone(complete=true) 由 defer 触发
	ok, err := store.IsComplete("sid-happy-1")
	if err != nil {
		t.Fatalf("IsComplete: %v", err)
	}
	if !ok {
		t.Errorf("mirror complete = false, want true (skeleton end path)")
	}
}

// TestSayStreamForward_NilMirrorSafe —— 不注入 mirror 也能跑通骨架路径
// （向后兼容：旧 Service 用法不破）。
func TestSayStreamForward_NilMirrorSafe(t *testing.T) {
	svc, _ := newSignedService() // 未 SetMirrorStore → nil
	stream := newMockBiDiStream()
	stream.in <- &a2av1.SayStreamMessage{
		Msg: &a2av1.SayStreamMessage_Init{
			Init: &a2av1.SayRequestInit{
				NpcId:     "npc_x",
				SessionId: "sid-nil-1",
			},
		},
	}
	close(stream.in)

	if err := svc.SayStreamForward(stream); err != nil {
		t.Fatalf("SayStreamForward with nil mirror: %v", err)
	}
	if len(stream.out) != 1 || stream.out[0].Type != "npc_say_stream_done" {
		t.Fatalf("expected 1 done frame, got %d frames", len(stream.out))
	}
}
