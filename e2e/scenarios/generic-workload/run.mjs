import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, statSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { execFileSync } from 'node:child_process';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';

const config = readConsoleConfig();
const directory = resolve(process.env.ORCH_TEST_RUN_DIR ?? '');
assert(process.env.ORCH_TEST_RUN_DIR && !directory.startsWith(process.cwd() + '/'));
assert.equal(statSync(directory).mode & 0o077, 0, 'Private run directory required');
const name = process.env.ORCH_TEST_NAME;
assert(name && /^[a-z0-9-]+$/.test(name));
const kubeconfig = process.env.ORCH_CLUSTER_KUBECONFIG_FILE;
assert(kubeconfig, 'Authorized cluster kubeconfig required');
const accountKey = process.env.ORCH_TEST_ACCOUNT;
assert(accountKey);
const mode = process.argv[2] ?? 'run';
assert(['run', 'verify', 'cleanup'].includes(mode));
const file = join(directory, 'flow.json');
let state = {};
try { state = JSON.parse(readFileSync(file, 'utf8')); } catch (error) { if (error.code !== 'ENOENT') throw error; }
const save = () => writeFileSync(file, JSON.stringify(state, null, 2), { mode: 0o600 });
const native = (args, input) => execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { input, encoding: 'utf8', timeout: 30000, stdio: ['pipe', 'pipe', 'pipe'] });
const json = args => JSON.parse(native([...args, '-o', 'json']));
const session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
const { page } = session;
page.setDefaultTimeout(30000);
const responseFor = (path, method) => page.waitForResponse(response => response.url().endsWith('/api/v1' + path) && response.request().method() === method, { timeout: 600000 });
const get = async path => { const response = await page.request.get(config.baseURL + '/api/v1' + path); assert(response.ok(), path + ': ' + response.status()); return response.json(); };
const setupPost = async (path, data) => { const response = await page.request.post(config.baseURL + '/api/v1' + path, { data }); const body = await response.json(); assert(response.ok(), `${path}: ${response.status()} ${JSON.stringify(body)}`); return body; };
const submit = async (path, method, button) => { const pending = responseFor(path, method); await button.click(); const response = await pending; const body = await response.json(); assert(response.ok(), JSON.stringify(body)); return body; };
const screenshot = phase => page.screenshot({ path: join(directory, phase + '.png'), fullPage: true });
let app;
let base;
let namespace;
function assertNamespace() {
 const object = json(['get', 'namespace', namespace]);
 assert.equal(object.metadata.labels['app.kubernetes.io/managed-by'], 'orchestrator');
 assert.equal(object.metadata.labels['orchestrator-managed'], 'true');
 state.namespaceUid ??= object.metadata.uid;
 assert.equal(object.metadata.uid, state.namespaceUid); save();
 return object;
}
async function verify(replicas) {
 assertNamespace();
 const deployment = json(['-n', namespace, 'get', 'deployment', 'order-api']);
 assert.equal(deployment.spec.replicas, replicas);
 assert((deployment.status.availableReplicas ?? 0) >= replicas);
 assert.equal(deployment.spec.template.spec.containers[0].image, process.env.ORCH_TEST_IMAGE ?? 'nginx:1.27-alpine');
 const service = json(['-n', namespace, 'get', 'service', 'order-api']);
 assert.equal(service.spec.ports[0].port, 80);
 assert.equal(service.spec.ports[0].targetPort, 80);
 const cm = json(['-n', namespace, 'get', 'configmap', 'workload-namespace']);
 assert.equal(cm.data.namespace, namespace);
 assert.equal(cm.metadata.labels['orchestrator.io/application'], app.key);
 const deployments = (await get(base + '/deployments')).deployments;
 const latest = deployments.find(entry => entry.status === 'SUCCEEDED'); assert(latest);
 const detail = await get(base + '/deployments/' + latest.id);
 assert.equal(detail.rendering['order-api'].driverType, 'template');
 assert.equal(detail.rendering['order-api'].definitionKey, 'generic-template');
 assert.equal(detail.rendering['order-api'].bundle.source.contentSha256, state.sourceDigest);
 assert(detail.rendering['order-api'].references.namespaceName);
 const repos = json(['get', 'gitrepos.fleet.cattle.io', '-A']).items.filter(repo => repo.metadata.labels?.['orchestrator.io/environment-id'] === detail.deployment.environmentId);
 assert(repos.length > 0, 'No Fleet GitRepos for the deployed environment');
 const bundles = json(['get', 'bundles.fleet.cattle.io', '-A']).items;
 for (const repo of repos) {
  assert.match(repo.spec.revision, /^[0-9a-f]{40}$/);
  assert.equal(repo.status.commit, repo.spec.revision);
  const owned = bundles.filter(bundle => bundle.metadata.namespace === repo.metadata.namespace && bundle.metadata.labels?.['fleet.cattle.io/repo-name'] === repo.metadata.name);
  assert.equal(owned.length, 1);
  assert.equal(owned[0].metadata.labels['fleet.cattle.io/commit'], repo.spec.revision);
  assert.equal(owned[0].status.summary?.desiredReady, 1);
  assert.equal(owned[0].status.summary?.ready, 1);
 }
 state.deploymentUid = deployment.metadata.uid;
 state.serviceUid = service.metadata.uid;
 state.deployments = deployments.map(entry => ({ id: entry.id, status: entry.status }));
 state.fleetRepositories = repos.map(repo => ({ namespace: repo.metadata.namespace, name: repo.metadata.name, uid: repo.metadata.uid, commit: repo.status?.commit }));
 state.verifiedReplicas = replicas; save();
}
async function previewDeploy() {
 await page.goto(config.baseURL + `/ui/applications/${app.key}?environment=staging`);
 const preview = await submit(base + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
 assert.equal(preview.changes.length, 1); assert.equal(preview.changes[0].workloadId, 'order-api');
 await screenshot('preview');
 const pending = responseFor(base + '/deploy', 'POST');
 await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
 const accessConfig = { platform: { account: { context: native(['config', 'current-context']).trim() }, delivery: { namespaceAccess: { enabled: true, rulesFile: resolve('scripts/bootstrap/applications/acceptance/fleet-access.json') } } }, runtime: { clusterKubeconfigFile: kubeconfig, timeoutMs: 300000 }, application: { environment: 'staging' } };
 state.deliverySubject = { name: process.env.ORCH_DELIVERY_SUBJECT_NAME ?? 'orchestrator-delivery', namespace: process.env.ORCH_DELIVERY_SUBJECT_NAMESPACE ?? 'cattle-fleet-system' };
 const rt = { state, save, record() {} };
 await ensureNamespaceAccess({ page }, accessConfig, app, rt);
 const response = await pending; const result = await response.json();
 state.lastDeploy = { httpStatus: response.status(), result }; save();
 assert(response.ok(), JSON.stringify(result)); assert.equal(result.status, 'SUCCEEDED');
 await screenshot('delivered');
}
async function importWorkload(replicas, existing) {
 await page.goto(config.baseURL + `/ui/applications/${app.key}/environments/staging/workloads/${existing ? 'order-api' : 'new'}`);
 const custom = page.getByRole('button', { name: /Tự cấu hình/ }); if (await custom.count()) await custom.click();
 await page.getByRole('button', { name: 'Nhập Workload', exact: true }).click();
 const workload = { name: 'order-api', image: process.env.ORCH_TEST_IMAGE ?? 'nginx:1.27-alpine', replicas, ports: [{ name: 'http', containerPort: 80 }], service: { enabled: true, port: 80 }, env: { LOG_LEVEL: replicas === 2 ? 'info' : 'debug' } };
 const input = join(directory, 'workload.json'); writeFileSync(input, JSON.stringify(workload), { mode: 0o600 });
 await page.getByLabel('File Workload', { exact: true }).setInputFiles(input);
 await page.getByText('Đã phân tích Workload: order-api', { exact: false }).waitFor();
 await submit(base + '/workloads/order-api', 'PUT', page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}
try {
 const apps = (await get('/applications')).applications;
 app = apps.find(entry => entry.name === name);
 if (state.app && !(mode === 'cleanup' && state.cleanupPassed && !app)) assert(app?.key === state.app, 'Recorded Application identity changed');
 if (!app && mode === 'run') {
  await page.goto(config.baseURL + '/ui/applications/new'); await page.getByLabel('Tên ứng dụng', { exact: true }).fill(name);
  await submit('/applications', 'POST', page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }));
  app = (await get('/applications')).applications.find(entry => entry.name === name);
 }
 if (!app && mode === 'cleanup' && state.cleanupPassed) { console.log('PASS: recorded cleanup remains complete'); }
 else {
 assert(app?.key); state.app = app.key; state.name = name; save(); base = `/applications/${app.key}/environments/staging`; namespace = `app-${app.key}-staging`;
 if (mode === 'run') {
  const account = (await get('/driver-accounts')).accounts.find(entry => entry.key === accountKey); assert(account?.status === 'READY');
  const defs = (await get('/resource-definitions')).resourceDefinitions;
  if (!defs.some(entry => entry.key === 'generic-cluster')) await setupPost('/resource-definitions', { key: 'generic-cluster', name: 'Generic workload test cluster', resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: accountKey, driverInputs: { values: { variables: {} } }, criteria: [{ app_id: app.key, class: 'generic' }] });
  const capability = (await get('/resource-definition-capabilities')).capabilities.find(entry => entry.resourceType === 'k8s-namespace' && entry.driverType === 'terraform'); assert(capability?.source);
  const pin = capability.source;
  if (!defs.some(entry => entry.key === 'generic-namespace')) await setupPost('/resource-definitions', { key: 'generic-namespace', name: 'Generic workload test namespace', resourceType: 'k8s-namespace', driverType: 'terraform', driverInputs: { values: { source: { source_id: pin.sourceId, module_id: pin.moduleId, revision: pin.revision, subdirectory: pin.subdirectory, content_sha256: pin.contentSha256 }, variables: { name: '${context.env.namespace}' }, input_mapping: {}, output_mapping: { name: 'name' }, provider_bindings: { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.generic#environments.@app.@env' } } } }, criteria: [{ app_id: app.key }] });
  if (!defs.some(entry => entry.key === 'generic-template')) {
   await page.goto(config.baseURL + '/ui/platform/resource-definitions?resourceType=workload'); await page.getByText('Manifest Templates', { exact: true }).waitFor();
   await page.getByLabel('ID cấu hình (Definition ID)', { exact: true }).fill('generic-template'); await page.getByLabel('Tên hiển thị', { exact: true }).fill('Generic workload templates');
   await page.getByRole('button', { name: 'Nhập trực tiếp', exact: true }).click();
   await page.getByLabel('Tên file template', { exact: true }).fill('configmap.tpl'); await page.getByRole('button', { name: 'Thêm template', exact: true }).click();
   await page.getByLabel('configmap.tpl', { exact: true }).fill('{{ toJson (list (dict "apiVersion" "v1" "kind" "ConfigMap" "metadata" (dict "name" "workload-namespace") "data" (dict "namespace" .Resources.namespaceName)))) }}'.replace('namespaceName))))', 'namespaceName)))'));
   const prepared = await submit('/template-sources/inline', 'POST', page.getByRole('button', { name: 'Kiểm tra template', exact: true })); state.sourceDigest = prepared.templateSource.source.contentSha256; save();
   const namespaceRef = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
   await page.getByLabel('Tham chiếu namespace', { exact: true }).fill(namespaceRef);
   await page.getByRole('textbox', { name: /^Tham chiếu output \(JSON\)/ }).fill(JSON.stringify({ namespaceName: namespaceRef }));
   await page.getByRole('tab', { name: /^Điều kiện áp dụng/ }).click(); await page.getByRole('button', { name: '+ Thêm điều kiện', exact: true }).click();
   await page.getByRole('combobox', { name: 'Trường hợp 1 thuộc tính 1', exact: true }).selectOption('app_id'); await page.getByRole('textbox', { name: /Điều kiện 1.*ID ứng dụng/ }).fill(app.key);
   await submit('/resource-definitions', 'POST', page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true })); await screenshot('definition-saved');
  }
  if (!state.baselinePassed) { await importWorkload(2, (await get(base + '/workloads')).workloads.some(entry => entry.id === 'order-api')); await previewDeploy(); await verify(2); state.baselinePassed = true; save(); }
  if (!state.updatePassed) { const uid = state.namespaceUid; await importWorkload(1, true); await previewDeploy(); await verify(1); assert.equal(state.namespaceUid, uid); state.updatePassed = true; save(); }
  console.log('PASS: Inline Definition, Generic Workload, references, Preview/Deploy, update and real manifests');
 } else if (mode === 'verify') { await verify(state.verifiedReplicas ?? 1); console.log('PASS: real workload and immutable pins verified'); }
 else {
  assertNamespace();
  const workloads = (await get(base + '/workloads')).workloads;
  if (workloads.some(entry => entry.id === 'order-api')) {
   page.on('dialog', async dialog => { assert.match(dialog.message(), /order-api/); await dialog.accept(); });
   await page.goto(config.baseURL + `/ui/applications/${app.key}?environment=staging`);
   const row = page.locator('.table-row').filter({ has: page.locator('.workload-name').filter({ hasText: /order-api$/ }) });
   await submit(base + '/workloads/order-api', 'DELETE', row.getByRole('button', { name: 'Xóa', exact: true }));
   const preview = await submit(base + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true })); assert(preview.changes.every(change => change.action === 'REMOVE'));
   const result = await submit(base + '/deploy', 'POST', page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true })); assert.equal(result.status, 'SUCCEEDED');
  }
  assert.equal(json(['-n', namespace, 'get', 'deployments']).items.length, 0);
  const accessName = process.env.ORCH_TEST_ACCESS_NAME ?? 'bootstrap-fleet-access';
  for (const kind of ['rolebinding', 'role']) {
   const raw = native(['-n', namespace, 'get', kind, accessName, '--ignore-not-found', '-o', 'json']); if (!raw.trim()) continue;
   const object = JSON.parse(raw); if (accessName === 'bootstrap-fleet-access') assert.equal(object.metadata.labels['bootstrap.orchestrator.dev/application'], app.key);
   else { assert.equal(accessName, 'generic-workload-fleet'); assert.equal(object.metadata.labels['orchestrator.io/test-run'], name); }
   native(['delete', '--raw', `/apis/rbac.authorization.k8s.io/v1/namespaces/${namespace}/${kind}s/${accessName}`, '-f', '-'], JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: object.metadata.uid } }));
  }
  await page.goto(config.baseURL + `/ui/applications/${app.key}/settings?environment=staging`); await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true }); await dialog.getByRole('textbox').fill(name); await screenshot('cleanup-confirmation');
  await submit(`/applications/${app.key}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
  await page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true }).waitFor({ timeout: 600000 });
  assert.equal((await page.request.get(config.baseURL + '/api/v1/applications/' + app.key)).status(), 404);
  assert.equal(native(['get', 'namespace', namespace, '--ignore-not-found', '-o', 'name']).trim(), ''); state.cleanupPassed = true; save(); await screenshot('cleanup-passed'); console.log('PASS: product cleanup and namespace absence');
 }
 }
} catch (error) { await screenshot('failure'); throw error; }
finally { await closeConsole(session, config); }
