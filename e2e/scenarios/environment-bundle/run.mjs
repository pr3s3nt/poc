// One Fleet bundle per Environment with versioned secrets — issue #67, ADR-042.
// Every business action goes through the Console UI as a real user: the Admin
// creates the developer and selects the Environment secret store, the
// developer creates the Application, keys, workloads, previews, deploys,
// rolls back and deletes. Kubernetes/Vault reads are read-only assertions;
// deleting one Pod (UID precondition) and forcing a Fleet resync are fault
// injection. See README.md.
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { closeConsole, openConsole, readConsoleConfig, readPrivateFile, signInContext, signOutContext } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';

process.umask(0o077);
const STAGES = ['users', 'prepare', 'deploy', 'verify-deploy', 'image', 'secret', 'fail', 'rollback', 'resync', 'cleanup', 'recover', 'verify-rollback'];
const stage = process.argv[2];
assert(STAGES.includes(stage), 'stage: ' + STAGES.join('|'));
const dir = process.env.ORCH_TEST_RUN_DIR;
assert(dir, 'ORCH_TEST_RUN_DIR');
outsideRepo(dir);
const ENV = 'staging';
const CONTEXT = process.env.ORCH_KUBE_CONTEXT || 'kind-k8s-4f';
const CONSOLE_NS = process.env.ORCH_CONSOLE_NAMESPACE;
assert(CONSOLE_NS, 'ORCH_CONSOLE_NAMESPACE: namespace of the Console and its Vault');
const TAG = process.env.ORCH_ACCEPTANCE_IMAGE_TAG || 'issue67-v1';
const TAG2 = process.env.ORCH_ACCEPTANCE_IMAGE_TAG2 || 'issue67-v2';
const STORE = process.env.ORCH_SECRET_STORE_NAME || 'Issue67 Vault';
const file = join(dir, 'state.json');
const state = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : {};
const save = () => writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
const assertions = [];
const pass = (text) => { assertions.push(text); console.log('PASS ' + text); };
const step = (text) => console.log('- ' + text);

// ---------- read-only cluster and Vault assertions ----------
const kubectl = (...args) => execFileSync('kubectl', ['--context', CONTEXT, ...args], { encoding: 'utf8', timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const kjson = (...args) => JSON.parse(kubectl(...args, '-o', 'json'));
const ns = () => state.namespace;
const items = (kind, selector) => kjson('-n', ns(), 'get', kind, ...(selector ? ['-l', selector] : [])).items;
const byName = (list) => Object.fromEntries(list.map((o) => [o.metadata.name, o]));
function vaultVersion(workload) {
  const path = `kv/orchestrator/apps/${state.app.key}/envs/${ENV}/values/workload-${workload}`;
  try {
    const out = execFileSync('kubectl', ['--context', CONTEXT, '-n', CONSOLE_NS, 'exec', 'deploy/vault', '--', 'sh', '-c', `VAULT_TOKEN=$(cat /app-token/token) vault kv metadata get -format=json ${path}`], { encoding: 'utf8', timeout: 60_000, stdio: ['ignore', 'pipe', 'pipe'] });
    return JSON.parse(out).data.current_version;
  } catch { return 0; }
}
function snapshot() {
  const vss = byName(items('vaultstaticsecrets.secrets.hashicorp.com'));
  const secrets = byName(items('secrets').filter((s) => s.metadata.ownerReferences?.some((o) => o.kind === 'VaultStaticSecret')));
  const accounts = byName(items('serviceaccounts').filter((a) => a.metadata.name.startsWith('orch-')));
  const auths = byName(items('vaultauths.secrets.hashicorp.com'));
  const uid = (map) => Object.fromEntries(Object.entries(map).map(([k, v]) => [k, v.metadata.uid]));
  return { vss: uid(vss), secrets: uid(secrets), accounts: uid(accounts), auths: uid(auths), retained: Object.keys(vss).filter((k) => vss[k].metadata.annotations?.['orchestrator.io/retained-secret'] === 'true').sort() };
}
function podSecrets(workload) {
  const pods = items('pods', `app.kubernetes.io/name=${workload}`).filter((p) => !p.metadata.deletionTimestamp && p.status.phase === 'Running');
  return pods.map((p) => ({ name: p.metadata.name, uid: p.metadata.uid, ready: p.status.containerStatuses?.every((c) => c.ready), secrets: [...new Set(p.spec.containers.flatMap((c) => (c.env ?? []).map((e) => e.valueFrom?.secretKeyRef?.name).filter(Boolean)))] }));
}
const deploymentSecret = (workload) => [...new Set(kjson('-n', ns(), 'get', 'deployment', workload).spec.template.spec.containers.flatMap((c) => (c.env ?? []).map((e) => e.valueFrom?.secretKeyRef?.name).filter(Boolean)))];
function fleetBundles() {
  // ADR-038: the IDP controller is named after the Application's delivery repository.
  const repoName = state.app.key.replace(/-([a-z0-9]+)$/, '-deploy-$1');
  const repos = kjson('-n', 'fleet-local', 'get', 'gitrepos.fleet.cattle.io').items.filter((r) => r.metadata.name === repoName);
  const bundles = kjson('-n', 'fleet-local', 'get', 'bundles.fleet.cattle.io').items.filter((b) => repos.some((r) => b.metadata.labels?.['fleet.cattle.io/repo-name'] === r.metadata.name));
  return { repos, bundles };
}

// ---------- Console sessions ----------
const admin = readConsoleConfig();
// ORCH_DEVELOPER_USERNAME/ORCH_DEVELOPER_PASSWORD_FILE select an existing
// account (for example on a shared Console); default is the run's developer.
const developerConfig = () => ({ ...admin, username: process.env.ORCH_DEVELOPER_USERNAME || 'developer', password: readPrivateFile(process.env.ORCH_DEVELOPER_PASSWORD_FILE || join(dir, 'developer-password'), 'mật khẩu developer') });
const developerIsPlatform = () => process.env.ORCH_DEVELOPER_IS_PLATFORM === '1';
let adminSession, devSession;
async function asAdmin() { adminSession ??= await openConsole(admin, {}); return makeUI(adminSession.page, admin.baseURL, 1_800_000); }
async function asDeveloper() {
  if (!devSession) {
    const browser = adminSession?.browser ?? (await asAdmin(), adminSession.browser);
    devSession = await signInContext(browser, developerConfig(), developerIsPlatform() ? {} : { role: 'DEVELOPER' });
  }
  return makeUI(devSession.page, admin.baseURL, 1_800_000);
}
const base = () => `/applications/${state.app.key}/environments/${ENV}`;
const shot = (ui, name) => ui.page.screenshot({ path: join(dir, `${stage}-${name}.png`), fullPage: true });

// ---------- UI actions ----------
async function createDeveloper(ui) {
  const users = (await ui.get('/users')).users ?? [];
  const password = randomBytes(18).toString('base64url');
  await ui.page.goto(`${admin.baseURL}/ui/platform/users`);
  const existing = users.find((u) => u.username === 'developer');
  if (existing) {
    // A seeded developer whose password is unknown here: the Admin resets it.
    const row = ui.page.locator('.settings-table-row').filter({ has: ui.page.locator('strong').getByText('developer', { exact: true }) });
    await row.getByRole('button', { name: 'Đặt lại mật khẩu', exact: true }).click();
    const form = ui.page.getByRole('form', { name: 'Đặt lại mật khẩu của developer' });
    const inputs = form.locator('input[autocomplete="new-password"]');
    await inputs.nth(0).fill(password);
    await inputs.nth(1).fill(password);
    const [reply] = await Promise.all([
      ui.page.waitForResponse((r) => r.request().method() !== 'GET' && new URL(r.url()).pathname.startsWith('/api/v1/users/')),
      form.getByRole('button', { name: 'Đặt lại mật khẩu', exact: true }).click(),
    ]);
    assert(reply.ok(), 'password reset HTTP ' + reply.status());
    writeFileSync(join(dir, 'developer-password'), password, { mode: 0o600 });
    return;
  }
  writeFileSync(join(dir, 'developer-password'), password, { mode: 0o600 });
  const form = ui.page.getByRole('form', { name: 'Tạo tài khoản' });
  await form.getByLabel('Tên đăng nhập').fill('developer');
  await form.getByLabel('Mật khẩu').fill(password);
  await form.getByLabel('Vai trò').selectOption('DEVELOPER');
  await ui.submit('/users', 'POST', form.getByRole('button', { name: 'Tạo tài khoản' }));
}

async function createApplication(ui, name) {
  await ui.page.goto(`${ui.baseURL}/ui/applications/new`);
  await ui.page.getByLabel('Tên ứng dụng', { exact: true }).fill(name);
  return (await ui.submit('/applications', 'POST', ui.page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }))).application;
}

