package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEconomyProxyProductEscapesNPCID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	proxy, err := NewEconomyProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/v1/products/:npcId", proxy.Product)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/products/npc_a_b", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if gotPath != "/api/v1/products/npc_a_b" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestEconomyProxyMarketplacePurchaseForwardsToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotAuthorization string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"purchase_id":71}`))
	}))
	defer server.Close()

	proxy, err := NewEconomyProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/v1/marketplace/purchase", func(c *gin.Context) {
		c.Set("player_id", "token-player")
		c.Next()
	}, proxy.MarketplacePurchase)

	body := bytes.NewBufferString(`{"template_kind":"npc","template_id":12,"idempotency_key":"city-ui-market"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/marketplace/purchase", body)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer test-jwt")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotPath != "/v1/marketplace/purchase" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuthorization != "Bearer test-jwt" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}
}

func TestEconomyProxyCreatorRevenueUsesTokenSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath string
	var gotAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"purchase_id":71,"amount_gold":100,"platform_cut_gold":0}]`))
	}))
	defer server.Close()

	proxy, err := NewEconomyProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/v1/marketplace/revenue", func(c *gin.Context) {
		c.Set("player_id", "11111111-1111-4111-8111-111111111111")
		c.Next()
	}, proxy.CreatorRevenue)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/marketplace/revenue", nil)
	request.Header.Set("Authorization", "Bearer test-jwt")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotPath != "/v1/marketplace/revenue/11111111-1111-4111-8111-111111111111" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuthorization != "Bearer test-jwt" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}
}

func TestEconomyProxyPurchaseOverridesUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"balance_after":359}`))
	}))
	defer server.Close()

	proxy, err := NewEconomyProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/v1/wallet/purchase", func(c *gin.Context) {
		c.Set("player_id", "token-player")
		c.Next()
	}, proxy.Purchase)

	body := bytes.NewBufferString(`{"product_id":1,"currency":"gold","idempotency_key":"city-ui-1-abc","user_id":"spoofed"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/wallet/purchase", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotBody["user_id"] != "token-player" {
		t.Fatalf("user_id = %v", gotBody["user_id"])
	}
}

func TestEconomyProxyTransferOverridesFromUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"gold_balance":90,"token_balance":100}`))
	}))
	defer server.Close()

	proxy, err := NewEconomyProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/v1/wallet/transfer", func(c *gin.Context) {
		c.Set("player_id", "11111111-1111-4111-8111-111111111111")
		c.Next()
	}, proxy.Transfer)

	body := bytes.NewBufferString(`{"to_user_id":"22222222-2222-4222-8222-222222222222","currency":"gold","amount":10,"idempotency_key":"city-ui-transfer","from_user_id":"spoofed"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/wallet/transfer", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotBody["from_user_id"] != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("from_user_id = %v", gotBody["from_user_id"])
	}
}

func TestEconomyProxyTransferRejectsSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)
	proxy, err := NewEconomyProxy("http://economy.invalid")
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	playerID := "11111111-1111-4111-8111-111111111111"
	router.POST("/v1/wallet/transfer", func(c *gin.Context) {
		c.Set("player_id", playerID)
		c.Next()
	}, proxy.Transfer)

	body := bytes.NewBufferString(`{"to_user_id":"` + playerID + `","currency":"gold","amount":10,"idempotency_key":"city-ui-self"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/wallet/transfer", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
