'use client';

import { useEffect, useRef } from 'react';
import maplibregl from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { ClientReconciler } from '@aicity/client-reconciler';

const reconciler = new ClientReconciler();

export function MapView() {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return;

    const map = new maplibregl.Map({
      container: containerRef.current,
      style: 'https://demotiles.maplibre.org/style.json',
      center: [116.4074, 39.9042], // 北京
      zoom: 13,
    });

    mapRef.current = map;

    // 客户端点击移动
    map.on('click', (e) => {
      const target = { x: e.lngLat.lng, y: e.lngLat.lat };
      const move = reconciler.nextMove('player_001', target);
      // TODO: 发送 WebSocket 消息
      console.log('move:', move);
    });

    return () => {
      map.remove();
      mapRef.current = null;
    };
  }, []);

  return <div ref={containerRef} className="absolute inset-0" />;
}