async function openSettings(ui) {
  await ui.page.goto(`${ui.baseURL}/ui/applications/${state.app.key}/settings?environment=${ENV}`);
  await ui.page.getByRole('heading', { name: 'Kho bí mật' }).waitFor();
}

async function selectStore(ui, storeKey) {
  await openSettings(ui);
  await ui.page.getByLabel('Kho bí mật cho Staging', { exact: true }).selectOption(storeKey);
  await ui.submit(base() + '/secret-store', 'PUT', ui.page.getByRole('button', { name: 'Lưu kho bí mật', exact: true }));
}

async function saveKey(ui, kind, name, value) {
  const section = ui.page.locator(`section[aria-label="${kind === 'SECRET' ? 'Bí mật' : 'Biến'}"]`);
  const row = section.locator('.settings-table-row').filter({ has: ui.page.locator('strong').getByText(name, { exact: true }) });
  if (await row.count()) await row.getByRole('button', { name: kind === 'SECRET' ? 'Cập nhật' : 'Sửa', exact: true }).click();
  else {
    await section.getByRole('button', { name: kind === 'SECRET' ? '+ Thêm bí mật' : '+ Thêm biến', exact: true }).click();
    await ui.page.getByLabel('Tên key', { exact: true }).fill(name);
  }
  const input = ui.page.getByLabel(kind === 'SECRET' ? 'Giá trị bí mật mới' : 'Giá trị', { exact: true });
  await input.fill(value);
  await ui.submit(base() + '/configuration/keys/' + encodeURIComponent(name), 'PUT', ui.page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai', exact: true }));
  await ui.page.getByLabel('Tên key', { exact: true }).waitFor({ state: 'detached' });
}

async function editImage(ui, name, image) {
  await ui.page.goto(`${ui.baseURL}/ui/applications/${state.app.key}/environments/${ENV}/workloads/${name}`);
  const field = ui.page.getByLabel('Ảnh Container', { exact: true }).first();
  await field.waitFor();
  await field.fill(image);
  await ui.submit(base() + '/workloads/' + name, 'PUT', ui.page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}

async function deliverySubject() {
  if (state.deliverySubject) return;
  // The delivery binding is platform configuration: read it as the Admin.
  const ui = await asAdmin();
  const binding = await ui.get('/delivery-default');
  const details = await ui.get(`/delivery-bindings/${binding.bindingId}?version=${binding.bindingVersion}`);
  state.deliverySubject = { name: details.config.serviceAccount, namespace: details.config.agentNamespace };
  save();
}

// Technical setup used by every earlier Fleet scenario: the Fleet agent needs
// a Role in the product-created application namespace.
async function namespaceAccess(ui) {
  if (state.namespaceAccessReady) return;
  await deliverySubject();
  const app = (await ui.get('/applications/' + state.app.key)).application;
  const namespace = app.environments.find((e) => e.key === ENV).namespaceIdentity;
  if (!kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'name')) return;
  const rt = { state, save, record: (k, v) => console.log(k, v) };
  const config = { runtime: { profile: 'dev-test', timeoutMs: 1_800_000, clusterKubeconfigFile: process.env.KUBECONFIG }, application: { environment: ENV }, platform: { account: { context: CONTEXT }, delivery: { namespaceAccess: { enabled: true, rulesFile: new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json', import.meta.url).pathname } } } };
  await ensureNamespaceAccess(ui, config, app, rt, { wait: false });
  state.namespace = namespace;
  state.namespaceAccessReady = true;
  save();
}

// Preview and one Deploy click; returns the report and the operation.
async function previewAndDeploy(ui, expectedWorkloads) {
  await ui.page.goto(`${ui.baseURL}/ui/applications/${state.app.key}?environment=${ENV}`);
  const preview = await ui.submit(base() + '/preview', 'POST', ui.page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  const changed = (preview.changes ?? []).map((c) => c.workloadId).sort();
  assert.deepEqual(changed, [...expectedWorkloads].sort(), 'Preview workloads');
  pass(`Preview lists ${changed.join(', ')}`);
  let count = 0;
  const watch = (r) => { if (r.method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + base() + '/deploy') count++; };
  ui.page.on('request', watch);
  const started = Date.now();
  const submission = ui.page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + base() + '/deploy', { timeout: 1_800_000 });
  await ui.page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  let response;
  for (;;) {
    response = await Promise.race([submission, ui.page.waitForTimeout(5000).then(() => undefined)]);
    if (response) break;
    await namespaceAccess(ui).catch((e) => console.log('namespace access pending:', e.message));
  }
  ui.page.off('request', watch);
  assert.equal(count, 1, 'exactly one Deploy request');
  const report = await response.json();
  const seconds = Math.round((Date.now() - started) / 1000);
  const result = ui.page.getByLabel('Kết quả triển khai');
  await result.waitFor({ timeout: 60_000 }).catch(() => {});
  await shot(ui, 'deploy-result');
  writeFileSync(join(dir, `${stage}-report.json`), JSON.stringify(report, null, 2));
  console.log('DEPLOY', JSON.stringify({ status: report.status, seconds, results: report.results?.map((r) => `${r.workloadId}:${r.status}${r.errorCode ? ':' + r.errorCode : ''}`) }));
  return { report, seconds, screen: await result.innerText().catch(() => '') };
}

// ---------- acceptance application checks (its own page) ----------
let forward;
async function appPage(ui) {
  if (!forward) {
    const child = spawn('kubectl', ['--context', CONTEXT, '-n', ns(), 'port-forward', 'svc/frontend', ':8080'], { stdio: ['ignore', 'pipe', 'pipe'] });
    const port = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error('port-forward timeout')), 30_000);
      child.stdout.on('data', (chunk) => { const m = String(chunk).match(/127\.0\.0\.1:(\d+)/); if (m) { clearTimeout(timer); resolve(m[1]); } });
      child.on('exit', () => { clearTimeout(timer); reject(new Error('port-forward stopped')); });
    });
    forward = { child, url: `http://127.0.0.1:${port}` };
  }
  const page = await ui.page.context().newPage();
  await page.goto(forward.url + '/');
  await page.getByRole('heading', { name: 'Acceptance application' }).waitFor();
  return page;
}
const CHECKS = ['backend connection', 'environment', 'secret', 'database', 'cache'];
async function checksPass(page) {
  const deadline = Date.now() + 300_000;
  for (;;) {
    await page.reload();
    const rows = await page.locator('#checks tr[data-check]').evaluateAll((trs) => trs.map((tr) => [tr.getAttribute('data-check'), tr.children[1].textContent.trim()]));
    if (CHECKS.every((n) => rows.some(([k, v]) => k === n && v === 'PASS'))) return rows;
    if (Date.now() > deadline) throw new Error('checks not PASS: ' + rows.map((r) => r.join('=')).join(', '));
    await page.waitForTimeout(3000);
  }
}
async function submitJob(page, payload) {
  await page.locator('input[name="payload"]').fill(payload);
  await page.getByRole('button', { name: 'Submit job' }).click();
  const deadline = Date.now() + 240_000;
  for (;;) {
    const text = await page.locator('#jobs tr.job').filter({ hasText: payload }).first().innerText().catch(() => '');
    if (text.includes('DONE')) return;
    if (Date.now() > deadline) throw new Error('job not DONE');
    await page.waitForTimeout(1500);
    await page.reload();
  }
}

// ---------- workloads authored through the form ----------
async function group(ui, title) {
  const page = ui.page;
  await page.getByLabel('Ảnh Container', { exact: true }).first().waitFor();
  while (await page.locator('button[data-workload-advanced][aria-expanded=false]').count()) await page.locator('button[data-workload-advanced][aria-expanded=false]').first().click();
  const g = page.locator('details.runtime-config-group').filter({ has: page.locator('summary strong').getByText(title, { exact: true }) });
  assert.equal(await g.count(), 1, 'Missing group ' + title);
  if (await g.getAttribute('open') === null) await g.locator('summary').click();
  return g;
}

async function chooseSource(ui, index, pattern) {
  const page = ui.page;
  const row = page.locator('tr[data-row]').nth(index);
  await row.getByRole('button', { name: new RegExp(`^(Chọn|Đổi) nguồn, dòng ${index + 1}$`) }).click();
  const option = page.getByRole('button', { name: pattern });
  if (await option.count() !== 1) {
    const names = await page.getByRole('button').allInnerTexts();
    throw new Error(`source ${pattern} not unique (${await option.count()}); buttons: ${names.filter((n) => n.length < 80).join(' | ')}`);
  }
  await option.click();
}

// spec: { template, name, image, port, deps: [{ alias, type, params } | { alias, reuse }], vars: { NAME: 'literal' | { key } | { ref } | { service } }, mount, schedule }
async function authorWorkload(ui, spec) {
  const page = ui.page;
  await page.goto(`${ui.baseURL}/ui/applications/${state.app.key}?environment=${ENV}`);
  await page.getByRole('button', { name: '+ Thêm Workload', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: new RegExp('^' + spec.template + '\\s+(Deployment|CronJob|StatefulSet)') }).click();
  await page.getByLabel('Tên Workload', { exact: true }).fill(spec.name);
  await page.getByLabel('Ảnh Container', { exact: true }).fill(spec.image);
  if (spec.port) {
    const network = await group(ui, 'Service & kết nối');
    await network.getByRole('button', { name: '+ Thêm cổng', exact: true }).click();
    await network.getByLabel('Tên cổng Service', { exact: true }).last().fill('http');
    await network.locator('input[type=number]').last().fill(String(spec.port));
  }
  if (spec.deps?.length) {
    const deps = await group(ui, 'Tài nguyên phụ thuộc');
    for (const dep of spec.deps) {
      await deps.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }).click();
      const row = deps.locator('.editor-grid').filter({ has: page.getByLabel('Alias tài nguyên', { exact: true }) }).last();
      if (dep.reuse) {
        const select = row.getByRole('combobox', { name: 'Nguồn tài nguyên', exact: true });
        const label = (await select.locator('option').allTextContents()).find((t) => dep.reuse.test(t));
        if (!label) throw new Error(`no shared resource ${dep.reuse}: ${(await select.locator('option').allTextContents()).join(' | ')}`);
        await select.selectOption({ label });
        await row.getByLabel('Alias tài nguyên', { exact: true }).fill(dep.alias);
        continue;
      }
      await row.getByLabel('Alias tài nguyên', { exact: true }).fill(dep.alias);
      await row.getByRole('combobox', { name: 'Loại tài nguyên', exact: true }).selectOption(dep.type);
      for (const [k, v] of Object.entries(dep.params ?? {})) await row.getByLabel('Tham số ' + k, { exact: true }).fill(v);
      // The shared resource class of the acceptance app; explicit when several classes exist.
      const klass = row.getByLabel(`Cách cung cấp (class) của ${dep.alias}`, { exact: true });
      if (await klass.inputValue() !== 'in-cluster') await klass.selectOption('in-cluster');
    }
  }
  if (spec.mount) {
    const volumes = await group(ui, 'Volumes & files');
    await volumes.getByRole('button', { name: 'Thêm volume hoặc file', exact: true }).click();
    const add = page.getByRole('dialog', { name: 'Thêm volume hoặc file', exact: true });
    await add.getByRole('button', { name: 'Tạo nguồn mới', exact: true }).click();
    await add.getByRole('combobox', { name: 'Loại', exact: true }).selectOption({ label: 'Lưu trữ bền vững (PVC)' });
    await add.getByRole('button', { name: 'Tạo hoặc chọn lưu trữ', exact: true }).click();
    const storage = page.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ', exact: true });
    const storageType = storage.getByRole('combobox', { name: 'Loại tài nguyên', exact: true });
    if (await storageType.inputValue() !== 'persistent-storage') await storageType.selectOption('persistent-storage');
    await storage.getByLabel('Tham số storage', { exact: true }).fill('1Gi');
    await storage.getByRole('button', { name: 'Thêm và chọn', exact: true }).click();
    await storage.waitFor({ state: 'detached' });
  }
  if (spec.schedule) {
    await page.getByLabel('Lịch cron', { exact: true }).fill(spec.schedule);
    await page.getByLabel('Múi giờ', { exact: true }).fill('UTC');
  }
  let i = 0;
  for (const [name, value] of Object.entries(spec.vars)) {
    await page.getByRole('button', { name: '+ Thêm biến', exact: true }).first().click();
    const row = page.locator('tr[data-row]').nth(i);
    await row.locator('td input').first().fill(name);
    if (typeof value === 'string') await row.getByRole('textbox').nth(1).fill(value);
    else if (value.key) await chooseSource(ui, i, new RegExp('/ ' + value.key + '(\\s*Bí mật)?$'));
    else if (value.ref) await chooseSource(ui, i, new RegExp('^' + value.ref.replace('.', '\\.')));
    else if (value.service) await chooseSource(ui, i, value.service);
    i++;
  }
  await shot(ui, `author-${spec.name}`);
  await ui.submit(base() + '/workloads/' + spec.name, 'PUT', page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}

