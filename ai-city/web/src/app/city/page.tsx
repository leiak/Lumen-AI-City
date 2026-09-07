'use client';

import { WorldMap } from '@/components/Map/WorldMap';
import { PlayerHUD } from '@/components/PlayerHUD';
import { ChatBox } from '@/components/ChatBox';

export default function CityPage() {
  return (
    <div className="relative h-screen w-screen overflow-hidden">
      <div className="map-container absolute inset-0">
        <WorldMap />
      </div>
      <PlayerHUD />
      <ChatBox />
    </div>
  );
}
