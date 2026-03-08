import { expect, test, type Page } from '@playwright/test';

async function waitWsReady(page: Page) {
  await page.waitForFunction(() => {
    const app = (window as any)._taburaApp;
    if (typeof app?.getState !== 'function') return false;
    const s = app.getState();
    return s.chatWs && s.chatWs.readyState === (window as any).WebSocket.OPEN;
  }, null, { timeout: 5_000 });
}

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await waitWsReady(page);
}

test.describe('bug report flow', () => {
  test('floating button captures a bundle with notes and annotations', async ({ page }) => {
    await waitReady(page);

    await expect(page.locator('#bug-report-button')).toBeVisible();
    await page.locator('#bug-report-button').click();
    await expect(page.locator('#bug-report-sheet')).toBeVisible();

    await page.locator('#bug-report-note').fill('The edge indicator froze after the second tap.');

    const canvas = page.locator('#bug-report-ink');
    const box = await canvas.boundingBox();
    expect(box).not.toBeNull();
    if (!box) return;
    await page.mouse.move(box.x + 30, box.y + 30);
    await page.mouse.down();
    await page.mouse.move(box.x + 120, box.y + 90);
    await page.mouse.up();

    await page.locator('#bug-report-save').click();

    await expect.poll(async () => {
      return page.evaluate(() => (window as any).__bugReportRequests.length);
    }).toBe(1);

    const request = await page.evaluate(() => (window as any).__bugReportRequests[0]);
    expect(request.trigger).toBe('button');
    expect(request.note).toContain('indicator froze');
    expect(String(request.screenshot_data_url || '')).toContain('data:image/png;base64,');
    expect(String(request.annotated_data_url || '')).toContain('data:image/png;base64,');
    expect(Array.isArray(request.recent_events)).toBe(true);
    expect(request.recent_events.length).toBeGreaterThan(0);
    expect(Array.isArray(request.browser_logs)).toBe(true);
    await expect(page.locator('#bug-report-sheet')).toBeHidden();
    await expect(page.locator('#canvas-text')).toContainText('Bug report filed');
    await expect(page.locator('#canvas-text')).toContainText('#77');
  });

  test('keyboard shortcut opens the bug report sheet', async ({ page }) => {
    await waitReady(page);

    await page.keyboard.down('Control');
    await page.keyboard.down('Alt');
    await page.keyboard.press('b');
    await page.keyboard.up('Alt');
    await page.keyboard.up('Control');

    await expect(page.locator('#bug-report-sheet')).toBeVisible();
    await page.locator('#bug-report-cancel').click();
    await expect(page.locator('#bug-report-sheet')).toBeHidden();
  });

  test('voice trigger phrase opens bug capture instead of sending chat', async ({ page }) => {
    await waitReady(page);

    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/app-chat-transport.js');
      await mod.submitMessage('report bug', { kind: 'voice_transcript' });
    });

    await expect(page.locator('#bug-report-sheet')).toBeVisible();
    const sentMessages = await page.evaluate(() => {
      return ((window as any).__harnessLog || []).filter((entry: any) => entry?.type === 'message_sent');
    });
    expect(sentMessages).toHaveLength(0);
  });

  test('two-finger hold opens the bug report sheet', async ({ page }) => {
    await waitReady(page);

    await page.evaluate(async () => {
      if (typeof Touch === 'undefined') return;
      const target = document.body;
      const first = new Touch({ identifier: 1, target, clientX: 40, clientY: 40, pageX: 40, pageY: 40 });
      const second = new Touch({ identifier: 2, target, clientX: 90, clientY: 40, pageX: 90, pageY: 40 });
      target.dispatchEvent(new TouchEvent('touchstart', {
        touches: [first, second],
        changedTouches: [first, second],
        bubbles: true,
      }));
      await new Promise((resolve) => setTimeout(resolve, 760));
      target.dispatchEvent(new TouchEvent('touchend', {
        touches: [],
        changedTouches: [first, second],
        bubbles: true,
      }));
    });

    await expect(page.locator('#bug-report-sheet')).toBeVisible();
  });
});
