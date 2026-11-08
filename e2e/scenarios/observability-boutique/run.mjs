// Issue #35 stage 6: observability acceptance for Online Boutique through Console UI.
// Business writes use browser controls only; API GETs and response observation are
// technical assertions. See README.md for prerequisites, side effects and cleanup.
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';

const mode = process.argv[2];
const modes = ['connect', 'bind', 'observe', 'oom', 'restore', 'receipts', 'native', 'unbind'];
if (!modes.includes(mode)) throw Error(`mode required: ${modes.join('|')}`);
const dir = process.env.ORCH_TEST_RUN_DIR; if (!dir) throw Error('ORCH_TEST_RUN_DIR required');
outsideRepo(dir);
const stateFile = dir + '/state.json', state = JSON.parse(readFileSync(stateFile, 'utf8'));
const obsFile = dir + '/observability.json', obs = existsSync(obsFile) ? JSON.parse(readFileSync(obsFile, 'utf8')) : {};
const save = () => writeFileSync(obsFile, JSON.stringify(obs, null, 2), { mode: 0o600 });
const inputs = JSON.parse(readFileSync(dir + '/observability-input.json', 'utf8'));
const app = state.app.key, base = `/applications/${app}/environments/staging`;
const c = readConsoleConfig(), s = await openConsole(c, { role: 'ADMIN' }), ui = makeUI(s.page, c.baseURL, 1200000), { page } = ui;
const shot = name => page.screenshot({ path: `${dir}/${name}.png`, fullPage: true });
const kubectl = (...args) => execFileSync('kubectl', ['--kubeconfig', process.env.ORCH_CLUSTER_KUBECONFIG, '--context', 'kind-k8s-4f', ...args], { encoding: 'utf8' });
const namespace = () => obs.namespace ??= [...new Set(JSON.parse(kubectl('get', 'pods', '-A', '-l', `orchestrator.io/application=${app},orchestrator.io/environment=staging`, '-o', 'json')).items.map(p => p.metadata.namespace))][0];

