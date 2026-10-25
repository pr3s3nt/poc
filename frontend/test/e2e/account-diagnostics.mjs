// Real end-user browser acceptance of Infrastructure Account diagnostics
// (DG-E01..E08, ADR-022). Driven by backend/test/integration/diagnostics-e2e.sh
// against the real Compose stack and an OWNED kind cluster. Nothing here mocks
// or intercepts a product API. The browser performs every user action (sign-in,
// account onboarding, setup, runs, inspection, rerun, stop, switching, history,
// export, reload). Native actions (kubectl on the owned cluster, docker restart
// or kill of the owned backend container) are faults, repairs or independent
// observations and are recorded in fixture-actions.txt; API reads only verify
// outcomes the UI already shows.
//
// Phases (ORCH_E2E_PHASE): general | desired
import { chromium, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';

const env = (name) => { const v = process.env[name]; if (!v) throw new Error(`missing ${name}`); return v; };
const URL_BASE = env('ORCH_E2E_URL').replace(/\/+$/, '');
const EVIDENCE = env('ORCH_E2E_EVIDENCE');
const PHASE = env('ORCH_E2E_PHASE');
const HOST_KUBECONFIG = env('ORCH_E2E_HOST_KUBECONFIG');
const CONTEXT = env('ORCH_E2E_CONTEXT');
const STATE_FILE = env('ORCH_E2E_STATE');
const BACKEND_CONTAINER = env('ORCH_E2E_BACKEND_CONTAINER');
const BACKEND_URL = env('ORCH_E2E_BACKEND');
const state = existsSync(STATE_FILE) ? JSON.parse(readFileSync(STATE_FILE, 'utf8')) : {};
const saveState = () => writeFileSync(STATE_FILE, JSON.stringify(state), { mode: 0o600 });

// ---- evidence ---------------------------------------------------------------
const assertions = existsSync(`${EVIDENCE}/assertions.json`) ? JSON.parse(readFileSync(`${EVIDENCE}/assertions.json`, 'utf8')) : [];
function record(id, ok, detail) {
  assertions.push({ id, ok: Boolean(ok), detail });
  writeFileSync(`${EVIDENCE}/assertions.json`, JSON.stringify(assertions, null, 1));
  appendFileSync(`${EVIDENCE}/assertions.txt`, `${ok ? 'PASS' : 'FAIL'} ${id} ${detail}\n`);
  console.log(`${ok ? 'PASS' : 'FAIL'} ${id} ${detail}`);
  if (!ok) throw new Error(`assertion failed: ${id}: ${detail}`);
}
const fixture = (text) => appendFileSync(`${EVIDENCE}/fixture-actions.txt`, `${new Date().toISOString()} ${text}\n`);
let page;
const shot = (name) => page.screenshot({ path: `${EVIDENCE}/screenshots/${PHASE}-${name}.png`, fullPage: true });

// ---- native helpers (owned cluster / owned backend only) -----------------------
const kubectl = (...args) => execFileSync('kubectl', ['--kubeconfig', HOST_KUBECONFIG, '--context', CONTEXT, ...args], { encoding: 'utf8', timeout: 60_000 });
const kubectlStdin = (input, ...args) => execFileSync('kubectl', ['--kubeconfig', HOST_KUBECONFIG, '--context', CONTEXT, ...args], { input, encoding: 'utf8', timeout: 60_000 });
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
async function until(label, fn, { timeout = 120_000, every = 500 } = {}) {
  const end = Date.now() + timeout;
  for (;;) {
    const value = await fn();
    if (value) return value;
    if (Date.now() > end) throw new Error(`timed out waiting for ${label}`);
    await sleep(every);
  }
}
const diagNamespaces = () => JSON.parse(kubectl('get', 'namespaces', '-l', 'app.kubernetes.io/managed-by=orchestrator-diagnostics', '-o', 'json')).items.map((n) => n.metadata.name);
const diagClaims = () => JSON.parse(kubectl('get', 'persistentvolumeclaims', '-A', '-l', 'app.kubernetes.io/managed-by=orchestrator-diagnostics', '-o', 'json')).items;
const leftoverVolumes = () => JSON.parse(kubectl('get', 'pv', '-o', 'json')).items.filter((pv) => (pv.spec.claimRef?.namespace ?? '').startsWith('orch-diag-')).map((pv) => pv.metadata.name);
function provisioner(replicas) {
  fixture(`kubectl scale deploy/local-path-provisioner --replicas=${replicas}`);
  kubectl('-n', 'local-path-storage', 'scale', 'deploy/local-path-provisioner', `--replicas=${replicas}`);
  if (replicas > 0) kubectl('-n', 'local-path-storage', 'rollout', 'status', 'deploy/local-path-provisioner', '--timeout=120s');
}
function createFastClass() {
  fixture('kubectl apply StorageClass fast (rancher.io/local-path, Delete, WaitForFirstConsumer)');
  kubectlStdin(JSON.stringify({ apiVersion: 'storage.k8s.io/v1', kind: 'StorageClass', metadata: { name: 'fast' }, provisioner: 'rancher.io/local-path', reclaimPolicy: 'Delete', volumeBindingMode: 'WaitForFirstConsumer' }), 'apply', '-f', '-');
}
const healthy = async () => { try { return (await fetch(`${BACKEND_URL}/api/v1/healthz`)).ok; } catch { return false; } };

// ---- page helpers ---------------------------------------------------------------
async function signIn(user) {
  await page.goto(`${URL_BASE}/ui/sign-in`);
  await page.getByLabel('Tên đăng nhập').fill(user);
  await page.getByLabel('Mật khẩu').fill('test-password');
  await page.getByRole('button', { name: 'Đăng nhập' }).click();
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
}
const api = async (path, init) => {
  const response = await page.request.fetch(`${URL_BASE}/api/v1${path}`, init);
  return { status: response.status(), body: response.status() === 204 ? null : await response.json().catch(() => null) };
};
const reportId = () => new URL(page.url()).searchParams.get('report');
const diagTab = (name) => page.getByRole('navigation', { name }).getByRole('button', { name: 'Chẩn đoán', exact: true });
async function openAccountDiagnose(name, purposeLabel) {
  await page.goto(`${URL_BASE}/ui/platform/driver-accounts?account=${encodeURIComponent(state.accountKey)}&tab=diagnose${purposeLabel === 'use' ? '&purpose=use-case' : ''}`);
  await expect(page.getByRole('navigation', { name }).getByRole('button', { name: 'Chẩn đoán', exact: true })).toHaveAttribute('aria-current', 'page');
}
async function purpose(label) {
  await page.getByRole('group', { name: 'Phạm vi kiểm tra' }).getByRole('button', { name: label, exact: true }).click();
  await expect(page.getByRole('group', { name: 'Phạm vi kiểm tra' }).getByRole('button', { name: label, exact: true })).toHaveAttribute('aria-pressed', 'true');
}
// Every start captures the real POST 202 and its returned id BEFORE any result
// is read, so an older report on screen can never satisfy a new run.
async function start(action) {
  const posted = page.waitForResponse((r) => r.request().method() === 'POST' && /\/diagnostics$/.test(new URL(r.url()).pathname));
  await action();
  const response = await posted;
  if (response.status() !== 202) throw new Error(`start returned HTTP ${response.status()}`);
  const id = (await response.json()).id;
  await until('URL selects the new run', () => reportId() === id, { timeout: 30_000, every: 200 });
  return id;
}
async function finishedFor(id, timeout = 240_000) {
  const final = await until(`terminal report ${id.slice(0, 8)}`, async () => { const r = await api(`/driver-accounts/${state.accountKey}/diagnostics/${id}`); return ['COMPLETED', 'CANCELLED', 'INTERRUPTED'].includes(r.body?.status) ? r.body : null; }, { timeout, every: 1000 });
  if (reportId() !== id) throw new Error('the screen no longer shows the run that was started');
  await expect(page.locator('.dg-running')).toHaveCount(0, { timeout });
  await expect(page.locator('.dg-summary')).toBeVisible({ timeout });
  return final;
}
const byId = (report) => Object.fromEntries(report.findings.map((f) => [f.id, f]));
async function selectFinding(title) {
  await page.getByRole('button', { name: /^Tất cả/ }).click();
  await page.getByRole('button', { name: new RegExp(`^${title}`) }).first().click();
  return page.getByRole('complementary', { name: 'Chi tiết' });
}
async function factOf(inspector, label) {
  const details = inspector.locator('details');
  if (!(await details.getAttribute('open'))) await details.locator('summary').click();
  return (await inspector.locator('dt', { hasText: new RegExp(`^${label}$`) }).locator('xpath=following-sibling::dd[1]').innerText()).trim();
}
async function startFromConfigure({ uses, application, environment, mode, unuse = [] }) {
  return start(async () => {
    await page.getByRole('button', { name: 'Thiết lập', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    for (const use of unuse) { const box = dialog.getByLabel(use, { exact: true }); if (await box.isChecked()) await box.uncheck(); }
    for (const use of uses) { const box = dialog.getByLabel(use, { exact: true }); if (!(await box.isChecked())) await box.check(); }
    if (application) {
      await dialog.getByLabel('Ứng dụng (tùy chọn)').selectOption(application);
      await dialog.getByLabel('Môi trường', { exact: true }).selectOption(environment);
    }
    await dialog.getByLabel(mode === 'EXECUTION' ? /Thực thi/ : /Điều kiện/).check();
    await dialog.getByRole('button', { name: 'Bắt đầu kiểm tra' }).click();
    await expect(dialog).toBeHidden();
  });
}
const VI = async (context) => { await context.addInitScript(() => { try { localStorage.setItem('orchestrator.locale', 'vi'); } catch { /* none */ } }); return context; };
const openContext = (options = {}) => browser.newContext({ locale: 'vi-VN', ...options }).then(VI);

// =============================================================================
const browser = await chromium.launch();
try {
  page = await (await openContext({ viewport: { width: 1440, height: 1000 } })).newPage();
  page.setDefaultTimeout(60_000);
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e.message).slice(0, 200)));
  await signIn('platform-engineer');

  if (PHASE === 'general') {
    // DG-E01 onboarding through the real form.
    await page.getByRole('link', { name: /Tài khoản hạ tầng/ }).click();
    await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toBeVisible();
    const create = page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    await create.getByLabel('Tên tài khoản', { exact: true }).fill('Owned kind');
    const chooser = page.waitForEvent('filechooser');
    await create.locator('input[type="file"]').click();
    await (await chooser).setFiles(env('ORCH_E2E_ACCOUNT_KUBECONFIG'));
    await expect(create.getByLabel('Đích đã chọn')).toBeVisible();
    const created = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/driver-accounts/internal', { timeout: 180_000 });
    await create.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }).click();
    const response = await created;
    const account = await response.json();
    record('DG-E01.onboard', response.status() === 201 && account.status === 'READY' && account.version === 1, `HTTP ${response.status()} ${account.status} v${account.version}`);
    state.accountKey = account.key; state.accountVersion = account.version; saveState();
    await shot('account-registered');
    const table = page.getByRole('table', { name: 'Tài khoản hạ tầng' });
    await expect(table.getByText('Sẵn sàng')).toBeVisible();
    await expect(table.getByText('Chưa chẩn đoán')).toBeVisible();
    record('DG-E02.never-checked', true, 'a new account shows connection READY and "not diagnosed yet"');

    // DG-E02 general check without an application.
    await table.getByRole('button', { name: /^Xem Owned kind/ }).click();
    await diagTab('Owned kind').click();
    await expect(page.getByText('Chưa có kiểm tra chung nào')).toBeVisible();
    const generalId = await start(() => page.getByRole('button', { name: 'Chạy kiểm tra' }).first().click());
    const report = await finishedFor(generalId);
    state.generalReport = report.id; saveState();
    const f = byId(report);
    record('DG-E02.general.counts', report.counts.fail === 0 && report.counts.blocked === 0 && report.counts.unknown === 0 && report.counts.pass >= 9 && report.status === 'COMPLETED' && report.scope.purpose === 'GENERAL' && !report.scope.context,
      `pass=${report.counts.pass} fail=${report.counts.fail} unknown=${report.counts.unknown}; no application context`);
    await shot('general-result');

    // Compare reported facts with the actual cluster (source runtime facts).
    const inspector = await selectFinding('Node');
    const nativeNodes = JSON.parse(kubectl('get', 'nodes', '-o', 'json')).items;
    const nodeFact = await factOf(inspector, 'Số node');
    record('DG-E02.nodes-match-cluster', nodeFact === String(nativeNodes.length), `reported ${nodeFact}, kubectl ${nativeNodes.length}`);
    const kubelet = nativeNodes[0].status.nodeInfo.kubeletVersion;
    record('DG-E02.kubelet-matches', (await factOf(inspector, 'Phiên bản kubelet')).includes(kubelet), `kubelet ${kubelet}`);
    const api1 = await selectFinding('API Kubernetes');
    const serverVersion = JSON.parse(kubectl('version', '-o', 'json')).serverVersion.gitVersion;
    record('DG-E02.server-version-matches', (await factOf(api1, 'Phiên bản máy chủ')) === serverVersion, `server ${serverVersion}`);
    const storage = await selectFinding('Kho StorageClass');
    const nativeClasses = JSON.parse(kubectl('get', 'storageclass', '-o', 'json')).items.map((c) => c.metadata.name).sort().join(',');
    record('DG-E02.storage-matches', (await factOf(storage, 'Các StorageClass')) === nativeClasses, `classes ${nativeClasses}`);
    const tls = await selectFinding('TLS của API server');
    record('DG-E02.tls-verified', f['k8s.tls'].reason === 'TLS_VERIFIED' && (await tls.innerText()).includes('xác minh bằng CA đã cấu hình'), 'certificate verified against the configured CA');

    // DG-E02: connection and diagnostics stay separate; no account write.
    const after = await api(`/driver-accounts/${state.accountKey}`);
    record('DG-E02.connection-untouched', after.body.version === state.accountVersion && after.body.status === 'READY' && after.body.latestDiagnostic?.reportId === report.id,
      `account v${after.body.version} ${after.body.status}; latest summary references ${report.id.slice(0, 8)}`);

    // DG-E07 reload.
    await page.reload();
    await expect(page.locator('.dg-summary')).toBeVisible();
    record('DG-E07.reload-keeps-report', reportId() === report.id, `report ${report.id.slice(0, 8)} after reload`);
    await page.goto(`${URL_BASE}/ui/platform/driver-accounts`);
    const latest = page.getByRole('table', { name: 'Tài khoản hạ tầng' }).getByRole('button', { name: /Mở chẩn đoán gần nhất/ });
    await expect(latest).toContainText('Kiểm tra chung');
    record('DG-E02.list-latest', true, 'the list shows the latest result with scope and time');
    record('no-page-errors', errors.length === 0, errors.join('; ') || 'none');
  }

  if (PHASE === 'desired') {
    await openAccountDiagnose('Owned kind', 'general');
    await purpose('Theo use case');
    const baseline = (await api(`/driver-accounts/${state.accountKey}`)).body;

    // DG-E03/E04: genuine pre-deploy draft, required StorageClass missing.
    record('DG-E04.class-absent', !JSON.parse(kubectl('get', 'storageclass', '-o', 'json')).items.some((c) => c.metadata.name === 'fast'), 'owned cluster has no StorageClass "fast" (start state)');
    await expect(page.getByText('Chưa có kiểm tra theo use case')).toBeVisible();
    const firstId = await startFromConfigure({ uses: ['Cơ sở dữ liệu'], application: state.application, environment: state.environment, mode: 'CONDITIONS' });
    const first = await finishedFor(firstId);
    let f = byId(first);
    record('DG-E03.draft-planned', f['uc.preview.requirements']?.verdict === 'PASS' && f['uc.preview.requirements'].reason === 'PLAN_RESOLVED',
      'a saved, never deployed draft was planned (' + JSON.stringify(f['uc.preview.requirements']?.facts) + ')');
    record('DG-E04.fault-observed', f['k8s.storage.requirements']?.verdict === 'FAIL' && f['k8s.storage.requirements'].reason === 'STORAGE_CLASS_MISSING' && f['uc.database.requirements']?.verdict === 'BLOCKED' && f['uc.database.requirements'].reason === 'PREREQUISITE_FAILED' && f['k8s.storage.requirements'].facts.requiredClasses === 'fast',
      'FAIL STORAGE_CLASS_MISSING for the exact class fast; the database requirement is BLOCKED by that failed prerequisite');
    const ctx = first.scope.context;
    record('DG-E03.versions-pinned', ctx.applicationVersion >= 1 && ctx.environmentVersion >= 1 && ctx.draftVersion >= 1 && ctx.definitions?.some((d) => d.key === 'diag-postgres'), `pins app=${ctx.applicationVersion} env=${ctx.environmentVersion} drafts=${ctx.draftVersion} defs=${ctx.definitions?.length}`);
    await expect(page.getByText('Cần sửa trước khi dùng')).toBeVisible();
    const fault = await selectFinding('Yêu cầu lưu trữ của cấu hình');
    await expect(fault).toContainText('StorageClass mà cấu hình yêu cầu không tồn tại');
    await expect(fault).toContainText('kubectl apply');
    record('DG-E04.remediation-concrete', true, 'the inspector shows the concrete remediation steps');
    await shot('fault-storage-class-missing');
    state.faultReport = first.id; saveState();

    // Repair on the owned cluster, rerun through the UI.
    createFastClass();
    const repairedId = await start(() => page.getByRole('button', { name: 'Chạy lại' }).click());
    const repaired = await finishedFor(repairedId);
    f = byId(repaired);
    record('DG-E04.repair-rerun', repaired.id !== first.id && f['k8s.storage.requirements']?.verdict === 'PASS' && f['uc.database.requirements']?.verdict === 'PASS' && repaired.counts.fail === 0,
      `rerun ${repaired.id.slice(0, 8)} passes after the StorageClass was created`);
    await shot('repaired');
    await page.getByRole('button', { name: /^Lịch sử/ }).click();
    const rows = page.getByRole('table', { name: 'Lịch sử' }).locator('tbody tr');
    await expect(rows).toHaveCount(2);
    await page.getByRole('button', { name: `Mở báo cáo ${first.id.slice(0, 8)}` }).click();
    await expect(page.getByText('Cần sửa trước khi dùng')).toBeVisible();
    record('DG-E06.history-keeps-old-failure', true, 'the earlier FAIL report is still reachable and unchanged');

    // A genuinely failing provisioner is observed, then repaired.
    provisioner(0);
    const brokenId = await start(() => page.getByRole('button', { name: 'Chạy lại' }).click());
    const broken = await finishedFor(brokenId);
    record('DG-E04.provisioner-fault', byId(broken)['k8s.storage.provisioner']?.verdict === 'FAIL' && byId(broken)['k8s.storage.provisioner'].reason === 'PROVISIONER_NOT_READY', 'FAIL PROVISIONER_NOT_READY with the controller scaled to zero');
    provisioner(1);

    // DG-E05 execution: exact class PVC, observed natively, then verified cleanup.
    const claimClasses = new Set();
    let watching = true;
    const watcher = (async () => { while (watching) { try { diagClaims().forEach((c) => claimClasses.add(c.spec.storageClassName)); } catch { /* cluster busy */ } await sleep(400); } })();
    const executionId = await start(async () => {
      await page.getByRole('button', { name: 'Thiết lập', exact: true }).click();
      await page.getByRole('dialog').getByLabel(/Thực thi/).check();
      await page.getByRole('dialog').getByRole('button', { name: 'Bắt đầu kiểm tra' }).click();
    });
    const executed = await finishedFor(executionId, 300_000);
    watching = false; await watcher;
    f = byId(executed);
    record('DG-E05.exact-class-pvc', f['k8s.exec.volume']?.verdict === 'PASS' && f['k8s.exec.volume'].facts.classesTested === 'fast' && claimClasses.has('fast'),
      `volume probe PASS on class fast; claim classes observed natively: ${[...claimClasses].join(',')}`);
    record('DG-E05.cleanup-verified', executed.cleanup.status === 'VERIFIED' && f['k8s.exec.cleanup']?.verdict === 'PASS' && diagNamespaces().length === 0 && leftoverVolumes().length === 0,
      `report cleanup ${executed.cleanup.status}; native namespaces=${diagNamespaces().length} volumes=${leftoverVolumes().length}`);
    await expect(page.locator('.dg-cleanup-note')).toContainText('xác minh');
    await shot('execution-cleanup');
    state.executionReport = executed.id; saveState();

    // DG-E06 cancellation: healthy provisioner, so the probe objects really exist.
    // Stop as soon as the run-owned namespace is observed natively, then switch
    // scope while the cancellation and cleanup are still in flight.
    const cancelId = await start(() => page.getByRole('button', { name: 'Chạy lại' }).click());
    await until('probe namespace', () => diagNamespaces().length > 0, { timeout: 120_000, every: 100 });
    const midRun = (await api(`/driver-accounts/${state.accountKey}/diagnostics/${cancelId}`)).body;
    record('DG-E06.cancel-boundary', midRun.status === 'RUNNING' || midRun.status === 'CANCELLING', `report ${midRun.status} while the native namespace exists`);
    await page.getByRole('button', { name: 'Dừng', exact: true }).click();
    await purpose('Kiểm tra chung');
    await expect(page.locator('.ac-report-scope')).toContainText('Kiểm tra chung');
    record('DG-E06.switch-purpose-isolated', reportId() !== cancelId, 'the general view does not show the use-case run that is being cancelled');
    await purpose('Theo use case');
    await until('use-case view selects the cancelled run again', () => reportId() === cancelId, { timeout: 30_000, every: 200 });
    const stopped = await finishedFor(cancelId, 300_000);
    record('DG-E06.cancelled-truthfully', stopped.status === 'CANCELLED' && stopped.cleanup.status === 'VERIFIED' && diagNamespaces().length === 0 && leftoverVolumes().length === 0 && stopped.counts.pending > 0,
      `status ${stopped.status}, cleanup ${stopped.cleanup.status}, pending=${stopped.counts.pending}; native clean`);
    await expect(page.getByText('Lượt kiểm tra đã dừng')).toBeVisible();
    await shot('cancelled');
    const prior = await api(`/driver-accounts/${state.accountKey}/diagnostics/${executed.id}`);
    record('DG-E06.previous-report-preserved', prior.body.status === 'COMPLETED' && prior.body.counts.pass === executed.counts.pass, 'the earlier completed report is unchanged by the cancelled run');

    // DG-E08/limits: honest evidence limits for secrets and HTTPS (no execution).
    const limitsId = await startFromConfigure({ uses: ['Bí mật', 'HTTPS'], unuse: ['Cơ sở dữ liệu'], application: state.application, environment: state.environment, mode: 'CONDITIONS' });
    const limits = await finishedFor(limitsId);
    f = byId(limits);
    const store = f['uc.secrets.store']?.verdict; const workload = f['uc.secrets.workload'];
    // The worker blocks only on FAIL/BLOCKED prerequisites: a failed store blocks
    // the workload check; an UNKNOWN or PASS store still leaves the workload-side
    // login honestly UNKNOWN. It is never reported ready.
    const branch = store === 'FAIL' || store === 'BLOCKED' ? workload?.verdict === 'BLOCKED' : workload?.verdict === 'UNKNOWN' && workload.reason === 'WORKLOAD_AUTH_UNOBSERVABLE';
    record('DG-E08.workload-auth-never-pass', Boolean(branch), `store ${store}/${f['uc.secrets.store']?.reason}; workload ${workload?.verdict}/${workload?.reason}`);
    record('DG-E08.https-not-faked', f['uc.https.certificate'] && f['uc.https.certificate'].verdict !== 'PASS', `public certificate ${f['uc.https.certificate']?.verdict}/${f['uc.https.certificate']?.reason}`);
    record('DG-E08.no-unrelated-plan', !f['uc.preview.requirements'] && !f['k8s.exec.volume'], 'a secrets/HTTPS selection plans neither the desired state nor any probe object');
    record('DG-E08.limits-visible', (await page.locator('.ac-limits').innerText()).includes('Chế độ điều kiện không chứng minh'), 'report limits are visible');
    await shot('secrets-https-limits');

    // DG-E07 backend restart with the same database.
    const before = (await api(`/driver-accounts/${state.accountKey}/diagnostics?purpose=USE_CASE&limit=100`)).body.reports.map((r) => r.reportId);
    fixture(`docker restart ${BACKEND_CONTAINER.slice(0, 12)}`);
    execFileSync('docker', ['restart', BACKEND_CONTAINER], { timeout: 120_000 });
    await until('backend healthy', healthy, { timeout: 120_000, every: 1000 });
    await page.reload();
    await expect(page.locator('.dg-summary')).toBeVisible();
    await page.getByRole('button', { name: /^Lịch sử/ }).click();
    const afterIds = (await api(`/driver-accounts/${state.accountKey}/diagnostics?purpose=USE_CASE&limit=100`)).body.reports.map((r) => r.reportId);
    record('DG-E07.restart-keeps-history', JSON.stringify(before) === JSON.stringify(afterIds) && afterIds.length >= 5, `${afterIds.length} reports before and after a backend restart`);

    // DG-E07 hard kill during an in-flight execution run: honest INTERRUPTED + recovery.
    await purpose('Theo use case');
    const killedId = await startFromConfigure({ uses: ['Cơ sở dữ liệu'], unuse: ['Bí mật', 'HTTPS'], application: state.application, environment: state.environment, mode: 'EXECUTION' });
    await until('probe namespace', () => diagNamespaces().length > 0, { timeout: 120_000, every: 100 });
    fixture(`docker kill ${BACKEND_CONTAINER.slice(0, 12)} (SIGKILL during an in-flight execution run)`);
    execFileSync('docker', ['kill', BACKEND_CONTAINER]);
    execFileSync('docker', ['start', BACKEND_CONTAINER]);
    await until('backend healthy', healthy, { timeout: 120_000, every: 1000 });
    const recovered = await until('interrupted report with verified cleanup', async () => {
      const r = await api(`/driver-accounts/${state.accountKey}/diagnostics/${killedId}`);
      return r.body?.status === 'INTERRUPTED' && r.body.cleanup.status === 'VERIFIED' ? r.body : null;
    }, { timeout: 300_000, every: 3000 });
    record('DG-E07.interrupted-and-recovered', recovered.counts.pending > 0 && diagNamespaces().length === 0 && leftoverVolumes().length === 0,
      `INTERRUPTED with ${recovered.findings.length} preserved findings; cleanup recovered and natively verified`);
    await page.reload();
    await expect(page.getByText('Lượt kiểm tra bị gián đoạn')).toBeVisible();
    await shot('interrupted-recovered');

    // DG-E06 second account: results never cross accounts.
    await page.goto(`${URL_BASE}/ui/platform/driver-accounts`);
    const create = page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    await create.getByLabel('Tên tài khoản', { exact: true }).fill('Owned kind B');
    const chooser = page.waitForEvent('filechooser');
    await create.locator('input[type="file"]').click();
    await (await chooser).setFiles(env('ORCH_E2E_ACCOUNT_KUBECONFIG'));
    await expect(create.getByLabel('Đích đã chọn')).toBeVisible();
    await create.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }).click();
    await expect(page.getByRole('table', { name: 'Tài khoản hạ tầng' }).getByText('Owned kind B')).toBeVisible();
    await page.getByRole('table', { name: 'Tài khoản hạ tầng' }).getByRole('button', { name: /^Mở chẩn đoán gần nhất/ }).first().click();
    await diagTab('Owned kind').click();
    await expect(page.locator('.dg-summary')).toBeVisible();
    await page.getByRole('table', { name: 'Tài khoản hạ tầng' }).getByRole('button', { name: /^Xem Owned kind B/ }).click();
    await diagTab('Owned kind B').click();
    await expect(page.getByText('Chưa có kiểm tra chung nào')).toBeVisible();
    record('DG-E06.account-isolation', true, 'the second account shows no result of the first');

    // Export from the browser keeps scope, versions and limits.
    await page.goto(`${URL_BASE}/ui/platform/driver-accounts?account=${encodeURIComponent(state.accountKey)}&tab=diagnose&purpose=use-case&report=${repaired.id}`);
    await expect(page.locator('.dg-summary')).toBeVisible();
    const download = page.waitForEvent('download');
    await page.getByRole('button', { name: 'Xuất JSON' }).click();
    const file = `${EVIDENCE}/export-${repaired.id.slice(0, 8)}.json`;
    await (await download).saveAs(file);
    const exported = JSON.parse(readFileSync(file, 'utf8'));
    record('DG-E06.export-exact', exported.report.id === repaired.id && exported.report.scope.context.draftVersion >= 1 && exported.report.limits.includes('CONDITIONS_ONLY') && exported.report.counts.pass === repaired.counts.pass && !/token|secret|kubeconfig/i.test(JSON.stringify(exported)),
      'export carries the exact scope, pinned versions, limits and counts, with no credential material');

    // DG-E08 roles: a Developer has no access to account diagnostics.
    const developer = await (await openContext()).newPage();
    await developer.goto(`${URL_BASE}/ui/sign-in`);
    await developer.getByLabel('Tên đăng nhập').fill('developer');
    await developer.getByLabel('Mật khẩu').fill('test-password');
    await developer.getByRole('button', { name: 'Đăng nhập' }).click();
    await expect(developer.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
    const denied = await developer.request.get(`${URL_BASE}/api/v1/driver-accounts/${state.accountKey}/diagnostics`);
    record('DG-E08.developer-forbidden', denied.status() === 403, `HTTP ${denied.status()}`);
    record('DG-E01.connection-unchanged', (await api(`/driver-accounts/${state.accountKey}`)).body.version === baseline.version, `account version stayed ${baseline.version} through every run`);
    record('no-page-errors', errors.length === 0, errors.join('; ') || 'none');
  }
  console.log(`PHASE ${PHASE} OK`);
} catch (error) {
  console.error(`PHASE ${PHASE} FAIL: ${String(error?.message ?? error).split('\n')[0]}`);
  try { await page.screenshot({ path: `${EVIDENCE}/screenshots/${PHASE}-failure.png`, fullPage: true }); } catch { /* no page */ }
  process.exitCode = 1;
} finally {
  await browser.close();
}
