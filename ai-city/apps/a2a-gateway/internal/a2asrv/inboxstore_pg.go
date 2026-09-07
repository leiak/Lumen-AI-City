// InboxStore PG 实现（Sprint 7）。
//
// a2a_inbox 表的 store-and-forward：
//   - Append：HTTPAdapter 失败 fallback 写入 / InboxAdapter 直接写入
//   - Fetch：按 to_agent_id + cursor 分页拉取（降序）
//   - MarkRead：可选，拉取即标已读
//   - Count：metrics / 调试用
//   - Cleanup（Sprint 7+）：删除 expires_at < NOW() 的行（cron 周期调用）
//
// 错误码（返回 error，由调用方映射）：
//   F_011 inbox 写失败
//   F_012 inbox 读失败
//
// 注：本文件不返回 F-code 字符串；调用方按 error 决定 envelope（F_011/F_012）。
package a2asrv

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// InboxEntry 是 a2a_inbox 表的行（Append 入参）。
type InboxEntry struct {
	MessageID      string
	ConversationID string
	FromAgentID    string
	ToAgentID      string
	Type           string
	Payload        []byte
	TsMs           int64
	TraceID        string
	Signature      string
	// ExpiresAt（Sprint 7+）：零值 → 用 store.defaultTTL 计算；
	// 非零 → 直接使用（用于测试 / 注入已过期行）
	ExpiresAt time.Time
	// 注：ReadAt / QueuedAt 由 PG 默认值生成；FailReason 由调用方传入
}

// InboxStore 是 a2a_inbox 表的 PG 封装。
type InboxStore struct {
	pool       *pgxpool.Pool
	defaultTTL time.Duration // Sprint 7+：新行默认 expires_at = queued_at + defaultTTL
}

// NewInboxStore 构造 InboxStore（alias：与 Sprint 7 测试兼容 → TTL=0 表示不设 expires_at）
//
//   - ttl == 0 → 不在 SQL 里写 expires_at，让 PG DEFAULT (NOW() + '7d') 兜底
//     （保留 Sprint 7 行为，便于旧测试不感知 Sprint 7+ 列）
//   - ttl  > 0 → 显式写 expires_at = NOW() + ttl
//
// 推荐生产用 NewInboxStoreWithTTL；本函数保留作 alias 不破现有 5 个 PG 测试 + adapter 引用。
func NewInboxStore(pool *pgxpool.Pool) *InboxStore {
	return NewInboxStoreWithTTL(pool, 0)
}

// NewInboxStoreWithTTL 构造 InboxStore with explicit TTL（Sprint 7+）。
//
//   - ttl == 0 → 同 NewInboxStore：依赖 PG DEFAULT（'7d'）
//   - ttl  > 0 → SQL 里写 expires_at = NOW() + ttl
func NewInboxStoreWithTTL(pool *pgxpool.Pool, ttl time.Duration) *InboxStore {
	return &InboxStore{pool: pool, defaultTTL: ttl}
}

