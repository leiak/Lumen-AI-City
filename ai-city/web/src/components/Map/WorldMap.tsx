'use client';

/**
 * SVG 世界地图（Sprint 8）
 *
 * - viewBox 直接用世界坐标 [-100, -100  300 300]，与 /v1/tiles 返回的 center_x/center_y 一致
 * - y 轴翻转：world 坐标 y 向上，SVG y 向下 → 整张 <g transform="scale(1,-1)"> 一次搞定
 * - 数据：3s 轮询 GET /v1/tiles；move 成功后立即再拉一次
 * - 点击：<svg> onClick 判 e.target.tagName，避开 polygon/circle 才视为"点空地"发 move
 * - 坐标系反推：getScreenCTM().inverse() → client (x,y) → world (x, -y)
 * - 乐观更新 + 失败回滚：move 成功才把 position 落 store
 */

import { useCallback, useEffect, useRef, useState } from 'react';
import { api, type Tile } from '@/lib/api';
import { useGameStore } from '@/store/game';
import { TILE_SIZE, tileIdAt } from '@/lib/world-coords';

const POLL_MS = 3000;
const HALF = 50; // tile 边长 100 的半值；tile 中心 = (50, 50) 之类的奇数倍

type LodColor = Record<Tile['lod_level'], string>;
const LOD_FILL: LodColor = {
  CBD: '#6366f1', // brand-500
  Residential: '#4338ca', // brand-700
  Suburb: '#4b5563', // gray-600
};
const LOD_STROKE: LodColor = {
  CBD: '#312e81',
  Residential: '#1e1b4b',
  Suburb: '#1f2937',
};

const BUILDING_FILL: Record<string, string> = {
  Tavern: '#a16207',
  Plaza: '#737373',
  House: '#475569',
  Shop: '#9a3412',
  Park: '#16a34a',
  Road: '#525252',
  Office: '#1e40af',
};

/** SVG 客户端坐标 → world 坐标（用 CTM 反推 + y 翻转） */
function screenToWorld(svg: SVGSVGElement, clientX: number, clientY: number): { x: number; y: number } {
  const pt = svg.createSVGPoint();
  pt.x = clientX;
  pt.y = clientY;
  const ctm = svg.getScreenCTM();
  if (!ctm) return { x: 0, y: 0 };
  const local = pt.matrixTransform(ctm.inverse());
  // svg 内 (x, y) 是已镜像的；用 scale(1,-1) 翻转后 worldY = -svgY
  return { x: local.x, y: -local.y };
}

interface MapState {
  tiles: Tile[];
  loading: boolean;
  error: string | null;
  lastFetch: number;
}

