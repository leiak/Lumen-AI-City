// acceptance_2_1 — 2.0 stage 2 (NPC 流式情感) Definition of Done 二进制
//
// 验证 5 步闭环：5 NPC 登录 → 5 NPC 流式 sentence_idx 0/1/2 → emotion ∈ 8 类
// → npc_say_stream_done 到达 → 5/5 PASS。
//
// 设计：docs/superpowers/plans/2026-10-05-2.0-stage2-stream-emotion.md (T15-T17)。
// - 复用 internal/streamcheck.Validator (T14) 做登录 + Redis 订阅。
// - 退出码 0=pass / 1=step fail / N=对应 step 失败。
// - T15 只实现 step 1；step 2-5 在 T16/T17 补齐（TODO 占位）。
//
// 用法（bake 进 a2a-gateway 镜像）：
//
//	docker compose exec -T a2a-gateway /app/acceptance_2_1 [flags]
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/aicity/a2a-gateway/internal/streamcheck"
)

const (
	apiBase = "http://api-gateway:8080"
	player  = "demo"
	pass    = "demo123"
)

var npcIDs = []string{
	"npc_wang_boss_001", "npc_grace_healer_001",
	"npc_snack_owner_001", "npc_book_keeper_001",
	"npc_dance_leader_001",
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: "redis:6379"})
	v := streamcheck.New(rdb, apiBase)

	// Step 1: 登录
	fmt.Println("Step 1: login 5 NPCs")
	token, err := v.Login(ctx, player, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL step 1: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  PASS login as %s → token=%s...\n", player, token[:8])

	// Step 2-5: 见后续 task
	fmt.Println("Step 2-5: TODO (后续 task 补)")
}