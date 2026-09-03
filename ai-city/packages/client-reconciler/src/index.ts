/**
 * 客户端预测 + 服务端协调（§E.6）
 *
 * 核心思想：
 * 1. 客户端立即应用预测位置（无延迟感）
 * 2. 发送 MoveRequest with predicted=true
 * 3. 收到 MoveResponse 校正位置
 * 4. 误差超过阈值则回滚
 */

export interface Vec2 {
  x: number;
  y: number;
}

export interface MoveRequest {
  entity_id: string;
  ts_ms: number;
  target: Vec2;
  predicted: boolean;
  sequence: number;
}

export interface MoveResponse {
  accepted: boolean;
  corrected_position: Vec2;
  sequence: number;
  server_ts_ms: number;
}

export class ClientReconciler {
  private pendingMoves: Map<number, MoveRequest> = new Map();
  private sequence = 0;

  /**
   * 生成下一个 move 请求（sequence 单调递增）
   */
  nextMove(entityId: string, target: Vec2, predicted = true): MoveRequest {
    const seq = ++this.sequence;
    const move: MoveRequest = {
      entity_id: entityId,
      ts_ms: Date.now(),
      target,
      predicted,
      sequence: seq,
    };
    this.pendingMoves.set(seq, move);
    return move;
  }

  /**
   * 收到服务端响应，应用校正
   */
  applyCorrection(
    currentPos: Vec2,
    response: MoveResponse,
    threshold = 0.5,
  ): Vec2 {
    const pending = this.pendingMoves.get(response.sequence);
    if (!pending) return currentPos;

    const dx = response.corrected_position.x - currentPos.x;
    const dy = response.corrected_position.y - currentPos.y;
    const dist = Math.sqrt(dx * dx + dy * dy);

    if (dist > threshold) {
      // 误差过大，回滚
      this.pendingMoves.delete(response.sequence);
      return response.corrected_position;
    }

    // 误差在容忍范围，保留预测
    this.pendingMoves.delete(response.sequence);
    return currentPos;
  }

  /**
   * 清理超时的 pending
   */
  cleanupStale(maxAgeMs = 5000): void {
    const now = Date.now();
    for (const [seq, move] of this.pendingMoves) {
      if (now - move.ts_ms > maxAgeMs) {
        this.pendingMoves.delete(seq);
      }
    }
  }
}
