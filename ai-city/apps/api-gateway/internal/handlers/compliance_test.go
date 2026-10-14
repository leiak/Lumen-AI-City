package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestComplianceEndpointsRequireTokenSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewComplianceHandler(nil)
	router := gin.New()
	router.GET("/v1/privacy/consent", handler.GetConsent)
	router.PUT("/v1/privacy/consent", handler.UpdateConsent)
	router.GET("/v1/privacy/export", handler.Export)
	router.POST("/v1/privacy/erase", handler.Erase)

	for _, method := range []string{http.MethodGet, http.MethodPut} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, "/v1/privacy/consent", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s consent status = %d, want %d", method, recorder.Code, http.StatusUnauthorized)
		}
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/privacy/export", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("export status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/privacy/erase", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("erase status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestComplianceConsentRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewComplianceHandler(nil)
	router := gin.New()
	router.PUT("/v1/privacy/consent", func(c *gin.Context) {
		c.Set("player_id", "00000000-0000-4000-8000-000000000001")
		c.Next()
	}, handler.UpdateConsent)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/v1/privacy/consent", strings.NewReader("{"))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
