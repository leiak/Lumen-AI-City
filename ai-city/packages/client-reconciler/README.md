# @aicity/client-reconciler

> **职责**：客户端预测 + 服务端协调协议
>
> **关键文档**：[docs/11-技术细节与玩法模式.md §E.6](../../docs/11-技术细节与玩法模式.md)

## 用法

```typescript
import { ClientReconciler } from '@aicity/client-reconciler';

const reconciler = new ClientReconciler();

// 玩家点击移动
const move = reconciler.nextMove('player_001', { x: 10, y: 5 });
ws.send(move);

// 收到服务端响应
ws.on('move_resp', (resp) => {
  const newPos = reconciler.applyCorrection(player.position, resp);
  player.position = newPos;
});
```

## 与服务端配合

对应 proto：[`packages/proto/world.proto`](../../packages/proto/world.proto) 中 `MoveRequest.predicted` 字段。
