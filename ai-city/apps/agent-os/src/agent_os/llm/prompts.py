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


def get_npc_stream_prompt(npc_id: str, player_input: str, npc_context: list) -> str:
    """NPC 流式 prompt 模板（T08 会基于 NPC 性格填充 5 NPC 个性化 prompt）。

    当前是占位实现：构造一个最小可用的 prompt 字符串，要求 LLM 在每句末
    输出 ``<emotion=...>...</emotion>`` tag，并以 ``<end>`` 收尾。T08 接管后会
    改为基于 NPC slug 选人格化 system prompt + few-shot examples。
    """
    context_str = "\n".join(str(c) for c in npc_context)
    return (
        f"You are NPC {npc_id}. "
        f"Use <emotion=happy|sad|angry|surprised|thinking|embarrassed|curious|neutral>"
        f" tags at the end of each sentence. End with <end>.\n"
        f"Context: {context_str}\n"
        f"Player: {player_input}\n"
        f"NPC:"
    )