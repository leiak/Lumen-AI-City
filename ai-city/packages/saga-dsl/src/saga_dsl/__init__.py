"""Saga DSL 核心包。

完整设计见 docs/13-Saga-DSL-RFC.md。
"""
from saga_dsl.parser import parse
from saga_dsl.compiler import compile_saga
from saga_dsl.runtime import SagaRuntime

__version__ = "0.1.0"
__all__ = ["parse", "compile_saga", "SagaRuntime"]
