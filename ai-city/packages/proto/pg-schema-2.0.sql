-- 2.0 跨城联邦路由表
CREATE TABLE IF NOT EXISTS a2a_npc_routes (
  npc_id TEXT PRIMARY KEY,
  city TEXT NOT NULL,
  world_engine_addr TEXT NOT NULL,
  grpc_cert_fingerprint TEXT NOT NULL,
  enabled BOOLEAN DEFAULT TRUE,
  last_heartbeat_ms BIGINT,
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_npc_routes_city ON a2a_npc_routes(city);

-- NPC 心跳表（每 30s 上报）
CREATE TABLE IF NOT EXISTS a2a_npc_heartbeats (
  npc_id TEXT PRIMARY KEY REFERENCES a2a_npc_routes(npc_id),
  last_seen_ms BIGINT NOT NULL,
  cpu_percent REAL,
  memory_mb INTEGER
);

-- 短时记忆 player_session 表（PG fallback）
CREATE TABLE IF NOT EXISTS memory_player_session (
  id SERIAL PRIMARY KEY,
  npc_id TEXT NOT NULL,
  player_id TEXT NOT NULL,
  message JSONB NOT NULL,
  embedding REAL[],  -- 1024 维
  created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_player_session_npc_player
  ON memory_player_session(npc_id, player_id, created_at DESC);