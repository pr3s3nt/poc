// Local UC-01 per-Environment connection (ADR-011) verification. Headless Chromium
// drives the Web Console served by a local fake-adapter backend with JSON
// state. Invoked by backend/test/integration/application-connection-playwright-local.sh,
// once per phase: "create" before the backend restart and "restart" after it.
// A Platform Engineer session registers the second Connection (no cluster
// Definition is needed, ADR-013) through the same authenticated API the console uses; the
// Developer session then works only through the UI. The set-once rule is also
// probed through the API (negative requests only, never as a fixture).
import { chromium, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_PHASE', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_APP_FILE', 'ORCH_E2E_STATE_FILE', 'ORCH_E2E_SHOTS']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const phase = env.ORCH_E2E_PHASE;
const runId = env.ORCH_E2E_RUN_ID;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const name = `Lab App ${runId}`;
const subdomain = runId;
const labKey = 'lab-cluster';
const byKey = (view, pick) => Object.fromEntries(view.environments.map((item) => [item.key, pick(item)]));
const scoreFor = (workload) => `apiVersion: score.dev/v1b1\nmetadata:\n  name: ${workload}\ncontainers:\n  main:\n    image: example.invalid/api:v1\n`;

const browser = await chromium.launch({ headless: true });
try {
  const platform = await browser.newContext();
  const developer = await browser.newContext();
  for (const context of [platform, developer]) context.setDefaultTimeout(15_000);
  const platformPage = await platform.newPage();
  const page = await developer.newPage();
  const createBodies = [];
  page.on('request', (request) => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications') createBodies.push(request.postData());
  });
  const shot = (label) => page.screenshot({ path: `${env.ORCH_E2E_SHOTS}/${phase}-${label}.png`, fullPage: true });

  async function signIn(target, username) {
    await target.goto(`${baseURL}/ui/applications`);
    await target.getByLabel('Username').fill(username);
    await target.getByLabel('Password').fill('test-password');
    await target.getByRole('button', { name: 'Sign in' }).click();
    await expect(target.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  }
  const targetText = (environment, key) => `${environment === 'staging' ? 'Staging' : 'Production'} · Connection ${key} (${key}) · profile internal-k8s`;
  async function expectTarget(label, environment = 'staging', key = labKey) {
    await expect(page.getByLabel('Execution target').first()).toContainText(targetText(environment, key), { timeout: 15_000 });
    return label;
  }
  async function setConnection(environment, key) {
    await page.getByRole('button', { name: 'Environment settings' }).click();
    await expect(page.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
    if (environment === 'production') await page.getByRole('tab', { name: 'Production' }).click();
    const title = environment === 'staging' ? 'Staging' : 'Production';
    const select = page.getByLabel(`Connection for ${title}`);
    await expect(select).toHaveValue('');
    await select.selectOption(key);
    const put = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname.endsWith(`/environments/${environment}/connection`));
    await page.getByRole('button', { name: 'Save connection' }).click();
    expect((await put).status()).toBe(200);
    // Saved, not locked: the selection stays editable and an unchanged one has nothing to save.
    await expect(page.getByText('Generation 0', { exact: true })).toBeVisible();
    await expect(page.getByLabel(`Connection for ${title}`)).toBeEnabled();
    await expect(page.getByLabel(`Connection for ${title}`)).toHaveValue(key);
    await expect(page.getByRole('button', { name: 'Save connection' })).toBeDisabled();
  }
  async function previewScore(environment, workload = 'api') {
    await page.getByRole('button', { name: 'Preview Score' }).click();
    await expect(page.getByRole('heading', { name: 'Preview Score' })).toBeVisible();
    // The page opens on the Environment tab selected at home; switch when needed.
    const tab = page.getByRole('tab', { name: environment === 'production' ? 'Production' : 'Staging' });
    if ((await tab.getAttribute('aria-selected')) !== 'true') await tab.click();
    await expectTarget('score-preview', environment, environment === 'production' ? 'internal-cluster' : labKey);
    await page.getByLabel('Workload ID').fill(workload);
    await page.getByLabel('Run ID').fill('run-1');
    await page.getByLabel('Score after (YAML or JSON)').fill(scoreFor(workload));
    await page.getByRole('button', { name: 'Preview', exact: true }).click();
  }
  async function matchedDefinitions(environment, workload = 'api') {
    await previewScore(environment, workload);
    const region = page.getByRole('region', { name: 'Score preview result' });
    await expect(region).toContainText('Resource execution bindings');
    return region;
  }
  async function saveWorkload(environment) {
    await page.getByRole('button', { name: '+ Add workload' }).click();
    await page.getByLabel('Workload name', { exact: true }).fill('api');
    await page.getByLabel('Image', { exact: true }).fill('example.invalid/api:v1');
    await page.getByRole('button', { name: '+ Add port' }).click();
    await page.getByLabel('Service port name', { exact: true }).fill('http');
    await page.locator('input[aria-label="Service port"]').fill('8080');
    await page.getByLabel('Container target port', { exact: true }).fill('8080');
    await page.getByRole('button', { name: 'Save pending workload' }).click();
    await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible();
    // The home page reopens on staging after a save.
    if (environment === 'production') await page.getByRole('button', { name: /Production/ }).click();
    await expect(page.locator('.table-row').filter({ hasText: 'api' })).toContainText('Pending change');
    return environment;
  }

  if (phase === 'create') {
    // Platform Engineer registers the second Connection (no cluster Definition is needed).
    await signIn(platformPage, 'platform-engineer');
    const registered = await platformPage.request.post(`${baseURL}/api/v1/connections/kubernetes`, { data: { key: labKey, clusterId: 'lab', kubeContext: 'lab-context' } });
    expect(registered.status()).toBe(201);
    await platformPage.getByRole('link', { name: /Connections/ }).click();
    await expect(platformPage.getByText(labKey).first()).toBeVisible();

    // Developer: the create form asks for name and subdomain only.
    await signIn(page, 'developer');
    await page.getByRole('button', { name: '+ Create application' }).first().click();
    await expect(page.getByLabel('Connection')).toHaveCount(0);
    await page.getByLabel('Application name').fill(name);
    await page.getByLabel('Subdomain').fill(subdomain);
    await shot('create-form');
    await page.getByRole('button', { name: 'Create application' }).click();
    await expect(page.getByText('Application created.')).toBeVisible();
    const appId = new URL(page.url()).pathname.match(/\/ui\/applications\/([0-9a-f-]{36})$/)?.[1];
    if (!appId) throw new Error(`unexpected Application URL ${page.url()}`);
    expect(createBodies.map((body) => JSON.parse(body ?? '{}'))).toEqual([{ name, subdomain }]);
    const apiView = async () => (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}`)).json()).application;
    let view = await apiView();
    expect(byKey(view, (item) => [item.configured, item.runtimeStatus])).toEqual({ staging: [false, 'UNCONFIGURED'], production: [false, 'UNCONFIGURED'] });
    // secretStoreKey is the public Secret Store selection (a key, never a credential), so only that field name is exempt.
    expect(JSON.stringify(view).replaceAll('"secretStoreKey"', '')).not.toMatch(/secret|kubeconfig|token/i);
    await expect(page.getByLabel('Execution target').first()).toContainText('Staging has no execution connection yet');
    await page.getByRole('button', { name: /Production/ }).click();
    await expect(page.getByLabel('Execution target').first()).toContainText('Production has no execution connection yet');
    await shot('home-unconfigured');
    await page.getByRole('button', { name: /Staging/ }).click();

    // UNCONFIGURED Preview is a safe 422 with guidance; drafts still work.
    await saveWorkload('staging');
    await page.getByRole('button', { name: 'Preview changes' }).click();
    await expect(page.getByRole('alert')).toContainText('set one in Environment Settings');
    await expect(page.getByLabel('Deployment preview')).toHaveCount(0);
    await shot('preview-unconfigured');
    expect((await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/deployments`)).json()).deployments ?? []).toHaveLength(0);

    // Staging selects the nondefault lab Connection; production stays unset.
    await page.getByRole('button', { name: 'Open Environment settings' }).click();
    const select = page.getByLabel('Connection for Staging');
    await expect(select).toHaveValue('');
    await expect(select.locator('option')).toHaveText([/Choose a connection/, /internal-cluster \(internal-cluster\)/, new RegExp(labKey)]);
    await select.selectOption(labKey);
    const staged = page.waitForResponse((response) => response.request().method() === 'PUT');
    await page.getByRole('button', { name: 'Save connection' }).click();
    expect((await staged).status()).toBe(200);
    await expect(page.getByText('Generation 0', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Connection for Staging')).toBeEnabled();
    await shot('staging-saved');
    await page.getByRole('tab', { name: 'Production' }).click();
    await expect(page.getByLabel('Connection for Production')).toHaveValue('');
    view = await apiView();
    expect(byKey(view, (item) => item.connectionKey)).toEqual({ staging: labKey, production: '' });
    // Versions are enforced by the API (probes that change nothing): a stale version is
    // refused, and the same selection at the current version is an idempotent no-op.
    const savedStaging = view.environments.find((item) => item.key === 'staging');
    const stagingConnectionURL = `${baseURL}/api/v1/applications/${appId}/environments/staging/connection`;
    const stale = await page.request.put(stagingConnectionURL, { data: { connectionKey: 'internal-cluster', expectedVersion: savedStaging.version - 1 } });
    expect(stale.status()).toBe(409);
    expect((await stale.json()).code).toBe('STALE_VERSION');
    const noop = await page.request.put(stagingConnectionURL, { data: { connectionKey: labKey, expectedVersion: savedStaging.version } });
    expect(noop.status()).toBe(200);
    expect((await apiView()).environments.find((item) => item.key === 'staging')).toMatchObject({ connectionKey: labKey, version: savedStaging.version });
    await page.getByRole('button', { name: `← ${name}` }).click();
    await expectTarget('home');

    // ADR-013: no cluster Definition is registered; staging previews and plans
    // the implicit cluster from its Environment Connection.
    await previewScore('staging');
    await expect(page.getByRole('region', { name: 'Score preview result' })).toContainText(`Environment connection ${labKey}`);
    await shot('preview-environment-connection');
    await page.getByRole('button', { name: `← ${name}` }).click();

    // Staging preview binds the lab Connection and deploys on lab only.
    await page.goto(`${baseURL}/ui/applications/${appId}`);
    const matches = await matchedDefinitions('staging');
    await expect(matches).toContainText(`Environment connection ${labKey}`);
    await expect(matches).not.toContainText('internal-cluster');
    await shot('score-preview-matched');
    await page.goto(`${baseURL}/ui/applications/${appId}`);
    await page.getByRole('button', { name: 'Preview changes' }).click();
    const preview = page.getByLabel('Deployment preview');
    await expect(preview).toContainText('1 workload(s) affected');
    await expect(preview.getByLabel('Execution target')).toContainText(targetText('staging', labKey));
    await page.getByRole('button', { name: 'Deploy these changes' }).click();
    const result = page.getByLabel('Deployment result');
    await expect(result).toContainText('Deploy succeeded', { timeout: 60_000 });
    await expect(result.getByLabel('Execution target')).toContainText(targetText('staging', labKey));
    await shot('deployed-staging');

    // Production independently chooses the default cluster Connection and
    // plans from its own Environment Connection, not the lab one.
    await page.getByRole('button', { name: /Production/ }).click();
    await setConnection('production', 'internal-cluster');
    await page.getByRole('button', { name: `← ${name}` }).click();
    await page.getByRole('button', { name: /Production/ }).click();
    await expectTarget('home-production', 'production', 'internal-cluster');
    await shot('production-saved');
    const productionMatches = await matchedDefinitions('production');
    await expect(productionMatches).toContainText('Environment connection internal-cluster');
    await expect(productionMatches).not.toContainText(labKey);
    await shot('production-score-preview');
    view = await apiView();
    expect(byKey(view, (item) => [item.connectionKey, item.configured])).toEqual({ staging: [labKey, true], production: ['internal-cluster', true] });
    writeFileSync(env.ORCH_E2E_APP_FILE, appId, { mode: 0o600 });
  } else if (phase === 'restart') {
    const appId = readFileSync(env.ORCH_E2E_APP_FILE, 'utf8').trim();
    await signIn(page, 'developer');
    await page.getByText(name).click();
    await expect(page).toHaveURL(new RegExp(`/ui/applications/${appId}$`));
    await expectTarget('home');
    await page.getByRole('button', { name: /Production/ }).click();
    await expectTarget('home-production', 'production', 'internal-cluster');
    const view = (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}`)).json()).application;
    expect(byKey(view, (item) => [item.connectionKey, item.infrastructureScope])).toEqual({ staging: [labKey, 'ENVIRONMENT'], production: ['internal-cluster', 'ENVIRONMENT'] });
    // The saved selection survives a restart and stays editable (no runtime exists yet).
    await page.getByRole('button', { name: 'Environment settings' }).click();
    await page.getByRole('tab', { name: 'Staging' }).click();
    await expect(page.getByText('Generation 0', { exact: true })).toBeVisible();
    await expect(page.getByLabel('Connection for Staging')).toHaveValue(labKey);
    await expect(page.getByLabel('Connection for Staging')).toBeEnabled();
    await page.getByRole('button', { name: `← ${name}` }).click();
    await page.getByRole('button', { name: /Staging/ }).click();
    const matches = await matchedDefinitions('staging', 'probe');
    await expect(matches).toContainText(`Environment connection ${labKey}`);
    await shot('restart-score-preview');
    // Persisted Active Resource evidence: staging's cluster ran on lab only.
    const raw = readFileSync(env.ORCH_E2E_STATE_FILE, 'utf8');
    const state = JSON.parse(raw);
    const active = Object.entries(state.activeResources).filter(([key]) => key.includes(appId) || key.includes('connections.'));
    const cluster = active.find(([key]) => key.includes('k8s-cluster.internal#connections.' + labKey));
    expect(cluster?.[1].connectionKey).toBe(labKey);
    expect(Object.keys(state.activeResources).some((key) => key.includes('connections.internal-cluster') && key.includes(appId))).toBe(false);
    expect(raw).not.toMatch(/BEGIN [A-Z ]*PRIVATE KEY/);
  } else {
    throw new Error(`unknown phase ${phase}`);
  }
  console.log(`environment connection ${phase}: ok`);
} finally {
  await browser.close();
}
