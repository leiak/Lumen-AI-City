package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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

	for i := 0; i < 10; i++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
		request.Header.Set("X-Player-ID", "spoofed-player")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, recorder.Code, http.StatusOK)
		}
	}

	request := httptest.NewRequest(http.MethodPost, "/v1/auth/login", nil)
	request.Header.Set("X-Player-ID", "spoofed-player")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitFailsOpenOnStoreError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &fakeRateLimiterStore{counts: map[string]int64{}, err: errors.New("redis unavailable")}
	router := gin.New()
	router.GET("/health", rateLimitHandler(store), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}
