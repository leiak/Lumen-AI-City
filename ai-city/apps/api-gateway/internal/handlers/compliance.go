package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const privacyPolicyVersion = "2026-10-14"

type ComplianceHandler struct {
	pool *pgxpool.Pool
}

func NewComplianceHandler(pool *pgxpool.Pool) *ComplianceHandler {
	return &ComplianceHandler{pool: pool}
}

type consentRequest struct {
	Analytics           bool `json:"analytics"`
	Marketing           bool `json:"marketing"`
	ThirdPartySharing   bool `json:"third_party_sharing"`
	WithdrawAllOptional bool `json:"withdraw_all_optional"`
}

type consentResponse struct {
	Essential         bool   `json:"essential"`
	Analytics         bool   `json:"analytics"`
	Marketing         bool   `json:"marketing"`
	ThirdPartySharing bool   `json:"third_party_sharing"`
	PolicyVersion     string `json:"policy_version"`
	AcceptedAt        string `json:"accepted_at,omitempty"`
	WithdrawnAt       string `json:"withdrawn_at,omitempty"`
	UpdatedAt         string `json:"updated_at"`
}

type exportResponse struct {
	RequestID    string         `json:"request_id"`
	Status       string         `json:"status"`
	CompletedAt  string         `json:"completed_at"`
	PolicyNotice string         `json:"policy_notice"`
	Data         map[string]any `json:"data"`
}

type erasureResponse struct {
	RequestID    string `json:"request_id"`
	Status       string `json:"status"`
	CompletedAt  string `json:"completed_at"`
	PolicyNotice string `json:"policy_notice"`
}

func (h *ComplianceHandler) subject(c *gin.Context) (string, bool) {
	playerID, exists := c.Get("player_id")
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "no_player_in_token"})
		return "", false
	}
	subject, _ := playerID.(string)
	if _, err := uuid.Parse(subject); err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_player_in_token"})
		return "", false
	}
	return subject, true
}

func (h *ComplianceHandler) GetConsent(c *gin.Context) {
	subject, ok := h.subject(c)
	if !ok {
		return
	}
	var response consentResponse
	var acceptedAt, withdrawnAt *string
	err := h.pool.QueryRow(c.Request.Context(), `
		SELECT essential, analytics, marketing, third_party_sharing,
		       policy_version, accepted_at, withdrawn_at, updated_at
		FROM privacy_consent WHERE user_id = $1
	`, subject).Scan(
		&response.Essential, &response.Analytics, &response.Marketing,
		&response.ThirdPartySharing, &response.PolicyVersion,
		&acceptedAt, &withdrawnAt, &response.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		response = consentResponse{
			Essential: true, PolicyVersion: privacyPolicyVersion, UpdatedAt: "never",
		}
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if acceptedAt != nil {
		response.AcceptedAt = *acceptedAt
	}
	if withdrawnAt != nil {
		response.WithdrawnAt = *withdrawnAt
	}
	c.JSON(http.StatusOK, response)
}

func (h *ComplianceHandler) UpdateConsent(c *gin.Context) {
	subject, ok := h.subject(c)
	if !ok {
		return
	}
	var req consentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "detail": err.Error()})
		return
	}
	if req.WithdrawAllOptional {
		req.Analytics = false
		req.Marketing = false
		req.ThirdPartySharing = false
	}
	if _, err := h.pool.Exec(c.Request.Context(), `
		INSERT INTO privacy_consent
			(user_id, essential, analytics, marketing, third_party_sharing, policy_version, accepted_at, withdrawn_at, updated_at)
		VALUES ($1, TRUE, $2, $3, $4, $5,
		        CASE WHEN $2 OR $3 OR $4 THEN NOW() ELSE NULL END,
		        CASE WHEN NOT $2 AND NOT $3 AND NOT $4 THEN NOW() ELSE NULL END,
		        NOW())
		ON CONFLICT (user_id) DO UPDATE SET
			analytics = EXCLUDED.analytics,
			marketing = EXCLUDED.marketing,
			third_party_sharing = EXCLUDED.third_party_sharing,
			policy_version = EXCLUDED.policy_version,
			accepted_at = COALESCE(EXCLUDED.accepted_at, privacy_consent.accepted_at),
			withdrawn_at = COALESCE(EXCLUDED.withdrawn_at, privacy_consent.withdrawn_at),
			updated_at = NOW()
	`, subject, req.Analytics, req.Marketing, req.ThirdPartySharing, privacyPolicyVersion); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "consent_update_failed", "detail": err.Error()})
		return
	}
	action := "consent.update"
	if req.WithdrawAllOptional {
		action = "consent.withdraw"
	}
	_, _ = h.pool.Exec(c.Request.Context(), `
		INSERT INTO privacy_action_audit (actor_id, action, metadata)
		VALUES ($1, $2, $3::jsonb)
	`, subject, action, `{"source":"api"}`)

	c.JSON(http.StatusOK, gin.H{
		"essential":             true,
		"analytics":             req.Analytics,
		"marketing":             req.Marketing,
		"third_party_sharing":   req.ThirdPartySharing,
		"policy_version":        privacyPolicyVersion,
		"withdraw_all_optional": req.WithdrawAllOptional,
	})
}

