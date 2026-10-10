package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEconomyWalletHandlerMeUsesTokenSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"gold_balance":409,"token_balance":100}`))
	}))
	defer server.Close()

	handler := NewEconomyWalletHandler(server.URL)
	router := gin.New()
	router.GET("/v1/wallet", func(c *gin.Context) {
		c.Set("player_id", "player-1")
		c.Next()
	}, handler.Me)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/wallet", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if gotPath != "/api/v1/wallet/player-1" {
		t.Fatalf("path = %q", gotPath)
	}
	if recorder.Body.String() != `{"gold_balance":409,"token_balance":100}` {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestEconomyWalletHandlerMeRequiresPlayerID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewEconomyWalletHandler("http://economy.invalid")
	router := gin.New()
	router.GET("/v1/wallet", handler.Me)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/wallet", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestEconomyWalletHandlerInventoryUsesTokenSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"product_id":1,"name":"招牌红烧肉","quantity":3,"currencies":"gold"}]`))
	}))
	defer server.Close()

	handler := NewEconomyWalletHandler(server.URL)
	router := gin.New()
	router.GET("/v1/inventory", func(c *gin.Context) {
		c.Set("player_id", "player-1")
		c.Next()
	}, handler.Inventory)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/inventory", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if gotPath != "/api/v1/wallet/player-1/inventory" {
		t.Fatalf("path = %q", gotPath)
	}
	if recorder.Body.String() != `[{"product_id":1,"name":"招牌红烧肉","quantity":3,"currencies":"gold"}]` {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}
