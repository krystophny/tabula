import { expect, test, type Page } from '@playwright/test';
import { assertCircleReadable } from './slopshell-circle-helpers';

async function clearLog(page: Page) {
  await page.evaluate(() => {
    (window as any).__harnessLog.splice(0);
  });
}

async function getLog(page: Page) {
  return page.evaluate(() => (window as any).__harnessLog.slice());
}

async function waitForAppReady(page: Page) {
  await page.waitForFunction(() => {
    const app = (window as any)._slopshellApp;
    if (typeof app?.getState !== 'function') return false;
    const s = app.getState();
    return s.chatWs && s.chatWs.readyState === (window as any).WebSocket.OPEN;
  }, null, { timeout: 5_000 });
  await page.waitForTimeout(200);
}

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/harness.html');
  await waitForAppReady(page);
}

async function switchToTestProject(page: Page) {
  await page.evaluate(() => {
    const buttons = Array.from(document.querySelectorAll('#edge-top-projects .edge-project-btn'));
    const button = buttons.find((node) => node.textContent?.trim().toLowerCase() === 'test');
    if (button instanceof HTMLButtonElement) button.click();
  });
  await expect.poll(async () => page.evaluate(() => {
    const app = (window as any)._slopshellApp;
    const state = app?.getState?.();
    if (String(state?.activeWorkspaceId || '') !== 'test') return 'switching';
    return state?.chatWs?.readyState === (window as any).WebSocket.OPEN ? 'ready' : 'waiting';
  })).toBe('ready');
}

async function openCircle(page: Page) {
  await page.evaluate(() => {
    const button = document.getElementById('slopshell-circle-dot');
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error('slopshell circle dot not found');
    }
    button.click();
  });
  await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-state', 'expanded');
}

