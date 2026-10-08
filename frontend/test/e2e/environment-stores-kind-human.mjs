// Human-paced recording of ADR-012 on the existing kind cluster: two real Vault
// KV v2 stores registered by a Platform Engineer, a Developer choosing a store and
// a deployment connection per Environment, a secret copied to a second store and
// rolled out through the Vault Secrets Operator, a stale transition Preview
// rejected from a second tab, and a MIGRATE_POSTGRES transition that restores a
// known job into a second generation, moves the public route, keeps the source
// quiesced and finally cleans it up on explicit request. Headed Chromium on a
// private Xvfb display; ffmpeg records the whole window. Invoked by
// backend/test/integration/environment-stores-kind-video.sh.
//
// Every shown product mutation is a UI action. kubectl here only OBSERVES
// (replicas, rows, ingress owners); no secret value, token or dump content is
// printed, typed into a log or written to evidence.
/* global document -- page.evaluate callbacks run in the browser. */
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { execFileSync } from 'node:child_process';
import { appendFileSync, readFileSync, writeFileSync, existsSync } from 'node:fs';
import { acceptanceApp, addWorkload, createHuman, installCursor, putKeys, TYPE_DELAY } from './human.mjs';
import { assertNoSecretLeaks } from './secret-scan.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_KUBE_CONTEXT', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_VIDEO_NAME', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_TRAEFIK_PORT',
  'ORCH_E2E_STORE_A_TOKEN_FILE', 'ORCH_E2E_STORE_B_TOKEN_FILE', 'ORCH_E2E_STORE_A_BACKEND', 'ORCH_E2E_STORE_B_BACKEND', 'ORCH_E2E_STORE_A_WORKLOAD', 'ORCH_E2E_STORE_B_WORKLOAD',
  'ORCH_E2E_SOURCE_KEY', 'ORCH_E2E_DEST_KEY', 'ORCH_E2E_SOURCE_NAME', 'ORCH_E2E_DEST_NAME', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const kubeContext = env.ORCH_E2E_KUBE_CONTEXT;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
const traefikPort = env.ORCH_E2E_TRAEFIK_PORT;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const videoPath = `${evidenceDir}/${env.ORCH_E2E_VIDEO_NAME}`;
const suffix = runId.replace(/[^0-9]/g, '').slice(-6);
const applicationName = `Environment stores ${suffix}`;
const subdomain = `es${suffix}`;
const stagingHost = `staging.${subdomain}.example.com`;
const tokenA = readFileSync(env.ORCH_E2E_STORE_A_TOKEN_FILE, 'utf8').trim();
const tokenB = readFileSync(env.ORCH_E2E_STORE_B_TOKEN_FILE, 'utf8').trim();
const stores = [
  { name: 'Alpha vault', token: tokenA, backend: env.ORCH_E2E_STORE_A_BACKEND, workload: env.ORCH_E2E_STORE_A_WORKLOAD, auth: 'kubernetes', mount: 'secret' },
  { name: 'Beta vault', token: tokenB, backend: env.ORCH_E2E_STORE_B_BACKEND, workload: env.ORCH_E2E_STORE_B_WORKLOAD, auth: 'k8s-b', mount: 'secret' },
];
const slug = (name) => name.toLowerCase().replace(/[^a-z0-9]+/g, '-');
const sourceKey = env.ORCH_E2E_SOURCE_KEY;
const destKey = env.ORCH_E2E_DEST_KEY;

const READ = 3000;
const SHORT_READ = 2000;
const LONG_READ = 4500;
const checks = (line) => { console.log(line); appendFileSync(`${evidenceDir}/checks.txt`, `${line}\n`); };

