import { expect, test, type Page } from '@playwright/test';

async function clearLog(page: Page) {
  await page.evaluate(() => {
    (window as any).__harnessLog.splice(0);
  });
}

async function getLog(page: Page) {
  return page.evaluate(() => (window as any).__harnessLog.slice());
}

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await page.waitForFunction(() => {
    const app = (window as any)._taburaApp;
    if (typeof app?.getState !== 'function') return false;
    const s = app.getState();
    return s.chatWs && s.chatWs.readyState === (window as any).WebSocket.OPEN;
  }, null, { timeout: 5_000 });
  await page.waitForTimeout(200);
}

async function switchToTestProject(page: Page) {
  await page.evaluate(() => {
    const buttons = Array.from(document.querySelectorAll('#edge-top-projects .edge-project-btn'));
    const button = buttons.find((node) => node.textContent?.trim().toLowerCase() === 'test');
    if (button instanceof HTMLButtonElement) button.click();
  });
  await expect.poll(async () => page.evaluate(() => {
    const app = (window as any)._taburaApp;
    const state = app?.getState?.();
    if (String(state?.activeWorkspaceId || '') !== 'test') return 'switching';
    return state?.chatWs?.readyState === (window as any).WebSocket.OPEN ? 'ready' : 'waiting';
  })).toBe('ready');
}

async function openCircle(page: Page) {
  await page.evaluate(() => {
    const button = document.getElementById('tabura-circle-dot');
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error('tabura circle dot not found');
    }
    button.click();
  });
  await expect(page.locator('#tabura-circle')).toHaveAttribute('data-state', 'expanded');
}

async function clickSegment(page: Page, segment: string) {
  await page.evaluate((name) => {
    const button = document.getElementById(`tabura-circle-segment-${name}`);
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error(`circle segment not found: ${name}`);
    }
    button.click();
  }, segment);
}

test.beforeEach(async ({ page }) => {
  await waitReady(page);
  await switchToTestProject(page);
});

test('top panel keeps summary only while Tabura Circle owns live controls', async ({ page }) => {
  await expect(page.locator('#tabura-circle-dot')).toBeVisible();
  await expect(page.locator('#edge-top-models')).toHaveAttribute('aria-label', 'Workspace runtime summary');
  await expect(page.locator('#edge-top-models .edge-live-status')).toContainText('Manual');
  await expect(page.locator('#edge-top-models button')).toHaveCount(0);
});

test('circle segments switch tools without using the top panel', async ({ page }) => {
  await openCircle(page);
  await clickSegment(page, 'ink');
  await expect(page.locator('#tabura-circle-segment-ink')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#tabura-circle-dot')).toHaveAttribute('data-tool', 'ink');
});

test('circle keeps live mode and tool mode visibly separate', async ({ page }) => {
  await expect(page.locator('#tabura-circle-dot')).toHaveAttribute('aria-label', /Live mode: Manual/);
  await expect(page.locator('#tabura-circle-dot .tabura-circle-dot-badge')).toHaveText('Manual');

  await openCircle(page);
  await expect(page.locator('#tabura-circle-segment-dialogue')).toHaveAttribute('aria-label', 'Live mode: Dialogue');
  await expect(page.locator('#tabura-circle-segment-prompt')).toHaveAttribute('aria-label', 'Tool: Prompt');

  await clickSegment(page, 'dialogue');
  await expect(page.locator('#tabura-circle-dot')).toHaveAttribute('aria-label', /Live mode: Dialogue/);
  await expect(page.locator('#tabura-circle-dot .tabura-circle-dot-badge')).toHaveText('Dialogue');
});

test('silent stays independent from tool selection', async ({ page }) => {
  await openCircle(page);
  await clickSegment(page, 'silent');
  await expect(page.locator('#tabura-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');

  await clickSegment(page, 'pointer');
  await expect(page.locator('#tabura-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#tabura-circle-segment-pointer')).toHaveAttribute('aria-pressed', 'true');
});

test('corner placement persists across reloads', async ({ page }) => {
  await page.locator('#edge-top-tap').click();
  await page.locator('#tabura-circle-corner-controls [data-corner="top_left"]').click();
  await expect(page.locator('#tabura-circle')).toHaveAttribute('data-corner', 'top_left');

  await page.reload();
  await waitReady(page);
  await switchToTestProject(page);

  await expect(page.locator('#tabura-circle')).toHaveAttribute('data-corner', 'top_left');
});

test.describe('mobile hit targets', () => {
  test.use({ viewport: { width: 375, height: 667 } });

  test('right edge strip does not steal live, silent, or tool taps', async ({ page }) => {
    await waitReady(page);
    await switchToTestProject(page);
    await clearLog(page);

    await page.locator('#tabura-circle-dot').click();
    await expect(page.locator('#tabura-circle')).toHaveAttribute('data-state', 'expanded');

    await page.locator('#tabura-circle-segment-meeting').click();
    await expect(page.locator('#tabura-circle-segment-meeting')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#edge-top-models .edge-live-status')).toContainText('Meeting');

    await page.locator('#tabura-circle-segment-silent').click();
    await expect(page.locator('#tabura-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#tabura-circle-segment-ink').click();
    await expect(page.locator('#tabura-circle-segment-ink')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#tabura-circle-dot')).toHaveAttribute('data-tool', 'ink');

    const log = await getLog(page);
    expect(log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'live_policy' && entry?.payload?.policy === 'meeting')).toBe(true);
    expect(log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'runtime_preferences' && entry?.payload?.silent_mode === true)).toBe(true);

  });
});
