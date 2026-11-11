// Issue #72: real Console authoring, reference round-trip and one Environment Deploy.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const stage = process.argv[2]; assert(['prepare', 'author', 'editor', 'deploy', 'verify', 'cleanup'].includes(stage));
const dir = process.env.ORCH_TEST_RUN_DIR; assert(dir); outsideRepo(dir);
const file = dir + '/state.json';
const state = existsSync(file) ? JSON.parse(readFileSync(file)) : {};
const save = () => writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
const config = readConsoleConfig(), session = await openConsole(config, {}), { page } = session;
const ui = makeUI(page, config.baseURL, 1_800_000);
page.on('pageerror', error => console.log('PAGE_ERROR', error.name, error.message));
const base = () => `/applications/${state.app.key}/environments/staging`;
const kube = (...args) => execFileSync('kubectl', ['--kubeconfig', process.env.KUBECONFIG, '--context', 'kind-k8s-4f', ...args], { encoding: 'utf8', timeout: 60_000, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const rows = () => page.getByLabel(/^Tên biến, dòng \d+$/).evaluateAll(inputs => inputs.map(input => input.value));
async function open(name) { await page.goto(config.baseURL + `/ui/applications/${state.app.key}/environments/staging/workloads/${name}`); await page.getByRole('heading', { name: 'Sửa ' + name, exact: true }).waitFor(); await page.getByLabel('Tên biến, dòng 1', { exact: true }).waitFor(); }
async function textTab() { await page.getByRole('button', { name: 'Nhập từ file .env', exact: true }).click(); return page.getByLabel('Nội dung .env', { exact: true }); }
async function group(title) { const item = page.locator('details').filter({ has: page.locator('summary strong').getByText(title, { exact: true }) }).first(); if (await item.getAttribute('open') === null) await item.locator('summary').first().click(); return item; }
async function prepare() {
  if (!state.app) {
    assert(!(await ui.get('/applications')).applications.some(app => app.name === 'envfile-issue72'), 'Inspect an existing same-name Application first');
    await page.goto(config.baseURL + '/ui/applications/new'); await page.getByLabel('Tên ứng dụng', { exact: true }).fill('envfile-issue72');
    state.app = (await ui.submit('/applications', 'POST', page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }))).application; save();
  }
  const stores = (await ui.get('/secret-store-choices')).secretStores;
  const store = stores.find(store => store.name === 'Dev Platform Vault'); assert(store);
  const app = (await ui.get('/applications/' + state.app.key)).application;
  if (!app.environments.find(env => env.key === 'staging').secretStoreKey) {
    await page.goto(config.baseURL + `/ui/applications/${state.app.key}/settings?environment=staging`);
    await page.getByLabel('Kho bí mật cho Staging', { exact: true }).selectOption(store.key);
    await ui.submit(base() + '/secret-store', 'PUT', page.getByRole('button', { name: 'Lưu kho bí mật', exact: true }));
  }
  await page.goto(config.baseURL + `/ui/applications/${state.app.key}/settings?environment=staging`);
  const current = await ui.get(base() + '/configuration');
  if (!current.keys.some(key => key.name === 'REGION')) {
    const section = page.locator('section[aria-label="Biến"]');
    await section.getByRole('button', { name: '+ Thêm biến', exact: true }).click();
    await page.getByLabel('Tên key', { exact: true }).fill('REGION');
    await page.getByLabel('Giá trị', { exact: true }).fill('ap-southeast-1');
    await ui.submit(base() + '/configuration/keys/REGION', 'PUT', page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai', exact: true }));
  }
  const def = await ui.get('/delivery-default');
  const binding = await ui.get(`/delivery-bindings/${def.bindingId}?version=${def.bindingVersion}`);
  state.deliverySubject = { name: binding.config.serviceAccount, namespace: binding.config.agentNamespace }; save();
  console.log('PASS prepared');
}
async function newWorkload(name) {
  await page.goto(config.baseURL + `/ui/applications/${state.app.key}/environments/staging/workloads/new`);
  await page.getByLabel('Tên Workload', { exact: true }).fill(name);
  await page.getByLabel('Ảnh Container', { exact: true }).fill('postgres:16-alpine');
  await page.getByRole('button', { name: 'Hiện nâng cao Container main', exact: true }).click();
  const startup = await group('Khởi chạy');
  await startup.getByLabel('Tham số (mỗi dòng một phần tử)', { exact: true }).fill('sh\n-c\nexec sleep 2147483647');
}
async function saveWorkload(name) {
  await ui.submit(base() + '/workloads/' + name, 'PUT', page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}
async function author() {
  const existing = (await ui.get(base() + '/workloads')).workloads;
  assert(existing.every(item => item.id === 'backend'), 'Inspect already saved envfile workload before retrying');
  if (!existing.length) {
    await newWorkload('backend');
    await page.getByRole('button', { name: '+ Thêm cổng', exact: true }).click();
    await page.getByLabel('Tên cổng Service', { exact: true }).fill('http');
    await page.getByLabel('Cổng Service', { exact: true }).fill('8080');
    await saveWorkload('backend');
  } else {
    assert.equal(existing[0].workload.containers.main.image, 'postgres:16-alpine');
    assert.equal(existing[0].workload.service.ports.http.port, 8080);
  }
  await newWorkload('envfile');
  for (const [index, alias] of ['db-chinh', 'db-baocao'].entries()) {
    await page.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }).click();
    await page.getByLabel('Alias tài nguyên', { exact: true }).nth(index).fill(alias);
    await page.getByLabel('Loại tài nguyên', { exact: true }).nth(index).selectOption('postgres');
    await page.getByLabel('Cách cung cấp (class) của ' + alias, { exact: true }).selectOption('in-cluster');
    await page.getByLabel('Tham số database', { exact: true }).nth(index).fill(index ? 'reports' : 'main');
    await page.getByLabel('Tham số username', { exact: true }).nth(index).fill('app');
  }
  const content = ['ZETA=production', 'REGION=@variable.REGION', 'DB_HOST=@db-chinh.host', 'DB_PASSWORD=@db-chinh.password', 'REPORT_HOST=@db-baocao.host', 'API_URL=@service.backend.http', 'CONTACT=@@team.example', 'EMPTY=', 'MULTILINE="Tiếng Việt\\nline=two"', '# @secret', 'TOKEN=issue72-synthetic-token', '# @secret', 'SIGNING=issue72-synthetic-signing', ...Array.from({ length: 25 }, (_, i) => `EXTRA_${i}=value-${i}`)].join('\n');
  await (await textTab()).fill(content);
  await page.getByRole('button', { name: 'Quay lại bảng', exact: true }).click();
  assert.equal((await rows()).length, 36);
  // Rename is local, keeping all bindings; no resource/provision mutation.
  await page.getByLabel('Alias tài nguyên', { exact: true }).first().fill('db-primary');
  const text = await textTab();
  assert((await text.inputValue()).includes('@db-primary.host'));
  assert(!(await text.inputValue()).includes('@db-chinh.'));
  await page.getByRole('button', { name: 'Xác nhận bí mật', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xác nhận bí mật', exact: true });
  await dialog.getByRole('button', { name: 'Tạo và gắn tham chiếu', exact: true }).click();
  await dialog.getByText('Đã tạo', { exact: true }).nth(1).waitFor();
  await dialog.getByRole('button', { name: 'Đóng', exact: true }).click();
  const value = await text.inputValue();
  assert(value.includes('@secret.TOKEN')); assert(!value.includes('issue72-synthetic'));
  state.order = value.split('\n').filter(line => !line.startsWith('#')).map(line => line.split('=')[0]); save();
  await page.screenshot({ path: dir + '/author-vi.png', fullPage: true });
  await saveWorkload('envfile');
  await open('envfile'); await textTab();
  assert((await page.getByLabel('Nội dung .env').inputValue()).includes('@secret.TOKEN'));
  const saved = (await ui.get(base() + '/workloads')).workloads.find(item => item.id === 'envfile').workload;
  assert.deepEqual(saved.containers.main.variableOrder, state.order);
  assert(!JSON.stringify(saved).includes('issue72-synthetic'));
  state.saved = saved; save(); console.log('PASS authored and reopened 36 rows with five sources');
}
async function editor() {
  await open('envfile');
  let mutations = 0;
  const observe = request => { if (request.method() === 'PUT' && new URL(request.url()).pathname.includes('/workloads/')) mutations++; };
  page.on('request', observe);
  const text = await textTab(), original = await text.inputValue();
  await text.fill(original + '\nMETADATA_CASE=@late-check.host');
  assert(await page.getByRole('button', { name: 'Quay lại bảng', exact: true }).isDisabled());
  assert(await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).isDisabled());
  const count = await page.getByLabel('Alias tài nguyên', { exact: true }).count();
  await page.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }).click();
  await page.getByLabel('Alias tài nguyên', { exact: true }).nth(count).fill('late-check');
  await page.getByLabel('Loại tài nguyên', { exact: true }).nth(count).selectOption('postgres');
  await page.getByRole('button', { name: 'Quay lại bảng', exact: true }).click();
  assert.equal(await page.getByLabel('Tên biến, dòng 37', { exact: true }).inputValue(), 'METADATA_CASE');
  assert((await page.getByRole('button', { name: 'Đổi nguồn, dòng 37', exact: true }).innerText()).includes('late-check.host'));
  await page.getByRole('button', { name: 'Hủy', exact: true }).click();
  await open('envfile'); assert.deepEqual(await rows(), state.order);
  page.off('request', observe); assert.equal(mutations, 0);
  await page.screenshot({ path: dir + '/metadata-reconciliation.png' });
  console.log('PASS metadata reconciliation and cancellation without a workload mutation');
}
async function deploy(expected = ['backend', 'envfile']) {
  assert(!state.attempt, 'Recorded Deploy must be inspected before another attempt');
  await page.goto(config.baseURL + `/ui/applications/${state.app.key}?environment=staging`);
  const preview = await ui.submit(base() + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  assert.deepEqual((preview.changes ?? []).map(item => item.workloadId).sort(), expected);
  assert(!JSON.stringify(preview).includes('issue72-synthetic'));
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1' + base() + '/deploy') { assert(!state.attempt); state.attempt = { reviewRequestId: request.postDataJSON()?.reviewRequestId }; save(); } });
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  const deadline = Date.now() + 1_800_000;
  for (;;) {
    const app = (await ui.get('/applications/' + state.app.key)).application;
    state.namespace = app.environments.find(env => env.key === 'staging').namespaceIdentity; save();
    if (!state.namespaceAccessReady && kube('get', 'namespace', state.namespace, '--ignore-not-found', '-o', 'name')) {
      await ensureNamespaceAccess(ui, { runtime: { profile: 'dev-test', timeoutMs: 1_800_000, clusterKubeconfigFile: process.env.KUBECONFIG }, application: { environment: 'staging' }, platform: { account: { context: 'kind-k8s-4f' }, delivery: { namespaceAccess: { enabled: true, rulesFile: new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json', import.meta.url).pathname } } } }, app, { state, save, record: console.log }, { wait: false }); state.namespaceAccessReady = true; save();
    }
    const operation = (await ui.get(base() + '/operations?reviewRequestId=' + state.attempt.reviewRequestId)).operations?.find(item => item.reviewRequestId === state.attempt.reviewRequestId);
    if (operation && ['SUCCEEDED', 'FAILED', 'RECOVERED', 'INTERRUPTED'].includes(operation.status)) { state.operationId = operation.id; state.status = operation.status; save(); assert.equal(operation.status, 'SUCCEEDED'); break; }
    assert(Date.now() < deadline, 'Deploy still active; inspect before retry'); await page.waitForTimeout(5000);
  }
  console.log('PASS one Environment deploy');
}
async function verify() {
  const deployment = JSON.parse(kube('-n', state.namespace, 'get', 'deployment', 'envfile', '-o', 'json'));
  assert.equal(deployment.status.readyReplicas, 1);
  const variables = deployment.spec.template.spec.containers[0].env;
  assert.equal(variables.find(item => item.name === 'CONTACT').value, '@team.example');
  assert.equal(variables.find(item => item.name === 'REGION').value, 'ap-southeast-1');
  assert.equal(variables.find(item => item.name === 'MULTILINE').value, 'Tiếng Việt\nline=two');
  assert.notEqual(variables.find(item => item.name === 'DB_HOST').value, variables.find(item => item.name === 'REPORT_HOST').value);
  assert.match(variables.find(item => item.name === 'API_URL').value, /backend.*8080/);
  assert(variables.find(item => item.name === 'TOKEN').valueFrom.secretKeyRef);
  assert(variables.find(item => item.name === 'DB_PASSWORD').valueFrom.secretKeyRef);
  const pods = JSON.parse(kube('-n', state.namespace, 'get', 'pods', '-l', 'app.kubernetes.io/name=envfile', '-o', 'json')).items;
  const token = kube('-n', state.namespace, 'exec', pods[0].metadata.name, '--', 'sh', '-c', 'test "$TOKEN" = issue72-synthetic-token && test "$SIGNING" = issue72-synthetic-signing && test -n "$DB_PASSWORD" && echo PASS');
  assert.equal(token, 'PASS');
  await open('envfile'); await page.getByRole('button', { name: 'Nhập từ file .env', exact: true }).waitFor(); assert.deepEqual(await rows(), state.order);
  const value = await (await textTab()).inputValue(); assert(value.includes('@service.backend.http')); assert(!value.includes('issue72-synthetic'));
  await page.screenshot({ path: dir + '/reopen-vi.png', fullPage: true });
  await page.locator('.language-select select').selectOption('en');
  await page.getByRole('button', { name: 'Import from .env file', exact: true }).click();
  assert(!(await page.getByLabel('.env content').inputValue()).includes('issue72-synthetic'));
  await page.screenshot({ path: dir + '/reopen-en.png', fullPage: true });
  await page.locator('.language-select select').selectOption('vi');
  await textTab();
  const downloaded = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Tải .env', exact: true }).click();
  const output = await downloaded; await output.saveAs(dir + '/reference.env');
  assert(!readFileSync(dir + '/reference.env', 'utf8').includes('issue72-synthetic'));
  const example = page.waitForEvent('download');
  await page.getByRole('button', { name: 'Tải mẫu', exact: true }).click();
  const sample = await example; await sample.saveAs(dir + '/example.env');
  assert(readFileSync(dir + '/example.env', 'utf8').includes('@db-primary.host'));
  await page.getByLabel('Chọn file UTF-8', { exact: true }).setInputFiles({ name: '.env', mimeType: 'text/plain', buffer: Buffer.from('ZETA=replace-me\nIMPORTED=added') });
  const review = page.getByRole('dialog', { name: 'Rà soát file .env', exact: true });
  assert.equal(await review.getByRole('combobox').first().inputValue(), 'keep');
  await review.getByRole('button', { name: 'Nhập vào bản nháp', exact: true }).click();
  const merged = await page.getByLabel('Nội dung .env', { exact: true }).inputValue();
  assert(merged.includes('ZETA="production"')); assert(merged.includes('IMPORTED="added"'));
  await page.getByRole('button', { name: 'Hủy', exact: true }).click();
  await open('envfile'); assert.deepEqual(await rows(), state.order);
  for (const width of [1512, 1280]) {
    await page.setViewportSize({ width, height: width === 1512 ? 980 : 900 });
    assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth));
    await page.screenshot({ path: dir + `/ui-${width}.png`, fullPage: true });
  }
  console.log('PASS runtime, secret delivery, reopen VI/EN, download and merge');
}
async function cleanup() {
  const namespace = JSON.parse(kube('get', 'namespace', state.namespace, '-o', 'json'));
  assert.equal(namespace.metadata.uid, state.namespaceUid, 'Owned namespace UID must match');
  assert.equal(namespace.metadata.labels['app.kubernetes.io/managed-by'], 'orchestrator');
  // These two objects are technical setup owned by this scenario, not product
  // resources. Remove them before the product's foreign-child namespace guard.
  for (const kind of ['rolebindings', 'roles']) {
    const object = JSON.parse(kube('-n', state.namespace, 'get', kind, 'bootstrap-fleet-access', '--ignore-not-found', '-o', 'json') || 'null');
    if (!object) continue;
    assert.equal(object.metadata.labels['bootstrap.orchestrator.dev/application'], state.app.key);
    const options = { apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: object.metadata.uid } };
    execFileSync('kubectl', ['--kubeconfig', process.env.KUBECONFIG, '--context', 'kind-k8s-4f', 'delete', '--raw', `/apis/rbac.authorization.k8s.io/v1/namespaces/${state.namespace}/${kind}/bootstrap-fleet-access`, '-f', '-'], { input: JSON.stringify(options), encoding: 'utf8', timeout: 60_000, stdio: ['pipe', 'pipe', 'pipe'] });
    assert.equal(kube('-n', state.namespace, 'get', kind, 'bootstrap-fleet-access', '--ignore-not-found', '-o', 'name'), '');
  }
  await page.goto(config.baseURL + `/ui/applications/${state.app.key}/settings?environment=staging`);
  const app = (await ui.get('/applications/' + state.app.key)).application;
  if (app.status === 'DELETING') await page.getByRole('region', { name: 'Tiến trình xóa ứng dụng', exact: true }).getByRole('button', { name: 'Thử lại', exact: true }).click();
  else await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  await dialog.getByRole('textbox').fill(state.app.name);
  const response = await ui.submit(`/applications/${state.app.key}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
  const operation = response.deletion.operationId; state.deletionId = operation; save();
  const deadline = Date.now() + 1_800_000;
  for (;;) { const result = (await ui.get(`/applications/${state.app.key}/deletions/${operation}`)).deletion; if (!result.running) { assert.equal(result.status, 'SUCCEEDED'); break; } assert(Date.now() < deadline); await page.waitForTimeout(3000); }
  assert.equal(kube('get', 'namespace', state.namespace, '--ignore-not-found', '-o', 'name'), ''); state.cleaned = true; save(); console.log('PASS cleanup');
}
try { await ({ prepare, author, editor, deploy, verify, cleanup })[stage](); }
catch (error) { console.log('UI_STATE', page.url(), await page.getByRole('heading').allTextContents(), await page.getByRole('tab').allTextContents()); throw error; }
finally { await closeConsole(session, config); }
