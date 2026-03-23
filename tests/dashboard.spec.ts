import { test, expect, Page } from '@playwright/test';

const BASE = 'http://localhost:8080';
const ADMIN_EMAIL = 'admin@example.com';
const ADMIN_PASSWORD = 'StrongPassword123';

// Helper: login and return authenticated page
async function login(page: Page) {
  await page.goto(`${BASE}/dashboard/login`);
  await page.fill('input[name="email"]', ADMIN_EMAIL);
  await page.fill('input[name="password"]', ADMIN_PASSWORD);
  await page.click('button[type="submit"]');
  await page.waitForURL('**/dashboard');
}

// Helper: wait for all HTMX partials to load
async function waitForPartialsLoaded(page: Page) {
  await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
  await expect(page.locator('#dashboard-usage')).not.toContainText('Loading usage...');
  await expect(page.locator('#dashboard-keys-list')).not.toContainText('Loading keys...');
}

// ─── Health & Public endpoints ─────────────────────────────────

test.describe('Health endpoints', () => {
  test('GET /health returns status ok', async ({ request }) => {
    const res = await request.get(`${BASE}/health`);
    expect(res.status()).toBe(200);
    const body = await res.json();
    expect(body.status).toBe('ok');
    expect(body).toHaveProperty('time');
    expect(body).toHaveProperty('provider');
  });

  test('GET /livez returns 200', async ({ request }) => {
    const res = await request.get(`${BASE}/livez`);
    expect(res.status()).toBe(200);
  });

  test('GET /readyz returns 200', async ({ request }) => {
    const res = await request.get(`${BASE}/readyz`);
    expect(res.status()).toBe(200);
  });
});

// ─── Login page ────────────────────────────────────────────────

test.describe('Login page', () => {
  test('GET /dashboard redirects to /dashboard/login when not authenticated', async ({ page }) => {
    await page.goto(`${BASE}/dashboard`);
    await expect(page).toHaveURL(/\/dashboard\/login/);
  });

  test('login page renders correctly', async ({ page }) => {
    await page.goto(`${BASE}/dashboard/login`);
    await expect(page).toHaveTitle(/Enclava/);
    await expect(page.locator('text=Enclava').first()).toBeVisible();
    await expect(page.locator('text=Sign in').first()).toBeVisible();
    await expect(page.locator('input[name="email"]')).toBeVisible();
    await expect(page.locator('input[name="password"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('login with wrong password shows error', async ({ page }) => {
    await page.goto(`${BASE}/dashboard/login`);
    await page.fill('input[name="email"]', ADMIN_EMAIL);
    await page.fill('input[name="password"]', 'wrong-password');
    await page.click('button[type="submit"]');
    await expect(page).toHaveURL(/\/dashboard\/login/);
    await expect(page.locator('text=Invalid email or password')).toBeVisible();
  });

  test('login with correct credentials redirects to dashboard', async ({ page }) => {
    await login(page);
    await expect(page).toHaveURL(/\/dashboard$/);
  });
});

// ─── Dashboard main page ───────────────────────────────────────

test.describe('Dashboard main page (authenticated)', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('dashboard page loads with correct structure', async ({ page }) => {
    await expect(page.locator('text=Enclava').first()).toBeVisible();
    await expect(page.locator('button[title="Toggle theme"]')).toBeVisible();
    await expect(page.locator(`text=${ADMIN_EMAIL}`)).toBeVisible();
    await expect(page.locator('button[title="Sign out"]')).toBeVisible();
  });

  test('dashboard heading and description visible', async ({ page }) => {
    await expect(page.locator('h1:text("Dashboard")')).toBeVisible();
    await expect(page.locator('text=Manage your Enclava service')).toBeVisible();
  });

  test('service health section loads', async ({ page }) => {
    await expect(page.locator('text=Service Health')).toBeVisible();
    await expect(page.locator('#system-status')).not.toHaveText('Checking service status...');
  });

  test('theme toggle works', async ({ page }) => {
    const html = page.locator('html');
    await expect(html).toHaveClass(/dark/);
    await page.click('button[title="Toggle theme"]');
    await expect(html).not.toHaveClass(/dark/);
    await page.click('button[title="Toggle theme"]');
    await expect(html).toHaveClass(/dark/);
  });
});

// ─── HTMX partials: Stats ──────────────────────────────────────

test.describe('Stats partial', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('stats section loads via HTMX', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=Active API Keys')).toBeVisible();
    await expect(page.locator('text=Extract Jobs (24h)')).toBeVisible();
    await expect(page.locator('text=Total Extract Jobs')).toBeVisible();
    await expect(page.locator('text=Budget Usage')).toBeVisible();
  });

  test('stats shows budget dollar amounts', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    // Budget card should show dollar amounts like "$0.00 / $100.00"
    await expect(page.locator('text=/\\$\\d/').first()).toBeVisible();
  });

  test('extract module card visible', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=Extract Documents')).toBeVisible();
    await expect(page.getByText('Extract Module', { exact: true })).toBeVisible();
  });

  test('API endpoint card visible with copy button', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=API Endpoint')).toBeVisible();
    // Wait for initializeEndpointCopy to populate the element
    await expect(page.locator('#api-endpoint')).toContainText('/api/v1');
    await expect(page.locator('#copy-api-endpoint')).toBeVisible();
  });

  test('quick link cards visible', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    // Quick links grid at the bottom of stats section
    await expect(page.locator('.grid >> p.font-medium:text-is("Extract")')).toBeVisible();
    await expect(page.locator('.grid >> p.font-medium:text-is("API Keys")')).toBeVisible();
    await expect(page.locator('.grid >> p.font-medium:text-is("Budgets")')).toBeVisible();
    await expect(page.locator('.grid >> p.font-medium:text-is("Analytics")')).toBeVisible();
  });

  test('stats partial API returns HTML with session cookie', async ({ page, request }) => {
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(c => c.name === 'enclava_dashboard_session');
    expect(sessionCookie).toBeTruthy();

    const res = await request.get(`${BASE}/dashboard/partials/stats`, {
      headers: {
        Cookie: `enclava_dashboard_session=${sessionCookie!.value}`,
      },
    });
    expect(res.status()).toBe(200);
    const contentType = res.headers()['content-type'];
    expect(contentType).toContain('text/html');
    const text = await res.text();
    expect(text).not.toContain('"error"');
    expect(text).toContain('Active API Keys');
  });
});