func (h *ComplianceHandler) Export(c *gin.Context) {
	subject, ok := h.subject(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	var profile map[string]any
	err := h.pool.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'player_id', id, 'username', username, 'email', email,
			'display_name', display_name, 'created_at', created_at,
			'last_login_at', last_login_at, 'is_active', is_active
		) FROM player WHERE id = $1 AND deleted_at IS NULL
	`, subject).Scan(&profile)
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "player_not_found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}

	var wallet map[string]any
	walletErr := h.pool.QueryRow(ctx, `
		SELECT jsonb_build_object(
			'user_id', user_id, 'gold_balance', gold_balance,
			'token_balance', token_balance, 'updated_at', updated_at
		) FROM wallet WHERE user_id = $1
	`, subject).Scan(&wallet)
	if walletErr != nil && !errors.Is(walletErr, pgx.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": walletErr.Error()})
		return
	}

	var transactions []map[string]any
	rows, err := h.pool.Query(ctx, `
		SELECT jsonb_build_object(
			'id', id, 'tx_type', tx_type, 'counterparty_id', counterparty_id,
			'currency', currency, 'amount', amount, 'balance_after', balance_after,
			'product_id', product_id, 'trace_id', trace_id, 'created_at', created_at
		)
		FROM transaction WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1000
	`, subject)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	for rows.Next() {
		var item map[string]any
		if scanErr := rows.Scan(&item); scanErr != nil {
			rows.Close()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": scanErr.Error()})
			return
		}
		transactions = append(transactions, item)
	}
	rows.Close()
	if transactions == nil {
		transactions = []map[string]any{}
	}

	var positions []map[string]any
	positionRows, err := h.pool.Query(ctx, `
		SELECT jsonb_build_object(
			'tile_id', tile_id, 'x', x, 'y', y, 'updated_at', updated_at
		) FROM player_position WHERE player_id = $1
	`, subject)
	if err == nil {
		for positionRows.Next() {
			var item map[string]any
			if scanErr := positionRows.Scan(&item); scanErr != nil {
				positionRows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": scanErr.Error()})
				return
			}
			positions = append(positions, item)
		}
		positionRows.Close()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if positions == nil {
		positions = []map[string]any{}
	}

	var purchases []map[string]any
	purchaseRows, err := h.pool.Query(ctx, `
		SELECT jsonb_build_object(
			'id', id, 'template_kind', template_kind, 'template_id', template_id,
			'price_paid_gold', price_paid_gold, 'idempotency_key', idempotency_key,
			'created_at', created_at
		) FROM template_purchase WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1000
	`, subject)
	if err == nil {
		for purchaseRows.Next() {
			var item map[string]any
			if scanErr := purchaseRows.Scan(&item); scanErr != nil {
				purchaseRows.Close()
				c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": scanErr.Error()})
				return
			}
			purchases = append(purchases, item)
		}
		purchaseRows.Close()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if purchases == nil {
		purchases = []map[string]any{}
	}

	payload := map[string]any{
		"subject":        subject,
		"policy_version": privacyPolicyVersion,
		"profile":        profile,
		"wallet":         wallet,
		"transactions":   transactions,
		"positions":      positions,
		"purchases":      purchases,
	}
	encoded, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": marshalErr.Error()})
		return
	}

	var requestID, completedAt string
	err = h.pool.QueryRow(ctx, `
		INSERT INTO privacy_export_request (user_id, status, payload, completed_at)
		VALUES ($1, 'completed', $2::jsonb, NOW())
		RETURNING id::text, completed_at::text
	`, subject, string(encoded)).Scan(&requestID, &completedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	_, _ = h.pool.Exec(ctx, `
		INSERT INTO privacy_action_audit (actor_id, action, request_id, metadata)
		VALUES ($1, 'data.export', $2, $3::jsonb)
	`, subject, requestID, `{"scope":"player_profile_financial"}`)

	c.Header("Content-Disposition", `attachment; filename="aicity-data-export.json"`)
	c.JSON(http.StatusOK, exportResponse{
		RequestID:    requestID,
		Status:       "completed",
		CompletedAt:  completedAt,
		PolicyNotice: "Financial and regulatory audit records are retained where legally required.",
		Data:         payload,
	})
}

func (h *ComplianceHandler) Erase(c *gin.Context) {
	subject, ok := h.subject(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	var exists bool
	err := h.pool.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM player WHERE id = $1 AND deleted_at IS NULL)
	`, subject).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "player_not_found"})
		return
	}

	requestID := uuid.New().String()
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO privacy_erasure_request (id, user_id, status, reason)
		VALUES ($1, $2, 'pending', 'subject_request')
	`, requestID, subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE player SET
			username = 'deleted_' || substr(md5(random()::text), 1, 24),
			email = NULL,
			password_hash = 'deleted:' || md5(random()::text),
			display_name = 'Deleted Player',
			avatar_url = NULL,
			settings = '{}'::jsonb,
			is_active = FALSE,
			last_login_at = NULL,
			deleted_at = NOW(),
			updated_at = NOW()
		WHERE id = $1
	`, subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE wallet SET gold_balance = 0, token_balance = 0 WHERE user_id = $1`, subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM player_position WHERE player_id = $1`, subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM a2a_inbox WHERE from_agent_id = $1 OR to_agent_id = $1`, subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `DELETE FROM episodic_memory WHERE player_id = $1`, subject); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO privacy_action_audit (actor_id, action, request_id, metadata)
		VALUES ($1, 'data.erase', $2, $3::jsonb)
	`, subject, requestID, `{"retained":"financial_and_audit_records"}`); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}

	var completedAt string
	err = tx.QueryRow(ctx, `
		UPDATE privacy_erasure_request SET status = 'completed', completed_at = NOW()
		WHERE id = $1 RETURNING completed_at::text
	`, requestID).Scan(&completedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusOK, erasureResponse{
		RequestID:    requestID,
		Status:       "completed",
		CompletedAt:  completedAt,
		PolicyNotice: "Personal data was anonymized. Financial and audit records remain retained where required.",
	})
}
