import { create } from 'zustand';
import { persist, createJSONStorage } from 'zustand/middleware';

interface PlayerState {
  selectedPlayerId: string | null;
  setSelected: (id: string) => void;
  clear: () => void;
}

export const usePlayerStore = create<PlayerState>()(
  persist(
    (set) => ({
      selectedPlayerId: null,
      setSelected: (id: string) => set({ selectedPlayerId: id }),
      clear: () => set({ selectedPlayerId: null }),
    }),
    {
      name: 'admin-portal-player',
      storage: createJSONStorage(() => sessionStorage),
    }
  )
);
