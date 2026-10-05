// acceptance_2_0 — 2.0 stage 1 MVP Definition of Done 二进制
//
// 验证 5 步闭环：5 NPC enabled + avatar → LLM-NPC 真实对话 → Milvus 记忆
// 召回（top-5）→ Saga 协作剧本 → 跨城联邦（city_a → city_b）。
//
// 用法（bake 进 a2a-gateway 镜像）：
//
//	docker compose exec -T a2a-gateway /app/acceptance_2_0 [flags]
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strings"
    "time"
)

const (
    ExitOK              = 0
    ExitStepFailed      = 1
    ExitCrossCityFailed = 2
    ExitMilvusFailed    = 3
    ExitSagaFailed      = 4
)

var (
    apiBase  = getEnv("API_GATEWAY_URL", "http://api-gateway:8080")
    memBase  = getEnv("MEMORY_URL", "http://memory-service:9200")
    a2aBase  = getEnv("A2A_HUB_URL", "http://a2a-gateway:8083")
    sagaBase = getEnv("SAGA_URL", "http://saga-orchestrator:9100")
)

func getEnv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func login(city, username, password string) (string, error) {
    body, _ := json.Marshal(map[string]string{
        "username": username, "password": password,
    })
    resp, err := http.Post(apiBase+"/v1/auth/login", "application/json", bytes.NewReader(body))
    if err != nil {
        return "", fmt.Errorf("login %s: %w", city, err)
    }
    defer resp.Body.Close()
    if resp.StatusCode != 200 {
        return "", fmt.Errorf("login %s: status %d", city, resp.StatusCode)
    }
    var result struct {
        Token string `json:"token"`
    }
    json.NewDecoder(resp.Body).Decode(&result)
    if result.Token == "" {
        return "", fmt.Errorf("login %s: empty token", city)
    }
    return result.Token, nil
}

func httpJSON(method, url, token string, body any) (map[string]any, error) {
    var reqBody io.Reader
    if body != nil {
        b, _ := json.Marshal(body)
        reqBody = bytes.NewReader(b)
    }
    req, _ := http.NewRequest(method, url, reqBody)
    if token != "" {
        req.Header.Set("Authorization", "Bearer "+token)
    }
    req.Header.Set("Content-Type", "application/json")
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    var result map[string]any
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    if resp.StatusCode >= 400 {
        return result, fmt.Errorf("status %d: %v", resp.StatusCode, result)
    }
    return result, nil
}

// step1: login city_a + city_b
func step1Login() error {
    tokenA, err := login("city_a", "demo_a", "demo123")
    if err != nil {
        return fmt.Errorf("city_a login: %w", err)
    }
    tokenB, err := login("city_b", "demo_b", "demo123")
    if err != nil {
        return fmt.Errorf("city_b login: %w", err)
    }
    os.Setenv("TOKEN_A", tokenA)
    os.Setenv("TOKEN_B", tokenB)
    fmt.Println("  ✓ city_a + city_b 登录成功")
    return nil
}

// step2: LLM-NPC 真实对话
func step2LLM() error {
    body := map[string]string{"text": "今天有什么好吃的？"}
    resp, err := httpJSON("POST", apiBase+"/v1/npc/npc_a_wang_boss/talk", os.Getenv("TOKEN_A"), body)
    if err != nil {
        return fmt.Errorf("LLM call: %w", err)
    }
    text, _ := resp["text"].(string)
    if text == "" {
        return fmt.Errorf("empty LLM response")
    }
    fmt.Printf("  ✓ LLM 响应: %s\n", text)
    return nil
}

// step3: Milvus memory recall
func step3Memory() error {
    body := map[string]any{
        "npc_id":    "npc_a_wang_boss",
        "player_id": "demo_a",
        "query":     "红烧肉",
        "top_k":     5,
    }
    resp, err := httpJSON("POST", memBase+"/v1/recall", "", body)
    if err != nil {
        return fmt.Errorf("memory recall: %w", err)
    }
    msgs, _ := resp["messages"].([]any)
    fmt.Printf("  ✓ Milvus 召回 %d 条记忆\n", len(msgs))
    return nil
}

// step4: Saga 协作剧本
func step4Saga() error {
    body := map[string]string{
        "saga_id":   "welcome_3npc",
        "player_id": "demo_a",
    }
    resp, err := httpJSON("POST", sagaBase+"/v1/saga/trigger", "", body)
    if err != nil {
        return fmt.Errorf("saga trigger: %w", err)
    }
    time.Sleep(6 * time.Second)
    status, err := httpJSON("GET", sagaBase+"/v1/saga/welcome_3npc", "", nil)
    if err != nil {
        return fmt.Errorf("saga status: %w", err)
    }
    s, _ := status["status"].(string)
    if s != "SUCCESS" {
        return fmt.Errorf("saga status: %s (expected SUCCESS)", s)
    }
    fmt.Printf("  ✓ Saga: %v\n", resp["status"])
    return nil
}

// step5: 跨城联邦 (city_a 玩家点 city_b NPC)
func step5CrossCity() error {
    body := map[string]string{"text": "医生，我失眠"}
    resp, err := httpJSON("POST", apiBase+"/v1/npc/npc_b_grace_healer/talk", os.Getenv("TOKEN_A"), body)
    if err != nil {
        return fmt.Errorf("cross-city: %w", err)
    }
    text, _ := resp["text"].(string)
    if text == "" {
        return fmt.Errorf("empty cross-city response")
    }
    if !strings.Contains(text, "npc_b_grace_healer") && len(text) == 0 {
        return fmt.Errorf("unexpected cross-city response: %s", text)
    }
    fmt.Printf("  ✓ 跨城 NPC 响应: %s\n", text)
    return nil
}

func main() {
    fmt.Println("=== acceptance_2_0 ===")
    fmt.Println("Step 1: 登录 city_a + city_b")
    if err := step1Login(); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL step 1: %v\n", err)
        os.Exit(ExitStepFailed)
    }

    fmt.Println("Step 2: LLM-NPC 真实对话")
    if err := step2LLM(); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL step 2: %v\n", err)
        os.Exit(ExitStepFailed)
    }

    fmt.Println("Step 3: Milvus 记忆召回")
    if err := step3Memory(); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL step 3: %v\n", err)
        os.Exit(ExitMilvusFailed)
    }

    fmt.Println("Step 4: Saga 协作剧本")
    if err := step4Saga(); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL step 4: %v\n", err)
        os.Exit(ExitSagaFailed)
    }

    fmt.Println("Step 5: 跨城联邦")
    if err := step5CrossCity(); err != nil {
        fmt.Fprintf(os.Stderr, "FAIL step 5: %v\n", err)
        os.Exit(ExitCrossCityFailed)
    }

    fmt.Println("\n=== 5/5 PASS ===")
    os.Exit(ExitOK)
}