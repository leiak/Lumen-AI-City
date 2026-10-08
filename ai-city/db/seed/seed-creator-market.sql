-- Phase 3.0 v3: creator-market demo identities.
-- Idempotent: safe to re-run after the player role migration.

UPDATE player
SET role = 'creator'
WHERE username = 'demo';

INSERT INTO player (username, email, password_hash, display_name, role)
VALUES (
    'creator_demo',
    'creator-demo@aicity.dev',
    crypt('creatorpass', gen_salt('bf', 10)),
    'Creator Demo',
    'creator'
)
ON CONFLICT (username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    role          = EXCLUDED.role;

INSERT INTO wallet (user_id, gold_balance, token_balance)
SELECT id, 1000, 100
FROM player
WHERE username = 'creator_demo'
ON CONFLICT (user_id) DO NOTHING;