async function clickSegment(page: Page, segment: string) {
  await page.evaluate((name) => {
    const button = document.getElementById(`slopshell-circle-segment-${name}`);
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

test('top panel keeps summary only while Slopshell Circle owns live controls', async ({ page }) => {
  await expect(page.locator('#slopshell-circle-dot')).toBeVisible();
  await expect(page.locator('#edge-top-models')).toHaveAttribute('aria-label', 'Workspace runtime summary');
  await expect(page.locator('#edge-top-models .edge-live-status')).toContainText('Manual');
  await expect(page.locator('#edge-top-models button')).toHaveCount(0);
});

test('circle segments switch tools without using the top panel', async ({ page }) => {
  await openCircle(page);
  await clickSegment(page, 'ink');
  await expect(page.locator('#slopshell-circle-segment-ink')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#slopshell-circle-dot')).toHaveAttribute('data-tool', 'ink');
});

test('circle keeps live mode and tool mode visibly separate', async ({ page }) => {
  await expect(page.locator('#slopshell-circle-dot')).toHaveAttribute('aria-label', /Live mode: Manual/);
  await expect(page.locator('#slopshell-circle-dot .slopshell-circle-dot-badge')).toHaveText('Manual');

  await openCircle(page);
  await expect(page.locator('#slopshell-circle-segment-dialogue')).toHaveAttribute('aria-label', 'Live mode: Dialogue');
  await expect(page.locator('#slopshell-circle-segment-prompt')).toHaveAttribute('aria-label', 'Tool: Prompt');

  await clickSegment(page, 'dialogue');
  await expect(page.locator('#slopshell-circle-dot')).toHaveAttribute('aria-label', /Live mode: Dialogue/);
  await expect(page.locator('#slopshell-circle-dot .slopshell-circle-dot-badge')).toHaveText('Dialogue');
});

test('silent stays independent from tool selection', async ({ page }) => {
  await openCircle(page);
  await clickSegment(page, 'silent');
  await expect(page.locator('#slopshell-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');

  await clickSegment(page, 'pointer');
  await expect(page.locator('#slopshell-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#slopshell-circle-segment-pointer')).toHaveAttribute('aria-pressed', 'true');
});

test('fast stays orthogonal and surfaces LOCAL plus FAST in runtime summary', async ({ page }) => {
  await page.evaluate(() => {
    (window as any).__setProjects([
      {
        id: 'test',
        name: 'Test',
        kind: 'managed',
        sphere: 'private',
        workspace_path: '/tmp/test',
        root_path: '/tmp/test',
        chat_session_id: 'chat-1',
        canvas_session_id: 'local',
        chat_mode: 'chat',
        chat_model: 'local',
        chat_model_reasoning_effort: 'none',
        unread: false,
        review_pending: false,
        run_state: { active_turns: 0, queued_turns: 0, is_working: false, status: 'idle' },
      },
    ], 'test');
  });
  await switchToTestProject(page);
  await clearLog(page);

  await expect(page.locator('#edge-top-models .edge-runtime-chip')).toHaveText(['LOCAL', 'Voice']);

  await openCircle(page);
  await clickSegment(page, 'fast');
  await expect(page.locator('#slopshell-circle-segment-fast')).toHaveAttribute('aria-pressed', 'true');

  await clickSegment(page, 'pointer');
  await expect(page.locator('#slopshell-circle-segment-fast')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#slopshell-circle-segment-pointer')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('#edge-top-models .edge-runtime-chip')).toHaveText(['LOCAL', 'FAST', 'Voice']);

  const log = await getLog(page);
  expect(log.some((entry: any) => entry?.type === 'api_fetch'
    && entry?.action === 'runtime_preferences'
    && entry?.payload?.fast_mode === true)).toBe(true);
});

test('yolo and silent toggles update runtime summary and survive reload', async ({ page }) => {
  await clearLog(page);

  await page.evaluate(async () => {
    const mod = await import('/internal/web/static/app-tts.js');
    await mod.setYoloMode(true);
  });
  await expect(page.locator('#edge-top-models .edge-runtime-chip')).toContainText(['LOCAL', 'YOLO', 'Voice']);

  await openCircle(page);
  await clickSegment(page, 'silent');
  await expect(page.locator('#slopshell-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');
  await expect(page.locator('body')).toHaveClass(/silent-mode/);
  await expect(page.locator('#edge-top-models .edge-runtime-chip')).toContainText(['LOCAL', 'YOLO', 'Silent']);

  const log = await getLog(page);
  expect(log.some((entry: any) => entry?.type === 'api_fetch'
    && entry?.action === 'runtime_yolo'
    && entry?.payload?.safety_yolo_mode === true)).toBe(true);
  expect(log.some((entry: any) => entry?.type === 'api_fetch'
    && entry?.action === 'runtime_preferences'
    && entry?.payload?.silent_mode === true)).toBe(true);

  await page.reload();
  await waitForAppReady(page);
  await switchToTestProject(page);

  await expect(page.locator('#edge-top-models .edge-runtime-chip')).toContainText(['LOCAL', 'YOLO', 'Silent']);
  await expect(page.locator('#slopshell-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');
  const persisted = await page.evaluate(() => (window as any).__getRuntimeState?.());
  expect(persisted?.safety_yolo_mode).toBe(true);
  expect(persisted?.silent_mode).toBe(true);
});

test('corner placement persists across reloads', async ({ page }) => {
  await page.locator('#edge-top-tap').click();
  await page.locator('#slopshell-circle-corner-controls [data-corner="top_left"]').click();
  await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-corner', 'top_left');

  await page.reload();
  await waitReady(page);
  await switchToTestProject(page);

  await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-corner', 'top_left');
});

test('circle stays readable in every corner on desktop and mobile viewports', async ({ page }) => {
  const viewports = [
    { width: 1280, height: 720 },
    { width: 390, height: 844 },
  ];
  const corners = ['top_left', 'top_right', 'bottom_left', 'bottom_right'];

  for (const viewport of viewports) {
    await page.setViewportSize(viewport);
    await waitReady(page);
    await switchToTestProject(page);

    for (const corner of corners) {
      await page.evaluate(async (nextCorner) => {
        const mod = await import('/internal/web/static/app-slopshell-circle.js');
        mod.setSlopshellCircleCorner(nextCorner);
      }, corner);
      await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-corner', corner);
      await assertCircleReadable(page);
    }
  }
});

test.describe('mobile hit targets', () => {
  test.use({ viewport: { width: 390, height: 844 }, hasTouch: true, isMobile: true });

  test('touch taps reach live, silent, fast, and tool segments', async ({ page }) => {
    await waitReady(page);
    await switchToTestProject(page);
    await clearLog(page);

    await page.locator('#slopshell-circle-dot').tap();
    await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-state', 'expanded');

    await page.locator('#slopshell-circle-segment-dialogue').tap();
    await expect(page.locator('#slopshell-circle-segment-dialogue')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#edge-top-models .edge-live-status')).toContainText('Dialogue');

    await page.locator('#slopshell-circle-segment-meeting').tap();
    await expect(page.locator('#slopshell-circle-segment-meeting')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#edge-top-models .edge-live-status')).toContainText('Meeting');

    await page.locator('#slopshell-circle-segment-silent').tap();
    await expect(page.locator('#slopshell-circle-segment-silent')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#slopshell-circle-segment-fast').tap();
    await expect(page.locator('#slopshell-circle-segment-fast')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#slopshell-circle-segment-ink').tap();
    await expect(page.locator('#slopshell-circle-segment-ink')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#slopshell-circle-dot')).toHaveAttribute('data-tool', 'ink');

    const log = await getLog(page);
    expect(log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'live_policy' && entry?.payload?.policy === 'meeting')).toBe(true);
    expect(log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'runtime_preferences' && entry?.payload?.silent_mode === true)).toBe(true);
    expect(log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'runtime_preferences' && entry?.payload?.fast_mode === true)).toBe(true);
  });
});