function workloadSpecs() {
  const pg = { PGDATABASE: { ref: 'db.database' }, PGHOST: { ref: 'db.host' }, PGPASSWORD: { ref: 'db.password' }, PGPORT: { ref: 'db.port' }, PGUSER: { ref: 'db.username' } };
  const redis = { REDIS_HOST: { ref: 'cache.host' }, REDIS_PASSWORD: { ref: 'cache.password' }, REDIS_PORT: { ref: 'cache.port' }, REDIS_TLS: { ref: 'cache.tls' }, REDIS_USERNAME: { ref: 'cache.username' } };
  const scope = { ACCEPTANCE_CACHE_APP: { key: 'ACCEPTANCE_CACHE_APP' }, ACCEPTANCE_CACHE_ENV: { key: 'ACCEPTANCE_CACHE_ENV' }, ACCEPTANCE_CACHE_RUN: { key: 'ACCEPTANCE_CACHE_RUN' }, ACCEPTANCE_CACHE_TTL_SECONDS: { key: 'ACCEPTANCE_CACHE_TTL_SECONDS' } };
  const shared = (re) => [{ alias: 'db', reuse: /postgres/ }, { alias: 'cache', reuse: /redis/ }].filter((d) => re.test(d.alias));
  return [
    { template: 'Backend API có migrate', name: 'backend', image: `acceptance-backend:${TAG}`, port: 8080, mount: true,
      deps: [{ alias: 'db', type: 'postgres', params: { database: 'acceptance', username: 'app' } }, { alias: 'cache', type: 'redis' }],
      vars: { ...pg, ...redis, ...scope, ACCEPTANCE_CONFIG: { key: 'ACCEPTANCE_CONFIG' }, ACCEPTANCE_SECRET: { key: 'ACCEPTANCE_SECRET' }, ACCEPTANCE_SECRET_SHA256: { key: 'ACCEPTANCE_SECRET_SHA256' }, ACCEPTANCE_SCHEDULE_RUN: { key: 'ACCEPTANCE_SCHEDULE_RUN' }, ACCEPTANCE_STORAGE_DIR: '/data', PORT: { key: 'PORT' } } },
    { template: 'Xử lý nền', name: 'worker', image: `acceptance-worker:${TAG}`, deps: shared(/db|cache/),
      vars: { ...pg, ...redis, ...scope, ACCEPTANCE_SECRET: { key: 'ACCEPTANCE_SECRET' }, POLL_INTERVAL_MS: { key: 'POLL_INTERVAL_MS' } } },
    { template: 'Frontend web', name: 'frontend', image: `acceptance-frontend:${TAG}`, port: 8080,
      vars: { BACKEND_URL: { service: /backend/ }, PORT: { key: 'PORT' } } },
    { template: 'Tác vụ theo lịch', name: 'scheduled', image: `acceptance-scheduled:${TAG}`, schedule: '* * * * *', deps: shared(/db/),
      vars: { ...pg, BACKEND_URL: { service: /backend/ }, ACCEPTANCE_SCHEDULE_RUN: { key: 'ACCEPTANCE_SCHEDULE_RUN' } } },
  ];
}

