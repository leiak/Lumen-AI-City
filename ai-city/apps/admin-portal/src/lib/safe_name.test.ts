import { describe, it, expect } from 'vitest';
import { safeName } from './safe_name';

describe('safeName', () => {
  it('accepts a typical NPC id', () => {
    expect(safeName('npc_wang_boss_001', 'npc_id')).toBe('npc_wang_boss_001');
  });

  it('accepts a typical tree name with dot+dash', () => {
    expect(safeName('tavern-greeting.v2', 'tree_name')).toBe(
      'tavern-greeting.v2',
    );
  });

  it('rejects empty string', () => {
    expect(() => safeName('', 'npc_id')).toThrow(/length/);
  });

  it('rejects path separators', () => {
    expect(() => safeName('a/b', 'npc_id')).toThrow(/invalid characters/);
    expect(() => safeName('a\\b', 'tree_name')).toThrow(/invalid characters/);
  });

  it('rejects parent traversal sequences (../ or ..\\)', () => {
    // Path traversal requires a path separator; the bare '..' token is
    // a valid single-name segment by itself. Only sequences that include
    // '/' or '\' (or encoded forms) should be rejected.
    expect(() => safeName('../etc', 'npc_id')).toThrow(/invalid characters/);
    expect(() => safeName('a..\\b', 'tree_name')).toThrow(/invalid characters/);
  });

  it('accepts bare ".." as a single safe name (saga-proxy semantics)', () => {
    // Matches the saga proxy's regex; downstream code is responsible for
    // not joining the name to a path separator.
    expect(safeName('..', 'npc_id')).toBe('..');
    expect(safeName('a..b', 'tree_name')).toBe('a..b');
  });

  it('rejects overlong npc_id (>64 chars)', () => {
    const long = 'a'.repeat(65);
    expect(() => safeName(long, 'npc_id')).toThrow(/length/);
  });

  it('accepts exactly 64-char npc_id', () => {
    const exact = 'a'.repeat(64);
    expect(safeName(exact, 'npc_id')).toBe(exact);
  });

  it('rejects overlong tree_name (>128 chars)', () => {
    const long = 'a'.repeat(129);
    expect(() => safeName(long, 'tree_name')).toThrow(/length/);
  });

  it('rejects spaces and special characters', () => {
    expect(() => safeName('has space', 'tree_name')).toThrow(/invalid characters/);
    expect(() => safeName('weird!name', 'tree_name')).toThrow(/invalid characters/);
    expect(() => safeName('with:colon', 'tree_name')).toThrow(/invalid characters/);
  });

  it('accepts exactly 128-char tree_name', () => {
    const exact = 'a'.repeat(128);
    expect(safeName(exact, 'tree_name')).toBe(exact);
  });
});