async function connectionSection() {
  await page.goto(c.baseURL + '/ui/platform/settings');
  const section = page.getByRole('region', { name: 'Kết nối giám sát', exact: true });
  await section.waitFor(); return section;
}
async function bindingSection() {
  await page.goto(c.baseURL + `/ui/applications/${app}/settings?environment=staging`);
  const section = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Giám sát môi trường', exact: true }) });
  await section.waitFor(); return section;
}
async function bind(source) {
  const section = await bindingSection();
  if (source !== 'native') await section.getByLabel('Kết nối giám sát', { exact: true }).selectOption(obs.connection);
  await section.getByLabel('Nguồn metrics', { exact: true }).selectOption(source === 'native' ? 'native' : 'prometheus');
  await section.getByLabel('Nguồn logs', { exact: true }).selectOption(source === 'native' ? 'native' : 'loki');
  return ui.submit(base + '/monitoring/binding', 'PUT', section.getByRole('button', { name: 'Gắn kết nối', exact: true }));
}
// Preview and one Deploy through the application page; waits for the exact operation.
async function previewDeploy(label) {
  await page.goto(c.baseURL + `/ui/applications/${app}?environment=staging`);
  const preview = await ui.submit(base + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  const changes = (preview.changes ?? []).map(x => ({ workload: x.workloadId, action: x.action }));
  console.log(label, 'Preview', JSON.stringify(changes)); obs[label] = { changes }; save();
  if (!changes.length) return 'NOOP';
  let request;
  page.on('request', r => { if (r.method() === 'POST' && new URL(r.url()).pathname === '/api/v1' + base + '/deploy') request = r.postDataJSON()?.reviewRequestId; });
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  const deadline = Date.now() + 1200000;
  for (;;) {
    if (request) {
      const op = (await ui.get(base + '/operations?reviewRequestId=' + encodeURIComponent(request))).operations?.find(o => o.reviewRequestId === request);
      if (op && ['SUCCEEDED', 'FAILED', 'RECOVERED', 'INTERRUPTED'].includes(op.status)) { obs[label].operation = op.status; save(); console.log(label, 'operation', op.status); return op.status; }
    }
    if (Date.now() > deadline) throw Error(label + ': operation still active; inspect before retry');
    await page.waitForTimeout(3000);
  }
}
async function monitoring() { return ui.get(base + '/monitoring'); }
async function openWorkloadEditor(name) {
  await page.goto(c.baseURL + `/ui/applications/${app}?environment=staging`);
  await page.getByRole('link', { name, exact: true }).or(page.getByRole('button', { name, exact: true })).first().click();
  await page.getByLabel('Tên Workload', { exact: true }).waitFor();
}
async function setCartMemoryLimit(value) {
  await openWorkloadEditor('cart');
  while (await page.locator('button[data-workload-advanced][aria-expanded=false]').count()) await page.locator('button[data-workload-advanced][aria-expanded=false]').first().click();
  const compute = page.locator('details.runtime-config-group').filter({ has: page.locator('summary strong').getByText('Tài nguyên tính toán', { exact: true }) });
  if (await compute.getAttribute('open') === null) await compute.locator('summary').first().click();
  await compute.getByLabel('Giới hạn bộ nhớ', { exact: true }).fill(value);
  if (/^\d+Mi$/.test(value) && parseInt(value) < 70) await compute.getByLabel('Yêu cầu bộ nhớ', { exact: true }).fill(value);
  else await compute.getByLabel('Yêu cầu bộ nhớ', { exact: true }).fill(inputs.cartMemoryRequest);
  await ui.submit(base + '/workloads/cart', 'PUT', page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
}
const receipts = () => JSON.parse(kubectl('get', '--raw', '/api/v1/namespaces/orch-observability/services/http:observability-test-receiver:8080/proxy/notifications'));

try {
  if (mode === 'connect') {
    const existing = (await ui.get('/monitoring/connections')).connections.find(x => x.name === inputs.name);
    const section = await connectionSection();
    if (!existing) {
      await section.getByRole('button', { name: 'Kết nối mới', exact: true }).click();
      for (const [label, value] of [['Tên', inputs.name], ['Prometheus endpoint', inputs.prometheusUrl], ['Loki endpoint', inputs.lokiUrl], ['Alertmanager endpoint', inputs.alertmanagerUrl], ['Grafana URL', inputs.grafanaUrl], ['UID cluster', inputs.clusterUid]])
        await section.getByLabel(label, { exact: true }).fill(value);
      await section.getByText('Nâng cao', { exact: true }).click();
      await section.getByLabel('Dashboard UID', { exact: true }).fill(inputs.dashboardUid);
      assert.deepEqual(JSON.parse(await section.getByLabel('Nhãn chọn rules (JSON)', { exact: true }).inputValue()), { release: 'orch-observability' });
      const saved = await ui.submit('/monitoring/connections', 'POST', section.getByRole('button', { name: 'Lưu kết nối', exact: true }));
      obs.connection = saved.connection?.id ?? saved.id; save(); console.log('Connection saved via UI');
    } else { obs.connection = existing.id; save(); console.log('Connection reused', inputs.name); }
    const row = section.locator('.section-header').filter({ hasText: inputs.name });
    const facts = await ui.submit(`/monitoring/connections/${obs.connection}/test`, 'POST', row.getByRole('button', { name: 'Kiểm tra', exact: true }));
    obs.connectionTest = facts; save(); console.log('Connection test', JSON.stringify({ connectivity: facts.connectivity, cluster: facts.cluster, metrics: facts.metrics, configSync: facts.configSync }));
    assert(facts.connectivity && facts.cluster, 'connection or cluster identity not verified');
    await shot('connection'); console.log('PASS connection created and tested through Console UI');
  }
  if (mode === 'bind') {
    obs.binding = await bind('external'); save();
    const view = await ui.get(base + '/monitoring/binding'); console.log('Binding', JSON.stringify({ metrics: view.binding?.metricsSource ?? view.metricsSource, logs: view.binding?.logsSource ?? view.logsSource }));
    await shot('binding'); console.log('PASS environment bound before deploy through Console UI');
  }
  if (mode === 'observe') {
    const view = await monitoring();
    const rows = view.workloads.map(w => ({ workload: w.workload, state: w.state, configuration: w.configuration }));
    obs.observe = { at: new Date().toISOString(), state: view.state, source: view.source, rows }; save();
    console.log(JSON.stringify(obs.observe));
    assert.equal(view.source, 'prometheus');
    for (const name of inputs.expectedWorkloads) assert(rows.some(r => r.workload === name), 'missing workload ' + name);
    assert(rows.every(r => r.configuration === 'APPLIED'), 'configuration not APPLIED for all workloads');
    await page.goto(c.baseURL + `/ui/applications/${app}?environment=staging`);
    await page.getByRole('heading', { name: 'Giám sát Kubernetes', exact: true }).waitFor(); await shot('monitoring-overview');
    await page.goto(c.baseURL + `/ui/applications/${app}/environments/staging/observability?tab=logs&workload=cart`);
    await page.locator('.obs-log-table').waitFor({ timeout: 60000 });
    assert.equal(await page.locator('.obs-log-controls select').first().inputValue(), '', 'logs must follow the bound source (Loki)');
    await shot('logs-cart');
    await page.goto(c.baseURL + `/ui/applications/${app}/environments/staging/observability?tab=metrics&workload=frontend`);
    await page.waitForLoadState('networkidle'); await shot('metrics-frontend');
    console.log('PASS Console overview and Loki logs opened for the Boutique environment');
  }
  if (mode === 'oom') {
    await setCartMemoryLimit(inputs.cartOomLimit);
    obs.oomStart = new Date().toISOString(); save();
    const result = await previewDeploy('oomDeploy');
    console.log('Controlled OOM deploy finished with', result, '(failure is expected to be reported honestly)');
    await shot('oom-deploy');
  }
  if (mode === 'restore') {
    await setCartMemoryLimit(inputs.cartMemoryLimit);
    obs.restoreStart = new Date().toISOString(); save();
    const result = await previewDeploy('restoreDeploy'); assert.equal(result, 'SUCCEEDED');
    await shot('restore-deploy'); console.log('PASS cart restored through workload form and Deploy');
  }
  if (mode === 'receipts') {
    const all = receipts(), mine = (all.notifications ?? all).filter(n => JSON.stringify(n).includes(namespace() ?? '#none'));
    const summary = mine.map(n => ({ at: n.receivedAt ?? n.at, status: n.body?.status ?? n.status, alerts: (n.body?.alerts ?? n.alerts ?? []).map(a => `${a.labels?.alertname}/${a.labels?.orchestrator_workload ?? a.labels?.container ?? ''}:${a.status}`) }));
    obs.receipts = summary; save(); console.log(JSON.stringify(summary, null, 1));
    await page.goto(c.baseURL + `/ui/applications/${app}?environment=staging`); await shot('alerts-view');
  }
  if (mode === 'native') {
    obs.native = await bind('native'); save();
    const result = await previewDeploy('nativeDeploy'); assert.equal(result, 'SUCCEEDED');
    const left = kubectl('get', 'prometheusrules,podmonitors', '-n', namespace(), '-o', 'name').trim();
    assert.equal(left, '', 'app-owned monitoring objects remain'); console.log('PASS native withdrawal through Console UI');
  }
  if (mode === 'unbind') {
    const section = await bindingSection();
    await ui.submit(base + '/monitoring/binding', 'DELETE', section.getByRole('button', { name: 'Gỡ binding', exact: true }));
    console.log('PASS unbind through Console UI');
  }
} catch (e) { await shot(`failure-${mode}`).catch(() => {}); console.error(mode, 'failed:', e.message); process.exitCode = 1; }
finally { await closeConsole(s, c); }
