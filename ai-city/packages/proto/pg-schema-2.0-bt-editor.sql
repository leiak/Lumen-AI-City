-- ============================================
-- Phase C.2 — BT tree persistence for admin-portal /bt-editor page
-- ============================================
-- Stores behavior-tree JSON blobs authored via the BT editor (admin-portal
-- Phase C.3 UI) and re-used by the dispatcher (Phase C.4). One row per
-- (npc_id, tree_name); upserts via UNIQUE constraint + ON CONFLICT.
--
-- Idempotent: safe to re-run on an already-initialised database. Trigger
-- is dropped + recreated so a function-body change picks up without manual
-- DDL surgery.

CREATE TABLE IF NOT EXISTS bt_tree (
    id SERIAL PRIMARY KEY,
    npc_id TEXT NOT NULL,
    name TEXT NOT NULL,
    tree_json JSONB NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(npc_id, name)
);

-- Hot-path index: list trees per NPC.
CREATE INDEX IF NOT EXISTS idx_bt_tree_npc_id ON bt_tree(npc_id);

-- Bump version + updated_at on every UPDATE so the editor can show "v3
-- edited 5 min ago" without the application layer needing to remember.
CREATE OR REPLACE FUNCTION bt_tree_bump_version()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_bt_tree_bump_version ON bt_tree;
CREATE TRIGGER trg_bt_tree_bump_version
    BEFORE UPDATE ON bt_tree
    FOR EACH ROW
    EXECUTE FUNCTION bt_tree_bump_version();
