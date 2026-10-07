// Local UC-00/UC-01 onboarding verification: a headless Chromium drives the
// Web Console served by a local fake-adapter backend with JSON state.
// Invoked by backend/test/integration/onboarding-playwright-local.sh, once per
// phase: "create" before the backend restart and "restart" after it.
import { chromium, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_PHASE', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_APP_FILE']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const phase = env.ORCH_E2E_PHASE;
const runId = env.ORCH_E2E_RUN_ID;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const name = `Onboarding ${runId}`;
const subdomain = runId;

const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext();
  context.setDefaultTimeout(15_000);
  const page = await context.newPage();
  const createBodies = [];
  const mutations = [];
  page.on('request', (request) => {
    const path = new URL(request.url()).pathname;
    if (request.method() === 'POST' && path === '/api/v1/applications') createBodies.push(request.postData());
    if (/\/(preview|deploy)$|^\/api\/v1\/deployments$/.test(path) && request.method() === 'POST') mutations.push(path);
  });

  async function signIn() {
    await page.getByLabel('Username').fill('developer');
    await page.getByLabel('Password').fill('test-password');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
    await expect(page).toHaveURL(/\/ui\/applications$/);
  }

  async function expectApplicationHome(appId) {
    await expect(page.getByRole('heading', { name })).toBeVisible();
    await expect(page.getByRole('button', { name: new RegExp(`Staging.*staging\\.${subdomain}\\.example\\.com`) })).toHaveClass(/tab-active/);
    await expect(page.getByRole('button', { name: new RegExp(`Production.*${subdomain}\\.example\\.com`) })).toBeVisible();
    const response = await page.request.get(`${baseURL}/api/v1/applications/${appId}`);
    expect(response.status()).toBe(200);
    const view = (await response.json()).application;
    expect(view.environments.map((item) => item.key).sort()).toEqual(['production', 'staging']);
    // A new Application starts with both Environments UNCONFIGURED, before and after a restart.
    for (const item of view.environments) expect([item.configured, item.connectionKey, item.runtimeStatus]).toEqual([false, '', 'UNCONFIGURED']);
    await expect(page.getByLabel('Execution target').first()).toContainText('Staging has no execution connection yet');
    const deployments = await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/deployments`)).json();
    expect(deployments.deployments ?? []).toHaveLength(0);
  }

  await page.goto(`${baseURL}/ui/applications`);
  await expect(page.getByLabel('Username')).toBeVisible();

  if (phase === 'create') {
    await signIn();
    const cookies = await context.cookies();
    const session = cookies.find((cookie) => cookie.name === 'orchestrator_session');
    if (!session?.httpOnly) throw new Error('session cookie must be HttpOnly');

    await page.getByRole('button', { name: '+ Create application' }).first().click();
    await page.getByLabel('Application name').fill(name);
    await page.getByLabel('Subdomain').fill(subdomain);
    await expect(page.getByText(`staging.${subdomain}.example.com`)).toBeVisible();
    await page.getByRole('button', { name: 'Create application' }).click();
    await expect(page.locator('.form-success')).toContainText('Application created.');
    const appId = new URL(page.url()).pathname.match(/\/ui\/applications\/([0-9a-f-]{36})$/)?.[1];
    if (!appId) throw new Error(`unexpected Application URL ${page.url()}`);
    await expectApplicationHome(appId);
    expect(createBodies.map((body) => JSON.parse(body ?? '{}'))).toEqual([{ name, subdomain }]);

    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByLabel('Username')).toBeVisible();
    await expect(page).toHaveURL(/\/ui\/sign-in$/);
    await page.goto(`${baseURL}/ui/applications/${appId}`);
    await expect(page.getByLabel('Username')).toBeVisible();
    expect((await page.request.get(`${baseURL}/api/v1/applications`)).status()).toBe(401);

    await signIn();
    await expect(page.getByText(name)).toBeVisible();
    writeFileSync(env.ORCH_E2E_APP_FILE, appId, { mode: 0o600 });
  } else if (phase === 'restart') {
    const appId = readFileSync(env.ORCH_E2E_APP_FILE, 'utf8').trim();
    await signIn();
    await page.getByText(name).click();
    await expect(page).toHaveURL(new RegExp(`/ui/applications/${appId}$`));
    await expectApplicationHome(appId);
  } else {
    throw new Error(`unknown phase ${phase}`);
  }
  expect(mutations).toEqual([]);
  console.log(`onboarding ${phase}: ok`);
} finally {
  await browser.close();
}
