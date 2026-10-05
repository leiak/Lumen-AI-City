// Package handlers —— a2a-gateway HTTP handler 集合（2.0 stage 1）。
//
// 本文件专门承载跨城 NPC 对话转发：
//
//	api-gateway POST /v1/npc/talk (npc_id 含 `_b_`)
//	  → a2a-gateway POST /v1/cross_city/talk/:npc_id （本 handler）
//	  → crosscity.Dial(route.Endpoint) + gRPC → 远端 world-engine
//
// 错误码（与 a2a F_xxx 对齐）：
//   - F_011 503 NPC 路由未命中（routing table 没这条 npc_id）
//   - F_010 502 mTLS gRPC 拨号失败 / 跨城 RPC 调用失败
//   - F_004 500 远端 world-engine 找不到 NPC 所在 tile
//
// 设计注：当前 world.proto 未暴露 NPC 对话 RPC（只有 Move/GetTile/SubscribePosition/
// ComputePath）。本 handler 用 GetTile 作为跨城连通性探针 —— 命中 routing table 且
// 能 mTLS 握手就算跨城路径通了；后续 world-engine 暴露 Talk 后替换 RPC 即可。
//
// 路由注册：cmd/main.go 通过 httpgw.Server.Engine() 挂到 POST /v1/cross_city/talk/:npc_id。
package handlers

import (
	"net/http"

	"github.com/aicity/a2a-gateway/internal/crosscity"
	"github.com/aicity/a2a-gateway/internal/router"
	worldv1 "github.com/aicity/proto/gen/go"
	"github.com/gin-gonic/gin"
)

// CrossCityTalkHandler 处理 POST /v1/cross_city/talk/:npc_id。
//
// 流程：
//  1. 从 URL 路径取 npc_id；
//  2. 在 router.GlobalTable 查 route（npc_id → endpoint + cert_cn）；
//  3. crosscity.Dial(route.Endpoint) 建立 mTLS gRPC 连接；
//  4. 以远端 world-engine.GetTile 做连通性探针（NPC 实际 dialogue 转发留待
//     world.proto 增加 Talk RPC 后替换）；
//  5. 返回远端响应（或统一 envelope 错误）。
//
// rt 通常传 router.GlobalTable（cmd/main.go 注册时传入）。
func CrossCityTalkHandler(rt *router.Table) gin.HandlerFunc {
	return func(c *gin.Context) {
		npcID := c.Param("npc_id")
		if npcID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "F_001",
				"message": "npc_id required in path",
			})
			return
		}

		// 跨城路由解析：未命中 → 503 F_011（路由表无该 NPC = 联邦目标暂不可达）
		route, ok := rt.Lookup(npcID)
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":    "F_011",
				"message": "NPC 暂时不可达：路由表无 " + npcID,
			})
			return
		}

		// mTLS gRPC 拨号（crosscity.Dial 内部依赖 GRPC_TLS_CERT/KEY/CA env）
		conn, err := crosscity.Dial(route.Endpoint)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"code":    "F_010",
				"message": "跨城 gRPC 拨号失败：" + err.Error(),
			})
			return
		}
		defer conn.Close()

		client := worldv1.NewWorldEngineClient(conn)

		// 远端 NPC 实际 talk RPC 未在 world.proto 暴露 —— 当前用 GetTile 做跨城
		// 连通性探针，验证 mTLS + 路由 + 端点可达三件套。body 仍解 JSON 以兼容
		// api-gateway forwardCrossCity 的请求形态（text/player_id 备用字段）。
		var body struct {
			Text     string `json:"text"`
			PlayerID string `json:"player_id"`
		}
		// body 不强制要求 —— 跨城 talk 在 world.proto 暴露前可空 body
		_ = c.ShouldBindJSON(&body)

		tile, err := client.GetTile(c.Request.Context(), &worldv1.GetTileRequest{
			TileId: npcID,
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{
				"code":    "F_010",
				"message": "跨城 RPC 失败：" + err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"npc_id":      npcID,
			"endpoint":    route.Endpoint,
			"city":        route.City,
			"tile_id":     tile.GetId(),
			"npc_ids":     tile.GetNpcIds(),
			"text":        body.Text,
			"player_id":   body.PlayerID,
			"relayed":     true,
			"via":         "a2a-gateway/cross_city/talk",
		})
	}
}