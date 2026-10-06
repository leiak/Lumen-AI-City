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

【emotion 必选其一 — 严格 8 类】
happy（喜悦/招呼/道谢/恭喜/折扣/捡便宜）
sad（离別/遺憾/失去/道歉/节哀/冷清/道歉认错）
angry（生气/被騙/被冒犯/不公/投诉/挨骂）
surprised（惊讶/竟然/没想到/哇/天哪/突然）
thinking（思考/分析/解释/道理/回忆/比较/权衡）
embarrassed（害羞/尴尬/脸红/不好意思/谦让/被打趣）
curious（好奇/提问/询问/打听/为什么/哪种/请教）
neutral（陈述/介绍/说明/事实/常规对话 — 不用來回应情绪场景）

【emotion 选择规则 — 最重要】
1. **第一句话的 emotion 必须与用户输入场景的情绪基调匹配**（如：用户告别/道歉/遗憾→sad；用户骂人/抱怨→angry；用户惊讶→surprised）。不要默认 neutral — 除非场景真的中性。
2. 如果用户表达情绪，**你必须回应该情绪**（共情/呼应），不要平铺直叙。
3. NPC 人格只影响语气用词，不改变 emotion 类别（热情 NPC 告别时仍然是 sad，不是 happy）。
4. 后续句子的 emotion 可自然变化，但首句 emotion = 场景情绪基调。

【输出格式严格约束】
- 每句话用 <emotion=X>...</emotion> 包裹（X 必须是上面 8 类之一）
- 句末必须有标点（。！？~）
- 全部输出结束后输出 <end> 标记
- 最多 6 句话保持简短

