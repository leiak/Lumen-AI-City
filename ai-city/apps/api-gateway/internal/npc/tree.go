// Package npc parses NPC talk_tree config for api-gateway /v1/npc/talk handler.
//
// Sprint 12 min slice: YAML loaded once at startup, in-memory map lookup.
// Future (Sprint 13+): back with PG + cache.
package npc

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Option is one selectable choice exposed by a Node.
type Option struct {
	ID   string `yaml:"id" json:"id"`
	Text string `yaml:"text" json:"text"`
}

// Node is one dialogue state. Say is the NPC's spoken text;
// Options are the choices the player can reply with (empty = terminal).
type Node struct {
	Say     string   `yaml:"say" json:"say"`
	Options []Option `yaml:"options,omitempty" json:"options,omitempty"`
}

// Tree is a single NPC's full dialogue graph.
//
// Nodes is keyed by choice_id — i.e. the id the player sent on the previous
// turn (or the NPC's "root" choice_id for the first turn).
//
// The canonical source is the shared `talk_tree:` block in
// packages/npc-templates/*.yaml (also parsed by agent-os), so LoadFromFile
// normalizes that nested block into these flat fields. Legacy top-level
// `nodes:` / `home_tile` are still honoured for backward compatibility.
type Tree struct {
	NpcID      string          `yaml:"npc_id" json:"npc_id"`
	Name       string          `yaml:"name" json:"name"`
	HomeTile   string          `yaml:"home_tile" json:"home_tile"`
	HomeTileID string          `yaml:"home_tile_id" json:"-"` // alias used by world-engine / agent-os templates
	DefaultSay string          `yaml:"default_say" json:"default_say"`
	Nodes      map[string]Node `yaml:"nodes" json:"nodes"` // legacy inline nodes
	TalkTree   *TalkTreeBlock  `yaml:"talk_tree" json:"-"` // preferred shared block
	Initial    string          `yaml:"-" json:"initial"`   // first-turn choice_id (root)
}

// TalkTreeBlock is the nested `talk_tree:` section that both api-gateway and
// agent-os read from the same packages/npc-templates/*.yaml files. It mirrors
// agent_os.npc_registry.TalkTree.
type TalkTreeBlock struct {
	Initial    string          `yaml:"initial"`
	DefaultSay string          `yaml:"default_say"`
	Nodes      map[string]Node `yaml:"nodes"`
}

// InitialChoice returns the choice_id to seed a fresh conversation from (the
// root node). Returns "" when the tree has no explicit root.
func (t *Tree) InitialChoice() string {
	return t.Initial
}

// InitialNode returns the root node (InitialChoice) that seeds a fresh
// conversation, plus false if the tree has no root node.
func (t *Tree) InitialNode() (Node, bool) {
	if t.Initial == "" {
		return Node{}, false
	}
	return t.Lookup(t.Initial)
}

// Lookup returns the Node for a given choice_id.
// Returns (Node{}, false) if the choice is not in the tree.
func (t *Tree) Lookup(choiceID string) (Node, bool) {
	if n, ok := t.Nodes[choiceID]; ok {
		return n, true
	}
	return Node{}, false
}

// HasChoice reports whether the tree has a node for the given choice_id.
func (t *Tree) HasChoice(choiceID string) bool {
	_, ok := t.Nodes[choiceID]
	return ok
}

// normalize folds the preferred nested `talk_tree:` block (and the
// world-engine style `home_tile_id` alias) into Tree's flat fields, so the
// handler only ever reads Tree.Nodes / Tree.HomeTile.
func normalize(t *Tree) {
	if len(t.Nodes) == 0 && t.TalkTree != nil {
		t.Nodes = t.TalkTree.Nodes
		if t.Initial == "" {
			t.Initial = t.TalkTree.Initial
		}
		if t.DefaultSay == "" {
			t.DefaultSay = t.TalkTree.DefaultSay
		}
	}
	if t.HomeTile == "" {
		t.HomeTile = t.HomeTileID
	}
	if t.Nodes == nil {
		t.Nodes = map[string]Node{}
	}
}

// LoadFromFile parses a single YAML file into a Tree.
// Returns error if the file is missing, malformed, or npc_id is empty.
func LoadFromFile(path string) (*Tree, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var t Tree
	if err := yaml.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	if t.NpcID == "" {
		return nil, fmt.Errorf("%s: npc_id is required", path)
	}
	normalize(&t)
	return &t, nil
}

// LoadAll loads every *.yaml / *.yml file in a directory into a map keyed by NpcID.
// Files that fail to parse are silently skipped (best-effort, like agent-os).
// Returns the partial map and a non-nil error only when the directory itself cannot be read.
func LoadAll(dir string) (map[string]*Tree, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	out := make(map[string]*Tree)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		full := filepath.Join(dir, e.Name())
		tree, err := LoadFromFile(full)
		if err != nil {
			// best-effort: skip bad ones, don't fail whole dir
			continue
		}
		out[tree.NpcID] = tree
	}
	return out, nil
}
