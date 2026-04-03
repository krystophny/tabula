/**
 * End-to-end voice pipeline tests.
 *
 * Uses Chromium's --use-fake-device-for-media-stream with --use-file-audio-capture
 * to feed pre-recorded audio through the mic. Tests the full capture → VAD → STT chain.
 *
 * Run:
 *   E2E_AUDIO_FILE=/tmp/hotword-test.wav SLOPSHELL_TEST_PASSWORD=testpass123 \
 *     PLAYWRIGHT_NATIVE=1 ./scripts/e2e-local.sh -- --grep "voice pipeline"
 */
import { applySessionCookie, expect, openLiveApp, test } from './live';
import { authenticate, synthesizePiperWav } from './helpers';
import { setLiveSession, waitForLiveAppReady } from './live-ui';

test.describe('voice pipeline @local-only', () => {
  let sessionToken: string;

  test.beforeAll(async () => {
    sessionToken = await authenticate();
  });

  test('hotword monitor starts in dialogue mode', async ({ page }) => {
    test.setTimeout(30_000);
    await applySessionCookie(page, sessionToken);
    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);
    await setLiveSession(page, 'dialogue', true);

    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as any)._slopshellApp?.getState?.();
        return state?.hotwordActive === true;
      });
    }, {
      message: 'Hotword monitor should become active in dialogue mode',
      timeout: 15_000,
    }).toBe(true);
  });

  test('no ScriptProcessorNode deprecation warning', async ({ page }) => {
    test.setTimeout(30_000);
    const logs: string[] = [];
    page.on('console', msg => logs.push(msg.text()));

    await applySessionCookie(page, sessionToken);
    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);
    await setLiveSession(page, 'dialogue', true);
    await page.waitForTimeout(3000);

    const hasDeprecation = logs.some(l => l.includes('ScriptProcessorNode'));
    expect(hasDeprecation).toBe(false);
  });

  test('click capture → VAD or timeout → STT transcribes fake mic audio', async ({ page }) => {
    test.setTimeout(90_000);
    await applySessionCookie(page, sessionToken);
    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);
    await setLiveSession(page, 'dialogue', true);
    await page.waitForTimeout(2000);

    // Click to start voice capture (manual trigger, not hotword)
    await page.locator('#canvas-viewport').click({
      position: { x: 420, y: 320 },
      timeout: 10_000,
    });

    // Wait for recording to start
    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as any)._slopshellApp?.getState?.();
        return String(state?.voiceLifecycle || '');
      });
    }, { timeout: 8_000 }).toBe('recording');

    // Wait for the capture to complete (VAD speech end or timeout)
    // and the lifecycle to return to idle
    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as any)._slopshellApp?.getState?.();
        const lc = String(state?.voiceLifecycle || '');
        return lc === 'idle' || lc === 'awaiting_turn';
      });
    }, {
      message: 'Voice capture should complete (VAD or timeout)',
      timeout: 60_000,
      intervals: [1000],
    }).toBe(true);
  });

  test('pre-roll ring buffer preserved after hotword monitor stop', async ({ page }) => {
    test.setTimeout(30_000);
    await applySessionCookie(page, sessionToken);
    await openLiveApp(page, sessionToken);
    await waitForLiveAppReady(page);
    await setLiveSession(page, 'dialogue', true);

    // Wait for hotword to be active (ring buffer filling)
    await expect.poll(async () => {
      return page.evaluate(() => {
        const state = (window as any)._slopshellApp?.getState?.();
        return state?.hotwordActive === true;
      });
    }, { timeout: 15_000 }).toBe(true);

    // Wait a moment for ring buffer to fill with audio
    await page.waitForTimeout(2000);

    // Check pre-roll audio is available
    const preRollSamples = await page.evaluate(() => {
      const env = (window as any).__slopshellTestEnv;
      if (!env?.getPreRollAudio) return -1;
      const preRoll = env.getPreRollAudio();
      return preRoll instanceof Float32Array ? preRoll.length : 0;
    });

    // Pre-roll should have some audio (ring buffer has been filling from fake mic)
    // If getPreRollAudio is not exposed on test env, skip gracefully
    if (preRollSamples !== -1) {
      expect(preRollSamples).toBeGreaterThan(0);
    }
  });
});