// ---------- stages ----------
async function users() {
  const ui = await asAdmin();
  await createDeveloper(ui);
  pass('Admin prepared the developer account in Người dùng');
}

async function prepare() {
  const dev = await asDeveloper();
  if (!state.app) {
    const name = process.env.ORCH_APP_NAME || `bundle-${Date.now().toString(36)}`;
    assert(!(await dev.get('/applications')).applications.some((a) => a.name === name), 'same-name Application exists');
    state.app = await createApplication(dev, name);
    state.appName = name;
    save();
    pass(`Developer created Application ${name}`);
  }
  // ADR-042: the developer selects the first store, then cannot change it.
  const store = (await dev.get('/secret-store-choices')).secretStores.find((s) => s.name === STORE);
  assert(store, 'secret store ' + STORE);
  const appView = (await dev.get('/applications/' + state.app.key)).application;
  if (!appView.environments.find((e) => e.key === ENV).secretStoreKey) await selectStore(dev, store.key);
  await openSettings(dev);
  if (!developerIsPlatform()) {
    await dev.page.getByText('Kho bí mật đã chọn thì chỉ Kỹ sư nền tảng hoặc Quản trị viên đổi được.').waitFor();
    assert.equal(await dev.page.getByLabel('Kho bí mật cho Staging', { exact: true }).count(), 0, 'developer must not change a selected store');
  }
  await shot(dev, 'developer-store');
  pass(`${developerConfig().username} selected ${STORE} for staging${developerIsPlatform() ? '' : ' and then sees it read-only'}`);

  const secret = randomBytes(24).toString('hex');
  state.secretHashes = [createHash('sha256').update(secret).digest('hex')];
  save();
  await openSettings(dev);
  const variables = { ACCEPTANCE_CACHE_APP: state.app.key.slice(0, 63), ACCEPTANCE_CACHE_ENV: ENV, ACCEPTANCE_CACHE_RUN: state.appName, ACCEPTANCE_CACHE_TTL_SECONDS: '30', ACCEPTANCE_CONFIG: 'acceptance-config-ok', ACCEPTANCE_SECRET_SHA256: state.secretHashes[0], ACCEPTANCE_SCHEDULE_RUN: state.appName, POLL_INTERVAL_MS: '500', PORT: '8080' };
  for (const [name, value] of Object.entries(variables)) await saveKey(dev, 'VARIABLE', name, value);
  await saveKey(dev, 'SECRET', 'ACCEPTANCE_SECRET', secret);
  assert(!(await dev.page.locator('body').innerText()).includes(secret), 'secret value shown');
  pass('Developer saved 9 variables and 1 secret in Settings');

  const existing = (await dev.get(base() + '/workloads')).workloads.map((w) => w.id);
  for (const spec of workloadSpecs()) {
    if (existing.includes(spec.name) && !process.env.ORCH_REAUTHOR) { step(`draft ${spec.name} exists`); continue; }
    await authorWorkload(dev, spec);
    pass(`Developer authored ${spec.name} (${spec.template}) in the form`);
  }
}

