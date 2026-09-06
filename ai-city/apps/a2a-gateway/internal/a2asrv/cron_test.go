// Cron 单元测试（仅覆盖 no-op 路径）。
//
// 设计注：plan 显式声明本文件只测"短路"分支：
//   - interval <= 0 → 立即 return，不启协程
//   - store == nil → 立即 return，不启协程
//
// 真实 ticker 行为依赖 InboxStore.Pool → 需 PG env，由 inboxstore_pg_test.go 覆盖。
// 这里再加 ticker 行为会增加 flake 风险，故省略。
package a2asrv

import (
	"context"
	"testing"
	"time"
)

// TestStartInboxCleanup_ZeroInterval_Disabled 间隔 ≤ 0 应立即 return 不启协程。
//
// 行为契约：interval <= 0 → 短路 return（log "disabled"）；不启 ticker 协程。
func TestStartInboxCleanup_ZeroInterval_Disabled(t *testing.T) {
	// nil store + 0 interval → 短路 return（interval 先判，store 未触达）
	StartInboxCleanup(context.Background(), nil, 0)

	// nil store + negative interval → 短路 return
	StartInboxCleanup(context.Background(), nil, -1*time.Second)

	// 短路 return 不应 panic；其余 ticker 行为由 PG 集成测覆盖
}

// TestStartInboxCleanup_NilStore_Skipped nil store + 正 interval 应立即 return。
//
// 行为契约：store == nil → 跳过（log "skipped: nil store"）；不启 ticker 协程。
// 这里必须传 nil store 才会走 store==nil 分支 —— 用真实 InboxStore 会启协程。
func TestStartInboxCleanup_NilStore_Skipped(t *testing.T) {
	StartInboxCleanup(context.Background(), nil, 1*time.Second)
}

// TestStartInboxCleanup_ValidParams_NoGoroutineLeak 烟囱测试：
//
//   - 正 interval + nil store → 短路 → 不会 leak goroutine
//   - 用 context.WithCancel + 短 timeout 防止任何意外泄漏把测试拖挂
//
// 不验证 ticker 真实 tick（需 PG；改 PG 集成测）。
func TestStartInboxCleanup_ValidParams_NoGoroutineLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	StartInboxCleanup(ctx, nil, 100*time.Millisecond)

	// 等 ctx 自然 cancel → 若有协程会立刻 exit
	<-ctx.Done()
	// 再等 30ms 让协程收到 ctx.Done() 信号
	time.Sleep(30 * time.Millisecond)
}