// CardStore PG 实现（Sprint 7）。
//
// 替换 Sprint 5/6 的内存 Registry：所有 Register/Get/Discover/Size 直查 a2a_agent_card 表。
//
// 设计：
//   - 用 pgxpool.Pool（与 api-gateway 共享 pgx/v5 v5.6.0）
//   - Register 幂等覆盖（ON CONFLICT DO UPDATE）；重复返 (true, "F_002")
//   - Discover 按 capability 数组 contains 查询（GIN 索引支撑）
//   - cityFilter 暂忽略（warn log，与 Sprint 5 MVP 一致；ACL 留给 Sprint 7+）
//
// 错误码：
//   F_001 agent_id/name 缺失
//   F_002 agent_id 已存在（仍 accepted:true）
//   F_006 auth["ed25519"] 非空但 base64 解不出 32 字节公钥
package a2asrv

import (
	"context"
	"encoding/base64"
	"log"

	"crypto/ed25519"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// CardStore 是 a2a_agent_card 表的 PG-backed Registry。
type CardStore struct {
	pool *pgxpool.Pool
}

// NewCardStore 构造 CardStore。
func NewCardStore(pool *pgxpool.Pool) *CardStore {
	return &CardStore{pool: pool}
}

// Register 注册 / 覆盖 AgentCard（upsert）。
//
// 返回 (accepted, errCode)：
//   - 缺失字段 → accepted=false, errCode=F_001
//   - auth["ed25519"] 非空但公钥解析失败 → accepted=false, errCode=F_006
//   - 重复 agent_id → accepted=true, errCode=F_002（幂等覆盖）
//   - 首次注册 → accepted=true, errCode=""
func (s *CardStore) Register(ctx context.Context, card *a2av1.AgentCard) (bool, string) {
	if card == nil || card.GetAgentId() == "" || card.GetName() == "" {
		return false, "F_001"
	}
	if code := validateEd25519(card); code != "" {
		return false, code
	}

	auth := card.GetAuth()
	caps := card.GetCapabilities()
	if caps == nil {
		caps = []string{}
	}

	// 序列化 auth → JSONB（map[string]string 编码）
	authJSON := encodeAuthJSON(auth)

	// upsert：同 agent_id 覆盖；判断是否曾存在靠 RETURNING (xmax = 0)
	// xmax=0 → INSERT；xmax!=0 → UPDATE
	const q = `
INSERT INTO a2a_agent_card (agent_id, name, description, url, provider, version, capabilities, auth)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)
ON CONFLICT (agent_id) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    url = EXCLUDED.url,
    provider = EXCLUDED.provider,
    version = EXCLUDED.version,
    capabilities = EXCLUDED.capabilities,
    auth = EXCLUDED.auth
RETURNING (xmax = 0) AS inserted
`
	var inserted bool
	err := s.pool.QueryRow(ctx, q,
		card.GetAgentId(),
		card.GetName(),
		card.GetDescription(),
		card.GetUrl(),
		card.GetProvider(),
		card.GetVersion(),
		caps,
		authJSON,
	).Scan(&inserted)
	if err != nil {
		log.Printf("[a2asrv] CardStore.Register(%s) pg error: %v", card.GetAgentId(), err)
		return false, "F_001" // 注册失败用 F_001 兜底（语义：被拒绝）
	}
	if !inserted {
		// 已有 → UPDATE 走 xmax != 0
		return true, "F_002"
	}
	return true, ""
}

// Get 按 agent_id 查 card。返回 (card, ok)。
func (s *CardStore) Get(ctx context.Context, agentID string) (*a2av1.AgentCard, bool) {
	if agentID == "" {
		return nil, false
	}
	const q = `
SELECT name, description, url, provider, version, capabilities, auth, registered_at_ms
FROM a2a_agent_card
WHERE agent_id = $1
`
	var (
		name, desc, url, provider, version string
		caps                               []string
		authJSON                           string
		registeredAtMs                     int64
	)
	err := s.pool.QueryRow(ctx, q, agentID).Scan(
		&name, &desc, &url, &provider, &version, &caps, &authJSON, &registeredAtMs,
	)
	if err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("[a2asrv] CardStore.Get(%s) pg error: %v", agentID, err)
		}
		return nil, false
	}
	return &a2av1.AgentCard{
		AgentId:        agentID,
		Name:           name,
		Description:    desc,
		Url:            url,
		Provider:       provider,
		Version:        version,
		Capabilities:   caps,
		Auth:           decodeAuthJSON(authJSON),
		RegisteredAtMs: registeredAtMs,
	}, true
}

