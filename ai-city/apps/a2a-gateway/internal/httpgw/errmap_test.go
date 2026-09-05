package httpgw

import (
	"testing"
)

// 表驱动：F_001-F_010 → HTTP status（错误码契约）
func TestFCodeToHTTP_TableDriven(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{"F_001", 400},
		{"F_003", 400},
		{"F_004", 404},
		{"F_005", 401},
		{"F_006", 400},
		{"F_007", 401},
		{"F_008", 401},
		{"F_009", 400},
		{"F_010", 502},
		{"F_999", 500}, // 未知 → 500
		{"", 500},
	}
	for _, tc := range cases {
		if got := fCodeToHTTP(tc.code); got != tc.want {
			t.Errorf("fCodeToHTTP(%q) = %d, want %d", tc.code, got, tc.want)
		}
		// 带 detail 后缀也要能解析
		withDetail := tc.code + ":some reason"
		if got := fCodeToHTTP(withDetail); got != tc.want {
			t.Errorf("fCodeToHTTP(%q) = %d, want %d", withDetail, got, tc.want)
		}
	}
}

func TestExtractFCode(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"F_007:signature mismatch", "F_007"},
		{"F_004:recipient not found", "F_004"},
		{"F_001", "F_001"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractFCode(tc.in); got != tc.want {
			t.Errorf("extractFCode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractDetail(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"F_007:signature mismatch", "signature mismatch"},
		{"F_004:recipient not found", "recipient not found"},
		{"F_001", "F_001"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractDetail(tc.in); got != tc.want {
			t.Errorf("extractDetail(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestErrorEnvelope(t *testing.T) {
	env := errorEnvelope("F_007", "signature mismatch", "trace-123")
	if env["error"] != "F_007" {
		t.Errorf("error field = %v", env["error"])
	}
	if env["detail"] != "signature mismatch" {
		t.Errorf("detail field = %v", env["detail"])
	}
	if env["trace_id"] != "trace-123" {
		t.Errorf("trace_id field = %v", env["trace_id"])
	}
}

func TestErrorFromMessage(t *testing.T) {
	status, env := errorFromMessage("F_004:recipient not found", "t1")
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
	if env["error"] != "F_004" {
		t.Errorf("error = %v", env["error"])
	}
	if env["detail"] != "recipient not found" {
		t.Errorf("detail = %v", env["detail"])
	}
	if env["trace_id"] != "t1" {
		t.Errorf("trace_id = %v", env["trace_id"])
	}
}
