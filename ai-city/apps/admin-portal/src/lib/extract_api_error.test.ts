import { describe, it, expect } from 'vitest';
import { extractApiError, toastKindForCode } from './extract_api_error';

describe('extractApiError', () => {
  it('unwraps {code, msg} envelopes from the proxy', () => {
    const raw = `simulate failed: 400 {"code":"R_020","msg":"tick_limit out of range"}`;
    expect(extractApiError(raw)).toBe('R_020: tick_limit out of range');
  });

  it('extracts R_019 (path validation) codes', () => {
    const raw = `BT API 422 {"code":"R_019","msg":"npc_id has invalid characters"}`;
    expect(extractApiError(raw)).toBe(
      'R_019: npc_id has invalid characters',
    );
  });

  it('extracts R_021 (body size) codes', () => {
    const raw = `BT API 413 {"code":"R_021","msg":"body too large: 300000 > 262144"}`;
    expect(extractApiError(raw)).toBe(
      'R_021: body too large: 300000 > 262144',
    );
  });

  it('falls back to the raw string when no envelope is present', () => {
    expect(extractApiError('NetworkError: failed to fetch')).toBe(
      'NetworkError: failed to fetch',
    );
    expect(extractApiError(new Error('boom'))).toBe('Error: boom');
  });
});

describe('toastKindForCode', () => {
  it('R_021 is a warning (size hint)', () => {
    expect(toastKindForCode('R_021')).toBe('warning');
  });

  it('R_019 / R_020 / R_PROXY are errors', () => {
    expect(toastKindForCode('R_019')).toBe('error');
    expect(toastKindForCode('R_020')).toBe('error');
    expect(toastKindForCode('R_PROXY')).toBe('error');
  });

  it('returns undefined for unknown codes', () => {
    expect(toastKindForCode('R_999')).toBeUndefined();
  });
});