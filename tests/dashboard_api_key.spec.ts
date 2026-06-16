import { test, expect } from '@playwright/test';

const BASE = process.env.DASHBOARD_BASE_URL || '';
const API_KEY = process.env.DASHBOARD_API_KEY || '';

test.describe('Dashboard API key mode', () => {
  test.skip(!BASE || !API_KEY, 'DASHBOARD_BASE_URL and DASHBOARD_API_KEY are required');

  test('loads stats partial after saving API key', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await expect(page.locator('#api-key-login-panel')).toBeVisible();

    await page.fill('#api-key-input', API_KEY);
    const [statsResponse] = await Promise.all([
      page.waitForResponse((response) =>
        response.url() === `${BASE}/dashboard/partials/stats` &&
        response.status() === 200,
      ),
      page.click('button:text("Use this key")'),
    ]);
    expect(statsResponse.ok()).toBeTruthy();

    await expect(page.locator('#dashboard-stats-mount')).toContainText('Active API Keys');
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
  });
});
