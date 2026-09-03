# @aicity/offline-sync

> **职责**：离线日志队列与重新上线同步
>
> **关键文档**：[docs/11-技术细节与玩法模式.md §E.7](../../docs/11-技术细节与玩法模式.md)

## 用法

```typescript
import { OfflineQueue } from '@aicity/offline-sync';

const queue = new OfflineQueue();

// 玩家发起一个 NPC 对话（在线）
if (navigator.onLine) {
  await sendToServer(op);
} else {
  queue.enqueue(op);  // 入队
}

// 重新上线时
window.addEventListener('online', () => {
  queue.flush(async (op) => {
    const resp = await fetch('/sync', { method: 'POST', body: JSON.stringify(op) });
    return resp.ok;
  });
});
```
