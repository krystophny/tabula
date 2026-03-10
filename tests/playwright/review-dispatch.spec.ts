import { expect, test, type Page } from '@playwright/test';

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await page.waitForFunction(() => {
    const app = (window as any)._taburaApp;
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
});
