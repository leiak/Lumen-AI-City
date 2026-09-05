// Package httpgw 提供 a2a-gateway 的 HTTP/JSON 网关（Sprint 6）。
//
// 路由（详见 router.go）：
//   POST /v1/cards       → RegisterCard
//   GET  /v1/discover    → Discover
//   POST /v1/messages    → SendMessage
//   GET  /v1/healthz     → Healthz
//
// 设计：
//   - in-process 调 *a2asrv.Service（与 gRPC 共享 Service 单例，零序列化）
//   - JSON DTO 走 mirror struct（避免 protojson payload 序列化为 base64 padding
//     与 canonical RawStdEncoding 不一致）
//   - 错误码 F_001-F_010 → HTTP status 映射见 errmap.go
package httpgw

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// fCodeToHTTP 把 a2a 错误码（"F_004:recipient not found"）映射为 HTTP 状态码。
// 不识别的前缀 → 500。
//
// 映射规则（与 docs/06-A2A协议.md §20.10 对齐）：
//   F_001 缺字段            → 400 Bad Request
//   F_003 capability 空     → 400 Bad Request
//   F_004 收件方未注册       → 404 Not Found
//   F_005 发件方未注册       → 401 Unauthorized
//   F_006 pubkey 解析失败    → 400 Bad Request
//   F_007 signature 失败     → 401 Unauthorized
//   F_008 ts_ms 出窗        → 401 Unauthorized
//   F_009 provider 路由失败  → 400 Bad Request
//   F_010 upstream 不可达    → 502 Bad Gateway
func fCodeToHTTP(errMsg string) int {
	code := extractFCode(errMsg)
	switch code {
	case "F_001", "F_003", "F_006", "F_009":
		return 400
	case "F_004":
		return 404
	case "F_005", "F_007", "F_008":
		return 401
	case "F_010":
		return 502
	default:
		return 500
	}
}

// extractFCode 从 "F_004:recipient not found" 提取 "F_004"。
// 没有冒号时整串视为 code（兼容纯 code 字符串）。
func extractFCode(errMsg string) string {
	if i := strings.IndexByte(errMsg, ':'); i >= 0 {
		return errMsg[:i]
	}
	return errMsg
}

// extractDetail 从 "F_004:recipient not found" 提取 "recipient not found"；
// 缺冒号时返回原串。
func extractDetail(errMsg string) string {
	if i := strings.IndexByte(errMsg, ':'); i >= 0 {
		return errMsg[i+1:]
	}
	return errMsg
}

// errorEnvelope 构造统一错误响应 JSON：
//   {"error":"F_007","detail":"signature mismatch","trace_id":"..."}
func errorEnvelope(code, detail, traceID string) gin.H {
	return gin.H{
		"error":    code,
		"detail":   detail,
		"trace_id": traceID,
	}
}

// errorFromMessage 把 a2a 错误码字符串 + traceID 转成 (HTTP status, envelope)。
// 任何位置传 "" 也安全（fallback 500 + unknown）。
func errorFromMessage(errMsg, traceID string) (int, gin.H) {
	code := extractFCode(errMsg)
	detail := extractDetail(errMsg)
	return fCodeToHTTP(errMsg), errorEnvelope(code, detail, traceID)
}
