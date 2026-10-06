/** Extract a readable error message from a FastAPI `{"detail":{"code":..., "msg":...}}` envelope.
 *
 *  The bt-editor-api returns error envelopes shaped:
 *    { detail: { code: "R_019" | "R_020" | "R_021" | "R_PROXY", msg: "..." } }
 *
 *  The proxy routes in this app forward those envelopes verbatim. Without
 *  extraction we'd dump the full JSON to the toast and show
 *  `[object Object]`-style strings to the user.
 *
 *  Returns `${code}: ${msg}` for known envelopes, otherwise the raw
 *  stringified error.
 */
export function extractApiError(e: unknown): string {
  const raw = String(e);
  const m = raw.match(/\{"code":"(R_\d+)","msg":"([^"]+)"\}/);
  if (m) return `${m[1]}: ${m[2]}`;
  return raw;
}

/** Map an API error code to a toast severity. R_021 is a warning (size
 *  hint), the rest are errors. Returns undefined if the code is unknown. */
export function toastKindForCode(code: string): 'success' | 'error' | 'warning' | undefined {
  if (code === 'R_021') return 'warning';
  if (code === 'R_019' || code === 'R_020' || code === 'R_PROXY') return 'error';
  return undefined;
}