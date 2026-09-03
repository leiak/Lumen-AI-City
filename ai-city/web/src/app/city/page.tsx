'use client';

import dynamic from 'next/dynamic';
import { PlayerHUD } from '@/components/PlayerHUD';
import { ChatBox } from '@/components/ChatBox';

const MapView = dynamic(() => import('@/components/Map/MapView').then(m => m.MapView), {
  ssr: false,
  loading: () => <div className="text-gray-400 p-4">加载地图中...</div>,
});

export default function CityPage() {
  return (
    <div className="relative h-screen w-screen overflow-hidden">
      <div className="map-container">
        <MapView />
      </div>
      <PlayerHUD />
      <ChatBox />
    </div>
  );
}
