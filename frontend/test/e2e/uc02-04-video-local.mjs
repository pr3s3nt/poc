// Human-paced UC-02/03/04 registration recording. Headed Chromium on Xvfb,
// ffmpeg records the whole window including the real address bar. Invoked by
// backend/test/integration/uc02-04-video-local.sh [--kind].
//
// No API fixtures: the backend has only its seeded accounts, catalog and
// connections. Every shown step is a UI action: the cursor moves to each
// target, text is typed key by key, native <select> popups are answered with
// X11 keys. API replies are only read to assert results.
//
// Local mode (default) contacts no cluster: it shows the seeded Connection
// and an ID rejection only. With ORCH_E2E_KIND_CONTEXT set (--kind), the
// backend's real kubectl verifier checks that existing host context: a
// missing context is rejected and a run-unique Connection is registered
// READY after the read-only API/permission checks. No kubeconfig content is
// entered anywhere.
import { expect } from '@playwright/test';
import { writeFileSync } from 'node:fs';
import { createApplication, createHuman, installCursor, TYPE_DELAY } from './human.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const kindContext = env.ORCH_E2E_KIND_CONTEXT;
const videoPath = `${evidenceDir}/${kindContext ? 'uc02-04-kind-review.mp4' : 'uc02-04-review.mp4'}`;
const connectionId = `kind-${runId.replace(/[^0-9]/g, '').slice(-8)}`;

const READ = 3000;
const LONG_READ = 4500;
const SHORT_READ = 2000;

const driverVariables = '{"image":"postgres:17-alpine","storage":"2Gi","namespace":"${resources[\'k8s-namespace.default#environments.@app.@env\'].outputs.name}"}';
const score = [
  'apiVersion: score.dev/v1b1',
  'metadata:',
  '  name: api',
  'containers:',
  '  main:',
  '    image: example.invalid/api:v1',
  'resources:',
  '  db:',
  '    type: postgres',
  '    class: fast',
  '    params:',
  '      database: shop',
  '      username: shop',
].join('\n');