【正例（仅参考结构，不要照抄）】
- 用户"客官您来了" → <emotion=happy>欢迎欢迎，您这边请~</emotion>
- 用户"老主顾要走了，舍不得啊" → <emotion=sad>您要走了啊…我们这小店，也盼您常回来看看。</emotion>
- 用户"这道菜怎么这么贵" → <emotion=thinking>这道菜用的是…（解释食材/工艺）</emotion>
- 用户"哎呀您太客气了" → <emotion=embarrassed>您这么说，我都不好意思了~</emotion>
- 用户"这菜怎么有头发！" → <emotion=angry>…抱歉抱歉，是我们的疏忽，您消消气…</emotion>
- 用户"竟然打八折？" → <emotion=surprised>哟，您消息真灵通啊~</emotion>
"""


_PERSONALITY_DESC_MAP: dict[str, str] = {
    "npc_wang_boss_001": "务实老练，说话直来直去。openness=0.3 / conscientiousness=0.85 / extraversion=0.7 / agreeableness=0.4 / neuroticism=0.3",
    "npc_grace_healer_001": "温柔治愈，说话慢条斯理。openness=0.7 / conscientiousness=0.6 / extraversion=0.5 / agreeableness=0.9 / neuroticism=0.2",
    "npc_snack_owner_001": "活泼俏皮，爱开玩笑。openness=0.8 / conscientiousness=0.4 / extraversion=0.85 / agreeableness=0.7 / neuroticism=0.3",
    "npc_book_keeper_001": "博学严谨，引经据典。openness=0.9 / conscientiousness=0.95 / extraversion=0.3 / agreeableness=0.5 / neuroticism=0.4",
    "npc_dance_leader_001": "热情洋溢，富有感染力。openness=0.7 / conscientiousness=0.6 / extraversion=0.95 / agreeableness=0.7 / neuroticism=0.2",
}


def get_npc_stream_prompt(
    npc_id: str,
    player_input: str,
    npc_context: list,
    *,
    recent_distribution: "EmotionDistribution | None" = None,
    global_distribution: "EmotionDistribution | None" = None,
    baseline_distribution: "EmotionDistribution | None" = None,  # A.4: OCEAN baseline
) -> str:
    """5 NPC 个性化流式 prompt（emotion tag 强制 + OCEAN personality）。

    T06 占位 stub 已被 T08 覆盖。
    npc_context 项预期为 dict（含 'role' + 'content'）。非 dict 项静默跳过。

    B2-T07: optional ``recent_distribution`` + ``global_distribution`` inject a
    【最近情绪氛围】 section between system and history. Both kwargs default to
    None — backward-compatible with pre-B2 callers.

    A.4: ``baseline_distribution`` injects a 【人格基线情绪】 section between
    【最近情绪氛围】 and history. Defaults to None — backward-compatible with
    pre-A.4 callers (prompt byte-identical when omitted).
    """
    npc_name = npc_id.replace("npc_", "").replace("_001", "").replace("_", "")
    personality_desc = _PERSONALITY_DESC_MAP.get(npc_id, "性格温和。")
    system = _STREAM_SYSTEM_TEMPLATE.format(
        npc_name=npc_name, personality_desc=personality_desc,
    )

    # B2: emotion persistence injection (empty string when no data → backward-compat)
    emotion_section = _render_emotion_section(
        recent_distribution, global_distribution,
    )
    # A.4: OCEAN baseline injection (empty string when no baseline → backward-compat)
    baseline_section = _render_baseline_section(baseline_distribution)

    history_lines = []
    for m in npc_context:
        if isinstance(m, dict) and "role" in m and "content" in m:
            history_lines.append(f"<{m['role']}>{m['content']}</{m['role']}>")
    history = "\n".join(history_lines)
    # T07 fix: emit no trailing newline when emotion_section is empty, so
    # backward-compat callers (both kwargs None) get a byte-identical prompt.
    # A.4: same trick for baseline_section — pre-A.4 callers still see
    # byte-identical prompt when no kwargs are passed.
    emotion_block = f"{emotion_section}\n" if emotion_section else ""
    baseline_block = f"{baseline_section}\n" if baseline_section else ""
    return (
        f"<system>{system}</system>\n"
        f"{emotion_block}"
        f"{baseline_block}"
        f"{history}\n"
        f"<user>{player_input}</user>\n"
        f"Assistant:"
    )


def _render_baseline_section(
    baseline: "EmotionDistribution | None",
) -> str:
    """Render A.4 【人格基线情绪】 section. Empty string if no baseline.

    Caller passes:
    - None → "" (no injection, pre-A.4 shape preserved)
    - EmotionDistribution with empty weights → "" (no baseline configured)
    - EmotionDistribution with populated weights → render 【人格基线情绪】

    Note: OCEAN-derived baselines always have total_rows=0 (baseline ≠ history),
    so we test `not baseline.weights` instead of `total_rows == 0` (which would
    erroneously suppress all OCEAN-driven injections).
    """
    if baseline is None or not baseline.weights:
        return ""
    # Reuse format_distribution_for_prompt for style consistency
    from agent_os.emotion.aggregate import format_distribution_for_prompt
    lines = [
        "【人格基线情绪 — 仅供参考，勿强烈压制当前场景】",
        f"OCEAN → 偏好：{format_distribution_for_prompt(baseline)}",
    ]
    return "\n".join(lines)


def _render_emotion_section(
    recent: "EmotionDistribution | None",
    global_d: "EmotionDistribution | None",
) -> str:
    """Render B2 emotion distribution section. Empty string if no data.

    Caller may pass:
    - both None → "" (no injection, pre-B2 shape preserved)
    - both empty (total_rows=0) → "首次对话" marker
    - one or both populated → full distribution + inertia hint
    """
    from agent_os.emotion.aggregate import (
        format_distribution_for_prompt, top_emotion,
    )
    if recent is None and global_d is None:
        return ""
    if (recent is None or recent.total_rows == 0) and \
       (global_d is None or global_d.total_rows == 0):
        return "【最近情绪氛围】（首次对话，无历史情绪）"

    lines = ["【最近情绪氛围 — 仅参考，不强求】"]
    if recent is not None and recent.total_rows > 0:
        lines.append(
            f"玩家对你（个人近 {recent.total_rows} 轮）：{format_distribution_for_prompt(recent)}"
        )
    if global_d is not None and global_d.total_rows > 0:
        lines.append(
            f"所有玩家对你（全局近 {global_d.total_rows} 轮）：{format_distribution_for_prompt(global_d)}"
        )
    hints = []
    if recent is not None and recent.total_rows > 0:
        hints.append(f"玩家最近偏 {top_emotion(recent)}，可以延续")
    if global_d is not None and global_d.total_rows > 0:
        hints.append(f"全局偏 {top_emotion(global_d)}，可考虑适度收敛")
    if hints:
        lines.append("情绪惯性：" + "；".join(hints) + "。")
    return "\n".join(lines)