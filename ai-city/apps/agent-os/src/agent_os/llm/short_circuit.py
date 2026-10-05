"""短路词 — 不走 LLM，直接返回固定响应（节省成本）。

背景：玩家常见的问候/告别属于高频低价值输入，调用 LLM 浪费 token 和延迟。
本模块提供 `match_short_circuit()`，匹配则直接返回固定响应字符串，
不匹配则返回 None，调用方继续走 LLM。

匹配顺序：
1. **整句精确匹配**（case-insensitive + strip）：完全等于关键词
2. **前缀匹配**：以问候词开头（用于"你好，请问..."这种带后缀的寒暄）

设计取舍：
- 用 dict 而非 list，O(1) 查找
- 关键词只覆盖问候/告别，不覆盖业务问题（业务问题必须走 LLM）
- 返回 str | None，不用 dataclass（无结构化字段）
"""

SHORT_CIRCUIT_RESPONSES = {
    "你好": "你好呀！欢迎来到我的小店。",
    "hello": "Hello! 欢迎光临。",
    "hi": "Hi！今天想聊点什么？",
    "拜拜": "拜拜，下次再来！",
    "再见": "再见，欢迎下次光临。",
    "你是谁": "我是 NPC，有什么可以帮你的？",
}


def match_short_circuit(player_input: str) -> str | None:
    """精确匹配短路词，返回响应或 None（需要走 LLM）。

    匹配策略：
    1. 先整句匹配（normalized = strip + lower）
    2. 再对常用问候/告别关键词做前缀匹配
    """
    normalized = player_input.strip().lower()
    # 优先整句匹配
    if normalized in SHORT_CIRCUIT_RESPONSES:
        return SHORT_CIRCUIT_RESPONSES[normalized]
    # 其次匹配常用问候词开头
    for kw in ["你好", "hello", "hi", "拜拜", "再见"]:
        if normalized.startswith(kw):
            return SHORT_CIRCUIT_RESPONSES[kw]
    return None