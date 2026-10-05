"""Generate offline eval dataset for NPC streaming emotion tagging.

Output: data/llm-emotion-eval-set.jsonl

设计：
- 5 NPC × 8 emotion × 5 sentences = 200 条（plan T22）
- 每条覆盖一个 (npc, emotion) 对，5 个 NPC 都覆盖每个 emotion 25 次
- 字段：id / npc_id / input / expected_emotion / expected_text_pattern / context / scenario
- scenario ∈ {greeting, farewell, storytelling, refusal, agreement, surprise, thinking, embarrassed, curious, neutral}
- expected_text_pattern 用于校验流式输出首句匹配（可选，正则）
- context 默认空列表；多轮样本（10% ≈ 20 条）带 1-3 轮上下文
- 全为合成文本，无 PII

运行：
    python data/generate-llm-emotion-eval-set.py
    # 写入 data/llm-emotion-eval-set.jsonl
"""
from __future__ import annotations

import json
from pathlib import Path

# 5 NPC × 8 emotion × 5 sentences = 200
NPC_IDS = [
    "npc_wang_boss_001",      # 王老板 / A 城酒馆老板 — 务实老练
    "npc_grace_healer_001",   # grace / 小护士 — 温柔治愈
    "npc_snack_owner_001",    # 小吃摊老板 — 活泼俏皮
    "npc_book_keeper_001",    # 书店掌柜 — 博学严谨
    "npc_dance_leader_001",   # 广场舞领队 — 热情洋溢
]

EMOTIONS = [
    "happy",
    "sad",
    "angry",
    "surprised",
    "thinking",
    "embarrassed",
    "curious",
    "neutral",
]

