-- ============================================
-- AI City - PostgreSQL Schema (v2.3)
-- 对应 docs/03-数据Schema.md §17.1
-- ============================================

-- 启用扩展
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";      -- UUID 生成
CREATE EXTENSION IF NOT EXISTS "pgcrypto";        -- 加密
CREATE EXTENSION IF NOT EXISTS "pg_trgm";         -- 模糊搜索
CREATE EXTENSION IF NOT EXISTS "btree_gin";       -- JSONB 索引

-- ============================================
-- 玩家表（player）
-- ============================================
CREATE TABLE IF NOT EXISTS player (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username        VARCHAR(64) UNIQUE NOT NULL,
    email           VARCHAR(128) UNIQUE,
    password_hash   TEXT NOT NULL,                       -- bcrypt
    display_name    VARCHAR(64) NOT NULL,
    avatar_url      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,

    -- OCEAN 五维人格（玩家自设，0-100）
    ocean           JSONB NOT NULL DEFAULT '{"O":50,"C":50,"E":50,"A":50,"N":50}',

    -- 玩家设置
    settings        JSONB NOT NULL DEFAULT '{}',

    -- 软删
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_player_username ON player(username) WHERE deleted_at IS NULL;
CREATE INDEX idx_player_email ON player(email) WHERE deleted_at IS NULL;
CREATE INDEX idx_player_created ON player(created_at DESC);

-- ============================================
-- NPC 表（npc）
-- ============================================
CREATE TABLE IF NOT EXISTS npc (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id        VARCHAR(128) UNIQUE NOT NULL,         -- 全局唯一业务 ID（如 npc_wang_boss_001）
    name            VARCHAR(64) NOT NULL,
    avatar_url      TEXT,
    home_tile_id    VARCHAR(64),

    -- OCEAN 五维人格（必填，0-100）
    ocean           JSONB NOT NULL,

    -- 说话风格
    speech_style    JSONB NOT NULL DEFAULT '{}',

    -- 背景故事
    backstory       TEXT,

    -- 标签
    tags            TEXT[] NOT NULL DEFAULT '{}',

    -- 行为树引用（packages/bt-editor-spec/bt-tree.schema.json）
    behavior_tree_id VARCHAR(128),

    -- 默认 LOD
    default_lod     SMALLINT NOT NULL DEFAULT 1 CHECK (default_lod BETWEEN 0 AND 2),

    -- LLM prompt hints
    prompt_hints    JSONB NOT NULL DEFAULT '[]',

    -- 时间表
    schedule        JSONB NOT NULL DEFAULT '[]',

    -- 元数据
    template_version VARCHAR(16) NOT NULL DEFAULT '1.0',
    author          VARCHAR(64),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_npc_agent_id ON npc(agent_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_npc_home_tile ON npc(home_tile_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_npc_tags ON npc USING GIN(tags) WHERE deleted_at IS NULL;

-- ============================================
-- 玩家位置表（player_position）
-- ============================================
CREATE TABLE IF NOT EXISTS player_position (
    player_id       UUID PRIMARY KEY REFERENCES player(id) ON DELETE CASCADE,
    tile_id         VARCHAR(64) NOT NULL,
    x               REAL NOT NULL DEFAULT 0,
    y               REAL NOT NULL DEFAULT 0,
    heading         REAL NOT NULL DEFAULT 0,    -- 朝向 0-360
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_position_tile ON player_position(tile_id);
CREATE INDEX idx_position_updated ON player_position(updated_at DESC);

-- ============================================
-- Tile 表（tile）—— 世界地理 + 静态建筑物
-- ============================================
CREATE TABLE IF NOT EXISTS tile (
    id              VARCHAR(64) PRIMARY KEY,           -- 如 'tile_0_0'
    center_x        REAL NOT NULL,                     -- 中心点 x（米）
    center_y        REAL NOT NULL,                     -- 中心点 y（米）
    size            REAL NOT NULL DEFAULT 100.0,        -- 边长（米）
    lod_level       SMALLINT NOT NULL DEFAULT 1        -- 0=CBD / 1=Residential / 2=Suburb
                    CHECK (lod_level BETWEEN 0 AND 2),
    buildings       JSONB NOT NULL DEFAULT '[]',       -- [{id, kind, polygon:[...]}, ...]
    npc_ids         TEXT[] NOT NULL DEFAULT '{}',      -- 种子 NPC 的 agent_id
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tile_enabled ON tile(enabled) WHERE enabled = TRUE;
CREATE INDEX idx_tile_lod ON tile(lod_level);

CREATE TRIGGER tile_updated_at BEFORE UPDATE ON tile
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

COMMENT ON TABLE tile IS '世界地理 Tile + 静态建筑物（运行期 player_ids 不落库）';

-- ============================================
-- Tile 种子数据：3×3 网格（中心 0,0 + 4 邻 + 4 角）
-- ============================================
INSERT INTO tile (id, center_x, center_y, lod_level, buildings, npc_ids) VALUES
  -- CBD 中心
  ('tile_0_0',  50.0,  50.0, 0,
   '[
      {"id":"bldg_tavern_0_0","kind":"Tavern","polygon":[[0.0,0.0],[20.0,0.0],[20.0,15.0],[0.0,15.0]]},
      {"id":"bldg_plaza_0_0","kind":"Plaza","polygon":[[30.0,30.0],[70.0,30.0],[70.0,70.0],[30.0,70.0]]}
    ]'::jsonb,
   ARRAY['npc_wang_boss_001']::text[]),
  -- 北
  ('tile_0_1',   50.0, 150.0, 1, '[]'::jsonb, ARRAY[]::text[]),
  -- 南
  ('tile_0_-1',  50.0, -50.0, 1, '[]'::jsonb, ARRAY[]::text[]),
  -- 东
  ('tile_1_0',  150.0,  50.0, 1,
   '[
      {"id":"bldg_house_1_0","kind":"House","polygon":[[10.0,10.0],[25.0,10.0],[25.0,25.0],[10.0,25.0]]},
      {"id":"bldg_shop_1_0","kind":"Shop","polygon":[[60.0,60.0],[80.0,60.0],[80.0,80.0],[60.0,80.0]]}
    ]'::jsonb,
   ARRAY['npc_lihua_001']::text[]),
  -- 西
  ('tile_-1_0', -50.0,  50.0, 1, '[]'::jsonb, ARRAY[]::text[]),
  -- 西北角（Suburb，含 NPC + Park）
  ('tile_-1_1', -50.0, 150.0, 2,
   '[
      {"id":"bldg_park_-1_1","kind":"Park","polygon":[[0.0,0.0],[90.0,0.0],[90.0,90.0],[0.0,90.0]]}
    ]'::jsonb,
   ARRAY['npc_zhang_granny_001']::text[]),
  -- 东北角
  ('tile_1_1',  150.0, 150.0, 2, '[]'::jsonb, ARRAY[]::text[]),
  -- 西南角
  ('tile_-1_-1', -50.0, -50.0, 2, '[]'::jsonb, ARRAY[]::text[]),
  -- 东南角
  ('tile_1_-1', 150.0, -50.0, 2, '[]'::jsonb, ARRAY[]::text[])
ON CONFLICT (id) DO NOTHING;

-- ============================================
-- NPC 位置表（npc_position）
-- ============================================
CREATE TABLE IF NOT EXISTS npc_position (
    npc_id          UUID PRIMARY KEY REFERENCES npc(id) ON DELETE CASCADE,
    tile_id         VARCHAR(64) NOT NULL,
    x               REAL NOT NULL DEFAULT 0,
    y               REAL NOT NULL DEFAULT 0,
    heading         REAL NOT NULL DEFAULT 0,
    lod_level       SMALLINT NOT NULL DEFAULT 1,    -- 当前 LOD
    current_state   VARCHAR(32) NOT NULL DEFAULT 'idle',  -- idle/planning/executing/reflecting
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_npc_position_tile ON npc_position(tile_id);
CREATE INDEX idx_npc_position_lod ON npc_position(lod_level);

-- ============================================
-- 关系表（relationship）—— Neo4j 同步的轻量读模型
-- ============================================
CREATE TABLE IF NOT EXISTS relationship (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    from_type       VARCHAR(16) NOT NULL,    -- 'player' | 'npc'
    from_id         UUID NOT NULL,
    to_type         VARCHAR(16) NOT NULL,
    to_id           UUID NOT NULL,
    kind            VARCHAR(32) NOT NULL,    -- 'friend' | 'family' | 'rival' | 'neighbor' | ...
    score           SMALLINT NOT NULL DEFAULT 0 CHECK (score BETWEEN -100 AND 100),
    last_interaction TIMESTAMPTZ,
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (from_type, from_id, to_type, to_id, kind)
);

CREATE INDEX idx_relationship_from ON relationship(from_type, from_id);
CREATE INDEX idx_relationship_to ON relationship(to_type, to_id);

-- ============================================
-- 情节记忆表（episodic_memory）
-- ============================================
CREATE TABLE IF NOT EXISTS episodic_memory (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    agent_id        VARCHAR(128) NOT NULL,        -- NPC 的 agent_id
    player_id        TEXT,
    content         TEXT NOT NULL,
    importance      SMALLINT NOT NULL DEFAULT 3 CHECK (importance BETWEEN 1 AND 5),
    expires_at      TIMESTAMPTZ,                  -- 默认 30d
    metadata        JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_memory_agent ON episodic_memory(agent_id, created_at DESC);
CREATE INDEX idx_memory_player ON episodic_memory(player_id, created_at DESC);
CREATE INDEX idx_memory_importance ON episodic_memory(importance DESC) WHERE importance >= 4;

-- 30 天后自动归档
CREATE INDEX idx_memory_expires ON episodic_memory(expires_at) WHERE expires_at IS NOT NULL;

-- ============================================
-- Saga 定义表（saga_def）
-- ============================================
CREATE TABLE IF NOT EXISTS saga_def (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(128) UNIQUE NOT NULL,
    version         VARCHAR(16) NOT NULL DEFAULT '1.0',
    source          TEXT NOT NULL,                       -- DSL 源码或 JSON
    compiled        JSONB NOT NULL,                      -- 编译后的 step 列表
    trigger         JSONB NOT NULL DEFAULT '{}',
    compensations   JSONB NOT NULL DEFAULT '{}',
    hooks           JSONB NOT NULL DEFAULT '{}',
    tags            TEXT[] NOT NULL DEFAULT '{}',
    author          VARCHAR(64),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    enabled         BOOLEAN NOT NULL DEFAULT TRUE
);

CREATE INDEX idx_saga_def_name ON saga_def(name);
CREATE INDEX idx_saga_def_tags ON saga_def USING GIN(tags);

-- ============================================
-- Saga 实例表（saga_instance）
-- ============================================
CREATE TABLE IF NOT EXISTS saga_instance (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    saga_def_id     UUID NOT NULL REFERENCES saga_def(id),
    player_id       UUID REFERENCES player(id),
    state           VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending/running/compensating/completed/failed
    trace_id        UUID NOT NULL,
    context         JSONB NOT NULL DEFAULT '{}',
    completed_steps TEXT[] NOT NULL DEFAULT '{}',
    compensations   TEXT[] NOT NULL DEFAULT '{}',
    error           TEXT,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at     TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saga_instance_state ON saga_instance(state);
CREATE INDEX idx_saga_instance_player ON saga_instance(player_id);
CREATE INDEX idx_saga_instance_trace ON saga_instance(trace_id);
CREATE INDEX idx_saga_instance_started ON saga_instance(started_at DESC);

-- ============================================
-- 决策日志表（decision_log）—— 单行 JSON 持久化
-- ============================================
CREATE TABLE IF NOT EXISTS decision_log (
    id              BIGSERIAL PRIMARY KEY,
    ts_ms           BIGINT NOT NULL,
    trace_id        UUID NOT NULL,
    service         VARCHAR(64) NOT NULL,
    agent_id        VARCHAR(128),
    player_id       UUID,
    type            VARCHAR(32) NOT NULL,                -- perception/planning/execution/reflection/error
    payload         JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_decision_trace ON decision_log(trace_id);
CREATE INDEX idx_decision_agent ON decision_log(agent_id, created_at DESC);
CREATE INDEX idx_decision_player ON decision_log(player_id, created_at DESC);
CREATE INDEX idx_decision_type ON decision_log(type, created_at DESC);
CREATE INDEX idx_decision_created ON decision_log(created_at DESC);

-- 按天分区（最近 30 天常驻，更老的归档）
-- 注：完整分区表见 docs/03-数据Schema.md §17.7

-- ============================================
-- Saga 事件表（saga_event）
-- ============================================
CREATE TABLE IF NOT EXISTS saga_event (
    id              BIGSERIAL PRIMARY KEY,
    ts_ms           BIGINT NOT NULL,
    saga_id         UUID NOT NULL,
    trace_id        UUID NOT NULL,
    event_type      VARCHAR(32) NOT NULL,                -- started/step_ok/step_failed/compensated/completed/failed
    step_name       VARCHAR(128),
    error           TEXT,
    duration_ms     BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_saga_event_saga ON saga_event(saga_id, created_at);
CREATE INDEX idx_saga_event_trace ON saga_event(trace_id);

-- ============================================
-- 触发器：updated_at 自动更新
-- ============================================
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER player_updated_at BEFORE UPDATE ON player
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER npc_updated_at BEFORE UPDATE ON npc
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER saga_def_updated_at BEFORE UPDATE ON saga_def
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

CREATE TRIGGER saga_instance_updated_at BEFORE UPDATE ON saga_instance
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ============================================
-- 种子数据：开发用玩家
-- ============================================
INSERT INTO player (username, email, password_hash, display_name)
VALUES (
    'demo',
    'demo@aicity.dev',
    crypt('demo123', gen_salt('bf', 10)),
    'Demo Player'
) ON CONFLICT (username) DO NOTHING;

INSERT INTO player (username, email, password_hash, display_name)
VALUES (
    'admin',
    'admin@aicity.dev',
    crypt('admin123', gen_salt('bf', 10)),
    'Admin'
) ON CONFLICT (username) DO NOTHING;

-- ============================================
-- 注释
-- ============================================
COMMENT ON TABLE player IS '玩家账号';
COMMENT ON TABLE npc IS 'NPC 模板（行为树 + LLM hints）';
COMMENT ON TABLE player_position IS '玩家位置（高频更新）';
COMMENT ON TABLE npc_position IS 'NPC 实时位置 + LOD';
COMMENT ON TABLE relationship IS '关系边（Neo4j 同步）';
COMMENT ON TABLE episodic_memory IS '情节记忆（30d 内）';
COMMENT ON TABLE saga_def IS 'Saga 定义';
COMMENT ON TABLE saga_instance IS 'Saga 运行实例';
COMMENT ON TABLE decision_log IS '决策日志（单行 JSON）';
COMMENT ON TABLE saga_event IS 'Saga 生命周期事件';

-- 记录 schema 版本
CREATE TABLE IF NOT EXISTS schema_version (
    version VARCHAR(16) PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_version (version) VALUES ('2.3.0') ON CONFLICT DO NOTHING;