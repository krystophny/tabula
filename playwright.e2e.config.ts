import { defineConfig } from '@playwright/test';

const audioFile = process.env.E2E_AUDIO_FILE;
if (!audioFile) {
  throw new Error('E2E_AUDIO_FILE env var required (path to speech WAV for fake mic input)');
}

export default defineConfig({
  testDir: 'tests/e2e',
  timeout: 60_000,
  fullyParallel: false,
  workers: 1,
  expect: {
    timeout: 10_000,
  },
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.E2E_BASE_URL || process.env.TABURA_TEST_SERVER_URL || 'http://127.0.0.1:8420',
    headless: true,
  },
  projects: [
    {
      name: 'chromium',
      use: {
        browserName: 'chromium',
        launchOptions: {
          args: [
            '--use-fake-device-for-media-stream',
            '--use-fake-ui-for-media-stream',
            `--use-file-audio-capture=${audioFile}`,
          ],
        },
      },
    },
  ],
});
