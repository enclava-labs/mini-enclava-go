import { test, expect, Page } from '@playwright/test';

const BASE = process.env.BASE_URL ?? 'http://localhost:8080';
const ADMIN_EMAIL = process.env.ADMIN_EMAIL ?? 'admin@example.com';
const ADMIN_PASSWORD = process.env.ADMIN_PASSWORD ?? 'StrongPassword123';

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
}

async function goToAnalytics(page: Page) {
  await login(page);
  await page.goto(`${BASE}/dashboard/analytics`);
  await expect(page.locator('#dashboard-usage')).not.toContainText('Loading analytics...');
}

async function goToKeys(page: Page) {
  await login(page);
  await page.goto(`${BASE}/dashboard/keys`);
  await expect(page.locator('#dashboard-keys-list')).not.toContainText('Loading keys...');
}

async function openCreateKeyForm(page: Page) {
  await page.click('button:text("Create API Key")');
  await expect(page.locator('#create-key-form')).toBeVisible();
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
    await expect(page.locator('nav a[href="/dashboard"]').first()).toBeVisible();
    await expect(page.locator('summary[title="API Keys menu"]')).toBeVisible();
    await expect(page.locator('button[title="Toggle theme"]')).toBeVisible();
    await expect(page.locator('summary[title="User menu"]')).toBeVisible();
  });

  test('API Keys menu includes budgets as a submenu item', async ({ page }) => {
    const apiKeysMenu = page.locator('details.dashboard-menu').filter({ has: page.locator('summary[title="API Keys menu"]') }).first();
    await apiKeysMenu.locator('summary[title="API Keys menu"]').click();
    await expect(apiKeysMenu.locator('a[href="/dashboard/keys"]')).toBeVisible();
    await expect(apiKeysMenu.locator('a[href="/dashboard/budgets"]')).toBeVisible();
  });

  test('user menu contains account actions', async ({ page }) => {
    await page.click('summary[title="User menu"]');
    const accountPanel = page.locator('.dashboard-account-panel');
    await expect(accountPanel.getByText(ADMIN_EMAIL, { exact: true })).toBeVisible();
    await expect(accountPanel.getByText('Logout', { exact: true })).toBeVisible();
  });

  test('dashboard heading and description visible', async ({ page }) => {
    await expect(page.locator('h1:text("Gateway overview")')).toBeVisible();
    await expect(page.getByText(`Signed in as ${ADMIN_EMAIL}`)).toBeVisible();
  });

  test('dashboard stats section loads', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=Active keys')).toBeVisible();
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

// ─── Resource dashboard pages ─────────────────────────────────

const resourcePages = [
  { path: '/dashboard/providers', heading: 'Providers', mount: '#dashboard-providers', text: 'Provider readiness' },
  { path: '/dashboard/pricing', heading: 'Pricing', mount: '#dashboard-pricing', text: 'Provider Pricing' },
  { path: '/dashboard/billing-audit', heading: 'Billing Audit', mount: '#dashboard-billing-audit', text: 'Billing Audit' },
  { path: '/dashboard/rag', heading: 'RAG', mount: '#dashboard-rag', text: 'RAG Collections' },
  { path: '/dashboard/chatbots', heading: 'Chatbots', mount: '#dashboard-chatbots', text: 'Chatbot Instances' },
  { path: '/dashboard/prompt-templates', heading: 'Prompt Templates', mount: '#dashboard-prompt-templates', text: 'Prompt Templates' },
  { path: '/dashboard/connectors', heading: 'Connectors', mount: '#dashboard-connectors', text: 'Connector Sources' },
];

test.describe('Resource dashboard pages (authenticated)', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  for (const resource of resourcePages) {
    test(`${resource.heading} page renders`, async ({ page }) => {
      await page.goto(`${BASE}${resource.path}`);
      await expect(page.locator(`h1:text("${resource.heading}")`)).toBeVisible();
      await expect(page.locator(resource.mount)).not.toContainText('Loading');
      await expect(page.locator(resource.mount)).toContainText(resource.text);
    });
  }
});

// ─── Resource dashboard workflows ─────────────────────────────

