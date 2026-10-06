/** Path-param sanitization helpers for admin-portal proxy routes.
 *
 *  Shared by the BT and saga proxy routes to fail fast on path traversal /
 *  oversized segments before forwarding to upstream services.
 *
 *  Pattern: must match `/^[A-Za-z0-9_.-]+$/` — no slashes, no parent refs.
 */

const SAFE_NAME = /^[A-Za-z0-9_.-]+$/;

export type SafeNameKind = 'npc_id' | 'tree_name' | 'saga_name';

const KIND_MAX: Record<SafeNameKind, number> = {
  npc_id: 64,
  tree_name: 128,
  saga_name: 128,
};

/** Throw `UnknownError` if `name` is not a safe single-name segment.
 *  Returns the name unchanged on success (so callers can use it verbatim). */
export function safeName(name: string, kind: SafeNameKind): string {
  const max = KIND_MAX[kind];
  if (name.length === 0 || name.length > max) {
    throw new Error(`${kind} length out of bounds (0 < len <= ${max})`);
  }
  if (!SAFE_NAME.test(name)) {
    throw new Error(
      `${kind} has invalid characters (allowed: A-Za-z0-9_.-)`,
    );
  }
  return name;
}