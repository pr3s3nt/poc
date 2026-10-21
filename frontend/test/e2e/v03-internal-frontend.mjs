// V03 headed recording (issue #4): the Orchestrator places the REAL acceptance FRONTEND on an existing
// Kubernetes cluster from Resource Definitions only (ADR-016). Invoked by
// backend/test/integration/v03-internal-frontend.sh, which starts the REAL product (planner ->
// pinned Terraform with encrypted PostgreSQL state -> required pinned score-k8s -> real Kubernetes
// delivery), the headed browser and the cleanup safety net.
//
// What is a UI action (counted as such in the assertions): Internal account upload, Resource Types,
// the existing-cluster / Terraform namespace / workload Definitions (with the workload companion
// rule), the Application, Score import, pending Preview, Deploy, update, remove and the owned
// namespace cleanup dialog. What is NOT: the labelled negative fixtures (`api` source below), the
// read-only kubectl assertions through the private host handle ("private-read") and the actual
// acceptance frontend page, which is opened in a second tab through `kubectl port-forward`.
//
// Scope (explicit user decision): ONLY the real acceptance frontend. Its backend-dependent rows
// (backend connection, environment, secret, database, cache) show FAIL and the job form shows a
// connection error; that is NOT IN SCOPE for V03 and is DEFERRED to V04. The scenario asserts that
// state; it never fakes a backend and never turns a check green.
//   node v03-internal-frontend.mjs --check V03
//   node v03-internal-frontend.mjs --scenario V03 --headed --captions vi --evidence DIR
// Marks, phases, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { createForwardManager, createRecorder, kubectlRunner, readScenarioInputAfterGate, safeError, startPortForward, waitFor } from './terraform-v2-lib.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const SHORT_READ = 2000;
const FIXTURE_PASSWORD = 'test-password'; // fixed synthetic fixture account of the isolated run, typed into a masked field
// Credential MATERIAL only (never a word such as "kubeconfig" or "terraform" that the UI legitimately shows).
const FORBIDDEN_IN_PAGE = [/AKIA[0-9A-Z]{16}/, /vault:\/\//i, /secretRef/i, /BEGIN [A-Z ]*PRIVATE KEY/, /client-key-data/i, /client-certificate-data/i, /hvs\.[A-Za-z0-9_-]{20,}/];

// Resource Types the catalog-free run registers through the Console (typed, no fields hidden):
// the cluster Type publishes the opaque target as `cluster`; the namespace Type has one optional input.
export const V03_TYPES = [
  { key: 'workload', inputs: [], outputs: [] },
  { key: 'k8s-cluster', inputs: [], outputs: [['name', 'string', false], ['cluster', 'any', true]] },
  { key: 'k8s-namespace', inputs: [['name', 'string', false]], outputs: [['name', 'string', true]] },
];

/** The authored catalog, shared with the post-restart expectations. */
export const catalogIds = (input) => ({
  cluster: `cluster-${input.runId}`, namespace: `namespace-${input.runId}`, template: `render-${input.runId}`,
  rivalA: `rival-a-${input.runId}`, rivalB: `rival-b-${input.runId}`,
});

function v2ctl(input, ...args) {
  return execFileSync(input.v2ctl, args, { encoding: 'utf8', env: { PATH: process.env.PATH }, stdio: ['ignore', 'pipe', 'pipe'] });
}
const kubeFlags = (input) => ['-kubectl', input.kube.kubectl, '-kubeconfig', input.kube.kubeconfig, '-context', input.kube.context];

/** Exact-text pattern source for a descriptor: `x` must not match `x-aux` (a plain hasText substring would match both). */
export const exactTextPattern = (text) => `^${String(text).replace(/[.*+?^${}()|[\]\\#]/g, '\\$&')}$`;

/** Assertion IDs a complete V03 run must have recorded as PASS. The restricted-credential negative (V03.08) is mandatory: the runner refuses to start without a real restricted kubeconfig. */
export const REQUIRED_ASSERTIONS = [
  'negative.unreachable-account-refused', 'negative.restricted-credential', 'ui.account.ready', 'ui.types.registered', 'ui.definition.cluster', 'ui.definition.namespace', 'ui.definition.workload',
  'ui.score.imported', 'preview.placement', 'preview.provider-first', 'preview.renderer', 'preview.pure', 'deploy.progress-persisted',
  'ready.workload', 'ready.owned-namespaces', 'ready.no-environment-target', 'ready.frontend-page', 'update.rollout', 'update.visible-config',
  'restart.catalog-persisted', 'restart.operation-after', 'restart.target-unchanged', 'negative.no-cluster-match', 'negative.unknown-account-refused',
  'negative.tied-cluster-definitions', 'negative.prerequisites-valid', 'negative.no-cluster-match.matcher', 'negative.tied-cluster-definitions.matcher', 'negative.no-side-effects', 'cleanup.workload-removed', 'cleanup.unreferenced',
  'cleanup.platform-excluded:main', 'cleanup.platform-excluded:aux', 'cleanup.namespaces-absent', 'cleanup.cluster-preserved',
];
export const REQUIRED_PHASES = ['PREPARE', 'FORM', 'PREVIEW', 'TERRAFORM', 'READY', 'CLEANUP'];

/** Returns the problems of a finished evidence directory (empty = complete). Pure and unit tested. */
export function verifyEvidence({ assertions, phases, assertionsText }) {
  const problems = [];
  for (const id of REQUIRED_ASSERTIONS) {
    const found = assertions.find((a) => a.id === id);
    if (!found) problems.push(`missing assertion ${id}`);
    else if (!found.pass) problems.push(`failed assertion ${id}`);
  }
  if (/^FAIL /m.test(assertionsText)) problems.push('assertions.txt contains a FAIL line');
  for (const name of REQUIRED_PHASES) if (!phases.some((p) => p.phase === name)) problems.push(`missing phase ${name}`);
  if (!assertions.some((a) => a.source === 'ui')) problems.push('no assertion was observed through the UI');
  return problems;
}

export const SCENARIOS = {
  async V03(ctx) {
    const { page, h, context, x11, caption, phase, mark, rec, baseURL, input, forwarders, evidenceDir } = ctx;
    const ids = catalogIds(input);
    const kubectl = kubectlRunner(input.kube);
    const appPath = `/applications/${input.applicationKey}/environments/staging`;
    const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
    const get = async (path) => (await (await page.request.get(`${baseURL}/api/v1${path}`)).json());
    const wrapped = new Set(['Loại tài nguyên', 'Module đã kiểm chứng', 'Tài khoản hạ tầng', 'Bundle dựng ứng dụng', 'Cluster đích của provider kubernetes', 'Class cluster EKS của provider kubernetes', 'Tài khoản Internal của cluster']);
    const form = (name) => (wrapped.has(name)
      ? page.locator(`select[aria-label="${name}"], label:has-text("${name}") > select`).first()
      : page.getByLabel(name, { exact: true }));
    const alertBox = () => page.getByRole('alert').first();
    const optionLabel = async (select, prefix) => {
      const texts = await select.locator('option').allTextContents();
      const found = texts.find((text) => text.trim().startsWith(prefix));
      if (!found) throw new Error(`no option starting with "${prefix}"`);
      return found;
    };
    const choose = async (select, prefix) => h.choose(select, await optionLabel(select, prefix));
    const noLeak = async (where) => {
      const text = await page.locator('body').innerText();
      for (const pattern of FORBIDDEN_IN_PAGE) expect(pattern.test(text), `${where}: the page shows credential-like text`).toBe(false);
    };
    // Every /api/v1 reply is scanned for credential-bearing data (also replies the scenario never waits on).
    const violations = [];
    page.on('response', async (response) => {
      const url = new URL(response.url());
      if (!url.pathname.startsWith('/api/v1/')) return;
      const text = await response.text().catch(() => '');
      if (FORBIDDEN_IN_PAGE.some((pattern) => pattern.test(text))) {
        violations.push(`${response.request().method()} ${url.pathname} ${response.status()}`);
        appendFileSync(`${evidenceDir}/assertions.txt`, `FAIL ${response.request().method()} ${url.pathname} carried credential-like data\n`);
      }
    });
    const assertNoViolations = () => expect(violations, 'a reply carried credential-bearing data').toEqual([]);
    const technicalJson = async (args) => {
      const result = await kubectl(args);
      if (result.code !== 0) throw new Error(`kubectl ${args[0]} failed`);
      return JSON.parse(result.stdout);
    };
    const namespaceUid = async (name) => {
      const result = await kubectl(['get', 'namespace', name, '-o', 'jsonpath={.metadata.uid}']);
      return result.code === 0 ? result.stdout.trim() : '';
    };
    const verifyWorkload = (image, digest, backendUrl, ready = true) => {
      try {
        const out = v2ctl(input, 'v03-workload', ...kubeFlags(input), '-namespace', input.namespace, '-name', 'frontend', '-image', image, '-digest', digest, '-env', `BACKEND_URL=${backendUrl}`, ...(ready ? ['-ready'] : []));
        lastVerdict = out.trim();
        return { ok: true, out: lastVerdict };
      } catch (error) {
        // stdout is the bounded summary line; stderr's first line is the named failure. The command line itself (with the expected values) is never used.
        lastVerdict = `${String(error?.stdout ?? '').trim().split('\n').pop() ?? ''} | ${String(error?.stderr ?? '').trim().split('\n')[0] ?? ''}`.slice(0, 700);
        return { ok: false, out: lastVerdict };
      }
    };
    // The Score Preview page (UC-05) shows the plan of the SAME Score and Environment snapshot: graph edges, matches, batches and pins.
    const scorePreview = async (scoreFile, tag) => {
      await h.click(page.getByRole('button', { name: 'Xem trước Score', exact: true }));
      await h.type(page.getByLabel('ID Workload', { exact: true }), 'frontend', { replace: true });
      await h.type(page.getByLabel('ID lần chạy', { exact: true }), `${tag}-${Date.now() % 100000}`, { replace: true });
      await h.paste(page.getByLabel(/Score sau/), readFileSync(scoreFile, 'utf8'));
      const replied = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith('/score-preview'));
      await h.click(page.getByRole('button', { name: 'Xem trước', exact: true }));
      const response = await replied;
      return { status: response.status(), body: await response.json().catch(() => ({})) };
    };
    // Persisted per-workload target pins read from the product's database (safe identities only).
    const appliedTarget = () => JSON.parse(v2ctl(input, 'v03-target', '-database-url-file', `${input.privateDir}/console.url`)).find((t) => t.workloadId === 'frontend');
    const sameTarget = (a, b) => Boolean(a && b) && ['connection', 'cluster', 'namespace', 'account', 'accountVersion', 'clusterDefinition', 'clusterNode'].every((field) => a[field] === b[field]);
    // The last safe verdict of a workload wait (rollout counters, image IDs, names of differing variables - never values) is kept for the timeout message.
    let lastVerdict = '';
    const apiSetup = (label) => appendFileSync(`${evidenceDir}/setup-api-calls.txt`, `${label}\n`);

    async function signIn(username) {
      await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), username);
      await h.type(page.getByLabel('Mật khẩu', { exact: true }), FIXTURE_PASSWORD);
      await expect(page.getByLabel('Mật khẩu', { exact: true })).toHaveAttribute('type', 'password');
      await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
      await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
    }
    async function signOut() {
      await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
      await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();
    }
    const deploy = () => h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
    // Walks back with the "←" buttons until the Application page is shown.
    async function backToApplication(name = input.applicationName) {
      const heading = page.getByRole('heading', { name });
      for (let i = 0; i < 5 && !(await heading.isVisible()); i += 1) await h.click(page.getByRole('button', { name: /^←/ }).first());
      await expect(heading).toBeVisible();
    }
    async function openApplication(name = input.applicationName) {
      await h.click(page.getByRole('button', { name: new RegExp(name) }).first());
      await expect(page.getByRole('heading', { name })).toBeVisible();
    }

    // Application keys of the Environment (the product allows only references in container variables, never literals).
    const variablesSection = () => page.locator('section[aria-label="Biến"]');
    async function openSettings() {
      await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
      await expect(variablesSection()).toBeVisible();
      await h.pause(800);
    }
    async function saveVariable() {
      const name = await page.getByLabel('Tên key', { exact: true }).inputValue();
      const saved = page.waitForResponse((r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/configuration/keys/${encodeURIComponent(name)}`));
      await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
      const response = await saved;
      if (!response.ok()) throw new Error(`saving variable ${name} returned HTTP ${response.status()}`);
      await expect(page.getByLabel('Tên key', { exact: true })).toHaveCount(0);
      await h.pause(600);
    }
    async function addVariable(name, value) {
      await h.click(variablesSection().getByRole('button', { name: '+ Thêm biến' }));
      await h.type(page.getByLabel('Tên key', { exact: true }), name);
      await h.type(page.getByLabel('Giá trị', { exact: true }), value);
      await saveVariable();
    }
    async function editVariable(name, value) {
      const row = variablesSection().locator('.settings-table-row').filter({ has: page.locator('strong', { hasText: new RegExp(`^${name}$`) }) });
      await h.click(row.getByRole('button', { name: 'Sửa', exact: true }));
      await h.type(page.getByLabel('Giá trị', { exact: true }), value, { replace: true });
      await saveVariable();
    }
    const leaveSettings = (name = input.applicationName) => h.click(page.getByRole('button', { name: `← ${name}` }));

    // ===================== PREPARE: Platform Engineer, Internal accounts =====================
    phase('PREPARE', 'Kỹ sư nền tảng đăng ký tài khoản Internal bằng kubeconfig tải lên; Console xác minh với cluster thật, cluster lỗi/thiếu quyền bị từ chối và không lưu gì');
    await h.showCursor();
    await expectVisibleCursor(page);
    expect((await get('/driver-accounts')).accounts ?? []).toHaveLength(0);
    await signIn('platform-engineer');
    await h.click(nav('Tài khoản hạ tầng'));
    await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toBeVisible();
    const create = () => page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    const createAlert = () => create().locator('.form-error[role="alert"]');
    const chooseFile = async (file) => {
      const chooser = page.waitForEvent('filechooser');
      chooser.catch(() => {});
      await h.click(create().locator('input[type="file"]'));
      await (await chooser).setFiles(file);
      await expect(create().getByLabel('Đích đã chọn')).toBeVisible();
      await h.pause(800);
    };
    const saveAccount = async () => {
      const replied = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/driver-accounts/internal', { timeout: 180_000 });
      await h.click(create().getByRole('button', { name: 'Kiểm tra và lưu', exact: true }));
      const response = await replied;
      let body = {};
      try { body = await response.json(); } catch { /* not JSON */ }
      assertNoViolations();
      return { status: response.status(), code: body.code, key: body.key, accountStatus: body.status, version: body.version };
    };

    // The deferred V04 business assertions are recorded explicitly so the PASS cannot be read as a jobs/cache proof.
    writeFileSync(`${evidenceDir}/deferred-v04.txt`, [
      'DEFERRED V04 (explicit user decision, NOT proven by this run):',
      '- backend Service and frontend API proxy against a real backend',
      '- PostgreSQL schema/jobs and worker (job submit, claim, complete, persistence)',
      '- Redis worker write, cache hit/miss, TTL and DB-backed refill',
      '- backend environment/secret/database/cache check rows turning PASS',
      'This run only asserts that those rows are FAIL / unreachable because their dependencies are not deployed.',
      '',
    ].join('\n'));

    // Negative (live): a kubeconfig whose API endpoint is unreachable is refused; nothing is stored.
    await h.type(create().getByLabel('Tên tài khoản', { exact: true }), input.unreachable.name);
    await chooseFile(input.unreachable.kubeconfigFile);
    const unreachable = await saveAccount();
    await expect(createAlert()).toBeVisible();
    const afterUnreachable = (await get('/driver-accounts')).accounts ?? [];
    rec.record('negative.unreachable-account-refused', 'PREPARE', 'ui', unreachable.status === 422 && afterUnreachable.length === 0,
      `unreachable cluster -> HTTP ${unreachable.status} ${unreachable.code ?? ''}; accounts stored: ${afterUnreachable.length}`);
    await h.moveTo(createAlert());
    await h.read('Cluster không truy cập được nên không lưu tài khoản nào');

    if (!input.restricted?.kubeconfigFile) throw new Error('a real restricted kubeconfig is required (V03.08 permission negative); the run cannot continue without it');
    {
      // Negative (live, real restricted credential): the Console must not accept an account that cannot create namespaces.
      await h.type(create().getByLabel('Tên tài khoản', { exact: true }), input.restricted.name, { replace: true });
      await chooseFile(input.restricted.kubeconfigFile);
      const restricted = await saveAccount();
      const stored = ((await get('/driver-accounts')).accounts ?? []).find((a) => a.name === input.restricted.name);
      // The verifier requires create on namespaces, Deployments, StatefulSets, Services and Secrets: a real restricted credential is refused with
      // VERIFICATION_PERMISSION_DENIED and nothing is stored, so it can never become the account of a cluster Definition.
      rec.record('negative.restricted-credential', 'PREPARE', 'ui', restricted.status === 422 && restricted.code === 'VERIFICATION_PERMISSION_DENIED' && !stored,
        `real restricted credential -> HTTP ${restricted.status} ${restricted.code ?? restricted.accountStatus ?? ''}; stored: ${Boolean(stored)}`);
      await h.pause(SHORT_READ);
    }

    await h.type(create().getByLabel('Tên tài khoản', { exact: true }), input.account.name, { replace: true });
    await chooseFile(input.account.kubeconfigFile);
    const account = await saveAccount();
    const accountOk = account.status === 201 && account.accountStatus === 'READY' && account.version === 1;
    rec.record('ui.account.ready', 'PREPARE', 'ui', accountOk, `Internal account ${account.key} HTTP ${account.status} ${account.accountStatus} v${account.version}; kubeconfig never shown`);
    const accountKey = account.key;
    // Exact Vault ownership for the safety net: the credential objects live under this run's own account key only.
    writeFileSync(`${input.privateDir}/owned-vault.json`, JSON.stringify({ accountKeys: [accountKey] }), { mode: 0o600 });
    writeFileSync(`${input.privateDir}/account-key`, accountKey, { mode: 0o600 });
    await noLeak('account page');
    await h.read('Tài khoản Internal đã sẵn sàng; thông tin xác thực không hiển thị');
    mark('account-ready');

    // ===================== FORM: Resource Types and Definitions (Platform Engineer) =====================
    phase('FORM', 'Danh mục bắt đầu trống: đăng ký Loại tài nguyên, cấu hình cluster có sẵn, namespace Terraform trỏ tới đúng cluster đó và mẫu dựng workload tham chiếu namespace, cluster cùng tài nguyên đi kèm');
    await h.click(nav('Loại tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Loại tài nguyên', level: 1, exact: true })).toBeVisible();
    expect((await get('/resource-types')).resourceTypes).toHaveLength(0);
    for (const type of V03_TYPES) {
      await h.type(form('ID loại tài nguyên'), type.key);
      for (const [index, [name, kind, required]] of type.inputs.entries()) {
        await h.click(page.getByRole('button', { name: '+ Thêm tham số đầu vào', exact: true }));
        await h.type(form(`Đầu vào ${index + 1} tên`), name);
        if (kind !== 'string') await h.choose(form(`Đầu vào ${index + 1} kiểu`), kind);
        if (required) await h.click(form(`Đầu vào ${index + 1} bắt buộc`));
      }
      for (const [index, [name, kind, required]] of type.outputs.entries()) {
        await h.click(page.getByRole('button', { name: '+ Thêm tham số đầu ra', exact: true }));
        await h.type(form(`Đầu ra ${index + 1} tên`), name);
        if (kind !== 'string') await h.choose(form(`Đầu ra ${index + 1} kiểu`), kind);
        if (required) await h.click(form(`Đầu ra ${index + 1} bắt buộc`));
      }
      await h.click(page.getByRole('button', { name: 'Đăng ký loại tài nguyên', exact: true }));
      await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký loại tài nguyên ${type.key}.` })).toBeVisible();
    }
    const types = (await get('/resource-types')).resourceTypes;
    const clusterType = types.find((t) => t.key === 'k8s-cluster');
    rec.record('ui.types.registered', 'FORM', 'ui', types.length === 3 && clusterType?.outputs.some((o) => o.name === 'cluster' && o.type === 'any' && o.required === true),
      `${types.length} types; the cluster Type publishes the typed target as output "cluster"`);
    mark('types-registered');

    await h.click(nav('Cấu hình tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
    const clusterSection = page.getByRole('region', { name: 'Cluster có sẵn (existing-cluster)' });
    // Existing-cluster Definition with its OWN Internal account; selected by class and application like any Definition.
    await h.click(clusterSection.getByRole('button', { name: 'Đăng ký cấu hình cluster có sẵn' }));
    await h.type(clusterSection.getByLabel('ID cấu hình cluster', { exact: true }), ids.cluster);
    await h.type(clusterSection.getByLabel('Tên hiển thị cluster', { exact: true }), 'Cluster k8s-4f (có sẵn)');
    await choose(form('Tài khoản Internal của cluster'), input.account.name);
    await h.click(clusterSection.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(clusterSection.getByLabel('Điều kiện 1 ID ứng dụng', { exact: true }), input.applicationKey);
    await h.type(clusterSection.getByLabel('Điều kiện 1 Class', { exact: true }), 'internal');
    await h.pause(SHORT_READ);
    await h.click(clusterSection.getByRole('button', { name: 'Đăng ký cấu hình cluster', exact: true }));
    await expect(clusterSection.getByRole('status')).toContainText(`Đã đăng ký cấu hình cluster ${ids.cluster}.`);
    const storedCluster = (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === ids.cluster);
    rec.record('ui.definition.cluster', 'FORM', 'ui', storedCluster?.driverType === 'existing-cluster' && storedCluster.driverAccount === accountKey && storedCluster.version === 1 && Object.keys(storedCluster.driverInputs?.values?.variables ?? {}).length === 0,
      `cluster Definition ${ids.cluster} uses its own Internal account ${accountKey}; no variables, no Environment connection`);
    await h.read('Cấu hình cluster dùng tài khoản Internal của chính nó');
    mark('cluster-definition');

    // Terraform namespace module bound to the cluster resource of class internal (never to the Environment).
    await h.type(form('ID cấu hình (Definition ID)'), ids.namespace);
    await h.type(form('Tên hiển thị'), 'Namespace Terraform trên cluster internal');
    await choose(form('Loại tài nguyên'), 'k8s-namespace');
    await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
    const pins = form('Module đã kiểm chứng');
    if (await pins.isEnabled()) await choose(pins, 'k8s-namespace');
    await h.click(page.getByRole('button', { name: 'Ghép đầu vào theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await choose(form('Cluster đích của provider kubernetes'), 'Cluster do node');
    await h.choose(page.getByLabel(/^Class cluster EKS của provider /), 'internal');
    await h.click(page.getByRole('button', { name: 'Dùng preset cho name', exact: true }));
    await h.pause(SHORT_READ);
    await h.click(page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }));
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${ids.namespace}.` })).toBeVisible();
    const storedNamespace = (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === ids.namespace);
    const binding = storedNamespace?.driverInputs?.values?.provider_bindings?.kubernetes;
    rec.record('ui.definition.namespace', 'FORM', 'ui', binding?.target === 'resource' && binding.resource_ref === 'k8s-cluster.internal#environments.@app.@env' && !storedNamespace.driverAccount,
      `namespace module binds its Kubernetes provider to ${binding?.resource_ref}; no account of its own`);

    // Workload rendering template: literal bundle + whole namespace/cluster references + a companion namespace.
    await h.click(nav('Mẫu dựng ứng dụng'));
    await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
    await h.type(form('ID mẫu'), ids.template);
    await h.type(form('Tên hiển thị'), 'Mẫu dựng frontend (score-k8s)');
    await expect(form('Bundle dựng ứng dụng')).toHaveValue(input.bundle);
    await h.click(page.getByRole('button', { name: 'Dùng gợi ý namespace', exact: true }));
    await h.click(page.getByRole('button', { name: 'Dùng gợi ý cluster', exact: true }));
    await h.click(page.getByRole('button', { name: '+ Thêm tài nguyên đi kèm', exact: true }));
    await h.type(page.getByLabel('Descriptor tài nguyên đi kèm 1', { exact: true }), 'k8s-namespace.default#environments.@app.@env-aux');
    await h.type(page.getByLabel('Params tài nguyên đi kèm 1 (JSON object)', { exact: true }), JSON.stringify({ name: input.companionNamespace }), { replace: true });
    await h.pause(SHORT_READ);
    await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }));
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký mẫu dựng ứng dụng ${ids.template}.` })).toBeVisible();
    const storedTemplate = (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === ids.template);
    const templateVars = storedTemplate?.driverInputs?.values?.variables ?? {};
    rec.record('ui.definition.workload', 'FORM', 'ui',
      templateVars.render_bundle === input.bundle && /outputs\.name\}$/.test(templateVars.namespace ?? '') && /outputs\.cluster\}$/.test(templateVars.cluster ?? '') && Boolean(storedTemplate.provision?.['k8s-namespace.default#environments.@app.@env-aux']),
      `workload Definition ${ids.template}: literal bundle, whole namespace/cluster output references, companion rule for ${input.companionNamespace}`);
    await h.read('Mẫu dựng tham chiếu namespace và cluster, kèm một namespace đi kèm do chính cấu hình sinh ra');
    mark('workload-definition');
    await signOut();

    // ===================== PREVIEW: Developer imports the real frontend Score =====================
    phase('PREVIEW', 'Nhà phát triển nhập Score của frontend thật (ảnh A), xem trước: Definition, tài khoản, cluster, namespace, thứ tự lô provider-first; chưa có gì được tạo');
    await signIn('developer');
    await openApplication();
    const staged = (await get(`${appPath}/deployments`)).deployments ?? [];
    expect(staged).toHaveLength(0);
    expect(await namespaceUid(input.namespace), 'the run namespace must not exist before Deploy').toBe('');
    // The frontend configuration is Environment variables (PORT, BACKEND_URL) that the Score only references.
    await openSettings();
    await addVariable('PORT', '8080');
    await addVariable('BACKEND_URL', input.backendUrl.a);
    await leaveSettings();
    const importScore = async (file, expectName) => {
      await h.click(page.getByRole('button', { name: 'Nhập Score', exact: true }));
      const chooser = page.waitForEvent('filechooser');
      chooser.catch(() => {});
      await h.click(page.getByLabel('File Score', { exact: true }));
      await (await chooser).setFiles(file);
      await expect(page.getByText(`Đã phân tích Workload: ${expectName}`, { exact: false })).toBeVisible();
      await h.pause(SHORT_READ);
      const saved = page.waitForResponse((r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/workloads/${expectName}`));
      await h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
      const response = await saved;
      if (!response.ok()) throw new Error(`saving the Score draft returned HTTP ${response.status()}`);
      await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
    };
    await h.click(page.getByRole('button', { name: '+ Thêm Workload', exact: true }));
    await importScore(input.scoreFiles.a, 'frontend');
    const drafts = (await get(`${appPath}/workloads`)).workloads ?? [];
    rec.record('ui.score.imported', 'FORM', 'ui', drafts.some((w) => w.id === 'frontend' && w.score?.containers?.main?.image === input.images.a.ref),
      `frontend Score draft saved with image ${input.images.a.ref}; Score sha256 ${input.scoreHashes.a}`);
    mark('score-imported');

    // The plan of the very same Score and Environment snapshot, read through the Score Preview page before the pending Preview.
    await backToApplication();
    const planned = await scorePreview(input.scoreFiles.a, 'plan');
    const plan = planned.body;
    await backToApplication();
    const previewResponse = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(`${appPath}/preview`));
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    const preview = await (await previewResponse).json();
    const panel = page.getByLabel('Xem trước triển khai');
    await expect(panel).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await h.moveTo(panel);
    await h.pause(SHORT_READ);
    const change = preview.changes?.[0] ?? {};
    const placements = change.placements ?? [];
    const work = placements.find((p) => p.node === 'workload.default#modules.frontend');
    const nsPlacement = placements.find((p) => p.node.startsWith('k8s-namespace.default#environments.') && !p.node.endsWith('-aux'));
    const auxPlacement = placements.find((p) => p.node.endsWith('-aux'));
    const clusterDescriptor = `k8s-cluster.internal#environments.${input.applicationKey}.staging`;
    rec.record('preview.placement', 'PREVIEW', 'ui',
      work?.cluster === clusterDescriptor && work.account === accountKey && work.clusterDefinition === ids.cluster && nsPlacement?.cluster === clusterDescriptor && auxPlacement?.cluster === clusterDescriptor && !preview.binding?.connectionKey,
      `workload, namespace and companion all resolve to ${work?.clusterDefinition} / account ${work?.account} v${work?.accountVersion}; no Environment connection pinned`);
    const mainNamespaceNode = nsPlacement?.node ?? '';
    const auxNamespaceNode = auxPlacement?.node ?? '';
    const workloadNode = 'workload.default#modules.frontend';
    const hasEdge = (consumer, provider, reason) => (plan.graph?.edges ?? []).some((e) => e.consumer === consumer && e.provider === provider && e.reason === reason);
    const batchIndex = (descriptor) => (plan.batches ?? []).findIndex((batch) => batch.includes(descriptor));
    const matched = (descriptor) => (plan.matches ?? []).find((m) => m.descriptor === descriptor);
    const orderOk = planned.status === 200 && batchIndex(clusterDescriptor) >= 0 && batchIndex(clusterDescriptor) < batchIndex(mainNamespaceNode) && batchIndex(clusterDescriptor) < batchIndex(auxNamespaceNode) && batchIndex(mainNamespaceNode) >= 0 && batchIndex(auxNamespaceNode) >= 0;
    const edgesOk = hasEdge(mainNamespaceNode, clusterDescriptor, 'provider-binding') && hasEdge(auxNamespaceNode, clusterDescriptor, 'provider-binding')
      && hasEdge(workloadNode, mainNamespaceNode, 'resource-reference') && hasEdge(workloadNode, clusterDescriptor, 'resource-reference');
    const definitionsOk = matched(clusterDescriptor)?.definitionKey === ids.cluster && matched(clusterDescriptor)?.driverAccount === accountKey
      && matched(mainNamespaceNode)?.definitionKey === ids.namespace && matched(auxNamespaceNode)?.definitionKey === ids.namespace;
    const intentOk = change.resourceIntents?.find((i) => i.descriptor === clusterDescriptor)?.ownership === 'PREEXISTING' && change.resourceIntents.length === 3;
    rec.record('preview.provider-first', 'PREVIEW', 'ui', orderOk && edgesOk && definitionsOk && intentOk && plan.planHash === change.planHash,
      `batches ${(plan.batches ?? []).map((b, i) => `#${i}:${b.length}`).join(' ')}; cluster batch ${batchIndex(clusterDescriptor)} before namespace ${batchIndex(mainNamespaceNode)} and companion ${batchIndex(auxNamespaceNode)}; edges namespace/companion->cluster (provider-binding) and workload->namespace/cluster (resource-reference); cluster Definition ${matched(clusterDescriptor)?.definitionKey} account ${matched(clusterDescriptor)?.driverAccount}; the existing cluster is PREEXISTING; the Score Preview plan hash equals the pending Preview plan hash`);
    rec.record('preview.renderer', 'PREVIEW', 'ui', change.rendering?.bundle?.id === input.bundle && change.rendering?.definitionKey === ids.template && change.rendering?.class === 'default',
      `renderer ${change.rendering?.bundle?.id} ${change.rendering?.bundle?.version} via ${change.rendering?.definitionKey}, class ${change.rendering?.class}`);
    rec.record('preview.pure', 'PREVIEW', 'private-read', (await namespaceUid(input.namespace)) === '' && (await namespaceUid(input.companionNamespace)) === '' && ((await get(`${appPath}/deployments`)).deployments ?? []).length === 0,
      'after Preview: no namespace, no Deployment record');
    await noLeak('preview');
    assertNoViolations();
    await h.read('Xem trước chỉ đọc: chưa có namespace hay Deployment nào được tạo');
    mark('preview-checked');

    // ===================== TERRAFORM / READY =====================
    phase('TERRAFORM', 'Triển khai: cluster có sẵn được kiểm tra bằng tài khoản của nó, Terraform tạo hai namespace do orchestrator sở hữu rồi score-k8s phát hành frontend');
    await deploy();
    const result = page.getByLabel('Kết quả triển khai');
    await expect(result).toContainText('Triển khai thành công', { timeout: input.timeouts.deployMs });
    await h.moveTo(result);
    await h.pause(SHORT_READ);
    const deployments = (await get(`${appPath}/deployments`)).deployments ?? [];
    const progressRows = [];
    for (const d of deployments) progressRows.push(...((await get(`${appPath}/deployments/${d.id}`)).resourceProgress ?? []));
    await h.click(page.getByRole('button', { name: 'Xem tất cả', exact: true }));
    await h.click(page.locator('button.deployment-entry').first());
    const progressRegion = page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' });
    await expect(progressRegion).toBeVisible();
    await h.moveTo(progressRegion);
    await h.pause(SHORT_READ);
    rec.record('deploy.progress-persisted', 'TERRAFORM', 'ui', progressRows.filter((r) => r.stage === 'READY' && r.status === 'SUCCEEDED').length >= 2, `${progressRows.length} persisted Terraform attempt rows, READY/SUCCEEDED ${progressRows.filter((r) => r.stage === 'READY').length} (namespace and companion)`);
    mark('deployed');

    phase('READY', 'Sẵn sàng: Deployment frontend đúng ảnh A và cấu hình BACKEND_URL trên đúng cluster; hai namespace do orchestrator sở hữu; trang frontend thật hiển thị tiêu đề, bảng kiểm tra và form job');
    const ready = await waitFor(async () => {
      const verdict = verifyWorkload(input.images.a.ref, input.images.a.digest, input.backendUrl.a, true);
      return verdict.ok ? verdict : false;
    }, { timeoutMs: 5 * 60_000, intervalMs: 3000, what: 'the frontend Deployment to be ready with image A and configuration A', describe: () => lastVerdict });
    rec.record('ready.workload', 'READY', 'private-read', Boolean(ready), `${ready.out}`);
    const baseline = JSON.parse(readFileSync(input.baselineFile, 'utf8'));
    v2ctl(input, 'v03-record-owned', ...kubeFlags(input), '-baseline', input.baselineFile, '-run-id', input.runId, '-database-url-file', `${input.privateDir}/console.url`, '-namespaces', `${input.namespace},${input.companionNamespace}`, '-out', input.ownedFile);
    rec.record('ready.owned-namespaces', 'READY', 'private-read', JSON.parse(readFileSync(input.ownedFile, 'utf8')).length === 2 && baseline.runNamespaces.length === 2,
      `${input.namespace} and ${input.companionNamespace} exist with this run's exact tags, match product receipts (instance and Terraform run) and have recorded UIDs`);
    const targetFirst = appliedTarget();
    const namespaceUidFirst = await namespaceUid(input.namespace);
    const instance = (await get(`${appPath}/deployments`)).deployments?.[0];
    rec.record('ready.no-environment-target', 'READY', 'ui', !(await get(`/applications/${input.applicationKey}`)).application?.environments?.find((e) => e.key === 'staging')?.connectionKey && Boolean(instance),
      'the Environment still has no deployment Connection; the Definitions decided the target');

    const frontendPort = input.forwardPort;
    const healthy = async (url) => fetch(`${url}/healthz`).then((r) => r.ok).catch(() => false);
    const forwarding = createForwardManager({ ...input.kube, namespace: input.namespace, service: 'frontend', remotePort: input.servicePort, localPort: frontendPort }, { forwarders, start: startPortForward, healthy });
    // A port-forward to a Service binds one Pod and dies when a rollout replaces it: it is re-established after every rollout.
    await forwarding.connect('the frontend Service to answer through the port-forward');
    const appTab = await context.newPage();
    await appTab.goto(forwarding.url + '/');
    await expect(appTab.getByRole('heading', { name: 'Acceptance application', level: 1 })).toBeVisible();
    const checkRows = await appTab.locator('#checks tr[data-check]').evaluateAll((rows) => rows.map((row) => [row.getAttribute('data-check'), row.children[1].textContent.trim()]));
    await expect(appTab.locator('#backend')).toHaveText(`backend: ${input.backendUrl.a}`);
    await expect(appTab.getByRole('button', { name: 'Submit job' })).toBeVisible();
    await h.moveTo(appTab.locator('#checks'));
    const everyFail = checkRows.length === 5 && checkRows.every(([, status]) => status === 'FAIL');
    rec.record('ready.frontend-page', 'READY', 'ui', everyFail, `real frontend page rendered: title, ${checkRows.length} check rows [${checkRows.map(([n, s]) => `${n}:${s}`).join(', ')}], job form. FAIL rows are the EXPECTED out-of-scope state (backend/worker/PostgreSQL/Redis are deferred to V04), never a pass`);
    caption('Trang frontend thật đã hiển thị. Các hàng backend/database/cache FAIL là đúng và NGOÀI PHẠM VI V03 (chuyển sang V04); không có backend giả');
    await h.pause(SHORT_READ * 2);
    mark('frontend-page');
    await page.bringToFront();

    // ===================== UPDATE =====================
    phase('READY', 'Cập nhật: Nhà phát triển nhập Score mới (ảnh B, BACKEND_URL mới); Xem trước là cập nhật cùng đích đã ghim, triển khai và rollout tới ảnh B');
    await page.bringToFront();
    await backToApplication();
    await openSettings();
    await editVariable('BACKEND_URL', input.backendUrl.b);
    await leaveSettings();
    const row = page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: /frontend$/ }) });
    await h.click(row.getByRole('button', { name: 'Sửa', exact: true }));
    await importScore(input.scoreFiles.b, 'frontend');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('cập nhật');
    await h.pause(SHORT_READ);
    await deploy();
    await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công', { timeout: input.timeouts.deployMs });
    const updated = await waitFor(async () => {
      const verdict = verifyWorkload(input.images.b.ref, input.images.b.digest, input.backendUrl.b, true);
      return verdict.ok ? verdict : false;
    }, { timeoutMs: 5 * 60_000, intervalMs: 3000, what: 'the rollout to reach image B and configuration B', describe: () => lastVerdict });
    rec.record('update.rollout', 'READY', 'private-read', Boolean(updated), `${updated.out}; rollout complete with the controller observing the current generation, and the digest of the image running in every Pod equals the externally prepared digest of image B`);
    const targetUpdated = appliedTarget();
    const targetUnchangedByUpdate = sameTarget(targetFirst, targetUpdated) && (await namespaceUid(input.namespace)) === namespaceUidFirst;
    await forwarding.connect('the frontend Service to answer through a fresh port-forward after the rollout to image B');
    await appTab.goto(`${forwarding.url}/`);
    await expect(appTab.getByRole('heading', { name: 'Acceptance application', level: 1 })).toBeVisible();
    await expect(appTab.locator('#backend')).toHaveText(`backend: ${input.backendUrl.b}`);
    await expect(appTab.getByRole('button', { name: 'Submit job' })).toBeVisible();
    rec.record('update.visible-config', 'READY', 'ui', true, 'the real frontend page now shows the updated BACKEND_URL (image B)');
    mark('updated');
    await h.pause(SHORT_READ);

    // ===================== RESTART the orchestrator =====================
    phase('READY', 'Khởi động lại orchestrator: kế hoạch, ghim đích và trạng thái tài nguyên nằm trong PostgreSQL; sau đó vẫn thao tác được và dùng đúng đích đã áp dụng');
    execFileSync(process.env.ORCH_E2E_RESTART_CMD, { stdio: 'ignore' });
    await page.reload();
    await expect(page.getByLabel('Tên đăng nhập', { exact: true }).or(page.getByRole('button', { name: 'Đăng xuất', exact: true }))).toBeVisible({ timeout: 120_000 });
    if (await page.getByRole('button', { name: 'Đăng xuất', exact: true }).isVisible()) await signOut();
    // The catalog (read as Platform Engineer) is intact in SQL after the restart; a display-name edit then must not move the applied workload.
    await signIn('platform-engineer');
    const afterRestart = (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === ids.cluster);
    rec.record('restart.catalog-persisted', 'READY', 'ui', afterRestart?.version === storedCluster.version && afterRestart?.driverAccount === accountKey, `cluster Definition v${afterRestart?.version} and account ${afterRestart?.driverAccount} survived the restart`);
    await h.click(nav('Cấu hình tài nguyên'));
    await h.click(page.getByRole('region', { name: 'Cluster có sẵn (existing-cluster)' }).getByRole('button', { name: `Sửa cluster ${ids.cluster}` }));
    const clusterEdit = page.getByRole('region', { name: 'Cluster có sẵn (existing-cluster)' });
    await h.type(clusterEdit.getByLabel('Tên hiển thị cluster', { exact: true }), 'Cluster k8s-4f (có sẵn, đã đổi tên)', { replace: true });
    await h.click(clusterEdit.getByRole('button', { name: 'Lưu thay đổi cluster', exact: true }));
    await expect(clusterEdit.getByRole('status')).toContainText('phiên bản 2');
    await signOut();
    await signIn('developer');
    await openApplication();
    await openSettings();
    await editVariable('BACKEND_URL', input.backendUrl.a);
    await leaveSettings();
    const rowAgain = page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: /frontend$/ }) });
    await h.click(rowAgain.getByRole('button', { name: 'Sửa', exact: true }));
    await importScore(input.scoreFiles.a, 'frontend');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await deploy();
    await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công', { timeout: input.timeouts.deployMs });
    const reverted = await waitFor(async () => {
      const verdict = verifyWorkload(input.images.a.ref, input.images.a.digest, input.backendUrl.a, true);
      return verdict.ok ? verdict : false;
    }, { timeoutMs: 5 * 60_000, intervalMs: 3000, what: 'the post-restart operation to roll out image A again', describe: () => lastVerdict });
    // The rollout back to image A replaced the Pod again: reconnect and read the real page once more.
    await forwarding.connect('the frontend Service to answer through a fresh port-forward after the post-restart rollout');
    await appTab.goto(`${forwarding.url}/`);
    await expect(appTab.getByRole('heading', { name: 'Acceptance application', level: 1 })).toBeVisible();
    await expect(appTab.locator('#backend')).toHaveText(`backend: ${input.backendUrl.a}`);
    rec.record('restart.operation-after', 'READY', 'private-read', Boolean(reverted), `${reverted.out}; same namespace and account after the restart and a catalog rename; the real page, reached through a fresh port-forward, shows configuration A again`);
    const targetAfter = appliedTarget();
    const sameAfterRestart = sameTarget(targetFirst, targetAfter) && (await namespaceUid(input.namespace)) === namespaceUidFirst;
    rec.record('restart.target-unchanged', 'READY', 'private-read', Boolean(targetFirst) && targetUnchangedByUpdate && sameAfterRestart,
      `persisted applied target (account ${targetAfter?.account} v${targetAfter?.accountVersion}, cluster ${targetAfter?.cluster}, namespace ${targetAfter?.namespace}, Definition ${targetAfter?.clusterDefinition}) and the namespace UID are identical after the first deploy, the update, the restart and the operation after it (the cluster Definition v${targetFirst?.clusterDefinitionVersion} -> v${targetAfter?.clusterDefinitionVersion} after the rename does not move the workload)`);
    mark('post-restart-operation');

    // ===================== NEGATIVES (no side effects) =====================
    phase('PREVIEW', 'Kiểm tra phủ định: ứng dụng khác không khớp cấu hình cluster nào và hai cấu hình đồng hạng đều bị chặn ngay khi xem trước, không tạo gì và không rơi về cluster khác');
    await signOut();
    await signIn('developer');
    await h.click(page.getByRole('button', { name: '+ Tạo ứng dụng' }).first());
    await h.type(page.getByLabel('Tên ứng dụng', { exact: true }), input.negativeApplication.name);
    await h.click(page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }));
    await expect(page.getByRole('heading', { name: input.negativeApplication.name })).toBeVisible();
    const negativeKey = (await get('/applications')).applications.find((a) => a.name === input.negativeApplication.name)?.key;
    // The negative Application is configured exactly like the main one (PORT and BACKEND_URL), so its Score is valid and the
    // workload and namespace Definitions (which match any Application) apply: the cluster Definition is the only missing piece.
    await openSettings();
    await addVariable('PORT', '8080');
    await addVariable('BACKEND_URL', input.backendUrl.a);
    await leaveSettings(input.negativeApplication.name);
    const negativeNames = ((await get(`/applications/${negativeKey}/environments/staging/configuration`)).keys ?? []).map((k) => k.name);
    const beforeNamespaces = JSON.stringify((await technicalJson(['get', 'namespaces', '-o', 'json'])).items.map((i) => i.metadata.uid).sort());
    const negativeOutcome = async (tag) => {
      const reply = await scorePreview(input.scoreFiles.a, tag);
      await expect(alertBox()).toBeVisible({ timeout: 60_000 });
      return { ...reply, alert: await alertBox().innerText(), resultShown: (await page.getByLabel('Kết quả xem trước Score').count()) > 0 };
    };
    const exactReason = (reply, reason) => reply.status === 400 && reply.body?.code === 'PLANNING_REJECTED' && reply.body?.params?.reason === reason && reply.body?.params?.stage === 'graph' && !reply.resultShown && !reply.body?.graph;
    const noMatch = await negativeOutcome('neg-none');
    rec.record('negative.prerequisites-valid', 'PREVIEW', 'ui', negativeNames.includes('PORT') && negativeNames.includes('BACKEND_URL') && planned.status === 200,
      `the negative Application has the same configuration keys (PORT, BACKEND_URL) and the same Score previews successfully on the main Application, so a failure can only come from the cluster Definition`);
    rec.record('negative.no-cluster-match', 'PREVIEW', 'ui', exactReason(noMatch, 'resource-no-match'),
      `an Application with no matching cluster Definition: HTTP ${noMatch.status} ${noMatch.body?.code} reason ${noMatch.body?.params?.reason} stage ${noMatch.body?.params?.stage}; no plan, no fallback cluster`);
    await backToApplication(input.negativeApplication.name);
    // Labelled negative fixture (API, not a UI claim): two equally specific cluster Definitions for the negative Application.
    apiSetup(`negative fixture: two equal cluster Definitions for application ${negativeKey}`);
    const forbidden = (await page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: { key: 'dev-attempt', name: 'x', resourceType: 'k8s-cluster', driverType: 'existing-cluster', criteria: [{}] } })).status();
    expect(forbidden, 'a Developer cannot register Definitions').toBe(403);
    await signOut();
    await signIn('platform-engineer');
    // Labelled read-only technical check (API): the Definition matcher itself, for the negative Application's cluster node.
    const matchingProbe = async (key, criteria) => (await page.request.post(`${baseURL}/api/v1/resource-definitions/matching-preview`, { data: {
      definition: { key, name: key, resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: accountKey, driverInputs: { values: { variables: {} } }, criteria },
      context: { applicationId: negativeKey, environmentId: 'staging', resourceId: `environments.${negativeKey}.staging`, class: 'internal' },
    } })).json();
    const noMatchProbe = await matchingProbe('probe-elsewhere', [{ app_id: 'nobody', class: 'internal' }]);
    rec.record('negative.no-cluster-match.matcher', 'PREVIEW', 'api', noMatchProbe.status === 'NO_MATCH' && !noMatchProbe.winner && (noMatchProbe.candidates ?? []).every((c) => !c.matched),
      `matcher for the negative Application's cluster node: ${noMatchProbe.status}, winner none (read-only matching preview, labelled API)`);
    for (const id of [ids.rivalA, ids.rivalB]) {
      const response = await page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: { key: id, name: id, resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: accountKey, driverInputs: { values: { variables: {} } }, criteria: [{ app_id: negativeKey, class: 'internal' }] } });
      expect(response.status(), 'negative fixture Definition').toBe(201);
    }
    const tieProbe = await matchingProbe('probe-third', [{ app_id: negativeKey, class: 'internal' }]);
    rec.record('negative.tied-cluster-definitions.matcher', 'PREVIEW', 'api',
      tieProbe.status === 'AMBIGUOUS' && !tieProbe.winner && JSON.stringify(tieProbe.ties) === JSON.stringify([ids.rivalA, ids.rivalB, 'probe-third'].sort()),
      `matcher for the same node with equally specific Definitions: ${tieProbe.status}, ties ${JSON.stringify(tieProbe.ties)}, winner none (read-only matching preview, labelled API)`);
    const unknownAccount = await page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: { key: 'ghost-cluster', name: 'ghost', resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: 'no-such-account', driverInputs: { values: { variables: {} } }, criteria: [{ class: 'internal', app_id: negativeKey }] } });
    rec.record('negative.unknown-account-refused', 'PREVIEW', 'api', unknownAccount.status() === 400, `registering a cluster Definition with an unknown account -> HTTP ${unknownAccount.status()} (labelled API negative)`);
    await signOut();
    await signIn('developer');
    await openApplication(input.negativeApplication.name);
    const tie = await negativeOutcome('neg-tie');
    rec.record('negative.tied-cluster-definitions', 'PREVIEW', 'ui', exactReason(tie, 'resource-ambiguous'),
      `two equally specific cluster Definitions: HTTP ${tie.status} ${tie.body?.code} reason ${tie.body?.params?.reason} stage ${tie.body?.params?.stage}; neither is chosen and no fallback is used`);
    const afterNamespaces = JSON.stringify((await technicalJson(['get', 'namespaces', '-o', 'json'])).items.map((i) => i.metadata.uid).sort());
    rec.record('negative.no-side-effects', 'PREVIEW', 'private-read', beforeNamespaces === afterNamespaces, 'the cluster namespace set (UIDs) is identical before and after the failed previews');
    mark('negatives');
    await h.pause(SHORT_READ);
    await signOut();

    // ===================== CLEANUP =====================
    phase('CLEANUP', 'Gỡ workload qua giao diện (xem trước rồi triển khai), rồi Kỹ sư nền tảng xóa hai namespace do orchestrator sở hữu; cluster có sẵn không bao giờ nằm trong danh sách xóa');
    await signIn('developer');
    await openApplication();
    const frontendRow = page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: /frontend$/ }) });
    const expectedConfirm = 'Đánh dấu xóa frontend trong staging? Workload đang chạy chỉ thay đổi sau khi Triển khai.';
    const opened = new Promise((resolve) => page.once('dialog', resolve));
    const clicking = h.click(frontendRow.getByRole('button', { name: 'Xóa', exact: true }));
    const dialogSeen = await opened;
    if (dialogSeen.message() !== expectedConfirm) { x11.keys(['Escape']); await clicking; throw new Error('the removal confirmation did not match the expected Application/Environment'); }
    await h.pause(SHORT_READ);
    x11.keys(['Return']);
    await clicking;
    await expect(frontendRow).toContainText('Chờ xóa');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await deploy();
    await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công', { timeout: input.timeouts.deployMs });
    const claimedKinds = 'deployments,replicasets,pods,services,ingresses,configmaps';
    const leftovers = async () => (await technicalJson(['-n', input.namespace, 'get', claimedKinds, '-o', 'json'])).items.filter((item) => !(item.kind === 'ConfigMap' && item.metadata.name === 'kube-root-ca.crt')).map((item) => `${item.kind}/${item.metadata.name}`);
    const gone = await waitFor(async () => (await leftovers()).length === 0, { timeoutMs: 180_000, intervalMs: 3000, what: 'every workload object (Deployment, ReplicaSet, Pod, Service, Ingress) to be removed' });
    rec.record('cleanup.workload-removed', 'CLEANUP', 'private-read', gone, `no ${claimedKinds} object of the workload remains in ${input.namespace} (only the platform-generated root CA ConfigMap is excluded)`);
    mark('workload-removed');
    await forwarding.stop();
    await appTab.close().catch(() => {});
    await signOut();

    await signIn('platform-engineer');
    await openApplication();
    const resources = page.getByRole('region', { name: 'Tài nguyên của môi trường' });
    // The list shows each descriptor in its own <strong>: match it exactly (the main namespace descriptor is a prefix of the companion's).
    const entry = (descriptor) => resources.locator('button.deployment-entry').filter({ has: page.locator('strong', { hasText: new RegExp(exactTextPattern(descriptor)) }) });
    const mainDescriptor = `k8s-namespace.default#environments.${input.applicationKey}.staging`;
    const auxDescriptor = `${mainDescriptor}-aux`;
    const statuses = await waitFor(async () => {
      const list = ((await get(`${appPath}/resources`)).resources ?? []).map((r) => ({ descriptor: r.descriptor, status: r.status }));
      return list.find((r) => r.descriptor === mainDescriptor)?.status === 'UNREFERENCED' && list.find((r) => r.descriptor === auxDescriptor)?.status === 'UNREFERENCED' ? list : false;
    }, { timeoutMs: 90_000, intervalMs: 2000, what: 'both namespaces to become UNREFERENCED' });
    rec.record('cleanup.unreferenced', 'CLEANUP', 'ui', Boolean(statuses), 'both owned namespaces are retained and marked UNREFERENCED; nothing was destroyed by the removal');
    for (const descriptor of [mainDescriptor, auxDescriptor]) {
      await h.click(entry(descriptor));
      await expect(page.getByTestId('resource-name')).toHaveText(descriptor);
      await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      // The existing cluster may be LISTED as a non-selectable dependency (with its reason) but is never selectable and never in the delete set.
      const clusterBoxes = dialog.locator('li', { hasText: 'k8s-cluster' }).locator('input[type="checkbox"]');
      const clusterSelectable = (await clusterBoxes.evaluateAll((boxes) => boxes.filter((box) => !box.disabled).length)) > 0;
      const deleteSet = await dialog.getByRole('region', { name: 'Các tài nguyên sẽ bị xóa' }).innerText();
      rec.record(`cleanup.platform-excluded:${descriptor.endsWith('-aux') ? 'aux' : 'main'}`, 'CLEANUP', 'ui', !clusterSelectable && !deleteSet.includes('k8s-cluster'), 'the existing cluster is not selectable (any checkbox for it is disabled) and is not in the delete set');
      const nameField = dialog.getByLabel('Nhập lại tên tài nguyên');
      // The dialog shows the technical name to retype; it is read from the dialog itself and must equal the descriptor.
      const confirmationName = (await dialog.getByTestId('root-name').innerText()).trim();
      expect(confirmationName, 'the dialog asks to retype the selected resource').toBe(descriptor);
      await h.type(nameField, confirmationName, { delay: 25, replace: true });
      await h.click(dialog.getByRole('button', { name: 'Xác nhận xóa vĩnh viễn', exact: true }));
      await expect(dialog).toHaveCount(0);
      const progressPanel = page.getByTestId('cleanup-progress');
      await expect(progressPanel).toContainText('Tất cả tài nguyên đã chọn đã được xác minh là không còn', { timeout: input.timeouts.cleanupMs });
      await h.pause(SHORT_READ);
      await backToApplication();
    }
    let absentOut = '';
    try { absentOut = v2ctl(input, 'v03-absent', ...kubeFlags(input), '-baseline', input.baselineFile, '-owned', input.ownedFile); } catch (error) { absentOut = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.namespaces-absent', 'CLEANUP', 'private-read', !absentOut.includes('FAIL') && (await namespaceUid(input.namespace)) === '' && (await namespaceUid(input.companionNamespace)) === '', absentOut.trim() || 'run namespaces absent; baseline namespaces unchanged');
    const accountAfter = ((await get('/driver-accounts')).accounts ?? []).find((a) => a.key === accountKey);
    const nodes = await technicalJson(['get', 'nodes', '-o', 'json']);
    rec.record('cleanup.cluster-preserved', 'CLEANUP', 'private-read', nodes.items.length >= 1 && accountAfter?.status === 'READY', 'the existing cluster is reachable and its Internal account is still READY; neither was created or deleted by the run');
    await noLeak('final page');
    mark('namespaces-cleaned');
    await signOut();
    mark('signed-out');
    await h.pause(2500);
  },
};

