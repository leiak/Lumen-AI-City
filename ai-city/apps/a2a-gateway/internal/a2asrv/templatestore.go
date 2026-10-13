package a2asrv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentTemplatePermission struct {
	AgentID      string
	CanCreateNPC bool
	DailyQuota   int
	GrantedBy    string
	Reason       string
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
	Version         int64
	ValidationTrace map[string]any
	SimulationTrace map[string]any
	ReviewedBy      string
	ReviewReason    string
	CreatedAt       time.Time
}

var (
	ErrTemplateNotFound      = errors.New("template draft not found")
	ErrTemplateState         = errors.New("template draft state rejected")
	ErrTemplatePermission    = errors.New("agent may not create NPC templates")
	ErrTemplateQuotaExceeded = errors.New("agent daily template quota exceeded")
)

type AgentActionAudit struct {
	AgentID    string
	Action     string
	ResourceID string
	Allowed    bool
	Reason     string
	TraceID    string
	Metadata   map[string]any
}

type AgentTemplateRevision struct {
	Version   int64
	TalkTree  map[string]any
	EditedBy  string
	CreatedAt time.Time
}

type TemplateStore interface {
	CanCreateNPC(ctx context.Context, agentID string) (bool, error)
	CreateDraft(ctx context.Context, draft AgentTemplateDraft) (int64, error)
	ListDrafts(ctx context.Context, agentID string, limit int) ([]AgentTemplateDraft, error)
	GetDraft(ctx context.Context, id int64) (AgentTemplateDraft, error)
	UpdateDraftTalkTree(ctx context.Context, id int64, agentID string, talkTree map[string]any, validationTrace map[string]any) (int64, error)
	SubmitDraft(ctx context.Context, id int64, agentID string) error
	SaveSimulationTrace(ctx context.Context, id int64, agentID string, trace map[string]any) error
	ReviewDraft(ctx context.Context, id int64, status, reviewer, reason string) error
	ListDraftsByStatus(ctx context.Context, status string, limit int) ([]AgentTemplateDraft, error)
	GrantPermission(ctx context.Context, permission AgentTemplatePermission) error
	Audit(ctx context.Context, audit AgentActionAudit) error
	ListDraftRevisions(ctx context.Context, id int64, agentID string) ([]AgentTemplateRevision, error)
	RollbackDraftToVersion(ctx context.Context, id int64, agentID string, version int64) (int64, error)
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
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire template connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin template transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var canCreate bool
	var quota int
	if err := tx.QueryRow(ctx,
		`SELECT can_create_npc, daily_quota FROM a2a_agent_permission WHERE agent_id = $1`,
		draft.AgentID,
	).Scan(&canCreate, &quota); err != nil {
		if err == pgx.ErrNoRows {
			return 0, ErrTemplatePermission
		}
		return 0, fmt.Errorf("agent permission query: %w", err)
	}
	if !canCreate {
		return 0, ErrTemplatePermission
	}

	var createdToday int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM a2a_npc_template_draft WHERE agent_id = $1 AND created_at >= date_trunc('day', NOW())`,
		draft.AgentID,
	).Scan(&createdToday); err != nil {
		return 0, fmt.Errorf("template quota query: %w", err)
	}
	if createdToday >= quota {
		return 0, ErrTemplateQuotaExceeded
	}

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
	if err := tx.QueryRow(ctx, q, draft.AgentID, draft.Name, draft.HomeTileID,
		draft.Description, personality, talkTree, catalog, trace).Scan(&id); err != nil {
		return 0, fmt.Errorf("create template draft: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO a2a_npc_template_revision
    (template_id, version, talk_tree, edited_by)
VALUES ($1, 1, $2::jsonb, $3)
`, id, talkTree, draft.AgentID); err != nil {
		return 0, fmt.Errorf("create template revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit template draft: %w", err)
	}
	return id, nil
}

