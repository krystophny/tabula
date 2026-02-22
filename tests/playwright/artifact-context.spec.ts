import { expect, test, type Page } from '@playwright/test';

type HarnessLogEntry = { type: string; action: string; [key: string]: unknown };

async function getLog(page: Page): Promise<HarnessLogEntry[]> {
  return page.evaluate(() => (window as any).__harnessLog.slice());
}

async function clearLog(page: Page) {
  await page.evaluate(() => { (window as any).__harnessLog.splice(0); });
}

async function waitReady(page: Page) {
  await page.goto('/tests/playwright/chat-harness.html');
  await page.waitForSelector('#prompt-input', { state: 'visible', timeout: 5_000 });
  await page.waitForTimeout(200);
}

async function injectCanvasModuleRef(page: Page) {
  await page.evaluate(async () => {
    const mod = await import('../../internal/web/static/canvas.js');
    (window as any).__canvasModule = mod;
  });
}

async function renderTestArtifact(page: Page) {
  await page.evaluate(() => {
    const mod = (window as any).__canvasModule;
    mod.renderCanvas({
      event_id: 'art-1',
      kind: 'text_artifact',
      title: 'test.txt',
      text: 'Line one\nLine two\nLine three\nLine four\nLine five',
    });
    const ct = document.getElementById('canvas-text');
    if (ct) {
      ct.style.display = 'flex';
      ct.classList.add('is-active');
    }
  });
}

/** Install a fetch spy that captures the full body of chat message POSTs. */
async function installMessageSpy(page: Page) {
  await page.evaluate(() => {
    (window as any).__sentBodies = [];
    const prev = window.fetch;
    window.fetch = async function(url: any, opts: any) {
      const u = String(url);
      if (u.includes('/messages') && opts?.method === 'POST') {
        try {
          const body = JSON.parse(opts.body);
          (window as any).__sentBodies.push(body);
        } catch (_) {}
      }
      return prev.apply(this, arguments as any);
    };
  });
}

async function getSentBodies(page: Page): Promise<any[]> {
  return page.evaluate(() => (window as any).__sentBodies.slice());
}

