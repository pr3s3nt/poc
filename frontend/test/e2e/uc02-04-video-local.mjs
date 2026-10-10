// Human-paced UC-02/03/04 registration recording. Headed Chromium on Xvfb,
// ffmpeg records the whole window including the real address bar. Invoked by
// backend/test/integration/uc02-04-video-local.sh [--kind].
//
// No API fixtures: the backend has only its seeded accounts, catalog and
// connections. Every shown step is a UI action: the cursor moves to each
// target, text is typed key by key, native <select> popups are answered with
// X11 keys. API replies are only read to assert results.
//
// The UC-04 segment uses the current kubeconfig upload form and contacts no
// cluster in either mode: an invalid pasted document is rejected, a synthetic
// single-context kubeconfig is inspected (safe metadata only) and Check and
// save is refused because this runner configures no Connection credential
// store, so nothing is verified or saved. The synthetic token is a fixed
// placeholder, not a credential. ORCH_E2E_KIND_CONTEXT (--kind) changes only
// the evidence names; the successful upload of the real kind context is
// recorded by backend/test/integration/uc04-kubeconfig-video-local.sh --kind.
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
const connectionName = `Demo cluster ${runId.replace(/[^0-9]/g, '').slice(-6)}`;
const kindUploadRunner = 'backend/test/integration/uc04-kubeconfig-video-local.sh --kind';
// Synthetic single-context kubeconfig. The endpoint is never contacted.
const syntheticToken = 'synthetic-placeholder-token-not-a-credential';
const syntheticKubeconfig = [
  'apiVersion: v1',
  'kind: Config',
  'clusters:',
  '- name: demo-cluster',
  '  cluster:',
  '    server: https://demo-cluster.invalid:6443',
  'users:',
  '- name: demo-user',
  '  user:',
  `    token: ${syntheticToken}`,
  'contexts:',
  '- name: demo',
  '  context:',
  '    cluster: demo-cluster',
  '    user: demo-user',
  'current-context: demo',
].join('\n');

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
  await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-definitions`);
  const definitionId = page.getByLabel('ID cấu hình');
  const variablesField = page.getByLabel('Tham số cấu hình (JSON object)');
  const registerDefinition = page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên' });
  await h.type(definitionId, 'Postgres Fast');
  await h.choose(page.getByLabel('Loại tài nguyên'), 'postgres');
  await h.click(page.getByRole('button', { name: 'Hiện nâng cao' }));
  await h.type(variablesField, driverVariables, { replace: true, delay: TYPE_DELAY });
  await h.type(page.getByLabel('Điều kiện 1 Class'), 'fast');
  mark('definition-filled');
  await h.pause(SHORT_READ);
  await h.click(registerDefinition);
  await expect(formError()).toContainText('ID chỉ gồm chữ thường a-z, số 0-9 và dấu gạch ngang');
  await expect(definitionId).toHaveValue('Postgres Fast');
  await expect(variablesField).toHaveValue(driverVariables);
  await h.moveTo(formError());
  mark('definition-invalid-id');
  await h.pause(READ);

  await h.type(definitionId, 'postgres-fast', { replace: true });
  const unsupported = '{"image":"postgres:17-alpine","replicas":"2"}';
  await h.type(variablesField, unsupported, { replace: true });
  await h.click(registerDefinition);
  await expect(formError()).toContainText('Máy chủ từ chối cấu hình này');
  await expect(variablesField).toHaveValue(unsupported);
  await h.moveTo(formError());
  mark('definition-unsupported-input');
  await h.pause(READ);

  const wrongType = '{"storage":2}';
  await h.type(variablesField, wrongType, { replace: true });
  await h.click(registerDefinition);
  await expect(formError()).toContainText('Máy chủ từ chối cấu hình này');
  await expect(variablesField).toHaveValue(wrongType);
  await expect(page.getByLabel('Điều kiện 1 Class')).toHaveValue('fast');
  await h.moveTo(formError());
  mark('definition-wrong-type');
  await h.pause(READ);

  await h.type(variablesField, driverVariables, { replace: true, delay: TYPE_DELAY });
  await h.click(registerDefinition);
  await expect(page.getByRole('status')).toHaveText('Đã đăng ký cấu hình tài nguyên postgres-fast.');
  const definitionEntry = page.locator('.catalog-entry').filter({ has: page.getByText('postgres-fast', { exact: true }) });
  await expect(definitionEntry).toContainText('postgres · Cluster nội bộ · Tạo trong cluster (Kubernetes) · 1 điều kiện áp dụng');
  await h.moveTo(definitionEntry);
  mark('definition-registered');
  await h.pause(READ);

  // Duplicate: the registered ID is rejected again.
  await h.type(definitionId, 'postgres-fast');
  await h.click(registerDefinition);
  await expect(formError()).toContainText('ID cấu hình đã tồn tại');
  await expect(definitionId).toHaveValue('postgres-fast');
  await h.moveTo(formError());
  mark('definition-duplicate');
  await h.pause(READ);

  // 6. Connections. The seeded entry is listed READY by seeding, not by
  // verification. The legacy ID/context form is gone; registration uses the
  // kubeconfig upload form.
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/connections`);
  const seeded = page.getByRole('table', { name: 'Registered connections' }).locator('tbody tr').first();
  await expect(seeded).toContainText('READY');
  await expect(page.getByLabel('Connection ID')).toHaveCount(0);
  await h.moveTo(seeded);
  mark('connections-list');
  await h.pause(READ);

  const inspectResponse = () => page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/connections/kubernetes/inspect');
  await h.type(page.getByLabel('Connection name'), connectionName);
  await h.click(page.getByLabel('Paste kubeconfig'));
  const content = page.getByLabel('Kubeconfig content');
  await expect(content).toHaveClass(/masked-text/);
  await h.paste(content, 'this is not a kubeconfig');
  let inspected = inspectResponse();
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  expect((await inspected).status()).toBe(400);
  await expect(formError()).toContainText('not a valid kubeconfig');
  await expect(page.getByLabel('Connection name')).toHaveValue(connectionName);
  await h.moveTo(formError());
  mark('connection-invalid');
  await h.pause(READ);

  // A synthetic single-context kubeconfig: the backend lists safe metadata
  // only and the only context is selected automatically.
  // Clear the rejected text first; a paste inserts at the caret.
  await h.click(content);
  await page.keyboard.press('Control+A');
  await page.keyboard.press('Delete');
  await expect(content).toHaveValue('');
  await h.paste(content, syntheticKubeconfig);
  await expect(content).toHaveValue(syntheticKubeconfig);
  inspected = inspectResponse();
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  const inspection = await inspected;
  expect(inspection.status()).toBe(200);
  const { contexts } = await inspection.json();
  expect(contexts).toEqual([{ name: 'demo', cluster: 'demo-cluster', endpoint: 'https://demo-cluster.invalid:6443' }]);
  const summary = page.getByLabel('Selected destination');
  await expect(summary).toContainText('Context demo');
  await expect(summary).toContainText('Endpoint https://demo-cluster.invalid:6443');
  await h.moveTo(summary);
  mark('connection-context-inspected');
  await h.pause(READ);

  // No Connection credential store is configured: Check and save fails
  // closed before any cluster call and nothing is saved.
  const rejected = page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/connections/kubernetes');
  await h.click(page.getByRole('button', { name: 'Check and save' }));
  expect((await rejected).status()).toBe(503);
  await expect(formError()).toContainText('no connection was saved');
  await expect(page.getByLabel('Connection name')).toHaveValue(connectionName);
  const list = await page.request.get(`${baseURL}/api/v1/connections`);
  expect(list.status()).toBe(200);
  expect((await list.json()).connections.some((item) => item.name === connectionName)).toBe(false);
  // The retained form keeps the pasted document only inside the masked field;
  // it must not appear anywhere else in the page.
  await expect(content).toHaveClass(/masked-text/);
  const outsideField = await page.locator('html').evaluate((root) => {
    const copy = root.cloneNode(true);
    for (const field of copy.querySelectorAll('textarea.masked-text')) field.remove();
    return copy.outerHTML;
  });
  if (outsideField.includes(syntheticToken)) throw new Error('kubeconfig content is rendered in the page');
  await h.moveTo(formError());
  mark('connection-store-unavailable');
  await h.pause(LONG_READ);
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
  const matches = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Resource execution bindings' }) });
  await expect(matches).toContainText('postgres-fast');
  await h.moveTo(matches.getByText('postgres-fast'));
  mark('preview-matches-new-definition');
  await h.pause(LONG_READ);

  // 8. Sign out.
  await signOut();
  mark('signed-out');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId, connection: { status: 'NOT_SAVED', reason: 'no connection credential store', kindUploadRunner } }, null, 2));
  console.log(`PASS: uc02-04 video application=${applicationId} connection=not-saved marks=${marks.length}`);
  if (kindContext) console.log(`NOTE: the successful kind kubeconfig upload is recorded by ${kindUploadRunner}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}
