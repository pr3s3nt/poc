// Definition-driven AWS acceptance scenarios (issue #4 V05/V06). Invoked from terraform-v2-acceptance.mjs, which supplies the
// harness (gated start, run-ID recording before Deploy, evidence files). Every business step is a Web Console or acceptance-app UI
// action; read-only technical assertions use the pinned aws CLI (operator host chain, never the product) and kubectl over a private
// EKS kubeconfig. Setup mutations are the supervised ECR repositories and short-lived LABELLED technical client Pods (A02/A03 only).
//
//   V05  EKS created by the authored graph in THE SAME execution; PostgreSQL and Redis run in the cluster.
//   A02  Aurora + ElastiCache (managed), workload on the EKS the graph creates.
//   A03  Aurora + in-cluster Redis on the EKS the graph creates.
//
// There is no Environment profile, Connection or binding: the AWS account (V01), every AWS Definition's own Driver Account and
// literal region, the cluster RESOURCE references and the typed `cluster` output decide every target. The optional ACCEPTANCE_SECRET
// binding is a LABELLED exception here (the run-owned Vault is bridged to a local cluster, not to EKS) and never excuses the
// PostgreSQL/Redis credentials, which arrive through Kubernetes Secret references.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { kubectlRunner, safeError, waitFor } from './terraform-v2-lib.mjs';
import { SHORT_READ, makeKit, recordAppSecret, technical, v2ctl as ctl } from './full-app-kit.mjs';
import {
  cacheKeyOf, onboardAwsAccount, openFrontend, readAppJSON, readCheckRows, registerCatalogDefinition, registerTypesUi, registerWorkloadTemplate, submitJob, waitRowsPass,
} from './full-app-flows.mjs';
import { assetsFromArns, previewPurityProof, verifyCleanupAWSAbsence } from './terraform-v2-aws.mjs';
import { expectVisibleCursor } from './human.mjs';
import { startKubectlProxy } from './kubernetes-proxy.mjs';
export { validateKubectlTarget, stopOwnedProcess, startKubectlProxy } from './kubernetes-proxy.mjs';

export const CLUSTER_REF = "${resources['k8s-cluster.eks#environments.@app.@env'].outputs.cluster}";
export const MANAGED_ROWS = ['backend connection', 'environment', 'database', 'cache'];

/** Open and verify the real technical connections before starting timed cache observations. */
export async function warmTechnicalClients({ psql, redis }) {
  await psql('SELECT 1');
  const pong = await redis('PING');
  if (pong !== 'PONG') throw new Error('the Redis technical client did not become ready');
}

/** Delete one already-verified Pod through Kubernetes DeleteOptions with an atomic UID precondition. */
export async function deletePodWithUIDPrecondition({ request, namespace, name, expectedUID, requiredLabels, waitForAbsent }) {
  if (!namespace || !name || typeof expectedUID !== 'string' || !expectedUID || !requiredLabels || Object.keys(requiredLabels).length === 0) throw new Error('technical client Pod ownership record is incomplete');
  const path = `/api/v1/namespaces/${encodeURIComponent(namespace)}/pods/${encodeURIComponent(name)}`;
  const current = await request('GET', path);
  if (current.status === 404) return;
  if (current.status !== 200 || current.body?.metadata?.namespace !== namespace || current.body?.metadata?.name !== name || current.body?.metadata?.uid !== expectedUID
      || Object.entries(requiredLabels).some(([key, value]) => current.body?.metadata?.labels?.[key] !== value)) {
    throw new Error('technical client Pod identity changed; refusing deletion');
  }
  const deleted = await request('DELETE', path, { apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: expectedUID } });
  if (deleted.status === 404) return;
  if (deleted.status === 409) throw new Error('technical client Pod UID changed before deletion');
  if (deleted.status !== 200 && deleted.status !== 202) throw new Error('technical client Pod deletion API failed');
  if (!await waitForAbsent(async () => (await request('GET', path)).status === 404)) {
    throw new Error('technical client Pod absence could not be verified; cleanup needs attention');
  }
}

export async function removeTechnicalPods({ target, pods, namespace, requiredLabels, waitForAbsent, startProxy = startKubectlProxy }) {
  if (pods.length === 0) return [];
  const proxy = await startProxy(target);
  const removed = [];
  try {
    for (const { name, uid } of pods) {
      await deletePodWithUIDPrecondition({ request: proxy.request, namespace, name, expectedUID: uid, requiredLabels, waitForAbsent });
      removed.push(name);
    }
    return removed;
  } finally {
    await proxy.stop();
  }
}

/** Assertion IDs of a complete scenario (kept equal to backend/test/integration/v2run/evidence.go by a Go test). */
export const COMMON_ASSERTIONS = [
  'aws.preflight-account-region-principal', 'aws.egress-32', 'aws.pod-capacity',
  'ui.signin', 'ui.account.ready', 'ui.types.registered', 'ui.definitions.registered', 'ui.workloads.three-saved',
  'preview.pure', 'preview.accounts-regions', 'preview.unknown-cluster-outputs', 'preview.graph', 'preview.pins', 'preview.score-k8s-bundle', 'preview.no-environment-target',
  'deploy.terraform-stages', 'deploy.cluster-ready-before-consumers',
  'ready.ownership-recorded', 'ready.aws-assets-tagged', 'ready.workloads', 'ready.nodes', 'ready.shared-endpoints', 'ready.bindings', 'ready.target-identity', 'persist.no-credential',
  'app.rows-pass', 'app.secret-row-not-configured',
  'job.completed-ui', 'job.postgres-row', 'job.no-duplicate', 'cache.worker-write', 'cache.backend-hit', 'cache.ttl-expiry-miss', 'cache.refill', 'job.reload-same-result',
  'update.data-and-worker-preserved', 'update.config-rollout', 'update.no-duplicate-infra', 'restart.operation-after', 'restart.target-unchanged', 'restart.job-persisted', 'restart.new-job',
  'cleanup.supervisor-clean', 'cleanup.aws-absent', 'aws.preexisting-preserved', 'evidence.no-secret-public',
];
export const requiredAssertions = (scenario) => (scenario === 'V05' ? COMMON_ASSERTIONS : [...COMMON_ASSERTIONS, 'technical.pods-owned-and-removed']);

