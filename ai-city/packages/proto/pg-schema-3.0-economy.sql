-- packages/proto/pg-schema-3.0-economy.sql
-- Phase 3.0: Economy system — wallet + transaction + product + central_bank_ledger
-- Idempotent: safe to re-run
--
-- NOTE on wallet.user_id type:
--   player.id is UUID (defined in pg-schema.sql:16). The plan originally
--   specified `user_id TEXT PRIMARY KEY REFERENCES player(id)`, but
--   PostgreSQL rejects FKs between TEXT and UUID. The verification
--   (Step 4) also expects `INSERT ... VALUES ('test_user')` to succeed
--   against an arbitrary string with no matching player row, which a
--   real FK would forbid. Resolution: keep user_id as TEXT (so wallet
--   can later hold non-UUID external IDs if needed) and drop the FK
--   constraint. Referential integrity becomes an application-layer
--   concern (wallet_service inserts the matching player first, or
--   the caller passes a verified UUID). If a hard FK is required
--   later, change user_id to UUID and update service code.

CREATE TABLE IF NOT EXISTS wallet (
    user_id       TEXT PRIMARY KEY,
    gold_balance  BIGINT NOT NULL DEFAULT 0 CHECK (gold_balance >= 0),
    token_balance BIGINT NOT NULL DEFAULT 0 CHECK (token_balance >= 0),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS transaction (
    id              BIGSERIAL PRIMARY KEY,
    tx_type         TEXT NOT NULL CHECK (tx_type IN
                       ('player_transfer','npc_purchase','central_bank_emit','npc_sink')),
    user_id         TEXT NOT NULL,
    counterparty_id TEXT,
    currency        TEXT NOT NULL CHECK (currency IN ('gold','token')),
    amount          BIGINT NOT NULL CHECK (amount != 0),
    balance_after   BIGINT NOT NULL,
    product_id      BIGINT,
    trace_id        TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tx_user_id_created ON transaction(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tx_type_created ON transaction(tx_type, created_at DESC);

-- wallet updated_at trigger
CREATE OR REPLACE FUNCTION wallet_bump_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_wallet_bump_updated_at ON wallet;
CREATE TRIGGER trg_wallet_bump_updated_at
    BEFORE UPDATE ON wallet
    FOR EACH ROW
    EXECUTE FUNCTION wallet_bump_updated_at();

-- transaction append-only (block UPDATE/DELETE)
CREATE OR REPLACE FUNCTION transaction_block_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'transaction is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_transaction_no_update ON transaction;
CREATE TRIGGER trg_transaction_no_update
    BEFORE UPDATE ON transaction
    FOR EACH ROW
    EXECUTE FUNCTION transaction_block_mutation();

DROP TRIGGER IF EXISTS trg_transaction_no_delete ON transaction;
CREATE TRIGGER trg_transaction_no_delete
    BEFORE DELETE ON transaction
    FOR EACH ROW
    EXECUTE FUNCTION transaction_block_mutation();
