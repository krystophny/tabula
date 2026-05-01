import { expect, test, annotatePlaytest, openLiveApp, applySessionCookie } from '../e2e/live';
import { authenticate, clearLiveChat } from '../e2e/helpers';
import {
  assertCircleNoOverlap,
  browserWsTtsToStt,
  circleSegment,
  clickCircleSegment,
  openCircle,
  resetCircleRuntimeState,
  setCircleToggle,
  setInteractionTool,
  setLiveSession,
  submitPrompt,
  waitForAssistantReply,
  waitForLiveAppReady,
} from '../e2e/live-ui';

async function openTopEdge(page: Parameters<typeof openLiveApp>[0]) {
  await page.locator('#edge-top-tap').evaluate((node: HTMLElement) => node.click());
  const edgeTop = page.locator('#edge-top');
  await expect(edgeTop).toHaveClass(/edge-pinned/);
  await expect(page.locator('#edge-top-projects')).toBeVisible();
  await expect(page.locator('#edge-top-models')).toBeVisible();
}

test.describe('live playtest smoke', () => {
  let sessionToken = '';

  test.beforeAll(async () => {
    sessionToken = await authenticate();
  });

  test('edge chrome opens and escape collapses the live shell', async ({ page }, testInfo) => {
    annotatePlaytest(testInfo, {
      tested: 'Live canvas shell navigation via top, right, and left edge controls.',
      expected: 'The edge panels should open on the real app, the file sidebar should toggle, and Escape should collapse transient UI.',
      steps: [
        './scripts/playtest.sh --grep "edge chrome opens and escape collapses the live shell"',
        'Open the live app at http://127.0.0.1:8420/ and trigger the top, right, and left edge controls.',
      ],
    });

    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);

    await openTopEdge(page);

    await page.locator('#edge-right-tap').click();
    await expect(page.locator('#edge-right')).toHaveClass(/edge-pinned/);

    await page.locator('#edge-left-tap').click();
    await expect(page.locator('body')).toHaveClass(/file-sidebar-open/);

    await page.keyboard.press('Escape');
    await expect(page.locator('body')).not.toHaveClass(/file-sidebar-open/);

    await page.keyboard.press('Escape');
    await expect(page.locator('#edge-right')).not.toHaveClass(/edge-pinned/);
    await expect(page.locator('#edge-top')).not.toHaveClass(/edge-pinned/);
  });

  test('slopshell circle stays readable and drives all desktop click states', async ({ page }, testInfo) => {
    annotatePlaytest(testInfo, {
      tested: 'Live Slopshell Circle geometry plus desktop click routing for live mode, toggles, and tools.',
      expected: 'The expanded circle should stay legible without overlapping segments, and every segment should update the live runtime state.',
      steps: [
        './scripts/playtest.sh --grep "slopshell circle stays readable and drives all desktop click states"',
        'Open the live app, expand the Slopshell Circle, verify non-overlapping geometry, then click live-mode, toggle, and tool segments.',
      ],
    });

    await openLiveApp(page, sessionToken);
    await openTopEdge(page);
    await waitForLiveAppReady(page);
    await resetCircleRuntimeState(page);

    await assertCircleNoOverlap(page);

    await setLiveSession(page, 'dialogue', true);
    await setLiveSession(page, 'meeting', true);
    await setLiveSession(page, 'meeting', false);
    await setCircleToggle(page, 'silent', true);
    await expect(page.locator('body')).toHaveClass(/silent-mode/);
    await setCircleToggle(page, 'fast', true);
    await expect(circleSegment(page, 'fast')).toHaveAttribute('aria-pressed', 'true');

    await setInteractionTool(page, 'prompt');
    await setInteractionTool(page, 'text_note');
    await setInteractionTool(page, 'highlight');
    await setInteractionTool(page, 'ink');
    await setInteractionTool(page, 'pointer');
    await expect(page.locator('#slopshell-circle-dot')).toHaveAttribute('data-tool', 'pointer');

    await page.reload({ waitUntil: 'networkidle' });
    await openLiveApp(page, sessionToken);
    await openTopEdge(page);
    await waitForLiveAppReady(page);

    await expect(circleSegment(page, 'silent')).toHaveAttribute('aria-pressed', 'true');
    await expect(circleSegment(page, 'fast')).toHaveAttribute('aria-pressed', 'true');
    await expect(page.locator('#slopshell-circle-dot')).toHaveAttribute('data-tool', 'pointer');
  });

  test('browser roundtrips piper-backed tts audio through http and websocket stt', async ({ page }, testInfo) => {
    annotatePlaytest(testInfo, {
      tested: 'Real browser websocket TTS output fed back through both HTTP and websocket STT on the live runtime.',
      expected: 'The live browser should be able to synthesize speech, submit that audio back to STT, and recover a non-empty transcript over both routes.',
      steps: [
        './scripts/playtest.sh --grep "browser roundtrips piper-backed tts audio through http and websocket stt"',
        'Open the live app and round-trip TTS-generated audio back through STT using both transport paths.',
      ],
    });

    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);

    const httpRoundtrip = await browserWsTtsToStt(page, 'Computer systems check for HTTP roundtrip.', 'http');
    expect(httpRoundtrip.wavBytes).toBeGreaterThan(44);
    expect(httpRoundtrip.status).toBe(200);
    expect(String(httpRoundtrip.transcript || '').trim().length).toBeGreaterThan(5);

    const wsRoundtrip = await browserWsTtsToStt(page, 'Computer systems check for websocket roundtrip.', 'ws');
    expect(wsRoundtrip.wavBytes).toBeGreaterThan(44);
    expect(wsRoundtrip.status).toBe('stt_result');
    expect(String(wsRoundtrip.transcript || '').trim().length).toBeGreaterThan(5);
  });

  test('live local llm answers typed turns in usual and fast modes', async ({ page }, testInfo) => {
    annotatePlaytest(testInfo, {
      tested: 'Real typed chat against the local model in usual and fast modes through the live UI.',
      expected: 'The live app should return assistant replies for a normal turn and a fast-mode turn without leaving the runtime stuck.',
      steps: [
        './scripts/playtest.sh --grep "live local llm answers typed turns in usual and fast modes"',
        'Open the live app, send a short deterministic prompt in usual mode, then toggle fast mode and send another prompt.',
      ],
    });

    await clearLiveChat(sessionToken);
    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);
    await setCircleToggle(page, 'fast', false);
    await setCircleToggle(page, 'silent', false);

    const usualCount = await page.locator('#chat-history .chat-message.chat-assistant:not(.is-pending)').count();
    await submitPrompt(page, 'Reply with the single word ORBIT.');
    await waitForAssistantReply(page, usualCount, 'orbit', 120_000);

    await setCircleToggle(page, 'fast', true);
    const fastCount = await page.locator('#chat-history .chat-message.chat-assistant:not(.is-pending)').count();
    await submitPrompt(page, 'Reply with the single word RIVET.');
    await waitForAssistantReply(page, fastCount, 'rivet', 120_000);
  });
});

