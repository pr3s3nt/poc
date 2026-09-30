// Local UC-09 verification. Chromium uses the Web Console and its same-origin
// API against a fake-adapter backend with a temporary JSON state file.
import { chromium, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_PHASE', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_DATA_FILE']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const name = `Observe ${env.ORCH_E2E_RUN_ID}`;
const subdomain = env.ORCH_E2E_RUN_ID;

const browser = await chromium.launch({ headless: true });
try {
  const context = await browser.newContext();
  context.setDefaultTimeout(15_000);
  const page = await context.newPage();
  const requestedURLs = [];
  page.on('request', (request) => requestedURLs.push(request.url()));

  async function signIn() {
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.getByLabel('Username').fill('developer');
    await page.getByLabel('Password').fill('test-password');
    await page.getByRole('button', { name: 'Sign in' }).click();
    await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  }

  async function api(method, path, data) {
    const response = await page.request.fetch(`${baseURL}/api/v1${path}`, { method, data });
    const body = await response.json().catch(() => ({}));
    return { response, body };
  }

  async function workloadSnapshot(id) {
    const detail = await api('GET', `/applications/${detailPath(id)}`);
    expect(detail.response.status()).toBe(200);
    expect(detail.body.workloads).toHaveLength(1);
    return detail.body.workloads[0];
  }

  function detailPath(id) {
    return `${currentData.applicationId}/environments/staging/deployments/${id}`;
  }

  let currentData;

  async function verifyHistory(data) {
    currentData = data;
    await page.goto(`${baseURL}/ui/applications/${data.applicationId}/environments/staging/deployments`);
    await expect(page.getByRole('heading', { name: 'Deployments' })).toBeVisible();
    // Scope every assertion to deployment entries: the status filter options
    // carry the same words as the status badges.
    const entries = page.locator('button.deployment-entry');
    await expect(entries).toHaveCount(3);
    const backendEntries = entries.filter({ hasText: 'backend' });
    const brokenEntry = entries.filter({ hasText: 'broken' });
    await expect(backendEntries).toHaveCount(2);
    for (const entry of await backendEntries.all()) {
      await expect(entry.locator('.status')).toHaveText('SUCCEEDED');
    }
    await expect(brokenEntry).toHaveCount(1);
    await expect(brokenEntry.locator('.status')).toHaveText('FAILED');

    await page.getByLabel('Filter deployment status').selectOption('FAILED');
    await expect.poll(() => requestedURLs.some((url) => url.endsWith('/deployments?status=FAILED'))).toBe(true);
    await expect(entries).toHaveCount(1);
    await expect(entries.first()).toContainText('broken');
    await expect(entries.first().locator('.status')).toHaveText('FAILED');

    await brokenEntry.click();
    await expect(page).toHaveURL(new RegExp(`/deployments/${data.failedDeploymentId}$`));
    await expect(page.getByRole('heading', { name: 'broken · deploy' })).toBeVisible();
    await expect(page.getByText('No provision plan was saved for this deployment.')).toBeVisible();
    await expect(page.getByText('No workload status was recorded.')).toBeVisible();

    // Each backend run shows its own workload snapshot, never the newer one.
    for (const [id, digest] of [[data.firstDeploymentId, data.firstDigest], [data.secondDeploymentId, data.secondDigest]]) {
      await page.goto(`${baseURL}/ui/applications/${detailPath(id)}`);
      await expect(page.getByRole('heading', { name: /^backend · (deploy|update)$/ })).toBeVisible();
      await expect(page.getByText(id, { exact: true })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Resources' })).toBeVisible();
      await expect(page.getByText(/password=\*\*\*redacted\*\*\*/)).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Provision plan' })).toBeVisible();
      await expect(page.getByText(digest, { exact: true })).toBeVisible();

      const snapshot = await workloadSnapshot(id);
      expect(snapshot.lastDeploymentId).toBe(id);
      expect(snapshot.manifestDigest).toBe(digest);
      expect(snapshot.status).toBe('READY');
    }

    const detail = await api('GET', `/applications/${detailPath(data.firstDeploymentId)}`);
    const serialized = JSON.stringify(detail.body);
    expect(serialized).not.toContain('resolvedInputs');
    expect(serialized).not.toContain('raw-secret');
    expect(serialized).toContain('***redacted***');
  }

  await signIn();
  if (env.ORCH_E2E_PHASE === 'create') {
    const created = await api('POST', '/applications', { name, subdomain });
    expect(created.response.status()).toBe(201);
    const applicationId = created.body.application.key;

    const samples = await api('GET', '/score-samples');
    expect(samples.response.status()).toBe(200);
    const deployBackend = async (score, scoreBefore) => {
      const result = await api('POST', '/deployments', {
        applicationKey: applicationId, environmentKey: 'staging', workloadId: 'backend',
        actor: 'uc09-browser', score, ...(scoreBefore ? { scoreBefore } : {}),
      });
      expect(result.response.status(), JSON.stringify(result.body)).toBe(201);
      expect(result.body.status).toBe('SUCCEEDED');
      return result.body.deploymentId;
    };
    const firstDeploymentId = await deployBackend(samples.body.samples.backend);
    currentData = { applicationId };
    const firstBefore = await workloadSnapshot(firstDeploymentId);

    const planningFailure = await api('POST', '/deployments', {
      applicationKey: applicationId, environmentKey: 'staging', workloadId: 'broken', actor: 'uc09-browser',
      score: {
        apiVersion: 'score.dev/v1b1', metadata: { name: 'broken' },
        containers: { main: { image: 'example.invalid/broken:test' } },
        resources: { unsupported: { type: 'missing-resource-type' } },
      },
    });
    expect(planningFailure.response.status()).not.toBe(201);

    // Redeploy the same workload with a different manifest.
    const changed = structuredClone(samples.body.samples.backend);
    changed.containers.main.variables = { ...changed.containers.main.variables, UC09_REVISION: env.ORCH_E2E_RUN_ID };
    const secondDeploymentId = await deployBackend(changed, samples.body.samples.backend);
    const second = await workloadSnapshot(secondDeploymentId);
    expect(second.manifestDigest).not.toBe(firstBefore.manifestDigest);
    const firstAfter = await workloadSnapshot(firstDeploymentId);
    expect(firstAfter).toEqual(firstBefore);

    const history = await api('GET', `/applications/${applicationId}/environments/staging/deployments`);
    expect(history.response.status()).toBe(200);
    expect(history.body.deployments.map((item) => item.id)[0]).toBe(secondDeploymentId);
    const failed = history.body.deployments.find((item) => item.workloadId === 'broken');
    expect(failed?.status).toBe('FAILED');
    const data = {
      applicationId, failedDeploymentId: failed.id,
      firstDeploymentId, firstDigest: firstBefore.manifestDigest, firstSnapshot: firstBefore,
      secondDeploymentId, secondDigest: second.manifestDigest,
    };
    writeFileSync(env.ORCH_E2E_DATA_FILE, JSON.stringify(data), { mode: 0o600 });
    await verifyHistory(data);

    await page.getByRole('button', { name: 'Sign out' }).click();
    await expect(page.getByLabel('Username')).toBeVisible();
    const anonymous = await page.request.get(`${baseURL}/api/v1/applications/${applicationId}/environments/staging/deployments`);
    expect(anonymous.status()).toBe(401);
  } else if (env.ORCH_E2E_PHASE === 'restart') {
    const data = JSON.parse(readFileSync(env.ORCH_E2E_DATA_FILE, 'utf8'));
    await verifyHistory(data);
    // The old run's snapshot is byte-for-byte what it was before the redeploy.
    expect(await workloadSnapshot(data.firstDeploymentId)).toEqual(data.firstSnapshot);
  } else {
    throw new Error(`unknown phase ${env.ORCH_E2E_PHASE}`);
  }
  console.log(`uc09 ${env.ORCH_E2E_PHASE}: ok`);
} finally {
  await browser.close();
}
