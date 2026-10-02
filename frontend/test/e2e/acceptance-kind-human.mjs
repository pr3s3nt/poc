// Human-paced recording of the diagnostic acceptance app deployed through the
// Web Console on the existing kind cluster. Headed Chromium on a private Xvfb
// display; ffmpeg records the whole window including tabs and address bar.
// Invoked by backend/test/integration/acceptance-playwright-kind.sh --human.
//
// Every product mutation is a UI action: the cursor moves to each target,
// values are typed key by key (secrets in masked fields), native <select>
// popups and the address bar receive X11 keys. Application variables/secrets
// are ticked in the current per-container checklists (same names, no alias);
// resource outputs and the backend Service use Other sources. API replies are
// only read to assert what the UI saved. No worker is deployed, so the
// submitted job stays PENDING.
/* global document -- page.evaluate callbacks run in the browser. */
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { writeFileSync } from 'node:fs';
import { acceptanceApp, addWorkload, createApplication, createHuman, installCursor, previewAndDeploy, putKeys, reviewAcceptancePage, signIn, TYPE_DELAY } from './human.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_KUBE_CONTEXT', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_NAMESPACE_FILE', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const kubeContext = env.ORCH_E2E_KUBE_CONTEXT;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const videoPath = `${evidenceDir}/acceptance-review.mp4`;

const READ = 3000;
const SHORT_READ = 2000;

