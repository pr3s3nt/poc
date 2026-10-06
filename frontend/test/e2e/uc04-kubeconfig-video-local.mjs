// Human-paced UC-04 kubeconfig upload recording. Headed Chromium on Xvfb,
// ffmpeg records the whole window including the real address bar. Invoked by
// backend/test/integration/uc04-kubeconfig-video-local.sh [--kind].
//
// No route mocks and no API-created Connection: every shown step is a UI
// action. API replies are only read to assert results. Kubeconfig files are
// chosen through the browser file chooser and their content is never typed,
// shown, printed or logged. Credential values read from the private files
// stay in this process and are only used to assert their absence from the
// page.
//
// Simulated mode (default): synthetic kubeconfig, explicit memory credential
// store, Check and save is rejected because the synthetic endpoint is not a
// cluster. With ORCH_E2E_KIND_CONTEXT (--kind) the uploaded file holds the
// flattened existing kind context plus one synthetic context; the backend
// verifies kind read-only and stores the credential in a run-owned Vault.
import { expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { basename } from 'node:path';
import { createHuman, installCursor } from './human.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY', 'ORCH_E2E_KUBECONFIG_FILE', 'ORCH_E2E_SINGLE_CONTEXT_FILE', 'ORCH_E2E_UNSUPPORTED_FILE', 'ORCH_E2E_SELECT_CONTEXT']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const kindContext = env.ORCH_E2E_KIND_CONTEXT ?? '';
const live = kindContext !== '';
const kubeconfigFile = env.ORCH_E2E_KUBECONFIG_FILE;
const singleContextFile = env.ORCH_E2E_SINGLE_CONTEXT_FILE;
const unsupportedFile = env.ORCH_E2E_UNSUPPORTED_FILE;
const selectContext = env.ORCH_E2E_SELECT_CONTEXT;
if (live && selectContext !== kindContext) throw new Error('live mode must select the explicit kind context');
const videoPath = `${evidenceDir}/${live ? 'uc04-kubeconfig-kind-review.mp4' : 'uc04-kubeconfig-review.mp4'}`;
const suffix = runId.replace(/[^0-9]/g, '').slice(-6);
const connectionName = live ? `Kind internal ${suffix}` : `Simulated lab ${suffix}`;
const expectedKey = connectionName.toLowerCase().replace(/[^a-z0-9]+/g, '-');

