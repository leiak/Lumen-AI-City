// Package a2asrv 单元测试：Registry（纯内存 AgentCard 注册表）。
//
// 不依赖 gRPC / 网络 —— 测的是 Map + RWMutex 的并发契约 + 错误码返回。
package a2asrv

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

func newCard(id, name string, caps ...string) *a2av1.AgentCard {
	return &a2av1.AgentCard{
		AgentId:      id,
		Name:         name,
		Capabilities: caps,
		Provider:     "aicity",
	}
}

func TestRegistry_Register_FirstTime_Accepted(t *testing.T) {
	r := NewRegistry()
	ok, errCode := r.Register(newCard("alice", "Alice Agent", "chat"))
	if !ok {
		t.Fatalf("want accepted=true, got %v / %q", ok, errCode)
	}
	if errCode != "" {
		t.Errorf("want errCode empty, got %q", errCode)
	}
	if got := r.Size(); got != 1 {
		t.Errorf("size want 1 got %d", got)
	}
}

func TestRegistry_Register_Duplicate_OverwritesIdempotent(t *testing.T) {
	r := NewRegistry()
	r.Register(newCard("alice", "Alice v1", "chat"))
	// 重复注册应幂等：accepted=true + 覆盖旧 card
	ok, errCode := r.Register(newCard("alice", "Alice v2", "chat", "search"))
	if !ok {
		t.Errorf("duplicate Register want accepted=true (idempotent), got false / %q", errCode)
	}
	if errCode != "F_002" {
		t.Errorf("duplicate Register want errCode=F_002, got %q", errCode)
	}
	if got := r.Size(); got != 1 {
		t.Errorf("size want 1 after dup, got %d", got)
	}
	got, ok := r.Get("alice")
	if !ok {
		t.Fatal("alice should still be present")
	}
	if got.GetName() != "Alice v2" {
		t.Errorf("name want Alice v2 got %q", got.GetName())
	}
	if len(got.GetCapabilities()) != 2 {
		t.Errorf("caps want 2 got %d", len(got.GetCapabilities()))
	}
}

func TestRegistry_Register_MissingFields_ReturnsF001(t *testing.T) {
	r := NewRegistry()

	cases := []struct {
		name string
		card *a2av1.AgentCard
	}{
		{"empty agent_id", &a2av1.AgentCard{Name: "X"}},
		{"empty name", &a2av1.AgentCard{AgentId: "x"}},
		{"both empty", &a2av1.AgentCard{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, errCode := r.Register(tc.card)
			if ok {
				t.Errorf("want accepted=false")
			}
			if errCode != "F_001" {
				t.Errorf("want F_001 got %q", errCode)
			}
			if !strings.HasPrefix(errCode+":", "F_001:") {
				t.Errorf("errCode format should be F_XXX prefix, got %q", errCode)
			}
		})
	}
	if got := r.Size(); got != 0 {
		t.Errorf("size want 0 after rejections, got %d", got)
	}
}

func TestRegistry_Discover_ByCapability(t *testing.T) {
	r := NewRegistry()
	r.Register(newCard("alice", "Alice", "chat"))
	r.Register(newCard("bob", "Bob", "search", "chat"))
	r.Register(newCard("carol", "Carol", "search"))

	chat, errCode := r.Discover("chat", "")
	if errCode != "" {
		t.Errorf("chat want errCode empty, got %q", errCode)
	}
	if len(chat) != 2 {
		t.Errorf("chat want 2 cards got %d", len(chat))
	}
	search, _ := r.Discover("search", "")
	if len(search) != 2 {
		t.Errorf("search want 2 cards got %d", len(search))
	}
	none, _ := r.Discover("nope", "")
	if len(none) != 0 {
		t.Errorf("nope want 0 cards got %d", len(none))
	}

	// capabilities 顺序无关（chat 应同时返回 alice + bob）
	ids := map[string]bool{}
	for _, c := range chat {
		ids[c.GetAgentId()] = true
	}
	if !ids["alice"] || !ids["bob"] {
		t.Errorf("chat should include alice+bob, got %v", ids)
	}
}

func TestRegistry_Discover_EmptyCapability_ReturnsF003(t *testing.T) {
	r := NewRegistry()
	r.Register(newCard("alice", "Alice", "chat"))

	cards, errCode := r.Discover("", "")
	if errCode != "F_003" {
		t.Errorf("want F_003 got %q", errCode)
	}
	if cards != nil {
		t.Errorf("want nil cards on error, got %d", len(cards))
	}
}

func TestRegistry_Get_And_Len(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("ghost"); ok {
		t.Error("ghost should not exist")
	}
	r.Register(newCard("alice", "Alice", "chat"))
	got, ok := r.Get("alice")
	if !ok || got.GetName() != "Alice" {
		t.Errorf("alice lookup failed: ok=%v got=%v", ok, got)
	}
}

func TestRegistry_ConcurrentRegister_NoRace(t *testing.T) {
	// 跑 -race 时应通过；该测试用 go vet 也能看到正确的锁粒度。
	r := NewRegistry()
	const n = 100
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			id := "agent_" + string(rune('A'+i%26)) + "_" + string(rune('0'+i%10))
			r.Register(newCard(id, "n", "chat"))
		}()
	}
	wg.Wait()
	if got := r.Size(); got <= 0 {
		t.Errorf("size want >0 got %d", got)
	}
}

func TestRegistry_Register_InvalidEd25519Pubkey_ReturnsF006(t *testing.T) {
	r := NewRegistry()
	cases := []struct {
		name string
		card *a2av1.AgentCard
	}{
		{"not base64", &a2av1.AgentCard{AgentId: "alice", Name: "Alice",
			Auth: map[string]string{"ed25519": "!!notbase64!!"}}},
		{"wrong length", &a2av1.AgentCard{AgentId: "bob", Name: "Bob",
			Auth: map[string]string{"ed25519": "AAAA"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, errCode := r.Register(tc.card)
			if ok {
				t.Errorf("want accepted=false")
			}
			if errCode != "F_006" {
				t.Errorf("want F_006 got %q", errCode)
			}
			if r.Size() != 0 {
				t.Errorf("registry should remain empty after F_006, got size=%d", r.Size())
			}
		})
	}
}

func TestRegistry_Register_ValidEd25519Pubkey_Accepted(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	r := NewRegistry()
	ok, errCode := r.Register(&a2av1.AgentCard{
		AgentId: "alice", Name: "Alice",
		Auth: map[string]string{"ed25519": base64.StdEncoding.EncodeToString(pub)},
	})
	if !ok || errCode != "" {
		t.Errorf("want accepted=true errCode=\"\", got %v / %q", ok, errCode)
	}
}
