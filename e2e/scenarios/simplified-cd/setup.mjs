import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, statSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { homedir } from 'node:os';
import { openConsole, closeConsole, readConsoleConfig, readPrivateFile } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';

const sourcePath = resolve(process.argv[2] ?? 'scripts/bootstrap/targets/dev.json');
const source = JSON.parse(readFileSync(sourcePath, 'utf8'));
assert.equal(source.environment, 'dev', 'This scenario is authorized for dev only');
const privatePath = value => value.startsWith('~/') ? join(homedir(), value.slice(2)) : resolve(sourcePath, '..', value);
const directory = process.env.ORCH_SIMPLIFIED_CD_RUN_DIR;
assert(directory && !resolve(directory).startsWith(process.cwd() + '/'), 'Set a private run directory outside the repository');
outsideRepo(directory);
assert.equal(statSync(directory).mode & 0o077, 0, 'Run directory must be private');
const file = join(directory, 'setup.json');
let state = {};
try { state = JSON.parse(readFileSync(file, 'utf8')); } catch (error) { if (error.code !== 'ENOENT') throw error; }
const config = readConsoleConfig({ ORCH_PLATFORM_URL: source.console.url, ORCH_PLATFORM_USERNAME: source.console.username, ORCH_PLATFORM_PASSWORD_FILE: privatePath(source.console.passwordFile), ORCH_HEADLESS: '1' });
const name = state.name ?? process.env.ORCH_SIMPLIFIED_CD_NAME ?? 'issue31-' + Date.now();
assert(/^[a-z0-9][a-z0-9-]{0,48}$/.test(name), 'Run name must be a bounded DNS label');
state.name = name;
assert(!state.origin || state.origin === config.baseURL, 'Run belongs to another Console');
state.origin = config.baseURL;
const save = () => writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
save();
const session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
const { page } = session;
const ui = makeUI(page, config.baseURL, 180000);
try {
 if (!state.originalDefault) {
  state.originalDefault = await ui.get('/delivery-default');
  state.pins = [];
  for (const app of (await ui.get('/applications')).applications) {
   for (const env of ['staging', 'production']) {
    const view = await ui.get(`/applications/${app.key}/environments/${env}/delivery`);
    state.pins.push({ app: app.key, env, desired: view.desired, applied: view.applied });
   }
  }
  save();
 }
 const accounts = (await ui.get('/driver-accounts')).accounts.filter(a => a.name === name);
 assert(accounts.length <= 1, 'Ambiguous task account');
 let account = accounts[0];
 if (!account) {
  await page.goto(config.baseURL + '/ui/platform/cd-providers');
  await page.getByRole('button', { name: 'Thêm cấu hình CD', exact: true }).click();
  await page.getByRole('button', { name: 'Thêm kết nối Kubernetes', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Thêm kết nối Kubernetes', exact: true });
  await dialog.getByLabel('Tên tài khoản', { exact: true }).fill(name);
  await dialog.getByLabel('File kubeconfig', { exact: true }).setInputFiles(privatePath(source.platform.account.kubeconfigFile));
  await dialog.getByRole('definition').filter({ hasText: source.platform.account.context }).first().waitFor();
  account = await ui.submit('/driver-accounts/internal', 'POST', dialog.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }));
  await dialog.waitFor({ state: 'hidden' });
  await page.getByRole('button', { name: 'Hủy', exact: true }).click();
 }
 assert.equal(account.status, 'READY');
 assert.equal(account.config.kubeContext, source.platform.account.context);
 assert.equal(account.config.endpoint, source.platform.account.endpoint);
 state.account = { key: account.key, version: account.version, name: account.name }; save();
 const connections = (await ui.get('/connections')).connections.filter(c => c.kind === 'GIT' && c.name === name + '-git');
 assert(connections.length <= 1, 'Ambiguous task Git connection');
 let git = connections[0];
 const bindings = (await ui.get('/delivery-bindings')).bindings.filter(b => b.name === name);
 assert(bindings.length <= 1, 'Ambiguous task binding');
 let binding = bindings[0];
 if (!binding) {
  await page.goto(config.baseURL + '/ui/platform/cd-providers');
  await page.getByRole('button', { name: 'Thêm cấu hình CD', exact: true }).click();
  const form = page.locator('form.secret-store-form');
  await form.getByLabel('Tên cấu hình', { exact: true }).fill(name);
  const pending = ui.responseFor('/delivery-providers/fleet/inspect', 'POST');
  await form.getByLabel('Tài khoản quản lý CD', { exact: true }).selectOption(account.key);
  const response = await pending;
  assert(response.ok(), 'Fleet discovery failed');
  const discovery = await response.json();
  assert(discovery.options.targetCluster.some(o => !o.disabled), 'No proven target');
  await form.getByLabel('Cluster đã đăng ký', { exact: true }).waitFor();
  if (!git) {
   await form.getByLabel('Kết nối Git', { exact: true }).selectOption('__new');
   await form.getByLabel('Tên kết nối Git', { exact: true }).fill(name + '-git');
   await form.getByLabel('URL phạm vi Git (tài khoản/nhóm)', { exact: true }).fill(source.platform.gitAccess.url);
   await form.getByLabel('Tên đăng nhập Git', { exact: true }).fill(source.platform.gitAccess.username);
   await form.getByLabel('Token hoặc mật khẩu Git', { exact: true }).fill(readPrivateFile(privatePath(source.platform.gitAccess.tokenFile), 'Git credential'));
   git = await ui.submit('/connections/git', 'POST', form.getByRole('button', { name: 'Lưu kết nối Git', exact: true }));
   assert.equal(await form.getByLabel('Token hoặc mật khẩu Git', { exact: true }).count(), 0);
  } else await form.getByLabel('Kết nối Git', { exact: true }).selectOption(git.key);
  state.gitKey = git.key; save();
  await page.screenshot({ path: join(directory, 'four-fields.png'), fullPage: true });
  binding = await ui.submit('/delivery-bindings', 'POST', form.getByRole('button', { name: 'Lưu cấu hình', exact: true }));
 }
 state.binding = binding; state.gitKey = git?.key ?? state.gitKey; save();
 await page.goto(config.baseURL + '/ui/platform/cd-providers');
 if (binding.status !== 'READY') {
  const row = page.locator('tbody tr').filter({ has: page.getByRole('cell', { name, exact: true }) });
  binding = await ui.submit(`/delivery-bindings/${binding.id}/verify`, 'POST', row.getByRole('button', { name: 'Xác minh', exact: true }));
 }
 state.binding = binding; save();
 assert.equal(binding.status, 'READY', `Binding verification: ${binding.reasonCode}`);
 const details = await ui.get(`/delivery-bindings/${binding.id}?version=${binding.version}`);
 assert.equal(details.managementAccountKey, account.key);
 assert.equal(details.config.gitConnectionKey, state.gitKey);
 assert.equal(details.config.readSecretMode, 'MANAGED');
 state.managedReader = { name: details.config.readSecret, workspace: details.config.workspace }; save();
 const current = await ui.get('/delivery-default');
 if (current.bindingId !== binding.id || current.bindingVersion !== binding.version) {
  const row = page.locator('tbody tr').filter({ has: page.getByRole('cell', { name, exact: true }) });
  await ui.submit('/delivery-default', 'PUT', row.getByRole('button', { name: 'Dùng cho ứng dụng mới', exact: true }));
 }
 for (const pin of state.pins) {
  const view = await ui.get(`/applications/${pin.app}/environments/${pin.env}/delivery`);
  assert.deepEqual(view.desired, pin.desired); assert.deepEqual(view.applied, pin.applied);
 }
 state.setupComplete = true; save();
 await page.screenshot({ path: join(directory, 'verified.png'), fullPage: true });
 console.log('PASS: inline Kubernetes/Git connections, real Fleet discovery/verification, default change preserves existing pins.');
} finally { await closeConsole(session, config); }
