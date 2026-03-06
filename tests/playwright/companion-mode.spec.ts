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

test('workspace sidebar exposes companion transcript, summary, and references viewer entries', async ({ page }) => {
  await page.setViewportSize({ width: 1280, height: 800 });
  await waitReady(page);

  await page.locator('#edge-left-tap').click();
  await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
  await expect(page.locator('#pr-file-list')).toContainText('Companion Transcript');
  await expect(page.locator('#pr-file-list')).toContainText('Companion Summary');
  await expect(page.locator('#pr-file-list')).toContainText('Companion References');

  await page.getByRole('button', { name: 'Companion Transcript' }).click();
  await expect(page.locator('#canvas-text')).toContainText('Harness companion transcript');

  await page.getByRole('button', { name: 'Companion Summary' }).click();
  await expect(page.locator('#canvas-text')).toContainText('Harness companion summary');

  await page.getByRole('button', { name: 'Companion References' }).click();
  await expect(page.locator('#canvas-text')).toContainText('Acme');
  await expect(page.locator('#canvas-text')).toContainText('Budget');
});
