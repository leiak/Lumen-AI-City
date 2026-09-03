"""Saga DSL 编译器：AST → Saga 执行计划。"""
from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class CompiledStep:
    name: str
    action: str
    args: dict
    timeout_ms: int = 30000
    retries: int = 3
    on_success: str | None = None
    on_failure: str | None = None
    compensation: "CompiledStep | None" = None


@dataclass
class CompiledSaga:
    name: str
    meta: dict = field(default_factory=dict)
    trigger: dict = field(default_factory=dict)
    steps: list[CompiledStep] = field(default_factory=list)
    compensations: dict[str, CompiledStep] = field(default_factory=dict)
    hooks: dict[str, list] = field(default_factory=dict)


def compile_saga(ast) -> CompiledSaga:
    """从 Lark AST 编译为 CompiledSaga。

    完整实现见 docs/13-Saga-DSL-RFC.md §5.2。
    """
    # TODO: 完整遍历 AST
    return CompiledSaga(name="stub", trigger={}, steps=[])