function awsCli(input) {
  const env = { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', AWS_REGION: input.aws.region, AWS_DEFAULT_REGION: input.aws.region, AWS_PAGER: '', ...input.aws.hostEnv };
  return (...args) => JSON.parse(execFileSync(input.aws.bin, ['--region', input.aws.region, '--output', 'json', ...args], { encoding: 'utf8', env, timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'] }) || '{}');
}

/** Entries of the shared Definition catalog for one scenario, in registration order. */
export const catalogEntries = (catalog, scenario) => [...catalog.definitions.common, ...catalog.definitions[scenario]];

/** Exact AWS provider pins authored by each scenario's Terraform modules. */
export function awsPinSetMatches(scenario, intents, accountKey, region) {
  const expected = {
    V05: ['eks', 'vpc'],
    A02: ['aurora', 'eks', 'elasticache', 'vpc'],
    A03: ['aurora', 'eks', 'vpc'],
  }[scenario];
  if (!expected) return false;
  const actual = intents.flatMap((intent) => (intent.providers ?? [])
    .filter((provider) => provider.name === 'aws')
    .map((provider) => ({ module: intent.module?.module, provider })));
  const modules = actual.map((item) => item.module).sort();
  return modules.length === expected.length && modules.join('|') === expected.join('|')
    && actual.every(({ provider }) => provider.target === 'account' && provider.connectionKey === accountKey && provider.region === region);
}

export function assertOwnedTechnicalPod(pod, namespace, name, uid, labels) {
  if (!pod || pod.metadata?.namespace !== namespace || pod.metadata?.name !== name || pod.metadata?.uid !== uid) throw new Error('technical client Pod identity changed; refusing deletion');
  for (const [key, value] of Object.entries(labels)) if (pod.metadata?.labels?.[key] !== value) throw new Error('technical client Pod run labels changed; refusing deletion');
}
export function kubernetesNotFound(result) {
  if (result.code === 0) return false;
  if (/\bnot found\b|\bnotfound\b/i.test(result.stderr)) return true;
  throw new Error('Kubernetes API did not prove resource absence');
}

export async function awsDefinitionScenario(ctx, scenario, { forbidden }) {
  const { page, h, rec, input, phase } = ctx;
  const kit = makeKit(ctx);
  const { get, signIn, signOut, appPath, nav } = kit;
  const aws = awsCli(input);
  const catalog = JSON.parse(readFileSync(input.aws.catalogFile, 'utf8'));
  const classes = catalog.classes[scenario];
  const inClusterData = scenario === 'V05';
  const subs = { EGRESS: input.aws.egressCidr, NODE_TYPE: input.aws.nodeInstanceType, REGION: input.aws.region };
  const pageIsSafe = async (label) => {
    const text = await page.locator('body').innerText();
    for (const pattern of forbidden) expect(text, `${label}: ${pattern}`).not.toMatch(pattern);
    expect(text, `${label}: access key`).not.toContain(input.aws.secretAccessKey);
  };
  const taggedArns = () => (aws('resourcegroupstaggingapi', 'get-resources', '--tag-filters', `Key=run-id,Values=${input.runId}`).ResourceTagMappingList ?? []).map((r) => r.ResourceARN);
  const ownedKey = (a) => `${a.kind}\t${a.id}`;
  const toAssets = (arns) => arns.map((arn) => assetsFromArns([arn])[0] ?? { kind: 'arn', id: arn, layer: 'aux' });
  const recordFailures = [];
  const recordAssets = () => {
    for (const a of toAssets(taggedArns())) {
      try {
        ctl(input, 'manifest-asset', '-manifest', input.manifestFile, '-kind', a.kind, '-id', a.id, '-layer', a.layer, '-owner', 'CREATED_BY_RUN');
      } catch (error) {
        recordFailures.push({ kind: a.kind, message: safeError(error) });
        throw new Error(`the run manifest could not acknowledge an AWS asset (${a.kind}); cleanup requires attention: ${safeError(error)}`);
      }
    }
  };
  const ownershipAcknowledged = () => {
    const owned = new Set(ctl(input, 'manifest-owned', '-manifest', input.manifestFile).split('\n').filter(Boolean));
    const missing = toAssets(taggedArns()).filter((a) => !owned.has(ownedKey(a)));
    return { missing, ok: missing.length === 0 };
  };
  const pods = { created: [], removed: [], leftover: [] };
  const labels = `run-id=${input.runId},managed-by=orchestrator-verification,app.kubernetes.io/component=technical-probe`;

  // ===================== PREPARE / FORM: Platform Engineer =====================
  phase('PREPARE', `Chuẩn bị ${scenario}: Kỹ sư nền tảng đăng ký tài khoản AWS bằng giao diện; danh mục bắt đầu trống, không có Kết nối hay hồ sơ thực thi của môi trường`);
  await h.showCursor();
  if (!ctx.browserOnly) await expectVisibleCursor(page);
  await signIn('platform-engineer');
  rec.record('ui.signin', 'PREPARE', 'ui', true, 'platform-engineer');
  await h.pause(SHORT_READ);
  const accountKey = await onboardAwsAccount(kit, { name: input.aws.connectionName, accessKeyId: input.aws.accessKeyId, secretAccessKey: input.aws.secretAccessKey, region: input.aws.region });

  writeFileSync(`${input.privateDir}/account-key`, accountKey, { mode: 0o600 });
  phase('FORM', 'Biểu mẫu: Loại tài nguyên, rồi từng cấu hình tài nguyên tự chọn tài khoản AWS và vùng của riêng nó; namespace và cơ sở dữ liệu trong cluster trỏ provider tới tài nguyên cluster EKS; mẫu dựng workload tham chiếu namespace và cluster');
  const types = await registerTypesUi(kit, catalog.types);
  const clusterType = types.find((t) => t.key === 'k8s-cluster');
  rec.record('ui.types.registered', 'FORM', 'ui', types.length === catalog.types.length && clusterType?.outputs.some((o) => o.name === 'cluster' && o.required === true), `${types.length} types; the cluster Type publishes the typed target as "cluster"`);
  await h.click(nav('Cấu hình tài nguyên'));
  await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
  const entries = catalogEntries(catalog, scenario);
  const stored = [];
  for (const entry of entries) stored.push(await registerCatalogDefinition(kit, entry, { accountName: input.aws.connectionName, region: input.aws.region, subs }));
  const capabilities = (await get('/resource-definition-capabilities')).capabilities;
  const pinned = entries.every((e) => capabilities.some((c) => c.driverType === 'terraform' && c.module === e.module && c.source?.contentSha256));
  const awsDefs = stored.filter((d, i) => entries[i].provider === 'aws');
  const accountsOk = awsDefs.every((d) => d?.driverAccount === accountKey && d.driverInputs?.values?.variables?.region === input.aws.region);
  rec.record('ui.definitions.registered', 'FORM', 'ui', stored.every(Boolean) && pinned && accountsOk, `definitions ${stored.map((d) => d?.key).join(',')} pinned=${pinned}; every AWS Definition names its own Driver Account and the literal region`);
  await registerWorkloadTemplate(kit, { id: 'render-eks', bundle: 'score-k8s-internal-v1', clusterRef: CLUSTER_REF });
  await signOut();

  // ===================== FORM: Developer =====================
  await signIn('developer');
  const application = (await get(`/applications/${input.applicationKey}`)).application;
  const environment = application?.environments?.find((e) => e.key === 'staging');
  const exact = ctl(input, 'manifest-namespace', '-manifest', input.manifestFile, '-identity', environment.namespaceIdentity, '-generation', String(environment.targetGeneration ?? 0)).trim();
  if (exact !== input.namespace) throw new Error('the Environment namespace differs from the run namespace of the manifest');
  // The Score documents of the REAL application come from the repository Score source (scores.go) through v2ctl, with the three ECR
  // images the runner published and the classes of this scenario; every difference from the source is recorded in scores.json.
  const scoreDir = `${ctx.evidenceDir}/scores`;
  ctl(input, 'v04-scores', '-out', scoreDir, '-frontend', input.images.frontend, '-backend', input.images.backend, '-worker', input.images.worker, '-application', input.applicationKey,
    '-environment', 'staging', '-run-id', input.runId, '-ttl', String(input.cache.ttlSeconds), '-database-class', classes.postgres, '-cache-class', classes.redis, '-no-app-secret');
  const scoresRecord = JSON.parse(readFileSync(`${scoreDir}/scores.json`, 'utf8'));
  await kit.openApplication();
  await kit.openSettings();
  for (const key of scoresRecord.config.filter((c) => c.kind === 'variable')) {
    await h.click(kit.variablesSection().getByRole('button', { name: '+ Thêm biến' }));
    await h.type(page.getByLabel('Tên key', { exact: true }), key.name);
    await h.type(page.getByLabel('Giá trị', { exact: true }), key.value);
    await kit.saveVariable();
  }
  await kit.leaveSettings();
  for (const name of ['backend', 'worker', 'frontend']) await kit.addWorkloadFromScore(`${scoreDir}/${name}.yaml`, name);
  const drafts = (await get(`${appPath}/workloads`)).workloads ?? [];
  rec.record('ui.workloads.three-saved', 'FORM', 'ui', ['backend', 'worker', 'frontend'].every((n) => drafts.some((w) => w.id === n)), `three Workload drafts saved with the ${scenario} classes (database ${classes.postgres}, cache ${classes.redis})`);

  // ===================== PREVIEW (pure) =====================
  phase('PREVIEW', 'Xem trước: tài khoản và vùng của từng nút, chuỗi VPC → EKS → namespace → workload, đầu ra của cluster chưa tồn tại chỉ hiện danh tính có kiểu; chưa có tài nguyên AWS nào được tạo');
  const baseline = taggedArns().sort();
  const prepared = input.aws.ecrPreparation;
  const expectedArns = [...(prepared?.repositories ?? [])].sort();
  const expectedAssets = expectedArns.map((arn) => assetsFromArns([arn])[0]);
  const ownedBeforePreview = new Set(ctl(input, 'manifest-owned', '-manifest', input.manifestFile).split('\n').filter(Boolean));
  // Score Preview is a separate route; inspect it before creating the final Deploy confirmation.
  const planned = await kit.scorePreview(`${scoreDir}/backend.yaml`, 'backend', 'plan');
  const plan = planned.body;
  await kit.backToApplication();
  const { preview, panel } = await kit.pendingPreview(3);
  const panelText = await panel.innerText();
  await h.moveTo(panel);
  await h.pause(SHORT_READ);
  const intents = new Map();
  const placements = new Map();
  for (const change of preview.changes) {
    for (const intent of change.resourceIntents ?? []) intents.set(intent.descriptor, intent);
    for (const placement of change.placements ?? []) placements.set(placement.node, placement);
  }
  const terraformIntents = [...intents.values()].filter((i) => i.driverType === 'terraform');
  const byType = (type) => [...intents.values()].filter((i) => i.resourceType === type);
  const pinOf = (module) => capabilities.find((c) => c.driverType === 'terraform' && c.module === module)?.source;
  const deploymentCount = ((await get(`${appPath}/deployments`)).deployments ?? []).length;
  const resourceCount = ((await get(`${appPath}/resources`)).resources ?? []).length;
  const afterPreview = taggedArns().sort();
  const purity = previewPurityProof({ baseline, expectedArns, expectedAssets, ownedKeys: ownedBeforePreview, afterPreview, deploymentCount, resourceCount, panelText, accessKeyId: input.aws.accessKeyId, secretAccessKey: input.aws.secretAccessKey });
  rec.record('preview.pure', 'PREVIEW', 'private-read', Object.values(purity).every(Boolean),
    `exact ECR preparation=${purity.exactPreparation}; unchanged after preview=${purity.unchanged}; no deployment/resource state=${purity.stateFree}; no AWS key material on screen=${purity.noKeyMaterial}`);
  const awsPins = terraformIntents.flatMap((i) => (i.providers ?? []).filter((p) => p.name === 'aws').map((p) => ({ module: i.module?.module, pin: p })));
  rec.record('preview.accounts-regions', 'PREVIEW', 'ui', awsPinSetMatches(scenario, terraformIntents, accountKey, input.aws.region),
    `per-node AWS pins ${awsPins.map(({ module, pin }) => `${module}:${pin.connectionKey}@${pin.region}`).join(' ')} (each Definition's own account and literal region)`);
  const clusterNode = `k8s-cluster.eks#environments.${input.applicationKey}.staging`;
  const workloadPlacement = placements.get('workload.default#modules.backend');
  const k8sProviders = terraformIntents.flatMap((i) => (i.providers ?? []).filter((p) => p.name === 'kubernetes'));
  rec.record('preview.unknown-cluster-outputs', 'PREVIEW', 'ui', workloadPlacement?.provider === 'eks' && workloadPlacement.region === input.aws.region && workloadPlacement.account === accountKey && workloadPlacement.cluster === clusterNode
    && k8sProviders.length >= 1 && k8sProviders.every((p) => p.target === 'resource' && p.provider === clusterNode && p.connectionKey === accountKey)
    // Explanatory UI prose names unavailable outputs; inspect actual placement data.
    && !/https:\/\/|"(?:endpoint|kubeconfig|token)"\s*:/i.test(JSON.stringify([...placements.values()])),
    `the workload is placed on ${workloadPlacement?.cluster} (provider ${workloadPlacement?.provider}, ${workloadPlacement?.region}); the cluster does not exist yet, so only its typed identity is shown: no endpoint, credential or Ready state`);
  const expectedModules = (scenario === 'V05' ? ['vpc', 'eks', 'k8s-namespace', 'postgres-k8s', 'redis-k8s'] : ['vpc', 'eks', 'k8s-namespace', 'aurora', scenario === 'A02' ? 'elasticache' : 'redis-k8s']).sort();
  rec.record('preview.pins', 'PREVIEW', 'ui', terraformIntents.length === expectedModules.length && terraformIntents.map((i) => i.module?.module).sort().join() === expectedModules.join()
    && terraformIntents.every((i) => { const pin = pinOf(i.module?.module); return pin && i.module.contentSha256 === pin.contentSha256 && i.module.revision === pin.revision; }),
    terraformIntents.map((i) => `${i.module?.module}@${String(i.module?.contentSha256).slice(0, 12)}`).join(' '));
  const dbDescriptor = `postgres.${classes.postgres}#shared.acceptance-db`;
  const cacheDescriptor = `redis.${classes.redis}#shared.acceptance-cache`;
  const hasEdge = (consumer, provider, reason) => (plan.graph?.edges ?? []).some((e) => e.consumer === consumer && e.provider === provider && e.reason === reason);
  const noEdge = (consumer, provider) => !(plan.graph?.edges ?? []).some((e) => e.consumer === consumer && e.provider === provider);
  const vpcNode = `vpc.default#environments.${input.applicationKey}.staging`;
  const nsNode = `k8s-namespace.default#environments.${input.applicationKey}.staging`;
  const batchIndex = (d) => (plan.batches ?? []).findIndex((b) => b.includes(d));
  let graphOk = planned.status === 200 && hasEdge(clusterNode, vpcNode, 'resource-reference') && hasEdge(nsNode, clusterNode, 'provider-binding')
    && hasEdge('workload.default#modules.backend', nsNode, 'resource-reference') && hasEdge('workload.default#modules.backend', clusterNode, 'resource-reference')
    && batchIndex(vpcNode) < batchIndex(clusterNode) && batchIndex(clusterNode) < batchIndex(nsNode);
  if (inClusterData) graphOk = graphOk && hasEdge(dbDescriptor, clusterNode, 'provider-binding') && hasEdge(dbDescriptor, nsNode, 'resource-reference') && hasEdge(cacheDescriptor, clusterNode, 'provider-binding');
  else {
    // Managed data services depend on the network inputs they actually reference, never on the EKS cluster or namespace.
    graphOk = graphOk && hasEdge(dbDescriptor, vpcNode, 'resource-reference') && noEdge(dbDescriptor, clusterNode) && noEdge(dbDescriptor, nsNode);
    graphOk = graphOk && (scenario === 'A02' ? hasEdge(cacheDescriptor, vpcNode, 'resource-reference') && noEdge(cacheDescriptor, clusterNode) : hasEdge(cacheDescriptor, clusterNode, 'provider-binding'));
  }
  rec.record('preview.graph', 'PREVIEW', 'ui', graphOk && byType('vpc').length === 1 && byType('k8s-cluster').length === 1 && byType('postgres').length === 1 && byType('redis').length === 1 && terraformIntents.every((i) => i.intent === 'CREATE' && i.ownership === 'ORCHESTRATOR'),
    `${[...intents.keys()].length} descriptors; edges: EKS->VPC (reference), namespace->EKS (provider-binding), workload->namespace/cluster (reference); ${inClusterData ? 'PostgreSQL and Redis follow the EKS provider binding' : 'Aurora/cache follow the VPC they reference and have NO EKS edge'}`);
  const bundles = preview.changes.map((c) => c.rendering?.bundle);
  rec.record('preview.score-k8s-bundle', 'PREVIEW', 'ui', preview.changes.length === 3 && bundles.every((b) => b?.id === 'score-k8s-internal-v1' && b.version === input.scoreVersion), `bundle ${bundles[0]?.id} ${bundles[0]?.version}`);
  rec.record('preview.no-environment-target', 'PREVIEW', 'ui', !preview.binding?.connectionKey && !preview.binding?.executionProfile && !preview.accountId, 'the Preview pins no Environment connection, execution profile or account: the Definitions decided every target');
  // Keep the verified transient Preview panel and its Deploy confirmation on this page.
  await h.read('Xem trước chỉ đọc: chưa có tài nguyên AWS nào mang thẻ của lần chạy');

  // ===================== TERRAFORM =====================
  phase('TERRAFORM', 'Triển khai: Terraform tạo VPC rồi EKS bằng tài khoản và vùng của từng cấu hình; hệ thống chờ API, xác thực, mạng và node Ready rồi mới tạo namespace và workload; tiến trình lấy từ dữ liệu đã lưu');
  const assetTimer = setInterval(() => { try { recordAssets(); } catch { /* retained in recordFailures and failed at the ownership gate */ } }, 45_000);
  ctx.forwarders.push({ stop: () => clearInterval(assetTimer) });
  const result = await kit.deployAndWait(input.deployTimeoutMs);
  clearInterval(assetTimer);
  recordAssets();
  for (const name of ['backend', 'worker', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [^:]+: thành công`) })).toHaveCount(1);
  await h.moveTo(result);
  await h.pause(SHORT_READ);
  const deployments = (await get(`${appPath}/deployments`)).deployments ?? [];
  const details = [];
  for (const d of deployments) details.push(await get(`${appPath}/deployments/${d.id}`));
  const progressRows = details.flatMap((d) => d.resourceProgress ?? []);
  await kit.openLatestDeploymentDetail();
  rec.record('deploy.terraform-stages', 'TERRAFORM', 'ui', progressRows.filter((r) => r.stage === 'READY' && r.status === 'SUCCEEDED').length >= expectedModules.length, `${progressRows.length} persisted attempt rows`);
  const clusterReady = progressRows.filter((r) => r.descriptor === clusterNode && r.stage === 'READY' && r.status === 'SUCCEEDED').map((r) => Date.parse(r.updatedAt)).sort()[0];
  const nsStart = progressRows.filter((r) => r.descriptor === nsNode).map((r) => Date.parse(r.startedAt)).sort()[0];
  rec.record('deploy.cluster-ready-before-consumers', 'TERRAFORM', 'ui', Number.isFinite(clusterReady) && Number.isFinite(nsStart) && clusterReady <= nsStart && details.every((d) => (d.blocked ?? []).length === 0),
    `the EKS node reached READY at ${new Date(clusterReady).toISOString()} before the namespace started at ${new Date(nsStart).toISOString()}; no consumer was blocked`);
  await pageIsSafe('deployment result');

  // ===================== READY =====================
  phase('READY', 'Sẵn sàng: EKS và ba workload Ready; danh tính đích đã lưu khớp cluster thật; mật khẩu chỉ qua Secret; không có token hay kubeconfig nào được lưu');
  const arns = taggedArns();
  const assets = assetsFromArns(arns);
  const ack = ownershipAcknowledged();
  rec.record('ready.ownership-recorded', 'READY', 'private-read', ack.ok && recordFailures.length === 0, `${arns.length} run-tagged asset(s) acknowledged in the manifest; ${ack.missing.length} missing, ${recordFailures.length} write failure(s)`);
  if (!ack.ok || recordFailures.length > 0) throw new Error('the manifest does not acknowledge every run-tagged asset; cleanup needs attention');
  const cluster = assets.find((a) => a.kind === 'eks-cluster');
  rec.record('ready.aws-assets-tagged', 'READY', 'private-read', Boolean(cluster) && assets.some((a) => a.kind === 'vpc') && (inClusterData || assets.some((a) => a.kind === 'rds-cluster')) && (scenario !== 'A02' || assets.some((a) => a.kind === 'elasticache-group')),
    `run-tagged assets: ${assets.map((a) => a.kind).sort().join(',')}`);
  const kubeconfig = `${input.privateDir}/eks.kubeconfig`;
  execFileSync(input.aws.bin, ['--region', input.aws.region, 'eks', 'update-kubeconfig', '--name', cluster.id, '--kubeconfig', kubeconfig, '--alias', 'eks-run'], { env: { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', ...input.aws.hostEnv }, stdio: 'ignore', timeout: 120_000 });
  const eks = { kubectl: input.kube.kubectl, kubeconfig, context: 'eks-run', env: { ...input.aws.hostEnv, AWS_REGION: input.aws.region } };
  const kubectl = kubectlRunner(eks);
  const ns = input.namespace;
  const json = async (args) => { const r = await kubectl(args); if (r.code !== 0) throw new Error(`kubectl ${args[0]} failed`); return JSON.parse(r.stdout); };
  const tech = technical({ ...input, kube: eks });
  const readyDeployments = await waitFor(async () => {
    const list = await json(['-n', ns, 'get', 'deployments', '-o', 'json']);
    const byName = Object.fromEntries(list.items.map((d) => [d.metadata.name, d]));
    return ['backend', 'worker', 'frontend'].every((n) => byName[n] && (byName[n].status?.readyReplicas ?? 0) >= 1 && byName[n].status.readyReplicas === byName[n].spec.replicas) ? byName : false;
  }, { timeoutMs: input.readyTimeoutMs, intervalMs: 5000, what: 'the three Deployments to be Ready on EKS' });
  rec.record('ready.workloads', 'READY', 'private-read', true, 'backend, worker, frontend Ready on EKS');
  const nodes = await json(['get', 'nodes', '-o', 'json']);
  rec.record('ready.nodes', 'READY', 'private-read', nodes.items.length >= 1 && nodes.items.every((n) => n.status.conditions.some((c) => c.type === 'Ready' && c.status === 'True')) && nodes.items.every((n) => n.status.nodeInfo.architecture === input.aws.nodeArch), `${nodes.items.length} Ready ${input.aws.nodeArch} node(s)`);
  const resolved = (d, name) => tech.resolvedEnv(d, name);
  const [pgHost, wPgHost, redisHost, wRedisHost, redisTls] = await Promise.all([resolved(readyDeployments.backend, 'PGHOST'), resolved(readyDeployments.worker, 'PGHOST'), resolved(readyDeployments.backend, 'REDIS_HOST'), resolved(readyDeployments.worker, 'REDIS_HOST'), resolved(readyDeployments.backend, 'REDIS_TLS')]);
  const secretBound = ['PGPASSWORD', 'REDIS_PASSWORD'].every((n) => ['backend', 'worker'].every((w) => { const e = tech.envOf(readyDeployments[w], n); return e?.value === undefined && e?.valueFrom?.secretKeyRef; }));
  rec.record('ready.shared-endpoints', 'READY', 'private-read', pgHost === wPgHost && redisHost === wRedisHost && (inClusterData || /rds\.amazonaws\.com$/.test(pgHost)) && (scenario !== 'A02' || /cache\.amazonaws\.com$/.test(redisHost)) && redisHost.length > 0,
    `backend and worker share one database host and one cache host (${inClusterData ? 'in-cluster services' : `Aurora${scenario === 'A02' ? ' and ElastiCache' : ' and an in-cluster Redis'}`})`);
  rec.record('ready.bindings', 'READY', 'private-read', secretBound, 'PGPASSWORD and REDIS_PASSWORD reach the Pods only through Secret references (no literal value)');
  const pgPassword = await resolved(readyDeployments.backend, 'PGPASSWORD');
  const redisPassword = await resolved(readyDeployments.backend, 'REDIS_PASSWORD');
  recordAppSecret(input, 'PGPASSWORD', pgPassword);
  recordAppSecret(input, 'REDIS_PASSWORD', redisPassword);
  const identityNow = async () => {
    const targets = JSON.parse(ctl(input, 'v03-target', '-database-url-file', `${input.privateDir}/console.url`));
    return targets.find((t) => t.workloadId === 'backend');
  };
  const eksDescribe = aws('eks', 'describe-cluster', '--name', cluster.id).cluster;
  const kubeSystemUid = (await kubectl(['get', 'namespace', 'kube-system', '-o', 'jsonpath={.metadata.uid}'])).stdout.trim();
  const target = await identityNow();
  rec.record('ready.target-identity', 'READY', 'private-read', target?.cluster === cluster.id && target.account === accountKey && eksDescribe?.status === 'ACTIVE' && eksDescribe?.arn?.includes(`:${input.aws.accountId}:`) && target.namespace === ns && target.clusterNode === clusterNode && kubeSystemUid.length > 0,
    `the persisted workload target (cluster ${target?.cluster}, account ${target?.account}) is the EKS cluster the AWS API reports ACTIVE in the configured account; kube-system UID recorded`);
  const scan = (() => { try { return { ok: true, out: ctl(input, 'v05-persisted-scan', '-database-url-file', `${input.privateDir}/console.url`).trim() }; } catch (error) { return { ok: false, out: String(error?.stdout ?? '').trim().split('\n').slice(-2).join(' ') }; } })();
  rec.record('persist.no-credential', 'READY', 'private-read', scan.ok, scan.out);

  // Data access for the technical assertions: in-cluster services by exec (V05), or a short-lived LABELLED client Pod inside the run
  // namespace (Aurora/ElastiCache are reachable only from the VPC). The password goes through stdin only.
  const labelMap = Object.fromEntries(labels.split(',').map((part) => part.split('=')));
  const helperClients = new Map();
  const ensureHelper = async (image) => {
    if (helperClients.has(image)) return helperClients.get(image);
    const name = `tech-${randomUUID().slice(0, 12)}`;
    ctl(input, 'manifest-asset', '-manifest', input.manifestFile, '-kind', 'pod', '-id', `${ns}/${name}`, '-layer', 'delivery', '-owner', 'CREATED_BY_RUN', '-linkage', 'persistent technical client Pod of this run');
    const created = await kubectl(['-n', ns, 'run', name, `--labels=${labels}`, '--restart=Never', `--image=${image}`, '--command', '--', 'sh', '-c', 'sleep 3600'], { timeoutMs: 120_000 });
    if (created.code !== 0) throw new Error('technical client Pod creation failed');
    pods.created.push(name);
    const waited = await kubectl(['-n', ns, 'wait', '--for=condition=Ready', `pod/${name}`, '--timeout=120s'], { timeoutMs: 130_000 });
    if (waited.code !== 0) throw new Error('technical client Pod did not become Ready');
    const found = await kubectl(['-n', ns, 'get', 'pod', name, '-o', 'json']);
    if (found.code !== 0) throw new Error('technical client Pod identity could not be read');
    const pod = JSON.parse(found.stdout);
    const uid = pod.metadata?.uid;
    if (!uid) throw new Error('technical client Pod has no UID');
    assertOwnedTechnicalPod(pod, ns, name, uid, labelMap);
    const client = { name, uid };
    helperClients.set(image, client);
    return client;
  };
  const helperResult = async (image, script, args, password, { timeoutMs = 120_000 } = {}) => {
    const client = await ensureHelper(image);
    return kubectl(['-n', ns, 'exec', '-i', client.name, '--', 'sh', '-c', script, 'x', ...args], { timeoutMs, input: `${password}\n` });
  };
  const helper = async (image, script, args, password, options) => {
    const result = await helperResult(image, script, args, password, options);
    if (result.code !== 0) throw new Error('the technical client command failed');
    return result.stdout.trim();
  };
  const removeHelpers = async () => {
    const ownedPods = [...helperClients.values()];
    if (ownedPods.length === 0) return;
    for (const { name, uid } of ownedPods) {
      const current = await kubectl(['-n', ns, 'get', 'pod', name, '-o', 'json']);
      if (current.code !== 0) {
        if (kubernetesNotFound(current)) { pods.removed.push(name); continue; }
        throw new Error('technical client Pod lookup failed; cleanup needs attention');
      }
      assertOwnedTechnicalPod(JSON.parse(current.stdout), ns, name, uid, labelMap);
    }
    const removed = await removeTechnicalPods({
      target: eks, pods: ownedPods.filter(({ name }) => !pods.removed.includes(name)), namespace: ns, requiredLabels: labelMap,
      waitForAbsent: async (check) => {
        try { return await waitFor(check, { timeoutMs: 120_000, intervalMs: 1000, what: 'the exact technical client Pod to be absent' }); }
        catch { return false; }
      },
    }).catch(() => { throw new Error('technical client Pod could not be removed or its absence was not proven; cleanup needs attention'); });
    pods.removed.push(...removed);
  };
  const pgUser = await resolved(readyDeployments.backend, 'PGUSER');
  const pgDb = await resolved(readyDeployments.backend, 'PGDATABASE');
  const pgPort = await resolved(readyDeployments.backend, 'PGPORT');
  const redisPort = await resolved(readyDeployments.backend, 'REDIS_PORT');
  const redisUser = await resolved(readyDeployments.backend, 'REDIS_USERNAME');
  const psql = async (sql) => (inClusterData
    ? tech.psql(readyDeployments.backend, sql)
    : helper(input.aws.probePostgresImage, 'IFS= read -r PGPASSWORD; export PGPASSWORD PGSSLMODE=require; exec psql -h "$1" -p "$2" -U "$3" -d "$4" -tAF "|" -c "$5"', [pgHost, pgPort, pgUser, pgDb, sql], pgPassword));
  const redisInCluster = scenario !== 'A02';
  const redis = async (...command) => (redisInCluster
    ? tech.redis(readyDeployments.backend, ...command)
    : helper(input.aws.probeRedisImage, `IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; host="$1"; port="$2"; user="$3"; tls="$4"; shift 4; if [ "$tls" = true ]; then exec redis-cli --no-auth-warning --tls -h "$host" -p "$port" --user "$user" "$@"; else exec redis-cli --no-auth-warning -h "$host" -p "$port" --user "$user" "$@"; fi`, [redisHost, redisPort, redisUser, String(redisTls), ...command], redisPassword));

  // Managed targets launch their labelled client Pods here, before job submission
  // and before the no-refill baseline is captured. The same clients are reused below.
  await warmTechnicalClients({ psql, redis });




  // Endpoints identify the shared logical PostgreSQL/Redis services. In-cluster
  // stateful services also retain their StatefulSet and PVC identities.
  const sharedServiceIdentity = async () => {
    const identity = { postgres: [pgHost, pgPort, pgDb, pgUser].join('|'), redis: [redisHost, redisPort, redisUser].join('|') };
    if (inClusterData || scenario === 'A03') {
      identity.statefulsets = (await tech.statefulsets()).items
        .filter((set) => (set.spec?.template?.spec?.containers ?? []).some((container) => /(?:postgres|redis)/i.test(container.image ?? '')))
        .map((set) => ({ name: set.metadata?.name, uid: set.metadata?.uid, claims: (set.spec?.volumeClaimTemplates ?? []).map((claim) => claim.metadata?.name).sort() }))
        .sort((a, b) => String(a.name).localeCompare(String(b.name)));
      identity.claims = (await tech.json(['-n', ns, 'get', 'persistentvolumeclaims', '-o', 'json'])).items
        .map((claim) => ({ name: claim.metadata?.name, uid: claim.metadata?.uid, volume: claim.spec?.volumeName }))
        .sort((a, b) => String(a.name).localeCompare(String(b.name)));
    }
    return identity;
  };
  const serviceIdentityBefore = await sharedServiceIdentity();

  // ===================== JOB / CACHE =====================
  phase('JOB', 'Công việc: mở frontend vừa triển khai trên EKS, gửi một job bằng form có sẵn rồi chờ worker xử lý');
  const front = await openFrontend({ ...kit, input: { ...input, kube: eks } });
  const mandatory = MANAGED_ROWS;
  await waitRowsPass(front.app, mandatory, 360_000);
  const lookupsBeforeJob = (await readAppJSON(front.app, '/api/checks')).cache?.lookups ?? {};
  const checksBody = await readAppJSON(front.app, '/api/checks');
  const rows = await readCheckRows(front.app);
  const secretRow = rows.find(([name]) => name === 'secret');
  rec.record('app.rows-pass', 'JOB', 'ui', mandatory.every((n) => rows.some(([name, status]) => name === n && status === 'PASS')) && checksBody.checks?.environment === true && checksBody.checks?.database === true && checksBody.checks?.cache === true
    && !JSON.stringify(checksBody).includes(pgPassword) && !JSON.stringify(checksBody).includes(redisPassword), `mandatory rows PASS: ${rows.filter(([n]) => mandatory.includes(n)).map(([n, s]) => `${n}:${s}`).join(', ')}; no secret value in /api/checks`);
  rec.record('app.secret-row-not-configured', 'JOB', 'ui', checksBody.checks?.secret === false && secretRow?.[1] !== 'PASS', `LABELLED EXCEPTION: the optional ACCEPTANCE_SECRET binding is NOT_CONFIGURED on EKS (row "${secretRow?.[1]}", never PASS); PostgreSQL/Redis credentials are still Secret references and the V04 run proved the real scoped secret`);
  const payload = `${scenario.toLowerCase()} acceptance ${input.runId}`;
  const job = await submitJob(front.ah, front.app, payload);
  const expected = `processed:${payload.toUpperCase()}`;
  const initialWorkerPods = (await tech.pods()).items.filter((p) => p.metadata?.name?.startsWith('worker'));
  const workerIdentity = new Map(initialWorkerPods.map((p) => [p.metadata.name, p.metadata.uid]));
  rec.record('job.completed-ui', 'JOB', 'ui', job.cells[2] === 'DONE' && job.cells[3] === expected && workerIdentity.has(job.cells[4]) && Boolean(workerIdentity.get(job.cells[4])), `job ${job.id} DONE, result matches, processed_by names a worker Pod with a recorded UID`);
  const firstPayloadCount = await psql(`SELECT count(*) FROM jobs WHERE payload = '${payload}'`);
  rec.record('job.no-duplicate', 'JOB', 'private-read', firstPayloadCount === '1', `PostgreSQL contains exactly one row for the UI-submitted payload (count ${firstPayloadCount})`);
  phase('CACHE', 'Bộ nhớ đệm: worker ghi cơ sở dữ liệu rồi ghi Redis; backend đọc cache; sau khi TTL tự hết hạn, tải lại để thấy fallback cơ sở dữ liệu và nạp lại Redis');
  const cacheKey = cacheKeyOf(input, job.id);
  const exists = await redis('EXISTS', cacheKey);
  const ttl = Number(await redis('TTL', cacheKey));
  const row = (await psql(`SELECT status, result, processed_by, cache_write FROM jobs WHERE id = ${job.id}`)).split('|');
  const workerNames = new Set(workerIdentity.keys());
  const lookupsAtWorkerWrite = (await readAppJSON(front.app, '/api/checks')).cache?.lookups ?? {};
  rec.record('cache.worker-write', 'CACHE', 'private-read', exists === '1' && ttl > 0 && ttl <= input.cache.ttlSeconds && row[3] === 'OK' && workerNames.has(row[2]) && (lookupsAtWorkerWrite['miss-refilled'] ?? 0) === (lookupsBeforeJob['miss-refilled'] ?? 0), `Redis key exists with TTL ${ttl}s, PostgreSQL records cache_write ${row[3]} by worker ${row[2]}, and no backend DB refill preceded the observation`);
  const hit = await readAppJSON(front.app, `/api/jobs/${job.id}`);
  rec.record('cache.backend-hit', 'CACHE', 'private-read', hit.cache === 'hit' && hit.status === 'DONE', `GET job ${job.id}: ${hit.cache}`);
  rec.record('job.postgres-row', 'JOB', 'private-read', row[0] === 'DONE' && row[1] === expected && workerIdentity.has(row[2]) && Boolean(workerIdentity.get(row[2])) && row[3] === 'OK', `database row ${job.id}: ${row[0]}, result matches, processed_by ${row[2]} (Pod UID ${workerIdentity.get(row[2])}), cache_write ${row[3]}`);
  const before = (await readAppJSON(front.app, '/api/checks')).cache.lookups ?? {};
  await waitFor(async () => (await redis('EXISTS', cacheKey)) === '0', { timeoutMs: (input.cache.ttlSeconds + 60) * 1000, intervalMs: 2000, what: 'the cache TTL to expire' });
  rec.record('cache.ttl-expiry-miss', 'CACHE', 'private-read', true, `the Redis key expired by its own TTL within ${input.cache.ttlSeconds + 60}s (never deleted)`);
  await front.app.reload();
  const reloaded = front.app.locator('#jobs tr.job').filter({ hasText: payload }).first();
  const sameCells = (await reloaded.locator('td').allInnerTexts()).map((t) => t.trim());
  await front.ah.moveTo(reloaded);
  await front.ah.pause(SHORT_READ);
  const after = (await readAppJSON(front.app, '/api/checks')).cache.lookups ?? {};
  const refillTTL = Number(await redis('TTL', cacheKey));
  rec.record('cache.refill', 'CACHE', 'private-read', (after['miss-refilled'] ?? 0) > (before['miss-refilled'] ?? 0) && (await redis('EXISTS', cacheKey)) === '1' && refillTTL > 0 && refillTTL <= input.cache.ttlSeconds, `miss-refilled ${before['miss-refilled'] ?? 0} -> ${after['miss-refilled'] ?? 0}; key refilled in Redis with positive TTL ${refillTTL}s`);
  rec.record('job.reload-same-result', 'JOB', 'ui', sameCells.join('|') === job.cells.join('|') && sameCells[3] === expected, 'the reloaded row shows the same job and result (database fallback)');

  // ===================== UPDATE / RESTART =====================
  phase('UPDATE', 'Cập nhật rồi khởi động lại orchestrator: đổi cấu hình và triển khai lại; EKS, VPC, cơ sở dữ liệu không được tạo thêm, đích đã lưu giữ nguyên và xác thực được cấp mới ở mỗi thao tác');
  // Stable infrastructure identity: VPC, EKS, Aurora, ElastiCache and IAM roles by exact ID (transient ENIs/volumes are not identity).
  const infraSet = () => toAssets(taggedArns()).filter((a) => !['arn', 'ecr-repository'].includes(a.kind)).map((a) => `${a.kind}:${a.id}`).sort();
  const infraBefore = infraSet();
  await signOut();
  await signIn('developer');
  await kit.openApplication();
  await kit.openSettings();
  await kit.editVariable('ACCEPTANCE_CACHE_TTL_SECONDS', String(input.cache.ttlSeconds + 15));
  await kit.leaveSettings();
  await kit.editWorkloadFromScore(`${scoreDir}/backend.yaml`, 'backend');
  await kit.editWorkloadFromScore(`${scoreDir}/worker.yaml`, 'worker');
  await kit.pendingPreview(2);
  await kit.deployAndWait(input.deployTimeoutMs);
  await waitFor(async () => {
    const deployments = (await tech.deployments()).items;
    return ['backend', 'worker'].every((name) => {
      const d = deployments.find((x) => x.metadata.name === name);
      return d && tech.envOf(d, 'ACCEPTANCE_CACHE_TTL_SECONDS')?.value === String(input.cache.ttlSeconds + 15) && d.status?.updatedReplicas === d.spec.replicas && d.status?.readyReplicas === d.spec.replicas && d.status?.observedGeneration === d.metadata.generation;
    });
  }, { timeoutMs: input.readyTimeoutMs, intervalMs: 4000, what: 'backend and worker to roll out with the updated configuration' });
  const rowAfterUpdate = (await psql(`SELECT status, result, processed_by, cache_write FROM jobs WHERE id = ${job.id}`)).split('|');
  const resultAfterUpdate = (await front.app.locator('#jobs tr.job').filter({ hasText: payload }).first().locator('td').allInnerTexts()).map((text) => text.trim());
  const serviceIdentityAfterUpdate = await sharedServiceIdentity();
  rec.record('update.data-and-worker-preserved', 'READY', 'private-read', rowAfterUpdate[0] === 'DONE' && rowAfterUpdate[1] === expected && workerNames.has(rowAfterUpdate[2]) && rowAfterUpdate[3] === 'OK' && resultAfterUpdate[3] === expected && JSON.stringify(serviceIdentityAfterUpdate) === JSON.stringify(serviceIdentityBefore),
    `same persisted job/result/worker attribution and shared PostgreSQL/Redis endpoint plus applicable StatefulSet/PVC identities survive both workload configuration rollouts`);
  const afterUpdate = infraSet();
  rec.record('update.config-rollout', 'READY', 'private-read', true, `backend and worker rolled out with ACCEPTANCE_CACHE_TTL_SECONDS ${input.cache.ttlSeconds + 15} through a normal Preview and Deploy`);
  rec.record('update.no-duplicate-infra', 'READY', 'private-read', JSON.stringify(afterUpdate) === JSON.stringify(infraBefore) && afterUpdate.filter((a) => a.startsWith('eks-cluster:')).length === 1,
    `the run-tagged AWS asset set is identical after the update (${afterUpdate.length} assets, one EKS cluster): nothing was duplicated or replaced`);
  await signOut();
  const { execFileSync: run } = await import('node:child_process');
  run(process.env.ORCH_E2E_RESTART_CMD ?? 'true', { stdio: 'ignore' });
  await page.reload();
  await expect(page.getByLabel('Tên đăng nhập', { exact: true }).or(page.getByRole('button', { name: 'Đăng xuất', exact: true }))).toBeVisible({ timeout: 120_000 });
  if (await page.getByRole('button', { name: 'Đăng xuất', exact: true }).isVisible()) await signOut();
  await signIn('developer');
  await kit.openApplication();
  // An unchanged Score is correctly a no-op after restart. Make a second
  // valid configuration change so the operation proves a real fresh deployment.
  await kit.openSettings();
  await kit.editVariable('ACCEPTANCE_CACHE_TTL_SECONDS', String(input.cache.ttlSeconds + 30));
  await kit.leaveSettings();
  await kit.editWorkloadFromScore(`${scoreDir}/backend.yaml`, 'backend');
  await kit.editWorkloadFromScore(`${scoreDir}/worker.yaml`, 'worker');
  await kit.pendingPreview(2);
  await kit.deployAndWait(input.deployTimeoutMs);
  await waitFor(async () => {
    const deployments = (await tech.deployments()).items;
    return ['backend', 'worker'].every((name) => {
      const d = deployments.find((x) => x.metadata.name === name);
      return d && tech.envOf(d, 'ACCEPTANCE_CACHE_TTL_SECONDS')?.value === String(input.cache.ttlSeconds + 30) && d.status?.updatedReplicas === d.spec.replicas && d.status?.readyReplicas === d.spec.replicas && d.status?.observedGeneration === d.metadata.generation;
    });
  }, { timeoutMs: input.readyTimeoutMs, intervalMs: 4000, what: 'backend and worker to roll out with the post-restart configuration' });
  const targetAfter = await identityNow();
  const kubeSystemAfter = (await kubectl(['get', 'namespace', 'kube-system', '-o', 'jsonpath={.metadata.uid}'])).stdout.trim();
  const infraAfterRestart = infraSet();
  const serviceIdentityAfterRestart = await sharedServiceIdentity();
  rec.record('restart.operation-after', 'READY', 'ui', true, 'an operation after the orchestrator restart deployed with freshly derived EKS authentication (no persisted token exists)');
  rec.record('restart.target-unchanged', 'READY', 'private-read', JSON.stringify([target.cluster, target.account, target.namespace, target.accountVersion, target.clusterNode]) === JSON.stringify([targetAfter?.cluster, targetAfter?.account, targetAfter?.namespace, targetAfter?.accountVersion, targetAfter?.clusterNode]) && kubeSystemAfter === kubeSystemUid && JSON.stringify(infraAfterRestart) === JSON.stringify(infraBefore) && JSON.stringify(serviceIdentityAfterRestart) === JSON.stringify(serviceIdentityBefore),
    `the per-resource target, kube-system UID, shared PostgreSQL/Redis endpoint, applicable StatefulSet/PVC and run-tagged managed-service identities are unchanged after update, restart and redeploy`);
  const rowAfterRestart = (await psql(`SELECT status, result, processed_by, cache_write FROM jobs WHERE id = ${job.id}`)).split('|');
  await front.app.reload();
  const persistedCells = (await front.app.locator('#jobs tr.job').filter({ hasText: payload }).first().locator('td').allInnerTexts()).map((text) => text.trim());
  const restartWorkerPods = new Map((await tech.pods()).items.filter((p) => p.metadata?.name?.startsWith('worker')).map((p) => [p.metadata.name, p.metadata.uid]));
  const persistedCount = await psql(`SELECT count(*) FROM jobs WHERE payload = '${payload}'`);
  rec.record('restart.job-persisted', 'READY', 'ui', persistedCells.join('|') === job.cells.join('|') && persistedCells[2] === 'DONE' && persistedCells[3] === expected && rowAfterRestart.join('|') === row.join('|') && workerIdentity.has(rowAfterRestart[2]) && Boolean(workerIdentity.get(rowAfterRestart[2])) && persistedCount === '1', 'the same single job, result and original recorded worker attribution remain unchanged in UI/PostgreSQL after restart and redeploy; the following new-job assertion verifies a current worker Pod and UID');
  const newPayload = `${payload} after restart`;
  const newJob = await submitJob(front.ah, front.app, newPayload);
  const newRow = (await psql(`SELECT status, result, processed_by, cache_write FROM jobs WHERE id = ${newJob.id}`)).split('|');
  const newCount = await psql(`SELECT count(*) FROM jobs WHERE payload = '${newPayload}'`);
  rec.record('restart.new-job', 'READY', 'ui', newJob.cells[2] === 'DONE' && newJob.cells[3] === `processed:${newPayload.toUpperCase()}` && newRow[0] === 'DONE' && newRow[1] === newJob.cells[3] && restartWorkerPods.has(newRow[2]) && Boolean(restartWorkerPods.get(newRow[2])) && newRow[3] === 'OK' && newJob.id !== job.id && newCount === '1',
    `new UI-submitted job ${newJob.id} is processed after restart by worker Pod ${newRow[2]} (UID ${restartWorkerPods.get(newRow[2])}) with one persisted row`);

  await signOut();
  await front.forwarding.stop().catch(() => {});

  if (!inClusterData) {
    ctx.caption('Dọn Pod kiểm chứng kỹ thuật sau các bước nghiệp vụ: xác minh ownership và UID, xóa rồi đọc API để chứng minh đã vắng.');
    await removeHelpers();
    const leftoverLabelled = (await json(['-n', ns, 'get', 'pods', '-l', `run-id=${input.runId},managed-by=orchestrator-verification,app.kubernetes.io/component=technical-probe`, '-o', 'json'])).items.length;
    const exactRemaining = await Promise.all(pods.created.map(async (name) => { const r = await kubectl(['-n', ns, 'get', 'pod', name, '-o', 'name']); return !kubernetesNotFound(r); }));
    rec.record('technical.pods-owned-and-removed', 'CLEANUP', 'private-read', pods.created.length > 0 && pods.removed.length === pods.created.length && pods.leftover.length === 0 && leftoverLabelled === 0 && exactRemaining.every((e) => !e),
      `${pods.created.length} run-labelled technical client Pod(s) created (exact names recorded in the manifest) and ${pods.removed.length} verified absent`);
    if (pods.leftover.length > 0) throw new Error('a technical client Pod is still present; cleanup needs attention');
  }


  // ===================== CLEANUP (independent supervisor) =====================
  phase('CLEANUP', 'Dọn dẹp: lưới an toàn độc lập hủy theo thứ tự ngược bằng module và đích ban đầu, rồi truy vấn từng ID vật lý và thẻ của lần chạy qua API dịch vụ');
  ctl(input, 'supervisor-finish', '-private', input.privateDir);
  let waited = '';
  try {
    waited = ctl(input, 'supervisor-wait', '-private', input.privateDir, '-timeout', input.cleanupWaitDuration);
  } catch (error) {
    rec.record('cleanup.supervisor-clean', 'CLEANUP', 'private-read', false, safeError(error));
  }
  rec.record('cleanup.supervisor-clean', 'CLEANUP', 'private-read', /"state": "CLEAN"/.test(waited), 'supervisor reached CLEAN (each layer verified absent by exact IDs and the run tag)');
  let verify = '';
  let verifyFailure = '';
  try { verify = verifyCleanupAWSAbsence(ctl, input); } catch (error) { verifyFailure = safeError(error); }
  rec.record('cleanup.aws-absent', 'CLEANUP', 'private-read', verify === 'layer aux verified absent', verify ? 'the fresh authoritative AWS verifier proved all auxiliary resources absent by exact IDs, ownership, identity and run tag' : `AWS auxiliary resource absence is unverified: ${verifyFailure}`);
  if (!ctx.browserOnly) await page.waitForTimeout(5000);
  writeFileSync(`${ctx.evidenceDir}/outcome.json`, JSON.stringify({ jobId: job.id, cacheKey, scenario }, null, 2));
}
