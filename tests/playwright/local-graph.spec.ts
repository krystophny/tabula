import { expect, test, type Page } from '@playwright/test';

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await page.waitForFunction(() => {
    const app = (window as any)._slopshellApp;
    if (typeof app?.getState !== 'function') return false;
    const state = app.getState();
    return state.chatWs && state.chatWs.readyState === (window as any).WebSocket.OPEN;
  }, null, { timeout: 5_000 });
}

async function seedBrainWorkspace(page: Page) {
  await page.evaluate(() => {
    (window as any).__setProjects([
      {
        id: 'brain',
        name: 'Brain',
        kind: 'linked',
        sphere: 'work',
        workspace_path: '/tmp/vault/brain',
        root_path: '/tmp/vault/brain',
        chat_session_id: 'chat-brain',
        canvas_session_id: 'brain',
        chat_mode: 'chat',
        chat_model: 'local',
        chat_model_reasoning_effort: 'none',
        unread: false,
        review_pending: false,
        run_state: { active_turns: 0, queued_turns: 0, is_working: false, status: 'idle' },
      },
    ], 'brain');
    const state = (window as any)._slopshellApp?.getState?.();
    if (state) {
      state.activeWorkspaceId = 'brain';
      state.projects = [
        {
          id: 'brain',
          name: 'Brain',
          sphere: 'work',
          workspace_path: '/tmp/vault/brain',
          root_path: '/tmp/vault/brain',
        },
      ];
    }
  });
  await page.waitForFunction(() => {
    const state = (window as any)._slopshellApp?.getState?.();
    return String(state?.activeWorkspaceId || '') === 'brain'
      && Array.isArray(state?.projects)
      && state.projects.some((project: any) => String(project?.root_path || '') === '/tmp/vault/brain');
  }, null, { timeout: 5_000 });
}

async function renderGraphArtifact(page: Page) {
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/canvas.js');
    (window as any).__canvasModule = mod;
    const state = (window as any)._slopshellApp?.getState?.();
    if (state) state.activeWorkspaceId = 'brain';
    (window as any).__mockWorkspaceLocalGraph = {
      ok: true,
      source_path: 'topics/active.md',
      nodes: [
        { id: 'note:topics/active.md', type: 'note', label: 'active', path: 'topics/active.md', file_url: '/api/workspaces/brain/markdown-link/file?path=brain%2Ftopics%2Factive.md', sphere: 'work' },
        { id: 'note:brain/topics/related.md', type: 'note', label: 'related', path: 'brain/topics/related.md', file_url: '/api/workspaces/brain/markdown-link/file?path=brain%2Ftopics%2Frelated.md', sphere: 'work' },
        { id: 'item:7', type: 'item', label: 'Follow up', source: 'todoist', sphere: 'work' },
        { id: 'source:todoist:task-7', type: 'source', label: 'task-7', source: 'todoist', sphere: 'work' },
      ],
      edges: [
        { source: 'note:topics/active.md', target: 'note:brain/topics/related.md', relation: 'markdown_link', label: 'markdown' },
        { source: 'item:7', target: 'source:todoist:task-7', relation: 'source_binding', label: 'todoist' },
      ],
    };
    (window as any).__mockMarkdownLinkFileText = '# Related\n\nOpened from graph';
    mod.renderCanvas({
      event_id: 'local-graph-active',
      kind: 'text_artifact',
      title: 'topics/active.md',
      path: 'topics/active.md',
      text: '[Related](related.md)',
    });
  });
}

test.beforeEach(async ({ page }) => {
  await waitReady(page);
  await seedBrainWorkspace(page);
});

test('local graph renders, filters, and opens nodes', async ({ page }) => {
  await renderGraphArtifact(page);

  await expect(page.locator('#canvas-markdown-link-panel .canvas-local-graph')).toBeVisible();
  await expect(page.locator('.canvas-local-graph-node', { hasText: 'active' })).toBeVisible();
  await expect(page.locator('.canvas-local-graph-node', { hasText: 'related' })).toBeVisible();

  await page.locator('.canvas-local-graph-controls input[name="source_filter"]').fill('todoist');
  await page.locator('.canvas-local-graph-controls input[name="label"]').fill('deep-work');
  await page.locator('.canvas-local-graph-controls button', { hasText: 'Apply' }).click();
  await expect.poll(async () => {
    const log = await page.evaluate(() => (window as any).__harnessLog.slice());
    return log.some((entry: any) => entry.action === 'local_graph'
      && String(entry.url || '').includes('source_filter=todoist')
      && String(entry.url || '').includes('label=deep-work'));
  }, { timeout: 5_000 }).toBe(true);

  await page.locator('.canvas-local-graph-node', { hasText: 'related' }).click();
  await expect(page.locator('#canvas-text')).toContainText('Opened from graph');

  await renderGraphArtifact(page);
  await page.locator('.canvas-local-graph-node', { hasText: 'task-7' }).click();
  await expect(page.locator('#canvas-text')).toContainText('Type: source');
  await expect(page.locator('#canvas-text')).toContainText('Source: todoist');
});