test.describe('mobile capture route', () => {
  test.use({
    viewport: { width: 390, height: 844 },
    hasTouch: true,
    isMobile: true,
  });

  let sessionToken = '';

  test.beforeAll(async () => {
    sessionToken = await authenticate();
  });

  test('mobile tap states reach the circle segments without overlap', async ({ page }, testInfo) => {
    annotatePlaytest(testInfo, {
      tested: 'Mobile tap routing for Slopshell Circle live-mode, toggle, and tool segments on the live runtime.',
      expected: 'The expanded mobile circle should stay readable without overlap and respond to touch interactions on the live runtime.',
      steps: [
        './scripts/playtest.sh --grep "mobile tap states reach the circle segments without overlap"',
        'Open the live app on a mobile viewport, expand the circle, then tap live-mode, toggle, and tool segments.',
      ],
    });

    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);
    await resetCircleRuntimeState(page, 'touch');

    await openCircle(page, 'touch');
    await assertCircleNoOverlap(page);
    await clickCircleSegment(page, 'dialogue', 'touch');
    await expect(circleSegment(page, 'dialogue')).toHaveAttribute('aria-pressed', 'true');
    await clickCircleSegment(page, 'silent', 'touch');
    await expect(circleSegment(page, 'silent')).toHaveAttribute('aria-pressed', 'true');
    await clickCircleSegment(page, 'fast', 'touch');
    await expect(circleSegment(page, 'fast')).toHaveAttribute('aria-pressed', 'true');
    await clickCircleSegment(page, 'ink', 'touch');
    await expect(circleSegment(page, 'ink')).toHaveAttribute('aria-pressed', 'true');
  });

  test('capture route loads without the full canvas shell', async ({ page }, testInfo) => {
    annotatePlaytest(testInfo, {
      tested: 'The dedicated mobile capture route on the live runtime.',
      expected: 'The capture UI should load on a mobile viewport while the full canvas shell stays absent.',
      steps: [
        './scripts/playtest.sh --grep "capture route loads without the full canvas shell"',
        'Open http://127.0.0.1:8420/capture on a mobile-sized viewport.',
      ],
    });

    await applySessionCookie(page, sessionToken);
    await page.goto('/capture');
    await page.waitForLoadState('networkidle');

    await expect(page.locator('#capture-page')).toBeVisible();
    await expect(page.locator('#capture-record')).toBeVisible();
    await expect(page.locator('#workspace')).toHaveCount(0);
    await expect(page.locator('#edge-left-tap')).toHaveCount(0);
  });
});
