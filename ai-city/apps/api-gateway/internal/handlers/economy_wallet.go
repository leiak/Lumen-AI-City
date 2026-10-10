package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type EconomyWalletHandler struct {
	baseURL string
	client  *http.Client
}

func NewEconomyWalletHandler(rawBaseURL string) *EconomyWalletHandler {
	return &EconomyWalletHandler{
		baseURL: strings.TrimSuffix(rawBaseURL, "/"),
		client:  &http.Client{},
	}
}

func (h *EconomyWalletHandler) Me(c *gin.Context) {
	playerID, exists := c.Get("player_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}
	playerIDStr, _ := playerID.(string)
	if playerIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}

	target := h.baseURL + "/api/v1/wallet/" + url.PathEscape(playerIDStr)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "economy_request_build_failed"})
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "economy_service_unreachable", "detail": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "economy_response_read_failed"})
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, bytes.TrimSpace(body))
}

func (h *EconomyWalletHandler) Inventory(c *gin.Context) {
	playerID, exists := c.Get("player_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}
	playerIDStr, _ := playerID.(string)
	if playerIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}

	target := h.baseURL + "/api/v1/wallet/" + url.PathEscape(playerIDStr) + "/inventory"
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "economy_request_build_failed"})
		return
	}
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "economy_service_unreachable", "detail": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "economy_response_read_failed"})
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, bytes.TrimSpace(body))
}
