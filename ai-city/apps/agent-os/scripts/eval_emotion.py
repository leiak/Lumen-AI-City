#!/usr/bin/env python3
"""离线评估 LLM 流式 emotion 分类准确率。

读 data/llm-emotion-eval-set.jsonl，对每条用例：
  1. 构造 NPC 流式 system prompt（emotion tag 强制，T08）
  2. 调 LiteLLMProvider.stream() → ActionDispatcher.say_stream()
  3. SentenceSplitter 解析 <emotion=X>...</emotion>
  4. EmotionValidator 归一化（8 类 / 异常→neutral）
  5. 与 expected_emotion 比较

输出每个 emotion 类的 precision/recall/F1 + 总体准确率 + per-NPC 准确率。
低于阈值时退出码 ≠0（默认 0.70，CI 可拦截）。

Spec ref: docs/superpowers/specs/2026-10-05-2.0-stage2-stream-emotion-design.md
"""
from __future__ import annotations

import argparse
import asyncio
import json
import os
import sys
import time
import warnings
from collections import Counter, defaultdict
from pathlib import Path
from typing import Any
from unittest.mock import AsyncMock, MagicMock

# 允许 ``uv run python scripts/eval_emotion.py`` 直接执行
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "src"))

from agent_os.dispatcher import ActionDispatcher  # noqa: E402
from agent_os.llm.litellm_provider import LiteLLMProvider  # noqa: E402
from agent_os.stream.emotion_validator import ALLOWED_EMOTIONS  # noqa: E402


def _redact(s: str) -> str:
    """隐藏 env var 里的 API key（防御性 — 不打印模型 env）。"""
    if not s:
        return "<empty>"
    return f"{s[:4]}...{s[-2:]} (len={len(s)})"


def load_dataset(path: Path) -> list[dict[str, Any]]:
    """JSONL 严格解析：每行一条 JSON，跳过畸形行（warning）。"""
    items: list[dict[str, Any]] = []
    with path.open(encoding="utf-8") as f:
        for lineno, raw in enumerate(f, start=1):
            raw = raw.strip()
            if not raw:
                continue
            try:
                obj = json.loads(raw)
            except json.JSONDecodeError as exc:
                warnings.warn(f"skip malformed line {lineno}: {exc}", stacklevel=2)
                continue
            if not isinstance(obj, dict) or "npc_id" not in obj or "expected_emotion" not in obj:
                warnings.warn(f"skip invalid record at line {lineno}", stacklevel=2)
                continue
            items.append(obj)
    return items


async def _eval_one(
    dispatcher: ActionDispatcher,
    item: dict[str, Any],
    semaphore: asyncio.Semaphore,
) -> dict[str, Any]:
    """单条评估：取第一句的 emotion 与 expected 比较。"""
    async with semaphore:
        item_id = item.get("id", "?")
        npc_id = item["npc_id"]
        player_input = item["input"]
        expected = item["expected_emotion"]
        context = item.get("context") or []

        try:
            emotion_pred: str | None = None
            t0 = time.perf_counter()
            async for ev in dispatcher.say_stream(
                npc_id=npc_id,
                player_input=player_input,
                npc_context=context if isinstance(context, list) else [],
                session_id=f"sess-eval-{item_id}",
                trace_id=f"tr-eval-{item_id}",
            ):
                if ev.sentence_idx == 0 and not ev.complete:
                    emotion_pred = ev.emotion
                    # 取首句即停，节省 token（spec: 流式节拍事件）
                    break
            elapsed_ms = int((time.perf_counter() - t0) * 1000)
        except Exception as exc:  # noqa: BLE001
            return {
                "id": item_id,
                "npc_id": npc_id,
                "expected": expected,
                "predicted": None,
                "ok": False,
                "elapsed_ms": -1,
                "error": type(exc).__name__,
            }

        return {
            "id": item_id,
            "npc_id": npc_id,
            "expected": expected,
            "predicted": emotion_pred,
            "ok": emotion_pred == expected,
            "elapsed_ms": elapsed_ms,
            "error": None,
        }


