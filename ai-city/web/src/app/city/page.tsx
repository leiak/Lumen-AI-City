'use client';

import { useEffect } from 'react';
import { WorldMap } from '@/components/Map/WorldMap';
import { PlayerHUD } from '@/components/PlayerHUD';
import { ChatBox } from '@/components/ChatBox';
import { NPCDialog } from '@/components/NPCDialog';
import { startWsBridge } from '@/lib/ws-events';

export default function CityPage() {
  // 进 city 页起 WS 桥：ws-gateway 的 player_moved 推送取代 WorldMap 的 3s 轮询
  useEffect(() => startWsBridge(), []);

  return (
    <div className="relative h-screen w-screen overflow-hidden">
      <div className="map-container absolute inset-0">
        <WorldMap />
      </div>
      <PlayerHUD />
      <ChatBox />
      {/* Sprint 12 T03e：顶层挂 NPCDialog，监听 aicity:npc_dialogue
          CustomEvent；payload 为 null 时返回 null，不影响其它层。 */}
      <NPCDialog />
    </div>
  );
}
