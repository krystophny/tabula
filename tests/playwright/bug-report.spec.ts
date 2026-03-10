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
    expect(String(request.device?.ua || '')).not.toBe('');
    expect(String(request.device?.platform || '')).not.toBe('');
    expect(String(request.device?.screen || '')).toMatch(/^\d+x\d+$/);
    expect(String(request.device?.timezone || '')).not.toBe('');
    expect(Number.isFinite(Number(request.device?.hardware_concurrency))).toBe(true);
    expect(Number.isFinite(Number(request.device?.max_touch_points))).toBe(true);
    await expect(page.locator('#bug-report-sheet')).toBeHidden();
    await expect(page.locator('#canvas-text')).toContainText('Bug report filed');
    await expect(page.locator('#canvas-text')).toContainText('#77');
  });

  test('filed bug reports appear in inbox immediately', async ({ page }) => {
    await waitReady(page);

    await page.locator('#edge-left-tap').click();
    await expect(page.locator('#pr-file-list')).toContainText('Review parser cleanup');

    await page.locator('#bug-report-button').click();
    await page.locator('#bug-report-note').fill('Harness inbox refresh');
    await page.locator('#bug-report-save').click();

    await expect(page.locator('#pr-file-list')).toContainText('Bug report: Harness repro');
    await expect(page.locator('#edge-left-tap')).toHaveAttribute('data-inbox-count', '3');
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

  test('firefox-bug-report uses the browser-safe preview on Firefox', async ({ page, browserName }) => {
    test.skip(browserName !== 'firefox', 'Firefox-only regression coverage');
    await waitReady(page);

    await page.evaluate(() => {
      (window as any).__taburaBugReportTestEnv = {};
    });

    await page.locator('#bug-report-button').click();
    await expect(page.locator('#bug-report-sheet')).toBeVisible();
    await expect(page.locator('.bug-report-sheet__preview')).toHaveAttribute('data-capture-mode', 'fallback-firefox');

    const preview = await page.evaluate(async () => {
      const img = document.getElementById('bug-report-preview') as HTMLImageElement | null;
      if (!img) return null;
      if (!img.complete) await img.decode();
      const canvas = document.createElement('canvas');
      canvas.width = img.naturalWidth || 1;
      canvas.height = img.naturalHeight || 1;
      const ctx = canvas.getContext('2d');
      if (!ctx) return null;
      ctx.drawImage(img, 0, 0);
      const pixel = Array.from(ctx.getImageData(20, 20, 1, 1).data);
      return {
        src: img.src,
        mode: img.dataset.captureMode || '',
        pixel,
      };
    });

    expect(preview).not.toBeNull();
    expect(preview?.mode).toBe('fallback-firefox');
    expect(String(preview?.src || '')).toContain('data:image/png;base64,');
    expect(preview?.pixel?.[0]).not.toBe(255);
    expect(preview?.pixel?.[1]).not.toBe(255);
    expect(preview?.pixel?.[2]).not.toBe(255);
  });
});
