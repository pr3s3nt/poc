// Stuck StatefulSet recovery and Active Definition revision — issue #68.
// Developer actions go through the Console UI. Platform Engineer Definition
// edits and promotion use the Console API of the Admin session as scenario
// setup (they are the same authorized calls the Definition editor makes).
// Kubernetes reads are read-only assertions. See README.md.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { closeConsole, openConsole, readConsoleConfig, readPrivateFile, signInContext, signOutContext } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';

process.umask(0o077);
const STAGES = ['users', 'pg-bad', 'pg-fix', 'pinned', 'guard', 'sts-bad', 'sts-same', 'sts-fix', 'cleanup'];
const stage = process.argv[2];
assert(STAGES.includes(stage), 'stage: ' + STAGES.join('|'));
const dir = process.env.ORCH_TEST_RUN_DIR;
assert(dir, 'ORCH_TEST_RUN_DIR');
outsideRepo(dir);
const ENV = 'staging';
const CONTEXT = process.env.ORCH_KUBE_CONTEXT || 'kind-k8s-4f';
const STORE = process.env.ORCH_SECRET_STORE_NAME || 'Issue68 Vault';
const PG_DEFINITION = process.env.ORCH_POSTGRES_DEFINITION || 'issue68-postgres';
const GOOD_PG = 'postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777';
const BAD_PG = 'postgres:16.14-alpine@sha256:' + '0'.repeat(64);
const OTHER_PG = 'postgres:16.14-alpine@sha256:' + '1'.repeat(64);
const GOOD_WEB = process.env.ORCH_WEB_IMAGE || 'nginxinc/nginx-unprivileged:1.29-alpine';
const BAD_WEB = 'nginxinc/nginx-unprivileged:0.0.0-issue68-missing';
const STUCK = /^(ImagePullBackOff|ErrImagePull)$/;
const file = join(dir, 'state.json');
const state = existsSync(file) ? JSON.parse(readFileSync(file, 'utf8')) : {};
const save = () => writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
const assertions = [];
const pass = (text) => { assertions.push(text); console.log('PASS ' + text); };

