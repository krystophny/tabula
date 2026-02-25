import { expect, test, type Page } from '@playwright/test';

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/zen-harness.html');
  await page.waitForFunction(() => {
    const app = (window as any)._taburaApp;
    if (typeof app?.getState !== 'function') return false;
    const s = app.getState();
    const wsOpen = (window as any).WebSocket.OPEN;
    return s.chatWs?.readyState === wsOpen && s.canvasWs?.readyState === wsOpen;
  }, null, { timeout: 8_000 });
}

async function injectCanvasEvent(page: Page, payload: Record<string, unknown>) {
  await page.evaluate((eventPayload) => {
    const app = (window as any)._taburaApp;
    if (typeof app?.getState !== 'function') {
      throw new Error('tabura app state unavailable');
    }
    const canvasWs = app.getState().canvasWs;
    if (!canvasWs || typeof canvasWs.injectEvent !== 'function') {
      throw new Error('canvas websocket not available in harness');
    }
    canvasWs.injectEvent(eventPayload);
  }, payload);
}

function twoFileDiff(): string {
  return [
    'diff --git a/docs/one.md b/docs/one.md',
    'index 1111111..2222222 100644',
    '--- a/docs/one.md',
    '+++ b/docs/one.md',
    '@@ -1 +1 @@',
    '-old',
    '+new',
    'diff --git a/src/two.js b/src/two.js',
    'index 3333333..4444444 100644',
    '--- a/src/two.js',
    '+++ b/src/two.js',
    '@@ -1 +1 @@',
    '-console.log("before");',
    '+console.log("after");',
  ].join('\n');
}

test.describe('pr review canvas mode', () => {
  test('shows changed file list and switches file diff on click', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);

    await injectCanvasEvent(page, {
      kind: 'text_artifact',
      event_id: 'evt-pr-1',
      title: '.tabura/artifacts/pr/pr-17.diff',
      text: twoFileDiff(),
    });

    await expect(page.locator('body')).toHaveClass(/pr-review-mode/);
    await expect(page.locator('#pr-file-list .pr-file-item')).toHaveCount(2);
    await expect(page.locator('#canvas-text')).toContainText('docs/one.md');

    await page.locator('#pr-file-list .pr-file-item').nth(1).click();
    await expect(page.locator('#pr-file-list .pr-file-item.is-active .pr-file-name')).toContainText('src/two.js');
    await expect(page.locator('#canvas-text')).toContainText('src/two.js');
  });

  test('supports keyboard file navigation', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);

    await injectCanvasEvent(page, {
      kind: 'text_artifact',
      event_id: 'evt-pr-2',
      title: '.tabura/artifacts/pr/pr-18.diff',
      text: twoFileDiff(),
    });
    await expect(page.locator('#pr-file-list .pr-file-item.is-active .pr-file-name')).toContainText('docs/one.md');

    await page.keyboard.press('ArrowRight');
    await expect(page.locator('#pr-file-list .pr-file-item.is-active .pr-file-name')).toContainText('src/two.js');

    await page.keyboard.press('ArrowLeft');
    await expect(page.locator('#pr-file-list .pr-file-item.is-active .pr-file-name')).toContainText('docs/one.md');
  });

  test('uses drawer-style file pane on mobile', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await waitReady(page);

    await injectCanvasEvent(page, {
      kind: 'text_artifact',
      event_id: 'evt-pr-3',
      title: '.tabura/artifacts/pr/pr-19.diff',
      text: twoFileDiff(),
    });

    const toggle = page.locator('#pr-file-drawer-toggle');
    await expect(toggle).toBeVisible();
    await page.evaluate(() => {
      (document.getElementById('pr-file-drawer-toggle') as HTMLButtonElement | null)?.click();
    });
    await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);

    await page.evaluate(() => {
      (document.getElementById('pr-file-drawer-backdrop') as HTMLDivElement | null)?.click();
    });
    await expect(page.locator('#pr-file-pane')).not.toHaveClass(/is-open/);
  });
});
