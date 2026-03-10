import { expect, test, type Page } from '@playwright/test';

type HarnessLogEntry = { type: string; action?: string; [key: string]: unknown };

async function getLog(page: Page): Promise<HarnessLogEntry[]> {
  return page.evaluate(() => (window as any).__harnessLog.slice());
}

async function clearLog(page: Page) {
  await page.evaluate(() => { (window as any).__harnessLog.splice(0); });
}

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
  await page.waitForTimeout(250);
}

async function setDocumentHidden(page: Page, hidden: boolean) {
  await page.evaluate((nextHidden) => {
    const value = Boolean(nextHidden);
    const visibility = value ? 'hidden' : 'visible';
    Object.defineProperty(document, 'hidden', {
      configurable: true,
      get: () => value,
    });
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => visibility,
    });
    document.dispatchEvent(new Event('visibilitychange'));
  }, hidden);
}

async function isRuntimeWatcherScheduled(page: Page): Promise<boolean> {
  return page.evaluate(async () => {
    const mod = await import('../../internal/web/static/app-runtime-ui.js');
    return mod.isRuntimeReloadWatcherScheduled();
  });
}

async function isAssistantWatcherScheduled(page: Page): Promise<boolean> {
  return page.evaluate(() => {
    const app = (window as any)._taburaApp;
    return Boolean(app?.getState?.().assistantActivityTimer);
  });
}

test('background tabs stop polling until visible again', async ({ page }) => {
  await waitReady(page);

  await expect.poll(() => isRuntimeWatcherScheduled(page)).toBe(true);
  await expect.poll(() => isAssistantWatcherScheduled(page)).toBe(true);

  await clearLog(page);
  await setDocumentHidden(page, true);

  await expect.poll(() => isRuntimeWatcherScheduled(page)).toBe(false);
  await expect.poll(() => isAssistantWatcherScheduled(page)).toBe(false);

  await page.waitForTimeout(1_800);
  const hiddenLog = await getLog(page);
  expect(hiddenLog.filter((entry) => entry.type === 'api_fetch' && entry.action === 'runtime_meta')).toHaveLength(0);
  expect(hiddenLog.filter((entry) => entry.type === 'api_fetch' && entry.action === 'projects_activity')).toHaveLength(0);

  await clearLog(page);
  await setDocumentHidden(page, false);

  await expect.poll(() => isRuntimeWatcherScheduled(page)).toBe(true);
  await expect.poll(() => isAssistantWatcherScheduled(page)).toBe(true);
  await expect.poll(async () => {
    const log = await getLog(page);
    return {
      runtime: log.some((entry) => entry.type === 'api_fetch' && entry.action === 'runtime_meta'),
      activity: log.some((entry) => entry.type === 'api_fetch' && entry.action === 'projects_activity'),
    };
  }).toEqual({ runtime: true, activity: true });
});
