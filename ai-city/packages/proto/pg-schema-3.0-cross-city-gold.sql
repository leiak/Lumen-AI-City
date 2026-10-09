-- packages/proto/pg-schema-3.0-cross-city-gold.sql
-- 3.0 v4: cross-city gold transfer bridge ledger
-- Idempotent: safe to re-run
--
-- Each city owns its local PostgreSQL database. `cross_city_transfer` stores the
-- local leg only. Source and destination rows share `global_id` but are never
-- copied into one database. `bridge_position` is the signed clearing account.

CREATE TABLE IF NOT EXISTS cross_city_transfer (
    id                  BIGSERIAL PRIMARY KEY,
    global_id           UUID NOT NULL,
    direction           TEXT NOT NULL CHECK (direction IN ('outbound','inbound')),
    source_city_id      TEXT NOT NULL,
    destination_city_id TEXT NOT NULL,
    source_user_id      TEXT NOT NULL,
    destination_user_id TEXT NOT NULL,
    currency            TEXT NOT NULL CHECK (currency = 'gold'),
    amount              BIGINT NOT NULL CHECK (amount > 0 AND amount <= 100000),
    status              TEXT NOT NULL CHECK (
                          (direction = 'outbound' AND status IN ('reserved','settled','refunded','failed'))
                          OR
                          (direction = 'inbound' AND status IN ('credited','failed'))
                        ),
    idempotency_key     TEXT NOT NULL,
    trace_id            TEXT,
    expires_at          TIMESTAMPTZ NOT NULL,
    reserved_at         TIMESTAMPTZ,
    credited_at         TIMESTAMPTZ,
    settled_at          TIMESTAMPTZ,
    refunded_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (direction, idempotency_key),
    UNIQUE (global_id, direction)
);

CREATE INDEX IF NOT EXISTS idx_cct_outbound_reconcile
    ON cross_city_transfer(expires_at)
    WHERE direction = 'outbound' AND status = 'reserved';
CREATE INDEX IF NOT EXISTS idx_cct_source_user
    ON cross_city_transfer(source_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cct_destination_user
    ON cross_city_transfer(destination_user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS bridge_position (
    id            BIGSERIAL PRIMARY KEY,
    peer_city_id  TEXT NOT NULL,
    direction     TEXT NOT NULL CHECK (direction IN ('outbound','inbound')),
    currency      TEXT NOT NULL CHECK (currency = 'gold'),
    balance       BIGINT NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (peer_city_id, direction, currency)
);

CREATE TABLE IF NOT EXISTS bridge_ledger_entry (
    id                 BIGSERIAL PRIMARY KEY,
    transfer_global_id UUID NOT NULL,
    direction          TEXT NOT NULL CHECK (direction IN ('outbound','inbound')),
    peer_city_id       TEXT NOT NULL,
    currency           TEXT NOT NULL CHECK (currency = 'gold'),
    amount             BIGINT NOT NULL CHECK (amount != 0),
    balance_after      BIGINT NOT NULL,
    reason             TEXT NOT NULL CHECK (
                         reason IN ('reserve','credit','refund','reconciliation')
                       ),
    trace_id           TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bridge_entry_transfer
    ON bridge_ledger_entry(transfer_global_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bridge_entry_peer
    ON bridge_ledger_entry(peer_city_id, direction, created_at DESC);

ALTER TABLE transaction DROP CONSTRAINT IF EXISTS transaction_tx_type_check;
ALTER TABLE transaction ADD CONSTRAINT transaction_tx_type_check
    CHECK (tx_type IN (
      'player_transfer','npc_purchase','central_bank_emit','npc_sink',
      'cross_city_out','cross_city_in'
    ));
