package handlers

import (
	"errors"
	"net/http"

	"github.com/aicity/api-gateway/internal/store"
	"github.com/gin-gonic/gin"
)

type PlayerHandler struct {
	players *store.PlayerStore
}

func NewPlayerHandler(players *store.PlayerStore) *PlayerHandler {
	return &PlayerHandler{players: players}
}

type playerResponse struct {
	PlayerID    string `json:"player_id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// GetByID 获取玩家信息
func (h *PlayerHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	p, err := h.players.FindByID(c.Request.Context(), id)
	if errors.Is(err, store.ErrPlayerNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "player_not_found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	c.JSON(http.StatusOK, playerResponse{
		PlayerID:    p.ID,
		Username:    p.Username,
		DisplayName:  p.DisplayName,
		AvatarURL:   p.AvatarURL,
	})
}

// Me 获取当前登录玩家信息（从 JWT 拿 sub）
func (h *PlayerHandler) Me(c *gin.Context) {
	pid, exists := c.Get("player_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}
	pidStr, _ := pid.(string)
	p, err := h.players.FindByID(c.Request.Context(), pidStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "player_not_found"})
		return
	}
	c.JSON(http.StatusOK, playerResponse{
		PlayerID:    p.ID,
		Username:    p.Username,
		DisplayName:  p.DisplayName,
		AvatarURL:   p.AvatarURL,
	})
}