// Discover 按 capability 过滤返回 AgentCard 列表。
// cityFilter 在 Sprint 7 仍忽略（warn log；ACL 留给 Sprint 7+）。
// 返回 (cards, errCode)：capability 为空时 errCode=F_003。
func (s *CardStore) Discover(ctx context.Context, capability, cityFilter string) ([]*a2av1.AgentCard, string) {
	if capability == "" {
		return nil, "F_003"
	}
	if cityFilter != "" {
		log.Printf("[a2asrv] CardStore.Discover cityFilter=%q 暂忽略（ACL 留给 Sprint 7+）", cityFilter)
	}
	const q = `
SELECT agent_id, name, description, url, provider, version, capabilities, auth, registered_at_ms
FROM a2a_agent_card
WHERE $1 = ANY(capabilities)
ORDER BY registered_at ASC
`
	rows, err := s.pool.Query(ctx, q, capability)
	if err != nil {
		log.Printf("[a2asrv] CardStore.Discover pg error: %v", err)
		return nil, ""
	}
	defer rows.Close()

	var out []*a2av1.AgentCard
	for rows.Next() {
		var (
			id, name, desc, url, provider, version string
			caps                                   []string
			authJSON                               string
			registeredAtMs                         int64
		)
		if err := rows.Scan(&id, &name, &desc, &url, &provider, &version, &caps, &authJSON, &registeredAtMs); err != nil {
			log.Printf("[a2asrv] CardStore.Discover scan: %v", err)
			continue
		}
		out = append(out, &a2av1.AgentCard{
			AgentId:        id,
			Name:           name,
			Description:    desc,
			Url:            url,
			Provider:       provider,
			Version:        version,
			Capabilities:   caps,
			Auth:           decodeAuthJSON(authJSON),
			RegisteredAtMs: registeredAtMs,
		})
	}
	if err := rows.Err(); err != nil {
		log.Printf("[a2asrv] CardStore.Discover rows.Err: %v", err)
	}
	return out, ""
}

// Size 返回当前注册数（测试 / metrics 用）。
func (s *CardStore) Size(ctx context.Context) int {
	const q = `SELECT COUNT(*) FROM a2a_agent_card`
	var n int
	if err := s.pool.QueryRow(ctx, q).Scan(&n); err != nil {
		log.Printf("[a2asrv] CardStore.Size pg error: %v", err)
		return 0
	}
	return n
}

// ---------- helpers ----------

// encodeAuthJSON 把 map[string]string → JSONB 兼容字符串。
// 空 / nil → "{}"。
func encodeAuthJSON(auth map[string]string) string {
	if len(auth) == 0 {
		return "{}"
	}
	// 简化：手写 JSON 避免引入 json.Marshal 依赖（字段值可能含 "）
	// 用 json.Marshal 更稳，但这里 auth map 字段值已知是 base64 / 短文本
	out := "{"
	first := true
	for k, v := range auth {
		if !first {
			out += ","
		}
		first = false
		out += `"` + escapeJSONString(k) + `":"` + escapeJSONString(v) + `"`
	}
	out += "}"
	return out
}

// decodeAuthJSON 把 JSONB 字符串 → map[string]string。
// 解析失败返 nil（auth 字段缺失等价）。
func decodeAuthJSON(s string) map[string]string {
	if s == "" || s == "{}" {
		return nil
	}
	// 简化：手工扫描 "k":"v" 对（不依赖 encoding/json 解析嵌套）
	out := make(map[string]string)
	i := 0
	for i < len(s) {
		// 找 "k"
		kStart := -1
		for i < len(s) && s[i] != '"' {
			i++
		}
		if i >= len(s) {
			break
		}
		kStart = i + 1
		i++
		for i < len(s) && s[i] != '"' {
			if s[i] == '\\' {
				i += 2
				continue
			}
			i++
		}
		if i >= len(s) {
			break
		}
		key := s[kStart:i]
		i++
		// 找 :
		for i < len(s) && s[i] != ':' {
			i++
		}
		if i >= len(s) {
			break
		}
		i++
		// 找 "v"
		for i < len(s) && s[i] != '"' {
			i++
		}
		if i >= len(s) {
			break
		}
		vStart := i + 1
		i++
		for i < len(s) && s[i] != '"' {
			if s[i] == '\\' {
				i += 2
				continue
			}
			i++
		}
		if i >= len(s) {
			break
		}
		val := s[vStart:i]
		i++
		out[key] = val
		// 跳到下一个 " 或 }
		for i < len(s) && s[i] != '"' && s[i] != '}' {
			i++
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// escapeJSONString 转义 JSON 字符串内的 " 和 \。
// auth map 值已知为短 base64 / 短文本，简单替换够用。
func escapeJSONString(s string) string {
	out := make([]byte, 0, len(s)+2)
	for i := 0; i < len(s); i++ {
		if s[i] == '"' || s[i] == '\\' {
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// validateEd25519 检查 auth["ed25519"] 若非空必须能解为 32 字节公钥。
// （与 Sprint 5 registry.go 行为一致；保留供 CardStore 复用）
func validateEd25519(card *a2av1.AgentCard) string {
	auth := card.GetAuth()
	if len(auth) == 0 {
		return ""
	}
	b64, ok := auth["ed25519"]
	if !ok || b64 == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "F_006"
	}
	return ""
}
