package crosscity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type EconomyTransfer struct {
	GlobalID          string     `json:"global_id"`
	Direction         string     `json:"direction"`
	Status            string     `json:"status"`
	SourceCityID      string     `json:"source_city_id"`
	DestinationCityID string     `json:"destination_city_id"`
	SourceUserID      string     `json:"source_user_id"`
	DestinationUserID string     `json:"destination_user_id"`
	Currency          string     `json:"currency"`
	Amount            int64      `json:"amount"`
	TraceID           string     `json:"trace_id"`
	ExpiresAt         time.Time  `json:"expires_at"`
	ReservedAt        *time.Time `json:"reserved_at"`
	CreditedAt        *time.Time `json:"credited_at"`
	SettledAt         *time.Time `json:"settled_at"`
	RefundedAt        *time.Time `json:"refunded_at"`
}

type MoneyGateway interface {
	Reserve(ctx context.Context, transfer *EconomyTransfer) (*EconomyTransfer, error)
	CreditInbound(ctx context.Context, transfer *EconomyTransfer) (*EconomyTransfer, error)
	SettleOutbound(ctx context.Context, globalID string) (*EconomyTransfer, error)
	RefundOutbound(ctx context.Context, globalID string) (*EconomyTransfer, error)
	GetOutbound(ctx context.Context, globalID string) (*EconomyTransfer, error)
	GetInbound(ctx context.Context, globalID string) (*EconomyTransfer, error)
	ListExpiredOutbound(ctx context.Context) ([]EconomyTransfer, error)
}

type MoneyError struct {
	Code    string
	Message string
	Status  int
}

func (e *MoneyError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type HTTPMoneyClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewHTTPMoneyClient(baseURL, token string, httpClient *http.Client) *HTTPMoneyClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &HTTPMoneyClient{BaseURL: baseURL, Token: token, HTTP: httpClient}
}

func (c *HTTPMoneyClient) Reserve(ctx context.Context, transfer *EconomyTransfer) (*EconomyTransfer, error) {
	return c.do(ctx, http.MethodPost, "/internal/v1/cross-city-transfers/reserve", transfer)
}

func (c *HTTPMoneyClient) CreditInbound(ctx context.Context, transfer *EconomyTransfer) (*EconomyTransfer, error) {
	return c.do(ctx, http.MethodPost, "/internal/v1/cross-city-transfers/credit", transfer)
}

func (c *HTTPMoneyClient) SettleOutbound(ctx context.Context, globalID string) (*EconomyTransfer, error) {
	return c.do(ctx, http.MethodPost, "/internal/v1/cross-city-transfers/"+globalID+"/settle", nil)
}

func (c *HTTPMoneyClient) RefundOutbound(ctx context.Context, globalID string) (*EconomyTransfer, error) {
	return c.do(ctx, http.MethodPost, "/api/v1/cross-city-transfers/"+globalID+"/refund", nil)
}

func (c *HTTPMoneyClient) GetOutbound(ctx context.Context, globalID string) (*EconomyTransfer, error) {
	return c.do(ctx, http.MethodGet, "/internal/v1/cross-city-transfers/"+globalID+"?direction=outbound", nil)
}

func (c *HTTPMoneyClient) GetInbound(ctx context.Context, globalID string) (*EconomyTransfer, error) {
	return c.do(ctx, http.MethodGet, "/internal/v1/cross-city-transfers/"+globalID+"?direction=inbound", nil)
}

func (c *HTTPMoneyClient) ListExpiredOutbound(ctx context.Context) ([]EconomyTransfer, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/internal/v1/cross-city-transfers/expired", nil)
	if err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_CLIENT_ERROR", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_ECONOMY_UNAVAILABLE", Message: err.Error(), Status: http.StatusServiceUnavailable}
	}
	defer resp.Body.Close()
	var transfers []EconomyTransfer
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&transfers); err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_ECONOMY_ERROR", Message: err.Error(), Status: resp.StatusCode}
	}
	return transfers, nil
}

func (c *HTTPMoneyClient) do(ctx context.Context, method, path string, body any) (*EconomyTransfer, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, &MoneyError{Code: "CROSS_CITY_CLIENT_ERROR", Message: err.Error()}
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, payload)
	if err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_CLIENT_ERROR", Message: err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_ECONOMY_UNAVAILABLE", Message: err.Error(), Status: http.StatusServiceUnavailable}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_ECONOMY_UNAVAILABLE", Message: err.Error(), Status: http.StatusBadGateway}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorBody struct {
			Detail struct {
				Code string `json:"code"`
				Msg  string `json:"msg"`
			} `json:"detail"`
		}
		_ = json.Unmarshal(raw, &errorBody)
		code := errorBody.Detail.Code
		if code == "" {
			code = "CROSS_CITY_ECONOMY_ERROR"
		}
		return nil, &MoneyError{Code: code, Message: errorBody.Detail.Msg, Status: resp.StatusCode}
	}
	var transfer EconomyTransfer
	if err := json.Unmarshal(raw, &transfer); err != nil {
		return nil, &MoneyError{Code: "CROSS_CITY_ECONOMY_ERROR", Message: err.Error(), Status: resp.StatusCode}
	}
	return &transfer, nil
}