async function createRAGCollection(page: Page, name: string) {
  await page.goto(`${BASE}/dashboard/rag`);
  const mount = page.locator('#dashboard-rag');
  await expect(mount).not.toContainText('Loading');
  const form = mount.locator('form:has(input[name="action"][value="create_collection"])');
  await form.locator('input[name="name"]').fill(name);
  await form.locator('input[name="embedding_model"]').fill('embed-model');
  await form.locator('button:text("Create")').click();
  await expect(mount).toContainText('RAG collection created');
  await expect(mount).toContainText(name);
}

test.describe('Resource dashboard workflows (authenticated)', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('prompt template create flow works', async ({ page }) => {
    await page.goto(`${BASE}/dashboard/prompt-templates`);
    const mount = page.locator('#dashboard-prompt-templates');
    await expect(mount).not.toContainText('Loading');
    const name = `pw-template-${Date.now()}`;

    const form = mount.locator('form').first();
    await form.locator('input[name="name"]').fill(name);
    await form.locator('textarea[name="system_prompt"]').fill('You answer with operational detail.');
    await form.locator('textarea[name="user_prompt"]').fill('Question: {{.user_input}}');
    await form.locator('button:text("Create")').click();

    await expect(mount).toContainText('Prompt template created');
    await expect(mount).toContainText(name);
  });

  test('RAG collection create and document upload flow works', async ({ page }) => {
    const name = `pw-rag-${Date.now()}`;
    await createRAGCollection(page, name);

    const mount = page.locator('#dashboard-rag');
    const uploadForm = mount.locator('form:has(input[name="action"][value="upload_document"])');
    await expect(uploadForm).toBeVisible();
    await uploadForm.locator('select[name="collection_id"]').selectOption({ label: name });
    await uploadForm.locator('input[type="file"]').setInputFiles({
      name: 'playwright-rag.txt',
      mimeType: 'text/plain',
      buffer: Buffer.from('playwright rag upload phrase'),
    });
    await uploadForm.locator('button:text("Upload")').click();

    await expect(mount).toContainText('Document uploaded');
    await expect(mount).toContainText(name);
    await expect(mount).toContainText('RAG Documents');
    const documentRow = mount.locator('tr:has-text("playwright-rag.txt")');
    await expect(documentRow).toBeVisible();

    await documentRow.locator('button:text("Reprocess")').click();
    await expect(mount).toContainText('Document reprocessed');

    page.on('dialog', dialog => dialog.accept());
    await mount.locator('tr:has-text("playwright-rag.txt") button:text("Delete")').click();
    await expect(mount).toContainText('Document deleted');
    await expect(mount.locator('tr:has-text("playwright-rag.txt")')).not.toBeVisible();
  });

  test('chatbot create flow works', async ({ page }) => {
    await page.goto(`${BASE}/dashboard/chatbots`);
    const mount = page.locator('#dashboard-chatbots');
    await expect(mount).not.toContainText('Loading');
    const name = `pw-chatbot-${Date.now()}`;
    const externalID = `pw_chatbot_${Date.now()}`;

    const form = mount.locator('form').first();
    await form.locator('input[name="name"]').fill(name);
    await form.locator('input[name="model"]').fill('contract-model');
    await form.locator('input[name="external_id"]').fill(externalID);
    await form.locator('button:text("Create")').click();

    await expect(mount).toContainText('Chatbot created');
    await expect(mount).toContainText(name);
    await expect(mount).toContainText(`/api/v1/chatbot/external/${externalID}/chat/completions`);

    const row = mount.locator(`tr:has-text("${name}")`);
    await row.locator('button:text("Disable")').click();
    await expect(mount).toContainText('Chatbot disabled');
    await expect(mount.locator(`tr:has-text("${name}")`)).toContainText('Disabled');

    page.on('dialog', dialog => dialog.accept());
    await mount.locator(`tr:has-text("${name}") button:text("Delete")`).click();
    await expect(mount).toContainText('Chatbot deleted');
    await expect(mount.locator(`tr:has-text("${name}")`)).not.toBeVisible();
  });

  test('connector create and sync flow works with source content', async ({ page }) => {
    const collectionName = `pw-connector-rag-${Date.now()}`;
    await createRAGCollection(page, collectionName);

    await page.goto(`${BASE}/dashboard/connectors`);
    const mount = page.locator('#dashboard-connectors');
    await expect(mount).not.toContainText('Loading');
    const connectorName = `pw-connector-${Date.now()}`;

    const createForm = mount.locator('form:has(input[name="action"][value="create_source"])');
    await createForm.locator('input[name="name"]').fill(connectorName);
    await createForm.locator('select[name="type"]').selectOption('github');
    await createForm.locator('select[name="target_collection_id"]').selectOption({ label: collectionName });
    await createForm.locator('input[name="file_name"]').fill('connector-source.txt');
    await createForm.locator('textarea[name="content"]').fill('connector playwright sync phrase');
    await createForm.locator('button:text("Create")').click();

    await expect(mount).toContainText('Connector source created');
    await expect(mount).toContainText(connectorName);

    const row = mount.locator(`tr:has-text("${connectorName}")`);
    await row.locator('button:text("Sync")').click();
    await expect(mount).toContainText('Connector sync started');
    await expect(mount).toContainText('succeeded');

    await mount.locator(`tr:has-text("${connectorName}") button:text("Disable")`).click();
    await expect(mount).toContainText('Connector source disabled');
    await expect(mount.locator(`tr:has-text("${connectorName}")`)).toContainText('Disabled');

    await mount.locator(`tr:has-text("${connectorName}") button:text("Enable")`).click();
    await expect(mount).toContainText('Connector source enabled');
    await expect(mount.locator(`tr:has-text("${connectorName}")`)).toContainText('Enabled');

    page.on('dialog', dialog => dialog.accept());
    await mount.locator(`tr:has-text("${connectorName}") button:text("Delete")`).click();
    await expect(mount).toContainText('Connector source deleted');
    await expect(mount.locator(`tr:has-text("${connectorName}")`)).not.toBeVisible();
  });
});

