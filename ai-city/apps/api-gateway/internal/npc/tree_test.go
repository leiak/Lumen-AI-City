package npc

import (
	"os"
	"path/filepath"
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
func TestLoadFromFile_ParsesNestedTalkTree(t *testing.T) {
	yaml := `npc_id: npc_wang_boss_001
home_tile_id: tile_0_0
say:
  greeting:
    - "来了您嘞！"
talk_tree:
  initial: root
  default_say: "您先看着。"
  nodes:
    root:
      say: "来了您嘞！今儿喝点什么？"
      options:
        - {id: ask_food, text: "有什么招牌菜？"}
        - {id: leave, text: "我先走了"}
    ask_food:
      say: "老北京炸酱面，十八块一碗。"
      options:
        - {id: leave, text: "来一碗"}
    leave:
      say: "慢走啊。"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "wang_boss.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	tree, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	// nested talk_tree block normalized into flat fields
	if tree.InitialChoice() != "root" {
		t.Errorf("InitialChoice = %q, want root", tree.InitialChoice())
	}
	if tree.HomeTile != "tile_0_0" {
		t.Errorf("HomeTile = %q, want tile_0_0 (from home_tile_id)", tree.HomeTile)
	}
	if tree.DefaultSay != "您先看着。" {
		t.Errorf("DefaultSay = %q, want 您先看着。", tree.DefaultSay)
	}
	// real choice resolves to its reply + options
	node, ok := tree.Lookup("ask_food")
	if !ok {
		t.Fatal("expected to find ask_food")
	}
	if node.Say != "老北京炸酱面，十八块一碗。" {
		t.Errorf("ask_food.Say = %q", node.Say)
	}
	if len(node.Options) != 1 || node.Options[0].ID != "leave" {
		t.Errorf("ask_food options = %+v", node.Options)
	}
	if !tree.HasChoice("root") {
		t.Errorf("expected root to be a choice")
	}
}

func TestLoadFromFile_ParsesLegacyInline(t *testing.T) {
	yaml := `npc_id: npc_lihua_002
home_tile: tile_1_0
nodes:
  greet:
    say: "哎，回来啦。"
    options:
      - {id: bye, text: "回见"}
`
	dir := t.TempDir()
	path := filepath.Join(dir, "lihua.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	tree, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if tree.HomeTile != "tile_1_0" {
		t.Errorf("HomeTile = %q, want tile_1_0", tree.HomeTile)
	}
	if node, ok := tree.Lookup("greet"); !ok || node.Say != "哎，回来啦。" {
		t.Errorf("greet node = %+v, ok=%v", node, ok)
	}
}

func TestTree_InitialNode(t *testing.T) {
	tree := &Tree{
		NpcID: "npc_wang_boss_001",
		Name:  "王老板",
		Nodes: map[string]Node{
			"root":  {Say: "来了您嘞！"},
			"other": {Say: "不看"},
		},
		Initial: "root",
	}
	node, ok := tree.InitialNode()
	if !ok {
		t.Fatal("expected InitialNode to hit root")
	}
	if node.Say != "来了您嘞！" {
		t.Errorf("InitialNode.Say = %q", node.Say)
	}
	// name parsed at top-level yaml
	yaml := "npc_id: npc_wang_boss_001\nname: 王老板\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "n.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	parsed, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if parsed.Name != "王老板" {
		t.Errorf("parsed.Name = %q, want 王老板", parsed.Name)
	}
}
