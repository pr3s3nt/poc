// Real Terraform v2 acceptance scenarios (M04 I04-10): V2-A01, V2-E01, V2-E02
// (V2-A02/A03: AWS, terraform-v2-aws.mjs; live execution only after the I04-11 gate). Invoked by
// backend/test/integration/terraform-v2-acceptance.sh, which starts the REAL
// product (planner -> pinned Terraform with encrypted PostgreSQL state ->
// typed outputs and durable Vault Secret refs -> required pinned score-k8s ->
// real Kubernetes delivery), the independent cleanup supervisor and the
// headed browser. Every business step is a Web Console or acceptance-app UI
// action; the scripts below only READ real resources privately (kubectl,
// psql, redis-cli inside the pods) and never create a job, a Workload or a
// resource. A FAKE or interception seam is not allowed anywhere in this file.
//   node terraform-v2-acceptance.mjs --check A01
//   node terraform-v2-acceptance.mjs --scenario A01 --headed --captions vi --evidence DIR
// Marks, phases, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor, putKeys } from './human.mjs';
import { createRecorder, kubectlRunner, nonexistentDigest, pickByImage, registryDigestStatus, safeError, startPortForward, waitFor } from './terraform-v2-lib.mjs';
import { ACCEPTANCE, REDIS_ANNOTATION, SHORT_READ, steps } from './terraform-v2-steps.mjs';
import { awsScenario } from './terraform-v2-aws.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const FORBIDDEN_IN_PAGE = [/AKIA/, /vault:\/\//i, /tfstate/i, /kubeconfig/i, /secretRef/i, /\/var\/lib/, /\.terraform/, /recoveryPath/i, /BEGIN [A-Z ]*PRIVATE KEY/];

/** Everything a scenario needs besides the page: private technical access. */
function technical(input) {
  const kubectl = kubectlRunner(input.kube);
  const ns = input.namespace;
  const json = async (args) => {
    const r = await kubectl(args);
    if (r.code !== 0) throw new Error(`kubectl ${args[0]} failed`);
    return JSON.parse(r.stdout);
  };
  const secretValue = async (name, key) => {
    const r = await kubectl(['-n', ns, 'get', 'secret', name, '-o', `jsonpath={.data.${key}}`]);
    if (r.code !== 0 || !r.stdout) throw new Error('secret value is not readable');
    return Buffer.from(r.stdout, 'base64').toString('utf8');
  };
  // The password is read privately and passed only to the exec'd client; it is never logged.
  const envOf = (deployment, name) => (deployment.spec.template.spec.containers[0].env ?? []).find((e) => e.name === name);
  async function resolvedEnv(deployment, name) {
    const entry = envOf(deployment, name);
    if (!entry) throw new Error(`${name} is not bound`);
    if (entry.value !== undefined) return entry.value;
    const ref = entry.valueFrom?.secretKeyRef;
    if (!ref) throw new Error(`${name} has no value source`);
    return secretValue(ref.name, ref.key);
  }
  return {
    kubectl, json, envOf, resolvedEnv,
    namespaceExists: async () => (await kubectl(['get', 'namespace', ns, '-o', 'name'])).code === 0,
    deployments: () => json(['-n', ns, 'get', 'deployments', '-o', 'json']),
    statefulsets: () => json(['-n', ns, 'get', 'statefulsets', '-o', 'json']),
    pods: () => json(['-n', ns, 'get', 'pods', '-o', 'json']),
    async psql(deployment, sql) {
      const pg = pickByImage(await this.statefulsets(), 'postgres')[0];
      if (!pg) throw new Error('no PostgreSQL StatefulSet');
      const [user, db, password] = await Promise.all([resolvedEnv(deployment, 'PGUSER'), resolvedEnv(deployment, 'PGDATABASE'), resolvedEnv(deployment, 'PGPASSWORD')]);
      // The password travels on stdin only: never in the kubectl argv, a log or an error text.
      const script = 'IFS= read -r PGPASSWORD; export PGPASSWORD; exec psql -h 127.0.0.1 -U "$0" -d "$1" -tAF "|" -c "$2"';
      const r = await kubectl(['-n', ns, 'exec', '-i', `statefulset/${pg.metadata.name}`, '--', 'sh', '-c', script, user, db, sql], { timeoutMs: 60_000, input: `${password}\n` });
      if (r.code !== 0) throw new Error('psql query failed');
      return r.stdout.trim();
    },
    async redis(deployment, ...command) {
      const rd = pickByImage(await this.statefulsets(), 'redis')[0];
      if (!rd) throw new Error('no Redis StatefulSet');
      const [user, password] = await Promise.all([resolvedEnv(deployment, 'REDIS_USERNAME'), resolvedEnv(deployment, 'REDIS_PASSWORD')]);
      const script = 'IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; exec redis-cli --no-auth-warning --user "$0" "$@"';
      const r = await kubectl(['-n', ns, 'exec', '-i', `statefulset/${rd.metadata.name}`, '--', 'sh', '-c', script, user, ...command], { timeoutMs: 30_000, input: `${password}\n` });
      if (r.code !== 0) throw new Error('redis-cli failed');
      return r.stdout.trim();
    },
  };
}

/**
 * Known run-created secret values go to a 0600 private scan input, as name=value
 * lines, BEFORE any public log or caption is scanned. They are never printed.
 */
function recordAppSecret(input, name, value) {
  appendFileSync(`${input.privateDir}/app-secrets.txt`, `${name}=${value}\n`, { mode: 0o600 });
}

function v2ctl(input, ...args) {
  return execFileSync(input.v2ctl, args, { encoding: 'utf8', env: { PATH: process.env.PATH, V2_KUBECTL: input.kube.kubectl, V2_KUBECONFIG: input.kube.kubeconfig, V2_KUBECONTEXT: input.kube.context }, stdio: ['ignore', 'pipe', 'pipe'] });
}

/** Records the Environment's exact physical namespace in the private manifest (never a guessed name) and checks it is the run namespace. */
function recordNamespace(input, env) {
  const exact = v2ctl(input, 'manifest-namespace', '-manifest', input.manifestFile, '-identity', env.namespaceIdentity, '-generation', String(env.targetGeneration ?? 0)).trim();
  if (exact !== input.namespace) throw new Error('the Environment namespace differs from the run namespace of the manifest');
  return exact;
}

/** Records the namespace and every PV claimed from it as run-owned assets (exact names, never a pattern) before anything is cleaned up. */
async function recordOwnedAssets(input, tech) {
  const ctl = (...args) => v2ctl(input, 'manifest-asset', '-manifest', input.manifestFile, ...args);
  ctl('-kind', 'namespace', '-id', input.namespace, '-layer', 'namespace', '-owner', 'CREATED_BY_RUN');
  const volumes = await tech.json(['get', 'pv', '-o', 'json']);
  for (const pv of volumes.items) if (pv.spec?.claimRef?.namespace === input.namespace) ctl('-kind', 'pv', '-id', pv.metadata.name, '-layer', 'app-resources', '-owner', 'CREATED_BY_RUN', '-linkage', `pvc ${pv.spec.claimRef.name} in ${input.namespace}`);
}

