// Human-paced recording of UC-01 per-Environment Connection selection (ADR-011)
// on the existing kind cluster. Headed Chromium on a private Xvfb display; ffmpeg
// records the whole window including tabs and address bar. Invoked by
// backend/test/integration/application-connection-kind-video.sh.
//
// Every shown product mutation is a UI action: a Platform Engineer uploads the
// kubeconfig twice (two READY nondefault logical Connections that point at the
// same physical cluster, an honest limit of this proof; no cluster Definition is
// needed, ADR-013); a Developer
// creates an UNCONFIGURED Application, sees Preview refuse it, then sets
// staging and production to different Connections in Environment Settings (the
// selection stays editable, survives a page refresh and a backend restart, and an
// empty production Environment is rebound; once staging has runtime resources a
// different destination needs a reviewed transition, never a plain Save),
// enters the diagnostic workloads through the workload form, then Previews and
// Deploys staging. API calls are only used to assert what the UI saved and for
// no-state-change probes (stale version, idempotent repeat, runtime-exists) that
// are never part of the shown flow. Credential
// material is checked to never appear on screen or in any asserted response.
/* global document -- page.evaluate callbacks run in the browser. */
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { spawn } from 'node:child_process';
import { createServer } from 'node:net';
import { readFileSync, writeFileSync } from 'node:fs';
import { basename } from 'node:path';
import { acceptanceApp, addWorkload, createHuman, installCursor, putKeys, reviewAcceptancePage, TYPE_DELAY } from './human.mjs';
import { existsSync } from 'node:fs';
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
// The first word of a name is what the native select types ahead, so the two
// logical Connections start with different words.
const stagingName = `Staging kind ${suffix}`;
const productionName = `Production kind ${suffix}`;
const slug = (name) => name.toLowerCase().replace(/[^a-z0-9]+/g, '-');
const stagingKey = slug(stagingName);
const productionKey = slug(productionName);
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
  const targetText = (environment, name, key) => `${environment === 'staging' ? 'Staging' : 'Production'} · Connection ${name} (${key}) · profile internal-k8s`;
  const expectTarget = async (scope, environment, name, key) => {
    await expect(scope.getByLabel('Execution target').first()).toContainText(targetText(environment, name, key));
  };

  // Uploads the selected-context kubeconfig as a new READY Connection.
  async function uploadConnection(connectionName, key, tag) {
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
    mark(`${tag}-inspected`);
    await h.pause(READ);
    await assertNoCredentialOnPage();
    const registered = page.waitForResponse((response) => response.request().method() === 'POST'
      && new URL(response.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 90_000 });
    await h.click(page.getByRole('button', { name: 'Check and save' }));
    const created = await (await assertNoCredentialInResponse(await registered)).json();
    expect(created).toMatchObject({ key, name: connectionName, kind: 'KUBERNETES', authenticationType: 'KUBECONFIG', status: 'READY' });
    expect(created.verification?.verified).toBe(true);
    await expect(page.getByRole('status')).toHaveText(`Registered connection ${connectionName} (${key}).`);
    const entry = page.getByRole('table', { name: 'Registered connections' }).locator('tbody tr').filter({ has: page.getByText(connectionName, { exact: true }) });
    await expect(entry).toContainText('READY');
    await h.moveTo(entry);
    mark(`${tag}-ready`);
    await h.pause(READ);
    await assertNoCredentialOnPage();
  }

  // 1. Platform Engineer uploads two logical Connections (same physical cluster).
  mark('sign-in');
  await signIn('platform-engineer');
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  const connectionRows = () => page.getByRole('table', { name: 'Registered connections' }).locator('tbody tr');
  await expect(connectionRows().filter({ hasText: defaultKey })).toContainText('READY');
  await h.moveTo(connectionRows().first());
  mark('connections-list');
  await h.pause(READ);
  await uploadConnection(stagingName, stagingKey, 'staging-connection');
  await uploadConnection(productionName, productionKey, 'production-connection');

  // 2. No cluster Definition is registered: the Environment Connection backs the
  //    implicit cluster node (ADR-013).
  await signOut();
  mark('platform-engineer-signed-out');

  // 3. Developer creates an Application: name and subdomain only.
  await signIn('developer');
  await h.click(page.getByRole('button', { name: /create application|new application/i }).first());
  await expect(page.getByRole('heading', { name: 'Create an application' })).toBeVisible();
  const applicationName = `Environment ${runId}`;
  await h.type(page.getByLabel('Application name'), applicationName);
  await h.type(page.getByLabel('Subdomain'), runId);
  await expect(page.getByLabel('Connection', { exact: true })).toHaveCount(0);
  await h.moveTo(page.getByText(/Choose each environment's connection afterwards/));
  mark('create-form-no-connection');
  await h.pause(READ);
  const createRequest = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications');
  await h.click(page.getByRole('button', { name: 'Create application' }));
  expect((await createRequest).postDataJSON()).toEqual({ name: applicationName, subdomain: runId });
  await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
  const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
  const namespace = `app-${appId}-staging`;
  writeFileSync(env.ORCH_E2E_NAMESPACE_FILE, namespace, { mode: 0o600 });
  const apiView = async () => (await (await page.request.get(`${baseURL}/api/v1/applications/${encodeURIComponent(appId)}`)).json()).application;
  const byKey = (view, pick) => Object.fromEntries(view.environments.map((item) => [item.key, pick(item)]));
  let view = await apiView();
  expect(byKey(view, (item) => [item.configured, item.runtimeStatus])).toEqual({ staging: [false, 'UNCONFIGURED'], production: [false, 'UNCONFIGURED'] });
  await expect(target()).toContainText('Staging has no execution connection yet');
  await h.moveTo(target());
  mark('application-created-unconfigured');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: /Production/ }));
  await expect(target()).toContainText('Production has no execution connection yet');
  await h.moveTo(target());
  mark('production-unconfigured');
  await h.pause(SHORT_READ);
  await h.click(page.getByRole('button', { name: /Staging/ }));

  // 4. Preview refuses an UNCONFIGURED Environment before anything is planned.
  await h.click(page.getByRole('button', { name: 'Preview changes' }));
  const refusal = page.getByRole('alert');
  await expect(refusal).toContainText('set one in Environment Settings');
  await expect(page.getByLabel('Deployment preview')).toHaveCount(0);
  await h.moveTo(refusal);
  mark('preview-refused');
  await h.pause(READ);

  // 5. Staging Settings: nothing is preselected; the saved selection stays editable (ADR-012).
  await h.click(refusal.getByRole('button', { name: 'Open Environment settings' }));
  await expect(page.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
  const panel = page.getByLabel('Execution connection');
  await expect(panel).toContainText('Not configured');
  const stagingSelect = page.getByLabel('Connection for Staging');
  await expect(stagingSelect).toHaveValue('');
  const options = await stagingSelect.locator('option').allTextContents();
  const stagingLabel = options.find((text) => text.includes(`(${stagingKey})`));
  expect(stagingLabel, 'staging Connection offered').toBeTruthy();
  expect(options.some((text) => text.includes(`(${defaultKey})`))).toBe(true);
  await h.moveTo(stagingSelect);
  mark('staging-choices');
  await h.pause(READ);
  await h.choose(stagingSelect, stagingLabel);
  await expect(stagingSelect).toHaveValue(stagingKey);
  view = await apiView();
  expect(byKey(view, (item) => item.configured)).toEqual({ staging: false, production: false }); // choosing persists nothing
  await h.moveTo(stagingSelect);
  mark('staging-selected');
  await h.pause(LONG_READ);
  const stagingPut = page.waitForRequest((request) => request.method() === 'PUT' && new URL(request.url()).pathname.endsWith('/environments/staging/connection'));
  await h.click(page.getByRole('button', { name: 'Save connection' }));
  expect((await stagingPut).postDataJSON()).toEqual({ connectionKey: stagingKey, expectedVersion: 1 });
  await expect(panel).toContainText('Generation 0');
  await expect(panel).toContainText(stagingName);
  // Nothing is locked: the control stays editable, and an unchanged selection has nothing to save.
  await expect(page.getByLabel('Connection for Staging')).toBeEnabled();
  await expect(page.getByLabel('Connection for Staging')).toHaveValue(stagingKey);
  await expect(page.getByRole('button', { name: 'Save connection' })).toBeDisabled();
  await expect(page.getByText(/Locked/)).toHaveCount(0);
  view = await apiView();
  const stagingSaved = byKey(view, (item) => ({ key: item.connectionKey, version: item.version })).staging;
  expect(stagingSaved.key).toBe(stagingKey);
  await h.moveTo(panel);
  mark('staging-saved-editable');
  await h.pause(LONG_READ);
  // Observer probes that change nothing: a stale version is refused, and the same
  // selection at the current version is an idempotent no-op.
  const putStaging = (connectionKey, expectedVersion) => page.request.put(`${baseURL}/api/v1/applications/${encodeURIComponent(appId)}/environments/staging/connection`, { data: { connectionKey, expectedVersion } });
  const stale = await putStaging(defaultKey, stagingSaved.version - 1);
  expect(stale.status()).toBe(409);
  expect((await stale.json()).code).toBe('STALE_VERSION');
  const noop = await putStaging(stagingKey, stagingSaved.version);
  expect(noop.status()).toBe(200);
  expect(byKey(await apiView(), (item) => ({ key: item.connectionKey, version: item.version })).staging).toEqual(stagingSaved);
  // A refresh keeps the saved selection, still editable.
  x11.keys(['F5']);
  await expect(page.getByLabel('Execution connection')).toContainText(stagingName);
  await expect(page.getByLabel('Connection for Staging')).toHaveValue(stagingKey);
  await expect(page.getByLabel('Connection for Staging')).toBeEnabled();
  mark('staging-saved-after-refresh');
  await h.pause(SHORT_READ);

  // 6. Production is still unset. It first saves the default Connection, then
  //    rebinds to its own: an Environment without runtime resources may change.
  await h.click(page.getByRole('tab', { name: 'Production' }));
  await expect(page.getByLabel('Execution connection')).toContainText('Not configured');
  const productionSelect = page.getByLabel('Connection for Production');
  await expect(productionSelect).toHaveValue('');
  const productionOptions = await productionSelect.locator('option').allTextContents();
  const defaultLabel = productionOptions.find((text) => text.includes(`(${defaultKey})`));
  const productionLabel = productionOptions.find((text) => text.includes(`(${productionKey})`));
  expect(defaultLabel, 'default Connection offered').toBeTruthy();
  expect(productionLabel, 'production Connection offered').toBeTruthy();
  mark('production-still-unset');
  await h.pause(SHORT_READ);
  await h.choose(productionSelect, defaultLabel);
  await expect(productionSelect).toHaveValue(defaultKey);
  const firstProductionPut = page.waitForRequest((request) => request.method() === 'PUT' && new URL(request.url()).pathname.endsWith('/environments/production/connection'));
  await h.click(page.getByRole('button', { name: 'Save connection' }));
  expect((await firstProductionPut).postDataJSON()).toEqual({ connectionKey: defaultKey, expectedVersion: 1 });
  await expect(page.getByLabel('Execution connection')).toContainText('Generation 0');
  const firstProduction = byKey(await apiView(), (item) => ({ key: item.connectionKey, version: item.version })).production;
  expect(firstProduction.key).toBe(defaultKey);
  mark('production-saved-default');
  await h.pause(READ);
  await h.choose(productionSelect, productionLabel);
  await expect(productionSelect).toHaveValue(productionKey);
  await h.moveTo(productionSelect);
  mark('production-rebind-selected');
  await h.pause(READ);
  const productionPut = page.waitForRequest((request) => request.method() === 'PUT' && new URL(request.url()).pathname.endsWith('/environments/production/connection'));
  await h.click(page.getByRole('button', { name: 'Save connection' }));
  expect((await productionPut).postDataJSON()).toEqual({ connectionKey: productionKey, expectedVersion: firstProduction.version });
  await expect(page.getByLabel('Execution connection')).toContainText(productionName);
  await expect(page.getByLabel('Connection for Production')).toBeEnabled();
  await h.moveTo(page.getByLabel('Execution connection'));
  mark('production-rebound');
  await h.pause(LONG_READ);
  view = await apiView();
  expect(byKey(view, (item) => [item.connectionKey, item.executionProfile, item.infrastructureScope, item.configured]))
    .toEqual({ staging: [stagingKey, 'internal-k8s', 'ENVIRONMENT', true], production: [productionKey, 'internal-k8s', 'ENVIRONMENT', true] });
  expect(byKey(view, (item) => item.version).production).toBeGreaterThan(firstProduction.version);
  // secretStoreKey is the public Secret Store selection (a key, never a credential), so only that field name is exempt.
  expect(JSON.stringify(view).replaceAll('"secretStoreKey"', '')).not.toMatch(/secret|kubeconfig|token/i);
  for (const value of secretValues) expect(JSON.stringify(view)).not.toContain(value);
  await h.click(page.getByRole('tab', { name: 'Staging' }));
  await expect(page.getByLabel('Execution connection')).toContainText(stagingName);
  await h.moveTo(page.getByLabel('Execution connection'));
  mark('staging-tab-keeps-its-own');
  await h.pause(SHORT_READ);

  // 7. A backend restart keeps both bindings, still editable. The runner restarts
  //    the API on the same state file and port when asked.
  writeFileSync(`${evidenceDir}/restart-request`, 'restart');
  await expect.poll(() => existsSync(`${evidenceDir}/restart-done`), { timeout: 120_000, intervals: [500] }).toBe(true);
  x11.keys(['F5']);
  await expect(page.getByLabel('Execution connection')).toContainText(stagingName, { timeout: 30_000 });
  await expect(page.getByLabel('Connection for Staging')).toBeEnabled();
  mark('saved-after-backend-restart');
  await h.pause(READ);
  await h.click(page.getByRole('tab', { name: 'Production' }));
  await expect(page.getByLabel('Execution connection')).toContainText(productionName);
  await expect(page.getByLabel('Connection for Production')).toHaveValue(productionKey);
  await h.pause(SHORT_READ);

  // 8. Back home: each Environment shows its own target.
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await expectTarget(page, 'staging', stagingName, stagingKey);
  await h.moveTo(target());
  mark('home-staging-target');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: /Production/ }));
  await expectTarget(page, 'production', productionName, productionKey);
  await h.moveTo(target());
  mark('home-production-target');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: /Staging/ }));
  await expectTarget(page, 'staging', stagingName, stagingKey);

  // 9. Variables, secrets and workloads through the current forms (staging).
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
  await expectTarget(page, 'staging', stagingName, stagingKey);

  // 10. Preview and Deploy show the persisted staging Connection, then deploy for real.
  await h.click(page.getByRole('button', { name: 'Preview changes' }));
  const preview = page.getByLabel('Deployment preview');
  await expect(preview).toContainText('2 workload(s) affected', { timeout: 120_000 });
  await expectTarget(preview, 'staging', stagingName, stagingKey);
  await h.moveTo(preview.getByLabel('Execution target'));
  mark('preview');
  await h.pause(4000);
  await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
  const result = page.getByLabel('Deployment result');
  await expect(result).toContainText('Deploy succeeded', { timeout: 600_000 });
  for (const name of ['backend', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [a-z]+: succeeded`) })).toHaveCount(1);
  await expectTarget(result, 'staging', stagingName, stagingKey);
  await h.moveTo(result.getByLabel('Execution target'));
  mark('deploy-succeeded');
  await h.pause(5000);
  await noSecretOnScreen();

  // 10b. Staging now has runtime resources: a different destination is no longer a
  //      plain Save but a reviewed transition (nothing is clicked or changed).
  x11.keys(['F5']);
  await expect(page.getByLabel('Deployment result')).toHaveCount(0); // the refresh reloads the current state
  await h.click(page.getByRole('button', { name: 'Environment settings' }));
  await expect(page.getByLabel('Execution connection')).toContainText(stagingName);
  const liveSelect = page.getByLabel('Connection for Staging');
  await expect(liveSelect).toBeEnabled();
  const liveOptions = await liveSelect.locator('option').allTextContents();
  await h.choose(liveSelect, liveOptions.find((text) => text.includes(`(${defaultKey})`)));
  await expect(page.getByRole('button', { name: 'Review transition…' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Save connection' })).toHaveCount(0);
  await h.moveTo(page.getByRole('button', { name: 'Review transition…' }));
  mark('runtime-requires-transition');
  await h.pause(READ);
  await h.choose(liveSelect, liveOptions.find((text) => text.includes(`(${stagingKey})`)));
  const liveBefore = byKey(await apiView(), (item) => ({ key: item.connectionKey, version: item.version })).staging;
  const direct = await putStaging(defaultKey, liveBefore.version);
  expect(direct.status()).toBe(409);
  expect((await direct.json()).code).toBe('RUNTIME_EXISTS');
  expect(byKey(await apiView(), (item) => ({ key: item.connectionKey, version: item.version })).staging).toEqual(liveBefore);
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await h.pause(SHORT_READ);

  // 11. Open the deployed app through a local port-forward started off screen
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

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId: appId, namespace, stagingKey, productionKey, defaultKey }, null, 2));
  console.log(`PASS: application=${appId} namespace=${namespace} staging=${stagingKey} production=${productionKey} checks=backend,environment,secret,database,job-submit marks=${marks.length}`);
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
