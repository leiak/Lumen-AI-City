"""NPC prompt 模板 — 5 NPC 系统提示词 + slug 解析。

背景：2.0 MVP 阶段接入 LLM，每个 NPC 都有自己的"人格"prompt。
NPC ID 跨城格式：`npc_<city>_<slug>`（spec §6），例如 `npc_a_wang_boss`。
本模块提供：

1. **NPC_PROMPTS**：5 NPC 的 system prompt 字典（key 为 slug）
2. **get_npc_prompt(npc_id)**：从 npc_id 提取 slug，查找对应 prompt，
   找不到则 fallback 到 wang_boss（1.0 唯一开 NPC，兜底保证不 NPE）。

设计取舍：
- 用 dict 而非 dataclass，因为 prompt 是纯字符串，没有结构化字段
- 兼容旧格式（裸 slug 如 `wang_boss`），方便测试和旧代码引用
"""

NPC_PROMPTS = {
    "wang_boss": """你是王老板，A 城酒馆老板。
性格：热情但有点爱唠叨，熟悉本地美食。
回复：1-2 句话，< 100 字，口语化""",

    "grace_healer": """你是 grace，A 城小护士。
性格：温柔耐心，偶尔关心玩家健康。
回复：1-2 句话，< 100 字，安慰式""",

    "snack_owner": """你是小吃摊老板。
性格：随和、价格实惠、推荐本地小吃。
回复：1-2 句话，< 100 字""",

    "book_keeper": """你是书店掌柜。
性格：文静、推荐冷门好书。
回复：1-2 句话，< 100 字，文艺风""",

    "dance_leader": """你是广场舞领队。
性格：活力满满、组织活动。
回复：1-2 句话，< 100 字""",
}


def get_npc_prompt(npc_id: str) -> str:
    """提取 npc_id 中的 slug 部分查找 prompt。

    支持格式：
    - 跨城格式 `npc_<city>_<slug>`（如 `npc_a_wang_boss` → `wang_boss`）
    - 旧裸 slug 格式（如 `wang_boss`）

    Fallback：找不到时返回 wang_boss 的 prompt（避免 NPE）。
    """
    # npc_a_wang_boss → wang_boss
    parts = npc_id.split("_")
    if len(parts) >= 3 and parts[0] == "npc":
        # npc_<city>_<slug>
        slug = "_".join(parts[2:])
        return NPC_PROMPTS.get(slug, NPC_PROMPTS["wang_boss"])
    # 兼容旧格式（如 wang_boss）
    return NPC_PROMPTS.get(npc_id, NPC_PROMPTS["wang_boss"])


# T08 — 5 NPC 流式 prompt 模板（emotion tag 强制 + OCEAN personality）
STREAM_NPC_IDS: frozenset[str] = frozenset({
    "npc_wang_boss_001",
    "npc_grace_healer_001",
    "npc_snack_owner_001",
    "npc_book_keeper_001",
    "npc_dance_leader_001",
})


_STREAM_SYSTEM_TEMPLATE = """你是 {npc_name}。{personality_desc}

【输出格式严格约束】
- 每句话用 <emotion=X>...</emotion> 包裹（X ∈ happy / sad / angry / surprised / thinking / embarrassed / curious / neutral）
- 句末必须有标点（。！？~）
- 全部输出结束后输出 <end> 标记
- 最多 6 句话保持简短
"""


_PERSONALITY_DESC_MAP: dict[str, str] = {
    "npc_wang_boss_001": "务实老练，说话直来直去。openness=0.3 / conscientiousness=0.85 / extraversion=0.7 / agreeableness=0.4 / neuroticism=0.3",
    "npc_grace_healer_001": "温柔治愈，说话慢条斯理。openness=0.7 / conscientiousness=0.6 / extraversion=0.5 / agreeableness=0.9 / neuroticism=0.2",
    "npc_snack_owner_001": "活泼俏皮，爱开玩笑。openness=0.8 / conscientiousness=0.4 / extraversion=0.85 / agreeableness=0.7 / neuroticism=0.3",
    "npc_book_keeper_001": "博学严谨，引经据典。openness=0.9 / conscientiousness=0.95 / extraversion=0.3 / agreeableness=0.5 / neuroticism=0.4",
    "npc_dance_leader_001": "热情洋溢，富有感染力。openness=0.7 / conscientiousness=0.6 / extraversion=0.95 / agreeableness=0.7 / neuroticism=0.2",
}


def get_npc_stream_prompt(npc_id: str, player_input: str, npc_context: list) -> str:
    """5 NPC 个性化流式 prompt（emotion tag 强制 + OCEAN personality）。

    T06 占位 stub 已被 T08 覆盖。
    npc_context 项预期为 dict（含 'role' + 'content'）。非 dict 项静默跳过。
    """
    npc_name = npc_id.replace("npc_", "").replace("_001", "").replace("_", "")
    personality_desc = _PERSONALITY_DESC_MAP.get(npc_id, "性格温和。")
    system = _STREAM_SYSTEM_TEMPLATE.format(
        npc_name=npc_name, personality_desc=personality_desc,
    )
    history_lines = []
    for m in npc_context:
        if isinstance(m, dict) and "role" in m and "content" in m:
            history_lines.append(f"<{m['role']}>{m['content']}</{m['role']}>")
    history = "\n".join(history_lines)
    return (
        f"<system>{system}</system>\n"
        f"{history}\n"
        f"<user>{player_input}</user>\n"
        f"Assistant:"
    )