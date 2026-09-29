// Browser-driven UC-00/01/12/16/05/06 acceptance flow against a run-scoped kind app.
// Invoked by backend/test/integration/acceptance-playwright-kind.sh.
import { chromium, expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { writeFileSync } from 'node:fs';

const baseURL = process.env.ORCH_E2E_URL;
const runId = process.env.ORCH_E2E_RUN_ID;
const context = process.env.ORCH_E2E_KUBE_CONTEXT;
if (!baseURL || !/^[a-z0-9-]+$/.test(runId ?? '') || !context) throw new Error('ORCH_E2E_URL, ORCH_E2E_RUN_ID and ORCH_E2E_KUBE_CONTEXT are required');

const browser = await chromium.launch({ headless: true });
let forward;
try {
  const page = await browser.newPage();
  page.setDefaultTimeout(30_000);
  await page.goto(`${baseURL}/ui/sign-in`);
  await page.getByLabel('Username').fill('developer');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: /applications/i })).toBeVisible();
  await page.getByRole('button', { name: /create application|new application/i }).first().click();
  await page.getByLabel('Application name').fill(`Acceptance ${runId}`);
  await page.getByLabel('Subdomain').fill(runId);
  await page.getByRole('button', { name: 'Create application' }).click();
  await expect(page.getByRole('heading', { name: `Acceptance ${runId}` })).toBeVisible();
  const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
  const namespace = `app-${appId}-staging`;
  if (process.env.ORCH_E2E_NAMESPACE_FILE) writeFileSync(process.env.ORCH_E2E_NAMESPACE_FILE, namespace, { mode: 0o600 });

  const secret = randomBytes(32).toString('hex');
  const secretHash = createHash('sha256').update(secret).digest('hex');
  await page.getByRole('button', { name: 'Variables & Secrets' }).click();
  await putKey(page, 'variable', 'ACCEPTANCE_CONFIG', 'acceptance-config-ok');
  await putKey(page, 'variable', 'ACCEPTANCE_SECRET_SHA256', secretHash);
  await putKey(page, 'secret', 'ACCEPTANCE_SECRET', secret);
  await expect(page.locator('section[aria-label="Secrets"]')).not.toContainText(secret);
  await page.getByRole('button', { name: `← Acceptance ${runId}` }).click();

  const backendScore = {
    apiVersion: 'score.dev/v1b1', metadata: { name: 'backend' },
    containers: { main: { image: `acceptance-backend:${runId}`, variables: {
      PGHOST: '${resources.db.host}', PGPORT: '${resources.db.port}',
      PGDATABASE: '${resources.db.database}', PGUSER: '${resources.db.username}',
      PGPASSWORD: '${resources.db.password}',
      ACCEPTANCE_CONFIG: '${resources.env.ACCEPTANCE_CONFIG}',
      ACCEPTANCE_SECRET_SHA256: '${resources.env.ACCEPTANCE_SECRET_SHA256}',
      ACCEPTANCE_SECRET: '${resources.env.ACCEPTANCE_SECRET}',
    } } },
    service: { ports: { http: { port: 8080, targetPort: 8080 } } },
    resources: {
      db: { type: 'postgres', class: 'default', id: 'acceptance-db', params: { database: 'acceptance', username: 'acceptance' } },
      env: { type: 'environment' },
    },
  };
  const frontendScore = {
    apiVersion: 'score.dev/v1b1', metadata: { name: 'frontend' },
    containers: { main: { image: `acceptance-frontend:${runId}`, variables: { BACKEND_URL: '${resources.api.url}' } } },
    service: { ports: { http: { port: 8080, targetPort: 8080 } }, publicRoutes: [{ path: '/', port: 'http' }] },
    resources: { api: { type: 'service', params: { workload: 'backend', port: 'http' } } },
  };
  await uploadWorkload(page, backendScore);
  await uploadWorkload(page, frontendScore);

  await page.getByRole('button', { name: 'Preview changes' }).click();
  const preview = page.getByLabel('Deployment preview');
  await expect(preview).toContainText('2 workload(s) affected', { timeout: 120_000 });
  await page.getByRole('button', { name: 'Deploy these changes' }).click();
  await expect(page.getByLabel('Deployment result')).toContainText('Deploy succeeded', { timeout: 600_000 });
  await expect(page.getByLabel('Deployment result')).toContainText('backend: succeeded');
  await expect(page.getByLabel('Deployment result')).toContainText('frontend: succeeded');

  const port = await freePort();
  forward = spawn('kubectl', ['--context', context, '-n', namespace, 'port-forward', 'svc/frontend', `${port}:8080`], { stdio: ['ignore', 'pipe', 'pipe'] });
  let forwardLog = '';
  forward.stdout.on('data', (chunk) => { forwardLog += chunk.toString(); });
  forward.stderr.on('data', (chunk) => { forwardLog += chunk.toString(); });
  await expect.poll(() => forwardLog.includes(`Forwarding from 127.0.0.1:${port}`), { timeout: 30_000 }).toBe(true);
  const appPage = await browser.newPage();
  await appPage.goto(`http://127.0.0.1:${port}/`);
  await expect(appPage.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
  for (const name of ['backend connection', 'environment', 'secret', 'database']) {
    await expect(appPage.locator(`[data-check="${name}"]`)).toContainText('PASS');
  }
  const response = await appPage.request.get(`http://127.0.0.1:${port}/api/checks`);
  expect(response.ok()).toBeTruthy();
  const body = await response.json();
  expect(body.checks).toEqual({ environment: true, secret: true, database: true });
  expect(JSON.stringify(body)).not.toContain(secret);
  console.log(`PASS: application=${appId} namespace=${namespace} checks=backend,environment,secret,database`);
} finally {
  if (forward) { forward.kill(); await new Promise((resolve) => forward.once('exit', resolve)); }
  await browser.close();
}

async function putKey(page, kind, name, value) {
  const section = page.locator(`section[aria-label="${kind === 'secret' ? 'Secrets' : 'Environment variables'}"]`);
  await section.getByRole('button', { name: `+ Add ${kind}` }).click();
  await page.getByLabel('Key name').fill(name);
  await page.getByLabel(kind === 'secret' ? 'New secret value' : 'Value').fill(value);
  await page.getByRole('button', { name: 'Save pending change' }).click();
  await expect(section).toContainText(name);
}

async function uploadWorkload(page, score) {
  await page.getByRole('button', { name: '+ Add workload' }).click();
  await page.getByRole('button', { name: 'Import Score' }).click();
  await page.getByLabel('Score file').setInputFiles({ name: `${score.metadata.name}.json`, mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(score)) });
  await expect(page.getByText(`Parsed workload: ${score.metadata.name}`)).toBeVisible();
  await page.getByRole('button', { name: 'Save pending workload' }).click();
  await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible();
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
