// ACLStore PG 集成测（env-gated，Sprint 8）。
//
// 运行：export A2A_TEST_DATABASE_URL=postgresql://... && go test ./apps/a2a-gateway/...
//
// 设计与 cardstore_pg_test.go 一致：
//   - 未设 A2A_TEST_DATABASE_URL → t.Skip（CI 无 PG service）
//   - 每个用例 TRUNCATE a2a_acl_policy 避免污染
package a2asrv

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// newTestACLStore 构造 ACLStore；env 未设 → t.Skip。
func newTestACLStore(t *testing.T) (*ACLStore, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("A2A_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("A2A_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("pg ping: %v", err)
	}
	_, _ = pool.Exec(ctx, `TRUNCATE a2a_acl_policy`)
	return NewACLStore(pool), pool
}

// insertDeny 插一条 deny 策略行。
func insertDeny(t *testing.T, pool *pgxpool.Pool, agentID, peerCity string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO a2a_acl_policy (agent_id, peer_city, action, reason)
		 VALUES ($1, $2, 'deny', 'test') ON CONFLICT DO NOTHING`,
		agentID, peerCity)
	if err != nil {
		t.Fatalf("insert deny (%s → %s): %v", agentID, peerCity, err)
	}
}

// ---------- 1) 定向 deny 命中 ----------

func TestACLStore_PG_DenyMatch(t *testing.T) {
	store, pool := newTestACLStore(t)
	defer pool.Close()
	ctx := context.Background()

	insertDeny(t, pool, "alice", "shanghai")

	denied, err := store.IsDenied(ctx, "alice", "shanghai")
	if err != nil {
		t.Fatalf("IsDenied: %v", err)
	}
	if !denied {
		t.Error("alice → shanghai: denied=false, want true")
	}

	// 别的城邦不受影响
	denied, err = store.IsDenied(ctx, "alice", "beijing")
	if err != nil {
		t.Fatalf("IsDenied: %v", err)
	}
	if denied {
		t.Error("alice → beijing: denied=true, want false (policy is shanghai-scoped)")
	}

	// 别的 agent 不受影响
	denied, err = store.IsDenied(ctx, "bob", "shanghai")
	if err != nil {
		t.Fatalf("IsDenied: %v", err)
	}
	if denied {
		t.Error("bob → shanghai: denied=true, want false (policy is alice-scoped)")
	}
}

// ---------- 2) 无策略 → 默认 allow ----------

func TestACLStore_PG_AllowByDefault(t *testing.T) {
	store, pool := newTestACLStore(t)
	defer pool.Close()
	ctx := context.Background()

	denied, err := store.IsDenied(ctx, "alice", "shanghai")
	if err != nil {
		t.Fatalf("IsDenied: %v", err)
	}
	if denied {
		t.Error("empty policy table: denied=true, want false (default allow)")
	}

	// 空 agent_id / 空 city 也不得误判
	if denied, _ := store.IsDenied(ctx, "", "shanghai"); denied {
		t.Error("empty agentID: denied=true, want false")
	}
	if denied, _ := store.IsDenied(ctx, "alice", ""); denied {
		t.Error("empty peerCity with no policy: denied=true, want false")
	}
}

// ---------- 3) peer_city='' = 全局 deny ----------

func TestACLStore_PG_GlobalDeny(t *testing.T) {
	store, pool := newTestACLStore(t)
	defer pool.Close()
	ctx := context.Background()

	insertDeny(t, pool, "alice", "") // 全局：alice 不可发往任何城邦

	for _, city := range []string{"beijing", "shanghai", "shenzhen", ""} {
		denied, err := store.IsDenied(ctx, "alice", city)
		if err != nil {
			t.Fatalf("IsDenied(alice, %q): %v", city, err)
		}
		if !denied {
			t.Errorf("alice → %q: denied=false, want true (global deny)", city)
		}
	}

	// 全局 deny 只作用于 alice
	denied, err := store.IsDenied(ctx, "bob", "beijing")
	if err != nil {
		t.Fatalf("IsDenied: %v", err)
	}
	if denied {
		t.Error("bob → beijing: denied=true, want false (alice-scoped global deny)")
	}
}

// ---------- 4) action != 'deny' 不阻断 ----------

// 'allow' 行是白名单模式的预留，本 sprint 不应被 IsDenied 误判为拒绝。
func TestACLStore_PG_AllowActionIgnored(t *testing.T) {
	store, pool := newTestACLStore(t)
	defer pool.Close()
	ctx := context.Background()

	_, err := pool.Exec(ctx,
		`INSERT INTO a2a_acl_policy (agent_id, peer_city, action) VALUES ('alice', 'shanghai', 'allow')`)
	if err != nil {
		t.Fatalf("insert allow: %v", err)
	}

	denied, err := store.IsDenied(ctx, "alice", "shanghai")
	if err != nil {
		t.Fatalf("IsDenied: %v", err)
	}
	if denied {
		t.Error("action='allow' row: denied=true, want false")
	}
}

// ---------- 5) 经 ACL 门的端到端（PG 后端） ----------

func TestACLStore_PG_ThroughACL_CheckDeliver(t *testing.T) {
	store, pool := newTestACLStore(t)
	defer pool.Close()
	ctx := context.Background()

	acl := NewACL(store)
	alice := &a2av1.AgentCard{AgentId: "alice", Name: "Alice", CityId: "beijing"}
	bob := &a2av1.AgentCard{AgentId: "bob", Name: "Bob", CityId: "shanghai"}

	// 无策略 → 放行
	if err := acl.CheckDeliver(ctx, alice, bob); err != nil {
		t.Errorf("before policy: CheckDeliver = %v, want nil", err)
	}

	insertDeny(t, pool, "alice", "shanghai")

	err := acl.CheckDeliver(ctx, alice, bob)
	if err == nil {
		t.Fatal("after deny policy: CheckDeliver = nil, want F_016")
	}
	if !strings.HasPrefix(err.Error(), "F_016:") {
		t.Errorf("error = %q, want F_016: prefix", err.Error())
	}
}
