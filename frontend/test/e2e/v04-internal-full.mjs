import v04RequiredAssertions from '../../../backend/test/integration/v2run/testdata/v04-required-assertions.json' with { type: 'json' };
// V04 headed recording (issue #4): the ENTIRE acceptance application (frontend, backend, worker) with a REAL
// shared PostgreSQL and a REAL shared Redis, placed on an existing Kubernetes cluster from Resource Definitions
// and a resolved dependency graph only (ADR-016/ADR-017). Invoked by backend/test/integration/v04-internal-full.sh,
// which starts the REAL product, the independent cleanup supervisor and the headed browser.
//
// UI actions (source "ui"): Internal account upload, Secret Store, Types, Definitions, Environment Settings keys
// and the secret, Score import, Score Preview and pending Preview, Deploy, the real acceptance page (job form, check
// rows), update, remove and the resource delete dialogs. Labelled otherwise: "private-read" (kubectl/psql/redis-cli in the
// run's own namespace through the private handle, passwords on stdin only), "api" (read-only matcher probes) and
// explicit technical cleanup mutations are labelled in narration. No fake, stub or interception exists anywhere.
//   node v04-internal-full.mjs --check V04
//   node v04-internal-full.mjs --scenario V04 --headed --captions vi --evidence DIR
import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { safeError, waitFor } from './terraform-v2-lib.mjs';
import { SHORT_READ, exactTextPattern, kubeFlags, makeKit, recordAppSecret, v2ctl } from './full-app-kit.mjs';
import {
  CHECK_ROWS, FULL_APP_TYPES, cacheKeyOf, configureEnvironment, onboardInternalAccount, openFrontend, reconnectFrontend, readCheckRows, registerExistingCluster,
  registerK8sModuleDefinition, registerTypesUi, registerWorkloadTemplate, submitJob, waitRowsPass,
} from './full-app-flows.mjs';
import { runMain } from './scenario-main.mjs';
import { cleanupRemainingDelivery } from './v04-probe-cleanup.mjs';

export const catalogIds = (input) => ({
  cluster: `cluster-${input.runId}`, namespace: `namespace-${input.runId}`, postgres: `pg-${input.runId}`, redis: `redis-${input.runId}`, template: `render-${input.runId}`,
});

