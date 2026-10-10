package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type EconomyProxy struct {
	baseURL string
	client  *http.Client
}

func NewEconomyProxy(rawBaseURL string) (*EconomyProxy, error) {
	return &EconomyProxy{
		baseURL: strings.TrimSuffix(rawBaseURL, "/"),
		client:  &http.Client{},
	}, nil
}

func (p *EconomyProxy) forward(c *gin.Context, target string, body []byte) {
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "economy_request_build_failed"})
		return
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "economy_service_unreachable", "detail": err.Error()})
		return
	}
	defer resp.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "economy_response_read_failed"})
		return
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, bytes.TrimSpace(payload))
}

func (p *EconomyProxy) Product(c *gin.Context) {
	target := p.baseURL + "/api/v1/products/" + url.PathEscape(c.Param("npcId"))
	p.forward(c, target, nil)
}

type EconomyPurchaseRequest struct {
	ProductID      int64  `json:"product_id" binding:"required,gt=0"`
	Currency       string `json:"currency" binding:"required,oneof=gold token"`
	IdempotencyKey string `json:"idempotency_key" binding:"required,min=8,max=64"`
	TraceID        string `json:"trace_id" binding:"omitempty,max=200"`
}

func (p *EconomyProxy) Purchase(c *gin.Context) {
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

	var request EconomyPurchaseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}

	payload, err := json.Marshal(map[string]any{
		"user_id":         playerIDStr,
		"product_id":      request.ProductID,
		"currency":        request.Currency,
		"idempotency_key": request.IdempotencyKey,
		"trace_id":        request.TraceID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "economy_request_encode_failed"})
		return
	}
	p.forward(c, p.baseURL+"/api/v1/wallet/purchase", payload)
}

func (p *EconomyProxy) Transactions(c *gin.Context) {
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

	target := p.baseURL + "/api/v1/transactions/" + url.PathEscape(playerIDStr) + "?limit=20&offset=0"
	p.forward(c, target, nil)
}

type EconomyTransferRequest struct {
	ToUserID       string `json:"to_user_id" binding:"required,uuid"`
	Currency       string `json:"currency" binding:"required,oneof=gold token"`
	Amount         int64  `json:"amount" binding:"required,gt=0,lt=100000"`
	IdempotencyKey string `json:"idempotency_key" binding:"required,min=8,max=64"`
	Memo           string `json:"memo" binding:"omitempty,max=200"`
}

func (p *EconomyProxy) Transfer(c *gin.Context) {
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

	var request EconomyTransferRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}
	if request.ToUserID == playerIDStr {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_transfer_to_self"})
		return
	}

	payload, err := json.Marshal(map[string]any{
		"from_user_id":    playerIDStr,
		"to_user_id":      request.ToUserID,
		"currency":        request.Currency,
		"amount":          request.Amount,
		"idempotency_key": request.IdempotencyKey,
		"memo":            request.Memo,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "economy_request_encode_failed"})
		return
	}
	p.forward(c, p.baseURL+"/api/v1/wallet/transfer", payload)
}
