import { expect, test } from '@playwright/test';

test.describe('capture page', () => {
  test('saves a typed note and stays outside the canvas shell', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/tests/playwright/capture-harness.html');

    await expect(page.locator('#capture-page')).toBeVisible();
    await expect(page.locator('#workspace')).toHaveCount(0);
    await expect(page.locator('#edge-left-tap')).toHaveCount(0);
    await expect(page.locator('#capture-save')).toBeDisabled();

    await page.locator('#capture-note').fill('Follow up with the review queue tomorrow morning. Capture the blockers too.');
    await expect(page.locator('#capture-save')).toBeEnabled();

    await page.locator('#capture-save').click();
    await expect(page.locator('#capture-note')).toHaveValue('');
    await expect(page.locator('#capture-status')).toContainText('Saved:');
    await expect(page.locator('#capture-save')).toBeDisabled();

    const requests = await page.evaluate(() => (window as any).__captureRequests);
    expect(requests).toHaveLength(1);
    expect(requests[0].title).toBe('Follow up with the review queue tomorrow morning.');
  });

  test('toggles record state with the large capture button', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto('/tests/playwright/capture-harness.html');

    await expect(page.locator('body')).toHaveAttribute('data-capture-state', 'idle');
    await page.locator('#capture-record').click({ force: true });
    await expect(page.locator('body')).toHaveAttribute('data-capture-state', 'recording');
    await expect(page.locator('#capture-record')).toHaveAttribute('aria-pressed', 'true');

    await page.locator('#capture-record').click({ force: true });
    await expect(page.locator('body')).toHaveAttribute('data-capture-state', 'idle');
    await expect(page.locator('#capture-record')).toHaveAttribute('aria-pressed', 'false');
    await expect(page.locator('#capture-status')).toContainText('Recording captured.');
  });
});
