import { expect, test, type Page } from '@playwright/test';

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await page.waitForFunction(() => {
    const app = (window as any)._slopshellApp;
    if (typeof app?.getState !== 'function') return false;
    const s = app.getState();
    const wsOpen = (window as any).WebSocket.OPEN;
    return s.chatWs?.readyState === wsOpen && s.canvasWs?.readyState === wsOpen;
  }, null, { timeout: 8_000 });
}

async function openInbox(page: Page) {
  await page.locator('#edge-left-tap').click();
  await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
  await page.locator('.sidebar-tab', { hasText: 'Inbox' }).click();
  await expect(page.locator('.sidebar-tab.is-active')).toContainText('Inbox');
}

test.describe('review dispatch', () => {
  test('dispatches PR review from the item sidebar menu', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);

    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [{
          id: 401,
          title: 'Review parser cleanup',
          state: 'inbox',
          sphere: 'private',
          workspace_id: 11,
          source: 'github',
          source_ref: 'owner/tabula#PR-21',
          artifact_id: 601,
          artifact_title: 'PR #21',
          artifact_kind: 'github_pr',
          created_at: '2026-03-10 09:00:00',
          updated_at: '2026-03-10 09:05:00',
        }],
        waiting: [],
        someday: [],
        done: [],
      });
      (window as any).__setItemSidebarWorkspaces([
        { id: 11, name: 'Repo', sphere: 'private' },
      ]);
    });

    await openInbox(page);

    const row = page.locator('#pr-file-list .pr-file-item[data-item-id="401"]');
    await row.click({ button: 'right' });
    await expect(page.locator('#item-sidebar-menu')).toContainText('Review...');
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'Review...' }).click();
    await expect(page.locator('#item-sidebar-menu')).toContainText('GitHub Reviewer...');
    page.once('dialog', (dialog) => dialog.accept('octocat'));
    await page.locator('#item-sidebar-menu .item-sidebar-menu-item', { hasText: 'GitHub Reviewer...' }).click();

    await expect(page.locator('#status-label')).toHaveText('review dispatched: github:octocat');
    await expect(page.locator('#pr-file-list')).not.toContainText('Review parser cleanup');
    await page.locator('.sidebar-tab', { hasText: 'Waiting' }).click();
    await expect(page.locator('#pr-file-list')).toContainText('Review parser cleanup');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.action === 'dispatch_review' && entry?.payload?.target === 'github' && entry?.payload?.reviewer === 'octocat');
    }).toBe(true);
  });

  test('renders and resolves source drift without workspace/project conflation', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);

    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [],
        waiting: [],
        someday: [],
        done: [],
        review: [{
          id: 742,
          drift_id: 9742,
          title: 'Renew lab access',
          kind: 'drift',
          state: 'review',
          local_state: 'waiting',
          upstream_state: 'done',
          local_title: 'Renew lab access',
          upstream_title: 'Renew lab access upstream',
          source_binding: 'todoist:task:task-742',
          source_container: 'Admin',
          project_item_links: ['Onboarding cleanup (support)'],
          detected_at: '2026-03-10T09:00:00Z',
          sphere: 'private',
        }],
      });
      (window as any).__itemSidebarSectionCounts = { drift_review: 1 };
    });

    await page.locator('#edge-left-tap').click();
    await page.locator('#sidebar-secondary-toggle').click();
    await page.locator('.sidebar-secondary-row[data-section-id="drift"]').click();

    const row = page.locator('#pr-file-list .pr-file-item[data-item-id="742"]');
    await expect(row).toContainText('local waiting');
    await expect(row).toContainText('upstream done');
    await expect(row).toContainText('todoist:task:task-742');
    await expect(row).toContainText('container Admin');
    await expect(row).toContainText('project item: Onboarding cleanup (support)');
    await expect(row).not.toContainText('Workspace');

    await row.locator('button', { hasText: 'Take upstream' }).click();
    await expect(page.locator('#status-label')).toHaveText('drift take upstream');
    await expect(page.locator('#pr-file-list')).not.toContainText('Renew lab access');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.action === 'drift_action' && entry?.payload?.action === 'take_upstream');
    }).toBe(true);
  });
});
