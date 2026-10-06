import { NextResponse } from 'next/server';
import { readFileSync, existsSync } from 'fs';
import { join } from 'path';
import { parseSagaToGraph } from '@/lib/saga_loader';

export const dynamic = 'force-dynamic';

/** Resolve the saga-scripts directory relative to the admin-portal cwd.
 *  Next.js runs the app from the repo root or apps/admin-portal depending on
 *  dev/start mode, so we check both candidate roots.
 */
function resolveSagaDir(): string | null {
  const cwd = process.cwd();
  const candidates = [
    join(cwd, 'packages', 'saga-scripts'),
    join(cwd, '..', '..', 'packages', 'saga-scripts'),
  ];
  for (const p of candidates) {
    if (existsSync(p)) return p;
  }
  return null;
}

/** Sanitize the saga name param to prevent path traversal. */
function safeName(name: string): string | null {
  // Reject any path separators, parent refs, or empty strings
  if (!name) return null;
  if (name.includes('/') || name.includes('\\') || name.includes('..')) {
    return null;
  }
  // Allow only alphanumeric, underscore, dash, dot
  if (!/^[A-Za-z0-9_.-]+$/.test(name)) return null;
  return name;
}

/** GET /api/sagas/[name] — return parsed graph for one saga
 *  Params: name = saga file stem (e.g. "welcome_3npc" without .yaml)
 *  Returns: SagaGraph JSON
 */
export async function GET(
  _req: Request,
  { params }: { params: Promise<{ name: string }> },
) {
  const { name } = await params;
  const safe = safeName(name);
  if (!safe) {
    return NextResponse.json(
      { error: 'invalid saga name' },
      { status: 400 },
    );
  }

  const dir = resolveSagaDir();
  if (!dir) {
    return NextResponse.json(
      { error: 'saga-scripts directory not found' },
      { status: 500 },
    );
  }

  // Try .yaml then .yml
  const candidates = [join(dir, `${safe}.yaml`), join(dir, `${safe}.yml`)];
  const found = candidates.find((p) => existsSync(p));
  if (!found) {
    return NextResponse.json(
      { error: `saga not found: ${safe}` },
      { status: 404 },
    );
  }

  let content: string;
  try {
    content = readFileSync(found, 'utf-8');
  } catch (err) {
    return NextResponse.json(
      {
        error: `failed to read saga file: ${
          err instanceof Error ? err.message : String(err)
        }`,
      },
      { status: 500 },
    );
  }

  try {
    const graph = parseSagaToGraph(content);
    return NextResponse.json(graph);
  } catch (err) {
    return NextResponse.json(
      {
        error: `failed to parse saga: ${
          err instanceof Error ? err.message : String(err)
        }`,
      },
      { status: 400 },
    );
  }
}