# 8 emotion × 5 sentence × 5 NPC = 200
# 每个 NPC 拥有自己的句子库（贴合人格），但 emotion 分布完全平衡
SENTENCES: dict[str, dict[str, list[str]]] = {
    "npc_wang_boss_001": {
        "happy": [
            "客官您来了！今天推荐红烧肉",
            "行，这道菜今天给您打个八折",
            "您尝了这道菜真是太识货了",
            "恭喜恭喜，发大财啊！",
            "哎呀，您太客气了",
        ],
        "sad": [
            "老主顾要走了，舍不得啊",
            "这道菜食材用完了，挺遗憾",
            "您脸色不好，节哀顺变吧",
            "想当年这店多红火，现在冷清了",
            "这事我做的不对，向您道歉",
        ],
        "angry": [
            "这价钱已经是最低了，不行就拉倒",
            "谁砸我场子的，给我出来",
            "这食材不能省，缺德！",
            "你这种客人我见多了，恕不奉陪",
            "说好付款又反悔，没信用！",
        ],
        "surprised": [
            "什么？！您从东城连夜赶来的？",
            "哎哟，这菜里怎么有金子？",
            "嚯，今天店里突然来了这么多人！",
            "真的假的？传说中那位大厨要来？",
            "哎呀妈呀，您手里这是什么稀罕物件？",
        ],
        "thinking": [
            "您点的这道菜嘛，让我想想做法",
            "这单子我得好好算算成本",
            "要把这口味调对，得琢磨一下火候",
            "嗯，这事儿要从长计议",
            "让我回忆一下当年师父怎么教的",
        ],
        "embarrassed": [
            "咳咳，刚才失言了，多担待",
            "哎，被您看穿了，有点不好意思",
            "惭愧惭愧，招牌菜差点糊了",
            "嘿嘿，被伙计偷听到我在吹牛",
            "哪里哪里，您过奖了",
        ],
        "curious": [
            "这食材我从来没见过，哪儿弄的？",
            "您这口味挺特别啊，啥讲究？",
            "嗯？这种做法头一回听说",
            "您那宝贝，能让我瞧瞧么？",
            "您从哪个城来的？有什么新鲜事？",
        ],
        "neutral": [
            "您好，请问几位？",
            "您要的菜马上就好",
            "嗯，记下了",
            "慢慢吃，不急",
            "好，账单在这儿",
        ],
    },
    "npc_grace_healer_001": {
        "happy": [
            "您气色好多了，真为您高兴",
            "太好了，按时吃药果然有效",
            "谢谢您的夸奖，我都不好意思了",
            "嗯嗯，今天出院我帮您办手续",
            "喜事临门呀，恭喜恭喜",
        ],
        "sad": [
            "节哀，您要保重身体啊",
            "病人走了，我心里也不好受",
            "您一个人在这儿，挺让人心疼的",
            "这病治不好，我也很难过",
            "想开些，时间会治愈一切的",
        ],
        "angry": [
            "怎么又不按时吃药？对自己负责点",
            "这种偏方不能信，会害了您的",
            "您怎么还抽烟？太过分了",
            "不许再喝酒，听见没",
            "那些黑心医院太可恶了！",
        ],
        "surprised": [
            "哎呀，您的伤口怎么愈合得这么快？",
            "真的？这种病还能自愈？",
            "哇，您气色比昨天好太多了！",
            "什么？您要出院了？还没到时间吧？",
            "嗯？这药方您从哪儿弄来的？",
        ],
        "thinking": [
            "您的症状，让我想想该换什么药",
            "这种病例我得查查资料",
            "嗯，这个指标有点反常",
            "您这病根可能要从情绪入手",
            "让我先给您把把脉",
        ],
        "embarrassed": [
            "啊，我刚才说错话了，对不起",
            "嗯...这事我不太擅长",
            "哎，被您问住了",
            "抱歉，我经验还不够",
            "不好意思，让您在走廊等这么久",
        ],
        "curious": [
            "您这偏方是哪儿的？我想学学",
            "嗯，您这体质挺特殊啊",
            "您从外地来？那边的医生怎么说？",
            "听说您会养生？有什么诀窍？",
            "您这病家族史有什么特征么？",
        ],
        "neutral": [
            "请坐，先量个血压",
            "嗯，按时吃药",
            "好，下周二复查",
            "您慢走",
            "嗯，记下了",
        ],
    },
    "npc_snack_owner_001": {
        "happy": [
            "哈哈，您来啦！老规矩来一份？",
            "哟呵，今天心情不错嘛！",
            "好嘞，加蛋加肠！太香了！",
            "恭喜您嘞，发大财！",
            "嘿嘿，您夸得我都不好意思了",
        ],
        "sad": [
            "哎，这行越来越难做了",
            "老顾客要回老家，挺舍不得",
            "哟，您脸色不好，节哀",
            "想当年我摆摊那会儿，多热闹",
            "这事儿嘛，挺让人难过的",
        ],
        "angry": [
            "不给钱？想吃霸王餐啊！",
            "嫌贵？嫌贵你别来啊！",
            "谁让你插队的？排队去！",
            "这种食物放地上？缺德不缺德！",
            "找茬是吧？信不信我报警！",
        ],
        "surprised": [
            "哎哟喂！您居然吃辣？",
            "真的假的？东城也有我们分店？",
            "嚯，今天生意咋这么好？",
            "啥？您要一百份？开玩笑吧！",
            "哎呀，您这手真快，我都没看清",
        ],
        "thinking": [
            "嗯，这配方得琢磨琢磨",
            "让我想想今天备料够不够",
            "您要加辣还是不加辣？",
            "这味道嘛，得好好调调",
            "嗯，让我算算成本",
        ],
        "embarrassed": [
            "哎呀，刚才手抖多放盐了",
            "咳咳，被您发现我偷偷吃东西",
            "嘿嘿，被老婆抓到偷懒了",
            "哎，刚才那个笑话讲砸了",
            "哪里哪里，您过奖了",
        ],
        "curious": [
            "哟，您这口味挺特别啊",
            "您从哪儿听说我这摊的？",
            "嗯？这种吃法头回见",
            "您那手机啥型号？挺酷啊",
            "您还会做什么新花样？",
        ],
        "neutral": [
            "您好，要点啥？",
            "好嘞，稍等",
            "嗯，这就做",
            "您慢走",
            "嗯，记下了",
        ],
    },
    "npc_book_keeper_001": {
        "happy": [
            "您挑的这本《边城》，好眼光！",
            "哈哈，知音难觅啊",
            "嗯，恭喜您读完这本",
            "这书再版了，太好了！",
            "承蒙夸奖，愧不敢当",
        ],
        "sad": [
            "这书绝版了，实在遗憾",
            "老主顾要远行，舍不得啊",
            "书店这行日渐式微，令人唏嘘",
            "哎，又一本好书下架了",
            "您提到的这位先生已经作古",
        ],
        "angry": [
            "请勿折损书页，多谢配合",
            "偷书？读书人的事不能叫偷...但还是要罚",
            "抱歉，本店概不赊账",
            "您这问法有辱斯文",
            "恕难奉陪，本店不欢迎不尊重书的客人",
        ],
        "surprised": [
            "哦？您居然知道这本书？",
            "哎呀，这版本我有十多年没见了",
            "真的？这位作者居然还在世？",
            "嗯？这种解读角度真新颖",
            "嚯，您读书量真惊人！",
        ],
        "thinking": [
            "您要的这种版本，让我想想库存",
            "这本书的脉络，得理一理",
            "嗯，作者本意可能要从这里说起",
            "这段话的意思嘛，见仁见智",
            "容我查查目录",
        ],
        "embarrassed": [
            "咳咳，方才口误，见谅",
            "惭愧，这本书我还没读完",
            "嗯，被您问住了",
            "哎，老了记性不好",
            "抱歉，这本我放错位置了",
        ],
        "curious": [
            "您对哪类书感兴趣？",
            "嗯？这位作者您也读过？",
            "您从何处听闻这位老先生？",
            "您那本藏书，能借阅否？",
            "还有什么冷门佳作推荐？",
        ],
        "neutral": [
            "您好，请随意翻阅",
            "这本五两银子",
            "嗯，那本在二楼",
            "您慢走",
            "好，记下了",
        ],
    },
    "npc_dance_leader_001": {
        "happy": [
            "哈哈，来跳舞吧！今天跳个新舞！",
            "太好了，又多一位舞友！",
            "您跳得真棒，恭喜出师！",
            "哇，今天广场真热闹！",
            "嘿嘿，您夸得我都不好意思",
        ],
        "sad": [
            "哎，老舞伴要搬走了",
            "这广场不让跳舞了，挺难过",
            "您身体不舒服？节哀顺变",
            "想起当年一起跳舞的姐妹们",
            "这事儿让人心里酸酸的",
        ],
        "angry": [
            "谁把音箱弄坏了？太过分了！",
            "不许抢舞伴位置，按顺序！",
            "跳舞请穿运动鞋，禁止穿拖鞋！",
            "谁扔的香蕉皮？找事儿是吧？",
            "别推搡，排队去！",
        ],
        "surprised": [
            "哇哦，您居然会跳这种舞？",
            "真的？北城也跳这种广场舞？",
            "啥？这么多人来围观？",
            "哎哟，您这舞步真特别！",
            "嗯？这位大爷也来凑热闹？",
        ],
        "thinking": [
            "嗯，新舞步得编排一下",
            "这段配乐得想想怎么踩点",
            "让我想想明天跳什么",
            "这队形得重新排一下",
            "嗯，节拍我得数清楚",
        ],
        "embarrassed": [
            "哎呀，刚才踩您脚了，对不起",
            "咳咳，舞步乱了",
            "嘿嘿，被徒弟超过了好丢人",
            "哎，刚才音乐放错了",
            "哪里哪里，您过奖了",
        ],
        "curious": [
            "您从哪儿来的？跳什么舞？",
            "哟，您这舞服挺漂亮啊",
            "嗯？这种舞步我没见过",
            "您那音响啥牌子的？音质真好",
            "您还会什么舞种？",
        ],
        "neutral": [
            "您好，加入我们吗？",
            "好嘞，下一首",
            "嗯，这边站",
            "您慢走",
            "嗯，记下了",
        ],
    },
}

