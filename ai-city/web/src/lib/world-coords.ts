/**
 * 世界坐标工具（与 apps/world-engine/src/tile.rs 镜像）
 *
 * 关键不变量：
 * - tile_(i,j) 中心 = (50 + 100·i, 50 + 100·j)，范围 [100i, 100(i+1)) × [100j, 100(j+1))
 * - world-engine Tile::from_xy: `format!("tile_{}_{}", (x/size).floor(), (y/size).floor())`
 * - 浏览器 SVG 渲染：world y 向上、SVG y 向下；用 scale(1,-1) 一次翻转
 * - 屏幕→世界：getScreenCTM().inverse() 反推 CTM；返回的 svgY 取反即得 worldY
 */

export const TILE_SIZE = 100;

/** world (x,y) → tile id（必须与 world-engine Tile::from_xy 一致） */
export function tileIdAt(x: number, y: number): string {
  return `tile_${Math.floor(x / TILE_SIZE)}_${Math.floor(y / TILE_SIZE)}`;
}

/** 把 world (x,y) 转成 SVG 内部 (x, -y)（在 scale(1,-1) 内使用时无需再翻转） */
export function worldToSvgY(worldY: number): number {
  return -worldY;
}
