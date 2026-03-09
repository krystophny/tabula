import { expect, test } from './live';
import { requireTestPassword } from './helpers';

test.describe('login UI flow', () => {
  test('password submit reveals the main app', async ({ page }) => {
    const setupResp = await page.request.get('/api/setup');
    const setup = await setupResp.json() as Record<string, unknown>;
    test.skip(!setup.has_password, 'auth disabled');

    const password = requireTestPassword();

    await page.goto('/');
    await expect(page.locator('#view-login')).toBeVisible();
    await expect(page.locator('#view-main')).toBeHidden();

    await page.fill('#login-password', password);
    await page.click('#btn-login');

    await expect(page.locator('#view-login')).toBeHidden();
    await expect(page.locator('#view-main')).toBeVisible();
    await expect(page.locator('#workspace')).toBeVisible();
    await expect(page.locator('#login-error')).toHaveText('');
    await expect.poll(async () => {
      const cookies = await page.context().cookies();
      return cookies.some((cookie) => cookie.name === 'tabura_session');
    }).toBe(true);
  });
});
