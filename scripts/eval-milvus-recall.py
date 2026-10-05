#!/usr/bin/env python3
"""Milvus recall top-5 准确率离线评估"""
import json
import requests
from collections import defaultdict

EVAL_SET = "data/milvus-eval-set.jsonl"
MEMORY_URL = "http://localhost:9200"


def load_eval_set():
    items = []
    with open(EVAL_SET) as f:
        for line in f:
            items.append(json.loads(line))
    return items


def evaluate():
    items = load_eval_set()
    hits = defaultdict(lambda: {"correct": 0, "total": 0})
    for item in items:
        resp = requests.post(f"{MEMORY_URL}/v1/recall", json={
            "npc_id": item["npc_id"],
            "player_id": item["player_id"],
            "query": item["query"],
            "top_k": 5,
        })
        recalled = set(resp.json()["messages"])
        expected = set(item["expected_messages"])
        if expected & recalled:
            hits[item["npc_id"]]["correct"] += 1
        hits[item["npc_id"]]["total"] += 1
    print(f"{'NPC':<25} {'Recall@5':<10} {'Samples'}")
    overall_correct = overall_total = 0
    for npc_id, stat in hits.items():
        acc = stat["correct"] / stat["total"] if stat["total"] else 0
        print(f"{npc_id:<25} {acc:<10.2%} {stat['total']}")
        overall_correct += stat["correct"]
        overall_total += stat["total"]
    overall = overall_correct / overall_total if overall_total else 0
    print(f"\n{'OVERALL':<25} {overall:<10.2%} {overall_total}")
    return overall >= 0.70


if __name__ == "__main__":
    import sys
    sys.exit(0 if evaluate() else 1)