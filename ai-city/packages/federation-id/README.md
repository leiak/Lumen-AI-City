# packages/federation-id

> **职责**：联邦 FUUID 解析（跨城 NPC 唯一标识）
>
> **关键文档**：[docs/11-技术细节与玩法模式.md §E.5](../../docs/11-技术细节与玩法模式.md)

## FUUID 格式

```
fuid:<city>:<region>:<npc_id>@<provider>
```

示例：
- `fuid:beijing:cb:0001@openclaw` - 北京 CBD 的 NPC，openClaw 接入
- `fuid:shanghai:pudong:0042@aicity` - 上海浦东的本平台 NPC

## 用法

```python
from federation_id import FederationID

fid = FederationID.parse("fuid:beijing:cb:0001@openclaw")
print(fid.city, fid.region, fid.npc_id, fid.provider)

fid2 = FederationID.parse("fuid:shanghai:pudong:0042@aicity")
# 同 npc_id → 同一实体（双胞胎识别）
print(fid.is_same_entity(fid2))  # False
```