// ─── HTMX partials: Usage ──────────────────────────────────────

test.describe('Usage partial', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('usage section loads via HTMX', async ({ page }) => {
    await expect(page.locator('#dashboard-usage')).not.toContainText('Loading usage...');
    await expect(page.locator('text=Total tokens')).toBeVisible();
    await expect(page.locator('text=Total cost')).toBeVisible();
  });

  test('usage table has correct headers', async ({ page }) => {
    await expect(page.locator('#dashboard-usage')).not.toContainText('Loading usage...');
    await expect(page.locator('#dashboard-usage th:text("API Key")')).toBeVisible();
    await expect(page.locator('#dashboard-usage th:text("Requests")')).toBeVisible();
    await expect(page.locator('#dashboard-usage th:text("Tokens")')).toBeVisible();
    await expect(page.locator('#dashboard-usage th:text("Cost")')).toBeVisible();
  });

  test('usage partial API returns HTML with session cookie', async ({ page, request }) => {
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(c => c.name === 'enclava_dashboard_session');

    const res = await request.get(`${BASE}/dashboard/partials/usage`, {
      headers: {
        Cookie: `enclava_dashboard_session=${sessionCookie!.value}`,
      },
    });
    expect(res.status()).toBe(200);
    const text = await res.text();
    expect(text).not.toContain('"error"');
    expect(text).toContain('Total tokens');
  });
});

// ─── HTMX partials: Keys ──────────────────────────────────────

