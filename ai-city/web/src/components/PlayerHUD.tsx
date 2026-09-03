'use client';

import { useEffect, useState } from 'react';

interface PlayerState {
  playerId: string;
  position: { x: number; y: number };
  health: number;
  money: number;
}

export function PlayerHUD() {
  const [player, setPlayer] = useState<PlayerState>({
    playerId: 'player_001',
    position: { x: 0, y: 0 },
    health: 100,
    money: 1000,
  });

  return (
    <div className="absolute top-4 left-4 bg-gray-800/80 backdrop-blur p-4 rounded-lg shadow-lg">
      <div className="text-sm">
        <div className="font-bold mb-1">玩家: {player.playerId}</div>
        <div className="text-gray-400">坐标: {player.position.x.toFixed(1)}, {player.position.y.toFixed(1)}</div>
        <div className="text-green-400">HP: {player.health}</div>
        <div className="text-yellow-400">¥: {player.money}</div>
      </div>
    </div>
  );
}
