-- Phase 3.0 v3: Creator marketplace + three-role identity
-- Idempotent: safe to re-run.

ALTER TABLE player
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'player';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'player_role_three_roles_check'
          AND conrelid = 'player'::regclass
    ) THEN
        ALTER TABLE player
            ADD CONSTRAINT player_role_three_roles_check
            CHECK (role IN ('player', 'creator', 'admin'));
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_player_non_player_role
    ON player(role)
    WHERE role <> 'player';

CREATE TABLE IF NOT EXISTS npc_template (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      UUID NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    name            TEXT NOT NULL,
    avatar_url      TEXT,
    ocean_json      JSONB NOT NULL,
    bt_skeleton     TEXT,
    product_catalog JSONB,
    price_gold      BIGINT NOT NULL CHECK (price_gold >= 10),
    status          TEXT NOT NULL DEFAULT 'live'
                    CHECK (status IN ('live', 'taken_down')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_npc_template_creator
    ON npc_template(creator_id);
CREATE INDEX IF NOT EXISTS idx_npc_template_live
    ON npc_template(status)
    WHERE status = 'live';

CREATE TABLE IF NOT EXISTS saga_template (
    id               BIGSERIAL PRIMARY KEY,
    creator_id       UUID NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    name             TEXT NOT NULL,
    icon_url         TEXT,
    description      TEXT,
    yaml_content     TEXT NOT NULL,
    npc_deps         TEXT[] NOT NULL DEFAULT '{}',
    price_gold       BIGINT NOT NULL DEFAULT 10
                     CONSTRAINT saga_template_price_min CHECK (price_gold >= 10),
    semantic_version TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'live'
                     CHECK (status IN ('live', 'taken_down')),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_saga_template_creator
    ON saga_template(creator_id);
CREATE INDEX IF NOT EXISTS idx_saga_template_live
    ON saga_template(status)
    WHERE status = 'live';

ALTER TABLE saga_template
    ADD COLUMN IF NOT EXISTS price_gold BIGINT NOT NULL DEFAULT 10;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'saga_template_price_min'
          AND conrelid = 'saga_template'::regclass
    ) THEN
        ALTER TABLE saga_template
            ADD CONSTRAINT saga_template_price_min
            CHECK (price_gold >= 10);
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS template_purchase (
    id              BIGSERIAL PRIMARY KEY,
    user_id         UUID NOT NULL REFERENCES player(id) ON DELETE CASCADE,
    template_kind   TEXT NOT NULL CHECK (template_kind IN ('npc', 'saga')),
    template_id     BIGINT NOT NULL,
    price_paid_gold BIGINT NOT NULL CHECK (price_paid_gold > 0),
    idempotency_key TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_template_purchase_user
    ON template_purchase(user_id);

CREATE TABLE IF NOT EXISTS creator_revenue (
    id                BIGSERIAL PRIMARY KEY,
    creator_id        UUID NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    purchase_id       BIGINT NOT NULL REFERENCES template_purchase(id),
    amount_gold       BIGINT NOT NULL CHECK (amount_gold > 0),
    platform_cut_gold BIGINT NOT NULL DEFAULT 0 CHECK (platform_cut_gold >= 0),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_creator_revenue_creator_created
    ON creator_revenue(creator_id, created_at DESC);

ALTER TABLE transaction
    DROP CONSTRAINT IF EXISTS transaction_tx_type_check;

ALTER TABLE transaction
    ADD CONSTRAINT transaction_tx_type_check
    CHECK (tx_type IN (
        'player_transfer', 'npc_purchase', 'central_bank_emit',
        'npc_sink', 'cross_city_out', 'cross_city_in', 'creator_withdrawal'
    ));

CREATE TABLE IF NOT EXISTS creator_withdrawal (
    id              BIGSERIAL PRIMARY KEY,
    creator_id      UUID NOT NULL REFERENCES player(id) ON DELETE RESTRICT,
    amount_gold     BIGINT NOT NULL CHECK (amount_gold > 0),
    balance_after   BIGINT NOT NULL,
    idempotency_key TEXT UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_creator_withdrawal_creator_created
    ON creator_withdrawal(creator_id, created_at DESC);
