import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync, readdirSync, mkdirSync, statSync } from 'node:fs';
import { resolve, join, relative } from 'node:path';
import { execFileSync } from 'node:child_process';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';

const config = readConsoleConfig();
const root = resolve(process.env.ORCH_TEST_RUN_DIR ?? '');
assert(process.env.ORCH_TEST_RUN_DIR && !root.startsWith(process.cwd() + '/') && statSync(root).isDirectory() && !(statSync(root).mode & 0o077), 'Private run directory outside Git required');
const mode = process.argv[2] ?? 'prepare';
assert(['prepare', 'deploy', 'observe', 'refresh-renderer', 'profile-update', 'noop', 'native'].includes(mode));
const kubeconfig = process.env.ORCH_CLUSTER_KUBECONFIG_FILE;
const runName = process.env.ORCH_TEST_NAME;
assert(kubeconfig && runName && /^[a-z0-9-]+$/.test(runName));
const stateFile = join(root, 'product.json');
const state = existsSync(stateFile) ? JSON.parse(readFileSync(stateFile, 'utf8')) : { runName };
assert.equal(state.runName, runName);
const save = () => writeFileSync(stateFile, JSON.stringify(state, null, 2), { mode: 0o600 });
const native = args => execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { encoding: 'utf8', timeout: 30000, stdio: ['pipe', 'pipe', 'pipe'] });
const ui = await openConsole(config);
const { page } = ui;
const api = async (path, method = 'GET', data) => {
 const r = await page.request.fetch(config.baseURL + '/api/v1' + path, { method, ...(data === undefined ? {} : { data }), timeout: 600000 });
 assert(r.ok(), `${method} ${path}: ${r.status()}`);
 return r.status() === 204 ? undefined : r.json();
};
const pending = (path, method) => page.waitForResponse(r => new URL(r.url()).pathname === '/api/v1' + path && r.request().method() === method, { timeout: 600000 });
const click = async (path, method, button) => { const wait = pending(path, method); await button.click(); const r = await wait; assert(r.ok(), `${method} ${path}: ${r.status()}`); return r.status() === 204 ? undefined : r.json(); };
try {
 let app = (await api('/applications')).applications.find(a => a.key === state.app);
 if (!state.app) {
  assert.equal(mode, 'prepare');
  assert(!(await api('/applications')).applications.some(a => a.name === runName), 'Unrecorded same-name Application; inspect before adoption');
  await page.goto(config.baseURL + '/ui/applications/new');
  await page.getByLabel('Tên ứng dụng', { exact: true }).fill(runName);
  await click('/applications', 'POST', page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }));
  app = (await api('/applications')).applications.find(a => a.name === runName); assert(app?.key); state.app = app.key; save();
 }
 assert.equal(app?.key, state.app);
 const base = `/applications/${app.key}/environments/staging`;
 if (mode === 'refresh-renderer') {
  // A rebuilt backend has a new immutable renderer binary pin. Reinspect the
  // same task-owned content and select the resulting Definition revision.
  const files = {}, dir = resolve('backend/template-sources/standard');
  const read = folder => { for (const entry of readdirSync(folder, { withFileTypes: true })) { const path=join(folder,entry.name); if(entry.isDirectory())read(path); else files[relative(dir,path)]=readFileSync(path,'utf8'); } }; read(dir);
  const source=(await api('/template-sources/inline','POST',{name:'obs35-workload',files})).templateSource;
  assert.equal(source.source.contentSha256,state.templateDigest,'Task content changed; inspect before upgrade');
  const def=(await api('/resource-definitions')).resourceDefinitions.find(d=>d.key==='obs35-workload');assert(def?.criteria.some(c=>c.app_id===state.app));
  const inputs=structuredClone(def.driverInputs);inputs.values.variables.template_source=source.id;
  if(def.driverInputs.values.variables.template_source!==source.id) await api('/resource-definitions/obs35-workload','PUT',{key:def.key,name:def.name,resourceType:def.resourceType,driverType:def.driverType,driverInputs:inputs,criteria:def.criteria,version:def.version});
  const history=(await api('/resource-definitions/obs35-workload/versions')).history;
  const targetRevision=history.versions.find(v=>v.definition.driverInputs.values.variables.template_source===source.id)?.id;assert(targetRevision);
  const selection=await api(base+'/definition-versions');
  await api(base+'/definition-versions','PUT',{definitionKey:def.key,revisionId:targetRevision,expectedRevision:selection.revision,expectedEnvironmentVersion:selection.environmentVersion});
  state.templateSourceId=source.id;save();console.log('PASS reinspection of unchanged template content for current runtime binary');
 }
 if (mode === 'profile-update' || mode === 'native') {
  const previous=await api(base+'/preview','POST',{});
  if(mode==='profile-update'){
   const connection=(await api('/monitoring/connections')).connections.find(c=>c.id===state.connection);assert(connection);
   if(connection.profile.version!=='development-http-v2'){
    await page.goto(config.baseURL+'/ui/platform/settings');
    const section=page.getByRole('region',{name:'Kết nối giám sát',exact:true});
    await section.locator('.section-header').filter({hasText:'Owned kind observability'}).getByRole('button',{name:'Sửa',exact:true}).click();
    await section.getByText('Profile thu thập',{exact:true}).click();
    const profile=structuredClone(connection.profile);profile.version='development-http-v2';profile.defaults.metrics.find(m=>m.key==='failure').alert.severity='critical';
    profile.defaults.metrics.push({key:'failure_warning',metric:'fixture_failure',aggregation:'gauge',unit:'count',alert:{above:0,forSeconds:15,severity:'warning'}});
    profile.defaults.metrics.push({key:'error_ratio',metric:'fixture_requests_total',matchers:{code:'5..'},aggregation:'rate_ratio',denominatorMetric:'fixture_requests_total',minimumDenominator:0.01,unit:'ratio',alert:{above:0.05,forSeconds:15,severity:'warning'}});
    await section.locator('#monitor-profile').fill(JSON.stringify(profile,null,2));
    await click('/monitoring/connections/'+state.connection,'PUT',section.getByRole('button',{name:'Lưu kết nối',exact:true}));
   }
   await page.goto(config.baseURL+'/ui/platform/settings');const section=page.getByRole('region',{name:'Kết nối giám sát',exact:true});
   const facts=await click('/monitoring/connections/'+state.connection+'/test','POST',section.getByRole('button',{name:'Kiểm tra',exact:true}));assert(facts.connectivity&&facts.cluster&&facts.metrics);
  }
  await page.goto(config.baseURL+`/ui/applications/${app.key}/settings?environment=staging`);
  const section=page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:'Giám sát môi trường',exact:true})});
  await section.getByLabel('Nguồn metrics',{exact:true}).selectOption(mode==='native'?'native':'prometheus');
  await section.getByLabel('Nguồn logs',{exact:true}).selectOption(mode==='native'?'native':'loki');
  state.binding=await click(base+'/monitoring/binding','PUT',section.getByRole('button',{name:'Gắn kết nối',exact:true}));save();
  const rejected=await page.request.post(config.baseURL+'/api/v1'+base+'/deploy',{data:{token:previous.token}});assert.equal(rejected.status(),409,'Old preview was admitted after monitoring change');
  const current=await api(base+'/preview','POST',{});assert(current.changes.some(c=>c.workloadId==='gateway-demo'&&c.action==='UPDATE'));
  state.monitoringUpdatePreview=current;save();console.log('PASS monitoring-only UPDATE and stale-preview fencing through Console/API');
 }
 if(mode==='noop'){
  const preview=await api(base+'/preview','POST',{});assert.equal(preview.changes.length,0);console.log('PASS unchanged configuration has no pending deploy');
 }
 if (mode === 'prepare') {
  const accounts = (await api('/driver-accounts')).accounts;
  const account = accounts.find(a => a.name === 'obs35-kind'); assert.equal(account?.status, 'READY'); state.account = account.key;
  const defs = (await api('/resource-definitions')).resourceDefinitions;
  const ensure = async body => { const old = defs.find(d => d.key === body.key); if (old) { assert(old.criteria.some(c => c.app_id === app.key), 'Definition scope changed'); return old; } return api('/resource-definitions', 'POST', body); };
  await ensure({ key: 'obs35-cluster', name: 'Observability test cluster', resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: account.key, driverInputs: { values: { variables: {} } }, criteria: [{ app_id: app.key, class: 'default' }] });
  const pin = (await api('/resource-definition-capabilities')).capabilities.find(c => c.resourceType === 'k8s-namespace' && c.driverType === 'terraform')?.source;
  assert(pin, 'Packaged namespace Terraform source required');
  await ensure({ key: 'obs35-namespace', name: 'Observability test namespace', resourceType: 'k8s-namespace', driverType: 'terraform', driverInputs: { values: { source: { source_id: pin.sourceId, module_id: pin.moduleId, revision: pin.revision, subdirectory: pin.subdirectory, content_sha256: pin.contentSha256 }, variables: { name: '${context.env.namespace}' }, input_mapping: {}, output_mapping: { name: 'name' }, provider_bindings: { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.default#environments.@app.@env' } } } }, criteria: [{ app_id: app.key }] });
  if (!defs.some(d => d.key === 'obs35-workload')) {
   const files = {}, dir = resolve('backend/template-sources/standard');
   const read = folder => { for (const entry of readdirSync(folder, { withFileTypes: true })) { const path = join(folder, entry.name); if (entry.isDirectory()) read(path); else files[relative(dir, path)] = readFileSync(path, 'utf8'); } }; read(dir);
   const source = await api('/template-sources/inline', 'POST', { name: 'obs35-workload', files });
   state.templateSourceId = source.templateSource.id; state.templateDigest = source.templateSource.source.contentSha256; save();
   await ensure({ key: 'obs35-workload', name: 'Observability workload', resourceType: 'workload', driverType: 'template', driverInputs: { values: { variables: { template_source: source.templateSource.id, template_inputs: {}, namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}" } } }, criteria: [{ app_id: app.key }] });
  }
  const connections = (await api('/monitoring/connections')).connections;
  const connection = connections.find(c => c.id === state.connection) ?? connections.find(c => c.name === 'Owned kind observability'); assert(connection); state.connection = connection.id; save();
  if (!state.profileSaved) {
   await page.goto(config.baseURL + '/ui/platform/settings');
   const section = page.getByRole('region', { name: 'Kết nối giám sát', exact: true });
   const row = section.locator('.section-header').filter({ hasText: 'Owned kind observability' });
   await row.getByRole('button', { name: 'Sửa', exact: true }).click();
   await section.getByText('Nâng cao', { exact: true }).click(); await section.locator('#monitor-ruleLabels').fill(JSON.stringify({ release: 'obs-metrics' }));
   await section.getByText('Profile thu thập', { exact: true }).click();
   const profile = { schemaVersion: 1, version: 'development-http-v1', defaults: { port: 'http', path: '/metrics', intervalSeconds: 30, sampleLimit: 500, metrics: [ { key: 'failure', metric: 'fixture_failure', aggregation: 'gauge', unit: 'count', alert: { above: 0, forSeconds: 15, severity: 'warning' } }, { key: 'requests', metric: 'fixture_requests_total', aggregation: 'rate', unit: 'per_second' }, { key: 'latency_p95', metric: 'fixture_request_duration_seconds_bucket', aggregation: 'p95', unit: 'seconds' } ] } };
   await section.locator('#monitor-profile').fill(JSON.stringify(profile, null, 2));
   await section.getByText('Cấu hình cảnh báo', { exact: true }).click(); await section.getByLabel('Thời gian thiếu replicas', { exact: true }).fill('30');
   await click('/monitoring/connections/' + connection.id, 'PUT', section.getByRole('button', { name: 'Lưu kết nối', exact: true }));
   state.profileSaved = true; save();
  }
  await page.goto(config.baseURL + '/ui/platform/settings');
  const connectionSection = page.getByRole('region', { name: 'Kết nối giám sát', exact: true });
  const result = await click(`/monitoring/connections/${connection.id}/test`, 'POST', connectionSection.getByRole('button', { name: 'Kiểm tra', exact: true }));
  assert(result.connectivity && result.cluster && result.metrics); state.coverage = result; save();
  if (!state.binding) {
   await page.goto(config.baseURL + `/ui/applications/${app.key}/settings?environment=staging`);
   const section = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Giám sát môi trường', exact: true }) });
   await section.getByLabel('Kết nối giám sát', { exact: true }).selectOption(connection.id);
   await section.getByLabel('Nguồn metrics', { exact: true }).selectOption('prometheus'); await section.getByLabel('Nguồn logs', { exact: true }).selectOption('loki');
   state.binding = await click(base + '/monitoring/binding', 'PUT', section.getByRole('button', { name: 'Gắn kết nối', exact: true })); save();
  }
  assert.equal((await api(base + '/monitoring')).state, 'NOT_DEPLOYED');
  await page.screenshot({ path: join(root, 'predeploy-binding.png'), fullPage: true }); console.log('PASS real connection, profile and predeploy binding');
 }
 if (mode === 'deploy') {
  assert(state.binding);
  if (!state.workloadSaved) {
   await page.goto(config.baseURL + `/ui/applications/${app.key}/environments/staging/workloads/new`);
   const custom = page.getByRole('button', { name: /Tự cấu hình/ }); if (await custom.count()) await custom.click();
   await page.getByRole('button', { name: 'Nhập Workload', exact: true }).click();
   const workload = { apiVersion: 'orchestrator.dev/workload/v1', metadata: { name: 'gateway-demo' }, replicas: 1, containers: { main: { image: 'observability-fixture:issue35', ports: [{ name: 'http', containerPort: 8080 }], readinessProbe: { path: '/health', port: 8080 } } }, service: { ports: { http: { port: 8080, targetPort: 8080 } } }, workloadExecution: { version: 1, kind: 'Deployment' } };
   const input = join(root, 'workload.json'); writeFileSync(input, JSON.stringify(workload), { mode: 0o600 }); await page.getByLabel('File Workload', { exact: true }).setInputFiles(input);
   await page.getByText('Đã phân tích Workload: gateway-demo', { exact: false }).waitFor();
   await click(base + '/workloads/gateway-demo', 'PUT', page.getByRole('button', { name: 'Lưu draft Workload', exact: true })); state.workloadSaved = true; save();
  }
  assert(!state.deployPending, 'Previous deployment outcome must be inspected before retry');
  await page.goto(config.baseURL + `/ui/applications/${app.key}?environment=staging`);
  const preview = await click(base + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true })); assert(preview.changes.some(c => c.workloadId === 'gateway-demo'));
  const details = await api(`/applications/${app.key}`); const actual = details.application ?? details; const env = actual.environments?.find(e => e.key === 'staging'); assert(env?.namespaceIdentity);
  const defaultCD = await api('/delivery-default'); const binding = await api(`/delivery-bindings/${defaultCD.bindingId}?version=${defaultCD.bindingVersion}`);
  const values = binding.config; assert(values?.serviceAccountMode);
  state.namespace = env.namespaceIdentity; state.deliverySubject = values.serviceAccountMode === 'NATIVE' ? { mode: 'NATIVE' } : { name: values.serviceAccount, namespace: values.agentNamespace }; save();
  state.deployPending = true; save(); const wait = pending(base + '/deploy', 'POST'); void wait.catch(() => {});
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  if (values.serviceAccountMode !== 'NATIVE') {
   const accessFile = process.env.ORCH_TEST_ACCESS_FILE; assert(accessFile);
   const setup = { platform: { account: { context: native(['config', 'current-context']).trim() }, delivery: { namespaceAccess: { enabled: true, rulesFile: accessFile } } }, runtime: { profile: 'dev-test', clusterKubeconfigFile: kubeconfig, timeoutMs: 300000 }, application: { environment: 'staging' } };
   await ensureNamespaceAccess(ui, setup, { ...app, environments: actual.environments }, { state, save, record() {} });
  }
  const response = await wait; const result = await response.json(); state.lastDeployment = { status: response.status(), result }; state.deployPending = false; save(); assert(response.ok() && result.status === 'SUCCEEDED', `Deployment status ${response.status()}: ${result.status ?? result.code}`);
  await page.screenshot({ path: join(root, 'deployed.png'), fullPage: true }); console.log('PASS actual Console deployment through Git/Fleet');
 }
 if (mode === 'observe') {
  const view = await api(base + '/monitoring'); assert.equal(view.source, 'prometheus'); assert(view.workloads.some(w => w.workload === 'gateway-demo'));
  state.monitoring = view; save(); console.log(JSON.stringify({ state: view.state, workloads: view.workloads.map(w => ({ workload: w.workload, state: w.state, configuration: w.configuration, profile: w.profileVersion })) }));
  await page.goto(config.baseURL + `/ui/applications/${app.key}?environment=staging`); await page.getByRole('heading', { name: 'Giám sát Kubernetes', exact: true }).waitFor(); await page.screenshot({ path: join(root, 'monitoring.png'), fullPage: true });
 }
} catch (e) { await page.screenshot({ path: join(root, 'failure.png'), fullPage: true }).catch(() => {}); throw e; }
finally { await closeConsole(ui, config); }