// ─── HTMX partials: Stats ──────────────────────────────────────

test.describe('Stats partial', () => {
  test.beforeEach(async ({ page }) => {
    await login(page);
  });

  test('stats section loads via HTMX', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=Active keys')).toBeVisible();
    await expect(page.locator('text=Extract · 24h')).toBeVisible();
    await expect(page.locator('text=Extract · total')).toBeVisible();
    await expect(page.locator('text=Budget used')).toBeVisible();
  });

  test('stats shows budget dollar amounts', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    // Budget card should show dollar amounts like "$0.00 / $100.00"
    await expect(page.locator('text=/\\$\\d/').first()).toBeVisible();
  });

  test('extract module card visible', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=System snapshot')).toBeVisible();
    await expect(page.getByText('Extract module', { exact: true })).toBeVisible();
  });

  test('API endpoint card visible with copy button', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('text=API access')).toBeVisible();
    // Wait for initializeEndpointCopy to populate the element
    await expect(page.locator('#api-endpoint')).toContainText('/api/v1');
    await expect(page.locator('#copy-api-endpoint')).toBeVisible();
  });

  test('dashboard relies on persistent navigation instead of shortcut cards', async ({ page }) => {
    await expect(page.locator('#dashboard-stats-mount')).not.toContainText('Loading dashboard...');
    await expect(page.locator('#dashboard-stats-mount a[href="/dashboard/budgets"]')).toHaveCount(1);
    const apiKeysMenu = page.locator('details.dashboard-menu').filter({ has: page.locator('summary[title="API Keys menu"]') }).first();
    await apiKeysMenu.locator('summary[title="API Keys menu"]').click();
    await expect(apiKeysMenu.locator('a[href="/dashboard/budgets"]')).toBeVisible();
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
    expect(text).toContain('Active keys');
  });
});

// ─── HTMX partials: Usage ──────────────────────────────────────

test.describe('Usage partial', () => {
  test.beforeEach(async ({ page }) => {
    await goToAnalytics(page);
  });

  test('usage section loads via HTMX', async ({ page }) => {
    await expect(page.locator('#dashboard-usage')).not.toContainText('Loading analytics...');
    await expect(page.locator('text=Total Tokens')).toBeVisible();
    await expect(page.locator('text=Total Cost')).toBeVisible();
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
    expect(text).toContain('Total Tokens');
  });
});

