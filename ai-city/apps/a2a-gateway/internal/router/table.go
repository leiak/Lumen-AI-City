// ai-city/apps/a2a-gateway/internal/router/table.go
//
// 跨城 NPC 路由表：内存 map（npc_id → Route）。
//
// 设计：
//   - 启动期从 yaml 文件加载，npc_id 唯一索引；
//   - RWMutex 保护并发读写（HTTP server 多 goroutine 并发 Lookup）；
//   - 暂不支持热更新（reload API 在后续 Task 引入）。
//
// YAML schema：list of Route（4 fields: npc_id / city / endpoint / cert_cn）。
package router

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Route 路由项 - 一个 NPC ID 路由到目标城市。
type Route struct {
	NPCID    string `yaml:"npc_id" json:"npc_id"`
	City     string `yaml:"city" json:"city"`
	Endpoint string `yaml:"endpoint" json:"endpoint"` // host:port (grpc)
	CertCN   string `yaml:"cert_cn" json:"cert_cn"`   // mTLS cert CN
}

// Table 路由表 - 内存 map（npc_id → Route）。
type Table struct {
	mu     sync.RWMutex
	routes map[string]*Route
}

// NewTable 构造空表。
func NewTable() *Table {
	return &Table{routes: make(map[string]*Route)}
}

// LoadFromYAML 加载 yaml 文件并原子替换内存 map。
//
// 返回 error 包含底层 os.ReadFile / yaml.Unmarshal 错误（带上下文前缀）。
func (t *Table) LoadFromYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read routing table: %w", err)
	}
	var routes []*Route
	if err := yaml.Unmarshal(data, &routes); err != nil {
		return fmt.Errorf("parse yaml: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.routes = make(map[string]*Route)
	for _, r := range routes {
		t.routes[r.NPCID] = r
	}
	return nil
}

// Lookup 查路由（read-locked）。未命中返 (nil, false)。
func (t *Table) Lookup(npcID string) (*Route, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	r, ok := t.routes[npcID]
	return r, ok
}

// All 返回所有路由（read-locked，slice 拷贝避免外部修改 map）。
func (t *Table) All() []*Route {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*Route, 0, len(t.routes))
	for _, r := range t.routes {
		out = append(out, r)
	}
	return out
}