// Credential values exist only in this process to prove they never render.
const secretValues = [];
for (const file of [kubeconfigFile, unsupportedFile]) {
  for (const match of readFileSync(file, 'utf8').matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)) {
    if (match[1].length >= 12) secretValues.push(match[1].replace(/^["']|["']$/g, ''));
  }
}
if (!secretValues.length) throw new Error('fixture files carry no embedded credential');

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
try {
  const page = await context.newPage();
  const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
  const h = createHuman(page, { selectNative: nativeSelect(x11) });
  const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
  const formError = () => page.locator('form .form-error[role="alert"]');
  // "Upload kubeconfig file" also matches a label search for "Kubeconfig file".
  const fileInput = () => page.locator('input[type="file"]');
  // The file input label names the chosen file, e.g. "single-context".
  const contextSelector = () => page.getByRole('combobox');
  const assertNoCredentialOnPage = async () => {
    const html = await page.content();
    for (const value of secretValues) {
      if (html.includes(value)) throw new Error('credential material is rendered in the page');
    }
  };
  // Every API reply asserted below is also checked for credential material.
  const assertNoCredentialInResponse = async (response) => {
    const text = await response.text();
    for (const value of secretValues) {
      if (text.includes(value)) throw new Error(`credential material in ${new URL(response.url()).pathname} response`);
    }
    return response;
  };
  const inspectResponse = () => page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/connections/kubernetes/inspect');
  async function upload(file) {
    const input = fileInput();
    const chooser = page.waitForEvent('filechooser');
    chooser.catch(() => {}); // a failed click must report its own error
    await h.click(input);
    await (await chooser).setFiles(file);
    await expect(page.getByText(`Selected file: ${basename(file)}.`, { exact: false })).toBeVisible();
    await h.pause(800);
  }

  await page.goto(`${baseURL}/ui/sign-in`);
  await expect(page.getByLabel('Username')).toBeVisible();
  await page.waitForTimeout(1500);
  x11.focusBrowser();
  recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
  startedAt = Date.now();
  await page.waitForTimeout(1500);

  // 1. Platform Engineer signs in.
  mark('sign-in');
  await h.showCursor();
  await h.type(page.getByLabel('Username'), 'platform-engineer');
  await h.type(page.getByLabel('Password'), 'test-password');
  await h.click(page.getByRole('button', { name: 'Sign in' }));
  await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  await h.pause(SHORT_READ);

  // 2. Connections page: the seeded host-context entry stays listed.
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  await expect(page).toHaveURL(`${baseURL}/ui/platform/connections`);
  const connectionRows = () => page.getByRole('table', { name: 'Registered connections' }).locator('tbody tr');
  const seeded = connectionRows().first();
  await expect(seeded).toContainText('READY');
  await expect(page.getByLabel('Connection ID')).toHaveCount(0);
  await h.moveTo(seeded);
  mark('connections-list');
  await h.pause(READ);

  // 3. Invalid pasted content: safe parser guidance, form retained.
  await h.type(page.getByLabel('Connection name'), connectionName);
  await h.click(page.getByLabel('Paste kubeconfig'));
  const content = page.getByLabel('Kubeconfig content');
  await expect(content).toHaveClass(/masked-text/);
  await h.paste(content, 'this is not a kubeconfig');
  let inspected = inspectResponse();
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  expect((await assertNoCredentialInResponse(await inspected)).status()).toBe(400);
  await expect(formError()).toContainText('not a valid kubeconfig');
  await expect(page.getByLabel('Connection name')).toHaveValue(connectionName);
  await h.moveTo(formError());
  mark('invalid-kubeconfig');
  await h.pause(READ);

  // 4. Unsupported authentication (exec plugin) from an uploaded file.
  await h.click(page.getByLabel('Upload kubeconfig file'));
  await upload(unsupportedFile);
  inspected = inspectResponse();
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  expect((await assertNoCredentialInResponse(await inspected)).status()).toBe(400);
  await expect(formError()).toContainText('unsupported authentication method');
  await expect(formError()).toContainText('never run');
  await h.moveTo(formError());
  mark('unsupported-authentication');
  await h.pause(LONG_READ);
  await assertNoCredentialOnPage();

  // 5. One-context file: the only context is selected automatically.
  await upload(singleContextFile);
  inspected = inspectResponse();
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  const single = await assertNoCredentialInResponse(await inspected);
  expect(single.status()).toBe(200);
  const singleContexts = (await single.json()).contexts;
  expect(singleContexts.map((item) => item.name)).toEqual([selectContext]);
  await expect(contextSelector()).toHaveCount(0);
  const autoSummary = page.getByLabel('Selected destination');
  await expect(autoSummary).toContainText(`Context ${selectContext}`);
  await expect(autoSummary).toContainText(`Endpoint ${singleContexts[0].endpoint}`);
  await h.moveTo(autoSummary);
  mark('single-context-auto-selected');
  await h.pause(LONG_READ);
  await assertNoCredentialOnPage();

  // 6. Upload the multi-context file; the backend lists safe metadata only.
  await upload(kubeconfigFile);
  await expect(page.getByLabel('Selected destination')).toHaveCount(0);
  inspected = inspectResponse();
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  const inspection = await assertNoCredentialInResponse(await inspected);
  expect(inspection.status()).toBe(200);
  const { contexts } = await inspection.json();
  expect(contexts.length).toBeGreaterThan(1);
  const target = contexts.find((item) => item.name === selectContext);
  expect(target).toBeTruthy();
  expect(target.endpoint).toMatch(/^https:\/\//);
  for (const item of contexts) expect(Object.keys(item).sort()).toEqual(['cluster', 'endpoint', 'name']);
  const contextSelect = contextSelector();
  await expect(contextSelect).toBeVisible();
  await expect(page.getByLabel('Selected destination')).toHaveCount(0);
  await h.moveTo(contextSelect);
  mark('contexts-listed');
  await h.pause(SHORT_READ);

  // 7. Choose the context; review cluster and endpoint before saving.
  await h.choose(contextSelect, selectContext);
  const summary = page.getByLabel('Selected destination');
  await expect(summary).toContainText(`Context ${selectContext}`);
  await expect(summary).toContainText(`Cluster ${target.cluster}`);
  await expect(summary).toContainText(`Endpoint ${target.endpoint}`);
  await h.moveTo(summary);
  mark('destination-reviewed');
  await h.pause(LONG_READ);
  await assertNoCredentialOnPage();

  // 8. Check and save.
  const registered = page.waitForResponse((response) => response.request().method() === 'POST'
    && new URL(response.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 90_000 });
  await h.click(page.getByRole('button', { name: 'Check and save' }));
  const response = await assertNoCredentialInResponse(await registered);
  let connection;
  if (live) {
    expect(response.status()).toBe(201);
    const created = await response.json();
    expect(created).toMatchObject({ key: expectedKey, name: connectionName, kind: 'KUBERNETES', authenticationType: 'KUBECONFIG', status: 'READY' });
    expect(created.config).toMatchObject({ kubeContext: selectContext, cluster: target.cluster, endpoint: target.endpoint });
    expect(created.verification?.verified).toBe(true);
    for (const forbidden of ['secretRef', 'kubeconfig']) expect(JSON.stringify(created)).not.toContain(forbidden);
    connection = { key: created.key, status: created.status, authenticationType: created.authenticationType, serverVersion: created.verification?.serverVersion };
    await expect(page.getByRole('status')).toHaveText(`Registered connection ${connectionName} (${expectedKey}).`);
    await h.moveTo(page.getByRole('status'));
    mark('check-and-save-ready');
    await h.pause(READ);

    // 9. READY list entry and cleared credential input.
    const entry = connectionRows().filter({ has: page.getByText(connectionName, { exact: true }) });
    await expect(entry.getByRole('cell')).toHaveText([`${connectionName}${expectedKey}`, 'KUBERNETES', `Cluster ${target.cluster}Endpoint ${target.endpoint}`, 'READY']);
    await h.moveTo(entry);
    mark('connection-ready-listed');
    await h.pause(LONG_READ);
    const list = await page.request.get(`${baseURL}/api/v1/connections`);
    expect(list.status()).toBe(200);
    const listed = (await (await assertNoCredentialInResponse(list)).json()).connections.find((item) => item.key === expectedKey);
    expect(listed).toMatchObject({ status: 'READY', authenticationType: 'KUBECONFIG' });
    expect(JSON.stringify(listed)).not.toContain('secretRef');
    await expect(page.getByLabel('Connection name')).toHaveValue('');
    await expect(fileInput()).toHaveValue('');
    await expect(page.getByText(/Selected file:/)).toHaveCount(0);
    await expect(summary).toHaveCount(0);
    await h.moveTo(fileInput());
    mark('credential-input-cleared');
    await h.pause(READ);
  } else {
    // Simulated evidence: the synthetic endpoint is not a cluster, so the
    // real read-only verifier rejects it and nothing READY is saved.
    expect(response.status()).toBe(422);
    await expect(formError()).toContainText('could not be reached');
    await expect(page.getByLabel('Connection name')).toHaveValue(connectionName);
    await expect(summary).toContainText(`Context ${selectContext}`);
    await h.moveTo(formError());
    mark('simulated-verification-rejected');
    await h.pause(LONG_READ);
    const list = await page.request.get(`${baseURL}/api/v1/connections`);
    expect((await (await assertNoCredentialInResponse(list)).json()).connections.some((item) => item.key === expectedKey)).toBe(false);
    connection = { key: null, status: 'NOT_SAVED', simulated: true };
  }
  await assertNoCredentialOnPage();

  // 10. Sign out.
  await h.click(page.getByRole('button', { name: 'Sign out' }));
  await expect(page.getByLabel('Username')).toBeVisible();
  mark('signed-out');
  await h.pause(READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ mode: live ? 'kind-read-only' : 'simulated', selectedContext: selectContext, connection }, null, 2));
  console.log(`PASS: uc04 kubeconfig video mode=${live ? 'kind' : 'simulated'} connection=${connection.key ?? 'none'} marks=${marks.length}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}