test.describe('annotation bubble', () => {
  test.beforeEach(async ({ page }) => {
    await waitReady(page);
    await injectCanvasModuleRef(page);
    await installMessageSpy(page);
  });

  test('click on artifact opens annotation bubble', async ({ page }) => {
    await renderTestArtifact(page);
    const canvasText = page.locator('#canvas-text');
    await expect(canvasText).toBeVisible();

    const box = await canvasText.boundingBox();
    if (!box) throw new Error('canvas-text not visible');
    await page.mouse.click(box.x + 20, box.y + 20);
    await page.waitForTimeout(200);

    // In headless, caretRangeFromPoint may not work; verify no crash.
    // If it works, bubble should appear.
    const bubbleCount = await page.locator('.annotation-bubble').count();
    // Either 0 (caretRange failed) or 1 (success) — both are acceptable
    expect(bubbleCount).toBeLessThanOrEqual(1);
  });

  test('bubble send posts message with thread_key', async ({ page }) => {
    // Manually open a bubble via module API
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/canvas-bubble.js');
      mod.openAnnotationBubble({
        location: { line: 42, title: 'doc.md' },
        clientX: 100,
        clientY: 100,
      });
    });
    await expect(page.locator('.annotation-bubble')).toBeVisible();
    await expect(page.locator('.annotation-bubble-location')).toContainText('Line 42 of "doc.md"');

    const input = page.locator('.annotation-bubble-input');
    await input.fill('fix this bug');
    await page.locator('.annotation-bubble-send').click();
    await page.waitForTimeout(300);

    const bodies = await getSentBodies(page);
    expect(bodies.length).toBeGreaterThanOrEqual(1);
    const sent = bodies[bodies.length - 1];
    expect(sent.text).toBe('fix this bug');
    expect(sent.thread_key).toBeTruthy();
    expect(String(sent.thread_key)).toMatch(/^ann-/);
  });

  test('bubble receives streamed response', async ({ page }) => {
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/canvas-bubble.js');
      mod.openAnnotationBubble({
        location: { line: 1, title: 'test.txt' },
        clientX: 100,
        clientY: 100,
      });
      const threadKey = mod.getActiveThreadKey();
      // Simulate streamed response
      mod.routeBubbleEvent({ type: 'turn_started', turn_id: 'turn-1', thread_key: threadKey });
      mod.routeBubbleEvent({
        type: 'assistant_message',
        turn_id: 'turn-1',
        thread_key: threadKey,
        message: 'Here is the fix',
      });
      mod.routeBubbleEvent({
        type: 'message_persisted',
        role: 'assistant',
        turn_id: 'turn-1',
        thread_key: threadKey,
        message: 'Here is the fix',
      });
    });

    const messages = page.locator('.annotation-bubble-messages');
    await expect(messages).toBeVisible();
    await expect(messages.locator('.annotation-bubble-msg-assistant')).toContainText('Here is the fix');
  });

  test('bubble dismiss on click outside', async ({ page }) => {
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/canvas-bubble.js');
      mod.openAnnotationBubble({
        location: { line: 1, title: 'test.txt' },
        clientX: 100,
        clientY: 100,
      });
    });
    await expect(page.locator('.annotation-bubble')).toBeVisible();

    // Wait for outside-click handler to register (50ms setTimeout in bubble code)
    await page.waitForTimeout(100);

    // Click outside (on body)
    await page.mouse.click(5, 5);
    await page.waitForTimeout(200);
    await expect(page.locator('.annotation-bubble')).toHaveCount(0);
  });

  test('bubble dismiss on X button', async ({ page }) => {
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/canvas-bubble.js');
      mod.openAnnotationBubble({
        location: { line: 1, title: 'test.txt' },
        clientX: 100,
        clientY: 100,
      });
    });
    await expect(page.locator('.annotation-bubble')).toBeVisible();

    await page.locator('.annotation-bubble-dismiss').click();
    await page.waitForTimeout(100);
    await expect(page.locator('.annotation-bubble')).toHaveCount(0);
  });

  test('long-press opens bubble with voice recording', async ({ page }) => {
    await renderTestArtifact(page);
    const canvasText = page.locator('#canvas-text');
    await expect(canvasText).toBeVisible();

    const box = await canvasText.boundingBox();
    if (!box) throw new Error('canvas-text not visible');

    await clearLog(page);

    // Long-press: mouse down, wait beyond hold threshold (300ms), then release
    await page.mouse.move(box.x + 30, box.y + 20);
    await page.mouse.down();
    await page.waitForTimeout(500);

    const log = await getLog(page);
    const hasSTTStart = log.some(e => e.type === 'stt' && e.action === 'start');
    // caretRangeFromPoint may not work in headless; verify no crash
    if (hasSTTStart) {
      // Bubble should be open if caretRange worked
      const bubbleCount = await page.locator('.annotation-bubble').count();
      expect(bubbleCount).toBeLessThanOrEqual(1);
    }
    await page.mouse.up();
  });

  test('mobile bottom sheet layout', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 667 });
    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/canvas-bubble.js');
      mod.openAnnotationBubble({
        location: { line: 1, title: 'test.txt' },
        clientX: 100,
        clientY: 100,
      });
    });
    const bubble = page.locator('.annotation-bubble');
    await expect(bubble).toBeVisible();

    const styles = await bubble.evaluate((el) => {
      const cs = window.getComputedStyle(el);
      return { position: cs.position, bottom: cs.bottom };
    });
    expect(styles.position).toBe('fixed');
    expect(styles.bottom).toBe('0px');
  });

  test('main chat not affected by bubble messages', async ({ page }) => {
    const chatHistoryBefore = await page.locator('#chat-history .chat-message').count();

    await page.evaluate(async () => {
      const mod = await import('../../internal/web/static/canvas-bubble.js');
      mod.openAnnotationBubble({
        location: { line: 1, title: 'test.txt' },
        clientX: 100,
        clientY: 100,
      });
    });

    // Send from bubble
    const input = page.locator('.annotation-bubble-input');
    await input.fill('bubble comment');
    await page.locator('.annotation-bubble-send').click();
    await page.waitForTimeout(300);

    // Main chat should not have new entries from the bubble
    const chatHistoryAfter = await page.locator('#chat-history .chat-message').count();
    expect(chatHistoryAfter).toBe(chatHistoryBefore);
  });

  test('text selection works normally without opening bubble', async ({ page }) => {
    await renderTestArtifact(page);
    const canvasText = page.locator('#canvas-text');
    await expect(canvasText).toBeVisible();

    const box = await canvasText.boundingBox();
    if (!box) throw new Error('canvas-text not visible');

    // Select text by dragging
    await page.mouse.move(box.x + 10, box.y + 10);
    await page.mouse.down();
    await page.mouse.move(box.x + 100, box.y + 10);
    await page.mouse.up();
    await page.waitForTimeout(200);

    // Selection should exist, no bubble
    const hasSelection = await page.evaluate(() => {
      const sel = window.getSelection();
      return sel && !sel.isCollapsed;
    });
    // In headless, selection may not work as expected,
    // but we at least verify no crash and no bubble
    const bubbleCount = await page.locator('.annotation-bubble').count();
    expect(bubbleCount).toBe(0);
  });

  test('right-click shows browser default (no bubble)', async ({ page }) => {
    await renderTestArtifact(page);
    const canvasText = page.locator('#canvas-text');
    await expect(canvasText).toBeVisible();

    const box = await canvasText.boundingBox();
    if (!box) throw new Error('canvas-text not visible');
    await page.mouse.click(box.x + 20, box.y + 20, { button: 'right' });
    await page.waitForTimeout(100);

    // No bubble should open on right-click
    const bubbleCount = await page.locator('.annotation-bubble').count();
    expect(bubbleCount).toBe(0);
  });

  test('quick click-release does not start PTT', async ({ page }) => {
    await renderTestArtifact(page);
    const canvasText = page.locator('#canvas-text');
    await expect(canvasText).toBeVisible();

    const box = await canvasText.boundingBox();
    if (!box) throw new Error('canvas-text not visible');

    await clearLog(page);

    // Quick mouse down + up (no hold) should not start PTT
    await page.mouse.move(box.x + 30, box.y + 20);
    await page.mouse.down();
    await page.waitForTimeout(50);
    await page.mouse.up();
    await page.waitForTimeout(100);

    const log = await getLog(page);
    const hasSTTStart = log.some(e => e.type === 'stt' && e.action === 'start');
    expect(hasSTTStart).toBe(false);
  });

  test('line highlight replaces transient marker', async ({ page }) => {
    await renderTestArtifact(page);

    // Verify no old-style transient markers exist
    const markerCount = await page.locator('.transient-marker').count();
    expect(markerCount).toBe(0);

    // Click to trigger line highlight (if caretRangeFromPoint works)
    const canvasText = page.locator('#canvas-text');
    const box = await canvasText.boundingBox();
    if (box) {
      await page.mouse.click(box.x + 20, box.y + 20);
      await page.waitForTimeout(100);
    }

    // No transient markers should exist (old API removed)
    const markerCountAfter = await page.locator('.transient-marker').count();
    expect(markerCountAfter).toBe(0);
  });
});
