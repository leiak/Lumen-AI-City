// Package handlers 实现 HTTP handler
package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/aicity/api-gateway/internal/store"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	players   *store.PlayerStore
	pool      *pgxpool.Pool
	jwtSecret string
	jwtExpiry time.Duration
}

func NewAuthHandler(players *store.PlayerStore, pool *pgxpool.Pool, jwtSecret string, jwtExpiry time.Duration) *AuthHandler {
	return &AuthHandler{
		players:   players,
		pool:      pool,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

// issueToken 签发 JWT
func (h *AuthHandler) issueToken(playerID, username string, expiresAt time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   playerID,
		"uname": username,
		"exp":   expiresAt.Unix(),
		"iat":   time.Now().Unix(),
	})
	return token.SignedString([]byte(h.jwtSecret))
}

type loginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=6,max=128"`
}

type loginResponse struct {
	Token       string    `json:"token"`
	PlayerID    string    `json:"player_id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Login 处理登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	player, err := h.players.FindByUsername(c.Request.Context(), req.Username)
	if errors.Is(err, store.ErrPlayerNotFound) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}

	if !player.IsActive {
		c.JSON(http.StatusForbidden, gin.H{"error": "account_disabled"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(player.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_credentials"})
		return
	}

	expiresAt := time.Now().Add(h.jwtExpiry)
	tokenStr, err := h.issueToken(player.ID, player.Username, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token_sign_failed"})
		return
	}

	_ = h.players.UpdateLastLogin(c.Request.Context(), player.ID)
	_ = h.players.CreatePosition(c.Request.Context(), player.ID)

	c.JSON(http.StatusOK, loginResponse{
		Token:       tokenStr,
		PlayerID:    player.ID,
		Username:    player.Username,
		DisplayName: player.DisplayName,
		ExpiresAt:   expiresAt,
	})
}

// Register 处理注册（开发版开放）
type registerRequest struct {
	Username    string `json:"username" binding:"required,min=3,max=64"`
	Email       string `json:"email" binding:"required,email"`
	Password    string `json:"password" binding:"required,min=6,max=128"`
	DisplayName string `json:"display_name" binding:"required,min=1,max=64"`
}

type registerResponse struct {
	PlayerID    string `json:"player_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "hash_failed"})
		return
	}

	playerID := ""
	err = h.pool.QueryRow(c.Request.Context(), `
		INSERT INTO player (username, email, password_hash, display_name)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, strings.ToLower(req.Username), strings.ToLower(req.Email), string(hash), req.DisplayName).Scan(&playerID)

	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "username_or_email_taken"})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "insert_failed"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, registerResponse{
		PlayerID:    playerID,
		Username:    strings.ToLower(req.Username),
		DisplayName: req.DisplayName,
	})
}