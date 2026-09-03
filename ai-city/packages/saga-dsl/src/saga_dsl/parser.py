"""Saga DSL 解析器（Lark-based）。"""
from __future__ import annotations

from lark import Lark, Transformer

# Lark 格式的 grammar（从 grammar.ebnf 转换）
LARK_GRAMMAR = r"""
start: saga_header statement_list

saga_header: "saga" CNAME "{" meta_entry* "}"
meta_entry: CNAME ":" STRING

statement: trigger_stmt | step_stmt | compensation_stmt | hook_stmt

trigger_stmt: "trigger" "{" trigger_field "}"
trigger_field: "on" STRING -> trigger_on
             | "when" expression -> trigger_when

step_stmt: "step" CNAME "{" step_field* "}"
step_field: "do" ":" CNAME "(" [arg_list] ")" -> step_do
          | "args" ":" object_literal -> step_args
          | "timeout" ":" DURATION -> step_timeout
          | "retries" ":" INT -> step_retries
          | "on_success" ":" CNAME -> step_on_success
          | "on_failure" ":" CNAME -> step_on_failure

compensation_stmt: "compensation" "for" CNAME "{" step_field* "}"

hook_stmt: "on" ("saga_start" | "saga_complete" | "saga_fail") "{" statement* "}"

object_literal: "{" [object_pair ("," object_pair)*] "}"
object_pair: CNAME ":" expression

expression: term ((EQ | NEQ | LT | GT | LTE | GTE) term)*
term: factor ((PLUS | MINUS) factor)*
factor: atom ((MUL | DIV) atom)*
atom: NUMBER -> atom_number
    | STRING -> atom_string
    | func_call
    | CNAME -> atom_var
    | "(" expression ")"

func_call: CNAME "(" [arg_list] ")"
arg_list: expression ("," expression)*

DURATION: INT ("ms" | "s" | "m" | "h")

EQ: "=="
NEQ: "!="
LT: "<"
GT: ">"
LTE: "<="
GTE: ">="
PLUS: "+"
MINUS: "-"
MUL: "*"
DIV: "/"

%import common.INT
%import common.NUMBER
%import common.STRING
%import common.CNAME
%import common.WS
%ignore WS
"""


_parser: Lark | None = None


def _get_parser() -> Lark:
    global _parser
    if _parser is None:
        _parser = Lark(LARK_GRAMMAR, parser="earley", ambiguity="resolve")
    return _parser


def parse(source: str):
    """解析 Saga DSL 源码为 AST。"""
    return _get_parser().parse(source)
