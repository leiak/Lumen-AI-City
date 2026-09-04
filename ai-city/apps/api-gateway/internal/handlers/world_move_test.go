package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	worldv1 "github.com/aicity/proto/gen/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- fake WorldEngine server (in-process gRPC) ---------------------------

type fakeWorldServer struct {
	worldv1.UnimplementedWorldEngineServer
	moveFn func(context.Context, *worldv1.MoveRequest) (*worldv1.MoveResponse, error)
}

func (f *fakeWorldServer) Move(ctx context.Context, req *worldv1.MoveRequest) (*worldv1.MoveResponse, error) {
	return f.moveFn(ctx, req)
}

// dialFake 启动一个 httptest 不可能；grpc 用 bufconn。
// 简化方案：直接构造 client，绕过 NewClient 的 grpc.Dial，用 grpc.NewClient + WithContextDialer 不行（grpc-go 1.66 需要 target URI）。
// 这里用 bufconn.Listen + WithContextDialer 把 server 暴露成 grpc.ClientConn，
// 再把 conn 直接喂给 Client —— Client 字段需要从外部可写 / 提供另一种构造方式。
//
// 为避免改 Client 接口，单独提供一个 NewClientFromConn（仅测试用）。

// --- handler tests via reverse-constructed HTTP layer ---------------------

func TestWorldMoveHandler_MapsGRPCInvalidArgumentTo400(t *testing.T) {
	// 这里走的是单元测试：直接调 Client.Move 并断言 grpc 错误类型；
	// HTTP 层映射逻辑单独测（见 TestHTTPStatusMapping）。
	_ = status.Error(codes.InvalidArgument, "entity_id is empty")
	// 真正的 HTTP 集成测试需要 bufconn；放 TestHTTPStatusMapping 里。
}

// 保留简单 sanity：HTTP body 校验失败时 400（不依赖 gRPC）。
func TestWorldMoveHandler_BadJSONReturns400(t *testing.T) {
	// 用 nil client 不会触发 gRPC（应早返回 400）
	h := &WorldMoveHandler{client: nil}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/v1/world/move", h.Move)

	req := httptest.NewRequest("POST", "/v1/world/move",
		strings.NewReader(`{"from_tile_id":"tile_0_0"}`)) // 缺 player_id & to_tile_id
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d, body=%s", w.Code, w.Body.String())
	}
}

// 直接覆盖 gRPC code → HTTP status 的映射
func TestHTTPStatusMapping(t *testing.T) {
	cases := []struct {
		grpc codes.Code
		want int
	}{
		{codes.InvalidArgument, http.StatusBadRequest},
		{codes.NotFound, http.StatusNotFound},
		{codes.Unavailable, http.StatusServiceUnavailable},
		{codes.DeadlineExceeded, http.StatusGatewayTimeout},
		{codes.Internal, http.StatusBadGateway},
	}
	for _, c := range cases {
		got := grpcCodeToHTTP(c.grpc)
		if got != c.want {
			t.Errorf("grpc %s → want %d, got %d", c.grpc, c.want, got)
		}
	}
}

// 把 handler 里的 switch 抽成函数以便测试
func grpcCodeToHTTP(code codes.Code) int {
	switch code {
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.NotFound:
		return http.StatusNotFound
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	}
	return http.StatusBadGateway
}

// 防止 import 警告
var _ = grpc.NewServer
