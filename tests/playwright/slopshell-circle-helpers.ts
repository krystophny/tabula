import { expect, type Locator, type Page } from '@playwright/test';

export type SlopshellCircleSegment =
  | 'dialogue'
  | 'meeting'
  | 'silent'
  | 'fast'
  | 'prompt'
  | 'text_note'
  | 'pointer'
  | 'highlight'
  | 'ink';

const LIVE_STATUS_SELECTOR = '#edge-top-models .edge-live-status';

export function circleSegment(page: Page, segment: SlopshellCircleSegment): Locator {
  return page.locator(`#slopshell-circle-menu .slopshell-circle-segment[data-segment="${segment}"]`);
}

export async function waitForCircleControls(page: Page) {
  await expect(circleSegment(page, 'dialogue')).toBeAttached();
  await expect(circleSegment(page, 'meeting')).toBeAttached();
  await expect(circleSegment(page, 'silent')).toBeAttached();
  await expect(circleSegment(page, 'fast')).toBeAttached();
}

export async function assertCircleReadable(page: Page, tolerancePx = 1) {
  await openCircle(page);
  const metrics = await page.evaluate((tolerance) => {
    const ids = ['dialogue', 'meeting', 'silent', 'fast', 'prompt', 'text_note', 'pointer', 'highlight', 'ink'];
    const rects = ids.map((id) => {
      const node = document.querySelector(`#slopshell-circle-menu .slopshell-circle-segment[data-segment="${id}"]`);
      if (!(node instanceof HTMLElement)) {
        throw new Error(`missing circle segment: ${id}`);
      }
      const rect = node.getBoundingClientRect();
      return {
        id,
        left: rect.left,
        right: rect.right,
        top: rect.top,
        bottom: rect.bottom,
      };
    });
    const overlapPairs: Array<{ a: string; b: string; overlapX: number; overlapY: number }> = [];
    for (let i = 0; i < rects.length; i += 1) {
      for (let j = i + 1; j < rects.length; j += 1) {
        const a = rects[i]!;
        const b = rects[j]!;
        const overlapX = Math.min(a.right, b.right) - Math.max(a.left, b.left);
        const overlapY = Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top);
        if (overlapX > tolerance && overlapY > tolerance) {
          overlapPairs.push({ a: a.id, b: b.id, overlapX, overlapY });
        }
      }
    }
    const clipped = rects.filter((rect) => (
      rect.left < -tolerance
      || rect.top < -tolerance
      || rect.right > window.innerWidth + tolerance
      || rect.bottom > window.innerHeight + tolerance
    ));
    return { rects, overlapPairs, clipped };
  }, tolerancePx);
  expect(metrics.overlapPairs, JSON.stringify(metrics, null, 2)).toEqual([]);
  expect(metrics.clipped, JSON.stringify(metrics, null, 2)).toEqual([]);
}

export async function switchToWorkspace(page: Page, workspaceId: string) {
  await page.evaluate((targetWorkspaceId) => {
    const buttons = Array.from(document.querySelectorAll('#edge-top-projects .edge-project-btn'));
    const button = buttons.find((node) => node.textContent?.trim().toLowerCase() === targetWorkspaceId);
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error(`project button not found: ${targetWorkspaceId}`);
    }
    button.click();
  }, workspaceId);
  await expect.poll(async () => page.evaluate((targetWorkspaceId) => {
    const app = (window as any)._slopshellApp;
    const state = app?.getState?.();
    const wsOpen = (window as any).WebSocket.OPEN;
    if (String(state?.activeWorkspaceId || '') !== targetWorkspaceId) return '';
    return state?.chatWs?.readyState === wsOpen ? 'ready' : 'waiting';
  }, workspaceId)).toBe('ready');
}

export async function openCircle(page: Page) {
  if (await page.locator('#slopshell-circle').getAttribute('data-state') === 'expanded') {
    return;
  }
  await page.evaluate(() => {
    const button = document.getElementById('slopshell-circle-dot');
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error('slopshell circle dot not found');
    }
    button.click();
  });
  await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-state', 'expanded');
}

export async function closeCircle(page: Page) {
  if (await page.locator('#slopshell-circle').getAttribute('data-state') !== 'expanded') {
    return;
  }
  await page.evaluate(() => {
    const button = document.getElementById('slopshell-circle-dot');
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error('slopshell circle dot not found');
    }
    button.click();
  });
  await expect(page.locator('#slopshell-circle')).toHaveAttribute('data-state', 'collapsed');
}

export async function clickCircleSegment(page: Page, segment: SlopshellCircleSegment) {
  await openCircle(page);
  await page.evaluate((targetSegment) => {
    const button = document.querySelector(`#slopshell-circle-menu .slopshell-circle-segment[data-segment="${targetSegment}"]`);
    if (!(button instanceof HTMLButtonElement)) {
      throw new Error(`slopshell circle segment not found: ${targetSegment}`);
    }
    button.click();
  }, segment);
}

export async function setLiveMode(page: Page, mode: 'dialogue' | 'meeting') {
  await switchToWorkspace(page, 'test');
  await waitForCircleControls(page);
  const segment = circleSegment(page, mode);
  if (await segment.getAttribute('aria-pressed') !== 'true') {
    await clickCircleSegment(page, mode);
  }
  await expect(page.locator(LIVE_STATUS_SELECTOR)).toContainText(mode === 'meeting' ? 'Meeting' : 'Dialogue');
  await expect(segment).toHaveAttribute('aria-pressed', 'true');
}

export async function stopLiveMode(page: Page, mode: 'dialogue' | 'meeting') {
  await switchToWorkspace(page, 'test');
  await waitForCircleControls(page);
  const segment = circleSegment(page, mode);
  await page.evaluate(async () => {
    const mod = await import('/internal/web/static/app-workspace-runtime.js');
    await mod.deactivateLiveSession({ disableMeetingConfig: true, silent: true });
  });
  await expect(page.locator(LIVE_STATUS_SELECTOR)).toContainText('Manual');
  await expect(segment).toHaveAttribute('aria-pressed', 'false');
}

export async function setSilentMode(page: Page, enabled: boolean) {
  await waitForCircleControls(page);
  const segment = circleSegment(page, 'silent');
  const current = await segment.getAttribute('aria-pressed');
  const target = enabled ? 'true' : 'false';
  if (current !== target) {
    await clickCircleSegment(page, 'silent');
  }
  await expect(segment).toHaveAttribute('aria-pressed', target);
}
