// Adapter + Dispatcher 框架（Sprint 5.5）：
//
//   Adapter.Deliver(ctx, recipient, msg) → (reply, err)
//   Dispatcher.Deliver → 按 recipient.Provider 选 adapter；fallback 处理空 provider。
//
// 选路规则（与 06-A2A协议.md §20.7 / §20.13 对齐）：
//   1) recipient.Provider == "" → fallback（兜底 echo）；fallback 也不接 → F_009
//   2) provider 命中注册表 → 该 adapter.Deliver
//   3) provider 未命中 → fallback；fallback 不接 → F_009
//
// Sprint 6+ 替换为真 HTTP/gRPC 转发时，只改 adapter_echo.go 的实现；
// Dispatcher 与 Service 保持零改动。
package a2asrv

import (
	"context"
	"log"

	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// Adapter 把消息投递到 recipient（联邦内 / 联邦外都走同一接口）。
//
// 返回 (*Message, error)：
//   - 成功 → reply（可为 nil：fire-and-forget 类 adapter 返 nil,nil）
//   - 协议层错误 → *SigError（如 adapter 自己做验签）
//   - 路由层错误 → *AdapterError
type Adapter interface {
	// Deliver 把 msg 投递给 recipient；ctx 可用于超时 / 取消。
	Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error)
	// Supports 声明本 adapter 接受的 provider 名（大小写敏感）。
	Supports(provider string) bool
}

// AdapterError 携带 F_009 等路由层错误码。
type AdapterError struct {
	Code   string
	Reason string
}

func (e *AdapterError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ":" + e.Reason
}

// Dispatcher 按 provider 选 adapter，未命中走 fallback（兜底 echo）。
type Dispatcher struct {
	byProv   map[string]Adapter
	fallback Adapter
}

// NewDispatcher 构造空 Dispatcher；默认 fallback=nil（调用方 Register）。
func NewDispatcher() *Dispatcher {
	return &Dispatcher{byProv: make(map[string]Adapter)}
}

// Register 注册 adapter；Supports() 返回的所有 provider 名都映射到 a。
// 同一 provider 重复 Register → 后者覆盖前者（warn log 便于排障）。
func (d *Dispatcher) Register(a Adapter) {
	if a == nil {
		return
	}
	for _, prov := range providerNames() {
		if a.Supports(prov) {
			if _, exists := d.byProv[prov]; exists {
				log.Printf("[a2asrv] Dispatcher: provider=%q overwritten", prov)
			}
			d.byProv[prov] = a
		}
	}
}

// SetFallback 设置兜底 adapter（aicity echo）；nil 表示不兜底。
func (d *Dispatcher) SetFallback(a Adapter) {
	d.fallback = a
}

// Deliver 选路：provider → fallback；fallback nil 且未命中 → F_009。
func (d *Dispatcher) Deliver(ctx context.Context, recipient *a2av1.AgentCard, msg *a2av1.Message) (*a2av1.Message, error) {
	prov := recipient.GetProvider()
	if a, ok := d.byProv[prov]; ok {
		return a.Deliver(ctx, recipient, msg)
	}
	if d.fallback != nil && d.fallback.Supports(prov) {
		return d.fallback.Deliver(ctx, recipient, msg)
	}
	return nil, &AdapterError{Code: "F_009", Reason: "unknown provider"}
}

// providerNames 是当前 A2A 协议允许的 provider 名（静态列表）。
// 后续 Sprint 6+ 若开放动态注册，改为读注册表。
func providerNames() []string {
	return []string{"aicity", "openclaw", "workbuddy"}
}