// Append 写入一条 inbox 记录。
//
//   - 重复 message_id → 静默忽略（ON CONFLICT DO NOTHING）；视为幂等（防重投递）
//   - failReason：HTTPAdapter fallback 写入时填（如 "F_010:upstream 503"）；InboxAdapter 写入时填 ""
//   - ExpiresAt 零值 + defaultTTL > 0 → 写 expires_at = NOW() + defaultTTL
//   - ExpiresAt 零值 + defaultTTL == 0 → 跳过 expires_at（PG DEFAULT 兜底）
//   - ExpiresAt 非零 → 直接使用（测试 / 注入已过期行用）
//
// 失败 → 返 error；service 层映射 F_011。
func (s *InboxStore) Append(ctx context.Context, e *InboxEntry, failReason string) error {
	if e == nil {
		return fmt.Errorf("nil entry")
	}
	if e.MessageID == "" {
		return fmt.Errorf("empty message_id")
	}
	if e.ToAgentID == "" {
		return fmt.Errorf("empty to_agent_id")
	}

	conv := e.ConversationID
	trace := e.TraceID
	sig := e.Signature

	// Sprint 7+：决定 expires_at 写入策略
	//   - explicit != zero → 用 explicit
	//   - explicit == zero 且 defaultTTL > 0 → NOW() + defaultTTL
	//   - explicit == zero 且 defaultTTL == 0 → 传 nil，由 SQL 的 COALESCE 兜底
	//     （注意：列已出现在 INSERT 列表里时传 nil 是写入 SQL NULL，不会触发
	//      列的 DEFAULT，而 expires_at 是 NOT NULL → 必须显式 COALESCE。
	//      兜底值与 packages/proto/pg-schema.sql 的 DEFAULT 保持一致：7 天）
	var expiresAt *time.Time
	if !e.ExpiresAt.IsZero() {
		expiresAt = &e.ExpiresAt
	} else if s.defaultTTL > 0 {
		t := time.Now().Add(s.defaultTTL)
		expiresAt = &t
	}

	const q = `
INSERT INTO a2a_inbox (message_id, conversation_id, from_agent_id, to_agent_id, type, payload, ts_ms, trace_id, signature, fail_reason, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''),
        COALESCE($11::timestamptz, NOW() + INTERVAL '7 days'))
ON CONFLICT (message_id) DO NOTHING
`
	_, err := s.pool.Exec(ctx, q,
		e.MessageID,
		conv,
		e.FromAgentID,
		e.ToAgentID,
		e.Type,
		e.Payload,
		e.TsMs,
		trace,
		sig,
		failReason,
		expiresAt, // nil → PG DEFAULT
	)
	if err != nil {
		log.Printf("[a2asrv] InboxStore.Append(%s → %s) pg error: %v",
			e.FromAgentID, e.ToAgentID, err)
		return fmt.Errorf("inbox append: %w", err)
	}
	return nil
}

// Fetch 按 to_agent_id 拉取未读（按 queued_at DESC）。
//
//   - limit: 1..500；超出返 error
//   - cursor: 空 = 拉最新；非空 = 解析为 RFC3339Nano 时间戳，返 queued_at < cursor 的
//   - markRead: true = 同事务里 UPDATE read_at=NOW()
//
// 返回 ([]messages, nextCursor, error)。
//   - nextCursor 空 = 已拉完；非空 = 上一批最后一条的 queued_at（RFC3339Nano）
//   - error → service 层映射 F_012
func (s *InboxStore) Fetch(ctx context.Context, agentID string, limit int, cursor string, markRead bool) ([]*a2av1.Message, string, error) {
	if agentID == "" {
		return nil, "", fmt.Errorf("empty agent_id")
	}
	if limit < 1 || limit > 500 {
		return nil, "", fmt.Errorf("limit out of range [1, 500]: %d", limit)
	}

	// 解析 cursor（RFC3339Nano）
	var cursorTime *time.Time
	if cursor != "" {
		t, err := time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor: %w", err)
		}
		cursorTime = &t
	}

	// 主查询：limit + 1 判断是否还有下一页
	const qBase = `
SELECT message_id, conversation_id, from_agent_id, to_agent_id, type, payload, ts_ms, trace_id, signature, queued_at
FROM a2a_inbox
WHERE to_agent_id = $1 AND read_at IS NULL
`
	args := []any{agentID}
	if cursorTime != nil {
		args = append(args, *cursorTime)
	}

	q := qBase
	if cursorTime != nil {
		q += fmt.Sprintf(" AND queued_at < $%d", len(args))
	}
	q += fmt.Sprintf(" ORDER BY queued_at DESC LIMIT $%d", len(args)+1)
	args = append(args, limit+1)

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, "", fmt.Errorf("inbox fetch query: %w", err)
	}
	defer rows.Close()

	var (
		out    []*a2av1.Message
		lastQ  time.Time
		ids    []string
	)
	for rows.Next() {
		var (
			msgID, conv, from, to, typ, trace, sig string
			payload                                 []byte
			tsMs                                    int64
			queuedAt                                time.Time
		)
		if err := rows.Scan(&msgID, &conv, &from, &to, &typ, &payload, &tsMs, &trace, &sig, &queuedAt); err != nil {
			return nil, "", fmt.Errorf("inbox fetch scan: %w", err)
		}
		out = append(out, &a2av1.Message{
			MessageId:      msgID,
			ConversationId: conv,
			FromAgentId:    from,
			ToAgentId:      to,
			Type:           typ,
			Payload:        payload,
			TsMs:           tsMs,
			TraceId:        trace,
			Signature:      sig,
		})
		ids = append(ids, msgID)
		lastQ = queuedAt
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("inbox fetch rows: %w", err)
	}

	// 判断是否还有下一页：拿到 limit+1 条 → 截断前 limit 条，剩 1 条 → 有下一页
	var nextCursor string
	if len(out) > limit {
		out = out[:limit]
		// 用最后一条的 queued_at 作为下一页 cursor
		nextCursor = lastQ.Add(-time.Nanosecond).Format(time.RFC3339Nano) // 严格小于
		if len(out) > 0 {
			// 重新查最后一条的 queued_at（因为 lastQ 是被截断的那条）
			nextCursor = rowsLastQueuedAt(ctx, s.pool, agentID, out[len(out)-1].GetMessageId())
		}
	}

	// 标已读：只标真正返回给调用方的那些（out 可能已被截断到 limit）。
	// 注意不能用 ids[:limit] —— 实际行数少于 limit 时会 panic
	// （slice bounds out of range），这是最常见的情形。
	if markRead && len(out) > 0 {
		if err := s.markRead(ctx, ids[:len(out)]...); err != nil {
			return nil, "", fmt.Errorf("inbox mark_read: %w", err)
		}
	}

	return out, nextCursor, nil
}