/** Assertion IDs a complete V04 run must have recorded as PASS (the full coverage map of the V04 gate). */
export const REQUIRED_ASSERTIONS = Object.freeze(v04RequiredAssertions);
export const REQUIRED_PHASES = ['PREPARE', 'FORM', 'PREVIEW', 'TERRAFORM', 'READY', 'JOB', 'CACHE', 'CLEANUP'];

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
  async V04(ctx) {
    const { page, h, x11, caption, phase, mark, rec, input, evidenceDir } = ctx;
    const kit = makeKit(ctx);
    const { nav, get, tech, signIn, signOut, openApplication, backToApplication, openSettings, editVariable, leaveSettings, appPath, noLeak, assertNoViolations, platform } = kit;
    const ids = catalogIds(input);
    const app = input.applicationKey;
    const clusterDescriptor = `k8s-cluster.internal#environments.${app}.staging`;
    const nsDescriptor = `k8s-namespace.default#environments.${app}.staging`;
    const auxDescriptor = `${nsDescriptor}-aux`;
    const pgDescriptor = 'postgres.in-cluster#shared.acceptance-db';
    const redisDescriptor = 'redis.in-cluster#shared.acceptance-cache';
    const kubeArgs = [...kubeFlags(input), '-namespace', input.namespace];
    const imageArgs = (frontend) => ['-frontend', frontend.ref, '-backend', input.images.backend.ref, '-worker', input.images.worker.ref, '-frontend-digest', frontend.digest];
    const appSecret = randomBytes(24).toString('hex');
    const appSecretSha = createHash('sha256').update(appSecret).digest('hex');
    recordAppSecret(input, 'ACCEPTANCE_SECRET', appSecret);
    const scoresRecord = JSON.parse(readFileSync(`${input.scoreDir}/scores.json`, 'utf8'));
    const config = scoresRecord.config.filter((c) => c.kind === 'variable').map((c) => (c.name === 'ACCEPTANCE_SECRET_SHA256' ? { ...c, value: appSecretSha } : c));
    let lastVerdict = '';
    const verifyApp = (frontend, extra = []) => {
      try {
        lastVerdict = v2ctl(input, 'v04-app', ...kubeArgs, ...imageArgs(frontend), ...extra).trim().slice(0, 400);
        return { ok: true, out: lastVerdict };
      } catch (error) {
        lastVerdict = `${String(error?.stdout ?? '').trim().split('\n').pop() ?? ''} | ${String(error?.stderr ?? '').trim().split('\n')[0] ?? ''}`.slice(0, 500);
        return { ok: false, out: lastVerdict };
      }
    };
    const waitApp = (frontend, what, extra = []) => waitFor(async () => { const v = verifyApp(frontend, extra); return v.ok ? v : false; }, { timeoutMs: input.timeouts.readyMs, intervalMs: 4000, what, describe: () => lastVerdict });
    const instances = (...extra) => v2ctl(input, 'v04-instances', '-database-url-file', `${input.privateDir}/console.url`, ...extra).trim();
    const appliedTargets = () => JSON.parse(v2ctl(input, 'v03-target', '-database-url-file', `${input.privateDir}/console.url`));
    const captureOutputRefs = () => v2ctl(input, 'v04-output-refs', '-database-owner-url-file', `${input.privateDir}/owner.url`, '-run-id', input.runId, '-out', `${input.privateDir}/output-vault-refs.json`);
    const sameTarget = (a, b) => Boolean(a && b) && ['connection', 'cluster', 'namespace', 'account', 'accountVersion', 'clusterDefinition', 'clusterNode'].every((f) => a[f] === b[f]);
    const jobRow = async (backend, id) => (await tech.psql(backend, `SELECT status, result, processed_by, coalesce(cache_write,'') FROM jobs WHERE id = ${id}`)).split('|');
    const checksJson = async (front) => (await fetch(`${front.forwarding.url}/api/checks`)).json();
    const edgesOf = (plan, consumer) => (plan.graph?.edges ?? []).filter((e) => e.consumer === consumer);
    const hasEdge = (plan, consumer, provider, reason) => (plan.graph?.edges ?? []).some((e) => e.consumer === consumer && e.provider === provider && e.reason === reason);
    const batchIndex = (plan, d) => (plan.batches ?? []).findIndex((b) => b.includes(d));

    // ===================== PREPARE: Platform Engineer =====================
    phase('PREPARE', 'Kỹ sư nền tảng đăng ký tài khoản Internal bằng kubeconfig tải lên và kho bí mật Vault; xác nhận account Ready và credential không hiển thị');
    await h.showCursor();
    const { expectVisibleCursor } = await import('./human.mjs');
    await expectVisibleCursor(page);
    await signIn('platform-engineer');
    const initialAccounts = await get('/driver-accounts');
    expect(Array.isArray(initialAccounts.accounts), 'driver-account catalog response must contain an accounts array').toBe(true);
    expect(initialAccounts.accounts).toHaveLength(0);
    const accountKey = await onboardInternalAccount(kit);
    const storeToken = readFileSync(input.secretStore.tokenFile, 'utf8').trim();
    const storeKey = await platform.registerSecretStore(input.secretStore, storeToken); // records ui.secret-store.ready

    // ===================== FORM: Types and Definitions =====================
    phase('FORM', 'Đăng ký Loại tài nguyên, cluster có sẵn, namespace, PostgreSQL, Redis và mẫu workload. Quan hệ phụ thuộc đến từ tham chiếu và provider binding.');
    const types = await registerTypesUi(kit, FULL_APP_TYPES);
    rec.record('ui.types.registered', 'FORM', 'ui', types.length === FULL_APP_TYPES.length && types.find((t) => t.key === 'postgres')?.outputs.find((o) => o.name === 'password')?.secret === true && types.find((t) => t.key === 'redis')?.outputs.find((o) => o.name === 'password')?.secret === true,
      `${types.length} types; password outputs of postgres and redis are secret`);
    const cluster = await registerExistingCluster(kit, { id: ids.cluster, accountName: input.account.name, appKey: app });
    const ns = await registerK8sModuleDefinition(kit, { id: ids.namespace, name: 'Namespace Terraform trên cluster internal', type: 'k8s-namespace', module: 'k8s-namespace', presetName: 'name', clusterClass: 'internal' });
    const nsBinding = ns?.driverInputs?.values?.provider_bindings?.kubernetes;
    rec.record('ui.definition.namespace', 'FORM', 'ui', nsBinding?.target === 'resource' && nsBinding.resource_ref === 'k8s-cluster.internal#environments.@app.@env' && !ns.driverAccount, `namespace module binds its Kubernetes provider to ${nsBinding?.resource_ref}; no account of its own`);
    const pg = await registerK8sModuleDefinition(kit, { id: ids.postgres, name: 'PostgreSQL trong cluster (Terraform module)', type: 'postgres', module: 'postgres-k8s', input: true, presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' });
    const redis = await registerK8sModuleDefinition(kit, { id: ids.redis, name: 'Redis trong cluster (Terraform module)', type: 'redis', module: 'redis-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' });
    const bindingOk = (def) => def?.driverInputs?.values?.provider_bindings?.kubernetes?.resource_ref === 'k8s-cluster.internal#environments.@app.@env' && /outputs\.name\}$/.test(def.driverInputs.values.variables?.namespace ?? '') && def.criteria?.some((c) => c.class === 'in-cluster');
    rec.record('ui.definition.postgres', 'FORM', 'ui', bindingOk(pg), 'PostgreSQL module: Kubernetes provider bound to the cluster resource, namespace from the namespace output reference, class in-cluster');
    rec.record('ui.definition.redis', 'FORM', 'ui', bindingOk(redis), 'Redis module: Kubernetes provider bound to the cluster resource, namespace from the namespace output reference, class in-cluster');
    const template = await registerWorkloadTemplate(kit, { id: ids.template, bundle: input.bundle, companion: { descriptor: 'k8s-namespace.default#environments.@app.@env-aux', params: { name: input.companionNamespace } } });
    const templateVars = template?.driverInputs?.values?.variables ?? {};
    rec.record('ui.definition.workload', 'FORM', 'ui', templateVars.render_bundle === input.bundle && /outputs\.name\}$/.test(templateVars.namespace ?? '') && /outputs\.cluster\}$/.test(templateVars.cluster ?? '') && Boolean(template.provision?.['k8s-namespace.default#environments.@app.@env-aux']),
      `workload Definition ${ids.template}: literal bundle, whole namespace/cluster output references and a companion rule with no flags`);
    mark('definitions');
    await signOut();

    // ===================== PREVIEW: Developer =====================
    phase('PREVIEW', 'Nhà phát triển cấu hình khóa môi trường và bí mật, nhập ba Score của ứng dụng thật, xem trước đồ thị phụ thuộc và thứ tự provider-first; chưa có gì được tạo');
    await signIn('developer');
    await openApplication();
    expect(((await get(`${appPath}/deployments`)).deployments ?? []).length).toBe(0);
    expect(await tech.namespaceUid(input.namespace), 'the run namespace must not exist before Deploy').toBe('');
    await configureEnvironment(kit, { config, secretName: 'ACCEPTANCE_SECRET', secretValue: appSecret, storeKey });
    rec.record('ui.config.secret-hidden', 'PREVIEW', 'ui', !(await page.locator('body').innerText()).includes(appSecret) && (await get(`${appPath}/configuration`)).keys?.some((k) => k.name === 'ACCEPTANCE_SECRET'),
      'the application secret was stored through the Environment secret store; its value is never visible; only its SHA-256 is a public key');
    for (const name of ['backend', 'worker', 'frontend']) await kit.addWorkloadFromScore(`${input.scoreDir}/${name}.yaml`, name);
    const drafts = (await get(`${appPath}/workloads`)).workloads ?? [];
    rec.record('ui.score.imported', 'PREVIEW', 'ui', ['backend', 'worker', 'frontend'].every((n) => drafts.some((w) => w.id === n)) && drafts.find((w) => w.id === 'frontend')?.score?.containers?.main?.image === input.images.frontendA.ref,
      `three Score drafts saved; Score sha256 backend ${scoresRecord.workloads.find((w) => w.name === 'backend').sha256.slice(0, 16)} worker ${scoresRecord.workloads.find((w) => w.name === 'worker').sha256.slice(0, 16)} frontend ${scoresRecord.workloads.find((w) => w.name === 'frontend').sha256.slice(0, 16)}; ${scoresRecord.changes.length} recorded differences from the repository Score source`);
    await backToApplication();
    const planned = await kit.scorePreview(`${input.scoreDir}/backend.yaml`, 'backend', 'plan');
    const plan = planned.body;
    await h.moveTo(page.getByLabel('Kết quả xem trước Score'));
    await h.pause(SHORT_READ);
    const backendNode = 'workload.default#modules.backend';
    const nodes = (plan.graph?.nodes ?? []).map((n) => n.descriptor);
    const expectedNodes = [clusterDescriptor, nsDescriptor, auxDescriptor, pgDescriptor, redisDescriptor, backendNode];
    rec.record('graph.fixed-point', 'PREVIEW', 'ui', planned.status === 200 && expectedNodes.every((d) => nodes.includes(d)) && nodes.length === expectedNodes.length, `graph nodes ${nodes.length}: cluster, namespace, companion, PostgreSQL, Redis, backend workload (companion and shared resources discovered by expansion)`);
    const order = [clusterDescriptor, nsDescriptor, pgDescriptor].map((d) => batchIndex(plan, d));
    rec.record('graph.provider-first', 'PREVIEW', 'ui', order[0] >= 0 && order[0] < order[1] && order[1] < order[2] && batchIndex(plan, redisDescriptor) > order[1] && batchIndex(plan, auxDescriptor) > order[0],
      `batches ${(plan.batches ?? []).map((b, i) => `#${i}:${b.length}`).join(' ')}; cluster ${order[0]} < namespace ${order[1]} < PostgreSQL ${order[2]}; Redis ${batchIndex(plan, redisDescriptor)} after the namespace`);
    const edgesOk = hasEdge(plan, nsDescriptor, clusterDescriptor, 'provider-binding') && hasEdge(plan, pgDescriptor, clusterDescriptor, 'provider-binding') && hasEdge(plan, redisDescriptor, clusterDescriptor, 'provider-binding')
      && hasEdge(plan, pgDescriptor, nsDescriptor, 'resource-reference') && hasEdge(plan, redisDescriptor, nsDescriptor, 'resource-reference')
      && hasEdge(plan, backendNode, pgDescriptor, 'workload-placeholder') && hasEdge(plan, backendNode, redisDescriptor, 'workload-placeholder')
      && hasEdge(plan, backendNode, nsDescriptor, 'resource-reference') && hasEdge(plan, backendNode, clusterDescriptor, 'resource-reference');
    rec.record('graph.edge-reasons', 'PREVIEW', 'ui', edgesOk && (await page.locator('li[data-reason="provider-binding"]').count()) >= 4, 'edges carry their reasons in the UI: provider-binding (module -> cluster), resource-reference (namespace/cluster output), workload-placeholder (shared database/cache outputs)');
    rec.record('graph.shared-one-node', 'PREVIEW', 'ui', nodes.filter((d) => d === pgDescriptor).length === 1 && nodes.filter((d) => d === redisDescriptor).length === 1, 'one scoped PostgreSQL node and one Redis node (shared by backend and worker through the shared resource ID)');
    rec.record('graph.companion-no-ordering-edge', 'PREVIEW', 'ui', edgesOf(plan, auxDescriptor).every((e) => e.provider === clusterDescriptor && e.reason === 'provider-binding') && !(plan.graph.edges ?? []).some((e) => e.provider === auxDescriptor),
      'the empty-flag companion namespace has only its provider-binding edge to the cluster: no ordering edge to or from the workload');
    await backToApplication();
    const { preview, panel } = await kit.pendingPreview(3);
    await h.moveTo(panel);
    await h.pause(SHORT_READ);
    const intents = new Map();
    for (const change of preview.changes) for (const intent of change.resourceIntents ?? []) intents.set(intent.descriptor, intent);
    const intentOk = intents.get(clusterDescriptor)?.ownership === 'PREEXISTING' && [nsDescriptor, auxDescriptor, pgDescriptor, redisDescriptor].every((d) => intents.get(d)?.intent === 'CREATE' && intents.get(d)?.ownership === 'ORCHESTRATOR') && intents.size === 5;
    rec.record('preview.intents', 'PREVIEW', 'ui', intentOk && !preview.binding?.connectionKey, `${intents.size} resource intents: the cluster is PREEXISTING; namespace, companion, PostgreSQL, Redis are created and owned by the orchestrator; no Environment connection`);
    const panelText = await panel.innerText();
    rec.record('preview.pure', 'PREVIEW', 'private-read', (await tech.namespaceUid(input.namespace)) === '' && ((await get(`${appPath}/deployments`)).deployments ?? []).length === 0 && !panelText.includes(appSecret) && !/password/i.test(panelText.replace(/bí mật/g, '')),
      'after the pending Preview: no namespace, no Deployment, no secret text on screen');
    await noLeak('preview');
    assertNoViolations();
    mark('preview');

    // ===================== TERRAFORM =====================
    phase('TERRAFORM', 'Triển khai: cluster có sẵn được kiểm tra, Terraform tạo namespace, PostgreSQL và Redis theo thứ tự đồ thị rồi score-k8s phát hành ba workload; tiến trình lấy từ dữ liệu đã lưu');
    const result = await kit.deployAndWait(input.timeouts.deployMs);
    captureOutputRefs();
    for (const name of ['backend', 'worker', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [^:]+: thành công`) })).toHaveCount(1);
    await h.moveTo(result);
    await h.pause(SHORT_READ);
    const deployments = (await get(`${appPath}/deployments`)).deployments ?? [];
    const details = [];
    for (const d of deployments) details.push(await get(`${appPath}/deployments/${d.id}`));
    const progressRows = details.flatMap((d) => d.resourceProgress ?? []);
    await kit.openLatestDeploymentDetail();
    rec.record('deploy.progress-persisted', 'TERRAFORM', 'ui', progressRows.filter((r) => r.stage === 'READY' && r.status === 'SUCCEEDED').length >= 4, `${progressRows.length} persisted attempt rows; READY/SUCCEEDED ${progressRows.filter((r) => r.stage === 'READY').length} (namespace, companion, PostgreSQL, Redis)`);
    rec.record('deploy.no-blocked', 'TERRAFORM', 'ui', details.every((d) => Array.isArray(d.blocked) && d.blocked.length === 0), 'no consumer was held back by a failed provider');
    mark('deployed');

    // ===================== READY =====================
    phase('READY', 'Sẵn sàng: cả ba workload Ready trên đúng cluster; chỉ một PostgreSQL và một Redis dùng chung cho backend và worker; mật khẩu chỉ qua Secret, không nằm trong giá trị của Pod');
    const ready = await waitApp(input.images.frontendA, 'the whole application to be ready with one PostgreSQL and one Redis', ['-identity-out', `${input.privateDir}/identity.json`]);
    rec.record('ready.app', 'READY', 'private-read', Boolean(ready), ready.out);
    rec.record('ready.single-postgres-redis', 'READY', 'private-read', /one PostgreSQL \(1\) and one Redis \(1\) shared by backend and worker/.test(ready.out), 'exactly one PostgreSQL and one Redis StatefulSet; backend and worker bind the same host');
    rec.record('ready.secret-references', 'READY', 'private-read', /passwords by Secret reference/.test(ready.out), 'PGPASSWORD, REDIS_PASSWORD and ACCEPTANCE_SECRET reach the Pods only through Secret references');
    instances('-out', `${input.privateDir}/instances.json`);
    rec.record('ready.instances-singleton', 'READY', 'private-read', true, instances('-compare', `${input.privateDir}/instances.json`));
    const deploymentsNow = await tech.deployments();
    const byName = Object.fromEntries(deploymentsNow.items.map((d) => [d.metadata.name, d]));
    recordAppSecret(input, 'PGPASSWORD', await tech.resolvedEnv(byName.backend, 'PGPASSWORD'));
    recordAppSecret(input, 'REDIS_PASSWORD', await tech.resolvedEnv(byName.backend, 'REDIS_PASSWORD'));
    v2ctl(input, 'v03-record-owned', ...kubeFlags(input), '-baseline', input.baselineFile, '-run-id', input.runId, '-database-url-file', `${input.privateDir}/console.url`, '-namespaces', `${input.namespace},${input.companionNamespace}`, '-out', input.ownedFile);
    rec.record('ready.owned-namespaces', 'READY', 'private-read', JSON.parse(readFileSync(input.ownedFile, 'utf8')).length === 2, `${input.namespace} and ${input.companionNamespace} carry this run's exact tags, match product receipts and have recorded UIDs`);
    const secretHash = createHash('sha256').update(await tech.resolvedEnv(byName.backend, 'ACCEPTANCE_SECRET')).digest('hex');
    rec.record('config.secret-hash-matches', 'READY', 'private-read', secretHash === appSecretSha, 'the SHA-256 of the secret the backend Pod really receives equals the public configured SHA-256 (value never printed)');
    const targetFirst = appliedTargets().find((t) => t.workloadId === 'backend');
    mark('ready');

    // ===================== JOB =====================
    phase('JOB', 'Công việc: mở frontend thật, năm hàng kiểm tra PASS (kể cả bí mật, cơ sở dữ liệu và cache), gửi job bằng form có sẵn rồi chờ worker xử lý');
    const front = await openFrontend(kit);
    await waitRowsPass(front.app);
    const rows = await readCheckRows(front.app);
    const checks = await checksJson(front);
    const checksText = JSON.stringify(checks);
    rec.record('app.checks-pass', 'JOB', 'ui', CHECK_ROWS.every((n) => rows.some(([name, status]) => name === n && status === 'PASS')) && checks.checks?.secret === true && checks.checks?.environment === true && checks.checks?.database === true, `real page rows: ${rows.map(([n, s]) => `${n}:${s}`).join(', ')} (the proxied /api/checks agrees)`);
    rec.record('app.checks-no-secret', 'JOB', 'ui', ![appSecret, await tech.resolvedEnv(byName.backend, 'PGPASSWORD'), await tech.resolvedEnv(byName.backend, 'REDIS_PASSWORD')].some((v) => checksText.includes(v)), '/api/checks carries no secret value and no database or cache credential');
    caption('Bảng kiểm tra của ứng dụng thật đã PASS hoàn toàn. Gửi job bằng form có sẵn của chính ứng dụng; worker thật xử lý và ghi PostgreSQL rồi Redis');
    await front.ah.pause(SHORT_READ);
    const payload = `v04 acceptance ${input.runId}`;
    const job = await submitJob(front.ah, front.app, payload);
    const expected = `processed:${payload.toUpperCase()}`;
    rec.record('job.completed-ui', 'JOB', 'ui', job.cells[2] === 'DONE' && job.cells[3] === expected && job.cells[4] !== '', `job ${job.id} DONE with result processed:UPPER(payload); processed by a worker`);
    const row = await jobRow(byName.backend, job.id);
    rec.record('job.postgres-row', 'JOB', 'private-read', row[0] === 'DONE' && row[1] === expected && row[3] === 'OK', `PostgreSQL row ${job.id}: ${row[0]}, result matches, cache_write ${row[3]}`);
    const podNames = (await tech.pods()).items.map((p) => p.metadata.name);
    rec.record('job.worker-identity', 'JOB', 'private-read', podNames.includes(row[2]) && row[2].startsWith('worker'), 'processed_by is the name of a real worker Pod of this namespace');
    rec.record('job.no-duplicate', 'JOB', 'private-read', (await tech.psql(byName.backend, `SELECT count(*) FROM jobs WHERE payload = '${payload}'`)) === '1', 'exactly one row exists for the submitted payload');

    phase('CACHE', 'Bộ nhớ đệm: worker đã ghi Redis trước khi backend đọc; backend trả cache hit; chờ khóa hết hạn thật (không xóa), tải lại để thấy PostgreSQL fallback rồi nạp lại Redis');
    const cacheKey = cacheKeyOf(input, job.id);
    const refilledBefore = (checks.cache?.lookups ?? {})['miss-refilled'] ?? 0;
    const exists = await tech.redis(byName.backend, 'EXISTS', cacheKey);
    const ttl = Number(await tech.redis(byName.backend, 'TTL', cacheKey));
    const lookupsNow = (await checksJson(front)).cache?.lookups ?? {};
    rec.record('cache.worker-write', 'CACHE', 'private-read', exists === '1' && ttl > 0 && ttl <= input.cache.ttlSeconds && row[3] === 'OK' && (lookupsNow['miss-refilled'] ?? 0) === refilledBefore,
      `Redis key exists with TTL ${ttl}s (<= ${input.cache.ttlSeconds}s), cache_write OK and no refill happened yet, so the WORKER wrote it`);
    const hit = await (await fetch(`${front.forwarding.url}/api/jobs/${job.id}`)).json();
    rec.record('cache.backend-hit', 'CACHE', 'private-read', hit.cache === 'hit' && hit.status === 'DONE', `GET job ${job.id} through the frontend proxy: ${hit.cache}`);
    await waitFor(async () => (await tech.redis(byName.backend, 'EXISTS', cacheKey)) === '0', { timeoutMs: (input.cache.ttlSeconds + 30) * 1000, intervalMs: 1000, what: 'the cache TTL to expire' });
    rec.record('cache.ttl-expiry-miss', 'CACHE', 'private-read', true, `the Redis key expired by its own TTL within ${input.cache.ttlSeconds + 30}s (it was never deleted)`);
    const before = (await checksJson(front)).cache?.lookups ?? {};
    await front.app.reload();
    const reloaded = front.app.locator('#jobs tr.job').filter({ hasText: payload }).first();
    const sameCells = (await reloaded.locator('td').allInnerTexts()).map((t) => t.trim());
    await front.ah.moveTo(reloaded);
    await front.ah.pause(SHORT_READ);
    const after = (await checksJson(front)).cache?.lookups ?? {};
    const keyBack = (await tech.redis(byName.backend, 'EXISTS', cacheKey)) === '1';
    const ttlBack = Number(await tech.redis(byName.backend, 'TTL', cacheKey));
    rec.record('cache.refill', 'CACHE', 'private-read', (after['miss-refilled'] ?? 0) > (before['miss-refilled'] ?? 0) && keyBack && ttlBack > 0, `miss-refilled ${before['miss-refilled'] ?? 0} -> ${after['miss-refilled'] ?? 0}; the key is back in Redis with TTL ${ttlBack}s`);
    rec.record('job.reload-same-result', 'JOB', 'ui', sameCells.join('|') === job.cells.join('|') && sameCells[3] === expected, 'after the reload the same job and result are shown (PostgreSQL fallback)');
    mark('job-cache');

    // ===================== UPDATE =====================
    phase('READY', 'Cập nhật: đổi frontend sang ảnh B và triển khai lại; dữ liệu job, PostgreSQL, Redis và danh tính hạ tầng phải giữ nguyên, không tạo bản sao');
    await signOut();
    await signIn('developer');
    await openApplication();
    await kit.editWorkloadFromScore(`${input.scoreDir}/frontend-b.yaml`, 'frontend');
    const updatePreview = await kit.pendingPreview(1);
    await expect(updatePreview.panel).toContainText('cập nhật');
    await kit.deployAndWait(input.timeouts.deployMs);
    captureOutputRefs();
    await waitApp(input.images.frontendB, 'the frontend rollout to image B with the same shared services', ['-identity-in', `${input.privateDir}/identity.json`]);
    await reconnectFrontend(front, 'the frontend Service to answer after the rollout to image B');
    const survivedRow = front.app.locator('#jobs tr.job').filter({ hasText: payload }).first();
    const survivedCells = (await survivedRow.locator('td').allInnerTexts()).map((t) => t.trim());
    const rowAfter = await jobRow(byName.backend, job.id);
    instances('-compare', `${input.privateDir}/instances.json`);
    rec.record('update.frontend-rollout', 'READY', 'private-read', true, `the frontend Deployment runs image B (digest verified) and the real page answers; backend, worker, PostgreSQL and Redis untouched`);
    rec.record('update.data-and-identity-preserved', 'READY', 'ui', survivedCells.join('|') === job.cells.join('|') && rowAfter[0] === 'DONE' && rowAfter[1] === expected, 'the job row and its result are identical after the redeploy; namespace, PostgreSQL, Redis, claims and volumes keep their UIDs and the product still records exactly one instance of each');

    caption('Cập nhật cấu hình hợp lệ: đổi TTL cache sang 35 giây, triển khai backend và worker; ứng dụng vẫn hoạt động và dữ liệu được giữ.');
    await backToApplication();
    await openSettings();
    await editVariable('ACCEPTANCE_CACHE_TTL_SECONDS', '35');
    await leaveSettings();
    await kit.editWorkloadFromScore(`${input.scoreDir}/backend.yaml`, 'backend');
    await kit.editWorkloadFromScore(`${input.scoreDir}/worker.yaml`, 'worker');
    await kit.pendingPreview(2);
    await kit.deployAndWait(input.timeouts.deployMs);
    captureOutputRefs();
    await waitApp(input.images.frontendB, 'valid cache TTL update to backend and worker without replacing shared services', ['-identity-in', `${input.privateDir}/identity.json`]);
    await waitRowsPass(front.app);
    const validConfigDeployments = (await tech.deployments()).items.filter((d) => ['backend', 'worker'].includes(d.metadata.name));
    rec.record('update.config-applied', 'READY', 'private-read', validConfigDeployments.length === 2 && validConfigDeployments.every((d) => tech.envOf(d, 'ACCEPTANCE_CACHE_TTL_SECONDS')?.value === '35'),
      'valid TTL35 applied to both actual workloads; normal checks and original shared-service identities pass');
    mark('updated');

    // ===================== RESTART =====================
    phase('READY', 'Khởi động lại orchestrator: kế hoạch, ghim đích, trạng thái và danh tính tài nguyên nằm trong PostgreSQL; sau đó vẫn thao tác được, dữ liệu job còn nguyên và không tạo bản sao');
    await signOut();
    const { execFileSync } = await import('node:child_process');
    execFileSync(process.env.ORCH_E2E_RESTART_CMD, { stdio: 'ignore' });
    await page.reload();
    await expect(page.getByLabel('Tên đăng nhập', { exact: true }).or(page.getByRole('button', { name: 'Đăng xuất', exact: true }))).toBeVisible({ timeout: 120_000 });
    if (await page.getByRole('button', { name: 'Đăng xuất', exact: true }).isVisible()) await signOut();
    await signIn('platform-engineer');
    const afterRestart = (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === ids.cluster);
    rec.record('restart.catalog-persisted', 'READY', 'ui', afterRestart?.version === cluster.version && afterRestart?.driverAccount === cluster.driverAccount, `cluster Definition v${afterRestart?.version} and account ${afterRestart?.driverAccount} survived the restart`);
    await h.click(nav('Cấu hình tài nguyên'));
    await h.click(page.getByRole('button', { name: `Sửa cấu hình ${ids.cluster}`, exact: true }));
    const section = page.getByRole('region', { name: 'Sửa cấu hình tài nguyên', exact: true });
    await h.type(section.getByLabel('Tên hiển thị', { exact: true }), 'Cluster k8s-4f (có sẵn, đã đổi tên)', { replace: true });
    await h.click(section.getByRole('button', { name: 'Lưu thay đổi', exact: true }));
    await expect(section.getByRole('status')).toContainText('phiên bản 2');
    await signOut();
    await signIn('developer');
    await openApplication();
    await kit.editWorkloadFromScore(`${input.scoreDir}/frontend.yaml`, 'frontend');
    await kit.pendingPreview(1);
    await kit.deployAndWait(input.timeouts.deployMs);
    captureOutputRefs();
    await waitApp(input.images.frontendA, 'the post-restart operation to roll the frontend back to image A', ['-identity-in', `${input.privateDir}/identity.json`]);
    await reconnectFrontend(front, 'the frontend Service to answer after the post-restart operation');
    rec.record('restart.operation-after', 'READY', 'private-read', true, 'an operation after the restart and a catalog rename deployed image A again');
    instances('-compare', `${input.privateDir}/instances.json`);
    const targetAfter = appliedTargets().find((t) => t.workloadId === 'backend');
    rec.record('restart.identity-reused', 'READY', 'private-read', sameTarget(targetFirst, targetAfter), `the backend target pin (account ${targetAfter?.account}, cluster ${targetAfter?.cluster}, namespace ${targetAfter?.namespace}) and every shared-service identity are unchanged after update, restart and the operation after it; no second PostgreSQL or Redis exists`);
    const persisted = front.app.locator('#jobs tr.job').filter({ hasText: payload }).first();
    rec.record('restart.job-persisted', 'READY', 'ui', (await persisted.locator('td').allInnerTexts()).map((t) => t.trim()).join('|') === job.cells.join('|'), `job ${job.id} is still DONE with the same result after the orchestrator restart and redeploys`);
    const second = await submitJob(front.ah, front.app, `${payload} again`);
    const secondRow = await jobRow(byName.backend, second.id);
    rec.record('restart.new-job', 'READY', 'ui', second.cells[2] === 'DONE' && secondRow[3] === 'OK' && second.id !== job.id, `a new job ${second.id} is processed by the worker after the restart and cache_write ${secondRow[3]}`);

    mark('restarted');

    // ===================== CLEANUP =====================
    phase('CLEANUP', 'UI gỡ frontend và backend cùng thay đổi, giữ database/cache cho worker. Cleanup kỹ thuật gỡ worker và VSO; UI hoàn tất xóa tài nguyên.');
    // The normal restart/job flow already holds the Developer session on this Application.
    await backToApplication();
    const removeWorkload = async (name) => {
      const workloadRow = page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) });
      const opened = new Promise((resolve) => page.once('dialog', resolve));
      const clicking = h.click(workloadRow.getByRole('button', { name: 'Xóa', exact: true }));
      const dialog = await opened;
      if (dialog.message() !== `Đánh dấu xóa ${name} trong staging? Workload đang chạy chỉ thay đổi sau khi Triển khai.`) { x11.keys(['Escape']); await clicking; throw new Error(`the ${name} confirmation did not match the expected Application/Environment`); }
      await h.pause(SHORT_READ);
      x11.keys(['Return']);
      await clicking;
      await expect(workloadRow).toContainText('Chờ xóa');
    };
    // Frontend references the backend Service: remove both in one valid desired set.
    await removeWorkload('frontend');
    await removeWorkload('backend');
    await kit.pendingPreview(2);
    await kit.deployAndWait(input.timeouts.deployMs);
    const stillShared = await waitFor(async () => {
      const list = ((await get(`${appPath}/resources`)).resources ?? []);
      const p = list.find((r) => r.descriptor === pgDescriptor);
      const r = list.find((x) => x.descriptor === redisDescriptor);
      return p?.status === 'READY' && r?.status === 'READY' ? list : false;
    }, { timeoutMs: 60_000, intervalMs: 2000, what: 'PostgreSQL and Redis to stay READY while the worker still uses them' });
    writeFileSync(`${evidenceDir}/cleanup-resource-metadata.json`, JSON.stringify(stillShared.map(({ id, descriptor, status, generation }) => ({ id, descriptor, status, generation }))), { mode: 0o600 });
    rec.record('cleanup.shared-kept-while-consumer', 'CLEANUP', 'ui', Boolean(stillShared) && (await tech.deployments()).items.some((d) => d.metadata.name === 'worker') && (await tech.statefulsets()).items.length === 2, 'after frontend and backend were removed together PostgreSQL and Redis stay READY (not UNREFERENCED, not deletable) because the worker still consumes them');
    caption('Cleanup kỹ thuật: product RemoveDelivered dùng đích đã lưu để gỡ worker và VSO; PostgreSQL, Redis cùng namespace vẫn được giữ.');
    if (!/^pending-[0-9a-f]{16}$/.test(preview?.runId)) throw new Error('initial Preview lacks a trusted run identity for delivery cleanup');
    writeFileSync(`${input.privateDir}/probe-cleanup-manifest.json`, JSON.stringify({ runId: input.runId, target: 'k8s-4f', scenario: 'V04', generation: 0, namespace: input.namespace, namespaceIdentity: input.namespace, terraformRuns: [preview.runId] }), { mode: 0o600 });
    const deliveryReport = cleanupRemainingDelivery(input, ['worker']);
    const vsoObjects = await tech.json(['-n', input.namespace, 'get', 'vaultstaticsecrets.secrets.hashicorp.com,vaultauths.secrets.hashicorp.com,vaultconnections.secrets.hashicorp.com', '-o', 'json']);
    rec.record('cleanup.delivery-and-vso-absent', 'CLEANUP', 'private-read', deliveryReport.removed.length === 1 && (await tech.deployments()).items.length === 0 && vsoObjects.items.length === 0 && (await tech.statefulsets()).items.length === 2,
      'technical product RemoveDelivered (not UI) removed exactly worker and VSO objects through original targets; cluster API verifies delivery/VSO absent and both shared StatefulSets retained; whole-run cleanup still pending');
    await removeWorkload('worker');
    await kit.pendingPreview(1);
    await kit.deployAndWait(input.timeouts.deployMs);
    await front.forwarding.stop().catch(() => {});
    await front.app.close().catch(() => {});
    rec.record('cleanup.workloads-removed', 'CLEANUP', 'private-read', await waitFor(async () => (await tech.deployments()).items.length === 0, { timeoutMs: 180_000, intervalMs: 3000, what: 'every workload Deployment to be removed' }), `no workload Deployment remains in ${input.namespace}; UI committed desired removals after labelled technical delivery/VSO cleanup; shared services still run`);
    const unref = await waitFor(async () => {
      const list = ((await get(`${appPath}/resources`)).resources ?? []).map((r) => ({ descriptor: r.descriptor, status: r.status }));
      return [pgDescriptor, redisDescriptor, nsDescriptor, auxDescriptor].every((d) => list.find((r) => r.descriptor === d)?.status === 'UNREFERENCED') ? list : false;
    }, { timeoutMs: 90_000, intervalMs: 2000, what: 'PostgreSQL, Redis and both namespaces to become UNREFERENCED' });
    rec.record('cleanup.unreferenced', 'CLEANUP', 'ui', Boolean(unref), 'PostgreSQL, Redis and both namespaces are retained UNREFERENCED; removing workloads destroyed none of them');
    await signOut();
    await signIn('platform-engineer');
    await openApplication();
    const resources = page.getByRole('region', { name: 'Tài nguyên của môi trường' });
    const entry = (descriptor) => resources.locator('button.deployment-entry').filter({ has: page.locator('strong', { hasText: new RegExp(exactTextPattern(descriptor)) }) });
    let clusterOffered = false;
    const deleteResource = async (descriptor) => {
      await h.click(entry(descriptor));
      await expect(page.getByTestId('resource-name')).toHaveText(descriptor);
      await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
      const dialog = page.getByRole('dialog');
      await expect(dialog).toBeVisible();
      const clusterBoxes = dialog.locator('li', { hasText: 'k8s-cluster' }).locator('input[type="checkbox"]');
      if ((await clusterBoxes.evaluateAll((boxes) => boxes.filter((box) => !box.disabled).length)) > 0) clusterOffered = true;
      if ((await dialog.getByRole('region', { name: 'Các tài nguyên sẽ bị xóa' }).innerText()).includes('k8s-cluster')) clusterOffered = true;
      const name = (await dialog.getByTestId('root-name').innerText()).trim();
      expect(name).toBe(descriptor);
      await h.type(dialog.getByLabel('Nhập lại tên tài nguyên'), name, { delay: 25, replace: true });
      await h.click(dialog.getByRole('button', { name: 'Xác nhận xóa vĩnh viễn', exact: true }));
      await expect(dialog).toHaveCount(0);
      await expect(page.getByTestId('cleanup-progress')).toContainText('Tất cả tài nguyên đã chọn đã được xác minh là không còn', { timeout: input.timeouts.cleanupMs });
      await h.pause(SHORT_READ);
      await backToApplication();
    };
    // The live Redis consumes PostgreSQL; namespaces are removed last via UI.
    for (const descriptor of [redisDescriptor, pgDescriptor, nsDescriptor, auxDescriptor]) await deleteResource(descriptor);
    rec.record('cleanup.platform-excluded', 'CLEANUP', 'ui', !clusterOffered, 'in every delete dialog the existing cluster was never selectable and never in the delete set');
    const gone = (await get(`${appPath}/resources`)).resources ?? [];
    rec.record('cleanup.resources-absent', 'CLEANUP', 'private-read', [pgDescriptor, redisDescriptor, nsDescriptor, auxDescriptor].every((d) => gone.find((r) => r.descriptor === d)?.status === 'DESTROYED') && (await tech.statefulsets()).items.length === 0,
      'all four run-owned resources are DESTROYED in the product and no StatefulSet remains (verified through the cluster API)');
    let absentOut = '';
    try { absentOut = v2ctl(input, 'v03-absent', ...kubeFlags(input), '-baseline', input.baselineFile, '-owned', input.ownedFile); } catch (error) { absentOut = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.namespaces-absent', 'CLEANUP', 'private-read', !absentOut.includes('FAIL') && (await tech.namespaceUid(input.namespace)) === '' && (await tech.namespaceUid(input.companionNamespace)) === '', absentOut.trim() || 'run namespaces absent; baseline namespaces unchanged');
    let volumes = '';
    try { volumes = v2ctl(input, 'v04-volumes-gone', ...kubeFlags(input), '-identity-in', `${input.privateDir}/identity.json`); } catch (error) { volumes = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.volumes-absent', 'CLEANUP', 'private-read', !volumes.includes('FAIL') && volumes.includes('verified absent'), volumes.trim());
    captureOutputRefs();
    let outputSecrets = '';
    try { outputSecrets = v2ctl(input, 'v04-output-clean', '-vault-addr', input.vaultAddress, '-mount', input.vaultMount, '-token-file', `${input.privateDir}/output-cleanup-token`, '-refs-file', `${input.privateDir}/output-vault-refs.json`, '-baseline-file', `${input.privateDir}/output-vault-baseline.json`, '-run-id', input.runId); }
    catch (error) { outputSecrets = `FAIL ${safeError(error)}`; }
    rec.record('cleanup.output-secrets-absent', 'CLEANUP', 'private-read', !outputSecrets.includes('FAIL'), outputSecrets.trim() || 'exact run-owned durable output references deleted and verified absent');
    const accountAfter = ((await get('/driver-accounts')).accounts ?? []).find((a) => a.key === accountKey);
    const clusterNodes = await tech.json(['get', 'nodes', '-o', 'json']);
    rec.record('cleanup.cluster-preserved', 'CLEANUP', 'private-read', clusterNodes.items.length >= 1 && accountAfter?.status === 'READY', 'the existing cluster is reachable and its Internal account is still READY; neither was created or deleted by the run');
    await noLeak('final page');
    mark('cleaned');
    await signOut();
    mark('signed-out');
    await h.pause(2500);
    writeFileSync(`${evidenceDir}/outcome.json`, JSON.stringify({ jobs: [job.id, second.id], cacheKey }, null, 2));
    appendFileSync(`${evidenceDir}/assertions.txt`, '');
  },
};

if (import.meta.url === `file://${process.argv[1]}`) {
  runMain({ SCENARIOS, verifyEvidence }).catch((error) => { console.error(safeError(error)); process.exit(2); });
}
