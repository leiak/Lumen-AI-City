// Package handlers - npc_position (Sprint 12 acceptance_1_0 §一.B)
//
// GET  /v1/npc/:id/position — 读 npc_position JOIN npc 返 JSON
// POST /v1/npc/:id/position — upsert (x, y) 到 npc_position，tile_id 自动算
//
// `:id` 接受 agent_id（"npc_wang_boss_001"），不是 UUID — 与 agent-os 模板 /
// acceptance 1.0 脚本一致。endpoint 把 agent_id 解析成 UUID 后读写 npc_position。
//
// enabled 字段：npc 表没有该列，用 `deleted_at IS NULL` 映射（NULL=活跃，
// 非 NULL=软删），与 sprint 11 plan §二.5 描述一致。
//
// 错误码：
//   - NPC_001: NPC 不存在（agent_id 在 npc 表查不到）        → 404
//   - NPC_002: bad request（缺 x/y / 无法解析为 float）       → 400
package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// NPCPositionHandler serves GET/POST /v1/npc/:id/position.
type NPCPositionHandler struct {
	DB     *pgxpool.Pool
	Logger *zap.Logger
}

// NewNPCPositionHandler wires the handler. db 必须已连接（启动期 Ping 过）。
func NewNPCPositionHandler(db *pgxpool.Pool, log *zap.Logger) *NPCPositionHandler {
	return &NPCPositionHandler{DB: db, Logger: log}
}

type npcPositionResp struct {
	NpcID    string  `json:"npc_id"`     // agent_id（输入的字符串形式）
	TileID   string  `json:"tile_id"`    // 派生：tile_${floor(x/100)}_${floor(y/100)}
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Enabled  bool    `json:"enabled"`    // npc.deleted_at IS NULL
	HomeTile string  `json:"home_tile"`  // npc.home_tile_id
}

type npcPositionSetReq struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

// resolveAgentID 查 npc.agent_id → uuid。返回 errors.Is(err, pgx.ErrNoRows) 时
// 表示 NPC 不存在，handler 转 404。
func resolveAgentID(ctx pgx.Tx, agentID string) (pgx.Tx, error) {
	// 占位：实际查询放 handle 内（保持单语句事务）；此处仅为占位
	return ctx, nil
}

// tileFromXY 与 world-engine Tile::from_xy 一致：floor(x/100), floor(y/100)。
// x/y 为负数时 Go 的 int 除法向零截断，需要手动 floor。
func tileFromXY(x, y float32) string {
	tx := int(x) / 100
	ty := int(y) / 100
	if x < 0 && float32(tx) > x {
		tx--
	}
	if y < 0 && float32(ty) > y {
		ty--
	}
	return formatTile(tx, ty)
}

func formatTile(x, y int) string {
	// 拼 tile_<x>_<y>（x/y 允许负数，自然格式）
	return "tile_" + itoa(x) + "_" + itoa(y)
}

// itoa 避免 strconv import 噪声（仅用于 tile id 拼装）。
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// HandleGet serves GET /v1/npc/:id/position.
func (h *NPCPositionHandler) HandleGet(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": "npc id required"})
		return
	}

	// 一条 SQL JOIN：拿到 uuid + enabled + home_tile + 当前位置。
	// LEFT JOIN npc_position：没位置行时返 (0,0) + home tile，不返 404（demo 体验）。
	var (
		uuid      string
		deletedAt *time.Time
		homeTile  *string
		posTile   *string
		x, y      float32
	)
	err := h.DB.QueryRow(c.Request.Context(), `
		SELECT n.id, n.deleted_at, n.home_tile_id,
		       p.tile_id, p.x, p.y
		FROM npc n
		LEFT JOIN npc_position p ON p.npc_id = n.id
		WHERE n.agent_id = $1
	`, agentID).Scan(&uuid, &deletedAt, &homeTile, &posTile, &x, &y)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":  "NPC_001",
			"detail": "NPC not found: " + agentID,
		})
		return
	}
	if err != nil {
		h.Logger.Error("npc position read failed",
			zap.String("agent_id", agentID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "NPC_003",
			"detail": "internal error",
		})
		return
	}

	enabled := deletedAt == nil
	tileID := ""
	if posTile != nil {
		tileID = *posTile
	}
	home := ""
	if homeTile != nil {
		home = *homeTile
	}
	c.JSON(http.StatusOK, npcPositionResp{
		NpcID:    agentID,
		TileID:   tileID,
		X:        x,
		Y:        y,
		Enabled:  enabled,
		HomeTile: home,
	})
}

// HandleSet serves POST /v1/npc/:id/position.
// Body: {"x": 50.0, "y": 50.0}. 缺字段 → 400；坐标非法 → 400。
// tile_id 自动按 tileFromXY 派生并写入；upsert ON CONFLICT。
//
// 注：本 handler 故意**不** publish 到 aicity:npc_moved。agent-os 的
// MoveScheduler 不知道这个"admin reset"操作，下次 tick 它会按 walk.tiles
// 把 NPC 拉回正常路径——这是 acceptance_1_0 §一.B 期望行为（"重置到 tile_0_0
// 准备阶段"），不与运行时轨迹混淆。如果将来需要联动，加一行
// `rdb.Publish(ctx, "aicity:npc_moved", payload)` 即可。
func (h *NPCPositionHandler) HandleSet(c *gin.Context) {
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": "npc id required"})
		return
	}

	var req npcPositionSetReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "NPC_002", "detail": err.Error()})
		return
	}
	if req.X == 0 && req.Y == 0 {
		// 0/0 是合法坐标，但与"未填 body"难以区分——Sprint 11 spec 说不接 body 时只查；
		// 此处 POST 强制要坐标，故拒绝 0,0（accept demo 阶段 tile_0_0 = (50, 50)）
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "NPC_002",
			"detail": "x and y must be non-zero; use tile center +/- jitter in demo",
		})
		return
	}

	tileID := tileFromXY(req.X, req.Y)

	// Upsert：先查 uuid（不存在 → 404），再 INSERT ... ON CONFLICT UPDATE。
	var uuid string
	err := h.DB.QueryRow(c.Request.Context(),
		`SELECT id FROM npc WHERE agent_id = $1 AND deleted_at IS NULL`, agentID).Scan(&uuid)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{
			"error":  "NPC_001",
			"detail": "NPC not found: " + agentID,
		})
		return
	}
	if err != nil {
		h.Logger.Error("npc uuid lookup failed",
			zap.String("agent_id", agentID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "NPC_003",
			"detail": "internal error",
		})
		return
	}

	_, err = h.DB.Exec(c.Request.Context(), `
		INSERT INTO npc_position (npc_id, tile_id, x, y, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (npc_id) DO UPDATE
		   SET tile_id = EXCLUDED.tile_id,
		       x       = EXCLUDED.x,
		       y       = EXCLUDED.y,
		       updated_at = NOW()
	`, uuid, tileID, req.X, req.Y)
	if err != nil {
		h.Logger.Error("npc position upsert failed",
			zap.String("agent_id", agentID),
			zap.String("uuid", uuid),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "NPC_003",
			"detail": "internal error",
		})
		return
	}

	// 返 GET 同样的 envelope（客户端 POST 后无需再 GET）。
	c.JSON(http.StatusOK, npcPositionResp{
		NpcID:    agentID,
		TileID:   tileID,
		X:        req.X,
		Y:        req.Y,
		Enabled:  true,
		HomeTile: "", // POST 不重查 home tile；客户端已有 home
	})
}