const browser = await launchHeaded({ width, height });
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(30_000);
await context.addInitScript(installCursor);
const marks = [];
let recorder;
let startedAt;
let forward;
try {
  const page = await context.newPage();
  const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
  const h = createHuman(page, { selectNative: nativeSelect(x11) });
  await page.goto(`${baseURL}/ui/sign-in`);
  await expect(page.getByLabel('Username')).toBeVisible();
  await page.waitForTimeout(1500);
  x11.focusBrowser();
  recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
  startedAt = Date.now();
  await page.waitForTimeout(1500);
  const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });

  // Synthetic per-run secret; only its SHA-256 is ever visible.
  const secret = randomBytes(32).toString('hex');
  const app = acceptanceApp(runId, secret, createHash('sha256').update(secret).digest('hex'));
  // Boolean checks: a failure message must not echo the secret.
  const noSecretOnScreen = async () => expect(await page.evaluate((value) => document.body.innerText.includes(value), secret), 'secret value on screen').toBe(false);

  // 1. Developer signs in and creates the Application.
  mark('sign-in');
  await signIn(h);
  const applicationName = `Acceptance ${runId}`;
  const appId = await createApplication(h, applicationName, runId);
  const namespace = `app-${appId}-staging`;
  writeFileSync(env.ORCH_E2E_NAMESPACE_FILE, namespace, { mode: 0o600 });
  mark('application-created');

  // 2. Variables & Secrets: each save completes before the next entry.
  await putKeys(h, applicationName, app.keys.map((key) => ({ ...key, delay: TYPE_DELAY })));
  await noSecretOnScreen();
  mark('settings-saved');

  // 3. backend: checklist keys + PostgreSQL outputs through Other sources.
  const [backend, frontend] = app.workloads;
  const backendScore = await addWorkload(h, backend);
  expect(backendScore.containers.main.variables).toEqual({
    ACCEPTANCE_CONFIG: '${resources.env.ACCEPTANCE_CONFIG}',
    ACCEPTANCE_SECRET_SHA256: '${resources.env.ACCEPTANCE_SECRET_SHA256}',
    ACCEPTANCE_SECRET: '${resources.env.ACCEPTANCE_SECRET}',
    PGHOST: '${resources.db.host}',
    PGPORT: '${resources.db.port}',
    PGDATABASE: '${resources.db.database}',
    PGUSER: '${resources.db.username}',
    PGPASSWORD: '${resources.db.password}',
  });
  expect(backendScore.resources).toEqual({
    db: { type: 'postgres', class: 'default', params: { database: 'acceptance', username: 'acceptance' } },
    env: { type: 'environment' },
  });
  expect(JSON.stringify(backendScore).includes(secret), 'secret value in Score').toBe(false);
  mark('backend-saved');

  // 4. Reopen backend: the saved Score restores the same selections.
  const workloadRow = (name) => page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) });
  await h.click(workloadRow('backend').getByRole('button', { name: 'Edit' }));
  await expect(page.getByRole('heading', { name: 'Edit backend' })).toBeVisible();
  const variables = page.getByRole('group', { name: 'Application variables for main', exact: true });
  const secrets = page.getByRole('group', { name: 'Application secrets for main', exact: true });
  for (const key of ['ACCEPTANCE_CONFIG', 'ACCEPTANCE_SECRET_SHA256']) await expect(variables.getByRole('checkbox', { name: key, exact: true })).toBeChecked();
  await expect(secrets.getByRole('checkbox', { name: 'ACCEPTANCE_SECRET', exact: true })).toBeChecked();
  await expect(page.getByRole('checkbox', { name: /^Use a different container name/ })).toHaveCount(3);
  for (const box of await page.getByRole('checkbox', { name: /^Use a different container name/ }).all()) await expect(box).not.toBeChecked();
  const otherNames = page.locator('.binding-row').getByLabel('Container variable name', { exact: true });
  await expect(otherNames).toHaveCount(5);
  // Score variables are a JSON object, so the reopened rows follow the saved
  // key order rather than the order they were added in.
  expect((await otherNames.evaluateAll((inputs) => inputs.map((input) => input.value))).sort()).toEqual(['PGDATABASE', 'PGHOST', 'PGPASSWORD', 'PGPORT', 'PGUSER']);
  await h.moveTo(secrets);
  mark('backend-selections-persisted');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: 'Cancel' }));
  await expect(page.getByRole('heading', { name: 'Workloads' })).toBeVisible();

  // 5. frontend: backend Service through Other sources, public path /.
  const frontendScore = await addWorkload(h, frontend);
  expect(frontendScore.containers.main.variables).toEqual({ BACKEND_URL: '${resources.svc_backend_http.url}' });
  expect(frontendScore.resources).toEqual({ svc_backend_http: { type: 'service', params: { workload: 'backend', port: 'http' } } });
  mark('frontend-saved');

  // Read-only check: the stored drafts equal the Scores the UI sent.
  const stored = await page.request.get(`${baseURL}/api/v1/applications/${encodeURIComponent(appId)}/environments/staging/workloads`);
  expect(stored.ok()).toBe(true);
  const drafts = Object.fromEntries((await stored.json()).workloads.map((item) => [item.id, item.score]));
  for (const [name, sent] of [['backend', backendScore], ['frontend', frontendScore]]) {
    expect(drafts[name]?.containers).toEqual(sent.containers);
    expect(drafts[name]?.resources).toEqual(sent.resources);
  }

  // 6. Preview and Deploy to kind.
  await previewAndDeploy(h, ['backend', 'frontend'], { mark });
  await noSecretOnScreen();

  // 7. Open the deployed app through a local port-forward (started off
  // screen) by typing its address, then check every row and submit a job.
  const port = await freePort();
  forward = spawn('kubectl', ['--context', kubeContext, '-n', namespace, 'port-forward', 'svc/frontend', `${port}:8080`], { stdio: ['ignore', 'pipe', 'pipe'] });
  let forwardLog = '';
  forward.stdout.on('data', (chunk) => { forwardLog += chunk.toString(); });
  forward.stderr.on('data', (chunk) => { forwardLog += chunk.toString(); });
  await expect.poll(() => forwardLog.includes(`Forwarding from 127.0.0.1:${port}`), { timeout: 30_000 }).toBe(true);
  const appURL = `http://127.0.0.1:${port}/`;
  x11.keys(['ctrl+l']);
  await h.pause(900);
  x11.type(appURL);
  await h.pause(SHORT_READ);
  x11.keys(['Return']);
  await expect(page).toHaveURL(appURL);
  mark('app-opened');
  await reviewAcceptancePage(h, runId, secret);
  const job = page.locator('#jobs tr.job').filter({ hasText: `hello from ${runId}` });
  await expect(job).toContainText('PENDING');
  await noSecretOnScreen();
  mark('job-submitted');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId: appId, namespace }, null, 2));
  console.log(`PASS: application=${appId} namespace=${namespace} checks=backend,environment,secret,database,job-submit marks=${marks.length}`);
} finally {
  if (forward && forward.exitCode === null && forward.signalCode === null) {
    forward.kill();
    await new Promise((resolve) => forward.once('exit', resolve));
  }
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}

async function freePort() {
  const server = createServer();
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const port = server.address().port;
  await new Promise((resolve) => server.close(resolve));
  return port;
}