async function deploy() {
  const dev = await asDeveloper();
  const { report } = await previewAndDeploy(dev, ['backend', 'frontend', 'scheduled', 'worker']);
  assert.equal(report.status, 'SUCCEEDED');
  pass('One Deploy of four workloads SUCCEEDED');
  await verifyDeploy(dev);
}

async function verifyDeploy(dev) {
  dev ??= await asDeveloper();
  await namespaceAccess(dev);
  // Bundles: one app and one migrate unit for the Environment.
  const { repos, bundles } = fleetBundles();
  assert.equal(repos.length, 1);
  const paths = repos[0].spec.paths.slice().sort();
  assert.deepEqual(paths.map((p) => p.split('/').slice(-1)[0]), ['app', 'migrate']);
  pass(`Fleet GitRepo ${repos[0].metadata.name} watches ${paths.join(', ')}; ${bundles.length} bundles`);
  // Migration Job completed before the app revision created the backend.
  const jobs = items('jobs').filter((j) => j.metadata.name.startsWith('backend-migrate-'));
  assert.equal(jobs.length, 1, 'one migration Job');
  const job = jobs[0];
  assert(job.status.conditions?.some((c) => c.type === 'Complete' && c.status === 'True'), 'Job Complete');
  const backend = kjson('-n', ns(), 'get', 'deployment', 'backend');
  assert(new Date(job.status.completionTime) <= new Date(backend.metadata.creationTimestamp), 'Job completed before the app revision');
  state.job = { name: job.metadata.name, uid: job.metadata.uid };
  pass(`Job ${job.metadata.name} Complete at ${job.status.completionTime}, backend created ${backend.metadata.creationTimestamp}`);
  // Secrets: one version per workload with UC-12 or resource secrets.
  const snap = snapshot();
  state.snapshots = { deploy: snap };
  save();
  for (const w of ['backend', 'worker', 'scheduled']) {
    assert(snap.vss[`${w}-config-v1`] && snap.secrets[`${w}-config-v1`], `${w}-config-v1 VSS and Secret`);
    assert.equal(vaultVersion(w), 1, `${w} Vault version 1`);
  }
  assert.deepEqual(deploymentSecret('backend'), ['backend-config-v1']);
  pass('backend/worker/scheduled read Secret <workload>-config-v1 pinned to Vault version 1');
  // The acceptance application itself.
  const app = await appPage(dev);
  const rows = await checksPass(app);
  pass('App checks: ' + rows.map((r) => r.join(' ')).join(', '));
  await submitJob(app, `job ${state.appName} 1`);
  pass('Job submitted on the frontend was processed by the worker (DONE)');
  const value = `pvc-${state.appName}-${Date.now()}`;
  await app.getByLabel('Test value', { exact: true }).fill(value);
  await app.getByRole('button', { name: 'Write to volume', exact: true }).click();
  await app.locator('#storage-value').filter({ hasText: value }).waitFor();
  state.storage = { value, hash: createHash('sha256').update(value).digest('hex') };
  save();
  pass('Value written to the backend PVC through the app page');
  // Pod recreation keeps the PVC value.
  const pod = podSecrets('backend')[0];
  execFileSync('kubectl', ['--context', CONTEXT, 'delete', '--raw', `/api/v1/namespaces/${ns()}/pods/${pod.name}`, '-f', '-'], { input: JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: pod.uid } }), encoding: 'utf8' });
  kubectl('-n', ns(), 'rollout', 'status', 'deployment/backend', '--timeout=180s');
  const deadline = Date.now() + 180_000;
  for (;;) {
    await app.reload();
    const text = await app.locator('#storage-value').textContent().catch(() => '');
    const process = await app.locator('#storage-process').textContent().catch(() => pod.name);
    if (text === value && process !== pod.name) break;
    assert(Date.now() < deadline, 'value retained after backend Pod recreation');
    await app.waitForTimeout(2000);
  }
  pass('PVC value retained after backend Pod recreation');
  const receiptsDeadline = Date.now() + 240_000;
  for (;;) {
    await app.reload();
    const receipts = await app.locator('#scheduled-runs tr, #schedule-receipts tr, [data-receipt]').allInnerTexts().catch(() => []);
    if (receipts.some((r) => r.includes(state.storage.hash.slice(0, 12)))) break;
    assert(Date.now() < receiptsDeadline, 'scheduled receipt with the stored hash');
    await app.waitForTimeout(5000);
  }
  const cronJobs = items('jobs').filter((j) => j.metadata.ownerReferences?.some((o) => o.kind === 'CronJob' && o.name === 'scheduled') && j.status.conditions?.some((c) => c.type === 'Complete' && c.status === 'True'));
  assert(cronJobs.length > 0, 'a scheduled Job Complete');
  pass(`scheduled CronJob ran Complete (${cronJobs.length} Jobs) and wrote a receipt with the PVC hash`);
  await app.screenshot({ path: join(dir, 'deploy-app.png'), fullPage: true });
}

