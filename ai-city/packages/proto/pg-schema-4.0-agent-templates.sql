-- Phase 4.0 M2 B1: explicit permissions and draft ownership for Agent templates.
-- Default deny: only rows in a2a_agent_permission grant authoring.

CREATE TABLE IF NOT EXISTS a2a_agent_permission (
    agent_id        VARCHAR(128) PRIMARY KEY,
    can_create_npc  BOOLEAN NOT NULL DEFAULT FALSE,
    daily_quota     INTEGER NOT NULL DEFAULT 10 CHECK (daily_quota > 0),
    granted_by      VARCHAR(128) NOT NULL DEFAULT '',
    reason          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_agent_permission_create_npc
    ON a2a_agent_permission(can_create_npc)
    WHERE can_create_npc;

CREATE TABLE IF NOT EXISTS a2a_npc_template_draft (
    id               BIGSERIAL PRIMARY KEY,
    agent_id         VARCHAR(128) NOT NULL,
    name             TEXT NOT NULL,
    home_tile_id     TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    personality      JSONB NOT NULL DEFAULT '{}'::jsonb,
    talk_tree        JSONB NOT NULL,
    product_catalog  JSONB NOT NULL DEFAULT '[]'::jsonb,
    status           TEXT NOT NULL DEFAULT 'draft'
                     CHECK (status IN ('draft', 'submitted', 'approved', 'rejected')),
    validation_trace JSONB NOT NULL DEFAULT '{}'::jsonb,
    simulation_trace JSONB NOT NULL DEFAULT '{}'::jsonb,
    submitted_at     TIMESTAMPTZ,
    reviewed_by      VARCHAR(128) NOT NULL DEFAULT '',
    review_reason    TEXT NOT NULL DEFAULT '',
    reviewed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_npc_template_draft_agent
    ON a2a_npc_template_draft(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a_npc_template_draft_status
    ON a2a_npc_template_draft(status);

ALTER TABLE a2a_npc_template_draft
    ADD COLUMN IF NOT EXISTS simulation_trace JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE a2a_npc_template_draft
    ADD COLUMN IF NOT EXISTS submitted_at TIMESTAMPTZ;
ALTER TABLE a2a_npc_template_draft
    ADD COLUMN IF NOT EXISTS reviewed_by VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE a2a_npc_template_draft
    ADD COLUMN IF NOT EXISTS review_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE a2a_npc_template_draft
    ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS a2a_agent_action_audit (
    id          BIGSERIAL PRIMARY KEY,
    agent_id    VARCHAR(128) NOT NULL,
    action      VARCHAR(64) NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    allowed     BOOLEAN NOT NULL,
    reason      TEXT NOT NULL DEFAULT '',
    trace_id    TEXT NOT NULL DEFAULT '',
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_agent_action_audit_agent_time
    ON a2a_agent_action_audit(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a_agent_action_audit_action
    ON a2a_agent_action_audit(action);
