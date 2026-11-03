import assert from 'node:assert/strict';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { loadBootstrapConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { openConsole, closeConsole } from '../../lib/console.mjs';
import { makeUI, waitIdle } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { kubectl } from '../../../scripts/bootstrap/lib/verification.mjs';

const directory = process.env.ORCH_SIMPLIFIED_CD_RUN_DIR;
assert(directory, 'Private run directory required');
const config = loadBootstrapConfig(process.argv[2]);
assert.equal(config.environment, 'dev');
assert.equal(config.application.environment, 'staging');
const file = join(directory, 'setup.json');
const setup = JSON.parse(readFileSync(file));
assert.equal(setup.origin, config.console.baseURL);
const state = JSON.parse(readFileSync(join(config.runtime.stateDir, 'identity.json')));
assert(setup.setupComplete && setup.rollbackPassed, 'Complete acceptance and rollback before cleanup');
assert(/^[0-9a-f-]{36}$/.test(state.appKey));
const namespace = `app-${state.appKey}-staging`;
const native = (args, input) => execFileSync('kubectl', ['--kubeconfig', config.runtime.clusterKubeconfigFile, '--context', config.platform.account.context, ...args], { input, encoding: 'utf8', timeout: 30000, stdio: ['pipe', 'pipe', 'pipe'] });
const session = await openConsole(config.console, { role: 'PLATFORM_ENGINEER' });
const { page } = session;
page.on('dialog', async dialog => {
 assert.equal(dialog.type(), 'confirm');
 assert.match(dialog.message(), /^Đánh dấu xóa (backend|frontend|worker) trong staging\?/);
 await dialog.accept();
});
const ui = makeUI(page, config.console.baseURL, config.runtime.timeoutMs);
const save = () => writeFileSync(file, JSON.stringify(setup, null, 2), { mode: 0o600 });
try {
 const reply = await page.request.get(config.console.baseURL + '/api/v1/applications/' + state.appKey);
 if (reply.status() !== 404) {
  assert(reply.ok());
  const app = (await reply.json()).application;
  assert.equal(app.name, config.application.name);
  assert.equal(app.subdomain, config.application.subdomain);
  assert.equal(kubectl(config, 'get', 'namespace', namespace).metadata.uid, state.namespaceUid);
  await waitIdle(ui, state.appKey, 'staging', config.runtime.timeoutMs);
  const base = `/applications/${state.appKey}/environments/staging`;
  if (!setup.workloadRemovalPassed) {
   await page.goto(config.console.baseURL + `/ui/applications/${state.appKey}?environment=staging`);
   await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).waitFor();
   const workloads = (await ui.get(base + '/workloads')).workloads;
   assert.deepEqual(workloads.map(w => w.id).sort(), ['backend', 'frontend', 'worker']);
   for (const workload of workloads) {
    const name = workload.id;
    if (workload.state === 'PENDING_DELETE') continue;
    assert(!workload.state, 'Unexpected pending workload state');
    const row = page.locator('.table-row').filter({ has: page.locator('.workload-name').filter({ hasText: new RegExp(name + '$') }) });
    await row.waitFor();
    const remove = row.getByRole('button', { name: 'Xóa', exact: true });
    await ui.submit(base + '/workloads/' + name, 'DELETE', remove);
   }
   const preview = await ui.submit(base + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
   assert.equal(preview.changes.length, 3);
   assert(preview.changes.every(change => change.action === 'REMOVE' && ['backend', 'frontend', 'worker'].includes(change.workloadId)));
   const report = await ui.submit(base + '/deploy', 'POST', page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
   assert.equal(report.status, 'SUCCEEDED');
   assert.equal(kubectl(config, '-n', namespace, 'get', 'deployments').items.length, 0);
   setup.workloadRemovalPassed = true; save();
  }
  setup.rbacCleanupUIDs ??= {};
  for (const kind of ['rolebinding', 'role']) {
   const raw = native(['-n', namespace, 'get', kind, 'bootstrap-fleet-access', '--ignore-not-found', '-o', 'json']);
   if (!raw.trim()) continue;
   const object = JSON.parse(raw);
   assert.equal(object.metadata.labels?.['bootstrap.orchestrator.dev/application'], state.appKey);
   setup.rbacCleanupUIDs[kind] ??= object.metadata.uid;
   assert.equal(object.metadata.uid, setup.rbacCleanupUIDs[kind]); save();
   native(['delete', '--raw', `/apis/rbac.authorization.k8s.io/v1/namespaces/${namespace}/${kind}s/bootstrap-fleet-access`, '-f', '-'], JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: object.metadata.uid } }));
  }
  await page.goto(config.console.baseURL + `/ui/applications/${state.appKey}/settings?environment=staging`);
  await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  await dialog.getByRole('textbox').fill(config.application.name);
  await page.screenshot({ path: join(directory, 'cleanup-confirmation.png'), fullPage: true });
  await ui.submit(`/applications/${state.appKey}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
  await page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true }).waitFor({ timeout: config.runtime.timeoutMs });
 }
 assert.equal((await page.request.get(config.console.baseURL + '/api/v1/applications/' + state.appKey)).status(), 404);
 assert.equal(native(['get', 'namespace', namespace, '--ignore-not-found', '-o', 'name']).trim(), '');
 assert.equal(kubectl(config, 'get', 'persistentvolumes').items.filter(p => p.spec.claimRef?.namespace === namespace).length, 0);
 const current = await ui.get('/delivery-default');
 const allowed = [setup.binding, setup.nextBinding].filter(Boolean).some(b => b.id === current.bindingId && b.version === current.bindingVersion);
 if (current.bindingId !== setup.originalDefault.bindingId || current.bindingVersion !== setup.originalDefault.bindingVersion) {
  assert(allowed, 'An unrelated default change must not be overwritten');
  const original = await ui.get(`/delivery-bindings/${setup.originalDefault.bindingId}?version=${setup.originalDefault.bindingVersion}`);
  await page.goto(config.console.baseURL + '/ui/platform/cd-providers');
  const row = page.locator('tbody tr').filter({ has: page.getByRole('cell', { name: original.binding.name, exact: true }) }).filter({ has: page.locator('td:nth-child(3)').filter({ hasText: new RegExp('^' + setup.originalDefault.bindingVersion + '$') }) });
  await ui.submit('/delivery-default', 'PUT', row.getByRole('button', { name: 'Dùng cho ứng dụng mới', exact: true }));
 }
 const restored = await ui.get('/delivery-default');
 assert.equal(restored.bindingId, setup.originalDefault.bindingId);
 assert.equal(restored.bindingVersion, setup.originalDefault.bindingVersion);
 for (const pin of setup.pins) {
  const view = await ui.get(`/applications/${pin.app}/environments/${pin.env}/delivery`);
  assert.deepEqual(view.desired, pin.desired); assert.deepEqual(view.applied, pin.applied);
 }
 if (state.forward?.pid) {
  try {
   const command = readFileSync(`/proc/${state.forward.pid}/cmdline`, 'utf8').split('\0');
   if (command.includes('port-forward') && command.includes(namespace) && command.includes(`${config.verification.forward.port}:8080`)) process.kill(state.forward.pid, 'SIGTERM');
  } catch (error) { if (error.code !== 'ENOENT' && error.code !== 'ESRCH') throw error; }
 }
 setup.cleanupPassed = true; save();
 await page.screenshot({ path: join(directory, 'cleanup-passed.png'), fullPage: true });
 console.log('PASS: product workload/application cleanup, namespace/PV absence, original default restored, existing pins preserved. Reusable connections/bindings, reader Secret and Git history retained.');
} finally { await closeConsole(session, config.console); }
