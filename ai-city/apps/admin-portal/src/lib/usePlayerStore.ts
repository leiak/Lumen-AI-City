/** Global zustand store for the currently selected player_id in admin-portal.
 *
 *  Persisted to sessionStorage under "admin-portal-player" so the selection
 *  survives page navigations within the tab. Consumed by /wallet,
 *  /transactions, /admin pages.
 *
 *  Why sessionStorage (not localStorage): per-tab isolation — admin running
 *  in two tabs can target different players without conflict. Selection is
 *  lost when the tab/browser is closed (this is intentional).
 *
 *  `clear()` resets the in-memory value but leaves the persisted key in place
 *  (next setSelected overwrites it). For full logout semantics, future work
 *  could call `usePlayerStore.persist.clearStorage()`.
 */
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
