"""规划模块：行为树 + LLM 升级。

详细设计见 docs/05-Agent-OS.md §19.10 + docs/11-技术细节与玩法模式.md §B.3。
"""
from __future__ import annotations

from agent_os.loop import Decision, Percept


class BehaviorTree:
    """行为树运行时（占位）。

    7 类节点：Sequence / Selector / Condition / Action / Decorator / SubTree / LLM。
    完整设计见 docs/12-BT编辑器PRD.md。
    """

    def __init__(self, root: dict | None = None) -> None:
        self.root = root or {"type": "Sequence", "children": []}

    def tick(self, percept: Percept) -> Decision | None:
        """执行一次行为树。

        返回 None 表示需要 LLM 兜底。
        """
        return self._tick_node(self.root, percept)

    def _tick_node(self, node: dict, percept: Percept) -> Decision | None:
        ntype = node.get("type")
        if ntype == "Sequence":
            for child in node.get("children", []):
                result = self._tick_node(child, percept)
                if result:
                    return result
        elif ntype == "Selector":
            for child in node.get("children", []):
                result = self._tick_node(child, percept)
                if result:
                    return result
        # TODO: 完整实现
        return None


class PlanningPipeline:
    """规划管道：先行为树 → 无匹配则 LLM 升级。"""

    def __init__(self, bt: BehaviorTree, llm_upgrade_threshold: int = 7) -> None:
        self.bt = bt
        self.llm_upgrade_threshold = llm_upgrade_threshold

    def plan(self, percepts: list[Percept]) -> Decision | None:
        """合并多个 percept，规划一次决策。"""
        if not percepts:
            return None

        # 1. 行为树先尝试
        for p in percepts:
            action = self.bt.tick(p)
            if action:
                # 玩家互动 → LLM 升级
                if any(p.kind == "player_dialogue" for p in percepts):
                    return None  # 调用方会升级到 LLM
                return action
        return None
