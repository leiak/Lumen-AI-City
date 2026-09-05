// InboxStore PG 集成测（env-gated）。
//
// 运行：export A2A_TEST_DATABASE_URL=postgresql://... && go test ./apps/a2a-gateway/...
//
// 设计：
//   - 未设 A2A_TEST_DATABASE_URL → t.Skip（CI 无 PG service）
//   - 每个用例清空 a2a_inbox 表
package a2asrv

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestInboxStore(t *testing.T) (*InboxStore, *pgxpool.Pool) {
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
	_, _ = pool.Exec(ctx, `TRUNCATE a2a_inbox`)
	return NewInboxStore(pool), pool
}

func makeEntry(msgID, from, to string) *InboxEntry {
	return &InboxEntry{
		MessageID:      msgID,
		ConversationID: "conv-1",
		FromAgentID:    from,
		ToAgentID:      to,
		Type:           "request",
		Payload:        []byte("hello"),
		TsMs:           time.Now().UnixMilli(),
		TraceID:        "trace-1",
		Signature:      "sig-1",
	}
}

// ---------- 1) Append OK ----------

func TestInboxStore_PG_Append_OK(t *testing.T) {
	store, pool := newTestInboxStore(t)
	defer pool.Close()
	ctx := context.Background()

	err := store.Append(ctx, makeEntry("m1", "alice", "bob"), "")
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if n := store.Count(ctx, "bob", true); n != 1 {
		t.Errorf("Count(bob, unread) = %d, want 1", n)
	}
}

// ---------- 2) Fetch 未读 ----------

func TestInboxStore_PG_Fetch_Unread(t *testing.T) {
	store, pool := newTestInboxStore(t)
	defer pool.Close()
	ctx := context.Background()

	store.Append(ctx, makeEntry("m1", "alice", "bob"), "")
	store.Append(ctx, makeEntry("m2", "alice", "bob"), "")
	store.Append(ctx, makeEntry("m3", "carol", "bob"), "")
	store.Append(ctx, makeEntry("m4", "alice", "ghost"), "") // 不同收件方

	msgs, next, err := store.Fetch(ctx, "bob", 10, "", false)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(msgs) != 3 {
		t.Errorf("Fetch bob want 3 got %d", len(msgs))
	}
	if next != "" {
		t.Errorf("next_cursor should be empty, got %q", next)
	}
	// 顺序：m3 → m2 → m1（queued_at DESC）
	if msgs[0].GetMessageId() != "m3" {
		t.Errorf("first msg = %q, want m3 (latest)", msgs[0].GetMessageId())
	}
}

// ---------- 3) Fetch mark_read → read_at 设置 ----------

func TestInboxStore_PG_Fetch_MarkRead(t *testing.T) {
	store, pool := newTestInboxStore(t)
	defer pool.Close()
	ctx := context.Background()

	store.Append(ctx, makeEntry("m1", "alice", "bob"), "")

	msgs, _, err := store.Fetch(ctx, "bob", 10, "", true)
	if err != nil {
		t.Fatalf("Fetch mark_read: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d msgs", len(msgs))
	}

	// 再拉 → 未读 0 条
	if n := store.Count(ctx, "bob", true); n != 0 {
		t.Errorf("after mark_read unread count = %d, want 0", n)
	}
}

// ---------- 4) Cursor 增量 ----------

func TestInboxStore_PG_Fetch_Cursor(t *testing.T) {
	store, pool := newTestInboxStore(t)
	defer pool.Close()
	ctx := context.Background()

	// 3 条时间错开
	for i, id := range []string{"m1", "m2", "m3"} {
		store.Append(ctx, makeEntry(id, "alice", "bob"), "")
		time.Sleep(2 * time.Millisecond) // 确保 queued_at 递增
		_ = i
	}

	// limit=1 → 应拿到最新 + next_cursor
	msgs, next, err := store.Fetch(ctx, "bob", 1, "", false)
	if err != nil {
		t.Fatalf("Fetch limit=1: %v", err)
	}
	if len(msgs) != 1 || msgs[0].GetMessageId() != "m3" {
		t.Errorf("first page should be [m3], got %+v", msgs)
	}
	if next == "" {
		t.Fatal("next_cursor should be non-empty")
	}

	// 用 cursor 拉下一页
	msgs2, next2, err := store.Fetch(ctx, "bob", 10, next, false)
	if err != nil {
		t.Fatalf("Fetch with cursor: %v", err)
	}
	if len(msgs2) != 2 {
		t.Errorf("second page should be 2 messages, got %d", len(msgs2))
	}
	if next2 != "" {
		t.Errorf("second page next_cursor should be empty, got %q", next2)
	}
	// 顺序：m2, m1
	if msgs2[0].GetMessageId() != "m2" || msgs2[1].GetMessageId() != "m1" {
		t.Errorf("second page order wrong: %+v", msgs2)
	}
}

// ---------- 5) Fetch 不存在 agent → 空 ----------

func TestInboxStore_PG_Fetch_NoMessages(t *testing.T) {
	store, pool := newTestInboxStore(t)
	defer pool.Close()
	ctx := context.Background()

	msgs, next, err := store.Fetch(ctx, "ghost", 10, "", false)
	if err != nil {
		t.Fatalf("Fetch ghost: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("ghost should have 0 msgs, got %d", len(msgs))
	}
	if next != "" {
		t.Errorf("next_cursor should be empty, got %q", next)
	}
}
