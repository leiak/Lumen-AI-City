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

  it('selectedPlayerId is written to sessionStorage', () => {
    usePlayerStore.getState().setSelected('persist-uuid-456');
    const raw = sessionStorage.getItem('admin-portal-player');
    expect(raw).toBeTruthy();
    const parsed = JSON.parse(raw!);
    expect(parsed.state.selectedPlayerId).toBe('persist-uuid-456');
  });
});
