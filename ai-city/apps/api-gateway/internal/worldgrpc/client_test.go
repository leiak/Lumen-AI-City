package worldgrpc

import (
	"context"
	"errors"
	"net"
	"testing"

	worldv1 "github.com/aicity/proto/gen/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeWorldServer 实现 Move + 返回受控响应。
type fakeWorldServer struct {
	worldv1.UnimplementedWorldEngineServer
	moveResp *worldv1.MoveResponse
	moveErr  error
	calls    int
}

func (f *fakeWorldServer) Move(_ context.Context, _ *worldv1.MoveRequest) (*worldv1.MoveResponse, error) {
	f.calls++
	if f.moveErr != nil {
		return nil, f.moveErr
	}
	return f.moveResp, nil
}

// newBufconnClient 用 bufconn 在进程内起 server + client（无需 TCP）。
func newBufconnClient(t *testing.T, srv *fakeWorldServer) (*Client, *fakeWorldServer) {
	t.Helper()
	lis := bufconn.Listen(1024 * 64)
	s := grpc.NewServer()
	worldv1.RegisterWorldEngineServer(s, srv)
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(func() { s.Stop(); _ = lis.Close() })

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	return &Client{conn: conn, wc: worldv1.NewWorldEngineClient(conn)}, srv
}

func TestClient_Move_Success(t *testing.T) {
	srv := &fakeWorldServer{
		moveResp: &worldv1.MoveResponse{
			Accepted:          true,
			CorrectedPosition: &worldv1.Vec2{X: 150.0, Y: 50.0},
			Sequence:          42,
			ServerTsMs:        1234567890,
		},
	}
	c, _ := newBufconnClient(t, srv)

	resp, err := c.Move(context.Background(), &worldv1.MoveRequest{
		EntityId: "p1",
		Target:   &worldv1.Vec2{X: 150.0, Y: 50.0},
	})
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if !resp.GetAccepted() {
		t.Errorf("accepted=false")
	}
	if resp.GetSequence() != 42 {
		t.Errorf("sequence want 42 got %d", resp.GetSequence())
	}
	if got := resp.GetCorrectedPosition().GetX(); got != 150.0 {
		t.Errorf("corrected x want 150 got %v", got)
	}
	if srv.calls != 1 {
		t.Errorf("server calls want 1 got %d", srv.calls)
	}
}

func TestClient_Move_PropagatesGRPCError(t *testing.T) {
	srv := &fakeWorldServer{
		moveErr: status.Error(codes.InvalidArgument, "entity_id is empty"),
	}
	c, _ := newBufconnClient(t, srv)

	_, err := c.Move(context.Background(), &worldv1.MoveRequest{EntityId: ""})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code want InvalidArgument got %s", status.Code(err))
	}
}

func TestIsGRPCUnavailable(t *testing.T) {
	if IsGRPCUnavailable(nil) {
		t.Error("nil should not be unavailable")
	}
	if !IsGRPCUnavailable(status.Error(codes.Unavailable, "down")) {
		t.Error("Unavailable should be detected")
	}
	if IsGRPCUnavailable(errors.New("plain error")) {
		t.Error("plain error should not match")
	}
	if IsGRPCUnavailable(status.Error(codes.NotFound, "")) {
		t.Error("NotFound should not be Unavailable")
	}
}

func TestClient_Move_NilRequest(t *testing.T) {
	c := &Client{} // 故意不连 server；只测 nil check
	if _, err := c.Move(context.Background(), nil); err == nil {
		t.Fatal("want error for nil req")
	}
}