const kube = (...args) => execFileSync('kubectl', ['--context', kubeContext, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
const kubeTry = (...args) => { try { return kube(...args); } catch { return ''; } };

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

  const secretValues = [tokenA, tokenB];
  // Structural scan (see secret-scan.mjs). `activeField` marks the one password
  // input the person is deliberately filling in; its value is the only place a
  // token may exist, and only while it is typed and masked. Without it nothing
  // is exempt.
  const noSecretsOnScreen = async (extra = [], { activeField = false } = {}) => {
    await assertNoSecretLeaks(page, [...secretValues, ...extra], { allow: activeField ? ['input[type="password"]'] : [] });
  };
  async function signIn(user) {
    await h.showCursor();
    await h.pause(1200);
    await h.type(page.getByLabel('Username'), user);
    await h.type(page.getByLabel('Password'), 'test-password');
    await h.click(page.getByRole('button', { name: 'Sign in' }));
    await expect(page.getByRole('heading', { name: /applications/i })).toBeVisible();
    await h.pause(SHORT_READ);
  }
  async function signOut() {
    await h.click(page.getByRole('button', { name: 'Sign out' }));
    await expect(page.getByLabel('Username')).toBeVisible();
    await h.pause(SHORT_READ);
  }

  // ---- 1. Platform Engineer registers two real Vault KV v2 stores -------------
  mark('sign-in');
  await signIn('platform-engineer');
  await h.click(page.getByRole('link', { name: /Secret stores/ }));
  await expect(page.getByRole('heading', { name: 'Secret stores', level: 1 })).toBeVisible();
  await expect(page.getByText('No secret store is registered yet.')).toBeVisible();
  await h.pause(READ);
  for (const store of stores) {
    await h.type(page.getByLabel(/^Name/), store.name);
    await h.type(page.getByLabel(/^Backend address/), store.backend);
    await h.type(page.getByLabel(/^Workload address/), store.workload);
    await h.type(page.getByLabel(/^KV v2 mount/), store.mount, { replace: true });
    await h.type(page.getByLabel(/^Kubernetes auth mount/), store.auth, { replace: true });
    await h.paste(page.getByLabel(/^Token/), store.token);
    await expect(page.getByLabel(/^Token/)).toHaveAttribute('type', 'password');
    // The pasted token lives only in the masked field (React mirrors it into that input's value attribute).
    await noSecretsOnScreen([], { activeField: true });
    mark(`${slug(store.name)}-form`);
    const registered = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/secret-stores', { timeout: 90_000 });
    await h.click(page.getByRole('button', { name: 'Verify and register' }));
    const response = await registered;
    const text = await response.text();
    for (const value of secretValues) if (text.includes(value)) throw new Error('token echoed by the registration response');
    expect(response.status(), 'registration status').toBe(201);
    await expect(page.getByRole('status')).toContainText(`"${store.name}" is READY`);
    const row = page.locator('.settings-table-row').filter({ hasText: store.name });
    await expect(row).toContainText('READY');
    await h.moveTo(row);
    mark(`${slug(store.name)}-ready`);
    await h.pause(READ);
    await expect(page.getByLabel(/^Token/)).toHaveValue('');
    await noSecretsOnScreen();
  }
  checks('ui: two Vault KV v2 stores registered READY by the Platform Engineer; token never rendered or echoed');
  await signOut();

  // ---- 2. Developer: application, per-Environment connection and store ----------
  await signIn('developer');
  await h.click(page.getByRole('button', { name: /create application|new application/i }).first());
  await h.type(page.getByLabel('Application name'), applicationName);
  await h.type(page.getByLabel('Subdomain'), subdomain);
  await h.click(page.getByRole('button', { name: 'Create application' }));
  await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
  await h.pause(SHORT_READ);
  const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
  // Recorded before any runtime effect so the wrapper's cleanup knows the application even if this run fails.
  writeFileSync(`${evidenceDir}/application-id`, `${appId}\n`, { mode: 0o600 });
  const sourceNamespace = `app-${appId}-staging`;

  await h.click(page.getByRole('button', { name: 'Environment settings' }));
  await expect(page.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
  const optionLabel = async (select, needle) => {
    const label = (await select.locator('option').allTextContents()).find((text) => text.includes(needle));
    if (!label) throw new Error(`option ${needle} is not offered`);
    return label;
  };
  async function selectConnection(title, key) {
    const select = page.getByLabel(`Connection for ${title}`);
    await h.choose(select, await optionLabel(select, `(${key})`));
    await expect(select).toHaveValue(key);
    const saved = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname.endsWith('/connection'));
    await h.click(page.getByRole('button', { name: 'Save connection' }));
    expect((await saved).status()).toBe(200);
    await expect(page.getByText('Generation 0', { exact: true })).toBeVisible();
    await h.pause(1000);
  }
  async function selectStore(title, storeName, expectCopied) {
    const select = page.getByLabel(`Secret store for ${title}`);
    await h.choose(select, await optionLabel(select, storeName));
    const saved = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname.endsWith('/secret-store'), { timeout: 120_000 });
    await h.click(page.getByRole('button', { name: 'Save secret store' }));
    const response = await saved;
    expect(response.status(), 'secret store save').toBe(200);
    await expect(page.getByRole('status').filter({ hasText: 'Secret store saved' })).toContainText(`${expectCopied} secret(s) were copied`);
    await h.pause(1200);
  }
  await selectConnection('Staging', sourceKey);
  mark('staging-connection');
  // A Secret needs a store: the prompt appears before any store is chosen, but a variable does not.
  await h.click(page.locator('section[aria-label="Secrets"]').getByRole('button', { name: '+ Add secret' }));
  await expect(page.getByText(/Select a secret store above before adding a Secret/)).toBeVisible();
  await expect(page.getByLabel('Key name')).toHaveCount(0);
  await h.moveTo(page.getByText(/Select a secret store above/));
  mark('secret-needs-store');
  await h.pause(READ);
  await selectStore('Staging', stores[0].name, 0);
  mark('staging-store-alpha');
  await h.click(page.getByRole('tab', { name: 'Production' }));
  await selectConnection('Production', sourceKey);
  await selectStore('Production', stores[1].name, 0);
  mark('production-store-beta');
  await expect(page.getByText('Beta vault').first()).toBeVisible();
  await h.click(page.getByRole('tab', { name: 'Staging' }));
  await expect(page.getByLabel('Secret store for Staging')).toHaveValue(/alpha-vault/);
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await h.pause(1200);

  // ---- 3. Variables, a secret in the chosen store, workloads, deploy --------------
  const secret = randomBytes(32).toString('hex');
  const secretHash = createHash('sha256').update(secret).digest('hex');
  const app = acceptanceApp(runId, secret, secretHash);
  await putKeys(h, applicationName, app.keys.map((key) => ({ ...key, delay: TYPE_DELAY })), { secretStore: stores[0].name });
  await noSecretsOnScreen([secret]);
  mark('settings-saved');
  const [backend, frontend] = app.workloads;
  await addWorkload(h, backend);
  mark('backend-saved');
  await addWorkload(h, frontend);
  mark('frontend-saved');
  async function previewAndDeploy(expected, label) {
    await h.click(page.getByRole('button', { name: 'Preview changes' }));
    const preview = page.getByLabel('Deployment preview');
    await expect(preview).toContainText(`${expected} workload(s) affected`, { timeout: 120_000 });
    await h.moveTo(preview);
    mark(`${label}-preview`);
    await h.pause(3500);
    await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
    const result = page.getByLabel('Deployment result');
    await expect(result).toContainText('Deploy succeeded', { timeout: 600_000 });
    await h.moveTo(result);
    mark(`${label}-deployed`);
    await h.pause(4000);
    await noSecretsOnScreen([secret]);
  }
  await previewAndDeploy(2, 'source');
  checks(`k8s: source deployed in ${sourceNamespace}`);

  async function openApp(label) {
    const url = `http://${stagingHost}:${traefikPort}/`;
    x11.keys(['ctrl+l']);
    await h.pause(900);
    x11.type(url);
    await h.pause(SHORT_READ);
    x11.keys(['Return']);
    await expect(page).toHaveURL(url);
    await expect(page.getByRole('heading', { name: 'Acceptance application' })).toBeVisible({ timeout: 90_000 });
    mark(label);
    await h.showCursor();
    for (const name of ['backend connection', 'environment', 'secret', 'database']) {
      const row = page.locator(`[data-check="${name}"]`);
      await expect(row).toContainText('PASS', { timeout: 90_000 });
      await h.moveTo(row);
      await h.pause(700);
    }
    await noSecretsOnScreen([secret]);
  }
  const payload = `hello from ${runId}`;
  await openApp('app-source');
  await h.type(page.locator('input[name="payload"]'), payload, { replace: true });
  await h.click(page.getByRole('button', { name: 'Submit job' }));
  const job = page.locator('#jobs tr.job').filter({ hasText: payload });
  await expect(job).toBeVisible();
  await h.moveTo(job);
  mark('job-submitted');
  await h.pause(READ);
  const sourcePostgres = () => kube('-n', sourceNamespace, 'get', 'statefulset', '-o', 'name').trim().split('\n')[0].replace('statefulset.apps/', '');
  const rows = (namespace, pod) => kube('-n', namespace, 'exec', `${pod}-0`, '--', 'psql', '-U', 'acceptance', '-d', 'acceptance', '-At', '-c', 'SELECT count(*) FROM jobs').trim();
  const sourceDb = sourcePostgres();
  expect(rows(sourceNamespace, sourceDb)).toBe('1');
  checks('db: the source PostgreSQL holds the persisted job (count=1)');

  // ---- 4. Move secrets to the second store; running workloads follow after Deploy --
  await page.goto(`${baseURL}/ui/applications/${appId}/settings?environment=staging`);
  await expect(page.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
  const vscBefore = kube('-n', sourceNamespace, 'get', 'vaultstaticsecret', '-o', 'jsonpath={.items[*].spec.path}');
  await selectStore('Staging', stores[1].name, 1);
  await expect(page.getByLabel('Secret store for Staging')).toHaveValue(/beta-vault/);
  mark('store-copied');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: `← ${applicationName}` }));
  await previewAndDeploy(1, 'store-rollout');
  const connections = kube('-n', sourceNamespace, 'get', 'vaultconnection', '-o', 'jsonpath={range .items[*]}{.spec.address}{"\\n"}{end}').trim().split('\n');
  const auths = kube('-n', sourceNamespace, 'get', 'vaultauth', '-o', 'jsonpath={range .items[*]}{.spec.mount}{"\\n"}{end}').trim().split('\n');
  if (!connections.includes(stores[0].workload) || !connections.includes(stores[1].workload)) throw new Error('both store-specific VaultConnections must exist side by side');
  if (!auths.includes('kubernetes') || !auths.includes('k8s-b')) throw new Error('both auth mounts must be pinned in their own VaultAuth');
  checks(`vso: VaultConnection per store kept (${connections.length}) and auth mounts ${[...new Set(auths)].join(',')} pinned; old synced path ${vscBefore ? 'retained' : 'n/a'}`);
  await openApp('app-after-store-switch');
  await expect(page.locator('#jobs tr.job').filter({ hasText: payload })).toBeVisible();
  mark('store-switch-verified');
  await h.pause(READ);

  // ---- 5. Transition preview, stale rejection from a second tab, execution -------
  await page.goto(`${baseURL}/ui/applications/${appId}/settings?environment=staging`);
  await expect(page.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
  const connectionSelect = page.getByLabel('Connection for Staging');
  await h.choose(connectionSelect, await optionLabel(connectionSelect, `(${destKey})`));
  await expect(page.getByText(/already has runtime resources/)).toBeVisible();
  await expect(page.getByRole('button', { name: 'Save connection' })).toHaveCount(0);
  mark('runtime-requires-transition');
  await h.pause(READ);
  await h.click(page.getByRole('button', { name: 'Review transition…' }));
  const panel = page.getByRole('region', { name: 'Connection transition' });
  await expect(panel.getByLabel('Destination connection')).toHaveValue(destKey);
  await h.click(panel.getByRole('button', { name: 'Preview transition' }));
  const impact = panel.getByLabel('Transition preview');
  await expect(impact).toContainText('Redeployed unchanged', { timeout: 120_000 });
  await expect(impact.getByLabel('Database mapping')).toContainText('postgres.default#modules.backend.externals.db');
  await h.moveTo(impact);
  mark('transition-preview');
  await h.pause(LONG_READ);

  // Second tab edits configuration while the first preview is open.
  const second = await context.newPage();
  await second.goto(`${baseURL}/ui/applications/${appId}/settings?environment=staging`);
  await second.bringToFront();
  const h2 = createHuman(second, { selectNative: nativeSelect(x11) });
  await expect(second.getByRole('heading', { name: 'Environment settings', level: 1 })).toBeVisible();
  await h2.showCursor();
  await h2.click(second.locator('section[aria-label="Environment variables"]').getByRole('button', { name: '+ Add variable' }));
  await h2.type(second.getByLabel('Key name'), 'RELEASE_NOTE');
  await h2.type(second.getByLabel('Value'), 'second-tab-edit');
  const edited = second.waitForResponse((response) => response.request().method() === 'PUT' && response.url().endsWith('/configuration/keys/RELEASE_NOTE'));
  await h2.click(second.getByRole('button', { name: 'Save pending change' }));
  expect((await edited).status()).toBe(200);
  await expect(second.locator('.settings-table-row').filter({ hasText: 'RELEASE_NOTE' })).toBeVisible();
  mark('second-tab-edit');
  await h2.pause(READ);
  await second.close();
  await page.bringToFront();
  x11.focusBrowser();
  await h.showCursor();
  await h.click(impact.getByLabel(/application is stopped while its data is copied/));
  const stale = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/connection-transitions'));
  await h.click(impact.getByRole('button', { name: 'Start transition' }));
  expect((await stale).status(), 'stale preview').toBe(409);
  await expect(panel.getByRole('alert')).toContainText('preview is stale');
  await expect(panel.getByLabel('Transition preview')).toHaveCount(0);
  mark('stale-preview-rejected');
  await h.moveTo(panel.getByRole('alert'));
  await h.pause(READ);
  expect(kubeTry('get', 'namespace', '-l', `orchestrator.io/application=${appId}`, '-o', 'name').trim().split('\n').filter(Boolean)).toEqual([`namespace/${sourceNamespace}`]);
  checks('conflict: stale transition Preview was rejected before any destination namespace existed');

  // Preview again (now includes the second tab's change), acknowledge, run.
  await h.click(panel.getByRole('button', { name: 'Preview transition' }));
  await expect(panel.getByLabel('Transition preview')).toContainText('Redeployed unchanged', { timeout: 120_000 });
  await h.moveTo(panel.getByLabel('Transition preview'));
  mark('transition-preview-2');
  await h.pause(SHORT_READ);
  await h.click(panel.getByLabel(/application is stopped while its data is copied/));
  const started = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/connection-transitions'));
  await h.click(panel.getByRole('button', { name: 'Start transition' }));
  expect((await started).status()).toBe(202);
  await expect(panel.getByLabel('Transition progress')).toBeVisible();
  mark('transition-started');
  await h.moveTo(panel.getByLabel('Transition progress'));
  await expect(panel.getByText('The destination is live. The source generation is retained and quiesced')).toBeVisible({ timeout: 1_200_000 });
  await h.moveTo(panel.getByLabel('Transition progress'));
  mark('transition-succeeded');
  await h.pause(LONG_READ);
  await noSecretsOnScreen([secret]);

  // ---- 6. Evidence: destination generation, restored data, route, quiesced source -
  const namespaces = kube('get', 'namespace', '-l', `orchestrator.io/application=${appId}`, '-o', 'name').trim().split('\n').map((line) => line.replace('namespace/', ''));
  const destNamespace = namespaces.find((name) => name !== sourceNamespace);
  if (namespaces.length !== 2 || !destNamespace || !/-g1-[0-9a-f]{8}$/.test(destNamespace)) throw new Error(`unexpected namespaces ${namespaces.join(',')}`);
  const destDb = kube('-n', destNamespace, 'get', 'statefulset', '-o', 'name').trim().split('\n')[0].replace('statefulset.apps/', '');
  expect(rows(destNamespace, destDb)).toBe('1');
  expect(kube('-n', destNamespace, 'exec', `${destDb}-0`, '--', 'psql', '-U', 'acceptance', '-d', 'acceptance', '-At', '-c', 'SELECT payload FROM jobs').trim()).toBe(payload);
  expect(rows(sourceNamespace, sourceDb)).toBe('1');
  for (const name of ['backend', 'frontend']) {
    expect(kube('-n', sourceNamespace, 'get', 'deployment', name, '-o', 'jsonpath={.spec.replicas}').trim(), `source ${name} replicas`).toBe('0');
    expect(Number(kube('-n', destNamespace, 'get', 'deployment', name, '-o', 'jsonpath={.status.availableReplicas}').trim())).toBeGreaterThanOrEqual(1);
  }
  const owners = kube('get', 'ingress', '-A', '-l', `orchestrator.io/application=${appId}`, '-o', 'jsonpath={range .items[*]}{.metadata.namespace}{"\\n"}{end}').trim().split('\n').filter(Boolean);
  expect(owners).toEqual([destNamespace]);
  checks(`transition: destination ${destNamespace} holds the restored job; source ${sourceNamespace} retained with its data and quiesced; Ingress owned only by the destination`);
  await openApp('app-destination');
  await expect(page.locator('#jobs tr.job').filter({ hasText: payload })).toBeVisible();
  mark('restored-record-over-http');
  await h.moveTo(page.locator('#jobs tr.job').filter({ hasText: payload }));
  await h.pause(READ);
  const httpBody = await page.evaluate(async () => (await fetch('/api/checks')).json());
  expect(httpBody.checks).toEqual({ environment: true, secret: true, database: true });
  checks('http: the destination served /api/checks (environment, secret via VSO, database PASS) and the restored job through the public route');

  // ---- 7. Refresh and backend restart keep the persisted truth -------------------
  await page.goto(`${baseURL}/ui/applications/${appId}/settings?environment=staging`);
  await expect(page.getByText('Generation 1', { exact: true })).toBeVisible();
  await expect(page.getByLabel('Connection for Staging')).toHaveValue(destKey);
  await expect(page.getByLabel('Secret store for Staging')).toHaveValue(/beta-vault/);
  mark('refresh-consistent');
  await h.pause(SHORT_READ);
  writeFileSync(`${evidenceDir}/restart-request`, 'restart');
  await expect.poll(() => existsSync(`${evidenceDir}/restart-done`), { timeout: 120_000 }).toBe(true);
  await page.reload();
  await expect(page.getByText('Generation 1', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Clean up source generation' })).toBeVisible();
  await h.moveTo(page.getByRole('button', { name: 'Clean up source generation' }));
  mark('restart-consistent');
  await h.pause(READ);

  // ---- 8. Explicit cleanup of the retained source generation ----------------------
  const cleaned = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/cleanup-source'), { timeout: 400_000 });
  await h.click(page.getByRole('button', { name: 'Clean up source generation' }));
  expect((await cleaned).status()).toBe(200);
  await expect(page.getByText('The destination is live and the source generation was cleaned up.')).toBeVisible({ timeout: 60_000 });
  mark('cleanup-done');
  expect(kubeTry('get', 'namespace', sourceNamespace, '--ignore-not-found', '-o', 'name').trim()).toBe('');
  expect(rows(destNamespace, destDb)).toBe('1');
  checks(`cleanup: source ${sourceNamespace} deleted on explicit request; destination ${destNamespace} kept with its data`);
  await h.pause(LONG_READ);

  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId: appId, sourceNamespace, destinationNamespace: destNamespace, sourceKey, destKey, stagingHost, subdomain }, null, 2));
  console.log(`PASS: application=${appId} source=${sourceNamespace} destination=${destNamespace} marks=${marks.length}`);
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    await browser.close();
  }
}
