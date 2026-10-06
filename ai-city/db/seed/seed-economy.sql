-- db/seed/seed-economy.sql
-- Phase 3.0: demo 钱包种子（1000 gold + 100 token）+ NPC 商品种子（W2.2）
-- Idempotent: ON CONFLICT 覆盖

INSERT INTO wallet (user_id, gold_balance, token_balance)
SELECT id, 1000, 100 FROM player WHERE username IN ('demo', 'admin')
ON CONFLICT (user_id) DO UPDATE SET
    gold_balance = 1000, token_balance = 100;

-- W2.2: 5 NPC 商品 across 4 NPCs
INSERT INTO product (npc_id, name, price_gold, price_token, stock) VALUES
    ('npc_wang_boss_001',   '招牌红烧肉', 50,  NULL, 10),
    ('npc_wang_boss_001',   '陈年花雕酒', 200, NULL, 5),
    ('npc_grace_healer_001','草药包',     30,  NULL, NULL),
    ('npc_snack_owner_001', '糖葫芦',     10,  NULL, NULL),
    ('npc_book_keeper_001', '古籍',       500, 50,  3)
ON CONFLICT (npc_id, name) DO UPDATE SET
    price_gold = EXCLUDED.price_gold,
    price_token = EXCLUDED.price_token,
    stock = EXCLUDED.stock,
    enabled = true;