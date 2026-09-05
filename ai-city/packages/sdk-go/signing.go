// Package aicity 第三方 Agent 接入 SDK（Sprint 5.5：signing）。
//
// 本文件不 import a2a.proto / tonic —— SDK 边界只依赖 stdlib + net/http
// （与 client.go 一致）。发件方按 Signable mirror struct 填字段 → SignMessage
// 拿 base64(std) 签名串 → 写到 Message.Signature 字段即完成签名。
//
// Canonical 形式必须与 server 端 apps/a2a-gateway/internal/a2asrv/verifier.go
// 的 canonicalBytes 完全一致；任何漂移都会导致验签失败。
package aicity

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
)

// Signable 是 Message 的本地 mirror struct（SDK 不依赖 a2a proto）。
//
// 字段顺序固定 → json.Marshal 输出稳定。新增字段需对齐 server canonical。
// signature 字段故意缺席：签名前把 Signature 留空，参与 hash 时也为空。
type Signable struct {
	MessageID      string `json:"message_id"`
	FromAgentID    string `json:"from_agent_id"`
	ToAgentID      string `json:"to_agent_id"`
	ConversationID string `json:"conversation_id"`
	Type           string `json:"type"`
	PayloadB64     string `json:"payload_b64"`
	TsMs           int64  `json:"ts_ms"`
	TraceID        string `json:"trace_id"`
}

// ParsePublicKey 从 base64(stdEncoding) 解 ed25519 公钥。
// 失败时返 error；调用方负责把 error 转成 MessageResponse.Error("F_007:...")。
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, errBadKey
	}
	return ed25519.PublicKey(raw), nil
}

// errBadKey 哨兵：仅用于"长度不对"区分（base64 错用原 err）。
var errBadKey = &signErr{msg: "ed25519 public key length invalid"}

// signErr 让调用方能 errors.Is 比对。
type signErr struct{ msg string }

func (e *signErr) Error() string { return e.msg }

// SignMessage 用 priv 对 s 做 ed25519 签名，返 base64(stdEncoding) 字符串。
//   - payload 用 base64.RawStdEncoding（无 padding）→ 与 server canonical 一致
//   - Signature 字段不参与 hash（envelope 里就没有它）
func SignMessage(priv ed25519.PrivateKey, s Signable) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, b)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// GenerateKey 是给 SDK 调用方的便利函数（封装 ed25519.GenerateKey）。
func GenerateKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}
