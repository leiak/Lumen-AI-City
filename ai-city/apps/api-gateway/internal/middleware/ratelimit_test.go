package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type hitCounterStore struct {
	hits int
}

type fakeRateLimiterStore struct {
	counts map[string]int64
	err    error
}

func (store *fakeRateLimiterStore) Hit(context.Context, string, time.Duration) (int64, error) {
	if store.err != nil {
		return 0, store.err
	}
	store.counts["total"]++
	return store.counts["total"], nil
}

func testJWT(subject string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256"}`))
	payload, _ := json.Marshal(map[string]string{"sub": subject})
	return fmt.Sprintf("%s.%s.signature", header, base64.RawURLEncoding.EncodeToString(payload))
}

func TestRateLimitCriticalMoneyScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeRateLimiterStore{counts: map[string]int64{}}
	router := gin.New()
	router.POST("/v1/wallet/transfer", rateLimitHandler(store), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	token := testJWT("player-1")
	for i := 0; i < 20; i++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/wallet/transfer", nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, recorder.Code, http.StatusOK)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/wallet/transfer", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitIgnoresIdentityHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeRateLimiterStore{counts: map[string]int64{}}
	router := gin.New()
	router.POST("/v1/auth/login", rateLimitHandler(store), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 60; i++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
		request.Header.Set("X-Player-ID", fmt.Sprintf("player-%d", i))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, recorder.Code, http.StatusOK)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	request.Header.Set("X-Player-ID", "player-61")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}

func (store *hitCounterStore) Hit(context.Context, string, time.Duration) (int64, error) {
	store.hits++
	return int64(store.hits), nil
}

func TestRateLimitSkipsHealthAndMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &hitCounterStore{}
	router := gin.New()
	router.Use(rateLimitHandler(store))
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/metrics", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{"/health", "/metrics", "/health"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, recorder.Code, http.StatusOK)
		}
	}
	if store.hits != 0 {
		t.Fatalf("rate limiter hits = %d, want 0", store.hits)
	}
}
