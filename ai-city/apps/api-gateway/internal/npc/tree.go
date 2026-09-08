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
type Tree struct {
	NpcID      string          `yaml:"npc_id" json:"npc_id"`
	HomeTile   string          `yaml:"home_tile" json:"home_tile"`
	DefaultSay string          `yaml:"default_say" json:"default_say"`
	Nodes      map[string]Node `yaml:"nodes" json:"nodes"`
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