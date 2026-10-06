-- ============================================
-- Phase C.4 — Admin role for BT editor access
-- ============================================
-- Adds a ``role`` column to the existing ``player`` table so that
-- admin-portal can issue a cookie-session JWT with a ``role`` claim
-- and its middleware can gate /bt-editor + /api/bt/* accordingly.
--
-- Idempotent: safe to re-run on an already-initialised database.
-- pgcrypto is required (for crypt() / gen_salt() in seed-admin.sql);
-- pg-schema.sql already CREATE EXTENSIONs it, but we add an IF NOT
-- EXISTS guard here so this migration can be applied standalone.
--
-- Default 'player' keeps existing rows untouched — no app-side
-- migration needed for the demo / lihua / etc. players.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

ALTER TABLE player
    ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'player';

-- Hot-path index: only admins are interesting for the BT editor gate.
-- Partial index keeps it tiny (only rows where role != 'player').
CREATE INDEX IF NOT EXISTS idx_player_role
    ON player(role)
    WHERE role <> 'player';

COMMENT ON COLUMN player.role IS 'Phase C.4: player (default) | admin. Drives BT editor auth gate.';
