/**
 * WebSocket 客户端封装。
 */
import { OfflineQueue } from '@aicity/offline-sync';

const WS_URL = process.env.NEXT_PUBLIC_WS_GATEWAY || 'ws://localhost:8082/ws';

class WSClient {
  private ws: WebSocket | null = null;
  private listeners = new Set<(msg: unknown) => void>();
  private offlineQueue = new OfflineQueue();
  private reconnectDelay = 1000;

  connect(token: string) {
    if (this.ws?.readyState === WebSocket.OPEN) return;

    this.ws = new WebSocket(`${WS_URL}?token=${token}`);

    this.ws.onopen = () => {
      console.log('WS connected');
      this.reconnectDelay = 1000;
      // flush offline queue
      this.offlineQueue.flush(async (op) => {
        if (this.ws?.readyState === WebSocket.OPEN) {
          this.ws.send(JSON.stringify(op));
          return true;
        }
        return false;
      });
    };

    this.ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data);
        this.listeners.forEach((fn) => fn(msg));
      } catch (err) {
        console.warn('invalid WS message', err);
      }
    };

    this.ws.onclose = () => {
      setTimeout(() => this.connect(token), this.reconnectDelay);
      this.reconnectDelay = Math.min(this.reconnectDelay * 2, 30000);
    };

    this.ws.onerror = (e) => console.warn('WS error', e);
  }

  send(msg: unknown) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg));
    } else {
      this.offlineQueue.enqueue({
        op_id: crypto.randomUUID(),
        ts_ms: Date.now(),
        trace_id: crypto.randomUUID(),
        type: 'ws_msg',
        payload: msg,
        retry_count: 0,
      });
    }
  }

  onMessage(fn: (msg: unknown) => void) {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }
}

export const ws = new WSClient();
