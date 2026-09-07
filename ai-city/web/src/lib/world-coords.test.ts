/**
 * world-coords 单元测试 —— 守护"tile id 公式与 world-engine 一致"这条不变量。
 *
 * 如果改 tileIdAt 公式忘了同步 world-engine（或反过来），这些断言会失败。
 */
import { describe, expect, it } from 'vitest';
import { TILE_SIZE, tileIdAt, worldToSvgY } from './world-coords';

describe('tileIdAt', () => {
  it('matches world-engine Tile::from_xy at tile centers', () => {
    // tile_0_0 center (50, 50), tile_1_0 (150, 50), tile_-1_1 (-50, 150)
    expect(tileIdAt(50, 50)).toBe('tile_0_0');
    expect(tileIdAt(150, 50)).toBe('tile_1_0');
    expect(tileIdAt(-50, 150)).toBe('tile_-1_1');
  });

  it('matches world-engine Tile::from_xy on the world-engine test cases', () => {
    // 这三个是 apps/world-engine/src/tile.rs::tests 里固化的测试点
    expect(tileIdAt(0, 0)).toBe('tile_0_0');
    expect(tileIdAt(150, 250)).toBe('tile_1_2');
    expect(tileIdAt(-50, -150)).toBe('tile_-1_-2');
  });

  it('boundary x=99 → tile_0 (still in [0,100))', () => {
    expect(tileIdAt(99, 0)).toBe('tile_0_0');
    expect(tileIdAt(99.999, 0)).toBe('tile_0_0');
  });

  it('boundary x=100 → tile_1 (next tile starts at 100)', () => {
    expect(tileIdAt(100, 0)).toBe('tile_1_0');
  });

  it('negative x: -1 is in tile_-1 (range [-100, 0))', () => {
    // Math.floor(-0.01) = -1；不是 Math.floor(0.49) = 0
    expect(tileIdAt(-1, 0)).toBe('tile_-1_0');
    expect(tileIdAt(-50, 0)).toBe('tile_-1_0');
    expect(tileIdAt(-99.999, 0)).toBe('tile_-1_0');
    // 边界 x=-100：Math.floor(-1) = -1 仍是 tile_-1_0；越界要更负
    expect(tileIdAt(-100, 0)).toBe('tile_-1_0');
    expect(tileIdAt(-100.001, 0)).toBe('tile_-2_0');
  });

  it('uses TILE_SIZE constant (not 50/200 hardcoded)', () => {
    expect(TILE_SIZE).toBe(100);
  });
});

describe('worldToSvgY', () => {
  it('flips sign (world y up → svg y down)', () => {
    expect(worldToSvgY(50)).toBe(-50);
    expect(worldToSvgY(0)).toBe(-0); // 0 is its own negation
    expect(worldToSvgY(-50)).toBe(50);
  });
});
