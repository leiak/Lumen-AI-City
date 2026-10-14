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

type SagaProxy struct {
	baseURL string
	client  *http.Client
}

func NewSagaProxy(rawBaseURL string) (*SagaProxy, error) {
	return &SagaProxy{
		baseURL: strings.TrimSuffix(rawBaseURL, "/"),
		client:  &http.Client{},
	}, nil
}

func (p *SagaProxy) forward(c *gin.Context, target string, body []byte) {
	request, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, target, bytes.NewReader(body))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "saga_request_build_failed"})
		return
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := p.client.Do(request)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "saga_orchestrator_unreachable", "detail": err.Error()})
		return
	}
	defer response.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "saga_response_read_failed"})
		return
	}
	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	c.Data(response.StatusCode, contentType, bytes.TrimSpace(payload))
}

func (p *SagaProxy) Create(c *gin.Context) {
	playerID, exists := c.Get("player_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request_body"})
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	payload["player_id"] = playerID
	encoded, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json"})
		return
	}
	p.forward(c, p.baseURL+"/v1/saga", encoded)
}

func (p *SagaProxy) Get(c *gin.Context) {
	playerID, exists := c.Get("player_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return
	}
	target := p.baseURL + "/v1/saga/" + url.PathEscape(c.Param("id")) + "?player_id=" + url.QueryEscape(playerID.(string))
	p.forward(c, target, nil)
}