export function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--scenario') args.scenario = argv[++i] ?? '';
    else if (argv[i] === '--captions') args.captions = argv[++i] ?? '';
    else if (argv[i] === '--evidence') args.evidence = argv[++i] ?? '';
    else if (argv[i] === '--headed') args.headed = true;
    else if (argv[i] === '--check') args.check = argv[++i] ?? '';
    else if (argv[i] === '--verify-evidence') args.verify = argv[++i] ?? '';
    else throw new Error(`unknown argument ${argv[i]}`);
  }
  return args;
}

export function requireScenario(id) {
  if (!Object.hasOwn(SCENARIOS, id)) throw new Error(`unsupported scenario "${id}"; supported: ${Object.keys(SCENARIOS).join(', ')}`);
  return SCENARIOS[id];
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.check !== undefined) { requireScenario(args.check); return; }
  if (args.verify !== undefined) {
    const dir = args.verify;
    const problems = verifyEvidence({
      assertions: JSON.parse(readFileSync(`${dir}/assertions.json`, 'utf8')), phases: JSON.parse(readFileSync(`${dir}/phases.json`, 'utf8')),
      assertionsText: readFileSync(`${dir}/assertions.txt`, 'utf8'),
    });
    if (problems.length) { console.error(problems.join('\n')); process.exit(1); }
    console.log('evidence complete');
    return;
  }
  const scenario = requireScenario(args.scenario);
  if (!args.headed) throw new Error('--headed is required: recordings use a headed browser');
  requireCaptionLocale(args.captions);
  const env = process.env;
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_INPUT', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_RESTART_CMD', 'DISPLAY']) if (!env[name]) throw new Error(`${name} is required`);
  const baseURL = env.ORCH_E2E_URL;
  const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
  const { width, height } = screenSize(env);
  const videoPath = `${evidenceDir}/${args.scenario}-raw.mp4`;
  const assertionsPath = `${evidenceDir}/assertions.txt`;
  const browser = await launchHeaded({ width, height });
  const context = await browser.newContext({ viewport: null });
  context.setDefaultTimeout(30_000);
  await context.addInitScript(installCursor);
  const marks = [];
  const forwarders = [];
  let input;
  let recorder;
  let startedAt;
  let currentPage;
  let result = 'FAIL';
  const secondsNow = () => (startedAt ? Number(((Date.now() - startedAt) / 1000).toFixed(1)) : 0);
  const captions = createCaptions(secondsNow);
  // V03 labels its few API-driven negative checks 'api' (never counted as UI observation; at least one assertion must still be 'ui').
  const rec = createRecorder({ dir: evidenceDir, now: Date.now, startedAt: () => startedAt ?? Date.now(), extraSources: ['api'] });
  const origRecord = rec.record;
  rec.record = (id, phaseName, source, pass, detail) => {
    appendFileSync(assertionsPath, `${pass ? 'PASS' : 'FAIL'} ${id} [${source}] ${String(detail).slice(0, 220)}\n`);
    return origRecord(id, phaseName, source, pass, detail);
  };
  let finalized = false;
  const finalize = async () => {
    if (finalized) return;
    finalized = true;
    for (const f of forwarders) f.stop();
    try {
      if (recorder) await stopRecording(recorder);
    } finally {
      const total = startedAt ? (Date.now() - startedAt) / 1000 : 0;
      writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
      if (startedAt) captions.writeSrt(`${evidenceDir}/captions.srt`, total);
      writeFileSync(`${evidenceDir}/result.txt`, `${result}\n`);
      await browser.close();
    }
  };
  for (const [signal, code] of [['SIGHUP', 129], ['SIGINT', 130], ['SIGTERM', 143]]) {
    process.once(signal, () => {
      appendFileSync(assertionsPath, `FAIL interrupted by ${signal}\n`);
      finalize().catch((error) => console.error(safeError(error))).finally(() => process.exit(code));
    });
  }
  try {
    // The runner arms its safety net first and only then releases the gate, so the first create can never precede it.
    input = await readScenarioInputAfterGate(env.ORCH_E2E_INPUT, env.ORCH_E2E_START_GATE);
    const page = await context.newPage();
    currentPage = page;
    const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
    page.on('dialog', () => {}); // keep native confirmations open until an X11 key answers them
    const h = createHuman(page, { selectNative: nativeSelect(x11) });
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
    x11.focusBrowser();
    recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
    startedAt = recorder.spawnedAtMs;
    await page.waitForTimeout(1500);
    const mark = (label) => marks.push({ label, seconds: secondsNow() });
    const phase = (name, text) => { mark(name.toLowerCase()); captions.caption(text); rec.phase(name, text); };
    await scenario({ page, h, context, x11, caption: captions.caption, phase, mark, rec, baseURL, input, forwarders, evidenceDir });
    const recorded = readFileSync(assertionsPath, 'utf8');
    if (/^FAIL /m.test(recorded)) throw new Error('FAIL assertions were recorded during the run');
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
    const text = safeError(error);
    appendFileSync(assertionsPath, `FAIL ${text}\n`);
    console.error(String(error?.stack ?? error).split('\n').slice(0, 6).join('\n'));
    try { if (currentPage) await currentPage.screenshot({ path: `${evidenceDir}/failure.png`, timeout: 10_000 }); } catch { /* the page may be closed */ }
    process.exitCode = 1;
  } finally {
    await finalize();
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => { console.error(safeError(error)); process.exit(2); });
}