// ─── HTMX partials: Keys ──────────────────────────────────────

test.describe('API Keys management', () => {
  test.beforeEach(async ({ page }) => {
    await goToKeys(page);
  });

  test('keys section loads via HTMX', async ({ page }) => {
    await expect(page.locator('#dashboard-keys-list')).not.toContainText('Loading keys...');
    const content = await page.locator('#dashboard-keys-list').textContent();
    const hasKeys = content?.includes('Active') || content?.includes('Inactive');
    const isEmpty = content?.includes('No API keys');
    expect(hasKeys || isEmpty).toBeTruthy();
  });

  test('API Key Management heading visible', async ({ page }) => {
    await expect(page.locator('h1:text("API Keys")')).toBeVisible();
    await expect(page.locator('h3:text("Key Inventory")')).toBeVisible();
  });

  test('create key form has correct fields', async ({ page }) => {
    await openCreateKeyForm(page);
    await expect(page.locator('label[for="key-name"]')).toBeVisible();
    await expect(page.locator('input[name="name"]')).toBeVisible();
    await expect(page.locator('label[for="key-description"]')).toBeVisible();
    await expect(page.locator('input[name="description"]')).toBeVisible();
    await expect(page.locator('button:text("Create key")')).toBeVisible();
  });

  test('create key name is required', async ({ page }) => {
    await openCreateKeyForm(page);
    const nameInput = page.locator('input[name="name"]');
    await expect(nameInput).toHaveAttribute('required', '');
  });

  test('create API key end-to-end', async ({ page }) => {
    await openCreateKeyForm(page);
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
    await openCreateKeyForm(page);
    const keyName = 'toggle-test-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Toggle test');
    await page.click('button:text("Create key")');
    await page.waitForTimeout(1000);

    const row = page.locator(`tr:has-text("${keyName}")`);
    await expect(row).toBeVisible();
    await row.locator('button[title="Disable key"]').click();
    await page.waitForTimeout(1000);

    const updatedRow = page.locator(`tr:has-text("${keyName}")`);
    await expect(updatedRow.locator('text=Inactive')).toBeVisible();
    await expect(updatedRow.locator('button[title="Enable key"]')).toBeVisible();
  });

  test('regenerate API key shows new secret', async ({ page }) => {
    await openCreateKeyForm(page);
    const keyName = 'regen-test-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Regen test');
    await page.click('button:text("Create key")');
    await page.waitForTimeout(1000);

    const row = page.locator(`tr:has-text("${keyName}")`);
    page.on('dialog', dialog => dialog.accept());
    await row.locator('button[title="Regenerate key"]').click();
    await page.waitForTimeout(1000);

    await expect(page.locator('#dashboard-keys-list')).toContainText('API key regenerated');
    await expect(page.locator('#dashboard-keys-list')).toContainText('Store this key now');
  });

  test('delete API key with confirmation', async ({ page }) => {
    await openCreateKeyForm(page);
    const keyName = 'delete-test-' + Date.now();
    await page.fill('input[name="name"]', keyName);
    await page.fill('input[name="description"]', 'Delete test');
    await page.click('button:text("Create key")');
    await page.waitForTimeout(1000);
    await expect(page.locator('#dashboard-keys-list')).toContainText(keyName);

    page.on('dialog', dialog => dialog.accept());

    const row = page.locator(`tr:has-text("${keyName}")`);
    await row.locator('button[title="Delete key"]').click();
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
    await page.click('summary[title="User menu"]');
    await page.click('button[title="Sign out"]');
    await expect(page).toHaveURL(/\/dashboard\/login/);
  });

  test('after logout, dashboard redirects to login', async ({ page }) => {
    await login(page);
    await page.click('summary[title="User menu"]');
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

  test('dashboard nav CSS loads', async ({ request }) => {
    const res = await request.get(`${BASE}/dashboard/static/dashboard-nav.css`);
    expect(res.status()).toBe(200);
    const text = await res.text();
    expect(text).toContain('.dashboard-nav');
    expect(text).toContain('.dashboard-account-panel');
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
