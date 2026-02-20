import { expect, test, type Page } from '@playwright/test';

type Header = {
  id: string;
  date: string;
  sender: string;
  subject: string;
};

type HarnessMessage = Record<string, unknown>;

function plainTextEvent(eventID: string, text: string) {
  return {
    kind: 'text_artifact',
    event_id: eventID,
    title: 'Notes',
    text,
    meta: {},
  };
}

function mailEvent(eventID: string, provider: string, headers: Header[]) {
  return {
    kind: 'text_artifact',
    event_id: eventID,
    title: 'Mail Headers',
    text: '# Mail Headers',
    meta: {
      producer_mcp_url: 'http://127.0.0.1:8090/mcp',
      message_triage_v1: {
        provider,
        folder: 'INBOX',
        count: headers.length,
        headers,
      },
    },
  };
}

function imageEvent(eventID: string) {
  return {
    kind: 'image_artifact',
    event_id: eventID,
    title: 'Image',
    path: 'missing.png',
  };
}

async function renderArtifact(page: Page, event: Record<string, unknown>) {
  await page.waitForFunction(() => typeof (window as any).renderHarnessArtifact === 'function');
  await page.evaluate((payload) => {
    // @ts-expect-error injected by harness module
    window.renderHarnessArtifact(payload);
  }, event);
}

async function clearHarnessMessages(page: Page) {
  await page.waitForFunction(() => typeof (window as any).clearHarnessMessages === 'function');
  await page.evaluate(() => {
    // @ts-expect-error injected by harness module
    window.clearHarnessMessages();
  });
}

async function getHarnessMessages(page: Page): Promise<HarnessMessage[]> {
  await page.waitForFunction(() => typeof (window as any).getHarnessMessages === 'function');
  return page.evaluate(() => {
    // @ts-expect-error injected by harness module
    return window.getHarnessMessages();
  });
}

async function selectTextFromSelector(page: Page, selector: string) {
  const selected = await page.evaluate((sel) => {
    const root = document.querySelector(sel);
    if (!root) return false;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node && !String(node.textContent || '').trim()) {
      node = walker.nextNode();
    }
    if (!node) return false;
    const text = String(node.textContent || '');
    const start = Math.max(0, text.search(/\S/));
    const range = document.createRange();
    range.setStart(node, start);
    range.setEnd(node, text.length);
    const selection = window.getSelection();
    if (!selection) return false;
    selection.removeAllRanges();
    selection.addRange(range);
    document.dispatchEvent(new Event('selectionchange'));
    return true;
  }, selector);
  if (!selected) throw new Error(`unable to select text from ${selector}`);
}

async function waitForLastSelectionMessage(page: Page): Promise<HarnessMessage> {
  await expect.poll(async () => {
    const messages = await getHarnessMessages(page);
    return messages.filter((m) => m.kind === 'text_selection').length;
  }).toBeGreaterThan(0);

  const messages = await getHarnessMessages(page);
  const selections = messages.filter((m) => m.kind === 'text_selection');
  return selections[selections.length - 1];
}

test.beforeEach(async ({ page }) => {
  await page.goto('/tests/playwright/harness.html');
  await clearHarnessMessages(page);
});

test('non-mail text artifacts enable review selection payloads', async ({ page }) => {
  await renderArtifact(page, plainTextEvent('evt-text-1', '# Header\nAlpha Beta'));
  await selectTextFromSelector(page, '#canvas-text');
  const msg = await waitForLastSelectionMessage(page);

  expect(msg.event_id).toBe('evt-text-1');
  expect(msg.artifact_id).toBe('evt-text-1');
  expect(String(msg.text || '')).toContain('Header');
  expect(Number(msg.line_start)).toBeGreaterThanOrEqual(1);
  expect(Number(msg.line_end)).toBeGreaterThanOrEqual(Number(msg.line_start));
});

test('mail text artifacts keep the same review selection behavior', async ({ page }) => {
  await page.route('**/api/mail/action-capabilities', async (route) => {
    await route.fulfill({
      json: {
        capabilities: {
          provider: 'gmail',
          supports_open: true,
          supports_archive: true,
          supports_delete_to_trash: true,
          supports_native_defer: true,
        },
      },
    });
  });

  await renderArtifact(page, mailEvent('evt-mail-1', 'gmail', [
    { id: 'm1', date: '2026-02-20T09:00:00Z', sender: 'a@example.com', subject: 'Quarterly Review' },
  ]));
  await selectTextFromSelector(page, 'tr[data-message-id="m1"] td:nth-child(3)');
  const msg = await waitForLastSelectionMessage(page);

  expect(msg.event_id).toBe('evt-mail-1');
  expect(msg.artifact_id).toBe('evt-mail-1');
  expect(String(msg.text || '')).toContain('Quarterly Review');
  expect(Number(msg.line_start)).toBeGreaterThanOrEqual(1);
});

test('switching artifacts tears down stale review and mail handlers', async ({ page }) => {
  await renderArtifact(page, mailEvent('evt-mail-2', 'gmail', [
    { id: 'm1', date: '2026-02-20T09:00:00Z', sender: 'a@example.com', subject: 'Switch Test' },
  ]));

  const before = await page.evaluate(() => {
    const root = document.getElementById('canvas-text') as any;
    return {
      hasSelectionHandler: Boolean(root?._selectionHandler),
      hasMailClickHandler: Boolean(root?._mailClickHandler),
      hasMailPointerDownHandler: Boolean(root?._mailPointerDownHandler),
      hasMailDetailKeyDownHandler: Boolean(root?._mailDetailKeyDownHandler),
      hasMailClass: root?.classList.contains('mail-artifact') || false,
    };
  });
  expect(before.hasSelectionHandler).toBe(true);
  expect(before.hasMailClickHandler).toBe(true);
  expect(before.hasMailPointerDownHandler).toBe(true);
  expect(before.hasMailClass).toBe(true);

  await clearHarnessMessages(page);
  await renderArtifact(page, imageEvent('evt-image-1'));

  const after = await page.evaluate(() => {
    const root = document.getElementById('canvas-text') as any;
    return {
      hasSelectionHandler: Boolean(root?._selectionHandler),
      hasMailClickHandler: Boolean(root?._mailClickHandler),
      hasMailPointerDownHandler: Boolean(root?._mailPointerDownHandler),
      hasMailDetailKeyDownHandler: Boolean(root?._mailDetailKeyDownHandler),
      hasMailClass: root?.classList.contains('mail-artifact') || false,
    };
  });
  expect(after.hasSelectionHandler).toBe(false);
  expect(after.hasMailClickHandler).toBe(false);
  expect(after.hasMailPointerDownHandler).toBe(false);
  expect(after.hasMailDetailKeyDownHandler).toBe(false);
  expect(after.hasMailClass).toBe(false);

  await page.evaluate(() => {
    const root = document.getElementById('canvas-text');
    if (!root) return;
    const cell = root.querySelector('tr[data-message-id="m1"] td');
    const text = cell?.firstChild;
    if (!text) return;
    const range = document.createRange();
    range.setStart(text, 0);
    range.setEnd(text, String(text.textContent || '').length);
    const selection = window.getSelection();
    if (!selection) return;
    selection.removeAllRanges();
    selection.addRange(range);
    document.dispatchEvent(new Event('selectionchange'));
  });
  await page.waitForTimeout(80);

  const messages = await getHarnessMessages(page);
  expect(messages).toHaveLength(0);
});
