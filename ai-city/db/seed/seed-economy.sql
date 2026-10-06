-- db/seed/seed-economy.sql
-- Phase 3.0: demo 钱包种子（1000 gold + 100 token）
-- Idempotent: ON CONFLICT 覆盖

INSERT INTO wallet (user_id, gold_balance, token_balance)
SELECT id, 1000, 100 FROM player WHERE username IN ('demo', 'admin')
ON CONFLICT (user_id) DO UPDATE SET
    gold_balance = 1000, token_balance = 100;