def _compute_metrics(records: list[dict[str, Any]]) -> dict[str, Any]:
    """算 per-emotion precision/recall/F1 + 总体准确率 + per-NPC 准确率。

    对所有 8 类 emotion（含数据集出现 0 次的类）都填表，确保输出列对齐。
    """
    tp: Counter[str] = Counter()
    fp: Counter[str] = Counter()
    fn: Counter[str] = Counter()
    total = 0
    correct = 0
    per_npc_correct: dict[str, int] = defaultdict(int)
    per_npc_total: dict[str, int] = defaultdict(int)
    parse_fail = 0

    for rec in records:
        exp = rec["expected"]
        pred = rec["predicted"]
        per_npc_total[rec["npc_id"]] += 1
        total += 1
        if pred is None:
            parse_fail += 1
            fn[exp] += 1  # 没预测出来 → false negative
            continue
        if rec["ok"]:
            correct += 1
            per_npc_correct[rec["npc_id"]] += 1
            tp[exp] += 1
        else:
            fp[pred] += 1  # 预测了别的 → false positive（对 pred 类）
            fn[exp] += 1  # 漏掉了 expected → false negative（对 expected 类）

    # 8 类 + 总体从每类记录
    per_emotion: dict[str, dict[str, float]] = {}
    for emo in sorted(ALLOWED_EMOTIONS):
        tp_e = tp[emo]
        fp_e = fp[emo]
        fn_e = fn[emo]
        precision = tp_e / (tp_e + fp_e) if (tp_e + fp_e) else 0.0
        recall = tp_e / (tp_e + fn_e) if (tp_e + fn_e) else 0.0
        f1 = (
            2 * precision * recall / (precision + recall)
            if (precision + recall)
            else 0.0
        )
        per_emotion[emo] = {
            "precision": precision,
            "recall": recall,
            "f1": f1,
            "support": tp_e + fn_e,
        }

    macro_f1 = sum(m["f1"] for m in per_emotion.values()) / len(per_emotion)
    overall_accuracy = correct / total if total else 0.0
    per_npc_acc = {
        npc: per_npc_correct[npc] / per_npc_total[npc] for npc in sorted(per_npc_total)
    }

    return {
        "total": total,
        "correct": correct,
        "parse_fail": parse_fail,
        "overall_accuracy": overall_accuracy,
        "macro_f1": macro_f1,
        "per_emotion": per_emotion,
        "per_npc_accuracy": per_npc_acc,
    }


def _print_report(metrics: dict[str, Any], records: list[dict[str, Any]], duration_s: float) -> None:
    """人类可读报告 — spec T23 DoD。"""
    print("=" * 60)
    print("Emotion Eval Results (LLM streaming <emotion=X> classifier)")
    print("=" * 60)
    print(f"Total cases     : {metrics['total']}")
    print(f"Correct         : {metrics['correct']} / {metrics['total']} ({metrics['overall_accuracy']:.1%})")
    print(f"Parse failures  : {metrics['parse_fail']}")
    print(f"Duration        : {duration_s:.1f}s")
    print()
    print("Per-emotion precision / recall / F1:")
    print(f"  {'emotion':<12} {'prec':>6} {'rec':>6} {'f1':>6} {'support':>8}")
    for emo, m in metrics["per_emotion"].items():
        print(
            f"  {emo:<12} {m['precision']:>6.2f} {m['recall']:>6.2f} {m['f1']:>6.2f} {m['support']:>8d}"
        )
    print()
    print(f"Macro F1        : {metrics['macro_f1']:.3f}")
    print(f"Overall accuracy: {metrics['overall_accuracy']:.1%}")
    print()
    print("Per-NPC accuracy:")
    for npc, acc in metrics["per_npc_accuracy"].items():
        print(f"  {npc:<30} {acc:.1%}")

    # 失败样本示例（便于调优）
    failures = [r for r in records if not r["ok"]]
    if failures:
        print()
        print(f"First 5 failures (of {len(failures)}):")
        for rec in failures[:5]:
            err = rec["error"] or ""
            err_str = f" [{err}]" if err else ""
            print(
                f"  id={rec['id']} npc={rec['npc_id']} "
                f"expected={rec['expected']} predicted={rec['predicted']}{err_str}"
            )


