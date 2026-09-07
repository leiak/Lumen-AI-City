// ACLStore PG 实现（Sprint 8）—— a2a_acl_policy 表的 deny list 查询。
//
// 表结构见 packages/proto/pg-schema.sql：
//   PRIMARY KEY (agent_id, peer_city, action)
//
// 本 sprint 只读：策略由运维直接 INSERT/DELETE 维护，无写 API
// （写 API 属 Sprint 9+ 范畴）。
package a2asrv

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ACLStore 是 a2a_acl_policy 表的 PG 封装。
type ACLStore struct {
	pool *pgxpool.Pool
}

// NewACLStore 构造 ACLStore。
func NewACLStore(pool *pgxpool.Pool) *ACLStore {
	return &ACLStore{pool: pool}
}

// IsDenied 查询 agentID 是否被禁止向 peerCity 投递。
//
// 默认 allow：只有命中 action='deny' 的行才返回 true。
//
// 两种命中方式：
//   - peer_city = peerCity  → 针对该城邦的定向 deny
//   - peer_city = ''        → 全局 deny（该 agent 不可发往任何城邦）
//
// 查询失败返回 error（不吞）；调用方 ACL.CheckDeliver 据此 fail-closed。
func (s *ACLStore) IsDenied(ctx context.Context, agentID, peerCity string) (bool, error) {
	if s == nil || s.pool == nil {
		return false, nil
	}
	if agentID == "" {
		return false, nil
	}
	const q = `
SELECT EXISTS (
    SELECT 1 FROM a2a_acl_policy
    WHERE agent_id = $1
      AND action = 'deny'
      AND (peer_city = $2 OR peer_city = '')
)`
	var denied bool
	if err := s.pool.QueryRow(ctx, q, agentID, peerCity).Scan(&denied); err != nil {
		return false, fmt.Errorf("acl query: %w", err)
	}
	return denied, nil
}
