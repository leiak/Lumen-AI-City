package crosscity

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPMoneyClientCreditInboundSendsServiceToken(t *testing.T) {
	var gotAuthorization, gotPath, gotMethod string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotMethod = r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"global_id":"936aea10-0000-4000-8000-000000000001","direction":"inbound","status":"credited"}`))
	}))
	defer server.Close()

	now := time.Now().UTC()
	client := NewHTTPMoneyClient(server.URL, "secret", server.Client())
	transfer, err := client.CreditInbound(context.Background(), &EconomyTransfer{
		GlobalID:   "936aea10-0000-4000-8000-000000000001",
		Amount:     100,
		ReservedAt: &now,
	})
	if err != nil {
		t.Fatalf("CreditInbound() error = %v", err)
	}
	if gotAuthorization != "Bearer secret" || gotPath != "/internal/v1/cross-city-transfers/credit" || gotMethod != http.MethodPost {
		t.Fatalf("unexpected request %s %s auth=%s", gotMethod, gotPath, gotAuthorization)
	}
	if gotBody["amount"].(float64) != 100 {
		t.Fatalf("unexpected amount: %v", gotBody["amount"])
	}
	if transfer.Status != "credited" {
		t.Fatalf("unexpected status: %s", transfer.Status)
	}
}

func TestHTTPMoneyClientMapsEconomyError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"detail":{"code":"CROSS_CITY_EXPIRED","msg":"expired"}}`))
	}))
	defer server.Close()

	client := NewHTTPMoneyClient(server.URL, "secret", server.Client())
	_, err := client.CreditInbound(context.Background(), &EconomyTransfer{})
	moneyErr, ok := err.(*MoneyError)
	if !ok {
		t.Fatalf("expected *MoneyError, got %T", err)
	}
	if moneyErr.Code != "CROSS_CITY_EXPIRED" || moneyErr.Status != http.StatusGone {
		t.Fatalf("unexpected error: %+v", moneyErr)
	}
}
