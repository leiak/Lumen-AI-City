package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSagaProxyCreateOverridesPlayer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/saga" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"saga_id":"saga-1"}`))
	}))
	defer server.Close()

	proxy, err := NewSagaProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/v1/sagas", func(c *gin.Context) {
		c.Set("player_id", "token-player")
		c.Next()
	}, proxy.Create)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/sagas", bytes.NewBufferString(`{"saga_id":"saga-1","player_id":"spoofed"}`))
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotBody["player_id"] != "token-player" {
		t.Fatalf("player_id = %v", gotBody["player_id"])
	}
}

func TestSagaProxyGetAddsPlayer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var gotRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/saga/saga-1" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotRawQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"saga_id":"saga-1"}`))
	}))
	defer server.Close()

	proxy, err := NewSagaProxy(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.GET("/v1/sagas/:id", func(c *gin.Context) {
		c.Set("player_id", "token player")
		c.Next()
	}, proxy.Get)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/sagas/saga-1", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotRawQuery != "player_id=token+player" {
		t.Fatalf("raw query = %q", gotRawQuery)
	}
}
