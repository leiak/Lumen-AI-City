-- Phase 4.0 M4 D1: privacy consent, data export, and erasure accounting.
-- Financial and regulatory audit records are retained; personal profile
-- fields are irreversibly anonymized when the subject requests erasure.

CREATE TABLE IF NOT EXISTS privacy_consent (
    user_id              UUID PRIMARY KEY REFERENCES player(id) ON DELETE CASCADE,
    essential            BOOLEAN NOT NULL DEFAULT TRUE,
    analytics            BOOLEAN NOT NULL DEFAULT FALSE,
    marketing            BOOLEAN NOT NULL DEFAULT FALSE,
    third_party_sharing  BOOLEAN NOT NULL DEFAULT FALSE,
    policy_version       TEXT NOT NULL DEFAULT '2026-10-14',
    accepted_at          TIMESTAMPTZ,
    withdrawn_at         TIMESTAMPTZ,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS privacy_export_request (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID NOT NULL REFERENCES player(id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'completed', 'failed')),
    payload      JSONB,
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_privacy_export_user_time
    ON privacy_export_request(user_id, requested_at DESC);

CREATE TABLE IF NOT EXISTS privacy_erasure_request (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id      UUID NOT NULL REFERENCES player(id) ON DELETE CASCADE,
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'completed', 'failed')),
    reason       TEXT NOT NULL DEFAULT 'subject_request',
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_privacy_erasure_user_time
    ON privacy_erasure_request(user_id, requested_at DESC);

CREATE TABLE IF NOT EXISTS privacy_action_audit (
    id         BIGSERIAL PRIMARY KEY,
    actor_id   TEXT NOT NULL,
    action     TEXT NOT NULL CHECK (action IN
                 ('consent.update', 'consent.withdraw', 'data.export', 'data.erase')),
    request_id TEXT NOT NULL DEFAULT '',
    allowed    BOOLEAN NOT NULL DEFAULT TRUE,
    metadata   JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_privacy_action_actor_time
    ON privacy_action_audit(actor_id, created_at DESC);

COMMENT ON TABLE privacy_consent IS 'PIPL/GDPR consent ledger; essential processing is never optional';
COMMENT ON TABLE privacy_export_request IS 'Data subject export evidence and immutable JSON payload';
COMMENT ON TABLE privacy_erasure_request IS 'Data subject erasure evidence; completed means anonymized';
COMMENT ON TABLE privacy_action_audit IS 'Privacy action evidence; actor_id intentionally has no cascade delete';