export const SCENARIOS = {
  async A01(ctx) {
    const { page, h, caption, rec, input } = ctx;
    const s = steps(ctx);
    const tech = technical(input);
    const phase = ctx.phase;
    const scope = `/applications/${input.applicationKey}/environments/staging`;
    const forwarders = ctx.forwarders;
    const pageIsSafe = async (label) => {
      const text = await page.locator('body').innerText();
      for (const pattern of FORBIDDEN_IN_PAGE) expect(text, `${label}: ${pattern}`).not.toMatch(pattern);
    };

    // ===================== PREPARE / FORM: Platform Engineer =====================
    phase('PREPARE', 'Chuẩn bị: Kỹ sư nền tảng đăng nhập; danh mục, Kết nối và Mẫu dựng đều bắt đầu trống và sẽ được tạo bằng giao diện');
    await h.showCursor();
    await expectVisibleCursor(page);
    await s.signIn('platform-engineer');
    rec.record('ui.signin', 'PREPARE', 'ui', true, 'platform-engineer');
    await h.pause(SHORT_READ);
    await s.registerKubernetesConnection();

    phase('FORM', 'Biểu mẫu: đăng ký Loại tài nguyên, Cấu hình tài nguyên (module đã ghim) và Mẫu dựng score-k8s');
    await s.registerTypes();
    const { defs, pinned, capabilities } = await s.registerDefinitions({ redisOverrides: [['annotations', REDIS_ANNOTATION]] });
    await s.registerRenderTemplate();
    const storeToken = readFileSync(input.secretStore.tokenFile, 'utf8').trim();
    const storeKey = await s.registerSecretStore(input.secretStore, storeToken);
    rec.record('ui.definitions.registered', 'FORM', 'ui', ['namespace-k8s-module', 'pg-in-cluster', 'redis-in-cluster'].every((k) => defs.includes(k)) && pinned, `definitions ${defs.join(',')} pinned=${pinned}`);
    await s.signOut();

    // ===================== FORM: Developer =====================
    await s.signIn('developer');
    const env = await s.bindEnvironment();
    recordNamespace(input, env);
    // A real, scoped Vault secret through the Console: masked input, the Environment's secret store, the stored value never shown.
    // The nonsecret expected SHA-256 is a plain variable; the app compares the bound secret with it.
    const appSecret = randomBytes(24).toString('hex');
    const appSecretSha = createHash('sha256').update(appSecret).digest('hex');
    recordAppSecret(input, 'ACCEPTANCE_SECRET', appSecret);
    await putKeys(h, input.applicationName, [
      ...[['PORT', '8080'], ['POLL_INTERVAL_MS', '500'], ['ACCEPTANCE_CONFIG', 'acceptance-config-ok'], ['ACCEPTANCE_SECRET_SHA256', appSecretSha],
        ['ACCEPTANCE_CACHE_APP', input.applicationKey], ['ACCEPTANCE_CACHE_ENV', 'staging'], ['ACCEPTANCE_CACHE_RUN', input.runId], ['ACCEPTANCE_CACHE_TTL_SECONDS', String(input.cache.ttlSeconds)]]
        .map(([name, value]) => ({ kind: 'variable', name, value })),
      { kind: 'secret', name: 'ACCEPTANCE_SECRET', value: appSecret, paste: true },
    ], { secretStore: storeKey });
    rec.record('ui.secret.configured', 'FORM', 'ui', !(await page.locator('body').innerText()).includes(appSecret), 'secret stored through the Environment secret store; value never visible; SHA-256 stored as a variable');
    await s.addAcceptanceWorkloads();
    const drafts = (await s.get(`${scope}/workloads`)).workloads ?? [];
    rec.record('ui.workloads.three-saved', 'FORM', 'ui', ['backend', 'worker', 'frontend'].every((n) => drafts.some((w) => w.id === n)), 'three Workload drafts saved');

    // ===================== PREVIEW (pure) =====================
    phase('PREVIEW', 'Xem trước: kiểm tra Definition, module đã ghim, đích, hồ sơ, quan hệ provider → namespace → PostgreSQL → Redis và bundle score-k8s');
    expect(await tech.namespaceExists(), 'the run namespace must not exist before Deploy').toBe(false);
    const previewResponse = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(`${scope}/preview`));
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    const preview = await (await previewResponse).json();
    const panel = page.getByLabel('Xem trước triển khai');
    await expect(panel).toContainText('3 Workload bị ảnh hưởng', { timeout: 120_000 });
    await h.moveTo(panel);
    await h.pause(SHORT_READ);
    const intents = new Map();
    for (const change of preview.changes) for (const intent of change.resourceIntents ?? []) intents.set(intent.descriptor, intent);
    const byType = (type) => [...intents.values()].filter((i) => i.resourceType === type);
    const clusterDescriptor = `k8s-cluster.internal#connections.${input.connection.name}`;
    const terraformIntents = [...intents.values()].filter((i) => i.driverType === 'terraform');
    const pinOf = (module) => capabilities.find((c) => c.driverType === 'terraform' && c.module === module)?.source;
    const sameDescription = await panel.innerText();
    const noLeak = FORBIDDEN_IN_PAGE.concat([/password/i, /secret:\/\//i]).every((p) => !p.test(sameDescription.replace(/bí mật/g, '')));
    const noState = !(await tech.namespaceExists()) && ((await s.get(`${scope}/deployments`)).deployments ?? []).length === 0 && ((await s.get(`${scope}/resources`)).resources ?? []).length === 0;
    rec.record('preview.pure', 'PREVIEW', 'private-read', noState && noLeak, 'after Preview: no namespace, no Deployment and no resource record exists (so no Terraform state), no secret text on screen');
    rec.record('preview.target', 'PREVIEW', 'ui', preview.binding?.connectionKey === input.connection.name && preview.binding?.executionProfile === 'internal-k8s' && !preview.accountId, `connection ${preview.binding?.connectionKey} profile ${preview.binding?.executionProfile}`);
    const pinsOk = terraformIntents.length === 3 && terraformIntents.every((i) => {
      const pin = pinOf(i.module?.module);
      return pin && i.module.contentSha256 === pin.contentSha256 && i.module.revision === pin.revision;
    });
    rec.record('preview.pins', 'PREVIEW', 'ui', pinsOk, terraformIntents.map((i) => `${i.module?.module}@${String(i.module?.contentSha256).slice(0, 12)}`).join(' '));
    const graphOk = byType('k8s-namespace').length === 1 && byType('postgres').length === 1 && byType('redis').length === 1
      && byType('postgres')[0].descriptor.endsWith(`shared.${ACCEPTANCE.dbId}`) && byType('redis')[0].descriptor.endsWith(`shared.${ACCEPTANCE.cacheId}`)
      && terraformIntents.every((i) => i.intent === 'CREATE' && i.ownership === 'ORCHESTRATOR' && (i.providers ?? []).some((p) => p.name === 'kubernetes' && p.target === 'environment' && (p.provider === clusterDescriptor || p.connectionKey === input.connection.name)));
    rec.record('preview.graph', 'PREVIEW', 'ui', graphOk, `${[...intents.keys()].length} descriptors; one shared PostgreSQL and one shared Redis; Kubernetes provider bound to the Environment cluster`);
    const bundles = preview.changes.map((c) => c.rendering?.bundle);
    rec.record('preview.score-k8s-bundle', 'PREVIEW', 'ui', preview.changes.length === 3 && bundles.every((b) => b?.id === 'score-k8s-internal-v1' && b.version === input.scoreVersion), `bundle ${bundles[0]?.id} ${bundles[0]?.version}`);
    await h.read('Xem trước chỉ đọc: chưa có namespace hay Deployment nào được tạo');

    // ===================== TERRAFORM / READY =====================
    phase('TERRAFORM', 'Triển khai: Terraform tạo namespace, PostgreSQL, Redis theo thứ tự rồi score-k8s phát hành ba Workload; tiến trình lấy từ dữ liệu đã lưu');
    await ctx.deploy();
    const result = page.getByLabel('Kết quả triển khai');
    await expect(result).toContainText('Triển khai thành công', { timeout: input.deployTimeoutMs });
    for (const name of ['backend', 'worker', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [^:]+: thành công`) })).toHaveCount(1);
    await h.moveTo(result);
    await h.pause(SHORT_READ);
    const deployments = (await s.get(`${scope}/deployments`)).deployments ?? [];
    const progressRows = [];
    for (const d of deployments) progressRows.push(...((await s.get(`${scope}/deployments/${d.id}`)).resourceProgress ?? []));
    const stagesOk = progressRows.length >= 3 && progressRows.filter((r) => r.stage === 'READY' && r.status === 'SUCCEEDED').length >= 3;
    await h.click(page.getByRole('button', { name: 'Xem tất cả', exact: true }));
    await h.click(page.locator('button.deployment-entry').first());
    const progress = page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' });
    await expect(progress).toBeVisible();
    await h.moveTo(progress);
    await h.pause(SHORT_READ);
    await pageIsSafe('deployment progress');
    rec.record('deploy.terraform-stages', 'TERRAFORM', 'ui', stagesOk, `${progressRows.length} persisted attempt rows, READY/SUCCEEDED ${progressRows.filter((r) => r.stage === 'READY').length}`);

    phase('READY', 'Sẵn sàng: cả ba Workload Ready trên k8s-4f; chỉ một PostgreSQL và một Redis dùng chung; mật khẩu đi qua Secret, không nằm trong giá trị của Pod');
    const names = { backend: 'backend', worker: 'worker', frontend: 'frontend' };
    const ready = await waitFor(async () => {
      const list = await tech.deployments();
      const byName = Object.fromEntries(list.items.map((d) => [d.metadata.name, d]));
      return Object.keys(names).every((n) => byName[n] && (byName[n].status?.readyReplicas ?? 0) >= 1 && byName[n].status.readyReplicas === byName[n].spec.replicas) ? byName : false;
    }, { timeoutMs: input.readyTimeoutMs, intervalMs: 3000, what: 'the three Deployments to be Ready' });
    rec.record('ready.workloads', 'READY', 'private-read', true, 'backend, worker, frontend Ready');
    // The generated application passwords join the private scan input (never printed).
    recordAppSecret(input, 'PGPASSWORD', await tech.resolvedEnv(ready.backend, 'PGPASSWORD'));
    recordAppSecret(input, 'REDIS_PASSWORD', await tech.resolvedEnv(ready.backend, 'REDIS_PASSWORD'));
    const sts = await tech.statefulsets();
    const pgs = pickByImage(sts, 'postgres');
    const redises = pickByImage(sts, 'redis');
    const [bHost, wHost, bRedis, wRedis] = await Promise.all([tech.resolvedEnv(ready.backend, 'PGHOST'), tech.resolvedEnv(ready.worker, 'PGHOST'), tech.resolvedEnv(ready.backend, 'REDIS_HOST'), tech.resolvedEnv(ready.worker, 'REDIS_HOST')]);
    rec.record('ready.single-postgres', 'READY', 'private-read', pgs.length === 1 && bHost === wHost && bHost.length > 0, 'one PostgreSQL StatefulSet; backend and worker use the same host');
    rec.record('ready.single-redis', 'READY', 'private-read', redises.length === 1 && bRedis === wRedis && bRedis.length > 0, 'one Redis StatefulSet; backend and worker use the same host');
    const secretBound = ['PGPASSWORD', 'REDIS_PASSWORD'].every((n) => ['backend', 'worker'].every((w) => { const e = tech.envOf(ready[w], n); return e?.value === undefined && e?.valueFrom?.secretKeyRef; }));
    const plainBound = ['PGHOST', 'PGPORT', 'PGDATABASE', 'PGUSER', 'REDIS_HOST', 'REDIS_PORT', 'REDIS_USERNAME', 'REDIS_TLS', 'ACCEPTANCE_CACHE_RUN', 'ACCEPTANCE_CACHE_TTL_SECONDS'].every((n) => ['backend', 'worker'].every((w) => tech.envOf(ready[w], n)));
    rec.record('ready.bindings', 'READY', 'private-read', secretBound && plainBound && (await tech.namespaceExists()), 'passwords via secretKeyRef (no literal value); hosts, ports, usernames, scope bound');

    await recordOwnedAssets(input, tech);

    // ===================== JOB / CACHE (acceptance app UI) =====================
    phase('JOB', 'Công việc: mở frontend vừa triển khai (port-forward), gửi một job bằng form có sẵn rồi chờ worker xử lý xong');
    const services = (await tech.json(['-n', input.namespace, 'get', 'services', '-o', 'json'])).items.map((svc) => svc.metadata.name);
    const frontendService = services.find((n) => n === 'frontend') ?? services.find((n) => n.startsWith('frontend'));
    if (!frontendService) throw new Error('the frontend Service was not delivered');
    const forward = startPortForward({ ...input.kube, namespace: input.namespace, service: frontendService, remotePort: 8080, localPort: input.forwardPort });
    forwarders.push(forward);
    await waitFor(async () => (await fetch(`${forward.url}/healthz`)).ok, { timeoutMs: 120_000, intervalMs: 1000, what: 'the frontend port-forward' });
    const app = await ctx.context.newPage();
    const ah = createHuman(app, { selectNative: ctx.selectNative });
    await app.goto(forward.url);
    await expect(app.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
    await ah.showCursor();
    for (const name of ['backend connection', 'environment', 'secret', 'database', 'cache']) {
      const row = app.locator(`[data-check="${name}"]`);
      await waitFor(async () => { await app.reload(); return (await row.innerText()).includes('PASS'); }, { timeoutMs: 180_000, intervalMs: 3000, what: `the ${name} check` });
    }
    const checksBody = await (await fetch(`${forward.url}/api/checks`)).json();
    rec.record('app.all-rows-pass', 'JOB', 'ui', ['backend connection', 'environment', 'secret', 'database', 'cache'].length === 5 && checksBody.checks?.secret === true && checksBody.checks?.environment === true && checksBody.checks?.database === true && !JSON.stringify(checksBody).includes(appSecret), 'every acceptance app row PASS including the real Vault secret; /api/checks carries no secret value');
    const payload = `v2 acceptance ${input.runId}`;
    await ah.type(app.locator('input[name="payload"]'), payload, { replace: true });
    await ah.click(app.getByRole('button', { name: 'Submit job' }));
    const jobRow = app.locator('#jobs tr.job').filter({ hasText: payload }).first();
    await expect(jobRow).toBeVisible();
    await waitFor(async () => { await app.reload(); return (await jobRow.innerText()).includes('DONE'); }, { timeoutMs: 180_000, intervalMs: 1500, what: 'the worker to complete the job' });
    const jobId = Number(await jobRow.getAttribute('data-id'));
    const cells = (await jobRow.locator('td').allInnerTexts()).map((t) => t.trim());
    const expectedResult = `processed:${payload.toUpperCase()}`;
    await ah.moveTo(jobRow);
    await ah.pause(SHORT_READ);
    rec.record('job.completed-ui', 'JOB', 'ui', cells[2] === 'DONE' && cells[3] === expectedResult && cells[4] !== '', `job ${jobId} DONE, result matches, processed by a worker pod`);

    phase('CACHE', 'Bộ nhớ đệm: worker ghi PostgreSQL rồi ghi Redis; backend đọc cache; sau khi TTL hết, tải lại bằng giao diện để xem fallback PostgreSQL và ghi lại Redis');
    const cacheKey = `acceptance:v1:${input.applicationKey}:staging:${input.runId}:job:${jobId}`;
    const exists = await tech.redis(ready.backend, 'EXISTS', cacheKey);
    const ttl = Number(await tech.redis(ready.backend, 'TTL', cacheKey));
    rec.record('cache.worker-write', 'CACHE', 'private-read', exists === '1' && ttl > 0 && ttl <= input.cache.ttlSeconds, `Redis key exists with TTL ${ttl}s (<= ${input.cache.ttlSeconds}s)`);
    const hit = await (await fetch(`${forward.url}/api/jobs/${jobId}`)).json();
    rec.record('cache.backend-hit', 'CACHE', 'private-read', hit.cache === 'hit' && hit.status === 'DONE', `GET job ${jobId}: ${hit.cache}`);
    const row = (await tech.psql(ready.backend, `SELECT status, result, cache_write FROM jobs WHERE id = ${jobId}`)).split('|');
    rec.record('job.postgres-row', 'JOB', 'private-read', row[0] === 'DONE' && row[1] === expectedResult && row[2] === 'OK', `PostgreSQL row ${jobId}: ${row[0]}, result matches, cache_write ${row[2]}`);
    const before = (await (await fetch(`${forward.url}/api/checks`)).json()).cache.lookups ?? {};
    await waitFor(async () => (await tech.redis(ready.backend, 'EXISTS', cacheKey)) === '0', { timeoutMs: (input.cache.ttlSeconds + 30) * 1000, intervalMs: 1000, what: 'the cache TTL to expire' });
    rec.record('cache.ttl-expiry-miss', 'CACHE', 'private-read', true, `Redis key expired within ${input.cache.ttlSeconds + 30}s`);
    await app.reload();
    const reloaded = app.locator('#jobs tr.job').filter({ hasText: payload }).first();
    const sameCells = (await reloaded.locator('td').allInnerTexts()).map((t) => t.trim());
    await ah.moveTo(reloaded);
    await ah.pause(SHORT_READ);
    const after = (await (await fetch(`${forward.url}/api/checks`)).json()).cache.lookups ?? {};
    const refilled = (after['miss-refilled'] ?? 0) > (before['miss-refilled'] ?? 0);
    const keyBack = (await tech.redis(ready.backend, 'EXISTS', cacheKey)) === '1';
    rec.record('cache.refill', 'CACHE', 'private-read', refilled && keyBack, `miss-refilled ${before['miss-refilled'] ?? 0} -> ${after['miss-refilled'] ?? 0}; key refilled in Redis`);
    rec.record('job.reload-same-result', 'JOB', 'ui', sameCells.join('|') === cells.join('|') && sameCells[3] === expectedResult, 'the reloaded row shows the same job and result (PostgreSQL fallback)');
    await app.close();

    // ===================== CLEANUP: UI deletion step =====================
    phase('CLEANUP', 'Xóa qua giao diện: gỡ cả ba Workload (pending → Xem trước → Triển khai), Redis và PostgreSQL còn lại ở trạng thái không còn được tham chiếu');
    await page.bringToFront();
    await s.backToApplication();
    for (const name of ['backend', 'worker', 'frontend']) {
      const workloadRow = page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) });
      const expected = `Đánh dấu xóa ${name} trong staging? Workload đang chạy chỉ thay đổi sau khi Triển khai.`;
      const opened = new Promise((resolve) => page.once('dialog', resolve));
      const clicking = h.click(workloadRow.getByRole('button', { name: 'Xóa', exact: true }));
      const dialog = await opened;
      if (dialog.message() !== expected) {
        ctx.x11.keys(['Escape']);
        await clicking;
        throw new Error(`the ${name} confirmation did not match the expected Application/Environment`);
      }
      await h.pause(SHORT_READ);
      ctx.x11.keys(['Return']);
      await clicking;
      await expect(workloadRow).toContainText('Chờ xóa');
    }
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('3 Workload bị ảnh hưởng', { timeout: 120_000 });
    await h.pause(SHORT_READ);
    await ctx.deploy();
    await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công', { timeout: input.deployTimeoutMs });
    await h.pause(SHORT_READ);
    const afterRemoval = await waitFor(async () => {
      const left = (await tech.deployments()).items.length === 0;
      return left;
    }, { timeoutMs: 180_000, intervalMs: 3000, what: 'the Workload Deployments to be removed' });
    rec.record('delete.workloads-removed', 'CLEANUP', 'private-read', afterRemoval, 'no Deployment remains in the run namespace');
    const statuses = async () => {
      const list = await s.get(`${scope}/resources`);
      return (list.resources ?? []).map((r) => ({ id: r.id, descriptor: r.descriptor, status: r.status, type: r.type }));
    };
    const redisDescriptor = `redis.in-cluster#shared.${ACCEPTANCE.cacheId}`;
    const pgDescriptor = `postgres.in-cluster#shared.${ACCEPTANCE.dbId}`;
    const afterRemovalStatuses = await waitFor(async () => {
      const list = await statuses();
      const r = list.find((x) => x.descriptor === redisDescriptor);
      const p = list.find((x) => x.descriptor === pgDescriptor);
      return r?.status === 'UNREFERENCED' && p?.status === 'UNREFERENCED' ? list : false;
    }, { timeoutMs: 60_000, intervalMs: 2000, what: 'Redis and PostgreSQL to become UNREFERENCED' });
    const stillThere = pickByImage(await tech.statefulsets(), 'redis').length === 1 && pickByImage(await tech.statefulsets(), 'postgres').length === 1;
    rec.record('delete.resources-unreferenced', 'CLEANUP', 'ui', Boolean(afterRemovalStatuses) && stillThere, 'Redis and PostgreSQL retained and marked UNREFERENCED');
    await s.signOut();

    // Developer has no delete button on the resource detail.
    await s.signIn('developer');
    await s.openApplication();
    const entry = (descriptor) => page.getByRole('region', { name: 'Tài nguyên của môi trường' }).locator('button.deployment-entry', { hasText: descriptor });
    await h.click(entry(redisDescriptor));
    await expect(page.getByTestId('resource-name')).toHaveText(redisDescriptor);
    const devNoButton = (await page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }).count()) === 0;
    rec.record('delete.developer-has-no-button', 'CLEANUP', 'ui', devNoButton, 'Developer sees the resource detail without the delete action');
    await s.signOut();

    await s.signIn('platform-engineer');
    await s.openApplication();
    await h.click(entry(redisDescriptor));
    await expect(page.getByTestId('resource-name')).toHaveText(redisDescriptor);
    await h.pause(SHORT_READ);
    await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
    const dialog = () => page.getByRole('dialog');
    const tick = (descriptor) => dialog().getByRole('checkbox', { name: new RegExp(descriptor.replace(/[.#]/g, '\\$&')) });
    await expect(dialog()).toBeVisible();
    await expect(page.getByTestId('root-name')).toHaveText(redisDescriptor);
    const checkedCount = await dialog().getByRole('checkbox', { checked: true }).count();
    rec.record('delete.root-only-checked', 'CLEANUP', 'ui', checkedCount === 1 && await tick(redisDescriptor).isChecked(), `${checkedCount} item checked by default: the root`);
    const platformShown = await dialog().getByText(clusterDescriptor).count();
    rec.record('delete.platform-excluded', 'CLEANUP', 'ui', platformShown === 0 && (await dialog().getByRole('checkbox', { name: /k8s-cluster/ }).count()) === 0, 'the platform cluster is not offered');
    await h.click(tick(pgDescriptor));
    await expect(tick(pgDescriptor)).toBeChecked();
    rec.record('delete.dependency-selected', 'CLEANUP', 'ui', await tick(pgDescriptor).isChecked(), 'PostgreSQL (a valid dependency) was ticked explicitly');
    const namespaceDescriptor = `k8s-namespace.default#environments.${input.applicationKey}.staging`;
    const nsBox = tick(namespaceDescriptor);
    const candidateLeft = (await nsBox.count()) === 1 && !(await nsBox.isChecked());
    rec.record('delete.unchecked-candidate-left', 'CLEANUP', 'ui', candidateLeft, 'the namespace candidate stays unchecked');
    await expect(dialog()).toContainText('Sẽ xóa 2 tài nguyên');
    await h.pause(SHORT_READ);
    const nameField = dialog().getByLabel('Nhập lại tên tài nguyên');
    const confirm = dialog().getByRole('button', { name: 'Xác nhận xóa vĩnh viễn', exact: true });
    await h.type(nameField, redisDescriptor.toUpperCase(), { delay: 25 });
    await expect(confirm).toBeDisabled();
    await h.type(nameField, redisDescriptor, { delay: 25, replace: true });
    await expect(confirm).toBeEnabled();
    rec.record('delete.exact-name-confirmation', 'CLEANUP', 'ui', true, 'a wrong-case name keeps confirm disabled; the exact technical name enables it');
    await h.click(confirm);
    await expect(dialog()).toHaveCount(0);
    const progressPanel = page.getByTestId('cleanup-progress');
    await expect(progressPanel).toBeVisible();
    const seen = new Set();
    const watcher = setInterval(async () => {
      try { const t = await progressPanel.locator('[data-state="current"]').first().innerText({ timeout: 500 }); seen.add(t.trim()); } catch { /* panel not ready */ }
    }, 250);
    await expect(progressPanel).toContainText('Tất cả tài nguyên đã chọn đã được xác minh là không còn', { timeout: input.cleanupUiTimeoutMs });
    clearInterval(watcher);
    const stageList = await progressPanel.getByLabel('Các giai đoạn').innerText();
    const allStages = ['Đã nhận yêu cầu', 'Đang kiểm tra', 'Đang xóa', 'Đang xác minh đã hết'].every((l) => stageList.includes(l));
    rec.record('delete.operation-stages', 'CLEANUP', 'ui', allStages, `observed current stages: ${[...seen].join(' / ') || 'completed before the first poll'}`);
    await expect(progressPanel.locator('tbody tr')).toHaveCount(2);
    await h.moveTo(progressPanel);
    await h.pause(SHORT_READ);
    await pageIsSafe('cleanup progress');
    const absent = pickByImage(await tech.statefulsets(), 'redis').length === 0 && pickByImage(await tech.statefulsets(), 'postgres').length === 0
      && (await tech.json(['-n', input.namespace, 'get', 'pvc', '-o', 'json'])).items.length === 0;
    rec.record('delete.selected-absent', 'CLEANUP', 'private-read', absent, 'Redis and PostgreSQL StatefulSets and PVCs are verified absent in the cluster');
    const finalStatuses = await statuses();
    const nsStatus = finalStatuses.find((x) => x.descriptor === namespaceDescriptor)?.status;
    rec.record('delete.unchecked-retained-present', 'CLEANUP', 'private-read', (await tech.namespaceExists()) && nsStatus !== 'DESTROYED', `namespace still present, product status ${nsStatus}`);
    const nodes = await tech.json(['get', 'nodes', '-o', 'json']);
    const platformOk = nodes.items.length >= 1 && ((await s.get('/connections')).connections.find((c) => c.key === input.connection.name)?.status === 'READY');
    rec.record('delete.platform-present', 'CLEANUP', 'private-read', platformOk, 'the cluster nodes and the Connection are untouched');
    await s.signOut();

    // Remaining run-owned objects are removed by the independent safety net, NOT by a UI step.
    caption('Phần còn lại (namespace của lần chạy) được lưới an toàn độc lập dọn sau bước giao diện; đây không phải thao tác giao diện và không được tính là thành công của bước xóa');
    v2ctl(input, 'supervisor-finish', '-private', input.privateDir);
    let waited = '';
    try {
      waited = v2ctl(input, 'supervisor-wait', '-private', input.privateDir, '-timeout', input.cleanupWaitDuration);
    } catch (error) {
      rec.record('cleanup.owned-after-ui-by-safety-net', 'CLEANUP', 'private-read', false, safeError(error));
    }
    rec.record('cleanup.owned-after-ui-by-safety-net', 'CLEANUP', 'private-read', /"state": "CLEAN"/.test(waited), 'supervisor reached CLEAN (delivery -> app resources -> namespace, each verified absent)');
    let absentOut = '';
    try { absentOut = v2ctl(input, 'k8s-absent', '-config', input.configFile, '-manifest', input.manifestFile); } catch (error) { absentOut = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.namespace-absent', 'CLEANUP', 'private-read', !absentOut.includes('FAIL') && !(await tech.namespaceExists()), 'run namespace and run-owned PVs verified absent');
    let sentinel = '';
    try { sentinel = v2ctl(input, 'k8s-sentinel', '-config', input.configFile); } catch (error) { sentinel = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.sentinel-preserved', 'CLEANUP', 'private-read', sentinel.includes('preserved'), 'the existing outside-run sentinel keeps its UID');
    await page.waitForTimeout(7000);
    writeFileSync(`${ctx.evidenceDir}/outcome.json`, JSON.stringify({ jobId, cacheKey }, null, 2));
  },

  async E01(ctx) {
    const { page, h, rec, input } = ctx;
    const s = steps(ctx);
    const tech = technical(input);
    const phase = ctx.phase;
    const scope = `/applications/${input.applicationKey}/environments/staging`;
    phase('PREPARE', 'Chuẩn bị: đăng nhập và đăng ký Kết nối k8s-4f bằng giao diện; chưa có hạ tầng nào được tạo');
    await h.showCursor();
    await expectVisibleCursor(page);
    await s.signIn('platform-engineer');
    rec.record('ui.signin', 'PREPARE', 'ui', true, 'platform-engineer');
    await s.registerKubernetesConnection();
    phase('FORM', 'Biểu mẫu: đăng ký danh mục hợp lệ, rồi thử đăng ký Redis thiếu đầu vào bắt buộc của module (namespace) và một đầu vào sai kiểu (annotations); Console chặn ngay tại trường');
    await s.registerTypes();
    const { defs } = await s.registerDefinitions({ redisOverrides: [] });
    await s.registerRenderTemplate();
    rec.record('ui.definitions.registered', 'FORM', 'ui', ['namespace-k8s-module', 'pg-in-cluster', 'redis-in-cluster'].every((k) => defs.includes(k)), defs.join(','));

    await h.click(s.nav('Cấu hình tài nguyên'));
    await s.startDefinition('redis-bad-input', 'redis', 'redis-k8s', 'Cluster nội bộ');
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await s.choose(s.form('Cluster đích của provider kubernetes'), 'Cluster của Môi trường');
    // The module variable "namespace" is required and has no default: it is deliberately left unmapped.
    await h.click(s.submitDefinition());
    const missing = page.getByRole('alert').first();
    await expect(missing).toContainText('Cần nhập trước khi đăng ký');
    const missingText = await missing.innerText();
    await h.moveTo(missing);
    await h.pause(SHORT_READ);
    const namesRequired = /namespace/.test(missingText);
    // A wrong-typed value for a map variable is refused as well and the field is named.
    await h.click(page.getByRole('button', { name: 'Dùng preset cho namespace', exact: true }));
    await s.override('annotations', JSON.stringify('not-a-map'));
    await h.click(s.submitDefinition());
    const wrong = page.getByRole('alert').first();
    await expect(wrong).toBeVisible();
    const wrongText = await wrong.innerText();
    await h.moveTo(wrong);
    await h.pause(SHORT_READ);
    const namesWrong = /annotations/.test(wrongText);
    const stored = (await s.get('/resource-definitions')).resourceDefinitions.some((d) => d.key === 'redis-bad-input');
    rec.record('e01.valid-schema-invalid-param', 'FORM', 'ui', true, 'both variables exist in the module schema; one is missing, one has the wrong type');
    rec.record('e01.field-error-in-ui', 'FORM', 'ui', namesRequired && namesWrong && !stored, `missing -> ${missingText.slice(0, 80)} | wrong type -> ${wrongText.slice(0, 80)} | stored=${stored}`);
    await s.signOut();

    phase('PREVIEW', 'Xem trước: một Workload dùng Redis hợp lệ; Xem trước chỉ đọc và không tạo gì, nên không có state apply hay provider create nào');
    await s.signIn('developer');
    recordNamespace(input, await s.bindEnvironment());
    await s.putVariables([['PORT', '8080']]);
    await s.addWorkload({
      name: 'api', image: input.images.backend, variables: ['PORT'],
      resources: [{ alias: 'cache', type: 'redis', sharedId: ACCEPTANCE.cacheId }],
      outputs: [['REDIS_HOST', 'cache', 'host']], port: { name: 'http', port: '8080' },
    });
    const existedBefore = await tech.namespaceExists();
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await h.moveTo(page.getByLabel('Xem trước triển khai'));
    await h.pause(SHORT_READ);
    const deployments = (await s.get(`${scope}/deployments`)).deployments ?? [];
    rec.record('e01.no-state-apply', 'PREVIEW', 'private-read', deployments.length === 0, 'no Deployment, hence no Terraform attempt or state apply, exists');
    rec.record('e01.no-provider-create', 'PREVIEW', 'private-read', !existedBefore && !(await tech.namespaceExists()), 'the run namespace was never created');
    const resources = (await s.get(`${scope}/resources`)).resources ?? [];
    const labelled = (await tech.kubectl(['get', 'all,pvc,secret,configmap', '-A', '-l', `run-id=${input.runId}`, '-o', 'name'])).stdout.trim();
    rec.record('e01.no-owned-resources', 'PREVIEW', 'private-read', resources.length === 0 && labelled === '', 'no resource record and no run-labelled object exists');
    phase('CLEANUP', 'Dọn: không có gì được tạo; lưới an toàn độc lập xác minh namespace không tồn tại và sentinel còn nguyên');
    await s.signOut();
    v2ctl(input, 'supervisor-finish', '-private', input.privateDir);
    const waited = v2ctl(input, 'supervisor-wait', '-private', input.privateDir, '-timeout', input.cleanupWaitDuration);
    rec.record('cleanup.owned-after-ui-by-safety-net', 'CLEANUP', 'private-read', /"state": "CLEAN"/.test(waited), 'supervisor CLEAN with nothing to remove');
    await page.waitForTimeout(5000);
  },

  async E02(ctx) {
    const { page, h, rec, input } = ctx;
    const s = steps(ctx);
    const tech = technical(input);
    const phase = ctx.phase;
    const scope = `/applications/${input.applicationKey}/environments/staging`;
    const badImage = `${input.e02.redisRepository}@${nonexistentDigest(input.runId)}`;
    phase('PREPARE', 'Chuẩn bị: kiểm tra digest giả định không tồn tại trong registry rồi đăng nhập, đăng ký Kết nối k8s-4f');
    const status = await registryDigestStatus(badImage);
    rec.record('e02.image-syntax-valid', 'PREPARE', 'private-read', /^[a-z0-9][a-z0-9._/-]*(:[A-Za-z0-9._-]{1,128})?@sha256:[0-9a-f]{64}$/.test(badImage), 'image reference passes the module syntax rule');
    rec.record('e02.digest-nonexistent-in-registry', 'PREPARE', 'private-read', status === 'missing', `registry answered ${status} for the manifest digest`);
    await h.showCursor();
    await expectVisibleCursor(page);
    await s.signIn('platform-engineer');
    rec.record('ui.signin', 'PREPARE', 'ui', true, 'platform-engineer');
    await s.registerKubernetesConnection();
    phase('FORM', 'Biểu mẫu: Definition Redis ghim image có digest không tồn tại (cú pháp hợp lệ); mọi thứ khác là module thật đã ghim');
    await s.registerTypes();
    const { defs } = await s.registerDefinitions({ redisOverrides: [['image', badImage]] });
    await s.registerRenderTemplate();
    rec.record('ui.definitions.registered', 'FORM', 'ui', ['namespace-k8s-module', 'pg-in-cluster', 'redis-in-cluster'].every((k) => defs.includes(k)), defs.join(','));
    await s.signOut();
    await s.signIn('developer');
    recordNamespace(input, await s.bindEnvironment());
    await s.putVariables([['PORT', '8080']]);
    await s.addWorkload({
      name: 'cache-probe', image: input.images.frontend, variables: ['PORT'],
      resources: [{ alias: 'cache', type: 'redis', sharedId: ACCEPTANCE.cacheId }],
      outputs: [['REDIS_HOST', 'cache', 'host']], port: { name: 'http', port: '8080' },
    });
    phase('PREVIEW', 'Xem trước: cú pháp hợp lệ nên Xem trước thành công; lỗi chỉ xuất hiện khi Pod thật không kéo được image');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await h.pause(SHORT_READ);
    phase('TERRAFORM', 'Triển khai: namespace được tạo, StatefulSet Redis không Ready vì image không tồn tại; node Terraform báo FAILED sau thời hạn có giới hạn');
    await ctx.deploy();
    const namespaceUp = await waitFor(() => tech.namespaceExists(), { timeoutMs: 600_000, intervalMs: 3000, what: 'the namespace to be created' });
    rec.record('e02.namespace-created', 'TERRAFORM', 'private-read', namespaceUp, 'the run namespace exists while Redis is applied');
    const notReady = await waitFor(async () => {
      const set = pickByImage(await tech.statefulsets(), 'redis')[0];
      if (!set) return false;
      const pods = (await tech.pods()).items.filter((p) => p.metadata.name.startsWith(set.metadata.name));
      const reasons = pods.flatMap((p) => (p.status?.containerStatuses ?? []).map((c) => c.state?.waiting?.reason)).filter(Boolean);
      return (set.status?.readyReplicas ?? 0) === 0 && reasons.some((r) => /ErrImagePull|ImagePullBackOff/.test(r)) ? reasons.join(',') : false;
    }, { timeoutMs: 600_000, intervalMs: 3000, what: 'the Redis pod to fail its image pull' });
    rec.record('e02.statefulset-not-ready', 'TERRAFORM', 'private-read', Boolean(notReady), `Redis StatefulSet not Ready (${notReady})`);
    await recordOwnedAssets(input, tech);
    await expect(page.getByRole('alert').first()).toBeVisible({ timeout: input.e02.failureTimeoutMs });
    await h.moveTo(page.getByRole('alert').first());
    await h.pause(SHORT_READ);
    const failedRows = await waitFor(async () => {
      const deployments = (await s.get(`${scope}/deployments`)).deployments ?? [];
      const rows = [];
      for (const d of deployments) rows.push(...((await s.get(`${scope}/deployments/${d.id}`)).resourceProgress ?? []));
      return rows.some((r) => r.status === 'FAILED') ? rows : false;
    }, { timeoutMs: input.e02.failureTimeoutMs, intervalMs: 5000, what: 'the Terraform node to be FAILED' });
    rec.record('e02.terraform-node-failed', 'TERRAFORM', 'ui', failedRows.some((r) => r.status === 'FAILED'), `failed rows: ${failedRows.filter((r) => r.status === 'FAILED').map((r) => `${r.stage}/${r.safeErrorCode ?? ''}`).join(',')}`);
    const resources = (await s.get(`${scope}/resources`)).resources ?? [];
    const redis = resources.find((r) => r.type === 'redis');
    const detail = redis ? (await s.get(`${scope}/resources/${redis.id}`)).resource : undefined;
    rec.record('e02.state-and-provenance-preserved', 'TERRAFORM', 'ui', Boolean(detail?.lastAttempt) && detail.status !== 'DESTROYED' && Boolean(detail.module?.revision), `resource ${detail?.status}, last attempt ${detail?.lastAttempt?.stage}/${detail?.lastAttempt?.status}, module ${detail?.module?.id}@${detail?.module?.revision}`);
    phase('CLEANUP', 'Dọn: lưới an toàn độc lập chỉ xóa đối tượng do lần chạy tạo ra, xác minh vắng mặt, và sentinel có sẵn ngoài lần chạy phải còn nguyên');
    await s.signOut();
    v2ctl(input, 'supervisor-finish', '-private', input.privateDir);
    const waited = v2ctl(input, 'supervisor-wait', '-private', input.privateDir, '-timeout', input.cleanupWaitDuration);
    rec.record('e02.cleanup-created-by-run-only', 'CLEANUP', 'private-read', /"state": "CLEAN"/.test(waited), 'supervisor CLEAN: only run-owned objects were removed');
    let absentOut = '';
    try { absentOut = v2ctl(input, 'k8s-absent', '-config', input.configFile, '-manifest', input.manifestFile); } catch (error) { absentOut = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.namespace-absent', 'CLEANUP', 'private-read', !absentOut.includes('FAIL') && !(await tech.namespaceExists()), 'run namespace and run-owned PVs verified absent');
    let sentinel = '';
    try { sentinel = v2ctl(input, 'k8s-sentinel', '-config', input.configFile); } catch (error) { sentinel = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.sentinel-preserved', 'CLEANUP', 'private-read', sentinel.includes('preserved'), 'the existing outside-run sentinel keeps its UID');
    await page.waitForTimeout(5000);
  },

  async A02(ctx) { return awsScenario(ctx, 'A02', { v2ctl, recordNamespace, forbidden: FORBIDDEN_IN_PAGE }); },
  async A03(ctx) { return awsScenario(ctx, 'A03', { v2ctl, recordNamespace, forbidden: FORBIDDEN_IN_PAGE }); },
};

export function parseArgs(argv) {
  const args = { scenario: '', captions: '', evidence: '', headed: false, devHeadless: false };
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--scenario') args.scenario = argv[++i] ?? '';
    else if (argv[i] === '--captions') args.captions = argv[++i] ?? '';
    else if (argv[i] === '--evidence') args.evidence = argv[++i] ?? '';
    else if (argv[i] === '--headed') args.headed = true;
    else if (argv[i] === '--dev-headless') args.devHeadless = true;
    else if (argv[i] === '--check') args.check = argv[++i] ?? '';
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
  const scenario = requireScenario(args.scenario);
  if (!args.headed && !(args.devHeadless && process.env.ORCH_E2E_DEV_HEADLESS_OK === '1')) throw new Error('--headed is required: recordings use a headed browser');
  requireCaptionLocale(args.captions);
  const env = process.env;
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_INPUT']) if (!env[name]) throw new Error(`${name} is required`);
  if (args.headed) for (const name of ['ORCH_E2E_XDOTOOL', 'DISPLAY']) if (!env[name]) throw new Error(`${name} is required`);
  const baseURL = env.ORCH_E2E_URL;
  const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
  const input = JSON.parse(readFileSync(env.ORCH_E2E_INPUT, 'utf8'));
  const { width, height } = screenSize(env);
  const videoPath = `${evidenceDir}/${args.scenario}-raw.mp4`;
  const assertionsPath = `${evidenceDir}/assertions.txt`;

  const browser = args.headed ? await launchHeaded({ width, height }) : await (await import('@playwright/test')).chromium.launch(env.ORCH_E2E_BROWSER_EXECUTABLE ? { executablePath: env.ORCH_E2E_BROWSER_EXECUTABLE } : {});
  const context = await browser.newContext(args.headed ? { viewport: null } : {});
  context.setDefaultTimeout(30_000);
  await context.addInitScript(installCursor);
  const marks = [];
  const forwarders = [];
  // Every Preview names the Terraform run ID its Deploy will use. It is recorded in the private manifest BEFORE the Deploy click:
  // that record is the only authority the independent safety net has over Terraform instances.
  const previewRuns = [];
  context.on('response', (response) => {
    if (response.request().method() !== 'POST' || !new URL(response.url()).pathname.endsWith('/preview')) return;
    previewRuns.push((async () => {
      const body = await response.json();
      if (!body?.runId) throw new Error('the Preview carries no run id');
      v2ctl(input, 'manifest-run', '-manifest', input.manifestFile, '-id', body.runId);
    })());
  });
  let recorder;
  let startedAt;
  let currentPage;
  let result = 'FAIL';
  const secondsNow = () => (startedAt ? Number(((Date.now() - startedAt) / 1000).toFixed(1)) : 0);
  const captions = createCaptions(secondsNow);
  const rec = createRecorder({ dir: evidenceDir, now: Date.now, startedAt: () => startedAt ?? Date.now() });
  const origRecord = rec.record;
  rec.record = (id, phaseName, source, pass, detail) => {
    appendFileSync(assertionsPath, `${pass ? 'PASS' : 'FAIL'} ${id} [${source}] ${String(detail).slice(0, 200)}\n`);
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
    // The runner arms the independent cleanup supervisor (it watches this process) and
    // only then releases the gate, so the first create can never precede the supervisor.
    if (env.ORCH_E2E_START_GATE) await waitFor(async () => existsSync(env.ORCH_E2E_START_GATE), { timeoutMs: 180_000, intervalMs: 200, what: 'the cleanup supervisor to be armed' });
    const page = await context.newPage();
    currentPage = page;
    const x11 = args.headed ? x11Input(env.ORCH_E2E_XDOTOOL) : undefined;
    if (x11) page.on('dialog', () => {}); // keep native confirmations open until an X11 key answers them
    const selectNative = x11 ? nativeSelect(x11) : undefined;
    const h = createHuman(page, selectNative ? { selectNative } : {});
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
    if (x11) {
      x11.focusBrowser();
      recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
      startedAt = recorder.spawnedAtMs;
    } else {
      startedAt = Date.now();
    }
    await page.waitForTimeout(1500);
    const stopPhase = args.devHeadless ? env.ORCH_E2E_DEV_STOP_PHASE : undefined; // development harness only: never set by the runner
    const phase = (name, text) => {
      if (stopPhase && name === stopPhase) throw new Error(`development stop at ${name}`);
      marks.push({ label: name.toLowerCase(), seconds: secondsNow() }); captions.caption(text); rec.phase(name, text);
    };
    const deploy = async () => {
      await Promise.all(previewRuns); // the run id of the Preview is in the manifest before anything is created
      if (previewRuns.length === 0) throw new Error('Deploy without a recorded Preview run id');
      await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
    };
    await scenario({ page, h, context, x11, selectNative, caption: captions.caption, phase, rec, baseURL, input, forwarders, evidenceDir, deploy });
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
    const text = safeError(error);
    appendFileSync(assertionsPath, `FAIL ${text}\n`);
    console.error(String(error?.stack ?? error).split('\n').slice(0, 6).join('\n'));
    // The Console and the acceptance app carry no secret text (scanned like every public file); the image documents where it failed.
    try { if (currentPage) await currentPage.screenshot({ path: `${evidenceDir}/failure.png`, timeout: 10_000 }); } catch { /* the page may be closed */ }
    process.exitCode = 1;
  } finally {
    await finalize();
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => { console.error(safeError(error)); process.exit(2); });
}
