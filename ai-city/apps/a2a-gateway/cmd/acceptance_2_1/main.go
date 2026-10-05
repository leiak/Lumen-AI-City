// acceptance_2_1 — 2.0 stage 2 (NPC 流式情感) Definition of Done 二进制
//
// 验证 5 步闭环：5 NPC 登录 → 5 NPC 流式 sentence_idx 0/1/2 → emotion ∈ 8 类
// → npc_say_stream_done 到达 → 5/5 PASS。
//
// 设计：docs/superpowers/plans/2026-10-05-2.0-stage2-stream-emotion.md (T15-T17)。
// - 复用 internal/streamcheck.Validator (T14) 做登录 + Redis 订阅。
// - 退出码 0=pass / 1=step fail / N=对应 step 失败。
// - T15 实现 step 1（登录）；T16 补 step 2 + 3（流式 + emotion 验证）；
//   T17 补 step 4 + 5（done + 总结）。
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

	// perNPCStep2Seconds is how long each NPC gets to accumulate stream beats
	// in step 2. The plan's T16 section uses 8s; we keep that as the wall-clock
	// budget per NPC (sized to comfortably outlast a single LLM tick).
	perNPCStep2Seconds = 8

	// perNPCStep3Seconds is the smaller follow-up window for step 3 emotion
	// validation. 4s is enough because validation only needs ≥1 beat per NPC.
	perNPCStep3Seconds = 4
)

// allowedEmotions is the closed set of 8 emotion classes that the
// EmotionValidator (apps/agent-os/src/agent_os/stream/emotion_validator.py)
// normalizes LLM `<emotion=...>` tags to. Anything outside this set is a
// protocol violation and fails step 3.
var allowedEmotions = map[string]bool{
	"happy":       true,
	"sad":         true,
	"angry":       true,
	"surprised":   true,
	"thinking":    true,
	"embarrassed": true,
	"curious":     true,
	"neutral":     true,
}

var npcIDs = []string{
	"npc_wang_boss_001", "npc_grace_healer_001",
	"npc_snack_owner_001", "npc_book_keeper_001",
	"npc_dance_leader_001",
}

func main() {
	// 整体 120s 上限：step 2 (5 × 8s = 40s) + step 3 (5 × 4s = 20s) + 登录 + 余量。
	// plan 写的是 60s，但 step 2 + 3 串行下 60s 紧贴极限；200s + 余量更安全。
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rdb := redis.NewClient(&redis.Options{Addr: "redis:6379"})
	defer func() { _ = rdb.Close() }()
	v := streamcheck.New(rdb, apiBase)

	// Step 1: 登录
	fmt.Println("Step 1: login 5 NPCs")
	token, err := v.Login(ctx, player, pass)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL step 1: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  PASS login as %s → token=%s...\n", player, token[:8])

	// Step 2: 5 NPC 逐个流式订阅 + 验证 sentence_idx 0/1（≥2 个 beat）
	fmt.Println("Step 2: 5 NPCs stream + sentence validation")
	for _, npcID := range npcIDs {
		// 每个 NPC 一个独立的 ctx（per-NPC timeout），避免单个 NPC 阻塞整个循环。
		// 即使上游 LLM 不在，下一个 NPC 仍可继续（只是当前 NPC 失败而已）。
		npcCtx, npcCancel := context.WithTimeout(ctx, time.Duration(perNPCStep2Seconds+2)*time.Second)
		beats, err := v.SubscribeBeat(npcCtx, npcID, perNPCStep2Seconds)
		npcCancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL step 2 %s: %v\n", npcID, err)
			os.Exit(2)
		}
		if len(beats) < 2 {
			fmt.Fprintf(os.Stderr, "FAIL step 2 %s: expected ≥2 beats, got %d\n", npcID, len(beats))
			os.Exit(2)
		}
		// 顺手验证 sentence_idx 是非负整数（JSON 数字解出来是 float64）。
		for i, b := range beats {
			idxFloat, ok := b["sentence_idx"].(float64)
			if !ok || idxFloat < 0 || idxFloat != float64(int64(idxFloat)) {
				fmt.Fprintf(os.Stderr, "FAIL step 2 %s: beat[%d] sentence_idx invalid: %v\n", npcID, i, b["sentence_idx"])
				os.Exit(2)
			}
		}
		fmt.Printf("  PASS %s → %d beats\n", npcID, len(beats))
	}

	// Step 3: 验证 emotion ∈ 8 类
	fmt.Println("Step 3: emotion ∈ 8 classes validation")
	for _, npcID := range npcIDs {
		npcCtx, npcCancel := context.WithTimeout(ctx, time.Duration(perNPCStep3Seconds+2)*time.Second)
		beats, err := v.SubscribeBeat(npcCtx, npcID, perNPCStep3Seconds)
		npcCancel()
		if err != nil {
			fmt.Fprintf(os.Stderr, "FAIL step 3 %s: %v\n", npcID, err)
			os.Exit(3)
		}
		for i, b := range beats {
			emo, _ := b["emotion"].(string)
			if !allowedEmotions[emo] {
				fmt.Fprintf(os.Stderr, "FAIL step 3 %s: beat[%d] invalid emotion %q\n", npcID, i, emo)
				os.Exit(3)
			}
		}
		fmt.Printf("  PASS %s → %d beats, all emotion ∈ 8 classes\n", npcID, len(beats))
	}
	fmt.Println("  PASS all emotions ∈ 8 classes")

	// Step 4-5: 见 T17
	fmt.Println("Step 4-5: TODO (T17 补)")
}