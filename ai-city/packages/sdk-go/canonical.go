// Canonical 字节序列唯一实现（Sprint 7+）。
//
// 本文件是整个 aicity 联邦中**唯一**的 canonical 字节生成器。
// 任何 server / SDK / smoke / 第三方 client 都必须通过 CanonicalBytes()
// 生成签名原文，否则会与 server 端 verifier 失同步 → 验签失败。
//
// 不变量（与 docs/06-A2A-canonical.md §一对齐）：
//   1) 8 字段固定（无 Signature，无 Provider）
//   2) payload_b64 用 base64.RawStdEncoding（无 padding）
//   3) signature 字段在 canonical 里恒为缺席（不是空字符串，而是字段不存在）
//   4) json.Marshal 默认行为（UTF-8，无 HTML escape，无 indent，字段顺序由 struct 声明决定）
//
// 跨实现护栏：apps/a2a-gateway/internal/a2asrv/verifier_test.go::TestVerifier_CanonicalBytes_MatchesSDK
// 会把 server 输出 byte-equal 与本文件输出 + 真实验签回路，确保 SDK / server 漂移时立刻失败。
package aicity

import "encoding/json"

// CanonicalBytes 把 Signable marshal 成 canonical 字节序列，供 ed25519 签名 / 验签使用。
//
// 返回值说明：
//   - 失败时（实际不可能，因为 Signable 全是基础类型）→ 返 nil；调用方可忽略（与 json.Marshal 同语义）
//   - 成功 → 稳定的字节序列；同输入重复调用 → 同输出（deterministic）
//
// 字段集合、顺序、tag、payload 编码全部由 Signable struct 钉死；
// 禁止修改 Signable 字段顺序 / JSON tag / 类型，否则会导致所有现存 agent 签名失效
// （属 docs/06-A2A-canonical.md §五 定义的"破坏性变更"，需走 BREAKING 协议）。
func CanonicalBytes(s Signable) []byte {
	b, _ := json.Marshal(s)
	return b
}