async function image() {
  const dev = await asDeveloper();
  const before = snapshot();
  const versions = { backend: vaultVersion('backend'), worker: vaultVersion('worker') };
  await editImage(dev, 'backend', `acceptance-backend:${TAG2}`);
  const { report } = await previewAndDeploy(dev, ['backend']);
  assert.equal(report.status, 'SUCCEEDED');
  const after = snapshot();
  assert.deepEqual(after.vss, before.vss, 'no new VaultStaticSecret');
  assert.deepEqual(after.secrets, before.secrets, 'no new Secret');
  assert.deepEqual(after.accounts, before.accounts, 'no new ServiceAccount');
  assert.deepEqual(after.auths, before.auths, 'no new VaultAuth');
  assert.equal(vaultVersion('backend'), versions.backend);
  const pods = podSecrets('backend');
  assert(pods.every((p) => p.secrets.includes('backend-config-v1')), 'backend still on v1');
  assert.equal(kjson('-n', ns(), 'get', 'deployment', 'backend').spec.template.spec.containers[0].image, `acceptance-backend:${TAG2}`);
  state.snapshots.image = after;
  save();
  pass('Image-only change: same VSS/Secret/ServiceAccount/VaultAuth UIDs, same Vault version, backend on new image');
  const app = await appPage(dev);
  await checksPass(app);
  pass('App checks PASS after image change');
}

async function changeSecret(dev) {
  const secret = randomBytes(24).toString('hex');
  const hash = createHash('sha256').update(secret).digest('hex');
  await openSettings(dev);
  await saveKey(dev, 'SECRET', 'ACCEPTANCE_SECRET', secret);
  await saveKey(dev, 'VARIABLE', 'ACCEPTANCE_SECRET_SHA256', hash);
  state.secretHashes.push(hash);
  save();
}

async function secret() {
  const dev = await asDeveloper();
  const before = { backend: vaultVersion('backend'), worker: vaultVersion('worker') };
  await changeSecret(dev);
  const { report } = await previewAndDeploy(dev, ['backend', 'worker']);
  assert.equal(report.status, 'SUCCEEDED');
  for (const w of ['backend', 'worker']) {
    assert.equal(vaultVersion(w), before[w] + 1, `${w} new Vault version`);
    assert.deepEqual(deploymentSecret(w), [`${w}-config-v2`]);
    assert(podSecrets(w).every((p) => p.secrets.includes(`${w}-config-v2`)), `${w} pods on v2`);
  }
  const snap = snapshot();
  assert(snap.secrets['backend-config-v2'] && snap.secrets['worker-config-v2']);
  assert.deepEqual(snap.retained, ['backend-config-v1', 'worker-config-v1'], 'previous successful versions retained');
  state.snapshots.secret = snap;
  save();
  pass('Secret change: Vault version 2, Secrets <workload>-config-v2, pods rolled to v2, v1 kept as retained');
  const app = await appPage(dev);
  await checksPass(app);
  pass('App checks PASS with the new secret');
}