// rowsLastQueuedAt 拿指定 message_id 的 queued_at（用于生成 next_cursor）。
func rowsLastQueuedAt(ctx context.Context, pool *pgxpool.Pool, agentID, msgID string) string {
	const q = `SELECT queued_at FROM a2a_inbox WHERE to_agent_id=$1 AND message_id=$2`
	var t time.Time
	if err := pool.QueryRow(ctx, q, agentID, msgID).Scan(&t); err != nil {
		if err != pgx.ErrNoRows {
			log.Printf("[a2asrv] rowsLastQueuedAt scan: %v", err)
		}
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

// markRead 把给定 message_ids 标已读（同 agent 上下文）。
func (s *InboxStore) markRead(ctx context.Context, messageIDs ...string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	const q = `UPDATE a2a_inbox SET read_at = NOW() WHERE message_id = ANY($1) AND read_at IS NULL`
	_, err := s.pool.Exec(ctx, q, messageIDs)
	if err != nil {
		return fmt.Errorf("mark_read: %w", err)
	}
	return nil
}

// Count 返回某 agent 的 inbox 计数（unreadOnly=true 时仅未读）。
func (s *InboxStore) Count(ctx context.Context, agentID string, unreadOnly bool) int {
	q := `SELECT COUNT(*) FROM a2a_inbox WHERE to_agent_id=$1`
	if unreadOnly {
		q += ` AND read_at IS NULL`
	}
	var n int
	if err := s.pool.QueryRow(ctx, q, agentID).Scan(&n); err != nil {
		log.Printf("[a2asrv] InboxStore.Count(%s) pg error: %v", agentID, err)
		return 0
	}
	return n
}

// Cleanup 删除 expires_at < NOW() 的行（Sprint 7+ cron 调用）。
//
// 返回：
//   - int64：本次删除的行数
//   - error：PG 执行错
//
// 调用方负责 ctx timeout（cron 用独立 30s ctx，不复用调用方 ctx）。
func (s *InboxStore) Cleanup(ctx context.Context) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM a2a_inbox WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("inbox cleanup: %w", err)
	}
	return tag.RowsAffected(), nil
}

// inboxEntryFromMessage 把 a2av1.Message 转 InboxEntry（append 辅助）。
func inboxEntryFromMessage(msg *a2av1.Message, failReason string) *InboxEntry {
	if msg == nil {
		return nil
	}
	return &InboxEntry{
		MessageID:      msg.GetMessageId(),
		ConversationID: msg.GetConversationId(),
		FromAgentID:    msg.GetFromAgentId(),
		ToAgentID:      msg.GetToAgentId(),
		Type:           msg.GetType(),
		Payload:        msg.GetPayload(),
		TsMs:           msg.GetTsMs(),
		TraceID:        msg.GetTraceId(),
		Signature:      msg.GetSignature(),
	}
}
