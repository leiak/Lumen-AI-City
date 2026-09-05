// CardStore PG 集成测（env-gated）。
//
// 运行：export A2A_TEST_DATABASE_URL=postgresql://... && go test ./apps/a2a-gateway/...
//
// 设计：
//   - 未设 A2A_TEST_DATABASE_URL → t.Skip（CI 无 PG service）
//   - 每个用例清空 a2a_agent_card 表（TRUNCATE）避免污染
//   - 与 Sprint 6 内存 Registry 行为兼容（重复 Register 返 F_002）
package a2asrv

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// newTestCardStore 构造 CardStore；env 未设 → t.Skip。
func newTestCardStore(t *testing.T) (*CardStore, *pgxpool.Pool) {
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
	// 清表
	_, _ = pool.Exec(ctx, `TRUNCATE a2a_agent_card`)
	return NewCardStore(pool), pool
}

// ---------- 1) Register OK ----------

func TestCardStore_PG_Register_OK(t *testing.T) {
	store, pool := newTestCardStore(t)
	defer pool.Close()
	ctx := context.Background()

	ok, code := store.Register(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Provider: "aicity", Capabilities: []string{"chat"},
	})
	if !ok || code != "" {
		t.Errorf("first register: ok=%v code=%q, want true/\"\"", ok, code)
	}

	// 校验已存
	card, ok := store.Get(ctx, "alice")
	if !ok {
		t.Fatal("alice not found after register")
	}
	if card.GetName() != "Alice" {
		t.Errorf("name = %q, want Alice", card.GetName())
	}
}

// ---------- 2) Register 缺字段 → F_001 ----------

func TestCardStore_PG_Register_MissingFields_F001(t *testing.T) {
	store, pool := newTestCardStore(t)
	defer pool.Close()
	ctx := context.Background()

	// 缺 name
	ok, code := store.Register(ctx, &a2av1.AgentCard{AgentId: "alice"})
	if ok || code != "F_001" {
		t.Errorf("missing name: ok=%v code=%q, want false/F_001", ok, code)
	}

	// 缺 agent_id
	ok, code = store.Register(ctx, &a2av1.AgentCard{Name: "Alice"})
	if ok || code != "F_001" {
		t.Errorf("missing agent_id: ok=%v code=%q, want false/F_001", ok, code)
	}
}

// ---------- 3) Register 重复 → F_002（幂等覆盖） ----------

func TestCardStore_PG_Register_Duplicate_F002(t *testing.T) {
	store, pool := newTestCardStore(t)
	defer pool.Close()
	ctx := context.Background()

	store.Register(ctx, &a2av1.AgentCard{AgentId: "alice", Name: "Alice"})
	ok, code := store.Register(ctx, &a2av1.AgentCard{AgentId: "alice", Name: "Alice2"})
	if !ok || code != "F_002" {
		t.Errorf("duplicate: ok=%v code=%q, want true/F_002", ok, code)
	}
	// 覆盖后应是 Alice2
	card, _ := store.Get(ctx, "alice")
	if card.GetName() != "Alice2" {
		t.Errorf("after overwrite name = %q, want Alice2", card.GetName())
	}
}

// ---------- 4) Get 找到 ----------

func TestCardStore_PG_Get_Found(t *testing.T) {
	store, pool := newTestCardStore(t)
	defer pool.Close()
	ctx := context.Background()

	store.Register(ctx, &a2av1.AgentCard{
		AgentId: "bob", Name: "Bob", Provider: "openclaw",
		Url: "https://openclaw.example.com/bob",
	})
	card, ok := store.Get(ctx, "bob")
	if !ok {
		t.Fatal("bob not found")
	}
	if card.GetProvider() != "openclaw" {
		t.Errorf("provider = %q, want openclaw", card.GetProvider())
	}
	if card.GetUrl() != "https://openclaw.example.com/bob" {
		t.Errorf("url = %q", card.GetUrl())
	}
}

// ---------- 5) Discover by capability ----------

func TestCardStore_PG_Discover_ByCapability(t *testing.T) {
	store, pool := newTestCardStore(t)
	defer pool.Close()
	ctx := context.Background()

	store.Register(ctx, &a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Capabilities: []string{"chat", "search"},
	})
	store.Register(ctx, &a2av1.AgentCard{
		AgentId: "bob", Name: "Bob",
		Capabilities: []string{"chat"},
	})
	store.Register(ctx, &a2av1.AgentCard{
		AgentId: "carol", Name: "Carol",
		Capabilities: []string{"search"},
	})

	cards, code := store.Discover(ctx, "chat", "")
	if code != "" {
		t.Fatalf("Discover code=%q", code)
	}
	if len(cards) != 2 {
		t.Errorf("Discover chat want 2 got %d", len(cards))
	}

	cards, code = store.Discover(ctx, "search", "")
	if len(cards) != 2 {
		t.Errorf("Discover search want 2 got %d", len(cards))
	}

	cards, code = store.Discover(ctx, "nonexistent", "")
	if len(cards) != 0 {
		t.Errorf("Discover nonexistent want 0 got %d", len(cards))
	}
}

// ---------- 6) Register bad pubkey → F_006 ----------

func TestCardStore_PG_Register_BadPubkey_F006(t *testing.T) {
	store, pool := newTestCardStore(t)
	defer pool.Close()
	ctx := context.Background()

	ok, code := store.Register(ctx, &a2av1.AgentCard{
		AgentId: "evil", Name: "Evil",
		Auth: map[string]string{"ed25519": "!!notbase64!!"},
	})
	if ok || code != "F_006" {
		t.Errorf("bad pubkey: ok=%v code=%q, want false/F_006", ok, code)
	}
}
