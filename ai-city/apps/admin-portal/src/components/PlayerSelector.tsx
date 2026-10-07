'use client';

import { useQuery } from '@tanstack/react-query';
import { usePlayerStore } from '@/lib/usePlayerStore';

interface Player {
  id: string;
  username: string;
  role: string;
}

export function PlayerSelector() {
  const selectedId = usePlayerStore((s) => s.selectedPlayerId);
  const setSelected = usePlayerStore((s) => s.setSelected);

  const { data, isLoading } = useQuery<{ players: Player[] }>({
    queryKey: ['players'],
    queryFn: () => fetch('/api/players').then((r) => r.json()),
    staleTime: 5 * 60 * 1000, // 5 min
  });

  if (isLoading) return <span className="text-sm text-gray-500">加载玩家…</span>;

  const players = data?.players ?? [];

  return (
    <label className="flex items-center gap-2 text-sm">
      <span className="text-gray-700">Player:</span>
      <select
        data-testid="player-selector"
        value={selectedId ?? ''}
        onChange={(e) => setSelected(e.target.value)}
        className="border border-gray-300 rounded px-2 py-1 bg-white"
      >
        <option value="" disabled>— 选择 —</option>
        {players.map((p) => (
          <option key={p.id} value={p.id}>
            {p.username} ({p.role})
          </option>
        ))}
      </select>
    </label>
  );
}