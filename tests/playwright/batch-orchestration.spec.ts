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

async function clearLog(page: Page) {
  await page.evaluate(() => {
    (window as any).__harnessLog.splice(0);
  });
}

async function injectChatEvent(page: Page, payload: Record<string, unknown>) {
  await page.evaluate((eventPayload) => {
    const app = (window as any)._taburaApp;
    const sessionId = String(app?.getState?.().chatSessionId || '');
    const sessions = (window as any).__mockWsSessions || [];
    const chatWs = sessions.find((ws: any) => typeof ws.url === 'string'
      && ws.url.includes('/ws/chat/')
      && (!sessionId || ws.url.includes(`/ws/chat/${sessionId}`)));
    if (chatWs?.injectEvent) {
      chatWs.injectEvent(eventPayload);
    }
  }, payload);
}

async function openInbox(page: Page) {
  await page.locator('#edge-left-tap').click();
  await expect(page.locator('#pr-file-pane')).toHaveClass(/is-open/);
  await page.locator('.sidebar-tab', { hasText: 'Inbox' }).click();
  await expect(page.locator('.sidebar-tab.is-active')).toContainText('Inbox');
}

test.describe('batch orchestration', () => {
  test('dialogue-triggered batch status and progress are surfaced in the UI', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await waitReady(page);

    await page.evaluate(() => {
      (window as any).__setItemSidebarData({
        inbox: [{
          id: 301,
          title: 'Fix parser cleanup',
          state: 'inbox',
          sphere: 'private',
          workspace_id: 11,
          source: 'github',
          source_ref: 'owner/tabula#12',
          created_at: '2026-03-10 09:00:00',
          updated_at: '2026-03-10 09:05:00',
        }],
        waiting: [],
        someday: [],
        done: [],
      });
    });

    await openInbox(page);
    await clearLog(page);

    const input = page.locator('#chat-pane-input');
    await input.fill('work through P0 issues');
    await input.press('Enter');

    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.type === 'message_sent' && entry?.text === 'work through P0 issues');
    }).toBe(true);

    await clearLog(page);

    await injectChatEvent(page, { type: 'turn_started', turn_id: 'batch-turn-1' });
    await injectChatEvent(page, {
      type: 'system_action',
      action: {
        type: 'batch_status',
        workspace_id: 11,
        item_count: 3,
        batch: {
          id: 7,
          workspace_id: 11,
          started_at: '2026-03-10T10:00:00Z',
          status: 'running',
          config_json: '{}',
        },
      },
    });
    await injectChatEvent(page, {
      type: 'assistant_message',
      turn_id: 'batch-turn-1',
      message: 'Started batch for workspace Repo with 3 open item(s).',
    });
    await injectChatEvent(page, {
      type: 'assistant_output',
      role: 'assistant',
      turn_id: 'batch-turn-1',
      message: 'Started batch for workspace Repo with 3 open item(s).',
    });

    await expect(page.locator('#status-label')).toHaveText('batch running: 3 item(s)');
    await expect(page.locator('#chat-history')).toContainText('Started batch for workspace Repo with 3 open item(s).');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'item_list' && String(entry?.payload?.view || '') === 'inbox');
    }).toBe(true);

    await clearLog(page);

    await injectChatEvent(page, {
      type: 'batch_progress',
      batch_id: 7,
      workspace_id: 11,
      batch: {
        id: 7,
        workspace_id: 11,
        started_at: '2026-03-10T10:00:00Z',
        status: 'running',
        config_json: '{}',
      },
      item: {
        batch_id: 7,
        item_id: 301,
        item_title: 'Fix parser cleanup',
        status: 'failed',
        error_msg: 'tests failed',
      },
    });

    await expect(page.locator('#chat-history')).toContainText('Batch update: Fix parser cleanup failed: tests failed.');
    await expect(page.locator('#status-label')).toHaveText('batch running');
    await expect.poll(async () => {
      const log = await page.evaluate(() => (window as any).__harnessLog || []);
      return log.some((entry: any) => entry?.type === 'api_fetch' && entry?.action === 'item_list' && String(entry?.payload?.view || '') === 'inbox');
    }).toBe(true);

    await injectChatEvent(page, {
      type: 'batch_progress',
      batch_id: 7,
      workspace_id: 11,
      batch: {
        id: 7,
        workspace_id: 11,
        started_at: '2026-03-10T10:00:00Z',
        finished_at: '2026-03-10T10:04:00Z',
        status: 'completed',
        config_json: '{}',
      },
    });

    await expect(page.locator('#status-label')).toHaveText('batch completed');
    await expect(page.locator('#chat-history')).toContainText('Batch update: batch completed.');
  });
});