export function WorldMap() {
  const playerId = useGameStore((s) => s.playerId);
  const setPosition = useGameStore((s) => s.setPosition);
  const myTileId = useGameStore((s) => s.currentTileId);
  const [state, setState] = useState<MapState>({
    tiles: [],
    loading: true,
    error: null,
    lastFetch: 0,
  });
  const [myPos, setMyPos] = useState<{ x: number; y: number } | null>(null);
  const [moving, setMoving] = useState(false);
  const svgRef = useRef<SVGSVGElement>(null);
  const lastMoveTileRef = useRef<string>(myTileId);

  // 拉一次 tiles
  const fetchTiles = useCallback(async () => {
    try {
      const tiles = await api.getTiles();
      setState({ tiles, loading: false, error: null, lastFetch: Date.now() });

      // 首次加载或还没找到自己 → 找一个"我"所在的 tile center 作为初始位置
      if (myPos === null && playerId) {
        const myTile = tiles.find((t) => t.player_ids.includes(playerId));
        if (myTile) {
          setMyPos({ x: myTile.center_x, y: myTile.center_y });
          setPosition({ x: myTile.center_x, y: myTile.center_y });
          // 同步 lastMoveTileRef，否则首次 move 的 from_tile_id 会是 store 兜底默认值
          lastMoveTileRef.current = myTile.id;
        }
      }
    } catch (err) {
      setState((s) => ({
        ...s,
        loading: false,
        error: err instanceof Error ? err.message : '加载失败',
      }));
    }
  }, [myPos, playerId, setPosition]);

  // 启动 3s 轮询
  useEffect(() => {
    fetchTiles();
    const t = setInterval(fetchTiles, POLL_MS);
    return () => clearInterval(t);
  }, [fetchTiles]);

  // 兜底：本地没 playerId 时每 200ms 试一次（处理 login 后跳 city 的极小窗口）
  useEffect(() => {
    if (playerId) return;
    const t = setInterval(() => {
      const id = window.localStorage.getItem('aicity_player_id');
      if (id) {
        useGameStore.getState().setPlayerId(id);
      }
    }, 200);
    return () => clearInterval(t);
  }, [playerId]);

  const onSvgClick = async (e: React.MouseEvent<SVGSVGElement>) => {
    if (!svgRef.current) return;
    if (!playerId) {
      setState((s) => ({ ...s, error: '未登录（playerId 缺失）' }));
      return;
    }
    // e.target 是事件触发的元素；点 polygon/circle 不发 move（留给后续 chat/NPC 交互）
    const tag = (e.target as Element).tagName;
    if (tag === 'polygon' || tag === 'circle') return;

    const world = screenToWorld(svgRef.current, e.clientX, e.clientY);
    const targetX = world.x;
    const targetY = world.y;
    const toTileId = tileIdAt(targetX, targetY);
    const fromTileId = lastMoveTileRef.current;

    setMoving(true);
    // 乐观更新
    const prev = myPos;
    setMyPos({ x: targetX, y: targetY });
    try {
      const resp = await api.move({
        player_id: playerId,
        from_tile_id: fromTileId,
        to_tile_id: toTileId,
        x: targetX,
        y: targetY,
      });
      // 用服务端校正位置（如有）
      setMyPos({ x: resp.x, y: resp.y });
      setPosition({ x: resp.x, y: resp.y });
      lastMoveTileRef.current = resp.current_tile_id || toTileId;
      setState((s) => ({ ...s, error: null }));
      // 成功后立即重拉（让 tile.player_ids 立即反映新位置）
      fetchTiles();
    } catch (err) {
      // 失败回滚
      if (prev) setMyPos(prev);
      setState((s) => ({
        ...s,
        error: err instanceof Error ? err.message : '移动失败',
      }));
    } finally {
      setMoving(false);
    }
  };

  if (state.loading && state.tiles.length === 0) {
    return (
      <div className="absolute inset-0 flex items-center justify-center bg-gray-900 text-gray-400">
        加载地图中...
      </div>
    );
  }

  return (
    <div className="relative h-full w-full bg-gray-950">
      <svg
        ref={svgRef}
        viewBox="-100 -100 300 300"
        className="absolute inset-0 h-full w-full"
        onClick={onSvgClick}
        style={{ cursor: moving ? 'wait' : 'crosshair' }}
      >
        {/* y 翻转：world y 向上 → SVG y 向下；text 单独放外面避免被翻倒 */}
        <g transform="scale(1,-1)">
          {state.tiles.map((t) => (
            <TileGroup key={t.id} tile={t} />
          ))}

          {/* NPC 圆点（用所在 tile 中心） */}
          {state.tiles.flatMap((t) =>
            t.npc_ids.map((nid) => (
              <circle
                key={nid}
                cx={t.center_x}
                cy={t.center_y}
                r={3}
                fill="#fbbf24"
                stroke="#78350f"
                strokeWidth={0.5}
                data-npc-id={nid}
              >
                <title>{nid}</title>
              </circle>
            )),
          )}

          {/* 其它玩家（不含自己）：所在 tile 中心；自己单独画在 myPos */}
          {state.tiles.flatMap((t) =>
            t.player_ids
              .filter((pid) => pid !== playerId)
              .map((pid) => (
                <circle
                  key={pid}
                  cx={t.center_x}
                  cy={t.center_y}
                  r={3}
                  fill="#9ca3af"
                  stroke="#374151"
                  strokeWidth={0.5}
                  data-player-id={pid}
                >
                  <title>{pid}</title>
                </circle>
              )),
          )}

          {/* 自己：myPos（move target / 初始 tile center），蓝大圆 + 浅蓝光圈 */}
          {myPos && (
            <g>
              <circle
                cx={myPos.x}
                cy={myPos.y}
                r={9}
                fill="none"
                stroke="#3b82f6"
                strokeOpacity={0.35}
                strokeWidth={0.5}
              />
              <circle
                cx={myPos.x}
                cy={myPos.y}
                r={5}
                fill="#3b82f6"
                stroke="#1e3a8a"
                strokeWidth={1}
                data-player-id={playerId}
              >
                <title>我 {playerId}</title>
              </circle>
            </g>
          )}
        </g>

        {/* tile id 标签（SVG 正常坐标，避免被 y-flip 翻倒）
            y = -(center_y - HALF + 4)：world 中"tile 顶部"对应 SVG 顶部 */}
        {state.tiles.map((t) => (
          <text
            key={`label-${t.id}`}
            x={t.center_x}
            y={-(t.center_y - HALF + 4)}
            textAnchor="middle"
            fontSize={5}
            fill="#e5e7eb"
            opacity={0.8}
            data-tile-label={t.id}
          >
            {t.id}
          </text>
        ))}
      </svg>

      {/* HUD overlay */}
      <div className="pointer-events-none absolute top-2 right-2 bg-gray-800/80 backdrop-blur px-3 py-2 rounded text-xs text-gray-300">
        <div>tile 数: {state.tiles.length}</div>
        <div>NPC: {state.tiles.reduce((n, t) => n + t.npc_ids.length, 0)}</div>
        <div>玩家: {state.tiles.reduce((n, t) => n + t.player_ids.length, 0)}</div>
        {myPos && (
          <div>
            坐标: {myPos.x.toFixed(0)}, {myPos.y.toFixed(0)} · {tileIdAt(myPos.x, myPos.y)}
          </div>
        )}
      </div>

      {state.error && (
        <div className="pointer-events-none absolute bottom-20 left-1/2 -translate-x-1/2 bg-red-900/80 border border-red-700 text-red-200 px-4 py-2 rounded text-sm shadow-lg">
          {state.error}
        </div>
      )}
    </div>
  );
}

function TileGroup({ tile }: { tile: Tile }) {
  const x = tile.center_x - HALF;
  const y = tile.center_y - HALF;
  return (
    <g data-tile-id={tile.id}>
      <rect
        x={x}
        y={y}
        width={TILE_SIZE}
        height={TILE_SIZE}
        fill={LOD_FILL[tile.lod_level]}
        fillOpacity={0.55}
        stroke={LOD_STROKE[tile.lod_level]}
        strokeWidth={0.5}
      />
      {tile.buildings.map((b) => {
        const points = b.polygon
          .map(([px, py]) => `${x + px},${y + py}`)
          .join(' ');
        return (
          <polygon
            key={b.id}
            points={points}
            fill={BUILDING_FILL[b.kind] ?? '#64748b'}
            fillOpacity={0.85}
            stroke="#0f172a"
            strokeWidth={0.3}
            data-building-id={b.id}
            data-building-kind={b.kind}
          >
            <title>
              {b.kind} {b.id}
            </title>
          </polygon>
        );
      })}
    </g>
  );
}
