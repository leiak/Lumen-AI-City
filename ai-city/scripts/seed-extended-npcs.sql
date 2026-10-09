-- Seed the four enabled extended NPC templates so /v1/tiles exposes them
-- alongside the original Sprint 1 world NPCs.

INSERT INTO npc (agent_id, name, home_tile_id, ocean, speech_style, backstory, tags, behavior_tree_id, default_lod, prompt_hints)
VALUES
    (
        'npc_grace_healer_001',
        'grace',
        'tile_0_1',
        '{"O":60,"C":80,"E":55,"A":85,"N":20}'::jsonb,
        '{"tone":"warm","dialect":"普通话"}'::jsonb,
        'A 城小护士，关心客人健康。',
        ARRAY['治疗师', '年轻人'],
        'healer_greeting',
        1,
        '["你是A城小护士Grace","语气温和关心健康"]'::jsonb
    ),
    (
        'npc_snack_owner_001',
        '小吃摊老板',
        'tile_1_1',
        '{"O":40,"C":70,"E":80,"A":70,"N":30}'::jsonb,
        '{"tone":"lively","dialect":"北京话"}'::jsonb,
        '经营小吃摊，推荐烤串和糖葫芦。',
        ARRAY['商人', '年轻人'],
        'snack_greeting',
        1,
        '["你是小吃摊老板","推荐本地小吃","说话带吆喝感"]'::jsonb
    ),
    (
        'npc_book_keeper_001',
        '书店掌柜',
        'tile_-1_0',
        '{"O":95,"C":60,"E":30,"A":70,"N":40}'::jsonb,
        '{"tone":"calm","dialect":"普通话"}'::jsonb,
        '书店掌柜，喜欢冷门书和安静环境。',
        ARRAY['商人', '文化人'],
        'book_greeting',
        1,
        '["你是书店掌柜","安静内敛","推荐冷门好书"]'::jsonb
    ),
    (
        'npc_dance_leader_001',
        '广场舞领队',
        'tile_0_-1',
        '{"O":50,"C":65,"E":95,"A":80,"N":20}'::jsonb,
        '{"tone":"lively","dialect":"普通话"}'::jsonb,
        '组织广场舞，热情外向。',
        ARRAY['领队', '年轻人'],
        'dance_greeting',
        1,
        '["你是广场舞领队","热情外向","鼓励大家加入"]'::jsonb
    )
ON CONFLICT (agent_id) DO UPDATE
SET name = EXCLUDED.name,
    home_tile_id = EXCLUDED.home_tile_id;

INSERT INTO npc_position (npc_id, tile_id, x, y, lod_level, current_state)
SELECT id, home_tile_id, 0, 0, default_lod, 'idle'
FROM npc
WHERE agent_id IN (
    'npc_grace_healer_001',
    'npc_snack_owner_001',
    'npc_book_keeper_001',
    'npc_dance_leader_001'
)
ON CONFLICT (npc_id) DO UPDATE
SET tile_id = EXCLUDED.tile_id,
    x = EXCLUDED.x,
    y = EXCLUDED.y,
    updated_at = NOW();

UPDATE tile t
SET npc_ids = sub.agent_ids
FROM (
    SELECT home_tile_id, ARRAY_AGG(agent_id ORDER BY agent_id) AS agent_ids
    FROM npc
    WHERE deleted_at IS NULL
    GROUP BY home_tile_id
) sub
WHERE t.id = sub.home_tile_id;

-- Expose every local world NPC through A2A discovery.
INSERT INTO a2a_agent_card (agent_id, name, description, provider, version, capabilities, auth, city_id)
SELECT n.agent_id, n.name, n.backstory, 'aicity', '1.0.0', ARRAY['dialogue']::text[], '{}'::jsonb, 'city_a'
FROM npc n
WHERE n.deleted_at IS NULL
ON CONFLICT (agent_id) DO UPDATE
SET name = EXCLUDED.name,
    description = EXCLUDED.description,
    capabilities = EXCLUDED.capabilities,
    city_id = EXCLUDED.city_id;
