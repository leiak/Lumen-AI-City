package a2asrv

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentTemplatePermission struct {
	AgentID      string
	CanCreateNPC bool
	DailyQuota   int
}

type AgentTemplateDraft struct {
	ID              int64
	AgentID         string
	Name            string
	HomeTileID      string
	Description     string
	Personality     map[string]float64
	TalkTree        map[string]any
	ProductCatalog  []map[string]any
	Status          string
	ValidationTrace map[string]any
	CreatedAt       time.Time
}

type AgentActionAudit struct {
	AgentID    string
	Action     string
	ResourceID string
	Allowed    bool
	Reason     string
	TraceID    string
	Metadata   map[string]any
}

type TemplateStore interface {
	CanCreateNPC(ctx context.Context, agentID string) (bool, error)
	CreateDraft(ctx context.Context, draft AgentTemplateDraft) (int64, error)
	ListDrafts(ctx context.Context, agentID string, limit int) ([]AgentTemplateDraft, error)
	Audit(ctx context.Context, audit AgentActionAudit) error
}

type TemplatePGStore struct {
	pool *pgxpool.Pool
}

func NewTemplatePGStore(pool *pgxpool.Pool) *TemplatePGStore {
	return &TemplatePGStore{pool: pool}
}

func (s *TemplatePGStore) CanCreateNPC(ctx context.Context, agentID string) (bool, error) {
	const q = `SELECT can_create_npc FROM a2a_agent_permission WHERE agent_id = $1`
	var allowed bool
	if err := s.pool.QueryRow(ctx, q, agentID).Scan(&allowed); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("agent permission query: %w", err)
	}
	return allowed, nil
}

func (s *TemplatePGStore) CreateDraft(ctx context.Context, draft AgentTemplateDraft) (int64, error) {
	personality, err := json.Marshal(draft.Personality)
	if err != nil {
		return 0, fmt.Errorf("encode personality: %w", err)
	}
	talkTree, err := json.Marshal(draft.TalkTree)
	if err != nil {
		return 0, fmt.Errorf("encode talk tree: %w", err)
	}
	catalog, err := json.Marshal(draft.ProductCatalog)
	if err != nil {
		return 0, fmt.Errorf("encode product catalog: %w", err)
	}
	trace, err := json.Marshal(draft.ValidationTrace)
	if err != nil {
		return 0, fmt.Errorf("encode validation trace: %w", err)
	}
	const q = `
INSERT INTO a2a_npc_template_draft
    (agent_id, name, home_tile_id, description, personality, talk_tree,
     product_catalog, status, validation_trace)
VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7::jsonb, 'draft', $8::jsonb)
RETURNING id
`
	var id int64
	if err := s.pool.QueryRow(ctx, q, draft.AgentID, draft.Name, draft.HomeTileID,
		draft.Description, personality, talkTree, catalog, trace).Scan(&id); err != nil {
		return 0, fmt.Errorf("create template draft: %w", err)
	}
	return id, nil
}

func (s *TemplatePGStore) ListDrafts(ctx context.Context, agentID string, limit int) ([]AgentTemplateDraft, error) {
	const q = `
SELECT id, agent_id, name, home_tile_id, description, personality, talk_tree,
       product_catalog, status, validation_trace, created_at
FROM a2a_npc_template_draft
WHERE agent_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2
`
	rows, err := s.pool.Query(ctx, q, agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("list template drafts: %w", err)
	}
	defer rows.Close()

	drafts := []AgentTemplateDraft{}
	for rows.Next() {
		var (
			draft           AgentTemplateDraft
			personality     []byte
			talkTree        []byte
			catalog         []byte
			validationTrace []byte
		)
		if err := rows.Scan(&draft.ID, &draft.AgentID, &draft.Name, &draft.HomeTileID,
			&draft.Description, &personality, &talkTree, &catalog, &draft.Status,
			&validationTrace, &draft.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan template draft: %w", err)
		}
		if err := json.Unmarshal(personality, &draft.Personality); err != nil {
			return nil, fmt.Errorf("decode personality: %w", err)
		}
		if err := json.Unmarshal(talkTree, &draft.TalkTree); err != nil {
			return nil, fmt.Errorf("decode talk tree: %w", err)
		}
		if err := json.Unmarshal(catalog, &draft.ProductCatalog); err != nil {
			return nil, fmt.Errorf("decode product catalog: %w", err)
		}
		if err := json.Unmarshal(validationTrace, &draft.ValidationTrace); err != nil {
			return nil, fmt.Errorf("decode validation trace: %w", err)
		}
		drafts = append(drafts, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate template drafts: %w", err)
	}
	return drafts, nil
}

func (s *TemplatePGStore) Audit(ctx context.Context, audit AgentActionAudit) error {
	metadata, err := json.Marshal(audit.Metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	const q = `
INSERT INTO a2a_agent_action_audit
    (agent_id, action, resource_id, allowed, reason, trace_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
`
	if _, err := s.pool.Exec(ctx, q, audit.AgentID, audit.Action, audit.ResourceID,
		audit.Allowed, audit.Reason, audit.TraceID, metadata); err != nil {
		return fmt.Errorf("write action audit: %w", err)
	}
	return nil
}

type MemoryTemplateStore struct {
	permissions map[string]bool
	drafts      []AgentTemplateDraft
	audits      []AgentActionAudit
	nextID      int64
}

func NewMemoryTemplateStore(permissions map[string]bool) *MemoryTemplateStore {
	if permissions == nil {
		permissions = map[string]bool{}
	}
	return &MemoryTemplateStore{permissions: permissions, nextID: 1}
}

func (s *MemoryTemplateStore) CanCreateNPC(_ context.Context, agentID string) (bool, error) {
	return s.permissions[agentID], nil
}

func (s *MemoryTemplateStore) CreateDraft(_ context.Context, draft AgentTemplateDraft) (int64, error) {
	draft.ID = s.nextID
	draft.Status = "draft"
	draft.CreatedAt = time.Now()
	s.nextID++
	s.drafts = append(s.drafts, draft)
	return draft.ID, nil
}

func (s *MemoryTemplateStore) ListDrafts(_ context.Context, agentID string, limit int) ([]AgentTemplateDraft, error) {
	out := []AgentTemplateDraft{}
	for i := len(s.drafts) - 1; i >= 0 && len(out) < limit; i-- {
		if s.drafts[i].AgentID == agentID {
			out = append(out, s.drafts[i])
		}
	}
	return out, nil
}

func (s *MemoryTemplateStore) Audit(_ context.Context, audit AgentActionAudit) error {
	s.audits = append(s.audits, audit)
	return nil
}

func (s *MemoryTemplateStore) Audits() []AgentActionAudit {
	return append([]AgentActionAudit(nil), s.audits...)
}

var _ TemplateStore = (*TemplatePGStore)(nil)
var _ TemplateStore = (*MemoryTemplateStore)(nil)

func init() {
	_ = log.Printf
}
