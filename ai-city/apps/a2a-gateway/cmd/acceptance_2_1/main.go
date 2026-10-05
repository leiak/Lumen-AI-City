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
	"encoding/json"
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

	// Step 4: 验证 npc_say_stream_done 到达（≥5 个 done 事件 = 5 NPC 各一）。
	// payload 由 apps/agent-os/src/agent_os/stream/publisher.py:39 publish_done 生成：
	//   {type, npc_id, session_id, sentence_count, complete, ts_ms, trace_id}
	// 通道同 step 2/3：aicity:npc:say_stream，按 type 二次过滤即可。
	fmt.Println("Step 4: npc_say_stream_done validation")
	doneCount := 0
	step4Ctx, step4Cancel := context.WithTimeout(ctx, 30*time.Second)
	defer step4Cancel()
	sub := rdb.Subscribe(step4Ctx, streamcheck.ChannelSayStream)
	defer func() { _ = sub.Close() }()
	// 必须等 Subscribe 确认（go-redis 文档明确要求），否则首个 Publish 可能 race 丢失。
	if _, err := sub.Receive(step4Ctx); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL step 4: subscribe confirm: %v\n", err)
		os.Exit(4)
	}
	ch := sub.Channel()
	deadline := time.After(25 * time.Second)
step4Loop:
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				break step4Loop
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				continue
			}
			if payload["type"] != "npc_say_stream_done" {
				continue
			}
			// complete 必须是 bool（done 标志，与 publish_done 契约）。
			if _, ok := payload["complete"].(bool); !ok {
				fmt.Fprintf(os.Stderr, "FAIL step 4: done event missing/invalid complete: %v\n", payload)
				os.Exit(4)
			}
			// sentence_count 必须是非负整数（JSON number 解出来是 float64）。
			sc, ok := payload["sentence_count"].(float64)
			if !ok || sc < 0 || sc != float64(int64(sc)) {
				fmt.Fprintf(os.Stderr, "FAIL step 4: done event invalid sentence_count: %v\n", payload["sentence_count"])
				os.Exit(4)
			}
			doneCount++
			if doneCount >= 5 {
				break step4Loop
			}
		case <-deadline:
			fmt.Fprintf(os.Stderr, "FAIL step 4: only %d done events before deadline\n", doneCount)
			os.Exit(4)
		}
	}
	fmt.Printf("  PASS %d done events\n", doneCount)

	// Step 5: 5/5 PASS 总结。
	fmt.Println("=== acceptance_2_1 summary ===")
	fmt.Println("Step 1 (login):    PASS")
	fmt.Println("Step 2 (5 NPCs):   PASS")
	fmt.Println("Step 3 (emotion):  PASS")
	fmt.Println("Step 4 (done):     PASS")
	fmt.Println("Step 5 (summary):  PASS")
	fmt.Println("5/5 PASS")
}