async def run(args: argparse.Namespace) -> int:
    api_key_present = bool(os.getenv("ANTHROPIC_API_KEY", "").strip())
    api_key_redacted = _redact(os.getenv("ANTHROPIC_API_KEY", ""))

    print(f"model           : {args.model}")
    print(f"max_tokens      : {args.max_tokens}")
    print(f"concurrency     : {args.concurrency}")
    print(f"dataset         : {args.dataset}")
    print(f"api_key_present : {api_key_present} ({api_key_redacted})")
    print()

    if not api_key_present:
        print("ERROR: ANTHROPIC_API_KEY not set.", file=sys.stderr)
        print("  export ANTHROPIC_API_KEY=sk-ant-...  # then re-run", file=sys.stderr)
        return 2

    items = load_dataset(args.dataset)
    if not items:
        print(f"ERROR: empty dataset {args.dataset}", file=sys.stderr)
        return 2

    if args.limit > 0:
        items = items[: args.limit]
    print(f"Loaded {len(items)} cases (limit={args.limit})")
    print()

    # Publisher mock — 解析但不真正发布到 Redis
    pub = MagicMock()
    pub.publish_beat = AsyncMock()
    pub.publish_done = AsyncMock()

    provider = LiteLLMProvider(model=args.model)
    dispatcher = ActionDispatcher(llm_client=provider, publisher=pub)

    semaphore = asyncio.Semaphore(args.concurrency)
    t0 = time.perf_counter()

    records: list[dict[str, Any]] = []
    # 顺序跑（避免对 haiku 并发过大），但保留 semaphore 钩子供将来扩展
    for item in items:
        rec = await _eval_one(dispatcher, item, semaphore)
        records.append(rec)
        if args.verbose:
            mark = "OK" if rec["ok"] else "FAIL"
            print(
                f"  [{mark}] id={rec['id']} npc={rec['npc_id']} "
                f"expected={rec['expected']} predicted={rec['predicted']} "
                f"({rec['elapsed_ms']}ms)",
                file=sys.stderr,
            )

    duration_s = time.perf_counter() - t0
    metrics = _compute_metrics(records)
    _print_report(metrics, records, duration_s)

    return 0 if metrics["overall_accuracy"] >= args.threshold else 1


def _resolve_dataset(path: Path | None) -> Path:
    """默认在 ``<repo>/data/llm-emotion-eval-set.jsonl`` 查找 dataset。

    优先级：explicit > /workspace-root/data/ > cwd/data/ > agent-os/data/
    """
    candidates: list[Path] = []
    if path is not None:
        candidates.append(path)
    script_dir = Path(__file__).resolve().parent
    # script_dir = apps/agent-os/scripts → 上三级 = repo root
    repo_root = script_dir.parent.parent.parent
    candidates.append(repo_root / "data" / "llm-emotion-eval-set.jsonl")
    candidates.append(Path.cwd() / "data" / "llm-emotion-eval-set.jsonl")
    candidates.append(script_dir.parent.parent / "data" / "llm-emotion-eval-set.jsonl")
    candidates.append(script_dir.parent / "data" / "llm-emotion-eval-set.jsonl")
    for cand in candidates:
        if cand.exists():
            return cand
    # 全失败 → 返回第一个让 argparse 之后报错用
    return candidates[1] if len(candidates) > 1 else candidates[0]


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    p = argparse.ArgumentParser(description="Offline LLM emotion classifier eval")
    p.add_argument(
        "--dataset",
        type=Path,
        default=None,
        help=(
            "JSONL eval dataset path. Default: 解析 <repo>/data/llm-emotion-eval-set.jsonl "
            "(兼容 apps/agent-os/data/ 和 cwd)"
        ),
    )
    p.add_argument(
        "--limit",
        type=int,
        default=0,
        help="Limit to first N cases (0 = all, useful for smoke test, e.g. --limit 5)",
    )
    p.add_argument(
        "--model",
        default=os.getenv("EVAL_MODEL", "claude-haiku-4-5"),
        help="LLM model id (default: claude-haiku-4-5, override via EVAL_MODEL)",
    )
    p.add_argument(
        "--max-tokens",
        type=int,
        default=200,
        help="Max output tokens per LLM call (default: 200)",
    )
    p.add_argument(
        "--concurrency",
        type=int,
        default=1,
        help="Max concurrent LLM calls (default: 1 = sequential; haiku rate-limit safe)",
    )
    p.add_argument(
        "--threshold",
        type=float,
        default=0.70,
        help="Minimum overall accuracy (default: 0.70 — CI fail if below)",
    )
    p.add_argument(
        "--verbose",
        action="store_true",
        help="Print per-case result to stderr",
    )
    return p.parse_args(argv)


def main() -> None:
    args = parse_args()
    args.dataset = _resolve_dataset(args.dataset)
    try:
        rc = asyncio.run(run(args))
    except KeyboardInterrupt:
        print("\ninterrupted", file=sys.stderr)
        rc = 130
    sys.exit(rc)


if __name__ == "__main__":
    main()