func (s *TemplatePGStore) ListDrafts(ctx context.Context, agentID string, limit int) ([]AgentTemplateDraft, error) {
	const q = `
SELECT id, agent_id, name, home_tile_id, description, personality, talk_tree,
       product_catalog, status, version, validation_trace, simulation_trace,
       reviewed_by, review_reason, created_at
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
			simulationTrace []byte
		)
		if err := rows.Scan(&draft.ID, &draft.AgentID, &draft.Name, &draft.HomeTileID,
			&draft.Description, &personality, &talkTree, &catalog, &draft.Status,
			&draft.Version, &validationTrace, &simulationTrace, &draft.ReviewedBy,
			&draft.ReviewReason, &draft.CreatedAt); err != nil {
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
		if err := json.Unmarshal(simulationTrace, &draft.SimulationTrace); err != nil {
			return nil, fmt.Errorf("decode simulation trace: %w", err)
		}
		drafts = append(drafts, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate template drafts: %w", err)
	}
	return drafts, nil
}

func (s *TemplatePGStore) GetDraft(ctx context.Context, id int64) (AgentTemplateDraft, error) {
	const q = `
SELECT id, agent_id, name, home_tile_id, description, personality, talk_tree,
       product_catalog, status, version, validation_trace, simulation_trace,
       reviewed_by, review_reason, created_at
FROM a2a_npc_template_draft
WHERE id = $1
`
	var (
		draft           AgentTemplateDraft
		personality     []byte
		talkTree        []byte
		catalog         []byte
		validationTrace []byte
		simulationTrace []byte
	)
	err := s.pool.QueryRow(ctx, q, id).Scan(&draft.ID, &draft.AgentID, &draft.Name,
		&draft.HomeTileID, &draft.Description, &personality, &talkTree, &catalog,
		&draft.Status, &draft.Version, &validationTrace, &simulationTrace, &draft.ReviewedBy,
		&draft.ReviewReason, &draft.CreatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return AgentTemplateDraft{}, ErrTemplateNotFound
		}
		return AgentTemplateDraft{}, fmt.Errorf("get template draft: %w", err)
	}
	if err := json.Unmarshal(personality, &draft.Personality); err != nil {
		return AgentTemplateDraft{}, fmt.Errorf("decode personality: %w", err)
	}
	if err := json.Unmarshal(talkTree, &draft.TalkTree); err != nil {
		return AgentTemplateDraft{}, fmt.Errorf("decode talk tree: %w", err)
	}
	if err := json.Unmarshal(catalog, &draft.ProductCatalog); err != nil {
		return AgentTemplateDraft{}, fmt.Errorf("decode product catalog: %w", err)
	}
	if err := json.Unmarshal(validationTrace, &draft.ValidationTrace); err != nil {
		return AgentTemplateDraft{}, fmt.Errorf("decode validation trace: %w", err)
	}
	if err := json.Unmarshal(simulationTrace, &draft.SimulationTrace); err != nil {
		return AgentTemplateDraft{}, fmt.Errorf("decode simulation trace: %w", err)
	}
	return draft, nil
}

func (s *TemplatePGStore) UpdateDraftTalkTree(ctx context.Context, id int64, agentID string,
	talkTree map[string]any, validationTrace map[string]any) (int64, error) {
	encodedTree, err := json.Marshal(talkTree)
	if err != nil {
		return 0, fmt.Errorf("encode editor talk tree: %w", err)
	}
	encodedTrace, err := json.Marshal(validationTrace)
	if err != nil {
		return 0, fmt.Errorf("encode editor validation trace: %w", err)
	}

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire editor connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin editor transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	err = tx.QueryRow(ctx,
		`SELECT status FROM a2a_npc_template_draft WHERE id = $1 AND agent_id = $2`,
		id, agentID).Scan(&currentStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, ErrTemplateNotFound
		}
		return 0, fmt.Errorf("editor draft query: %w", err)
	}
	if currentStatus != "draft" && currentStatus != "rejected" {
		return 0, ErrTemplateState
	}

	var version int64
	err = tx.QueryRow(ctx, `
UPDATE a2a_npc_template_draft
SET talk_tree = $3::jsonb, validation_trace = $4::jsonb,
    version = version + 1, updated_at = NOW()
WHERE id = $1 AND agent_id = $2 AND status IN ('draft', 'rejected')
RETURNING version
`, id, agentID, encodedTree, encodedTrace).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("update editor talk tree: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO a2a_npc_template_revision
    (template_id, version, talk_tree, edited_by)
VALUES ($1, $2, $3::jsonb, $4)
`, id, version, encodedTree, agentID); err != nil {
		return 0, fmt.Errorf("insert editor revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit editor talk tree: %w", err)
	}
	return version, nil
}

