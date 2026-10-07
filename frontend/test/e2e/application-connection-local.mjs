// Local UC-01 Application connection selection verification. Headless Chromium
// drives the Web Console served by a local fake-adapter backend with JSON
// state. Invoked by backend/test/integration/application-connection-playwright-local.sh,
// once per phase: "create" before the backend restart and "restart" after it.
// A Platform Engineer session registers the second Connection and its matching
// cluster Definition through the same authenticated API the console uses; the
// Developer session then works only through the UI.
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
  async function expectTarget(label) {
    await expect(page.getByLabel('Execution target').first()).toContainText(`Connection ${labKey} · profile internal-k8s · both environments`, { timeout: 15_000 });
    return label;
  }
  async function previewScore(environment, workload = 'api') {
    await page.getByRole('button', { name: 'Preview Score' }).click();
    await expect(page.getByRole('heading', { name: 'Preview Score' })).toBeVisible();
    await expectTarget('score-preview');
    if (environment === 'production') await page.getByRole('tab', { name: 'Production' }).click();
    await page.getByLabel('Workload ID').fill(workload);
    await page.getByLabel('Run ID').fill('run-1');
    await page.getByLabel('Score after (YAML or JSON)').fill(scoreFor(workload));
    await page.getByRole('button', { name: 'Preview', exact: true }).click();
  }
  async function matchedDefinitions(environment, workload = 'api') {
    await previewScore(environment, workload);
    const region = page.getByRole('region', { name: 'Score preview result' });
    await expect(region).toContainText('Matched Resource Definitions');
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
    // Platform Engineer registers the second Connection (no matching Definition yet).
    await signIn(platformPage, 'platform-engineer');
    const registered = await platformPage.request.post(`${baseURL}/api/v1/connections/kubernetes`, { data: { key: labKey, clusterId: 'lab', kubeContext: 'lab-context' } });
    expect(registered.status()).toBe(201);
    await platformPage.getByRole('link', { name: /Connections/ }).click();
    await expect(platformPage.getByText(labKey).first()).toBeVisible();

    // Developer: default preselected, explicit non-default choice.
    await signIn(page, 'developer');
    await page.getByRole('button', { name: '+ Create application' }).first().click();
    const select = page.getByLabel('Connection');
    await expect(select).toHaveValue('internal-cluster');
    await expect(select.locator('option')).toHaveText([/internal-cluster.*default/, new RegExp(labKey)]);
    await page.getByLabel('Application name').fill(name);
    await page.getByLabel('Subdomain').fill(subdomain);
    await select.selectOption(labKey);
    await shot('create-form');
    await page.getByRole('button', { name: 'Create application' }).click();
    await expect(page.getByText('Application created.')).toBeVisible();
    const appId = new URL(page.url()).pathname.match(/\/ui\/applications\/([0-9a-f-]{36})$/)?.[1];
    if (!appId) throw new Error(`unexpected Application URL ${page.url()}`);
    expect(createBodies.map((body) => JSON.parse(body ?? '{}'))).toEqual([{ name, subdomain, connectionKey: labKey }]);
    await expectTarget('home');
    await page.getByRole('button', { name: /Production/ }).click();
    await expectTarget('home-production');
    await shot('home');
    const view = (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}`)).json()).application;
    expect(view.connectionKey).toBe(labKey);
    expect(view.executionProfile).toBe('internal-k8s');
    expect(JSON.stringify(view)).not.toMatch(/secret|kubeconfig|token/i);
    await page.getByRole('button', { name: /Staging/ }).click();

    // Negative: the seeded internal-cluster Definition must not silently
    // retarget this Application before a matching Definition is registered.
    await previewScore('staging');
    await expect(page.getByRole('alert')).toContainText('Resource Definitions');
    await expect(page.getByRole('region', { name: 'Score preview result' })).toHaveCount(0);
    await shot('preview-before-definition');
    await page.getByRole('button', { name: `← ${name}` }).click();
    await saveWorkload('staging');
    await page.getByRole('button', { name: 'Preview changes' }).click();
    await expect(page.getByRole('alert')).toBeVisible();
    await expect(page.getByLabel('Deployment preview')).toHaveCount(0);
    expect((await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/deployments`)).json()).deployments ?? []).toHaveLength(0);
    await shot('pending-preview-blocked');

    // Platform Engineer registers the matching cluster Definition.
    const definition = await platformPage.request.post(`${baseURL}/api/v1/resource-definitions`, { data: {
      key: `cluster-${labKey}`, resourceType: 'k8s-cluster', executionProfile: 'internal-k8s', driverType: 'existing-cluster', connectionKey: labKey,
      driverInputs: { values: { variables: { name: '${context.connection.cluster}', kubeContext: '${context.connection.context}' } } },
      criteria: [{ class: 'internal', res_id: `connections.${labKey}` }],
    } });
    expect(definition.status()).toBe(201);

    // Developer: Score Preview matches the new Definition in both Environments.
    for (const environment of ['staging', 'production']) {
      await page.goto(`${baseURL}/ui/applications/${appId}`);
      const matches = await matchedDefinitions(environment);
      await expect(matches).toContainText(`cluster-${labKey}`);
      await expect(matches).not.toContainText('cluster-internal-registered');
      if (environment === 'staging') await shot('score-preview-matched');
    }

    // Pending Preview/Deploy succeed on the selected target in both Environments.
    for (const environment of ['staging', 'production']) {
      await page.goto(`${baseURL}/ui/applications/${appId}`);
      if (environment === 'production') {
        await page.getByRole('button', { name: /Production/ }).click();
        await saveWorkload(environment);
      }
      await page.getByRole('button', { name: 'Preview changes' }).click();
      const preview = page.getByLabel('Deployment preview');
      await expect(preview).toContainText('1 workload(s) affected');
      await expect(preview.getByLabel('Execution target')).toContainText(`Connection ${labKey}`);
      await page.getByRole('button', { name: 'Deploy these changes' }).click();
      const result = page.getByLabel('Deployment result');
      await expect(result).toContainText('Deploy succeeded', { timeout: 60_000 });
      await expect(result.getByLabel('Execution target')).toContainText(`Connection ${labKey}`);
      await shot(`deployed-${environment}`);
    }
    writeFileSync(env.ORCH_E2E_APP_FILE, appId, { mode: 0o600 });
  } else if (phase === 'restart') {
    const appId = readFileSync(env.ORCH_E2E_APP_FILE, 'utf8').trim();
    await signIn(page, 'developer');
    await page.getByText(name).click();
    await expect(page).toHaveURL(new RegExp(`/ui/applications/${appId}$`));
    await expectTarget('home');
    await page.getByRole('button', { name: /Production/ }).click();
    await expectTarget('home-production');
    const view = (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}`)).json()).application;
    expect(view.connectionKey).toBe(labKey);
    const matches = await matchedDefinitions('staging', 'probe');
    await expect(matches).toContainText(`cluster-${labKey}`);
    await shot('restart-score-preview');
    // Persisted Active Resource evidence: the cluster ran on the selected Connection only.
    const raw = readFileSync(env.ORCH_E2E_STATE_FILE, 'utf8');
    const active = Object.entries(JSON.parse(raw).activeResources).filter(([key]) => key.includes(appId));
    expect(active.map(([key]) => key.split('|')[1].split('#')[0]).sort()).toEqual(['k8s-cluster.internal', 'k8s-namespace.default', 'k8s-namespace.default']);
    for (const [key, resource] of active) expect(resource.connectionKey, key).toBe(labKey);
    expect(raw).not.toMatch(/BEGIN [A-Z ]*PRIVATE KEY/);
  } else {
    throw new Error(`unknown phase ${phase}`);
  }
  console.log(`application connection ${phase}: ok`);
} finally {
  await browser.close();
}
