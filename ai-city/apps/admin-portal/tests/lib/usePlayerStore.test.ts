import { describe, it, expect, beforeEach } from 'vitest';
import { usePlayerStore } from '@/lib/usePlayerStore';

describe('usePlayerStore', () => {
  beforeEach(() => {
    // Reset store + clear sessionStorage between tests
    usePlayerStore.getState().clear();
    sessionStorage.clear();
  });

  it('setSelected updates selectedPlayerId', () => {
    usePlayerStore.getState().setSelected('player-uuid-123');
    expect(usePlayerStore.getState().selectedPlayerId).toBe('player-uuid-123');
  });

  it('clear resets selectedPlayerId to null', () => {
    usePlayerStore.getState().setSelected('player-uuid-123');
    usePlayerStore.getState().clear();
    expect(usePlayerStore.getState().selectedPlayerId).toBeNull();
  });

  it('selectedPlayerId persists across store re-imports (sessionStorage)', async () => {
    usePlayerStore.getState().setSelected('persist-uuid-456');
    // Wait for persist middleware to write
    await new Promise(resolve => setTimeout(resolve, 10));
    const raw = sessionStorage.getItem('admin-portal-player');
    expect(raw).toBeTruthy();
    const parsed = JSON.parse(raw!);
    expect(parsed.state.selectedPlayerId).toBe('persist-uuid-456');
  });
});
