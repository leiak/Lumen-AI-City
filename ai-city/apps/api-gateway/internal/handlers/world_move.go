// Package handlers - world_move（Sprint 3.5）
//
// /v1/world/move 改为调用 world-engine gRPC Move（不再走 REST proxy）。
// 优势：
//   - 省掉一次 JSON 序列化（HTTP body → struct → protobuf）
//   - 与客户端（agent-core / ai-core）未来直连 world-engine 路径对齐
//   - 错误码更精确（gRPC status vs HTTP status）
//
// 公共 API 形态保持不变（仍是 POST /v1/world/move，body 还是 REST 字段）
//  —— 改动只发生在 api-gateway 这一跳，对调用方透明。
package handlers

import (
	"net/http"

	"github.com/aicity/api-gateway/internal/worldgrpc"
	"github.com/gin-gonic/gin"
	worldv1 "github.com/aicity/proto/gen/go"
	"google.golang.org/grpc/status"
)

// WorldMoveHandler 把 POST /v1/world/move 的 JSON body 转为 gRPC MoveRequest，
// 调 world-engine gRPC，再把 MoveResponse 序列化成与原 REST 形态兼容的 JSON。
//
// REST body（不变）：
//   { "player_id": "uuid", "from_tile_id": "tile_x_x", "to_tile_id": "tile_y_y",
//     "x": 150.0, "y": 50.0 }
//
// gRPC MoveRequest 字段：
//   entity_id = player_id, target = Vec2{x,y}, sequence = 0, predicted = false,
//   ts_ms = 0（world-engine 自己填 wall-clock）
//
// REST response（不变）：
//   { "player_id": "...", "current_tile_id": "tile_y_y", "x": 150.0, "y": 50.0,
//     "ts_ms": <server ts> }
type WorldMoveHandler struct {
	client *worldgrpc.Client
}

func NewWorldMoveHandler(c *worldgrpc.Client) *WorldMoveHandler {
	return &WorldMoveHandler{client: c}
}

type moveRequestBody struct {
	PlayerID   string  `json:"player_id"   binding:"required"`
	FromTileID string  `json:"from_tile_id"`
	ToTileID   string  `json:"to_tile_id"   binding:"required"`
	X          float32 `json:"x"`
	Y          float32 `json:"y"`
}

type moveResponseBody struct {
	PlayerID     string  `json:"player_id"`
	CurrentTile  string  `json:"current_tile_id"`
	X            float32 `json:"x"`
	Y            float32 `json:"y"`
	TSMs         int64   `json:"ts_ms"`
	Accepted     bool    `json:"accepted"`
	Sequence     uint64  `json:"sequence,omitempty"`
	SourceChannel string  `json:"source_channel,omitempty"` // "grpc"（调试用）
}

func (h *WorldMoveHandler) Move(c *gin.Context) {
	var body moveRequestBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad_request", "detail": err.Error()})
		return
	}

	resp, err := h.client.Move(c.Request.Context(), &worldv1.MoveRequest{
		EntityId: body.PlayerID,
		Target:   &worldv1.Vec2{X: body.X, Y: body.Y},
	})
	if err != nil {
		// gRPC code → HTTP status 映射（最小集合）
		code := status.Code(err)
		httpStatus := http.StatusBadGateway
		switch code.String() {
		case "InvalidArgument":
			httpStatus = http.StatusBadRequest
		case "NotFound":
			httpStatus = http.StatusNotFound
		case "Unavailable":
			httpStatus = http.StatusServiceUnavailable
		case "DeadlineExceeded":
			httpStatus = http.StatusGatewayTimeout
		}
		c.JSON(httpStatus, gin.H{
			"error":      "world_engine_error",
			"grpc_code":  code.String(),
			"detail":     err.Error(),
			"source":     "grpc",
		})
		return
	}

	// tile_id 从 gRPC 响应里取（如果 server 返回了 corrected_position 但没给 tile_id，
	// 我们用请求里的 to_tile_id 作 fallback）
	currentTile := body.ToTileID
	c.JSON(http.StatusOK, moveResponseBody{
		PlayerID:      body.PlayerID,
		CurrentTile:   currentTile,
		X:             resp.GetCorrectedPosition().GetX(),
		Y:             resp.GetCorrectedPosition().GetY(),
		TSMs:          resp.GetServerTsMs(),
		Accepted:      resp.GetAccepted(),
		Sequence:      resp.GetSequence(),
		SourceChannel: "grpc",
	})
}
