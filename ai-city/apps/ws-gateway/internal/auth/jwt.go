// Package auth 校验 api-gateway 签发的 HS256 JWT。
//
// token 从 WS 连接的 query string 取（`/ws?token=<jwt>`）—— 浏览器 WebSocket
// 构造器不支持自定义 header，所以不能走 Authorization。这与
// web/src/lib/ws.ts 现有实现一致。
package auth

import (
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrMissingToken token 为空
	ErrMissingToken = errors.New("missing token")
	// ErrInvalidToken 签名 / 过期 / 格式任一不合法
	ErrInvalidToken = errors.New("invalid token")
)

// Claims 是我们关心的子集；与 api-gateway/internal/handlers/auth.go
// issueToken 写入的 MapClaims 对应（sub / uname / exp / iat）。
type Claims struct {
	PlayerID string
	Username string
}

// Verifier 持有共享密钥。
type Verifier struct {
	secret []byte
}

func NewVerifier(secret string) *Verifier {
	return &Verifier{secret: []byte(secret)}
}

// Verify 解析并校验 token，返回 player_id / username。
//
// 显式限定 HS256：jwt.Parse 默认接受任意 alg，若不限定，攻击者可以用
// alg=none 或非对称算法把公钥当密钥绕过签名校验。
func (v *Verifier) Verify(tokenStr string) (*Claims, error) {
	if tokenStr == "" {
		return nil, ErrMissingToken
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	mc, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	sub, _ := mc["sub"].(string)
	if sub == "" {
		return nil, ErrInvalidToken
	}
	uname, _ := mc["uname"].(string)

	return &Claims{PlayerID: sub, Username: uname}, nil
}
