// Package aicity 提供第三方 Agent 接入 AI City 的 Go SDK。
//
// 详细设计见 docs/06-A2A协议.md §20.17。
package aicity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AgentCard struct {
	AgentID      string            `json:"agent_id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	URL          string            `json:"url"`
	Provider     string            `json:"provider"`
	Version      string            `json:"version"`
	Capabilities []string          `json:"capabilities"`
	Auth         map[string]string `json:"auth,omitempty"`
}

type Client struct {
	GatewayURL string
	APIKey     string
	http       *http.Client
}

func NewClient(gatewayURL, apiKey string) *Client {
	return &Client{
		GatewayURL: gatewayURL,
		APIKey:     apiKey,
		http:       &http.Client{},
	}
}

func (c *Client) RegisterCard(card AgentCard) error {
	body, _ := json.Marshal(card)
	req, _ := http.NewRequest("POST", c.GatewayURL+"/v1/cards", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("register card failed: %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func (c *Client) Discover(capability string) ([]AgentCard, error) {
	req, _ := http.NewRequest("GET", c.GatewayURL+"/v1/discover?capability="+capability, nil)
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var cards []AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&cards); err != nil {
		return nil, err
	}
	return cards, nil
}
