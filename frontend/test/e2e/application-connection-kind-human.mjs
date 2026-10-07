// Human-paced recording of UC-01 Application-level Connection selection on the
// existing kind cluster. Headed Chromium on a private Xvfb display; ffmpeg
// records the whole window including tabs and address bar. Invoked by
// backend/test/integration/application-connection-kind-video.sh.
//
// Every shown product mutation is a UI action: a Platform Engineer uploads a
// kubeconfig (new READY nondefault Connection) and registers the matching
// existing-cluster Resource Definition; a Developer creates an Application
// choosing that Connection in the native select, enters the diagnostic
// workloads through the workload form, then Previews and Deploys. API replies
// are read only to assert what the UI saved. Credential material is checked
// to never appear on screen or in any asserted response.
/* global document -- page.evaluate callbacks run in the browser. */
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { readFileSync, writeFileSync } from 'node:fs';
import { basename } from 'node:path';
import { acceptanceApp, addWorkload, createHuman, installCursor, putKeys, reviewAcceptancePage, TYPE_DELAY } from './human.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_KUBE_CONTEXT', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_NAMESPACE_FILE', 'ORCH_E2E_KUBECONFIG_FILE', 'ORCH_E2E_VIDEO_NAME', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const kubeContext = env.ORCH_E2E_KUBE_CONTEXT;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
const kubeconfigFile = env.ORCH_E2E_KUBECONFIG_FILE;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const videoPath = `${evidenceDir}/${env.ORCH_E2E_VIDEO_NAME}`;
const suffix = runId.replace(/[^0-9]/g, '').slice(-6);
const connectionName = `Uploaded kind ${suffix}`;
const connectionKey = connectionName.toLowerCase().replace(/[^a-z0-9]+/g, '-');
const definitionKey = `cluster-${connectionKey}`;
const defaultKey = 'internal-cluster';

// Credential values of the uploaded file: compared in-process only.
const secretValues = [...readFileSync(kubeconfigFile, 'utf8').matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)]
  .map((match) => match[1]).filter((value) => value.length >= 12);
if (!secretValues.length) throw new Error('uploaded kubeconfig carries no embedded credential');