async function fail() {
  const dev = await asDeveloper();
  const frontendBefore = { uid: kjson('-n', ns(), 'get', 'deployment', 'frontend').metadata.uid, pods: podSecrets('frontend').map((p) => p.uid).sort() };
  const oldWorker = podSecrets('worker');
  const backendVersion = vaultVersion('backend');
  await changeSecret(dev);
  await editImage(dev, 'worker', `acceptance-worker:issue67-missing`);
  const { report, screen } = await previewAndDeploy(dev, ['backend', 'worker']);
  assert.notEqual(report.status, 'SUCCEEDED');
  const byWorkload = Object.fromEntries(report.results.map((r) => [r.workloadId, r]));
  assert.equal(byWorkload.worker.status, 'FAILED');
  assert(['ROLLOUT_NOT_READY', 'ROLLOUT_DEGRADED'].includes(byWorkload.worker.errorCode), 'worker rollout failure code');
  assert.equal(byWorkload.backend.status, 'SUCCEEDED');
  assert(screen.includes('worker'), 'Console result names worker');
  pass(`Deploy FAILED; Console names worker (${byWorkload.worker.errorCode}); backend SUCCEEDED`);
  const appView = (await dev.get('/applications/' + state.app.key)).application.environments.find((e) => e.key === ENV);
  assert(!appView.activeOperation, 'Environment released');
  // Old worker Pods keep the last successful Secret and survive a restart.
  const running = podSecrets('worker').filter((p) => p.secrets.includes('worker-config-v2'));
  assert(running.length >= 1 && running.every((p) => oldWorker.some((o) => o.uid === p.uid)), 'old worker Pod still running on v2');
  assert(snapshot().secrets['worker-config-v2'], 'Secret v2 kept');
  const victim = running[0];
  execFileSync('kubectl', ['--context', CONTEXT, 'delete', '--raw', `/api/v1/namespaces/${ns()}/pods/${victim.name}`, '-f', '-'], { input: JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: victim.uid } }), encoding: 'utf8' });
  const deadline = Date.now() + 180_000;
  let replacement;
  for (;;) {
    replacement = podSecrets('worker').find((p) => p.uid !== victim.uid && p.ready && p.secrets.includes('worker-config-v2'));
    if (replacement) break;
    assert(Date.now() < deadline, 'restarted old worker Pod ready on v2');
    await new Promise((r) => setTimeout(r, 3000));
  }
  pass(`Old worker Pod restarted (${replacement.name}) and runs with worker-config-v2`);
  const failedVersion = vaultVersion('backend');
  assert.equal(failedVersion, backendVersion + 1);
  state.failedVersion = failedVersion;
  save();
  assert(podSecrets('backend').every((p) => p.secrets.includes(`backend-config-v${failedVersion}`)), 'backend on the new version');
  const frontendAfter = { uid: kjson('-n', ns(), 'get', 'deployment', 'frontend').metadata.uid, pods: podSecrets('frontend').map((p) => p.uid).sort() };
  assert.deepEqual(frontendAfter, frontendBefore, 'frontend not restarted');
  pass('Unchanged frontend kept its Deployment and Pods');
  const { bundles } = fleetBundles();
  const app = bundles.find((b) => (b.spec?.helm?.releaseName ?? '') && b.metadata.name.endsWith('-app'));
  state.failedBundleReady = app?.status?.summary?.ready;
  save();
  const page = await appPage(dev);
  await submitJob(page, `job ${state.appName} during failure`);
  pass('Application still serves jobs while the Environment bundle is not Ready');
}

async function rollback() {
  const dev = await asDeveloper();
  await dev.page.goto(`${dev.baseURL}/ui/applications/${state.app.key}?environment=${ENV}`);
  await dev.page.getByRole('button', { name: 'Xem trước Rollback', exact: true }).click();
  const dialog = dev.page.getByRole('dialog', { name: 'Xác nhận Rollback môi trường', exact: true });
  await dialog.waitFor();
  await shot(dev, 'preview');
  await dialog.getByRole('textbox').fill(state.app.key);
  const started = Date.now();
  const [reply] = await Promise.all([
    dev.page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + base() + '/rollback', { timeout: 1_800_000 }),
    dialog.getByRole('button', { name: 'Rollback môi trường', exact: true }).click(),
  ]);
  const report = await reply.json();
  writeFileSync(join(dir, 'rollback-report.json'), JSON.stringify(report, null, 2));
  assert.equal(report.status, 'SUCCEEDED');
  await dev.page.getByText('Đã khôi phục toàn bộ môi trường.').waitFor();
  await shot(dev, 'done');
  pass(`Rollback SUCCEEDED in ${Math.round((Date.now() - started) / 1000)} s`);
  await verifyRollback(dev);
}

async function verifyRollback(dev) {
  dev ??= await asDeveloper();
  for (const w of ['backend', 'worker']) {
    assert.deepEqual(deploymentSecret(w), [`${w}-config-v2`], `${w} back on v2`);
    assert(podSecrets(w).every((p) => p.secrets.includes(`${w}-config-v2`)), `${w} pods on v2`);
  }
  assert.equal(vaultVersion('backend'), state.failedVersion, 'rollback reused Vault version 2 without a new write');
  // Not used by any successful release any more: removed and garbage collected.
  const deadline = Date.now() + 120_000;
  for (;;) {
    const snap = snapshot();
    const unused = Object.keys({ ...snap.vss, ...snap.secrets }).filter((n) => /^(backend|worker)-config-v(\d+)$/.test(n) && !/-v2$/.test(n));
    if (!unused.length) break;
    assert(Date.now() < deadline, 'v3 VSS/Secret removed after rollback');
    await new Promise((r) => setTimeout(r, 3000));
  }
  pass('Rollback restored <workload>-config-v2 (Vault version reused); every other backend/worker VSS and Secret was deleted and garbage collected');
  const app = await appPage(dev);
  await checksPass(app);
  pass('App checks PASS after rollback (secret hash of the restored release)');
}

