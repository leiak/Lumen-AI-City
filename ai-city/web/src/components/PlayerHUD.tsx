'use client';

/**
 * 玩家 HUD —— 显式从 useGameStore 读，不持有私有 state。
 * 坐标、tile 跟随 WorldMap 的 setPosition 实时更新。
 */

import { useGameStore } from '@/store/game';

export function PlayerHUD() {
  const playerId = useGameStore((s) => s.playerId);
  const position = useGameStore((s) => s.position);
  const currentTileId = useGameStore((s) => s.currentTileId);

  return (
    <div className="absolute top-4 left-4 bg-gray-800/80 backdrop-blur p-4 rounded-lg shadow-lg">
      <div className="text-sm">
        <div className="font-bold mb-1">玩家: {playerId || '未登录'}</div>
        <div className="text-gray-400">
          坐标: {position.x.toFixed(1)}, {position.y.toFixed(1)}
        </div>
        <div className="text-gray-500">tile: {currentTileId}</div>
        <div className="text-green-400">HP: 100</div>
        <div className="text-yellow-400">¥: 1000</div>
      </div>
    </div>
  );
}
