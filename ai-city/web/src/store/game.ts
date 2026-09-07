/**
 * 全局状态管理（Zustand）。
 *
 * 坐标 / tile id 规则（与 apps/world-engine/src/tile.rs::Tile::from_xy 一致）：
 *   tile 边长 100；tile_(i,j) 中心 = (50 + 100i, 50 + 100j)；范围 [i*100, (i+1)*100) × [j*100, (j+1)*100)
 *   因此 (x,y) → tile id = `tile_${Math.floor(x/100)}_${Math.floor(y/100)}`
 *   例：x=0 → tile_0_x；x=-1 → tile_-1_x；x=-50 → tile_-1_x（边界属于左/下/负向 tile）
 */
import { create } from 'zustand';

interface GameState {
  /** 玩家 UUID，登录后由 localStorage.aicity_player_id 注入；移动前必须非空 */
  playerId: string;
  position: { x: number; y: number };
  currentTileId: string;
  selectedNpcId: string | null;
  isOnline: boolean;

  setPosition: (pos: { x: number; y: number }) => void;
  selectNpc: (id: string | null) => void;
  setOnline: (online: boolean) => void;
  /** 设置 playerId（一般只在登录后第一次调） */
  setPlayerId: (id: string) => void;
}

function readPlayerIdFromStorage(): string {
  if (typeof window === 'undefined') return '';
  return window.localStorage.getItem('aicity_player_id') || '';
}

export const useGameStore = create<GameState>((set) => ({
  playerId: readPlayerIdFromStorage(),
  position: { x: 0, y: 0 },
  currentTileId: 'tile_0_0',
  selectedNpcId: null,
  isOnline: true,

  setPosition: (position) =>
    set({
      position,
      currentTileId: `tile_${Math.floor(position.x / 100)}_${Math.floor(position.y / 100)}`,
    }),
  selectNpc: (selectedNpcId) => set({ selectedNpcId }),
  setOnline: (isOnline) => set({ isOnline }),
  setPlayerId: (playerId) => set({ playerId }),
}));