async function resync() {
  const { repos } = fleetBundles();
  const repo = repos[0];
  const jobsBefore = items('jobs').filter((j) => j.metadata.name.startsWith('backend-migrate-')).map((j) => j.metadata.uid).sort();
  const generation = (repo.spec.forceSyncGeneration ?? 0) + 1;
  kubectl('-n', 'fleet-local', 'patch', 'gitrepos.fleet.cattle.io', repo.metadata.name, '--type=merge', '-p', JSON.stringify({ spec: { forceSyncGeneration: generation } }));
  step(`forced Fleet resync of ${repo.metadata.name} (generation ${generation}); waiting 90 s`);
  await new Promise((r) => setTimeout(r, 90_000));
  const jobsAfter = items('jobs').filter((j) => j.metadata.name.startsWith('backend-migrate-')).map((j) => j.metadata.uid).sort();
  assert.deepEqual(jobsAfter, jobsBefore, 'no migration Job re-run');
  pass(`Forced Fleet resync re-ran no migration Job (${jobsAfter.length} Job UIDs unchanged)`);
}

// Settles an interrupted Environment operation through Settings (explicit
// operator confirmation that the earlier execution stopped).
async function recover() {
  const dev = await asDeveloper();
  const env = (await dev.get('/applications/' + state.app.key)).application.environments.find((e) => e.key === ENV);
  assert(env.activeOperation, 'no held operation');
  await openSettings(dev);
  await dev.page.getByLabel('Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn').check();
  const result = await dev.submit(`${base()}/operations/${env.activeOperation.id}/recover`, 'POST', dev.page.getByRole('button', { name: 'Khôi phục môi trường', exact: true }));
  writeFileSync(join(dir, 'recover-result-body.json'), JSON.stringify(result, null, 2));
  await shot(dev, 'done');
  const after = (await dev.get('/applications/' + state.app.key)).application.environments.find((e) => e.key === ENV);
  console.log('RECOVER', JSON.stringify({ outcome: result.recovery?.outcome, active: after.activeOperation?.status ?? null }));
  assert(!after.activeOperation, 'Environment released');
  pass('Recovered the interrupted operation from Settings; Environment released');
}

async function cleanup() {
  const dev = await asDeveloper();
  const app = (await dev.get('/applications/' + state.app.key)).application;
  const namespace = app.environments.find((e) => e.key === ENV).namespaceIdentity;
  const { repos } = fleetBundles();
  // Deletion requires the applied schedule to be suspended (UC-18).
  const scheduled = (await dev.get(base() + '/workloads')).workloads.find((w) => w.id === 'scheduled');
  if (scheduled && !scheduled.workload.workloadExecution?.schedule?.suspend) {
    await dev.page.goto(`${dev.baseURL}/ui/applications/${state.app.key}/environments/${ENV}/workloads/scheduled`);
    await dev.page.locator('summary').getByText('Chính sách Job', { exact: true }).click();
    await dev.page.getByLabel('Tạm dừng lịch', { exact: true }).check();
    await dev.submit(base() + '/workloads/scheduled', 'PUT', dev.page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
    const { report } = await previewAndDeploy(dev, ['scheduled']);
    assert.equal(report.status, 'SUCCEEDED');
    pass('Suspended the scheduled workload through Preview and Deploy');
  }
  if (kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'name')) kubectl('-n', namespace, 'delete', 'role,rolebinding', '-l', 'bootstrap.orchestrator.dev/application=' + state.app.key, '--ignore-not-found');
  await dev.page.goto(`${dev.baseURL}/ui/applications/${state.app.key}/settings`);
  if (app.status === 'DELETING') await dev.page.getByRole('region', { name: 'Tiến trình xóa ứng dụng', exact: true }).getByRole('button', { name: 'Thử lại', exact: true }).click();
  else await dev.page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = dev.page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  await dialog.getByRole('textbox').fill(app.name);
  const deletion = await dev.submit(`/applications/${state.app.key}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
  const id = deletion.deletion.operationId;
  const deadline = Date.now() + 1_800_000;
  let progress;
  for (;;) {
    progress = (await dev.get(`/applications/${state.app.key}/deletions/${id}`)).deletion;
    if (!progress.running) break;
    assert(Date.now() < deadline, 'deletion still running');
    await dev.page.waitForTimeout(5000);
  }
  writeFileSync(join(dir, 'deletion.json'), JSON.stringify(progress, null, 2));
  assert.equal(progress.status, 'SUCCEEDED', progress.failureCode);
  assert.equal(kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'name'), '');
  for (const w of ['backend', 'worker', 'scheduled', 'frontend']) assert.equal(vaultVersion(w), 0, `${w} Vault path removed`);
  for (const repo of repos) assert.equal(kubectl('-n', 'fleet-local', 'get', 'gitrepos.fleet.cattle.io', repo.metadata.name, '--ignore-not-found', '-o', 'name'), '', 'GitRepo removed');
  pass(`Deleted through the Console: namespace ${namespace}, Fleet GitRepo and workload Vault paths are gone`);
}

let status = 'FAIL';
try {
  if (stage !== 'users') assert(stage === 'prepare' || state.app, 'run prepare first');
  await ({ users, prepare, deploy, 'verify-deploy': () => verifyDeploy(), image, secret, fail, rollback, resync, cleanup, recover, 'verify-rollback': () => verifyRollback() })[stage]();
  status = 'PASS';
} catch (error) {
  console.error(`FAIL ${stage}: ${String(error?.message ?? error).split('\n')[0]}`);
  for (const s of [devSession, adminSession]) if (s) await s.page.screenshot({ path: join(dir, `${stage}-error-${s === devSession ? 'developer' : 'admin'}.png`), fullPage: true }).catch(() => {});
  process.exitCode = 1;
} finally {
  writeFileSync(join(dir, `${stage}-result.json`), JSON.stringify({ stage, status, assertions, at: new Date().toISOString() }, null, 2));
  forward?.child.kill();
  if (devSession) await signOutContext(devSession.context, developerConfig()).catch(() => {});
  if (adminSession) await closeConsole(adminSession, admin).catch(() => {});
}
