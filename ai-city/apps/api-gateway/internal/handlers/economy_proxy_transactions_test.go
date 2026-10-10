package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEconomyProxyTransactionsUsesTokenSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"transactions":[],"total":0}`))
	}))
	defer server.Close()

	proxy, err := NewEconomyProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/v1/transactions", func(c *gin.Context) {
		c.Set("player_id", "token-player")
		c.Next()
	}, proxy.Transactions)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/transactions?limit=999", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if gotPath != "/api/v1/transactions/token-player" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "limit=20&offset=0" {
		t.Fatalf("query = %q", gotQuery)
	}
}
