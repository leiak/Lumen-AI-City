# 数据模型 Schema（详细 DDL）

[← 返回目录](00-目录.md) | [← 02-NPC人设与剧本.md](02-NPC人设与剧本.md)

> 本文档对应原文档 §17：PostgreSQL / Neo4j / Milvus 三库详细 DDL，所有表结构可直接建表使用。

---

## 17. 数据模型 Schema（详细 DDL）

### 17.1 PostgreSQL DDL

```sql
-- 用户表
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           VARCHAR(255) UNIQUE NOT NULL,
    display_name    VARCHAR(64) NOT NULL,
    tier            VARCHAR(16) NOT NULL DEFAULT 'free', -- free | pro | enterprise
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    last_login_at   TIMESTAMPTZ,
    metadata        JSONB DEFAULT '{}'::JSONB
);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_tier ON users(tier);

-- Agent 主表
CREATE TABLE agents (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id       UUID REFERENCES users(id) ON DELETE CASCADE,
    npc_template_id     VARCHAR(64),  -- 如果是 NPC，引用模板
    display_name        VARCHAR(64) NOT NULL,
    avatar_url          TEXT,
    voice_id            VARCHAR(64),
    persona             JSONB NOT NULL,  -- 性格、人设、背景
    home_tile_id        BIGINT,
    current_tile_id     BIGINT,
    status              VARCHAR(16) DEFAULT 'active', -- active | sleeping | dead | banned
    is_external         BOOLEAN DEFAULT FALSE,  -- 是否第三方接入
    external_endpoint   TEXT,  -- 第三方 Agent 的回调 URL
    external_auth_token TEXT,  -- 加密存储
    reputation          INTEGER DEFAULT 50,  -- 0-100
    gray_score          INTEGER DEFAULT 0,   -- 灰色值 0-100
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    last_active_at      TIMESTAMPTZ,
    metadata            JSONB DEFAULT '{}'::JSONB
);
CREATE INDEX idx_agents_owner ON agents(owner_user_id);
CREATE INDEX idx_agents_current_tile ON agents(current_tile_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_external ON agents(is_external) WHERE is_external = TRUE;

-- 地图分块
CREATE TABLE tiles (
    id              BIGSERIAL PRIMARY KEY,
    geo             GEOGRAPHY(POINT, 4326) NOT NULL,
    tile_type       VARCHAR(32) NOT NULL, -- road | building | park | commercial | residential
    region_id       UUID,
    owner_agent_id  UUID REFERENCES agents(id),
    is_public       BOOLEAN DEFAULT TRUE,
    capacity        INTEGER DEFAULT 0,
    poi_data        JSONB DEFAULT '{}'::JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_tiles_geo ON tiles USING GIST(geo);
CREATE INDEX idx_tiles_region ON tiles(region_id);
CREATE INDEX idx_tiles_type ON tiles(tile_type);

-- 区域
CREATE TABLE regions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(64) NOT NULL,
    bounds      GEOGRAPHY(POLYGON, 4326) NOT NULL,
    rules       JSONB DEFAULT '{}'::JSONB,
    parent_id   UUID REFERENCES regions(id),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- 消息
CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id   UUID REFERENCES agents(id),
    to_agent_id     UUID REFERENCES agents(id),  -- 单聊；群聊用 group_id
    to_group_id     UUID,
    msg_type        VARCHAR(16) NOT NULL, -- chat | invitation | trade | alert | system
    geo_tag_id      BIGINT REFERENCES tiles(id),
    emotion         JSONB,
    content         TEXT NOT NULL,
    mentions        UUID[] DEFAULT '{}',
    attachments     JSONB DEFAULT '[]'::JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_messages_from ON messages(from_agent_id, created_at DESC);
CREATE INDEX idx_messages_to ON messages(to_agent_id, created_at DESC);
CREATE INDEX idx_messages_group ON messages(to_group_id, created_at DESC);
CREATE INDEX idx_messages_geo ON messages(geo_tag_id);

-- 物品/资产
CREATE TABLE items (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(64) NOT NULL,
    item_type       VARCHAR(32) NOT NULL, -- consumable | equipment | decor | currency
    owner_agent_id  UUID REFERENCES agents(id),
    location_tile_id BIGINT REFERENCES tiles(id),
    quantity        INTEGER DEFAULT 1,
    properties      JSONB DEFAULT '{}'::JSONB,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_items_owner ON items(owner_agent_id);
CREATE INDEX idx_items_location ON items(location_tile_id);

-- 技能
CREATE TABLE skills (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID REFERENCES agents(id) ON DELETE CASCADE,
    skill_name      VARCHAR(64) NOT NULL,
    level           INTEGER DEFAULT 1,  -- 1-10
    experience      INTEGER DEFAULT 0,
    learned_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(agent_id, skill_name)
);
CREATE INDEX idx_skills_agent ON skills(agent_id);

-- 事件
CREATE TABLE events (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type      VARCHAR(32) NOT NULL,
    payload         JSONB NOT NULL,
    region_id       UUID REFERENCES regions(id),
    start_at        TIMESTAMPTZ NOT NULL,
    end_at          TIMESTAMPTZ,
    status          VARCHAR(16) DEFAULT 'scheduled' -- scheduled | active | ended | cancelled
);
CREATE INDEX idx_events_start ON events(start_at);
CREATE INDEX idx_events_status ON events(status);

-- 交易/账本
CREATE TABLE ledger (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_agent_id   UUID REFERENCES agents(id),
    to_agent_id     UUID REFERENCES agents(id),
    amount          INTEGER NOT NULL,
    currency        VARCHAR(16) DEFAULT 'credit',
    reason          VARCHAR(128),
    tx_hash         VARCHAR(64),  -- 链上哈希（若启用区块链）
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_ledger_from ON ledger(from_agent_id, created_at DESC);
CREATE INDEX idx_ledger_to ON ledger(to_agent_id, created_at DESC);

-- 好感度（冗余在 Neo4j，但保留热数据在 PG）
CREATE TABLE affinity_cache (
    agent_a_id      UUID REFERENCES agents(id),
    agent_b_id      UUID REFERENCES agents(id),
    score           INTEGER DEFAULT 0,  -- 0-100
    relationship_level VARCHAR(16) DEFAULT 'stranger', -- stranger | familiar | friendly | intimate | soulmate
    interaction_count INTEGER DEFAULT 0,
    last_interaction_at TIMESTAMPTZ,
    PRIMARY KEY (agent_a_id, agent_b_id)
);
```

