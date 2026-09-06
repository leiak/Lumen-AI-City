// 后台 cron（Sprint 7+）。
//
// StartInboxCleanup 启动 a2a_inbox TTL cleanup fire-and-forget 协程。
//
// 设计（与 api-gateway cmd/main.go subscriber 同模式）：
//   - ctx.Done() 立即退出（SIGINT 响应快）
//   - Cleanup SQL 用独立 context.WithTimeout(30s)（让最后一次能跑完）
//   - interval <= 0 → 禁用（log 一行；让运维可一键关）
//   - store == nil → 跳过（log 一行；test mode）
//
// 失败容忍：Cleanup 失败仅 log（不返回 error；fire-and-forget 模型）。
package a2asrv

import (
	"context"
	"log"
	"time"
)

// StartInboxCleanup 启动后台 cleanup 协程。interval=0 禁用；store=nil 跳过。
//
// 协程退出条件：ctx.Done() 触发（SIGINT/SIGTERM）。
// 单次 SQL 用 30s 独立 timeout 包住，防止 cron 撞 ctx 超时正被取消导致 DELETE 半截。
func StartInboxCleanup(ctx context.Context, store *InboxStore, interval time.Duration) {
	if interval <= 0 {
		log.Printf("[a2asrv] inbox cleanup disabled (interval <= 0)")
		return
	}
	if store == nil {
		log.Printf("[a2asrv] inbox cleanup skipped: nil store")
		return
	}
	log.Printf("[a2asrv] inbox cleanup started: interval=%s", interval)

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Printf("[a2asrv] inbox cleanup stopped (ctx done)")
				return
			case <-t.C:
				cctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				n, err := store.Cleanup(cctx)
				cancel()
				if err != nil {
					log.Printf("[a2asrv] inbox cleanup: deleted=%d err=%v", n, err)
				} else if n > 0 {
					log.Printf("[a2asrv] inbox cleanup: deleted=%d", n)
				}
			}
		}
	}()
}