// Package streamcheck provides helpers used by acceptance_2_1 to validate
// the NPC streaming pipeline (ws-gateway + api-gateway + Redis pub/sub).
//
// The 2.0 stage 2 acceptance binary exercises:
//  1. Player login (api-gateway POST /v1/auth/login)
//  2. Subscribe to the npc_say_stream Redis channel and collect beat packets
//     for a fixed window per NPC
//
// Both building blocks are exposed as a small Validator so tests can wire in
// miniredis instead of a live Redis.
package streamcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

// ChannelSayStream is the Redis pub/sub channel that ws-gateway fans out
// per-NPC streaming sentence beats onto. npc_say_stream_done events ride the
// same channel.
const ChannelSayStream = "aicity:npc:say_stream"

// defaultTimeoutMs is used as the default per-call deadline when callers do
// not derive their own context. Kept as a field so tests can override if
// needed in the future.
const defaultTimeoutMs = 8000

// Validator bundles the connections needed to run the acceptance steps:
// a Redis subscriber for the say_stream channel and an api-gateway HTTP base.
type Validator struct {
	rdb       *redis.Client
	apiBase   string
	timeoutMs int
}

// New constructs a Validator with sensible defaults. timeoutMs is currently a
// placeholder for future per-call timeouts; SubscribeBeat already respects
// the caller-provided context.
func New(rdb *redis.Client, apiBase string) *Validator {
	return &Validator{rdb: rdb, apiBase: apiBase, timeoutMs: defaultTimeoutMs}
}

// Login POSTs {username,password} to api-gateway /v1/auth/login and returns
// the bearer token. The endpoint binds JSON (see api-gateway internal/handlers/auth.go),
// so a JSON body is used — http.PostForm would be rejected with 400.
func (v *Validator) Login(ctx context.Context, username, password string) (string, error) {
	body, err := json.Marshal(map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		return "", fmt.Errorf("marshal login body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.apiBase+"/v1/auth/login", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login status %d", resp.StatusCode)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode login response: %w", err)
	}
	if out.Token == "" {
		return "", fmt.Errorf("login response missing token")
	}
	return out.Token, nil
}

// SubscribeBeat subscribes to ChannelSayStream for `seconds` and returns the
// decoded beat payloads whose npc_id matches the requested NPC. Malformed
// payloads are skipped (defensive — ws-gateway should never publish garbage
// but a stale message in the channel must not fail the whole step).
//
// The returned slice preserves publish order, which is important so callers
// can assert on sentence_idx monotonicity downstream.
func (v *Validator) SubscribeBeat(ctx context.Context, npcID string, seconds int) ([]map[string]any, error) {
	sub := v.rdb.Subscribe(ctx, ChannelSayStream)
	defer func() { _ = sub.Close() }()

	// Wait for subscription confirmation before publishing / waiting — without
	// this, a fast Publish + short Subscribe can race and drop the first
	// message. See https://pkg.go.dev/github.com/redis/go-redis/v9#PubSub.
	if _, err := sub.Receive(ctx); err != nil {
		return nil, fmt.Errorf("subscribe confirm: %w", err)
	}

	ch := sub.Channel()
	deadline := time.After(time.Duration(seconds) * time.Second)
	var beats []map[string]any
	for {
		select {
		case <-ctx.Done():
			return beats, ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return beats, nil
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				continue
			}
			if payload["npc_id"] == npcID {
				beats = append(beats, payload)
			}
		case <-deadline:
			return beats, nil
		}
	}
}
