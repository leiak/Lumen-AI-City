package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Auth JWT 鉴权
func Auth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过公开路径
		if c.Request.URL.Path == "/health" || strings.HasPrefix(c.Request.URL.Path, "/v1/auth/") {
			c.Next()
			return
		}

		tokenStr := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if tokenStr == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "missing_token"})
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(401, gin.H{"error": "invalid_token"})
			return
		}

		if claims, ok := token.Claims.(jwt.MapClaims); ok {
			c.Set("player_id", claims["sub"])
		}
		c.Next()
	}
}