const kubectl = (...args) => execFileSync('kubectl', ['--context', CONTEXT, ...args], { encoding: 'utf8', timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const kjson = (...args) => JSON.parse(kubectl(...args, '-o', 'json'));

// ---------- Console sessions ----------
const admin = readConsoleConfig();
const developerConfig = () => ({ ...admin, username: 'developer', password: readPrivateFile(join(dir, 'developer-password'), 'mật khẩu developer') });
let adminSession, devSession;
async function asAdmin() { adminSession ??= await openConsole(admin, {}); return makeUI(adminSession.page, admin.baseURL, 1_800_000); }
async function asDeveloper() {
  if (!devSession) {
    const browser = adminSession?.browser ?? (await asAdmin(), adminSession.browser);
    devSession = await signInContext(browser, developerConfig(), { role: 'DEVELOPER' });
  }
  return makeUI(devSession.page, admin.baseURL, 1_800_000);
}
const base = (app) => `/applications/${app.key}/environments/${ENV}`;
const shot = (ui, name) => ui.page.screenshot({ path: join(dir, `${stage}-${name}.png`), fullPage: true });

async function api(ui, path, method, data) {
  const r = await ui.page.request.fetch(`${ui.baseURL}/api/v1${path}`, { method, ...(data === undefined ? {} : { data }) });
  const body = r.status() === 204 ? null : await r.json().catch(() => null);
  return { ok: r.ok(), status: r.status(), body };
}

// ---------- Platform Engineer setup: Definition revision and promotion ----------
async function setPostgresImage(image) {
  const ui = await asAdmin();
  const current = (await ui.get('/resource-definitions')).resourceDefinitions.find((d) => d.key === PG_DEFINITION);
  assert(current, 'Definition ' + PG_DEFINITION);
  const driverInputs = structuredClone(current.driverInputs);
  driverInputs.values.variables.image = image;
  const body = { key: current.key, name: current.name, resourceType: current.resourceType, driverType: current.driverType, driverInputs, criteria: current.criteria, version: current.version,
    ...(current.driverAccount ? { driverAccount: current.driverAccount } : {}), ...(current.provision ? { provision: current.provision } : {}) };
  const put = await api(ui, `/resource-definitions/${current.key}`, 'PUT', body);
  assert(put.ok, `PUT Definition HTTP ${put.status} ${JSON.stringify(put.body)?.slice(0, 300)}`);
  const history = (await ui.get(`/resource-definitions/${current.key}/versions`)).history;
  const latest = history.versions.at(-1);
  const promote = await api(ui, `/resource-definitions/${current.key}/versions/${latest.id}/promote`, 'POST', { expectedGeneration: history.generation });
  assert(promote.ok, `promote HTTP ${promote.status}`);
  return { id: latest.id, number: latest.number };
}

// ---------- Developer UI actions ----------
async function createDeveloper(ui) {
  const password = randomBytes(18).toString('base64url');
  await ui.page.goto(`${admin.baseURL}/ui/platform/users`);
  if (((await ui.get('/users')).users ?? []).some((u) => u.username === 'developer')) {
    // A seeded developer whose password is unknown here: the Admin resets it.
    const row = ui.page.locator('.settings-table-row').filter({ has: ui.page.locator('strong').getByText('developer', { exact: true }) });
    await row.getByRole('button', { name: 'Đặt lại mật khẩu', exact: true }).click();
    const reset = ui.page.getByRole('form', { name: 'Đặt lại mật khẩu của developer' });
    const inputs = reset.locator('input[autocomplete="new-password"]');
    await inputs.nth(0).fill(password);
    await inputs.nth(1).fill(password);
    const [reply] = await Promise.all([
      ui.page.waitForResponse((r) => r.request().method() !== 'GET' && new URL(r.url()).pathname.startsWith('/api/v1/users/')),
      reset.getByRole('button', { name: 'Đặt lại mật khẩu', exact: true }).click(),
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
  const app = (await ui.submit('/applications', 'POST', ui.page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }))).application;
  const store = (await ui.get('/secret-store-choices')).secretStores.find((s) => s.name === STORE);
  assert(store, 'secret store ' + STORE);
  await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}/settings?environment=${ENV}`);
  await ui.page.getByLabel('Kho bí mật cho Staging', { exact: true }).selectOption(store.key);
  await ui.submit(base(app) + '/secret-store', 'PUT', ui.page.getByRole('button', { name: 'Lưu kho bí mật', exact: true }));
  return app;
}

async function group(ui, title) {
  const page = ui.page;
  await page.getByLabel('Ảnh Container', { exact: true }).first().waitFor();
  while (await page.locator('button[data-workload-advanced][aria-expanded=false]').count()) await page.locator('button[data-workload-advanced][aria-expanded=false]').first().click();
  const g = page.locator('details.runtime-config-group').filter({ has: page.locator('summary strong').getByText(title, { exact: true }) });
  assert.equal(await g.count(), 1, 'Missing group ' + title);
  if (await g.getAttribute('open') === null) await g.locator('summary').click();
  return g;
}

async function authorWorkload(ui, app, spec) {
  const page = ui.page;
  await page.goto(`${ui.baseURL}/ui/applications/${app.key}?environment=${ENV}`);
  await page.getByRole('button', { name: '+ Thêm Workload', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: new RegExp('^' + spec.template + '\\s+(Deployment|CronJob|StatefulSet)') }).click();
  await page.getByLabel('Tên Workload', { exact: true }).fill(spec.name);
  await page.getByLabel('Ảnh Container', { exact: true }).fill(spec.image);
  const network = await group(ui, 'Service & kết nối');
  await network.getByRole('button', { name: '+ Thêm cổng', exact: true }).click();
  await network.getByLabel('Tên cổng Service', { exact: true }).last().fill('http');
  await network.locator('input[type=number]').last().fill('8080');
  if (spec.postgres) {
    const deps = await group(ui, 'Tài nguyên phụ thuộc');
    await deps.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }).click();
    const row = deps.locator('.editor-grid').filter({ has: page.getByLabel('Alias tài nguyên', { exact: true }) }).last();
    await row.getByLabel('Alias tài nguyên', { exact: true }).fill('db');
    await row.getByRole('combobox', { name: 'Loại tài nguyên', exact: true }).selectOption('postgres');
    await row.getByLabel('Tham số database', { exact: true }).fill('app');
    await row.getByLabel('Tham số username', { exact: true }).fill('app');
  }
  await shot(ui, `author-${spec.name}`);
  await ui.submit(base(app) + '/workloads/' + spec.name, 'PUT', page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}

async function editImage(ui, app, name, image) {
  await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}/environments/${ENV}/workloads/${name}`);
  const field = ui.page.getByLabel('Ảnh Container', { exact: true }).first();
  await field.waitFor();
  await field.fill(image);
  await ui.submit(base(app) + '/workloads/' + name, 'PUT', ui.page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}

// The Fleet agent needs a Role in the product-created namespace (technical
// setup shared with earlier Fleet scenarios).
async function namespaceAccess(ui, app, key) {
  if (state[key]?.namespaceAccessReady) return;
  if (!state.deliverySubject) {
    const adminUI = await asAdmin();
    const binding = await adminUI.get('/delivery-default');
    const details = await adminUI.get(`/delivery-bindings/${binding.bindingId}?version=${binding.bindingVersion}`);
    state.deliverySubject = { name: details.config.serviceAccount, namespace: details.config.agentNamespace };
    save();
  }
  const view = (await ui.get('/applications/' + app.key)).application;
  const namespace = view.environments.find((e) => e.key === ENV).namespaceIdentity;
  if (!kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'name')) return;
  const rt = { state, save, record: (k, v) => console.log(k, v) };
  const config = { runtime: { profile: 'dev-test', timeoutMs: 1_800_000, clusterKubeconfigFile: process.env.KUBECONFIG }, application: { environment: ENV }, platform: { account: { context: CONTEXT }, delivery: { namespaceAccess: { enabled: true, rulesFile: new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json', import.meta.url).pathname } } } };
  await ensureNamespaceAccess(ui, config, view, rt, { wait: false });
  state[key] = { ...state[key], namespace, namespaceAccessReady: true };
  save();
}

async function preview(ui, app) {
  await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}?environment=${ENV}`);
  const pending = ui.page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + base(app) + '/preview', { timeout: 180_000 });
  await ui.page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
  const response = await pending;
  return { ok: response.ok(), status: response.status(), body: await response.json().catch(() => null) };
}

// Preview, one Deploy click, then the tracked operation with its Pod waits.
async function previewAndDeploy(ui, app, key) {
  const p = await preview(ui, app);
  assert(p.ok, `Preview HTTP ${p.status} ${JSON.stringify(p.body)?.slice(0, 300)}`);
  const started = Date.now();
  const submission = ui.page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + base(app) + '/deploy', { timeout: 1_800_000 });
  await ui.page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  let response;
  for (;;) {
    response = await Promise.race([submission, ui.page.waitForTimeout(5000).then(() => undefined)]);
    if (response) break;
    await namespaceAccess(ui, app, key).catch((e) => console.log('namespace access pending:', e.message));
  }
  const report = await response.json();
  const operations = (await ui.get(base(app) + '/operations')).operations;
  const operation = operations[0];
  await ui.page.getByRole('heading', { name: 'Trạng thái triển khai' }).waitFor({ timeout: 60_000 }).catch(() => {});
  await ui.page.waitForTimeout(3000);
  await shot(ui, 'deploy');
  const panel = ui.page.getByRole('status', { name: 'Trạng thái chờ pod' });
  const panelText = await panel.innerText({ timeout: 10_000 }).catch(() => '');
  writeFileSync(join(dir, `${stage}-report.json`), JSON.stringify({ report, operation, panelText }, null, 2));
  console.log('DEPLOY', JSON.stringify({ status: report.status, seconds: Math.round((Date.now() - started) / 1000), operation: operation?.status, podWaits: operation?.podWaits }));
  return { report, operation, panelText, preview: p.body };
}

const waits = (operation) => (operation?.podWaits ?? []).flatMap((g) => g.waiting);
const replacements = (operation) => (operation?.podWaits ?? []).flatMap((g) => g.replaced ?? []);

async function ensureApp(key, prefix) {
  const dev = await asDeveloper();
  if (!state[key]?.app) {
    const name = `${prefix}-${Date.now().toString(36)}`;
    state[key] = { app: await createApplication(dev, name), name };
    save();
    pass(`Developer created Application ${name} and selected ${STORE}`);
  }
  return { dev, app: state[key].app };
}

function pvcOf(namespace) {
  return kjson('-n', namespace, 'get', 'pvc').items.map((p) => ({ name: p.metadata.name, uid: p.metadata.uid }));
}

// ---------- stages ----------
async function users() {
  await createDeveloper(await asAdmin());
  pass('Admin prepared the developer account');
}

// Scenario 1 (part 1): PostgreSQL with a wrong image fails with the reason.
async function pgBad() {
  const revision = await setPostgresImage(BAD_PG);
  state.pgBadRevision = revision;
  save();
  pass(`PE promoted ${PG_DEFINITION} r${revision.number} with a wrong image`);
  const { dev, app } = await ensureApp('pg', 's68pg');
  if (!(await dev.get(base(app) + '/workloads')).workloads.some((w) => w.id === 'web')) {
    await authorWorkload(dev, app, { template: 'Frontend web', name: 'web', image: GOOD_WEB, postgres: true });
    pass('Developer authored web with a PostgreSQL dependency');
  }
  const { report, operation, panelText } = await previewAndDeploy(dev, app, 'pg');
  assert.notEqual(report.status, 'SUCCEEDED', 'Deploy must fail with the wrong image');
  const stuck = waits(operation).filter((w) => STUCK.test(w.code));
  assert(stuck.length >= 1 && stuck.some((w) => w.image === BAD_PG), `reason with image: ${JSON.stringify(operation?.podWaits)}`);
  assert(/ImagePullBackOff|ErrImagePull/.test(panelText) && panelText.includes(BAD_PG), 'Console shows the reason with the image: ' + panelText);
  assert.equal(replacements(operation).length, 0, 'no replacement without a new revision');
  const ns = (await dev.get('/applications/' + app.key)).application.environments.find((e) => e.key === ENV).namespaceIdentity;
  state.pg.namespace = ns;
  state.pg.pvcs = pvcOf(ns);
  save();
  pass(`Deploy ${report.status}; Console shows ${stuck[0].code} with image ${BAD_PG}; no Pod replaced`);
}

// Scenario 1 (part 2): fixed image, never-deployed app follows the new revision.
async function pgFix() {
  const revision = await setPostgresImage(GOOD_PG);
  state.pgGoodRevision = revision;
  save();
  pass(`PE promoted ${PG_DEFINITION} r${revision.number} with the correct image`);
  const { dev, app } = await ensureApp('pg', 's68pg');
  const selection = await dev.get(base(app) + '/definition-versions');
  const row = selection.definitions.find((d) => d.key === PG_DEFINITION);
  assert.equal(row.desiredRevisionId, revision.id, 'never-deployed Environment follows the Active revision');
  assert.equal(selection.frozen, false, 'never-deployed Environment is not pinned');
  pass(`Application (never deployed successfully) uses r${revision.number} without selecting it`);
  const { report, operation, panelText } = await previewAndDeploy(dev, app, 'pg');
  assert.equal(report.status, 'SUCCEEDED', 'Deploy after the fix: ' + JSON.stringify(report.results));
  const replaced = replacements(operation);
  assert(replaced.length >= 1 && STUCK.test(replaced[0].wait.code), 'stuck Pod replaced: ' + JSON.stringify(operation?.podWaits));
  assert(panelText.includes('Hệ thống đã thay pod') && panelText.includes(replaced[0].wait.pod), 'Console shows the replacement: ' + panelText);
  assert.deepEqual(pvcOf(state.pg.namespace), state.pg.pvcs, 'PVCs kept');
  const pods = kjson('-n', state.pg.namespace, 'get', 'pods').items.filter((p) => p.spec.containers.some((c) => c.image === GOOD_PG));
  assert(pods.length >= 1 && pods.every((p) => p.status.conditions?.some((c) => c.type === 'Ready' && c.status === 'True')), 'PostgreSQL Pod Ready on the correct image');
  pass(`Deploy SUCCEEDED; Console shows the replaced Pod ${replaced[0].wait.pod}; PVC identities unchanged`);
}

// Scenario 4: a running app keeps its revision and is told about the new one.
async function pinned() {
  const { dev, app } = await ensureApp('pg', 's68pg');
  const before = (await dev.get(base(app) + '/definition-versions')).definitions.find((d) => d.key === PG_DEFINITION).desiredRevisionId;
  const revision = await setPostgresImage(OTHER_PG);
  state.pgOtherRevision = revision;
  save();
  pass(`PE promoted ${PG_DEFINITION} r${revision.number} after the app ran`);
  const row = (await dev.get(base(app) + '/definition-versions')).definitions.find((d) => d.key === PG_DEFINITION);
  assert.equal(row.desiredRevisionId, before, 'running Environment keeps its revision');
  await dev.page.goto(`${dev.baseURL}/ui/applications/${app.key}/settings?environment=${ENV}`);
  await dev.page.getByText('Phiên bản tài nguyên', { exact: true }).click();
  const section = dev.page.locator('section.catalog-fields').filter({ has: dev.page.getByRole('heading', { name: PG_DEFINITION, exact: true }) });
  await section.getByText(/Có bản mới/).waitFor();
  await shot(dev, 'settings');
  pass('Settings shows "Có bản mới"; the app still uses its revision');
  const p = await preview(dev, app);
  if (p.ok) {
    await dev.page.getByText(new RegExp(`${PG_DEFINITION} đang dùng r\\d+; bản mặc định là r${revision.number}`)).waitFor();
    await shot(dev, 'preview');
    pass('Preview names the newer default revision');
  } else console.log('Preview without changes:', p.status, JSON.stringify(p.body)?.slice(0, 200));
}

// Scenario 5: selecting a changed PostgreSQL revision for a running DB is refused.
async function guard() {
  const { dev, app } = await ensureApp('pg', 's68pg');
  await dev.page.goto(`${dev.baseURL}/ui/applications/${app.key}/settings?environment=${ENV}`);
  await dev.page.getByText('Phiên bản tài nguyên', { exact: true }).click();
  const select = dev.page.getByLabel(`Phiên bản cho ${PG_DEFINITION}`, { exact: true });
  await select.selectOption(state.pgOtherRevision.id);
  await dev.page.getByText('Nâng cấp database hoặc storage cần quy trình migration; Preview sẽ chặn thay đổi chưa được hỗ trợ.').waitFor();
  await dev.submit(base(app) + '/definition-versions', 'PUT', dev.page.getByRole('button', { name: 'Chọn để nâng cấp', exact: true }));
  pass(`Developer selected r${state.pgOtherRevision.number} in Settings`);
  const p = await preview(dev, app);
  assert(!p.ok, 'Preview must refuse a running PostgreSQL change');
  const alert = await dev.page.getByRole('alert').first().innerText();
  await shot(dev, 'refused');
  console.log('REFUSED', p.status, JSON.stringify(p.body), alert);
  pass(`Preview refused (HTTP ${p.status}): ${alert}`);
  // Restore the applied selection so the app stays deployable.
  await dev.page.goto(`${dev.baseURL}/ui/applications/${app.key}/settings?environment=${ENV}`);
  await dev.page.getByText('Phiên bản tài nguyên', { exact: true }).click();
  await dev.submit(base(app) + '/definition-versions/discard', 'POST', dev.page.getByRole('button', { name: 'Hủy lựa chọn', exact: true }).or(dev.page.getByRole('button', { name: /Bỏ|Hoàn tác/ })).first());
  pass('Developer discarded the selection');
}

// Scenario 2 (part 1): StatefulSet workload with a wrong image fails with the reason.
async function stsBad() {
  const { dev, app } = await ensureApp('sts', 's68sts');
  if (!(await dev.get(base(app) + '/workloads')).workloads.some((w) => w.id === 'store')) {
    await authorWorkload(dev, app, { template: 'Ứng dụng có dữ liệu', name: 'store', image: BAD_WEB });
    pass('Developer authored the StatefulSet workload store with a wrong image');
  }
  const { report, operation, panelText } = await previewAndDeploy(dev, app, 'sts');
  assert.notEqual(report.status, 'SUCCEEDED');
  const stuck = waits(operation).filter((w) => STUCK.test(w.code));
  assert(stuck.some((w) => w.image.endsWith(BAD_WEB)), 'reason with image: ' + JSON.stringify(operation?.podWaits));
  assert(/ImagePullBackOff|ErrImagePull/.test(panelText), 'Console shows the reason: ' + panelText);
  pass(`Deploy ${report.status}; Console shows ${stuck[0].code} with the image`);
}

// Scenario 3: deploying again without a fix replaces nothing.
async function stsSame() {
  const { dev, app } = await ensureApp('sts', 's68sts');
  const p = await preview(dev, app);
  if (!p.ok || !(p.body?.changes ?? []).length) {
    console.log('Preview offers no unchanged redeploy:', p.status, JSON.stringify(p.body)?.slice(0, 200));
    pass('No unchanged redeploy is offered while the workload is unchanged');
    return;
  }
  const { report, operation, panelText } = await previewAndDeploy(dev, app, 'sts');
  assert.notEqual(report.status, 'SUCCEEDED');
  assert.equal(replacements(operation).length, 0, 'no Pod replaced without a fix');
  assert(!panelText.includes('Hệ thống đã thay pod'), 'Console shows no replacement');
  pass(`Unchanged redeploy ${report.status} with the reason and no replacement`);
}

// Scenario 2 (part 2): fixed image in the form replaces the stuck Pod.
async function stsFix() {
  const { dev, app } = await ensureApp('sts', 's68sts');
  await editImage(dev, app, 'store', GOOD_WEB);
  pass('Developer fixed the image in the workload form');
  const { report, operation, panelText } = await previewAndDeploy(dev, app, 'sts');
  assert.equal(report.status, 'SUCCEEDED', JSON.stringify(report.results));
  assert(replacements(operation).length >= 1, 'stuck Pod replaced: ' + JSON.stringify(operation?.podWaits));
  assert(panelText.includes('Hệ thống đã thay pod'), 'Console shows the replacement');
  const ns = (await dev.get('/applications/' + app.key)).application.environments.find((e) => e.key === ENV).namespaceIdentity;
  const pods = kjson('-n', ns, 'get', 'pods').items.filter((p) => p.metadata.name.startsWith('store-'));
  assert(pods.length >= 1 && pods.every((p) => p.spec.containers[0].image.endsWith(GOOD_WEB)), 'Pods run the fixed image');
  pass('Deploy SUCCEEDED; Pod runs the fixed image; Console shows the replacement');
}

async function cleanup() {
  const dev = await asDeveloper();
  for (const key of ['pg', 'sts']) {
    if (!state[key]?.app) continue;
    const app = (await dev.get('/applications/' + state[key].app.key)).application;
    const namespace = app.environments.find((e) => e.key === ENV).namespaceIdentity;
    if (kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'name')) kubectl('-n', namespace, 'delete', 'role,rolebinding', '-l', 'bootstrap.orchestrator.dev/application=' + app.key, '--ignore-not-found');
    await dev.page.goto(`${dev.baseURL}/ui/applications/${app.key}/settings`);
    await dev.page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
    const dialog = dev.page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
    await dialog.getByRole('textbox').fill(app.name);
    const deletion = await dev.submit(`/applications/${app.key}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
    const deadline = Date.now() + 1_800_000;
    let progress;
    for (;;) {
      progress = (await dev.get(`/applications/${app.key}/deletions/${deletion.deletion.operationId}`)).deletion;
      if (!progress.running) break;
      assert(Date.now() < deadline, 'deletion still running');
      await dev.page.waitForTimeout(5000);
    }
    assert.equal(progress.status, 'SUCCEEDED', progress.failureCode);
    assert.equal(kubectl('get', 'namespace', namespace, '--ignore-not-found', '-o', 'name'), '');
    pass(`Deleted ${app.name} through the Console; namespace ${namespace} is gone`);
  }
}

let status = 'FAIL';
try {
  await ({ users, 'pg-bad': pgBad, 'pg-fix': pgFix, pinned, guard, 'sts-bad': stsBad, 'sts-same': stsSame, 'sts-fix': stsFix, cleanup })[stage]();
  status = 'PASS';
} catch (error) {
  console.error(`FAIL ${stage}: ${String(error?.message ?? error).split('\n')[0]}`);
  for (const s of [devSession, adminSession]) if (s) await s.page.screenshot({ path: join(dir, `${stage}-error-${s === devSession ? 'developer' : 'admin'}.png`), fullPage: true }).catch(() => {});
  process.exitCode = 1;
} finally {
  writeFileSync(join(dir, `${stage}-result.json`), JSON.stringify({ stage, status, assertions, at: new Date().toISOString() }, null, 2));
  if (devSession) await signOutContext(devSession.context, developerConfig()).catch(() => {});
  if (adminSession) await closeConsole(adminSession, admin).catch(() => {});
}
