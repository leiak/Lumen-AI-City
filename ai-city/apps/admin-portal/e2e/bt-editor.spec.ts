import { test, expect } from '@playwright/test';

/**
 * BT editor E2E — Phase C.3
 *
 * Mocks the 4 backend endpoints (list / get / save / simulate) so the test
 * runs without a live `bt-editor-api`. Mirrors the saga-viz.spec.ts pattern
 * (page.route() mocking).
 */

const NPC = 'npc_wang_boss_001';
const TREE = 'tavern_greeting';

const SIMPLE_TREE_JSON = {
  version: '1.0.0',
  name: TREE,
  description: '酒馆欢迎树',
  root: {
    id: 'root',
    type: 'sequence',
    name: 'Greeting',
    children: [
      { id: 'c1', type: 'condition', expression: 'player.distance_to(npc) < 10' },
      { id: 'c2', type: 'action', action: 'say', args: { text: '欢迎来到酒馆！' } },
    ],
  },
};

test.describe('BT editor page', () => {
  test('loads NPC list, renders Monaco + graph, saves and simulates', async ({
    page,
  }) => {
    // ---- Mock list endpoint ----
    await page.route('**/api/bt/*', async (route) => {
      const url = route.request().url();
      // Match the LIST endpoint (no /simulate, no second segment)
      // e.g. /api/bt/npc_wang_boss_001
      const listMatch = /\/api\/bt\/([^/]+)$/.exec(url);
      const detailMatch = /\/api\/bt\/([^/]+)\/([^/]+)$/.exec(url);
      const simulateMatch = /\/api\/bt\/([^/]+)\/([^/]+)\/simulate$/.exec(url);

      if (simulateMatch) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            npc_id: NPC,
            name: TREE,
            final_status: 'success',
            trace: [
              { tick: 1, node_id: 'root', status: 'running', message: 'enter sequence' },
              { tick: 1, node_id: 'c1', status: 'success', message: 'condition ok' },
              { tick: 1, node_id: 'c2', status: 'success', message: 'said greeting' },
            ],
          }),
        });
        return;
      }

      if (detailMatch) {
        const [, npcId, treeName] = detailMatch;
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            npc_id: npcId,
            name: treeName,
            tree_json: SIMPLE_TREE_JSON,
            version: 1,
            created_at: '2026-10-06T00:00:00Z',
            updated_at: '2026-10-06T00:00:00Z',
          }),
        });
        return;
      }

      if (listMatch) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify([
            { name: TREE, version: 1, updated_at: '2026-10-06T00:00:00Z' },
          ]),
        });
        return;
      }

      // POST save (detail URL with POST method)
      if (route.request().method() === 'POST') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({
            npc_id: NPC,
            name: TREE,
            tree_json: SIMPLE_TREE_JSON,
            version: 2,
            created_at: '2026-10-06T00:00:00Z',
            updated_at: '2026-10-06T00:00:01Z',
          }),
        });
        return;
      }

      await route.continue();
    });

    await page.goto('/bt-editor');

    // Header rendered
    await expect(page.getByRole('heading', { name: 'BT 编辑器' })).toBeVisible();

    // NPC + Tree dropdowns present
    const npcDropdown = page.getByTestId('bt-npc-dropdown');
    const treeDropdown = page.getByTestId('bt-tree-dropdown');
    await expect(npcDropdown).toBeVisible();
    await expect(treeDropdown).toBeVisible();

    // Wait for tree list to populate (autoload on mount)
    await expect(treeDropdown.locator('option').nth(1)).toHaveText(TREE);

    // Select NPC + Tree explicitly
    await npcDropdown.selectOption(NPC);
    await treeDropdown.selectOption(TREE);

    // Monaco editor visible (it's a complex <div> tree under data-testid)
    await expect(page.getByTestId('bt-monaco')).toBeVisible();

    // Save button → POST mock → success toast
    const saveBtn = page.getByTestId('bt-save-btn');
    await expect(saveBtn).toBeVisible();
    await saveBtn.click();
    await expect(page.getByTestId('bt-toast')).toBeVisible({ timeout: 5000 });

    // Simulate button → POST mock → trace panel
    const simulateBtn = page.getByTestId('bt-simulate-btn');
    await expect(simulateBtn).toBeVisible();
    await simulateBtn.click();

    // Trace log should show at least one entry
    await expect(page.getByText(/trace log/i)).toBeVisible();
    await expect(page.getByText(/enter sequence/i)).toBeVisible({ timeout: 5000 });

    // No error state
    await expect(page.getByTestId('bt-error')).toHaveCount(0);
  });
});
