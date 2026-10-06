import { NextResponse } from 'next/server';
import { readFileSync, readdirSync, existsSync } from 'fs';
import { join } from 'path';
import yaml from 'js-yaml';

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

/** GET /api/sagas — list all saga scripts in packages/saga-scripts/
 *  Returns: [{ name: string, saga_id: string, description: string }, ...]
 *  Sorted by file name ascending.
 */
export async function GET() {
  const dir = resolveSagaDir();
  if (!dir) {
    return NextResponse.json(
      { error: 'saga-scripts directory not found' },
      { status: 500 },
    );
  }

  let files: string[];
  try {
    files = readdirSync(dir).filter((f) => f.endsWith('.yaml') || f.endsWith('.yml'));
  } catch (err) {
    return NextResponse.json(
      {
        error: `failed to read saga-scripts dir: ${
          err instanceof Error ? err.message : String(err)
        }`,
      },
      { status: 500 },
    );
  }

  const items: Array<{ name: string; saga_id: string; description: string }> = [];

  for (const file of files) {
    const fullPath = join(dir, file);
    const name = file.replace(/\.(ya?ml)$/i, '');
    try {
      const raw = readFileSync(fullPath, 'utf-8');
      const doc = yaml.load(raw);
      if (doc && typeof doc === 'object') {
        const d = doc as Record<string, unknown>;
        const sagaId =
          typeof d.saga_id === 'string' && d.saga_id.length > 0
            ? d.saga_id
            : name;
        const description =
          typeof d.description === 'string' ? d.description : '';
        items.push({ name, saga_id: sagaId, description });
      } else {
        // Skip non-mapping docs but include a placeholder so they show in the list
        items.push({ name, saga_id: name, description: '' });
      }
    } catch {
      // skip files that fail to parse — don't break the whole list
      continue;
    }
  }

  items.sort((a, b) => a.name.localeCompare(b.name));

  return NextResponse.json(items);
}
