-- Emotion persistence: denormalize emotion tag from JSONB message for fast aggregation.
ALTER TABLE memory_player_session ADD COLUMN IF NOT EXISTS emotion TEXT;

-- Index for the aggregation hot path:
--   SELECT emotion, created_at FROM memory_player_session
--   WHERE npc_id=? AND player_id=? ORDER BY created_at DESC LIMIT 50
CREATE INDEX IF NOT EXISTS idx_memory_player_session_emotion_recent
  ON memory_player_session(npc_id, player_id, created_at DESC, emotion);