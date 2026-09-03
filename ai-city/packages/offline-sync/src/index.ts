/**
 * 离线日志同步协议（§E.7）
 *
 * 流程：
 * 1. 检测离线（navigator.onLine === false 或心跳超时）
 * 2. 写操作入队 IndexedDB
 * 3. 重新上线时按时间顺序回放
 * 4. 服务端按 trace_id 去重
 */

export interface OfflineOp {
  op_id: string;
  ts_ms: number;
  trace_id: string;
  type: string;
  payload: unknown;
  retry_count: number;
}

export class OfflineQueue {
  private queue: OfflineOp[] = [];
  private isOnline = true;

  constructor(
    private storageKey = 'aicity_offline_queue',
    private maxQueueSize = 1000,
  ) {
    this.loadFromStorage();
    if (typeof window !== 'undefined') {
      window.addEventListener('online', () => this.flush());
    }
  }

  enqueue(op: OfflineOp): void {
    this.queue.push(op);
    if (this.queue.length > this.maxQueueSize) {
      this.queue.shift(); // 丢弃最老的
    }
    this.persist();
  }

  async flush(sendFn: (op: OfflineOp) => Promise<boolean>): Promise<void> {
    if (!this.isOnline) return;

    const remaining: OfflineOp[] = [];
    for (const op of this.queue) {
      try {
        const ok = await sendFn(op);
        if (!ok) {
          op.retry_count += 1;
          if (op.retry_count < 5) remaining.push(op);
        }
      } catch {
        remaining.push(op);
      }
    }

    this.queue = remaining;
    this.persist();
  }

  setOnline(online: boolean): void {
    this.isOnline = online;
    if (online) this.flush(op => fetch('/sync', { method: 'POST', body: JSON.stringify(op) }).then(r => r.ok));
  }

  private loadFromStorage(): void {
    if (typeof localStorage === 'undefined') return;
    const data = localStorage.getItem(this.storageKey);
    if (data) this.queue = JSON.parse(data);
  }

  private persist(): void {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(this.storageKey, JSON.stringify(this.queue));
  }
}