const browser = await launchHeaded({ width, height });
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(30_000);
await context.addInitScript(installCursor);
const marks = [];
let recorder;
let startedAt;
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
  async function signIn(user) {
    await h.showCursor();
    await h.type(page.getByLabel('Username'), user);
    await h.type(page.getByLabel('Password'), 'test-password');
    await h.click(page.getByRole('button', { name: 'Sign in' }));
    await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  }
  async function signOut() {
    await h.click(page.getByRole('button', { name: 'Sign out' }));
    await expect(page.getByLabel('Username')).toBeVisible();
  }
  const formError = () => page.locator('form .form-error[role="alert"]');

  // 1. Platform Engineer signs in.
  mark('sign-in');
  await signIn('platform-engineer');
  mark('platform-engineer-home');
  await h.pause(SHORT_READ);

  // 2. Resource Type: valid registration.
  await h.click(page.getByRole('link', { name: /Resource types/ }));
  await expect(page.getByRole('heading', { name: 'Resource types', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-types`);
  await h.type(page.getByLabel('Resource type ID'), 'cache');
  await h.click(page.getByRole('button', { name: '+ Add input' }));
  await h.type(page.getByLabel('Inputs 1 name'), 'size');
  await h.choose(page.getByLabel('Inputs 1 type'), 'number');
  await h.click(page.locator('.catalog-field-row').first().getByLabel('Required'));
  await h.click(page.getByRole('button', { name: '+ Add output' }));
  await h.type(page.getByLabel('Outputs 1 name'), 'host');
  await h.pause(1200);
  await h.click(page.getByRole('button', { name: 'Register resource type' }));
  await expect(page.getByRole('status')).toHaveText('Registered resource type cache.');
  const typeList = page.locator('.catalog-entry').filter({ has: page.getByText('cache', { exact: true }) });
  await expect(typeList).toContainText('1 inputs · 1 outputs');
  await h.moveTo(typeList);
  mark('type-registered');
  await h.pause(READ);

  // 3. Duplicate: the same ID is rejected and the form is kept.
  await h.type(page.getByLabel('Resource type ID'), 'cache');
  await h.click(page.getByRole('button', { name: '+ Add output' }));
  await h.type(page.getByLabel('Outputs 1 name'), 'url');
  await h.click(page.getByRole('button', { name: 'Register resource type' }));
  await expect(formError()).toContainText('duplicate id');
  await expect(page.getByLabel('Resource type ID')).toHaveValue('cache');
  await h.moveTo(formError());
  mark('type-duplicate');
  await h.pause(READ);

  // 4. Invalid: two inputs with the same name.
  await h.type(page.getByLabel('Resource type ID'), 'broken', { replace: true });
  await h.click(page.getByRole('button', { name: '+ Add input' }));
  await h.type(page.getByLabel('Inputs 1 name'), 'size');
  await h.click(page.getByRole('button', { name: '+ Add input' }));
  await h.type(page.getByLabel('Inputs 2 name'), 'size');
  await h.click(page.getByRole('button', { name: 'Register resource type' }));
  await expect(formError()).toContainText('duplicate input "size"');
  await expect(page.getByLabel('Inputs 2 name')).toHaveValue('size');
  await h.moveTo(formError());
  mark('type-invalid');
  await h.pause(READ);

  // 5. PostgreSQL Definition for class "fast": an invalid ID and strict
  // Driver Inputs are rejected with the form kept, then it registers.
  await h.click(page.getByRole('link', { name: /Resource definitions/ }));
  await expect(page.getByRole('heading', { name: 'Resource definitions', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-definitions`);
  const definitionId = page.getByLabel('Definition ID');
  const variablesField = page.getByLabel('Driver variables (JSON object)');
  const registerDefinition = page.getByRole('button', { name: 'Register resource definition' });
  await h.type(definitionId, 'Postgres Fast');
  await h.choose(page.getByLabel('Resource Type'), 'postgres');
  await h.type(variablesField, driverVariables, { replace: true, delay: TYPE_DELAY });
  await h.type(page.getByLabel('Criterion 1 Class'), 'fast');
  mark('definition-filled');
  await h.pause(SHORT_READ);
  await h.click(registerDefinition);
  await expect(formError()).toContainText('ID must be non-empty and contain only lowercase letters, digits and hyphens');
  await expect(definitionId).toHaveValue('Postgres Fast');
  await expect(variablesField).toHaveValue(driverVariables);
  await h.moveTo(formError());
  mark('definition-invalid-id');
  await h.pause(READ);

  await h.type(definitionId, 'postgres-fast', { replace: true });
  const unsupported = '{"image":"postgres:17-alpine","replicas":"2"}';
  await h.type(variablesField, unsupported, { replace: true });
  await h.click(registerDefinition);
  await expect(formError()).toContainText('driverInputs.values.variables.replicas is not supported by the kubernetes driver for postgres');
  await expect(variablesField).toHaveValue(unsupported);
  await h.moveTo(formError());
  mark('definition-unsupported-input');
  await h.pause(READ);

  const wrongType = '{"storage":2}';
  await h.type(variablesField, wrongType, { replace: true });
  await h.click(registerDefinition);
  await expect(formError()).toContainText('driverInputs.values.variables.storage must be string');
  await expect(variablesField).toHaveValue(wrongType);
  await expect(page.getByLabel('Criterion 1 Class')).toHaveValue('fast');
  await h.moveTo(formError());
  mark('definition-wrong-type');
  await h.pause(READ);

  await h.type(variablesField, driverVariables, { replace: true, delay: TYPE_DELAY });
  await h.click(registerDefinition);
  await expect(page.getByRole('status')).toHaveText('Registered resource definition postgres-fast.');
  const definitionEntry = page.locator('.catalog-entry').filter({ has: page.getByText('postgres-fast', { exact: true }) });
  await expect(definitionEntry).toContainText('postgres · internal-k8s · kubernetes · 1 criteria');
  await h.moveTo(definitionEntry);
  mark('definition-registered');
  await h.pause(READ);

  // Duplicate: the registered ID is rejected again.
  await h.type(definitionId, 'postgres-fast');
  await h.click(registerDefinition);
  await expect(formError()).toContainText('duplicate id');
  await expect(definitionId).toHaveValue('postgres-fast');
  await h.moveTo(formError());
  mark('definition-duplicate');
  await h.pause(READ);

  // 6. Connections. The seeded entry is listed READY by seeding, not by
  // verification. An invalid ID is rejected before any verification.
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  const seeded = page.locator('.catalog-entry').first();
  await expect(seeded).toContainText('context');
  await expect(seeded).toContainText('READY');
  await h.moveTo(seeded);
  mark('connections-list');
  await h.pause(READ);
  const connectionField = page.getByLabel('Connection ID');
  const contextField = page.getByLabel('Kube context');
  const registerCluster = page.getByRole('button', { name: 'Register cluster' });
  await h.type(connectionField, 'Bad ID');
  await h.type(page.getByLabel('Cluster ID'), kindContext ? 'idp-internal' : 'demo');
  await h.type(contextField, `no-such-context-${connectionId}`);
  await h.click(registerCluster);
  await expect(formError()).toContainText('valid connection ID');
  await expect(connectionField).toHaveValue('Bad ID');
  await h.moveTo(formError());
  mark('connection-invalid');
  await h.pause(READ);

  let verification;
  if (kindContext) {
    // Real verifier: a context that does not exist on the backend host.
    await h.type(connectionField, connectionId, { replace: true });
    await h.click(registerCluster);
    await expect(formError()).toContainText('kube context is not configured on the backend host', { timeout: 60_000 });
    await expect(contextField).toHaveValue(`no-such-context-${connectionId}`);
    await h.moveTo(formError());
    mark('connection-context-missing');
    await h.pause(READ);

    // Existing kind context: read-only API/permission checks, then READY.
    await h.type(contextField, kindContext, { replace: true });
    const registered = page.waitForResponse((response) => response.request().method() === 'POST'
      && new URL(response.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 60_000 });
    await h.click(registerCluster);
    const response = await registered;
    expect(response.status()).toBe(201);
    const created = await response.json();
    expect(created.key).toBe(connectionId);
    expect(created.status).toBe('READY');
    expect(created.verification?.verified).toBe(true);
    expect(created.verification?.endpoint).toMatch(/^https:\/\//);
    expect(created.config).toMatchObject({ cluster: 'idp-internal', kubeContext: kindContext });
    verification = { connectionId, status: created.status, serverVersion: created.verification.serverVersion };
    await expect(page.getByRole('status')).toHaveText(`Registered connection ${connectionId}.`);
    const entry = page.locator('.catalog-entry').filter({ has: page.getByText(connectionId, { exact: true }) });
    await expect(entry).toContainText(`KUBERNETES · cluster idp-internal · context ${kindContext} · READY`);
    await h.moveTo(entry);
    mark('connection-verified-ready');
    await h.pause(LONG_READ);
  }
  await signOut();
  mark('platform-engineer-signed-out');

  // 7. Developer: new Application, Score Preview matches the new Definition.
  await signIn('developer');
  const applicationId = await createApplication(h, 'Catalog Consumer', `catalog-${runId.slice(-6)}`);
  mark('developer-application');
  await h.click(page.getByRole('button', { name: 'Preview Score' }));
  await expect(page.getByRole('heading', { name: 'Preview Score', level: 1 })).toBeVisible();
  await h.type(page.getByLabel('Workload ID'), 'api');
  await h.type(page.getByLabel('Run ID'), 'catalog-demo');
  await h.type(page.getByLabel('Score after (YAML or JSON)'), score, { delay: TYPE_DELAY });
  await h.click(page.getByRole('button', { name: 'Preview', exact: true }));
  const matches = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Matched Resource Definitions' }) });
  await expect(matches).toContainText('postgres-fast');
  await h.moveTo(matches.getByText('postgres-fast'));
  mark('preview-matches-new-definition');
  await h.pause(LONG_READ);

  // 8. Sign out.
  await signOut();
  mark('signed-out');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId, ...(verification ? { connection: verification } : {}) }, null, 2));
  console.log(`PASS: uc02-04 video${kindContext ? ` connection=${connectionId} READY` : ''} application=${applicationId} marks=${marks.length}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}
