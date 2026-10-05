#!/usr/bin/env python3
"""生成 1000 条标注集（5 NPC × 200 query）"""
import json
import random

NPCS = [
    ("npc_a_wang_boss", ["红烧肉", "酒馆", "菜单", "价格", "推荐"]),
    ("npc_a_snack_owner", ["小吃", "价格", "营业时间", "推荐"]),
    ("npc_a_book_keeper", ["书", "推荐", "作者", "文学"]),
    ("npc_b_grace_healer", ["健康", "药品", "失眠", "感冒"]),
    ("npc_b_dance_leader", ["广场舞", "活动", "时间", "报名"]),
]

random.seed(42)
items = []
for npc_id, keywords in NPCS:
    for i in range(200):
        kw = random.choice(keywords)
        items.append({
            "npc_id": npc_id,
            "player_id": f"player_{i % 50}",
            "query": f"关于 {kw} 的问题 {i}",
            "expected_messages": [f"历史对话提到 {kw} 的内容 {i}"],
        })

with open("data/milvus-eval-set.jsonl", "w") as f:
    for item in items:
        f.write(json.dumps(item, ensure_ascii=False) + "\n")
print(f"generated {len(items)} eval items")