### 17.2 Neo4j 关系图 Schema

```cypher
// 节点
CREATE CONSTRAINT agent_id IF NOT EXISTS FOR (a:Agent) REQUIRE a.id IS UNIQUE;
CREATE CONSTRAINT region_id IF NOT EXISTS FOR (r:Region) REQUIRE r.id IS UNIQUE;
CREATE CONSTRAINT event_id IF NOT EXISTS FOR (e:Event) REQUIRE e.id IS UNIQUE;
CREATE CONSTRAINT item_id IF NOT EXISTS FOR (i:Item) REQUIRE i.id IS UNIQUE;

// 关系
CREATE RELATIONSHIP TYPE KNOWS FROM (a:Agent) TO (b:Agent);
CREATE RELATIONSHIP TYPE FRIEND_OF FROM (a:Agent) TO (b:Agent);
CREATE RELATIONSHIP TYPE FAMILY FROM (a:Agent) TO (b:Agent);
CREATE RELATIONSHIP TYPE ENEMY FROM (a:Agent) TO (b:Agent);
CREATE RELATIONSHIP TYPE WORKS_AT FROM (a:Agent) TO (r:Region);
CREATE RELATIONSHIP TYPE OWNS FROM (a:Agent) TO (i:Item);
CREATE RELATIONSHIP TYPE LIVES_IN FROM (a:Agent) TO (r:Region);
CREATE RELATIONSHIP TYPE MARRIED_TO FROM (a:Agent) TO (b:Agent);
CREATE RELATIONSHIP TYPE PARTICIPATED_IN FROM (a:Agent) TO (e:Event);
CREATE RELATIONSHIP TYPE MEMBER_OF FROM (a:Agent) TO (g:Group);

// 示例
CREATE (alice:Agent {id: '...', name: 'Alice'})-[:FRIEND_OF {score: 75, since: '2026-01-01'}]->(bob:Agent {id: '...', name: 'Bob'});
```