func (s *TemplatePGStore) ListDraftRevisions(ctx context.Context, id int64, agentID string,
) ([]AgentTemplateRevision, error) {
	const q = `
SELECT r.version, r.talk_tree, r.edited_by, r.created_at
FROM a2a_npc_template_revision r
JOIN a2a_npc_template_draft d ON d.id = r.template_id
WHERE d.id = $1 AND d.agent_id = $2
ORDER BY r.version DESC
`
	rows, err := s.pool.Query(ctx, q, id, agentID)
	if err != nil {
		return nil, fmt.Errorf("list template revisions: %w", err)
	}
	defer rows.Close()

	revisions := []AgentTemplateRevision{}
	for rows.Next() {
		var revision AgentTemplateRevision
		var talkTree []byte
		if err := rows.Scan(&revision.Version, &talkTree, &revision.EditedBy,
			&revision.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan template revision: %w", err)
		}
		if err := json.Unmarshal(talkTree, &revision.TalkTree); err != nil {
			return nil, fmt.Errorf("decode template revision: %w", err)
		}
		revisions = append(revisions, revision)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate template revisions: %w", err)
	}
	return revisions, nil
}

func (s *TemplatePGStore) RollbackDraftToVersion(ctx context.Context, id int64,
	agentID string, version int64) (int64, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire rollback connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin rollback transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM a2a_npc_template_draft WHERE id = $1 AND agent_id = $2`,
		id, agentID).Scan(&status)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, ErrTemplateNotFound
		}
		return 0, fmt.Errorf("rollback draft query: %w", err)
	}
	if status != "draft" && status != "rejected" {
		return 0, ErrTemplateState
	}

	var targetTree []byte
	err = tx.QueryRow(ctx, `
SELECT talk_tree FROM a2a_npc_template_revision
WHERE template_id = $1 AND version = $2
`, id, version).Scan(&targetTree)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, ErrTemplateNotFound
		}
		return 0, fmt.Errorf("rollback revision query: %w", err)
	}

	var newVersion int64
	err = tx.QueryRow(ctx, `
UPDATE a2a_npc_template_draft
SET talk_tree = $3::jsonb, validation_trace = '{}'::jsonb,
    version = version + 1, updated_at = NOW()
WHERE id = $1 AND agent_id = $2 AND status IN ('draft', 'rejected')
RETURNING version
`, id, agentID, targetTree).Scan(&newVersion)
	if err != nil {
		return 0, fmt.Errorf("rollback template draft: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO a2a_npc_template_revision
    (template_id, version, talk_tree, edited_by)
VALUES ($1, $2, $3::jsonb, $4)
`, id, newVersion, targetTree, agentID); err != nil {
		return 0, fmt.Errorf("insert rollback revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit rollback: %w", err)
	}
	return newVersion, nil
}