const READ = 3000;
const LONG_READ = 4500;
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

  const assertNoCredentialOnPage = async () => {
    const html = await page.content();
    for (const value of secretValues) if (html.includes(value)) throw new Error('credential material is rendered in the page');
  };
  const assertNoCredentialInResponse = async (response) => {
    const text = await response.text();
    for (const value of secretValues) if (text.includes(value)) throw new Error(`credential material in ${new URL(response.url()).pathname} response`);
    return response;
  };
  async function signIn(user) {
    await h.showCursor();
    await h.pause(1200);
    await h.type(page.getByLabel('Username'), user);
    await h.type(page.getByLabel('Password'), 'test-password');
    await h.click(page.getByRole('button', { name: 'Sign in' }));
    await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
    await h.pause(SHORT_READ);
  }
  async function signOut() {
    await h.click(page.getByRole('button', { name: 'Sign out' }));
    await expect(page.getByLabel('Username')).toBeVisible();
    await h.pause(SHORT_READ);
  }
  const target = () => page.getByLabel('Execution target').first();
  const expectTarget = async (scope) => {
    await expect(scope.getByLabel('Execution target').first()).toContainText(`Connection ${connectionKey} · profile internal-k8s · both environments`);
  };

  // 1. Platform Engineer uploads a kubeconfig: a new READY Connection.
  mark('sign-in');
  await signIn('platform-engineer');
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  const connectionRows = () => page.getByRole('table', { name: 'Registered connections' }).locator('tbody tr');
  await expect(connectionRows().filter({ hasText: defaultKey })).toContainText('READY');
  await h.moveTo(connectionRows().first());
  mark('connections-list');
  await h.pause(READ);

  await h.type(page.getByLabel('Connection name'), connectionName);
  await h.click(page.getByLabel('Upload kubeconfig file'));
  const input = page.locator('input[type="file"]');
  const chooser = page.waitForEvent('filechooser');
  chooser.catch(() => {});
  await h.click(input);
  await (await chooser).setFiles(kubeconfigFile);
  await expect(page.getByText(`Selected file: ${basename(kubeconfigFile)}.`, { exact: false })).toBeVisible();
  await h.pause(800);
  const inspected = page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/connections/kubernetes/inspect');
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  const inspection = await assertNoCredentialInResponse(await inspected);
  expect(inspection.status()).toBe(200);
  const { contexts } = await inspection.json();
  expect(contexts.map((item) => item.name)).toEqual([kubeContext]);
  const summary = page.getByLabel('Selected destination');
  await expect(summary).toContainText(`Context ${kubeContext}`);
  await h.moveTo(summary);
  mark('kubeconfig-inspected');
  await h.pause(LONG_READ);
  await assertNoCredentialOnPage();

  const registered = page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 90_000 });
  await h.click(page.getByRole('button', { name: 'Check and save' }));
  const created = await (await assertNoCredentialInResponse(await registered)).json();
  expect(created).toMatchObject({ key: connectionKey, name: connectionName, kind: 'KUBERNETES', authenticationType: 'KUBECONFIG', status: 'READY' });
  expect(created.verification?.verified).toBe(true);
  await expect(page.getByRole('status')).toHaveText(`Registered connection ${connectionName} (${connectionKey}).`);
  const entry = connectionRows().filter({ has: page.getByText(connectionName, { exact: true }) });
  await expect(entry).toContainText('READY');
  await h.moveTo(entry);
  mark('connection-ready');
  await h.pause(LONG_READ);
  await assertNoCredentialOnPage();

  // 2. Matching existing-cluster Definition: Resource ID connections.<key>.
  await h.click(page.getByRole('link', { name: /Resource definitions/ }));
  await expect(page.getByRole('heading', { name: 'Resource definitions', level: 1 })).toBeVisible();
  await h.type(page.getByLabel('Definition ID'), definitionKey);
  await h.choose(page.getByLabel('Resource Type'), 'k8s-cluster');
  await h.choose(page.locator('select').filter({ has: page.locator('option[value="existing-cluster"]') }), 'Existing cluster');
  await h.type(page.getByLabel('Connection key'), connectionKey);
  await h.type(page.getByLabel('Criterion 1 Resource ID'), `connections.${connectionKey}`);
  await h.type(page.getByLabel('Criterion 1 Class'), 'internal');
  await h.type(page.getByLabel('Driver variables (JSON object)'), '{"name":"${context.connection.cluster}"}', { replace: true });
  mark('definition-filled');
  await h.pause(SHORT_READ);
  await h.click(page.getByRole('button', { name: 'Register resource definition' }));
  await expect(page.getByRole('status')).toHaveText(`Registered resource definition ${definitionKey}.`);
  const definitionEntry = page.locator('.catalog-entry').filter({ has: page.getByText(definitionKey, { exact: true }) });
  await expect(definitionEntry).toContainText('k8s-cluster · internal-k8s · existing-cluster · 1 criteria');
  await h.moveTo(definitionEntry);
  mark('definition-registered');
  await h.pause(LONG_READ);
  await signOut();
  mark('platform-engineer-signed-out');

  // 3. Developer creates the Application with the new nondefault Connection.
  await signIn('developer');
  await h.click(page.getByRole('button', { name: /create application|new application/i }).first());
  await expect(page.getByRole('heading', { name: 'Create an application' })).toBeVisible();
  const applicationName = `Connection ${runId}`;
  await h.type(page.getByLabel('Application name'), applicationName);
  await h.type(page.getByLabel('Subdomain'), runId);
  const select = page.getByLabel('Connection', { exact: true });
  await expect(select).toHaveValue(defaultKey);
  const options = await select.locator('option').allTextContents();
  const label = options.find((text) => text.includes(`(${connectionKey})`));
  expect(label, 'new Connection offered in the select').toBeTruthy();
  expect(options.some((text) => text.includes(`(${defaultKey})`) && text.includes('default'))).toBe(true);
  await h.moveTo(select);
  mark('connection-choices');
  await h.pause(READ);
  await h.choose(select, label);
  await expect(select).toHaveValue(connectionKey);
  await h.moveTo(select);
  mark('connection-selected');
  await h.pause(LONG_READ);
  const createRequest = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications');
  await h.click(page.getByRole('button', { name: 'Create application' }));
  expect((await createRequest).postDataJSON()).toEqual({ name: applicationName, subdomain: runId, connectionKey });
  await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
  const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
  const namespace = `app-${appId}-staging`;
  writeFileSync(env.ORCH_E2E_NAMESPACE_FILE, namespace, { mode: 0o600 });
  await expectTarget(page);
  await h.moveTo(target());
  mark('application-created');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: /Production/ }));
  await expectTarget(page);
  await h.moveTo(target());
  mark('production-same-binding');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: /Staging/ }));
  const view = (await (await page.request.get(`${baseURL}/api/v1/applications/${encodeURIComponent(appId)}`)).json()).application;
  expect(view.connectionKey).toBe(connectionKey);
  expect(view.executionProfile).toBe('internal-k8s');
  expect(JSON.stringify(view)).not.toMatch(/secret|kubeconfig|token/i);

  // 4. Variables, secrets and workloads through the current forms.
  const secret = randomBytes(32).toString('hex');
  const app = acceptanceApp(runId, secret, createHash('sha256').update(secret).digest('hex'));
  const noSecretOnScreen = async () => expect(await page.evaluate((value) => document.body.innerText.includes(value), secret), 'secret value on screen').toBe(false);
  await putKeys(h, applicationName, app.keys.map((key) => ({ ...key, delay: TYPE_DELAY })));
  await noSecretOnScreen();
  mark('settings-saved');
  const [backend, frontend] = app.workloads;
  const backendScore = await addWorkload(h, backend);
  expect(JSON.stringify(backendScore).includes(secret), 'secret value in Score').toBe(false);
  mark('backend-saved');
  const frontendScore = await addWorkload(h, frontend);
  expect(frontendScore.resources).toEqual({ svc_backend_http: { type: 'service', params: { workload: 'backend', port: 'http' } } });
  mark('frontend-saved');
  await expectTarget(page);

  // 5. Preview and Deploy show the persisted Connection, then deploy for real.
  await h.click(page.getByRole('button', { name: 'Preview changes' }));
  const preview = page.getByLabel('Deployment preview');
  await expect(preview).toContainText('2 workload(s) affected', { timeout: 120_000 });
  await expectTarget(preview);
  await h.moveTo(preview.getByLabel('Execution target'));
  mark('preview');
  await h.pause(4000);
  await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
  const result = page.getByLabel('Deployment result');
  await expect(result).toContainText('Deploy succeeded', { timeout: 600_000 });
  for (const name of ['backend', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [a-z]+: succeeded`) })).toHaveCount(1);
  await expectTarget(result);
  await h.moveTo(result.getByLabel('Execution target'));
  mark('deploy-succeeded');
  await h.pause(5000);
  await noSecretOnScreen();

  // 6. Open the deployed app through a local port-forward started off screen
  // and check every diagnostic row.
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
  await expect(page.locator('#jobs tr.job').filter({ hasText: `hello from ${runId}` })).toContainText('PENDING');
  await noSecretOnScreen();
  mark('job-submitted');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId: appId, namespace, connectionKey, defaultKey, definitionKey }, null, 2));
  console.log(`PASS: application=${appId} namespace=${namespace} connection=${connectionKey} checks=backend,environment,secret,database,job-submit marks=${marks.length}`);
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
