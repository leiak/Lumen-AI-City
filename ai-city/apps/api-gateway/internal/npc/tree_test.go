package npc

import (
	"testing"
)

func TestTree_Lookup_ReturnsSay(t *testing.T) {
	tree := &Tree{
		NpcID:      "npc_wang_boss_001",
		HomeTile:   "tile_1_1",
		DefaultSay: "来了您嘞！",
		Nodes: map[string]Node{
			"ask_business": {
				Say: "小店经营杂货。",
				Options: []Option{
					{ID: "yes_browse", Text: "我看看"},
					{ID: "no_leave", Text: "改天再来"},
				},
			},
			"yes_browse": {Say: "好嘞。", Options: nil},
		},
	}
	node, ok := tree.Lookup("ask_business")
	if !ok {
		t.Fatal("expected to find ask_business")
	}
	if node.Say != "小店经营杂货。" {
		t.Errorf("say = %q, want 小店经营杂货。", node.Say)
	}
	if len(node.Options) != 2 {
		t.Errorf("options len = %d, want 2", len(node.Options))
	}
}

func TestTree_Lookup_MissingReturnsDefault(t *testing.T) {
	tree := &Tree{
		NpcID:      "npc_x",
		DefaultSay: "...",
		Nodes:      map[string]Node{},
	}
	node, ok := tree.Lookup("nonexistent_choice")
	if ok {
		t.Errorf("expected miss for unknown choice")
	}
	if node.Say != "" {
		t.Errorf("expected zero-value Node, got Say=%q", node.Say)
	}
	if tree.DefaultSay != "..." {
		t.Errorf("default say mismatch")
	}
}

func TestTree_HasChoice(t *testing.T) {
	tree := &Tree{
		NpcID: "npc_x",
		Nodes: map[string]Node{
			"greet": {Say: "hi"},
		},
	}
	if !tree.HasChoice("greet") {
		t.Error("expected greet to be a choice")
	}
	if tree.HasChoice("unknown") {
		t.Error("expected unknown to NOT be a choice")
	}
}