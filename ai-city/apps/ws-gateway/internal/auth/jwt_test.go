package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-not-the-dev-one"

// signWith 用给定 method/secret 造 token，模拟 api-gateway issueToken 的 claims。
func signWith(t *testing.T, method jwt.SigningMethod, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func validClaims(exp time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"sub":   "1fce8ddd-0000-0000-0000-000000000001",
		"uname": "demo",
		"exp":   exp.Unix(),
		"iat":   time.Now().Unix(),
	}
}

func TestVerify_Valid(t *testing.T) {
	v := NewVerifier(testSecret)
	tok := signWith(t, jwt.SigningMethodHS256, testSecret, validClaims(time.Now().Add(time.Hour)))

	c, err := v.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c.PlayerID != "1fce8ddd-0000-0000-0000-000000000001" {
		t.Errorf("PlayerID = %q", c.PlayerID)
	}
	if c.Username != "demo" {
		t.Errorf("Username = %q, want demo", c.Username)
	}
}

func TestVerify_Expired(t *testing.T) {
	v := NewVerifier(testSecret)
	tok := signWith(t, jwt.SigningMethodHS256, testSecret, validClaims(time.Now().Add(-time.Minute)))

	if _, err := v.Verify(tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	v := NewVerifier(testSecret)
	tok := signWith(t, jwt.SigningMethodHS256, "some-other-secret", validClaims(time.Now().Add(time.Hour)))

	if _, err := v.Verify(tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestVerify_Malformed(t *testing.T) {
	v := NewVerifier(testSecret)
	for _, tok := range []string{"not-a-jwt", "a.b.c", "eyJhbGciOiJIUzI1NiJ9."} {
		if _, err := v.Verify(tok); err != ErrInvalidToken {
			t.Errorf("token %q: err = %v, want ErrInvalidToken", tok, err)
		}
	}
}

func TestVerify_MissingToken(t *testing.T) {
	v := NewVerifier(testSecret)
	if _, err := v.Verify(""); err != ErrMissingToken {
		t.Errorf("err = %v, want ErrMissingToken", err)
	}
}

// alg=none 绕过：不限定 SigningMethod 时，无签名 token 会被接受。
func TestVerify_RejectsAlgNone(t *testing.T) {
	v := NewVerifier(testSecret)
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, validClaims(time.Now().Add(time.Hour)))
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := v.Verify(s); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

// sub 缺失的 token 不能通过 —— 没有 player_id 的连接无法归属。
func TestVerify_MissingSub(t *testing.T) {
	v := NewVerifier(testSecret)
	tok := signWith(t, jwt.SigningMethodHS256, testSecret, jwt.MapClaims{
		"uname": "demo",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(tok); err != ErrInvalidToken {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}
