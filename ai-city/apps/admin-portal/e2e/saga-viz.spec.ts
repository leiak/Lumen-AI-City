import { test, expect } from '@playwright/test';

test.describe('Saga viz page', () => {
  test('renders welcome_3npc graph', async ({ page }) => {
    // Mock the /api/sagas list endpoint to avoid filesystem dependency
    await page.route('**/api/sagas', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([
          { name: 'welcome_3npc', saga_id: 'welcome_3npc', description: '3 NPC 协作欢迎新玩家' },
        ]),
      });
    });
    // Mock the /api/sagas/welcome_3npc detail endpoint with a known graph
    await page.route('**/api/sagas/welcome_3npc', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          saga_id: 'welcome_3npc',
          description: '3 NPC 协作欢迎新玩家',
          nodes: [
            { id: 'start', label: 'start', type: 'start' },
            { id: 'step_0', label: 'worker_a · say', type: 'forward', worker_id: 'worker_a', npc_id: 'npc_a_wang_boss', action: 'say', text: '欢迎来到 A 城！' },
            { id: 'step_1', label: 'worker_b · say', type: 'forward', worker_id: 'worker_b', npc_id: 'npc_b_grace_healer', action: 'say', text: '我是护士 grace，欢迎你！' },
            { id: 'step_2', label: 'worker_c · say', type: 'forward', worker_id: 'worker_c', npc_id: 'npc_a_book_keeper', action: 'say', text: '书店在城东，随时来坐坐。' },
            { id: 'comp_0', label: 'worker_a · revert_say', type: 'compensation', worker_id: 'worker_a', action: 'revert_say' },
            { id: 'comp_1', label: 'worker_b · revert_say', type: 'compensation', worker_id: 'worker_b', action: 'revert_say' },
            { id: 'comp_2', label: 'worker_c · revert_say', type: 'compensation', worker_id: 'worker_c', action: 'revert_say' },
            { id: 'end', label: 'end', type: 'end' },
          ],
          edges: [
            { id: 'e_start_step_0', source: 'start', target: 'step_0', kind: 'forward' },
            { id: 'e_step_0_step_1', source: 'step_0', target: 'step_1', kind: 'forward' },
            { id: 'e_step_1_step_2', source: 'step_1', target: 'step_2', kind: 'forward' },
            { id: 'e_step_2_end', source: 'step_2', target: 'end', kind: 'forward' },
            { id: 'e_step_0_comp_0', source: 'step_0', target: 'comp_0', kind: 'compensation' },
            { id: 'e_step_1_comp_1', source: 'step_1', target: 'comp_1', kind: 'compensation' },
            { id: 'e_step_2_comp_2', source: 'step_2', target: 'comp_2', kind: 'compensation' },
          ],
        }),
      });
    });

    await page.goto('/saga-viz');

    // Verify the dropdown is rendered
    await expect(page.getByTestId('saga-dropdown')).toBeVisible();

    // Verify no error state
    await expect(page.getByTestId('saga-error')).toHaveCount(0);

    // Select welcome_3npc from the dropdown
    await page.getByTestId('saga-dropdown').selectOption('welcome_3npc');

    // Wait for nodes to render (3 forward + 3 comp + start + end = 8 nodes)
    await expect(page.getByTestId('saga-node-worker_a · say')).toBeVisible();
    await expect(page.getByTestId('saga-node-worker_a · revert_say')).toBeVisible();

    // Verify metadata panel appears
    await expect(page.getByText('Saga 元数据')).toBeVisible();
    await expect(page.getByText('节点数: 8')).toBeVisible();
  });
});