### 17.3 Milvus 向量集合 Schema

```
Collection: agent_memories
- id: VARCHAR(64) (PK)
- agent_id: VARCHAR(64) (indexed)
- content: VARCHAR(2048)
- embedding: FLOAT_VECTOR(1536)
- importance: FLOAT (0-1)
- emotion_vec: FLOAT_VECTOR(6)  // 六维情感
- created_at: INT64
- geo_tag_id: INT64

Collection: city_knowledge
- id: VARCHAR(64) (PK)
- content: VARCHAR(4096)
- embedding: FLOAT_VECTOR(1536)
- source: VARCHAR(64)
- tags: VARCHAR(256)
```

---

## 附：第三部分新增的 Schema（§25）

> 在第三部分"图数据库异步化重构"中，我们扩充了关系存储结构（详见 [08-架构优化v1.md](08-架构优化v1.md)）：

```sql
-- 关系主表（高频读写，PG JSONB 灵活扩展）
CREATE TABLE relationships (
    agent_a_id       UUID NOT NULL,
    agent_b_id       UUID NOT NULL,
    rel_type         VARCHAR(16) NOT NULL,  -- friend | family | enemy | lover | colleague | neutral
    scores           JSONB NOT NULL DEFAULT '{}',  -- {"intimacy": 75, "trust": 80, "familiarity": 60, "last_meeting_days": 3}
    interaction_count INTEGER DEFAULT 0,
    last_interaction_at TIMESTAMPTZ,
    first_met_at     TIMESTAMPTZ,
    history          JSONB DEFAULT '[]'::JSONB,
    source           VARCHAR(16) DEFAULT 'direct',  -- direct | reported | inferred
    trust_level      FLOAT DEFAULT 1.0,  -- 0-1
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    updated_at       TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (agent_a_id, agent_b_id),
    CHECK (agent_a_id < agent_b_id)
);

-- 关系变更事件（CDC 来源）
CREATE TABLE relationship_events (
    id               BIGSERIAL PRIMARY KEY,
    agent_a_id       UUID,
    agent_b_id       UUID,
    event_type       VARCHAR(16) NOT NULL,
    delta            JSONB NOT NULL,
    source_event_id  BIGINT,
    created_at       TIMESTAMPTZ DEFAULT NOW()
);
```

## 附：第五部分新增的 Schema（§45）

> 在第五部分"记忆检索不精准"中，我们推荐极简字段（详见 [10-低成本规则.md](10-低成本规则.md)）：

```sql
-- 极简记忆表（替换或并存于原有 memories 表）
CREATE TABLE agent_memories_simple (
    id              BIGSERIAL PRIMARY KEY,
    agent_id        UUID NOT NULL,
    content         TEXT NOT NULL,
    importance      SMALLINT NOT NULL DEFAULT 1,  -- 1-5
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_mem_simple_agent_created  ON agent_memories_simple(agent_id, created_at DESC);
CREATE INDEX idx_mem_simple_agent_import   ON agent_memories_simple(agent_id, importance DESC);
```

---

[← 返回目录](00-目录.md) | [← 02-NPC人设与剧本.md](02-NPC人设与剧本.md) | [继续阅读：04-API设计.md →](04-API设计.md)