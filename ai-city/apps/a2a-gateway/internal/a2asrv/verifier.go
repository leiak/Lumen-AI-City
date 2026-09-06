// Verifier 负责 ed25519 验签 + 重放时间窗校验（Sprint 5.5）。
//
// Canonical 签名形式（docs/06-A2A-canonical.md）：
//   - 固定 8 字段 struct（message_id, from_agent_id, to_agent_id,
//     conversation_id, type, payload_b64, ts_ms, trace_id）
//   - json.Marshal 默认行为（UTF-8，无 HTML escape）
//   - payload_b64 用 base64.RawStdEncoding（无 padding）
//   - signature 字段在 canonical 里恒为 ""
//
// 注：proto Message 没有 provider 字段，故 canonical 不含 provider；
// provider 路由只用于 Dispatcher 选择 adapter，与签名无关。
//
// Sprint 7+：canonical 字节生成统一走 packages/sdk-go.CanonicalBytes（唯一实现）。
// 本文件 canonicalBytes 是薄适配器：proto → aicity.Signable → CanonicalBytes。
// 跨实现护栏见 verifier_test.go::TestVerifier_CanonicalBytes_MatchesSDK。
//
// 错误码（与 Service 共用前缀）：
//   F_005 sender 未注册
//   F_007 signature required / decode / mismatch
//   F_008 ts_ms out of window
package a2asrv

import (
	"crypto/ed25519"
	"encoding/base64"
	"time"

	aicity "github.com/aicity/sdk-go"
	a2av1 "github.com/aicity/proto/gen/go/a2a/v1"
)

// SigError 携带错误码前缀 + 人类可读原因。
// Code 为 "F_005/F_007/F_008" 等；Reason 为短句（无冒号）。
// Error() 返 "Code:Reason"，可直接用作 MessageResponse.Error。
type SigError struct {
	Code   string
	Reason string
}

func (e *SigError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ":" + e.Reason
}

// canonicalBytes 把 Message 转成 canonical 字节（薄适配器 → aicity.CanonicalBytes）。
//
//   - payload 用 RawStdEncoding（无 padding）→ 与 Sprint 6+ 跨语言对齐
//   - signature 字段在 Signable 里缺席，所以恒定不参与 hash
//   - Signable 字段顺序 / JSON tag 由 packages/sdk-go/canonical.go 钉死
func canonicalBytes(m *a2av1.Message) []byte {
	if m == nil {
		// 极端防御：nil 输入返空字节；调用方应先判 m==nil
		return nil
	}
	return aicity.CanonicalBytes(aicity.Signable{
		MessageID:      m.GetMessageId(),
		FromAgentID:    m.GetFromAgentId(),
		ToAgentID:      m.GetToAgentId(),
		ConversationID: m.GetConversationId(),
		Type:           m.GetType(),
		PayloadB64:     base64.RawStdEncoding.EncodeToString(m.GetPayload()),
		TsMs:           m.GetTsMs(),
		TraceID:        m.GetTraceId(),
	})
}

// Verifier 持有重放窗口 + ed25519 公钥解析能力。
type Verifier struct {
	ReplayWindow time.Duration
}

// NewVerifier 构造 Verifier；window <= 0 时回落到 5min 默认。
func NewVerifier(window time.Duration) *Verifier {
	if window <= 0 {
		window = 5 * time.Minute
	}
	return &Verifier{ReplayWindow: window}
}

// ParsePublicKey 从 base64(stdEncoding) 字符串解 ed25519 公钥。
// 长度错 / base64 错 → 返 *SigError{Code:"F_007"}。
func (v *Verifier) ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	if b64 == "" {
		return nil, &SigError{Code: "F_007", Reason: "signature required"}
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, &SigError{Code: "F_007", Reason: "signature decode"}
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, &SigError{Code: "F_007", Reason: "signature decode"}
	}
	return ed25519.PublicKey(raw), nil
}

// Verify 按以下顺序检查 sender 身份 + 签名 + 时间窗：
//   1) sender == nil → F_005（注册表未找到发件方）
//   2) sender.Auth["ed25519"] == "" → 跳过签名校验（opt-in 放行）
//   3) sig == ""  → F_007:signature required
//   4) ParsePublicKey 失败 → F_007:signature decode
//   5) |now - ts_ms| > ReplayWindow → F_008:ts_ms out of window
//   6) ed25519.Verify 失败 → F_007:signature mismatch
//
// 成功返 nil；失败返 *SigError。
func (v *Verifier) Verify(sender *a2av1.AgentCard, m *a2av1.Message, now time.Time) error {
	if sender == nil {
		return &SigError{Code: "F_005", Reason: "sender not registered"}
	}
	if sender.GetAuth() == nil {
		return nil // opt-in 放行
	}
	pubB64, hasKey := sender.GetAuth()["ed25519"]
	if !hasKey || pubB64 == "" {
		return nil // opt-in 放行
	}
	// 公钥已注册 → 强制签
	if m == nil {
		return &SigError{Code: "F_007", Reason: "signature required"}
	}
	if m.GetSignature() == "" {
		return &SigError{Code: "F_007", Reason: "signature required"}
	}
	pub, err := v.ParsePublicKey(pubB64)
	if err != nil {
		return err
	}
	sigBytes, err := base64.StdEncoding.DecodeString(m.GetSignature())
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return &SigError{Code: "F_007", Reason: "signature decode"}
	}
	// 时间窗（基于 ts_ms 毫秒）
	tsMs := m.GetTsMs()
	delta := now.UnixMilli() - tsMs
	if delta < 0 {
		delta = -delta
	}
	if time.Duration(delta)*time.Millisecond > v.ReplayWindow {
		return &SigError{Code: "F_008", Reason: "ts_ms out of window"}
	}
	// 验签
	if !ed25519.Verify(pub, canonicalBytes(m), sigBytes) {
		return &SigError{Code: "F_007", Reason: "signature mismatch"}
	}
	return nil
}