func (s *TemplatePGStore) SubmitDraft(ctx context.Context, id int64, agentID string) error {
	const q = `
UPDATE a2a_npc_template_draft
SET status = 'submitted', submitted_at = NOW(), updated_at = NOW()
WHERE id = $1 AND agent_id = $2 AND status IN ('draft', 'rejected')
`
	tag, err := s.pool.Exec(ctx, q, id, agentID)
	if err != nil {
		return fmt.Errorf("submit template draft: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTemplateState
	}
	return nil
}

func (s *TemplatePGStore) SaveSimulationTrace(ctx context.Context, id int64, agentID string, trace map[string]any) error {
	encoded, err := json.Marshal(trace)
	if err != nil {
		return fmt.Errorf("encode simulation trace: %w", err)
	}
	const q = `
UPDATE a2a_npc_template_draft
SET simulation_trace = $3::jsonb, updated_at = NOW()
WHERE id = $1 AND agent_id = $2
`
	tag, err := s.pool.Exec(ctx, q, id, agentID, encoded)
	if err != nil {
		return fmt.Errorf("save simulation trace: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTemplateNotFound
	}
	return nil
}

func (s *TemplatePGStore) ReviewDraft(ctx context.Context, id int64, status, reviewer, reason string) error {
	const q = `
UPDATE a2a_npc_template_draft
SET status = $2, reviewed_by = $3, review_reason = $4,
    reviewed_at = NOW(), updated_at = NOW()
WHERE id = $1 AND status = 'submitted'
`
	tag, err := s.pool.Exec(ctx, q, id, status, reviewer, reason)
	if err != nil {
		return fmt.Errorf("review template draft: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrTemplateState
	}
	return nil
}

func (s *TemplatePGStore) ListDraftsByStatus(ctx context.Context, status string, limit int) ([]AgentTemplateDraft, error) {
	const q = `
SELECT id, agent_id, name, home_tile_id, description, personality, talk_tree,
       product_catalog, status, version, validation_trace, simulation_trace,
       reviewed_by, review_reason, created_at
FROM a2a_npc_template_draft
WHERE status = $1
ORDER BY created_at DESC, id DESC
LIMIT $2
`
	rows, err := s.pool.Query(ctx, q, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list review drafts: %w", err)
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
			simulationTrace []byte
		)
		if err := rows.Scan(&draft.ID, &draft.AgentID, &draft.Name, &draft.HomeTileID,
			&draft.Description, &personality, &talkTree, &catalog, &draft.Status,
			&draft.Version, &validationTrace, &simulationTrace, &draft.ReviewedBy,
			&draft.ReviewReason, &draft.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan review draft: %w", err)
		}
		for _, pair := range []struct {
			raw    []byte
			target any
		}{
			{personality, &draft.Personality}, {talkTree, &draft.TalkTree},
			{catalog, &draft.ProductCatalog}, {validationTrace, &draft.ValidationTrace},
			{simulationTrace, &draft.SimulationTrace},
		} {
			if err := json.Unmarshal(pair.raw, pair.target); err != nil {
				return nil, fmt.Errorf("decode review draft: %w", err)
			}
		}
		drafts = append(drafts, draft)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate review drafts: %w", err)
	}
	return drafts, nil
}

func (s *TemplatePGStore) GrantPermission(ctx context.Context, permission AgentTemplatePermission) error {
	const q = `
INSERT INTO a2a_agent_permission
    (agent_id, can_create_npc, daily_quota, granted_by, reason)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (agent_id) DO UPDATE SET
    can_create_npc = EXCLUDED.can_create_npc,
    daily_quota = EXCLUDED.daily_quota,
    granted_by = EXCLUDED.granted_by,
    reason = EXCLUDED.reason,
    updated_at = NOW()
`
	if _, err := s.pool.Exec(ctx, q, permission.AgentID, permission.CanCreateNPC,
		permission.DailyQuota, permission.GrantedBy, permission.Reason); err != nil {
		return fmt.Errorf("grant template permission: %w", err)
	}
	return nil
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
	draft.Version = 1
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

func (s *MemoryTemplateStore) GetDraft(_ context.Context, id int64) (AgentTemplateDraft, error) {
	for _, draft := range s.drafts {
		if draft.ID == id {
			return draft, nil
		}
	}
	return AgentTemplateDraft{}, ErrTemplateNotFound
}

func (s *MemoryTemplateStore) UpdateDraftTalkTree(_ context.Context, id int64, agentID string,
	talkTree map[string]any, validationTrace map[string]any) (int64, error) {
	for index, draft := range s.drafts {
		if draft.ID != id || draft.AgentID != agentID {
			continue
		}
		if draft.Status != "draft" && draft.Status != "rejected" {
			return 0, ErrTemplateState
		}
		s.drafts[index].TalkTree = talkTree
		s.drafts[index].ValidationTrace = validationTrace
		s.drafts[index].Version = draft.Version + 1
		return s.drafts[index].Version, nil
	}
	return 0, ErrTemplateNotFound
}

func (s *MemoryTemplateStore) SubmitDraft(_ context.Context, id int64, agentID string) error {
	for index, draft := range s.drafts {
		if draft.ID == id && draft.AgentID == agentID &&
			(draft.Status == "draft" || draft.Status == "rejected") {
			s.drafts[index].Status = "submitted"
			return nil
		}
	}
	return ErrTemplateState
}

func (s *MemoryTemplateStore) SaveSimulationTrace(_ context.Context, id int64, agentID string, trace map[string]any) error {
	for index, draft := range s.drafts {
		if draft.ID == id && draft.AgentID == agentID {
			s.drafts[index].SimulationTrace = trace
			return nil
		}
	}
	return ErrTemplateNotFound
}

func (s *MemoryTemplateStore) ReviewDraft(_ context.Context, id int64, status, reviewer, reason string) error {
	for index, draft := range s.drafts {
		if draft.ID == id && draft.Status == "submitted" {
			s.drafts[index].Status = status
			s.drafts[index].ReviewedBy = reviewer
			s.drafts[index].ReviewReason = reason
			return nil
		}
	}
	return ErrTemplateState
}

func (s *MemoryTemplateStore) ListDraftsByStatus(_ context.Context, status string, limit int) ([]AgentTemplateDraft, error) {
	out := []AgentTemplateDraft{}
	for i := len(s.drafts) - 1; i >= 0 && len(out) < limit; i-- {
		if s.drafts[i].Status == status {
			out = append(out, s.drafts[i])
		}
	}
	return out, nil
}

func (s *MemoryTemplateStore) GrantPermission(_ context.Context, permission AgentTemplatePermission) error {
	s.permissions[permission.AgentID] = permission.CanCreateNPC
	return nil
}

func (s *MemoryTemplateStore) ListDraftRevisions(_ context.Context, id int64, agentID string,
) ([]AgentTemplateRevision, error) {
	for _, draft := range s.drafts {
		if draft.ID == id && draft.AgentID == agentID {
			revisions := []AgentTemplateRevision{}
			for version := draft.Version; version >= 1; version-- {
				revisions = append(revisions, AgentTemplateRevision{
					Version:   version,
					TalkTree:  draft.TalkTree,
					EditedBy:  agentID,
					CreatedAt: draft.CreatedAt,
				})
			}
			return revisions, nil
		}
	}
	return nil, ErrTemplateNotFound
}

func (s *MemoryTemplateStore) RollbackDraftToVersion(_ context.Context, id int64,
	agentID string, version int64) (int64, error) {
	for index, draft := range s.drafts {
		if draft.ID != id || draft.AgentID != agentID {
			continue
		}
		if draft.Status != "draft" && draft.Status != "rejected" {
			return 0, ErrTemplateState
		}
		if version < 1 || version > draft.Version {
			return 0, ErrTemplateNotFound
		}
		s.drafts[index].ValidationTrace = map[string]any{}
		s.drafts[index].Version = draft.Version + 1
		return s.drafts[index].Version, nil
	}
	return 0, ErrTemplateNotFound
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
