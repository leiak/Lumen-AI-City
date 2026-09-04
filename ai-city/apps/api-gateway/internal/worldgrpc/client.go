// Package worldgrpc 提供 world-engine gRPC 客户端封装（Sprint 3.5）
//
// 替代原先的 REST 反向代理（world_proxy.go）用于 /v1/world/move 高频写路径。
// 读路径（/v1/tiles/*）继续走 REST proxy（web 端用，便于缓存 / JSON 直接消费）。
//
// 客户端使用 google.golang.org/grpc + 由 packages/proto 生成的 worldv1 stub。
// 默认无 TLS（本地）；生产应加 mTLS + WithTransportCredentials。
package worldgrpc

import (
	"context"
	"fmt"
	"time"

	worldv1 "github.com/aicity/proto/gen/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// Client 是 world-engine 的 gRPC 客户端封装。
// 线程安全（底层 grpc.ClientConn 是安全的），可全局共享一份。
type Client struct {
	conn *grpc.ClientConn
	wc   worldv1.WorldEngineClient
}

// NewClient 拨号到 addr（格式 "host:port"，如 "127.0.0.1:50051"）。
// 拨号失败也返回 error —— 调用方应决定是否 fatal。
func NewClient(addr string) (*Client, error) {
	// 短超时拨号，避免启动期 hang
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // 等同 grpc.Dial（旧 API 的"ready on dial"语义）
	)
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	return &Client{
		conn: conn,
		wc:   worldv1.NewWorldEngineClient(conn),
	}, nil
}

// Close 释放连接。Sprint 3.5 在 main shutdown 时调用。
func (c *Client) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

// Move 转发到 world-engine gRPC Move。
// 返回服务端校正后的位置 + tile_id。
//
// 错误语义：
//   - context.Canceled / DeadlineExceeded：调用方应识别为上游超时
//   - codes.InvalidArgument：entity_id 空等校验错
//   - codes.Unavailable：world-engine 不可达（应触发 circuit breaker）
//
// 调用方拿到的 err 可能是 grpc.Status 类型，可 status.Code(err) 提取 code。
func (c *Client) Move(ctx context.Context, req *worldv1.MoveRequest) (*worldv1.MoveResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("move: nil request")
	}
	resp, err := c.wc.Move(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("move: %w", err)
	}
	return resp, nil
}

// GetTile 单 Tile 查询（gRPC 形态）。当前 router 仍走 REST proxy，
// 此方法供未来想替换 tiles 路径时复用。
func (c *Client) GetTile(ctx context.Context, tileID string) (*worldv1.Tile, error) {
	resp, err := c.wc.GetTile(ctx, &worldv1.GetTileRequest{TileId: tileID})
	if err != nil {
		return nil, fmt.Errorf("get_tile %q: %w", tileID, err)
	}
	return resp, nil
}

// ComputePath 计算两点间路径（Sprint 3：直线 stub）。
func (c *Client) ComputePath(ctx context.Context, entityID string, start, end *worldv1.Vec2) (*worldv1.PathResponse, error) {
	resp, err := c.wc.ComputePath(ctx, &worldv1.PathRequest{
		EntityId: entityID,
		Start:    start,
		End:      end,
	})
	if err != nil {
		return nil, fmt.Errorf("compute_path: %w", err)
	}
	return resp, nil
}

// IsGRPCUnavailable 判断 error 是否为 gRPC Unavailable（连接断开 / 服务下线）。
// 供 circuit breaker / fallback 决策用。
func IsGRPCUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return status.Code(err).String() == "Unavailable"
}
