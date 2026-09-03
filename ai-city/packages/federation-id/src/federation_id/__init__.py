"""Federation FUUID - 跨城联邦唯一 ID 解析。

格式：fuid:<city>:<region>:<npc_id>@<provider>
示例：fuid:beijing:cb:0001@openclaw

详细设计见 docs/11-技术细节与玩法模式.md §E.5。
"""
from __future__ import annotations

import re
from dataclasses import dataclass

_FUUID_RE = re.compile(r"^fuid:([a-z0-9_-]+):([a-z0-9_-]+):([a-z0-9_-]+)@([a-z0-9_-]+)$")


@dataclass
class FederationID:
    city: str
    region: str
    npc_id: str
    provider: str

    @property
    def fuuid(self) -> str:
        return f"fuid:{self.city}:{self.region}:{self.npc_id}@{self.provider}"

    @classmethod
    def parse(cls, s: str) -> "FederationID":
        m = _FUUID_RE.match(s)
        if not m:
            raise ValueError(f"invalid FUUID: {s}")
        return cls(city=m.group(1), region=m.group(2), npc_id=m.group(3), provider=m.group(4))

    def is_same_entity(self, other: "FederationID") -> bool:
        """跨城识别同一 NPC（如双胞胎 NPC）。"""
        return self.npc_id == other.npc_id
