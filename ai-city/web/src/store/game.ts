/**
 * 全局状态管理（Zustand）。
 */
import { create } from 'zustand';

interface GameState {
  playerId: string;
  position: { x: number; y: number };
  currentTileId: string;
  selectedNpcId: string | null;
  isOnline: boolean;

  setPosition: (pos: { x: number; y: number }) => void;
  selectNpc: (id: string | null) => void;
  setOnline: (online: boolean) => void;
}

export const useGameStore = create<GameState>((set) => ({
  playerId: 'player_001',
  position: { x: 0, y: 0 },
  currentTileId: 'tile_0_0',
  selectedNpcId: null,
  isOnline: true,

  setPosition: (position) => set({ position, currentTileId: `tile_${Math.floor(position.x / 100)}_${Math.floor(position.y / 100)}` }),
  selectNpc: (selectedNpcId) => set({ selectedNpcId }),
  setOnline: (isOnline) => set({ isOnline }),
}));
