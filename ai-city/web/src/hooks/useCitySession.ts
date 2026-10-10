'use client';

import { useEffect } from 'react';
import { api } from '@/lib/api';
import { useGameStore } from '@/store/game';

function createGuestId(): string {
  return `guest_${Date.now().toString(36)}`;
}

function restoreGuest(): string {
  const existing = window.localStorage.getItem('aicity_player_id') ?? '';
  const isGuest = existing.startsWith('guest_');
  const guestId = isGuest ? existing : createGuestId();
  if (existing !== guestId) {
    window.localStorage.setItem('aicity_player_id', guestId);
  }
  useGameStore.getState().setGuestPlayer(guestId);
  return guestId;
}

export function useCitySession(): void {
  useEffect(() => {
    let cancelled = false;
    const token = window.localStorage.getItem('aicity_token');

    if (!token) {
      api.setToken('');
      restoreGuest();
      return () => {
        cancelled = true;
      };
    }

    api.setToken(token);
    api
      .getMe()
      .then((profile) => {
        if (cancelled) return;
        window.localStorage.setItem('aicity_player_id', profile.player_id);
        window.localStorage.setItem('aicity_username', profile.username);
        useGameStore.getState().setAuthenticatedPlayer({
          playerId: profile.player_id,
          username: profile.username,
          displayName: profile.display_name,
        });
      })
      .catch(() => {
        if (cancelled) return;
        window.localStorage.removeItem('aicity_token');
        window.localStorage.removeItem('aicity_username');
        api.setToken('');
        restoreGuest();
      });

    return () => {
      cancelled = true;
    };
  }, []);
}
