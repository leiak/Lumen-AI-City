package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"
)

var criticalActionTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{Name: "aicity_critical_action_total"},
	[]string{"action", "result"},
)

var criticalActionPrefixes = map[string]string{
	"/v1/auth/login":                   "login",
	"/v1/auth/register":                "register",
	"/v1/wallet/purchase":              "npc_purchase",
	"/v1/wallet/transfer":              "wallet_transfer",
	"/v1/marketplace/purchase":         "marketplace_purchase",
	"/v1/marketplace/revenue-withdraw": "creator_withdraw",
}

func criticalAction(path string) string {
	for prefix, action := range criticalActionPrefixes {
		if strings.HasPrefix(path, prefix) {
			return action
		}
	}
	return ""
}

// AuditCritical emits a structured security event for every authentication
// and money-movement endpoint. Database-side append-only transaction rows
// remain the source of truth for balances; these events add the gateway
// request context needed for incident response.
func AuditCritical(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		action := criticalAction(c.Request.URL.Path)
		if action == "" {
			c.Next()
			return
		}

		c.Next()

		logger.Info("critical_action_audit",
			zap.String("event", action),
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.String("actor_id", c.GetString("player_id")),
			zap.String("client_ip", c.ClientIP()),
			zap.String("trace_id", c.GetString("trace_id")),
			zap.Int("status", c.Writer.Status()),
			zap.String("result", auditResult(c.Writer.Status())),
		)
		criticalActionTotal.WithLabelValues(action, auditResult(c.Writer.Status())).Inc()
	}
}

func auditResult(status int) string {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return "succeeded"
	}
	return "failed"
}