test.describe('API Keys management', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('keys section loads via HTMX', async ({ page }) => {
    await expect(page.locator('#dashboard-keys-list')).not.toContainText('Loading keys...');
    const content = await page.locator('#dashboard-keys-list').textContent();
    const hasKeys = content?.includes('Active') || content?.includes('Inactive');
    const isEmpty = content?.includes('No API keys');
    expect(hasKeys || isEmpty).toBeTruthy();
  });

  test('API Key Management heading visible', async ({ page }) => {
    await expect(page.locator('text=API Key Management')).toBeVisible();
  });

  test('create key form has correct fields', async ({ page }) => {
    await expect(page.locator('label[for="key-name"]')).toBeVisible();
    await expect(page.locator('input[name="name"]')).toBeVisible();
    await expect(page.locator('label[for="key-description"]')).toBeVisible();
    await expect(page.locator('input[name="description"]')).toBeVisible();
    await expect(page.locator('button:text("Create key")')).toBeVisible();
  });

  test('create key name is required', async ({ page }) => {
    const nameInput = page.locator('input[name="name"]');
    await expect(nameInput).toHaveAttribute('required', '');
  });

  test('create API key end-to-end', async ({ page }) => {
    const keyName = 'test-key-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Playwright test key');
    await page.click('button:text("Create key")');

    await page.waitForTimeout(1000);
    await expect(page.locator('#dashboard-keys-list')).toContainText('API key created');
    await expect(page.locator('#dashboard-keys-list')).toContainText('Store this key now');
    await expect(page.locator('#dashboard-keys-list')).toContainText(keyName);
  });

  test('toggle API key status', async ({ page }) => {
    const keyName = 'toggle-test-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Toggle test');
    await page.click('button:text("Create key")');
    await page.waitForTimeout(1000);

    const row = page.locator(`tr:has-text("${keyName}")`);
    await expect(row).toBeVisible();
    await row.locator('button:text("Disable")').click();
    await page.waitForTimeout(1000);

    const updatedRow = page.locator(`tr:has-text("${keyName}")`);
    await expect(updatedRow.locator('text=Inactive')).toBeVisible();
    await expect(updatedRow.locator('button:text("Enable")')).toBeVisible();
  });

  test('regenerate API key shows new secret', async ({ page }) => {
    const keyName = 'regen-test-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Regen test');
    await page.click('button:text("Create key")');
    await page.waitForTimeout(1000);

    const row = page.locator(`tr:has-text("${keyName}")`);
    await row.locator('button:text("Regenerate")').click();
    await page.waitForTimeout(1000);

    await expect(page.locator('#dashboard-keys-list')).toContainText('API key regenerated');
    await expect(page.locator('#dashboard-keys-list')).toContainText('Store this key now');
  });

  test('delete API key with confirmation', async ({ page }) => {
    const keyName = 'delete-test-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Delete test');
    await page.click('button:text("Create key")');
    await page.waitForTimeout(1000);
    await expect(page.locator('#dashboard-keys-list')).toContainText(keyName);

    page.on('dialog', dialog => dialog.accept());

    const row = page.locator(`tr:has-text("${keyName}")`);
    await row.locator('button:text("Delete")').click();
    await page.waitForTimeout(1000);

    await expect(page.locator('#dashboard-keys-list')).toContainText('API key deleted');
    await expect(page.locator(`tr:has-text("${keyName}")`)).not.toBeVisible();
  });

  test('keys partial API returns HTML with session cookie', async ({ page, request }) => {
    const cookies = await page.context().cookies();
    const sessionCookie = cookies.find(c => c.name === 'enclava_dashboard_session');

    const res = await request.get(`${BASE}/dashboard/partials/keys`, {
      headers: {
        Cookie: `enclava_dashboard_session=${sessionCookie!.value}`,
      },
    });
    expect(res.status()).toBe(200);
    const text = await res.text();
    expect(text).not.toContain('"error"');
    expect(text).not.toContain('missing API key');
  });
});

// ─── Logout ────────────────────────────────────────────────────

test.describe('Logout', () => {
  test('logout redirects to login page', async ({ page }) => {
    await login(page);
    await page.click('button[title="Sign out"]');
    await expect(page).toHaveURL(/\/dashboard\/login/);
  });

  test('after logout, dashboard redirects to login', async ({ page }) => {
    await login(page);
    await page.click('button[title="Sign out"]');
    await expect(page).toHaveURL(/\/dashboard\/login/);
    await page.goto(`${BASE}/dashboard`);
    await expect(page).toHaveURL(/\/dashboard\/login/);
  });
});

// ─── Unauthenticated partial access ───────────────────────────

test.describe('Unauthenticated access to partials', () => {
  test('partials/stats returns 401 without auth', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/partials/stats`);
    expect(res.status()).toBe(401);
  });

  test('partials/keys returns 401 without auth', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/partials/keys`);
    expect(res.status()).toBe(401);
  });

  test('partials/usage returns 401 without auth', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/partials/usage`);
    expect(res.status()).toBe(401);
  });
});

// ─── Static assets ─────────────────────────────────────────────

test.describe('Static assets', () => {
  test('tailwind.css loads', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/static/tailwind.css`);
    expect(res.status()).toBe(200);
    const text = await res.text();
    expect(text).toContain('--background');
    expect(text).toContain('--foreground');
  });

  test('dashboard.js loads', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/static/dashboard.js`);
    expect(res.status()).toBe(200);
    const text = await res.text();
    expect(text).toContain('toggleTheme');
    expect(text).toContain('showToast');
  });

  test('htmx.min.js loads', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/static/htmx.min.js`);
    expect(res.status()).toBe(200);
  });
});

// ─── Console errors ────────────────────────────────────────────

test.describe('No JS console errors', () => {
  test('dashboard page has no console errors', async ({ page }) => {
    const errors: string[] = [];
    page.on('console', msg => {
      if (msg.type() === 'error') errors.push(msg.text());
    });

    await login(page);
    await waitForPartialsLoaded(page);

    const unexpected = errors.filter(e => !e.includes('favicon'));
    expect(unexpected).toEqual([]);
  });
});
