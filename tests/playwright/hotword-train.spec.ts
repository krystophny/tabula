import { expect, test } from '@playwright/test';

function wavBuffer() {
  const samples = new Int16Array([0, 1200, -1200, 600, -600, 0]);
  const dataSize = samples.length * 2;
  const buffer = new ArrayBuffer(44 + dataSize);
  const view = new DataView(buffer);
  let offset = 0;
  const writeString = (value: string) => {
    for (let i = 0; i < value.length; i += 1) {
      view.setUint8(offset, value.charCodeAt(i));
      offset += 1;
    }
  };
  writeString('RIFF');
  view.setUint32(offset, 36 + dataSize, true); offset += 4;
  writeString('WAVE');
  writeString('fmt ');
  view.setUint32(offset, 16, true); offset += 4;
  view.setUint16(offset, 1, true); offset += 2;
  view.setUint16(offset, 1, true); offset += 2;
  view.setUint32(offset, 16000, true); offset += 4;
  view.setUint32(offset, 32000, true); offset += 4;
  view.setUint16(offset, 2, true); offset += 2;
  view.setUint16(offset, 16, true); offset += 2;
  writeString('data');
  view.setUint32(offset, dataSize, true); offset += 4;
  samples.forEach((sample) => {
    view.setInt16(offset, sample, true);
    offset += 2;
  });
  return Buffer.from(buffer);
}

test('hotword training page uploads recordings and runs the core workflow', async ({ page }) => {
  await page.goto('/tests/playwright/hotword-train-harness.html');

  await expect(page.locator('#train-banner')).toContainText('Wake word assets are not fully deployed yet');
  await page.setInputFiles('#recording-upload', {
    name: 'sample.wav',
    mimeType: 'audio/wav',
    buffer: wavBuffer(),
  });

  await expect(page.locator('#recording-list')).toContainText('.wav');
  await page.locator('#generation-start').click();
  await expect(page.locator('#generation-status')).toContainText('Generation complete.');

  await page.locator('#training-start').click();
  await expect(page.locator('#training-status')).toContainText('Training complete.');
  await expect(page.locator('#model-list')).toContainText('sloppy.onnx');

  await page.getByRole('button', { name: 'Deploy' }).click();
  await expect(page.locator('#deployment-status')).toContainText('Deployed sloppy.onnx.');

  const requests = await page.evaluate(() => (window as any).__hotwordTrainRequests);
  expect(requests).toEqual({
    uploads: 1,
    deletes: 0,
    generate: 1,
    train: 1,
    deploy: 1,
  });
});