# scenario 标签 → emotion 映射（用于语义标注，可选）
SCENARIO_BY_EMOTION: dict[str, str] = {
    "happy": "greeting",          # /agreement
    "sad": "farewell",            # /sympathy
    "angry": "refusal",
    "surprised": "surprise",
    "thinking": "storytelling",
    "embarrassed": "embarrassed",
    "curious": "curious",
    "neutral": "neutral",
}

# 期望正则匹配（仅 emotion 强指示场景加；纯情绪不强制文本匹配）
# 留 None 表示 T23 评估时只校验 emotion，不校验首句文本
TEXT_PATTERN_HINTS: dict[str, str] = {
    "happy": r".*(?:欢迎|您来了|好|真为您|谢|恭喜|太好|哈哈|嘿嘿|哟呵).*",
    "sad": r".*(?:节哀|舍不得|遗憾|难过|想念|抱歉).*",
    "angry": r".*(?:太过分|不行|不允许|不能|不许|恕不).*",
    "surprised": r".*(?:什么|真的|哎呀|哇|嚯|哎哟|哎).*",
    "thinking": r".*(?:想想|琢磨|回忆|考虑|让我).*",
    "embarrassed": r".*(?:抱歉|不好意思|惭愧|哎|咳咳|哪里).*",
    "curious": r".*(?:哪儿|什么|哪类|听说|您从|如何).*",
    "neutral": r".*",
}


def main() -> None:
    out_path = Path(__file__).parent / "llm-emotion-eval-set.jsonl"
    rows: list[dict] = []
    cid = 1
    for npc_id in NPC_IDS:
        for emo in EMOTIONS:
            for sentence in SENTENCES[npc_id][emo]:
                # 10% 概率注入多轮上下文（每 10 条样本里 1 条）
                ctx: list[dict] = []
                if cid % 10 == 0:
                    ctx = [
                        {"role": "user", "content": "您好"},
                        {"role": "assistant", "content": "您好，欢迎光临"},
                    ]
                rows.append({
                    "id": cid,
                    "npc_id": npc_id,
                    "input": sentence,
                    "expected_emotion": emo,
                    "expected_text_pattern": TEXT_PATTERN_HINTS[emo],
                    "scenario": SCENARIO_BY_EMOTION[emo],
                    "context": ctx,
                })
                cid += 1

    assert len(rows) == 200, f"expected 200 rows, got {len(rows)}"

    # 平衡性检查
    per_npc: dict[str, int] = {}
    per_emo: dict[str, int] = {}
    for r in rows:
        per_npc[r["npc_id"]] = per_npc.get(r["npc_id"], 0) + 1
        per_emo[r["expected_emotion"]] = per_emo.get(r["expected_emotion"], 0) + 1
    assert all(v == 40 for v in per_npc.values()), f"unbalanced NPC: {per_npc}"
    assert all(v == 25 for v in per_emo.values()), f"unbalanced emotion: {per_emo}"

    with out_path.open("w", encoding="utf-8") as f:
        for r in rows:
            f.write(json.dumps(r, ensure_ascii=False) + "\n")

    print(f"wrote {len(rows)} rows -> {out_path}")
    print(f"per NPC: {per_npc}")
    print(f"per emotion: {per_emo}")


if __name__ == "__main__":
    main()
