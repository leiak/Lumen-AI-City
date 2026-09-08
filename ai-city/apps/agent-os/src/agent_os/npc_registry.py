"""从 packages/npc-templates/*.yaml 加载 NPC 模板；过滤 enabled。

Sprint 12 只读 wang_boss.yaml（其它 NPC enabled: false）。
1.0 demo 接受启动期一次性 load，不监听 mtime。
"""
from __future__ import annotations

import logging
from dataclasses import dataclass, field
from pathlib import Path

import yaml

logger = logging.getLogger(__name__)


@dataclass
class Say:
    greeting: list[str] = field(default_factory=list)
    welcome: list[str] = field(default_factory=list)
    default_reply: str = ""


@dataclass
class DialogOption:
    id: str
    text: str


@dataclass
class TalkNode:
    say: str = ""
    options: list[DialogOption] = field(default_factory=list)
    reply_map: dict[str, str] = field(default_factory=dict)
    next_options: list[DialogOption] = field(default_factory=list)


@dataclass
class TalkTree:
    initial: str = ""
    nodes: dict[str, TalkNode] = field(default_factory=dict)


@dataclass(frozen=True)
class OceanPersonality:
    """OCEAN 五维人格 (Openness, Conscientiousness, Extraversion, Agreeableness, Neuroticism).
    每个字段 0.0–1.0 float；Sprint 13+ 才会被 dispatcher 用来影响台词选择。
    """
    openness: float          # 开放性
    conscientiousness: float # 尽责性
    extraversion: float      # 外向性
    agreeableness: float     # 宜人性
    neuroticism: float       # 神经质


@dataclass
class NpcTemplate:
    npc_id: str
    name: str = ""
    enabled: bool = False
    home_tile_id: str = ""
    avatar_url: str = ""
    say: Say = field(default_factory=Say)
    talk_tree: TalkTree = field(default_factory=TalkTree)
    personality: OceanPersonality | None = None  # 可选；旧 YAML 没填则 None


@dataclass
class _LoadError:
    """Sentinel for files that failed to parse — get() raises ValueError."""

    file: str
    err: str


def _load_one(path: Path) -> NpcTemplate:
    data = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    npc_id = data.get("npc_id")
    if not npc_id:
        raise ValueError(f"{path.name}: missing npc_id")

    say_data = data.get("say", {}) or {}
    say = Say(
        greeting=list(say_data.get("greeting", []) or []),
        welcome=list(say_data.get("welcome", []) or []),
        default_reply=str(say_data.get("default_reply", "") or ""),
    )

    tree_data = data.get("talk_tree", {}) or {}
    nodes: dict[str, TalkNode] = {}
    for nid, n in (tree_data.get("nodes", {}) or {}).items():
        opts = [DialogOption(id=o["id"], text=o["text"]) for o in (n.get("options", []) or [])]
        next_opts = [DialogOption(id=o["id"], text=o["text"]) for o in (n.get("next_options", []) or [])]
        nodes[nid] = TalkNode(
            say=str(n.get("say", "") or ""),
            options=opts,
            reply_map=dict(n.get("reply_map", {}) or {}),
            next_options=next_opts,
        )
    tree = TalkTree(initial=str(tree_data.get("initial", "") or ""), nodes=nodes)

    personality = None
    if "personality" in data and data["personality"] is not None:
        p = data["personality"]
        if not isinstance(p, dict):
            raise ValueError(f"{path.name}: personality must be a mapping")
        try:
            personality = OceanPersonality(
                openness=float(p["openness"]),
                conscientiousness=float(p["conscientiousness"]),
                extraversion=float(p["extraversion"]),
                agreeableness=float(p["agreeableness"]),
                neuroticism=float(p["neuroticism"]),
            )
        except KeyError as e:
            raise ValueError(f"{path.name}: personality missing field {e}")

    return NpcTemplate(
        npc_id=npc_id,
        name=str(data.get("name", "") or ""),
        enabled=bool(data.get("enabled", False)),
        home_tile_id=str(data.get("home_tile_id", "") or ""),
        avatar_url=str(data.get("avatar_url", "") or ""),
        say=say,
        talk_tree=tree,
        personality=personality,
    )


class NpcRegistry:
    """启动期一次性 load 全部 yaml。"""

    def __init__(self, templates_dir: str | Path) -> None:
        self._dir = Path(templates_dir)
        self._by_id: dict[str, NpcTemplate | _LoadError] = {}
        if not self._dir.exists():
            logger.warning("npc_templates_dir not found", extra={"dir": str(self._dir)})
            return
        for p in sorted(self._dir.glob("*.yaml")):
            try:
                t = _load_one(p)
                self._by_id[t.npc_id] = t
            except ValueError as e:
                logger.error("npc yaml config error", extra={"file": p.name, "err": str(e)})
                # Use filename stem as key so get("bad") raises ValueError for bad.yaml
                self._by_id[p.stem] = _LoadError(file=p.name, err=str(e))
            except Exception as e:
                logger.warning("npc yaml load failed", extra={"file": p.name, "err": str(e)})
                continue
        logger.info("npc_registry loaded", extra={"count": len(self._by_id)})

    def get(self, npc_id: str) -> NpcTemplate:
        val = self._by_id.get(npc_id)
        if val is None:
            raise KeyError(npc_id)
        if isinstance(val, _LoadError):
            raise ValueError(f"{val.file}: {val.err}")
        return val

    def list_enabled(self) -> list[NpcTemplate]:
        return [t for t in self._by_id.values() if t.enabled]
