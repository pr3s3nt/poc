// Human-paced recording that registers a NEW kind cluster in the persistent
// Docker Web Console and deploys the diagnostic acceptance app (frontend,
// backend, PostgreSQL) to it, retained afterwards. Headed Chromium on a private
// Xvfb display; ffmpeg records the whole window. Invoked by
// backend/test/integration/k8s4f-dev-playwright.sh.
//
// Real adapters only: the Console is the Compose backend, the target is the
// physical kind cluster, the Secret Store is a dedicated workload Vault. Every
// product mutation is a UI action. API reads only assert what the UI saved.
// Kubeconfig content, the store token and the synthetic app secret are never
// typed into a visible field, printed or captioned. No worker is deployed, so
// the submitted job stays PENDING.
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { execFileSync, spawn } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { basename } from 'node:path';
import { createCaptions } from './captions.mjs';
import { acceptanceApp, addWorkload, createApplication, createHuman, installCursor, previewAndDeploy, putKeys, reviewAcceptancePage, setEnvironmentConnections, TYPE_DELAY } from './human.mjs';
import { assertNoSecretLeaks } from './secret-scan.mjs';
import { launchHeaded, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const env = process.env;
for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_KUBE_CONTEXT', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_KUBECONFIG_FILE', 'ORCH_E2E_STORE_TOKEN_FILE',
  'ORCH_E2E_STORE_BACKEND', 'ORCH_E2E_STORE_WORKLOAD', 'ORCH_E2E_FORWARD_SCRIPT', 'ORCH_E2E_FORWARD_PORT', 'ORCH_E2E_FORWARD_PIDFILE', 'DISPLAY']) {
  if (!env[name]) throw new Error(`${name} is required`);
}
const baseURL = env.ORCH_E2E_URL;
const runId = env.ORCH_E2E_RUN_ID;
const kubeContext = env.ORCH_E2E_KUBE_CONTEXT;
const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
const kubeconfigFile = env.ORCH_E2E_KUBECONFIG_FILE;
const forwardPort = env.ORCH_E2E_FORWARD_PORT;
if (!/^[a-z0-9-]+$/.test(runId)) throw new Error('ORCH_E2E_RUN_ID must be a DNS label');
const { width, height } = screenSize(env);
const videoPath = `${evidenceDir}/k8s4f-dev-raw.mp4`;
const connectionName = env.ORCH_E2E_CONNECTION_NAME ?? 'K8S-4F';
const storeName = env.ORCH_E2E_STORE_NAME ?? 'K8S-4F Vault';
const applicationName = env.ORCH_E2E_APPLICATION_NAME ?? runId;
const subdomain = env.ORCH_E2E_SUBDOMAIN ?? runId;
const reuseApp = env.ORCH_E2E_REUSE_APP === '1';
const reuse = env.ORCH_E2E_REUSE === '1';
const storeToken = readFileSync(env.ORCH_E2E_STORE_TOKEN_FILE, 'utf8').trim();
const kubeconfig = readFileSync(kubeconfigFile, 'utf8');
const credentialValues = [storeToken, ...[...kubeconfig.matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)].map((m) => m[1].replace(/^["']|["']$/g, ''))].filter((v) => v.length >= 12);

const redactions = [...credentialValues];
const READ = 3000;
const SHORT_READ = 2000;
const checks = (line) => { console.log(line); writeFileSync(`${evidenceDir}/assertions.txt`, `${line}\n`, { flag: 'a' }); };
const kube = (...args) => execFileSync('kubectl', ['--context', kubeContext, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });

const browser = await launchHeaded({ width, height });
const context = await browser.newContext({ viewport: null });
context.setDefaultTimeout(30_000);
await context.addInitScript(installCursor);
const marks = [];
let recorder;
let startedAt;
let result = 'FAIL';
const captions = createCaptions(() => (startedAt ? Number(((Date.now() - startedAt) / 1000).toFixed(1)) : 0));
try {
  const page = await context.newPage();
  const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
  // Native <select> by arrow keys: the type-ahead of nativeSelect() would pick
  // "k8s-4f" for "K8S-4F" because option labels differ only in case and suffix.
  const selectByArrows = async (human, select, label) => {
    await human.click(select);
    await human.pause(700);
    const labels = await select.locator('option').allTextContents();
    const current = await select.evaluate((element) => element.selectedIndex);
    const target = labels.indexOf(label);
    if (target < 0) throw new Error('option not offered');
    if (target !== current) x11.keys(Array(Math.abs(target - current)).fill(target > current ? 'Down' : 'Up'));
    await human.pause(500);
    x11.keys(['Return']);
    await human.pause(700);
    await expect.poll(() => select.evaluate((element) => element.selectedOptions[0]?.textContent ?? '')).toBe(label);
  };
  const h = createHuman(page, { selectNative: selectByArrows });
  await page.goto(`${baseURL}/ui/sign-in`);
  await expect(page.getByLabel('Username')).toBeVisible();
  await page.waitForTimeout(1500);
  x11.focusBrowser();
  recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
  startedAt = Date.now();
  await page.waitForTimeout(1500);
  const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
  const { caption } = captions;

  const secret = randomBytes(32).toString('hex');
  redactions.push(secret);
  const app = acceptanceApp(runId, secret, createHash('sha256').update(secret).digest('hex'));
  const noLeaks = async ({ activeField = false } = {}) => assertNoSecretLeaks(page, [secret, ...credentialValues], { allow: activeField ? ['input[type="password"]'] : [] });
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
  const apiJson = async (path) => {
    const response = await page.request.get(`${baseURL}${path}`);
    expect(response.ok(), `GET ${path}`).toBe(true);
    const text = await response.text();
    for (const value of credentialValues) if (text.includes(value)) throw new Error(`credential material in ${path} response`);
    return JSON.parse(text);
  };

  // 1. Platform Engineer: baseline of the existing, unrelated records.
  caption('Bước 1: Kỹ sư nền tảng đăng nhập Web Console Docker đang chạy lâu dài (http://127.0.0.1:3001)');
  mark('sign-in');
  await signIn('platform-engineer');
  const baseline = await apiJson('/api/v1/connections');
  const oldConnection = baseline.connections.find((c) => c.key === 'k8s-4f');
  expect(oldConnection, 'existing k8s-4f Connection').toBeTruthy();
  const baselineSnapshot = JSON.stringify(oldConnection);
  const baselineStoreRecords = Object.fromEntries((await apiJson('/api/v1/secret-stores')).secretStores.map((s) => [s.key, JSON.stringify(s)]));

  // 2. Register the NEW Connection through the UI (kubeconfig only via file chooser).
  caption('Bước 2: Mở Platform → Connections; Connection k8s-4f cũ giữ nguyên, không sửa');
  const rows = () => page.getByRole('table', { name: 'Registered connections' }).locator('tbody tr');
  let connectionKey;
  let storeKey;
  if (reuse) {
    // Resume after an aborted run: the records registered through the UI by that run are reused.
    // Identity preflight, before anything is deployed: exactly one READY match,
    // compared with the target derived from the private kubeconfig input.
    const inspectReply = await page.request.post(`${baseURL}/api/v1/connections/kubernetes/inspect`, { data: { kubeconfig } });
    expect(inspectReply.status(), 'kubeconfig inspect').toBe(200);
    const inspectedContexts = (await inspectReply.json()).contexts;
    expect(inspectedContexts.length, 'kubeconfig contexts').toBe(1);
    const intended = { kubeContext: inspectedContexts[0].name, endpoint: inspectedContexts[0].endpoint };
    expect(intended.kubeContext).toBe(kubeContext);
    const connectionMatches = baseline.connections.filter((c) => c.name === connectionName && c.key !== 'k8s-4f');
    expect(connectionMatches.length, 'reusable Connection matches').toBe(1);
    const [reusedConnection] = connectionMatches;
    expect({ kind: reusedConnection.kind, auth: reusedConnection.authenticationType, status: reusedConnection.status, kubeContext: reusedConnection.config?.kubeContext, endpoint: reusedConnection.config?.endpoint })
      .toEqual({ kind: 'KUBERNETES', auth: 'KUBECONFIG', status: 'READY', kubeContext: intended.kubeContext, endpoint: intended.endpoint });
    connectionKey = reusedConnection.key;
    const storeMatches = (await apiJson('/api/v1/secret-stores')).secretStores.filter((st) => st.name === storeName);
    expect(storeMatches.length, 'reusable Secret Store matches').toBe(1);
    const [reusedStore] = storeMatches;
    expect({ status: reusedStore.status, backend: reusedStore.backendAddress, workload: reusedStore.workloadAddress, mount: reusedStore.mount, authMount: reusedStore.authMount, auth: reusedStore.verification?.kubernetesAuth })
      .toEqual({ status: 'READY', backend: env.ORCH_E2E_STORE_BACKEND, workload: env.ORCH_E2E_STORE_WORKLOAD, mount: 'kv', authMount: 'kubernetes', auth: 'CONFIGURED' });
    storeKey = reusedStore.key;
    checks(`reuse-preflight: Connection ${connectionKey} and store ${storeKey} matched by identity (single READY match)`);
    expect(connectionKey && storeKey, 'reusable Connection and Secret Store').toBeTruthy();
    caption(`Connection ${connectionName} (${connectionKey}) và Secret store ${storeName} (${storeKey}) đã đăng ký từ lần chạy trước, dùng lại`);
    await h.click(page.getByRole('link', { name: /Connections/ }));
    await h.pause(READ);
    await h.click(page.getByRole('link', { name: /Secret stores/ }));
    await h.pause(READ);
    mark('connection-ready');
    await signOut();
  } else {
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();
  await expect(rows().filter({ hasText: 'k8s-4f' }).first()).toContainText('READY');
  await h.moveTo(rows().first());
  mark('connections-list');
  await h.pause(READ);
  caption(`Bước 3: Đăng ký Connection mới tên đúng "${connectionName}": nhập tên, chọn file kubeconfig của cụm kind mới (nội dung không hiển thị)`);
  await h.type(page.getByLabel('Connection name'), connectionName);
  await h.click(page.getByLabel('Upload kubeconfig file'));
  const input = page.locator('input[type="file"]');
  const chooser = page.waitForEvent('filechooser');
  chooser.catch(() => {});
  await h.click(input);
  await (await chooser).setFiles(kubeconfigFile);
  await expect(page.getByText(`Selected file: ${basename(kubeconfigFile)}.`, { exact: false })).toBeVisible();
  await h.pause(800);
  const inspected = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/connections/kubernetes/inspect');
  await h.click(page.getByRole('button', { name: 'Inspect kubeconfig' }));
  const inspectResponse = await inspected;
  for (const value of credentialValues) if ((await inspectResponse.text()).includes(value)) throw new Error('credential material in inspect response');
  expect(inspectResponse.status()).toBe(200);
  const summary = page.getByLabel('Selected destination');
  await expect(summary).toContainText(`Context ${kubeContext}`);
  await h.moveTo(summary);
  mark('connection-inspected');
  caption(`Bước 3 (tiếp): Inspect thấy đúng context ${kubeContext}; bấm Check and save để Console kiểm tra cụm thật`);
  await h.pause(READ);
  await noLeaks();
  const registered = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 90_000 });
  await h.click(page.getByRole('button', { name: 'Check and save' }));
  const registeredResponse = await registered;
  const createdText = await registeredResponse.text();
  for (const value of credentialValues) if (createdText.includes(value)) throw new Error('credential material in registration response');
  expect(registeredResponse.status()).toBe(201);
  const created = JSON.parse(createdText);
  connectionKey = created.key;
  expect(created).toMatchObject({ name: connectionName, kind: 'KUBERNETES', authenticationType: 'KUBECONFIG', status: 'READY' });
  expect(connectionKey, 'server-assigned key must differ from the existing k8s-4f').not.toBe('k8s-4f');
  expect(created.verification?.verified).toBe(true);
  await expect(page.getByRole('status')).toContainText(`Registered connection ${connectionName} (${connectionKey}).`);
  const entry = rows().filter({ has: page.getByText(connectionName, { exact: true }) });
  await expect(entry).toContainText('READY');
  await h.moveTo(entry);
  mark('connection-ready');
  caption(`Kết quả: Connection "${connectionName}" READY, key do server cấp: ${connectionKey}`);
  checks(`connection: name=${connectionName} key=${connectionKey} status=READY verified=true`);
  await h.pause(READ);
  await noLeaks();

  caption('Bước 4: Platform → Secret stores: đăng ký Vault riêng của cụm mới (token dán vào ô mật khẩu, luôn bị che)');
  await h.click(page.getByRole('link', { name: /Secret stores/ }));
  await expect(page.getByRole('heading', { name: 'Secret stores', level: 1 })).toBeVisible();
  await h.pause(SHORT_READ);
  await h.type(page.getByLabel(/^Name/), storeName);
  await h.type(page.getByLabel(/^Backend address/), env.ORCH_E2E_STORE_BACKEND);
  await h.type(page.getByLabel(/^Workload address/), env.ORCH_E2E_STORE_WORKLOAD);
  await h.type(page.getByLabel(/^KV v2 mount/), 'kv', { replace: true });
  await h.type(page.getByLabel(/^Kubernetes auth mount/), 'kubernetes', { replace: true });
  await h.paste(page.getByLabel(/^Token/), storeToken);
  await expect(page.getByLabel(/^Token/)).toHaveAttribute('type', 'password');
  await noLeaks({ activeField: true });
  mark('store-form');
  const storeRegistered = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/secret-stores', { timeout: 90_000 });
  await h.click(page.getByRole('button', { name: 'Verify and register' }));
  const storeResponse = await storeRegistered;
  const storeText = await storeResponse.text();
  if (storeText.includes(storeToken)) throw new Error('store token echoed by the registration response');
  expect(storeResponse.status()).toBe(201);
  const storeBody = JSON.parse(storeText);
  storeKey = storeBody.key ?? storeBody.secretStore?.key;
  expect(storeKey, 'registered store key').toBeTruthy();
  await expect(page.getByRole('status')).toContainText(`"${storeName}" is READY`);
  const storeRow = page.locator('.settings-table-row').filter({ hasText: storeName });
  await expect(storeRow).toContainText('READY');
  await h.moveTo(storeRow);
  mark('store-ready');
  caption(`Kết quả: Secret store "${storeName}" READY (key ${storeKey}); Kubernetes auth đã cấu hình`);
  checks(`secret-store: name=${storeName} key=${storeKey} status=READY`);
  await h.pause(READ);
  expect((await page.getByLabel(/^Token/).inputValue()) === '', 'token field cleared').toBe(true);
  await noLeaks();
  await signOut();
  }
  mark('platform-engineer-signed-out');

  // 4. Developer: Application, staging Connection and store.
  caption('Bước 5: Developer đăng nhập, tạo Application mới và chọn Connection staging vừa đăng ký');
  await signIn('developer');
  let appId;
  if (reuseApp) {
    await h.click(page.getByText(applicationName, { exact: true }).first());
    await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
    appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
    await setEnvironmentConnections(h, applicationName, { staging: connectionKey });
  } else {
    appId = await createApplication(h, applicationName, subdomain, { connections: { staging: connectionKey } });
  }
  const namespace = `app-${appId}-staging`;
  writeFileSync(`${evidenceDir}/run.json`, JSON.stringify({ applicationId: appId, applicationName, namespace, connectionKey, storeKey, imageTag: runId }, null, 2));
  await expect(page.getByLabel('Execution target').first()).toContainText(`Connection ${connectionName} (${connectionKey})`);
  mark('application-created');
  caption(`Kết quả: Application "${applicationName}" (id ${appId}), Staging chạy trên Connection ${connectionName} (${connectionKey})`);
  await h.moveTo(page.getByLabel('Execution target').first());
  await h.pause(READ);

  // 5. Configuration (secret store selection + variables/secret).
  caption('Bước 6: Environment settings: chọn Secret store, thêm 2 biến và 1 secret (giá trị secret bị che)');
  await putKeys(h, applicationName, app.keys.map((key) => ({ ...key, delay: TYPE_DELAY })), { secretStore: storeKey });
  await noLeaks();
  mark('settings-saved');

  // 6. Workloads.
  caption('Bước 7: Thêm workload backend: image, resource postgres db, binding PGHOST/PGPORT/... và 3 key cấu hình');
  const [backend, frontend] = app.workloads;
  const backendScore = await addWorkload(h, backend);
  expect(backendScore.resources).toEqual({
    db: { type: 'postgres', class: 'default', params: { database: 'acceptance', username: 'acceptance' } },
    env: { type: 'environment' },
  });
  expect(JSON.stringify(backendScore).includes(secret), 'secret value in Score').toBe(false);
  mark('backend-saved');
  caption('Bước 8: Thêm workload frontend: BACKEND_URL lấy từ Service của backend');
  const frontendScore = await addWorkload(h, frontend);
  expect(frontendScore.resources).toEqual({ svc_backend_http: { type: 'service', params: { workload: 'backend', port: 'http' } } });
  mark('frontend-saved');

  // 7. Preview and Deploy to the new cluster.
  caption('Bước 9: Preview changes rồi Deploy these changes lên Staging của cụm mới (adapter Kubernetes thật)');
  await expect(page.getByLabel('Execution target').first()).toContainText(`(${connectionKey})`);
  await previewAndDeploy(h, ['backend', 'frontend'], { mark });
  await noLeaks();
  caption('Kết quả: backend và frontend deploy succeeded');
  await h.pause(SHORT_READ);

  // 8. Metadata-safe cluster proof (names, phases and counts only).
  expect(kube('config', 'view', '--minify', '-o', 'jsonpath={.clusters[0].name}')).toBe('kind-k8s-4f');
  expect(kube('get', 'nodes', '-o', 'jsonpath={.items[*].metadata.name}')).toBe('k8s-4f-control-plane');
  await expect.poll(() => {
    const items = JSON.parse(kube('-n', namespace, 'get', 'deployments,statefulsets,pvc', '-o', 'json')).items;
    const ready = (kind, name) => items.some((i) => i.kind === kind && i.metadata.name === name && (i.status.readyReplicas ?? 0) >= 1);
    const sts = items.filter((i) => i.kind === 'StatefulSet' && (i.status.readyReplicas ?? 0) >= 1);
    const bound = items.filter((i) => i.kind === 'PersistentVolumeClaim' && i.status.phase === 'Bound');
    return ready('Deployment', 'backend') && ready('Deployment', 'frontend') && sts.length >= 1 && bound.length >= 1;
  }, { timeout: 240_000, intervals: [3000] }).toBe(true);
  const proof = JSON.parse(kube('-n', namespace, 'get', 'deployments,statefulsets,pods,pvc,svc', '-o', 'json')).items.map((i) => ({
    kind: i.kind, name: i.metadata.name, ready: i.status?.readyReplicas ?? i.status?.phase ?? null, node: i.spec?.nodeName ?? null,
  }));
  writeFileSync(`${evidenceDir}/cluster-proof.json`, JSON.stringify({ context: kubeContext, namespace, labels: JSON.parse(kube('get', 'namespace', namespace, '-o', 'json')).metadata.labels, items: proof }, null, 2));
  checks(`cluster: context=${kubeContext} node=k8s-4f-control-plane namespace=${namespace} deployments backend,frontend ready; postgres StatefulSet ready; PVC Bound`);
  mark('cluster-verified');

  // 9. Retained frontend URL (detached port-forward owned by the user) opened by typing.
  const forward = spawn('setsid', ['-f', env.ORCH_E2E_FORWARD_SCRIPT, kubeContext, namespace, 'frontend', forwardPort, '8080', env.ORCH_E2E_FORWARD_PIDFILE], { stdio: 'ignore', detached: true });
  forward.unref();
  const appURL = `http://127.0.0.1:${forwardPort}/`;
  await expect.poll(async () => (await fetch(appURL).catch(() => ({ ok: false }))).ok, { timeout: 60_000, intervals: [1000] }).toBe(true);
  caption(`Bước 10: Mở frontend thật qua ${appURL} (port-forward giữ lại), gõ địa chỉ vào thanh địa chỉ`);
  x11.keys(['ctrl+l']);
  await h.pause(900);
  x11.type(appURL);
  await h.pause(SHORT_READ);
  x11.keys(['Return']);
  await expect(page).toHaveURL(appURL);
  mark('app-opened');
  await reviewAcceptancePage(h, runId, secret);
  const payload = `hello from ${runId}`;
  const job = page.locator('#jobs tr.job').filter({ hasText: payload });
  await expect(job).toContainText('PENDING');
  await noLeaks();
  mark('job-submitted');
  caption('Kết quả: 4 kiểm tra backend connection / environment / secret / database đều PASS; job được nhận, trạng thái PENDING (không có worker)');
  checks('app: 4 diagnostic checks PASS (backend connection, environment, secret, database); job accepted and PENDING');
  await h.pause(READ);

  // 10. Persistence: reload the app, then reopen the Console and the Application.
  caption('Bước 11: Tải lại frontend — job vẫn còn trong PostgreSQL');
  await page.reload();
  await expect(page.locator('#jobs tr.job').filter({ hasText: payload })).toContainText('PENDING');
  await h.pause(READ);
  caption('Bước 12: Mở lại Web Console, kiểm tra Application, Connection và workload còn nguyên sau khi làm mới');
  x11.keys(['ctrl+l']);
  await h.pause(900);
  x11.type(`${baseURL}/ui/`);
  await h.pause(SHORT_READ);
  x11.keys(['Return']);
  await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  await h.click(page.getByText(applicationName, { exact: true }).first());
  await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
  await page.reload();
  await expect(page.getByLabel('Execution target').first()).toContainText(`Connection ${connectionName} (${connectionKey})`);
  await expect(page.locator('.table-row').filter({ hasText: 'backend' }).first()).toBeVisible();
  await expect(page.locator('.table-row').filter({ hasText: 'frontend' }).first()).toBeVisible();
  await h.moveTo(page.getByLabel('Execution target').first());
  mark('persisted');
  caption(`Kết quả: Application vẫn dùng Connection ${connectionName} (${connectionKey}); hai workload còn nguyên`);
  await h.pause(5000);
  await signOut();
  caption('Bước 13: Kỹ sư nền tảng xem lại danh sách Connection: bản k8s-4f cũ không đổi, bản mới đang READY');
  await signIn('platform-engineer');
  await h.click(page.getByRole('link', { name: /Connections/ }));
  await expect(rows().filter({ hasText: connectionKey }).first()).toContainText('READY');
  await h.moveTo(rows().filter({ hasText: connectionKey }).first());
  const after = await apiJson('/api/v1/connections');
  expect(JSON.stringify(after.connections.find((c) => c.key === 'k8s-4f')), 'existing k8s-4f Connection unchanged').toBe(baselineSnapshot);
  const storesAfter = Object.fromEntries((await apiJson('/api/v1/secret-stores')).secretStores.map((s) => [s.key, JSON.stringify(s)]));
  for (const [key, record] of Object.entries(baselineStoreRecords)) expect(storesAfter[key], `store ${key} record unchanged`).toBe(record);
  expect(Object.keys(storesAfter)).toContain(storeKey);
  checks(`persistence: reload OK; old Connection k8s-4f record unchanged; new key ${connectionKey}; pre-existing store records unchanged (${Object.keys(baselineStoreRecords).join(',')})`);
  mark('final-connections');
  await h.pause(READ);
  caption('Hoàn tất: cụm kind-k8s-4f, ứng dụng và PostgreSQL được giữ lại');
  await h.pause(SHORT_READ);
  result = 'PASS';
  console.log(`PASS: application=${appId} namespace=${namespace} connection=${connectionKey} store=${storeKey} frontend=${appURL}`);
} catch (error) {
  // Playwright messages can quote field values; credentials never reach the log.
  let text = String(error?.stack ?? error);
  for (const value of redactions) text = text.split(value).join('[redacted]');
  console.error(text);
  process.exitCode = 1;
} finally {
  try {
    if (recorder) await stopRecording(recorder);
  } finally {
    const total = startedAt ? (Date.now() - startedAt) / 1000 : 0;
    writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
    if (startedAt) captions.writeSrt(`${evidenceDir}/captions.srt`, total);
    writeFileSync(`${evidenceDir}/result.txt`, `${result}\n`);
    await browser.close();
  }
}
