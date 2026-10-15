// Real Terraform v2 acceptance V2-A02 / V2-A03 on AWS (M04 I04-10 code; I04-12 live
// execution by Codex after the I04-11 gate). Invoked from terraform-v2-acceptance.mjs;
// every business step is a Web Console or acceptance-app UI action. Read-only technical
// assertions use the pinned aws CLI (operator host chain, never the product) and kubectl
// over a private EKS kubeconfig. Setup mutations are three supervised ECR repositories and short-lived, labelled technical client Pods
// (run-id / managed-by labels, digest-pinned image) whose exact name is recorded as an owned asset and whose
// removal is verified; no job, Workload or resource is created through an API.
// A02: Redis cloud-managed (ElastiCache) + PostgreSQL cloud-managed (Aurora).
// A03: Redis in-cluster on EKS + PostgreSQL cloud-managed (Aurora).
// The shared catalog (Types, Definitions, classes) is backend/test/integration/v2run/testdata/aws-catalog.json,
// planned by backend/internal/planning/awsacceptance_plan_test.go.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { createHuman } from './human.mjs';
import { expectVisibleCursor } from './human.mjs';
import { kubectlRunner, safeError, startPortForward, waitFor } from './terraform-v2-lib.mjs';
import { SHORT_READ, steps } from './terraform-v2-steps.mjs';


/** Read-only AWS CLI of the operator (host chain handles only; the product never uses it). */
function awsCli(input) {
  const env = { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', AWS_REGION: input.aws.region, AWS_DEFAULT_REGION: input.aws.region, AWS_PAGER: '', ...input.aws.hostEnv };
  return (...args) => JSON.parse(execFileSync(input.aws.bin, ['--region', input.aws.region, '--output', 'json', ...args], { encoding: 'utf8', env, timeout: 120_000, stdio: ['ignore', 'pipe', 'pipe'] }) || '{}');
}

/** Parses the run-tagged ARNs into manifest assets of the exact owner chain. */
export function assetsFromArns(arns) {
  const out = [];
  for (const arn of arns) {
    let m;
    if ((m = /^arn:aws:ec2:[^:]+:\d+:vpc\/(vpc-[0-9a-f]+)$/.exec(arn))) out.push({ kind: 'vpc', id: m[1], layer: 'vpc' });
    else if ((m = /^arn:aws:eks:[^:]+:\d+:cluster\/([A-Za-z0-9-_]+)$/.exec(arn))) out.push({ kind: 'eks-cluster', id: m[1], layer: 'eks' });
    else if ((m = /^arn:aws:rds:[^:]+:\d+:cluster:([a-z0-9-]+)$/.exec(arn))) out.push({ kind: 'rds-cluster', id: m[1], layer: 'app-resources' });
    else if ((m = /^arn:aws:elasticache:[^:]+:\d+:replicationgroup:([a-z0-9-]+)$/.exec(arn))) out.push({ kind: 'elasticache-group', id: m[1], layer: 'app-resources' });
    else if ((m = /^arn:aws:ecr:[^:]+:\d+:repository\/(.+)$/.exec(arn))) out.push({ kind: 'ecr-repository', id: m[1], layer: 'aux' });
    else if ((m = /^arn:aws:iam::\d+:role\/(.+)$/.exec(arn))) out.push({ kind: 'iam-role', id: m[1], layer: 'aux' });
  }
  return out;
}

/** Keeps the preview gate's independent proofs visible without exposing AWS keys. */
export function previewPurityProof({ baseline, expectedArns, expectedAssets, ownedKeys, afterPreview, deploymentCount, resourceCount, panelText, accessKeyId, secretAccessKey }) {
  const sorted = (values) => [...values].sort();
  const exactPreparation = expectedArns.length === 3
    && JSON.stringify(sorted(baseline)) === JSON.stringify(sorted(expectedArns))
    && expectedAssets.length === 3
    && expectedAssets.every((asset) => asset?.kind === 'ecr-repository' && ownedKeys.has(`${asset.kind}\t${asset.id}`));
  const unchanged = JSON.stringify(sorted(afterPreview)) === JSON.stringify(sorted(baseline));
  const stateFree = deploymentCount === 0 && resourceCount === 0;
  // Preserve the established broad credential-text guard, including configured key values.
  const noKeyMaterial = !/AKIA|secret/i.test(panelText.replace(/bí mật/g, ''))
    && !(accessKeyId && panelText.includes(accessKeyId))
    && !(secretAccessKey && panelText.includes(secretAccessKey));
  return { exactPreparation, unchanged, stateFree, noKeyMaterial };
}

export async function awsScenario(ctx, scenario, { v2ctl, recordNamespace, forbidden }) {
  const { page, h, rec, input } = ctx;
  const s = steps(ctx);
  const phase = ctx.phase;
  const aws = awsCli(input);
  const catalog = JSON.parse(readFileSync(input.aws.catalogFile, 'utf8'));
  const scope = `/applications/${input.applicationKey}/environments/staging`;
  const classes = catalog.classes[scenario];
  const pageIsSafe = async (label) => {
    const text = await page.locator('body').innerText();
    for (const pattern of forbidden) expect(text, `${label}: ${pattern}`).not.toMatch(pattern);
    expect(text, `${label}: access key`).not.toContain(input.aws.secretAccessKey);
  };
  const taggedArns = () => (aws('resourcegroupstaggingapi', 'get-resources', '--tag-filters', `Key=run-id,Values=${input.runId}`).ResourceTagMappingList ?? []).map((r) => r.ResourceARN);
  // Every run-tagged ARN is an owned asset of the manifest: parsed kinds carry the exact owner chain, any other ARN is recorded as kind "arn".
  // A manifest write failure is NEVER swallowed: it is kept and the final acknowledgement fails the run (cleanup then rests on the run-tag sweep).
  const ownedKey = (a) => `${a.kind}\t${a.id}`;
  const toAssets = (arns) => arns.map((arn) => assetsFromArns([arn])[0] ?? { kind: 'arn', id: arn, layer: 'aux' });
  const recordFailures = [];
  const recordAssets = () => {
    for (const a of toAssets(taggedArns())) {
      try {
        v2ctl(input, 'manifest-asset', '-manifest', input.manifestFile, '-kind', a.kind, '-id', a.id, '-layer', a.layer, '-owner', 'CREATED_BY_RUN');
      } catch (error) {
        recordFailures.push({ kind: a.kind, message: safeError(error) });
        throw new Error(`the run manifest could not acknowledge an AWS asset (${a.kind}); cleanup requires attention: ${safeError(error)}`);
      }
    }
  };
  const ownershipAcknowledged = () => {
    const owned = new Set(v2ctl(input, 'manifest-owned', '-manifest', input.manifestFile).split('\n').filter(Boolean));
    const missing = toAssets(taggedArns()).filter((a) => !owned.has(ownedKey(a)));
    return { missing, ok: missing.length === 0 };
  };
  // Technical client Pods: labelled, exactly named, recorded and removed (a setup mutation, not a read-only query).
  const pods = { created: [], removed: [], leftover: [] };
  const labels = `run-id=${input.runId},managed-by=orchestrator-verification,app.kubernetes.io/component=technical-probe`;

  // ===================== PREPARE / FORM: Platform Engineer =====================
  phase('PREPARE', `Chuẩn bị ${scenario}: Kỹ sư nền tảng đăng nhập; danh mục, Kết nối AWS và Mẫu dựng đều bắt đầu trống và được tạo bằng giao diện`);
  await h.showCursor();
  await expectVisibleCursor(page);
  await s.signIn('platform-engineer');
  rec.record('ui.signin', 'PREPARE', 'ui', true, 'platform-engineer');
  await h.pause(SHORT_READ);
  const connectionKey = await s.registerAwsConnection({ name: input.aws.connectionName, accessKeyId: input.aws.accessKeyId, secretAccessKey: input.aws.secretAccessKey, region: input.aws.region });

  phase('FORM', 'Biểu mẫu: đăng ký Loại tài nguyên, Cấu hình tài nguyên AWS (module đã ghim) và Mẫu dựng score-k8s cho AWS');
  await s.registerTypes(catalog.types);
  const { defs, pinned, capabilities } = await s.registerAwsDefinitions(catalog, scenario, { EGRESS: input.aws.egressCidr, NODE_TYPE: input.aws.nodeInstanceType });
  await s.registerRenderTemplate('AWS', 'render-aws');
  const wanted = [...catalog.definitions.common, ...catalog.definitions[scenario]].map((d) => d.id);
  rec.record('ui.definitions.registered', 'FORM', 'ui', wanted.every((id) => defs.includes(id)) && pinned, `definitions ${defs.join(',')} pinned=${pinned}`);
  await s.signOut();

  // ===================== FORM: Developer =====================
  await s.signIn('developer');
  const env = await s.bindEnvironment(connectionKey, 'aws-eks');
  recordNamespace(input, env);
  await s.putVariables([
    ['PORT', '8080'], ['POLL_INTERVAL_MS', '500'], ['ACCEPTANCE_CONFIG', 'acceptance-config-ok'],
    ['ACCEPTANCE_CACHE_APP', input.applicationKey], ['ACCEPTANCE_CACHE_ENV', 'staging'], ['ACCEPTANCE_CACHE_RUN', input.runId], ['ACCEPTANCE_CACHE_TTL_SECONDS', String(input.cache.ttlSeconds)],
  ]);
  await s.addAcceptanceWorkloads(classes, { secret: false });
  const drafts = (await s.get(`${scope}/workloads`)).workloads ?? [];
  rec.record('ui.workloads.three-saved', 'FORM', 'ui', ['backend', 'worker', 'frontend'].every((n) => drafts.some((w) => w.id === n)), 'three Workload drafts saved with the cloud classes');

  // ===================== PREVIEW (pure) =====================
  phase('PREVIEW', 'Xem trước: kiểm tra Definition, module đã ghim, tài khoản/vùng AWS, hồ sơ aws-eks, quan hệ VPC → EKS → namespace → Aurora/cache và bundle score-k8s');
  // The runner publishes exactly three immutable ECR images before the browser
  // gate. Their ARNs and repository names are the only allowed tagged baseline.
  const baseline = taggedArns().sort();
  const prepared = input.aws.ecrPreparation;
  const expectedArns = [...(prepared?.repositories ?? [])].sort();
  const expectedAssets = expectedArns.map((arn) => assetsFromArns([arn])[0]);
  const ownedBeforePreview = new Set(v2ctl(input, 'manifest-owned', '-manifest', input.manifestFile).split('\n').filter(Boolean));
  const previewResponse = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(`${scope}/preview`));
  await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  const preview = await (await previewResponse).json();
  const panel = page.getByLabel('Xem trước triển khai');
  await expect(panel).toContainText('3 Workload bị ảnh hưởng', { timeout: 180_000 });
  const panelText = await panel.innerText();
  await h.moveTo(panel);
  await h.pause(SHORT_READ);
  const intents = new Map();
  for (const change of preview.changes) for (const intent of change.resourceIntents ?? []) intents.set(intent.descriptor, intent);
  const terraformIntents = [...intents.values()].filter((i) => i.driverType === 'terraform');
  const byType = (type) => [...intents.values()].filter((i) => i.resourceType === type);
  const pinOf = (module) => capabilities.find((c) => c.driverType === 'terraform' && c.module === module)?.source;
  const deploymentCount = ((await s.get(`${scope}/deployments`)).deployments ?? []).length;
  const resourceCount = ((await s.get(`${scope}/resources`)).resources ?? []).length;
  const afterPreview = taggedArns().sort();
  const purity = previewPurityProof({ baseline, expectedArns, expectedAssets, ownedKeys: ownedBeforePreview, afterPreview, deploymentCount, resourceCount, panelText, accessKeyId: input.aws.accessKeyId, secretAccessKey: input.aws.secretAccessKey });
  const purityPassed = Object.values(purity).every(Boolean);
  rec.record('preview.pure', 'PREVIEW', 'private-read', purityPassed,
    `exact ECR preparation=${purity.exactPreparation}; unchanged after preview=${purity.unchanged}; no deployment/resource state=${purity.stateFree}; no AWS key material on screen=${purity.noKeyMaterial}`);
  rec.record('preview.target', 'PREVIEW', 'ui', preview.binding?.connectionKey === connectionKey && preview.binding?.executionProfile === 'aws-eks' && preview.accountId === input.aws.accountId && preview.binding?.region === input.aws.region,
    `connection ${preview.binding?.connectionKey} profile ${preview.binding?.executionProfile} account ${preview.accountId} region ${preview.binding?.region}`);
  const expectedModules = ['vpc', 'eks', 'k8s-namespace', 'aurora', scenario === 'A02' ? 'elasticache' : 'redis-k8s'].sort();
  const pinsOk = terraformIntents.length === expectedModules.length && terraformIntents.map((i) => i.module?.module).sort().join() === expectedModules.join()
    && terraformIntents.every((i) => { const pin = pinOf(i.module?.module); return pin && i.module.contentSha256 === pin.contentSha256 && i.module.revision === pin.revision; });
  rec.record('preview.pins', 'PREVIEW', 'ui', pinsOk, terraformIntents.map((i) => `${i.module?.module}@${String(i.module?.contentSha256).slice(0, 12)}`).join(' '));
  const redisClass = scenario === 'A02' ? 'cloud-managed' : 'in-cluster';
  const graphOk = byType('vpc').length === 1 && byType('k8s-cluster').length === 1 && byType('k8s-namespace').length === 1 && byType('postgres').length === 1 && byType('redis').length === 1
    && byType('postgres')[0].descriptor.startsWith('postgres.cloud-managed#shared.') && byType('redis')[0].descriptor.startsWith(`redis.${redisClass}#shared.`)
    && terraformIntents.every((i) => i.intent === 'CREATE' && i.ownership === 'ORCHESTRATOR');
  rec.record('preview.graph', 'PREVIEW', 'ui', graphOk, `${[...intents.keys()].length} descriptors; Aurora ${byType('postgres')[0]?.descriptor}; cache ${byType('redis')[0]?.descriptor}`);
  const bundles = preview.changes.map((c) => c.rendering?.bundle);
  rec.record('preview.score-k8s-bundle', 'PREVIEW', 'ui', preview.changes.length === 3 && bundles.every((b) => b?.id === 'score-k8s-internal-v1' && b.version === input.scoreVersion), `bundle ${bundles[0]?.id} ${bundles[0]?.version}`);
  await h.read('Xem trước chỉ đọc: chưa có tài nguyên AWS nào mang thẻ của lần chạy');

  // ===================== TERRAFORM / READY =====================
  phase('TERRAFORM', 'Triển khai: Terraform tạo VPC, EKS, namespace, Aurora và bộ nhớ đệm theo thứ tự; tiến trình lấy từ dữ liệu đã lưu');
  const assetTimer = setInterval(() => { try { recordAssets(); } catch { /* retained in recordFailures and failed at the ownership gate */ } }, 45_000);
  ctx.forwarders.push({ stop: () => clearInterval(assetTimer) });
  await ctx.deploy();
  const result = page.getByLabel('Kết quả triển khai');
  await expect(result).toContainText('Triển khai thành công', { timeout: input.deployTimeoutMs });
  clearInterval(assetTimer);
  recordAssets();
  for (const name of ['backend', 'worker', 'frontend']) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [^:]+: thành công`) })).toHaveCount(1);
  await h.moveTo(result);
  await h.pause(SHORT_READ);
  const deployments = (await s.get(`${scope}/deployments`)).deployments ?? [];
  const progressRows = [];
  for (const d of deployments) progressRows.push(...((await s.get(`${scope}/deployments/${d.id}`)).resourceProgress ?? []));
  rec.record('deploy.terraform-stages', 'TERRAFORM', 'ui', progressRows.filter((r) => r.stage === 'READY' && r.status === 'SUCCEEDED').length >= expectedModules.length, `${progressRows.length} persisted attempt rows`);
  await pageIsSafe('deployment result');

  phase('READY', 'Sẵn sàng: EKS, ba Workload Ready; Aurora và bộ nhớ đệm do tài nguyên AWS/EKS cung cấp; mật khẩu đi qua Secret, không nằm trong giá trị của Pod');
  const arns = taggedArns();
  const assets = assetsFromArns(arns);
  const ack = ownershipAcknowledged();
  rec.record('ready.ownership-recorded', 'READY', 'private-read', ack.ok && recordFailures.length === 0, `${arns.length} run-tagged asset(s) acknowledged in the manifest; ${ack.missing.length} missing, ${recordFailures.length} write failure(s)`);
  if (!ack.ok || recordFailures.length > 0) throw new Error('the manifest does not acknowledge every run-tagged asset; cleanup needs attention');
  const cluster = assets.find((a) => a.kind === 'eks-cluster');
  rec.record('ready.aws-assets-tagged', 'READY', 'private-read', Boolean(cluster) && assets.some((a) => a.kind === 'vpc') && assets.some((a) => a.kind === 'rds-cluster') && (scenario !== 'A02' || assets.some((a) => a.kind === 'elasticache-group')),
    `run-tagged assets: ${assets.map((a) => a.kind).sort().join(',')}`);
  // Private read-only EKS access for the technical assertions.
  const kubeconfig = `${input.privateDir}/eks.kubeconfig`;
  execFileSync(input.aws.bin, ['--region', input.aws.region, 'eks', 'update-kubeconfig', '--name', cluster.id, '--kubeconfig', kubeconfig, '--alias', 'eks-run'], { env: { PATH: process.env.PATH, HOME: process.env.HOME ?? '/tmp', ...input.aws.hostEnv }, stdio: 'ignore', timeout: 120_000 });
  const eks = { kubectl: input.kube.kubectl, kubeconfig, context: 'eks-run', env: { ...input.aws.hostEnv, AWS_REGION: input.aws.region } };
  const kubectl = kubectlRunner(eks);
  const ns = input.namespace;
  const json = async (args) => { const r = await kubectl(args); if (r.code !== 0) throw new Error(`kubectl ${args[0]} failed`); return JSON.parse(r.stdout); };
  const ready = await waitFor(async () => {
    const list = await json(['-n', ns, 'get', 'deployments', '-o', 'json']);
    const byName = Object.fromEntries(list.items.map((d) => [d.metadata.name, d]));
    return ['backend', 'worker', 'frontend'].every((n) => byName[n] && (byName[n].status?.readyReplicas ?? 0) >= 1 && byName[n].status.readyReplicas === byName[n].spec.replicas) ? byName : false;
  }, { timeoutMs: input.readyTimeoutMs, intervalMs: 5000, what: 'the three Deployments to be Ready on EKS' });
  rec.record('ready.workloads', 'READY', 'private-read', true, 'backend, worker, frontend Ready on EKS');
  const nodes = await json(['get', 'nodes', '-o', 'json']);
  rec.record('ready.nodes', 'READY', 'private-read', nodes.items.length >= 1 && nodes.items.every((n) => n.status.conditions.some((c) => c.type === 'Ready' && c.status === 'True')) && nodes.items.every((n) => n.status.nodeInfo.architecture === input.aws.nodeArch), `${nodes.items.length} Ready ${input.aws.nodeArch} node(s)`);
  const envOf = (d, name) => (d.spec.template.spec.containers[0].env ?? []).find((e) => e.name === name);
  const secretValue = async (name, key) => { const r = await kubectl(['-n', ns, 'get', 'secret', name, '-o', `jsonpath={.data.${key}}`]); if (r.code !== 0 || !r.stdout) throw new Error('secret value is not readable'); return Buffer.from(r.stdout, 'base64').toString('utf8'); };
  const resolved = async (d, name) => {
    const e = envOf(d, name);
    if (!e) throw new Error(`${name} is not bound`);
    if (e.value !== undefined) return e.value;
    return secretValue(e.valueFrom.secretKeyRef.name, e.valueFrom.secretKeyRef.key);
  };
  const [pgHost, wPgHost, redisHost, wRedisHost, redisTls] = await Promise.all([resolved(ready.backend, 'PGHOST'), resolved(ready.worker, 'PGHOST'), resolved(ready.backend, 'REDIS_HOST'), resolved(ready.worker, 'REDIS_HOST'), resolved(ready.backend, 'REDIS_TLS')]);
  const secretBound = ['PGPASSWORD', 'REDIS_PASSWORD'].every((n) => ['backend', 'worker'].every((w) => { const e = envOf(ready[w], n); return e?.value === undefined && e?.valueFrom?.secretKeyRef; }));
  rec.record('ready.shared-endpoints', 'READY', 'private-read', pgHost === wPgHost && redisHost === wRedisHost && /rds\.amazonaws\.com$/.test(pgHost)
    && (scenario === 'A02' ? /cache\.amazonaws\.com$/.test(redisHost) : redisHost.length > 0), `backend and worker share one Aurora host; cache host kind ${scenario === 'A02' ? 'ElastiCache' : 'in-cluster'}`);
  rec.record('ready.bindings', 'READY', 'private-read', secretBound, 'passwords via secretKeyRef, no literal value');
  const appSecrets = `${input.privateDir}/app-secrets.txt`;
  const append = (name, value) => writeFileSync(appSecrets, `${name}=${value}\n`, { flag: 'a', mode: 0o600 });
  const pgPassword = await resolved(ready.backend, 'PGPASSWORD');
  const redisPassword = await resolved(ready.backend, 'REDIS_PASSWORD');
  append('PGPASSWORD', pgPassword);
  append('REDIS_PASSWORD', redisPassword);
  const pgUser = await resolved(ready.backend, 'PGUSER');
  const pgDb = await resolved(ready.backend, 'PGDATABASE');
  const pgPort = await resolved(ready.backend, 'PGPORT');
  const redisPort = await resolved(ready.backend, 'REDIS_PORT');
  const redisUser = await resolved(ready.backend, 'REDIS_USERNAME');

  // A short-lived, LABELLED client Pod inside the run namespace (the data is only reachable from the VPC / cluster); the password goes through stdin only.
  // Its exact name is recorded as an owned asset before it exists; it is removed and its absence verified even on failure or timeout.
  const helper = async (image, script, args, password, { timeoutMs = 120_000 } = {}) => {
    const name = `tech-${randomUUID().slice(0, 12)}`;
    pods.created.push(name);
    v2ctl(input, 'manifest-asset', '-manifest', input.manifestFile, '-kind', 'pod', '-id', `${ns}/${name}`, '-layer', 'delivery', '-owner', 'CREATED_BY_RUN', '-linkage', 'technical client Pod of this run');
    let output;
    let operationError;
    try {
      const r = await kubectl(['-n', ns, 'run', name, `--labels=${labels}`, '--rm', '-i', '--restart=Never', '--quiet', `--image=${image}`, '--command', '--', 'sh', '-c', script, 'x', ...args], { timeoutMs, input: `${password}\n` });
      if (r.code !== 0) throw new Error('the technical client Pod failed');
      output = r.stdout.trim();
    } catch (error) {
      operationError = error;
    }
    let cleanupError;
    try {
      const deleted = await kubectl(['-n', ns, 'delete', 'pod', name, '--ignore-not-found', '--wait=true', '--timeout=120s'], { timeoutMs: 150_000 });
      if (deleted.code !== 0) throw new Error('the technical client Pod could not be removed; cleanup needs attention');
      const checked = await kubectl(['-n', ns, 'get', 'pod', name, '-o', 'name']);
      const gone = checked.code !== 0 && /notfound/i.test(checked.stderr);
      (gone ? pods.removed : pods.leftover).push(name);
      if (!gone) throw new Error('the technical client Pod absence could not be verified; cleanup needs attention');
    } catch (error) {
      cleanupError = error;
    }
    if (cleanupError && operationError) throw new AggregateError([operationError, cleanupError], `technical client operation failed and Pod cleanup also failed: ${safeError(operationError)}; ${safeError(cleanupError)}`);
    if (cleanupError) throw cleanupError;
    if (operationError) throw operationError;
    return output;
  };
  const psql = (sql) => helper(input.aws.probePostgresImage, 'IFS= read -r PGPASSWORD; export PGPASSWORD PGSSLMODE=require; exec psql -h "$1" -p "$2" -U "$3" -d "$4" -tAF "|" -c "$5"', [pgHost, pgPort, pgUser, pgDb, sql], pgPassword);
  const redis = (...command) => helper(input.aws.probeRedisImage, `IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; host="$1"; port="$2"; user="$3"; tls="$4"; shift 4; if [ "$tls" = true ]; then exec redis-cli --no-auth-warning --tls -h "$host" -p "$port" --user "$user" "$@"; else exec redis-cli --no-auth-warning -h "$host" -p "$port" --user "$user" "$@"; fi`, [redisHost, redisPort, redisUser, String(redisTls), ...command], redisPassword);

  // ===================== JOB / CACHE (acceptance app UI) =====================
  phase('JOB', 'Công việc: mở frontend vừa triển khai (port-forward), gửi một job bằng form có sẵn rồi chờ worker xử lý xong');
  const services = (await json(['-n', ns, 'get', 'services', '-o', 'json'])).items.map((svc) => svc.metadata.name);
  const frontendService = services.find((n) => n === 'frontend') ?? services.find((n) => n.startsWith('frontend'));
  if (!frontendService) throw new Error('the frontend Service was not delivered');
  const forward = startPortForward({ ...eks, namespace: ns, service: frontendService, remotePort: 8080, localPort: input.forwardPort });
  ctx.forwarders.push(forward);
  await waitFor(async () => (await fetch(`${forward.url}/healthz`)).ok, { timeoutMs: 180_000, intervalMs: 1500, what: 'the frontend port-forward' });
  const app = await ctx.context.newPage();
  const ah = createHuman(app, { selectNative: ctx.selectNative });
  await app.goto(forward.url);
  await expect(app.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
  await ah.showCursor();
  // The optional ACCEPTANCE_SECRET binding is not configured on AWS (acceptance-app README: a missing optional binding shows FAIL while the job form stays
  // available). Its row is read and recorded honestly below; it is neither bound nor counted as PASS. The mandatory rows are waited for.
  for (const name of ['backend connection', 'environment', 'database', 'cache']) {
    const row = app.locator(`[data-check="${name}"]`);
    await waitFor(async () => { await app.reload(); return (await row.innerText()).includes('PASS'); }, { timeoutMs: 300_000, intervalMs: 4000, what: `the ${name} check` });
  }
  const checksBody = await (await fetch(`${forward.url}/api/checks`)).json();
  const mandatoryRows = await Promise.all(['backend connection', 'environment', 'database', 'cache'].map(async (name) => (await app.locator(`[data-check="${name}"]`).innerText()).replace(/\s+/g, ' ').trim()));
  rec.record('app.rows-pass', 'JOB', 'ui', mandatoryRows.every((row) => /PASS$/.test(row) && !/FAIL/.test(row))
    && checksBody.checks?.environment === true && checksBody.checks?.database === true && checksBody.checks?.cache === true
    && !JSON.stringify(checksBody).includes(pgPassword) && !JSON.stringify(checksBody).includes(redisPassword),
    `mandatory rows PASS: ${mandatoryRows.join('; ')} (cache fails closed); no secret value in /api/checks`);
  const secretRow = (await app.locator('[data-check="secret"]').innerText()).replace(/\s+/g, ' ').trim();
  rec.record('app.secret-row-not-configured', 'JOB', 'ui', checksBody.checks?.secret === false && !/PASS/.test(secretRow),
    `optional ACCEPTANCE_SECRET binding is NOT_CONFIGURED on AWS: the row shows "${secretRow.slice(0, 60)}", not PASS (A01 covers the real scoped secret)`);
  const payload = `v2 acceptance ${input.runId}`;
  await ah.type(app.locator('input[name="payload"]'), payload, { replace: true });
  await ah.click(app.getByRole('button', { name: 'Submit job' }));
  const jobRow = app.locator('#jobs tr.job').filter({ hasText: payload }).first();
  await expect(jobRow).toBeVisible();
  await waitFor(async () => { await app.reload(); return (await jobRow.innerText()).includes('DONE'); }, { timeoutMs: 300_000, intervalMs: 2000, what: 'the worker to complete the job' });
  const jobId = Number(await jobRow.getAttribute('data-id'));
  const cells = (await jobRow.locator('td').allInnerTexts()).map((t) => t.trim());
  const expectedResult = `processed:${payload.toUpperCase()}`;
  await ah.moveTo(jobRow);
  await ah.pause(SHORT_READ);
  rec.record('job.completed-ui', 'JOB', 'ui', cells[2] === 'DONE' && cells[3] === expectedResult && cells[4] !== '', `job ${jobId} DONE, result matches, processed by a worker pod`);

  phase('CACHE', 'Bộ nhớ đệm: worker ghi Aurora rồi ghi Redis; backend đọc cache; sau TTL, tải lại bằng giao diện để thấy fallback Aurora và ghi lại Redis');
  const cacheKey = `acceptance:v1:${input.applicationKey}:staging:${input.runId}:job:${jobId}`;
  const exists = await redis('EXISTS', cacheKey);
  const ttl = Number(await redis('TTL', cacheKey));
  rec.record('cache.worker-write', 'CACHE', 'private-read', exists === '1' && ttl > 0 && ttl <= input.cache.ttlSeconds, `Redis key exists with TTL ${ttl}s (<= ${input.cache.ttlSeconds}s)`);
  const hit = await (await fetch(`${forward.url}/api/jobs/${jobId}`)).json();
  rec.record('cache.backend-hit', 'CACHE', 'private-read', hit.cache === 'hit' && hit.status === 'DONE', `GET job ${jobId}: ${hit.cache}`);
  const row = (await psql(`SELECT status, result, cache_write FROM jobs WHERE id = ${jobId}`)).split('|');
  rec.record('job.postgres-row', 'JOB', 'private-read', row[0] === 'DONE' && row[1] === expectedResult && row[2] === 'OK', `Aurora row ${jobId}: ${row[0]}, result matches, cache_write ${row[2]}`);
  const before = (await (await fetch(`${forward.url}/api/checks`)).json()).cache.lookups ?? {};
  await waitFor(async () => (await redis('EXISTS', cacheKey)) === '0', { timeoutMs: (input.cache.ttlSeconds + 60) * 1000, intervalMs: 2000, what: 'the cache TTL to expire' });
  rec.record('cache.ttl-expiry-miss', 'CACHE', 'private-read', true, `Redis key expired within ${input.cache.ttlSeconds + 60}s`);
  await app.reload();
  const reloaded = app.locator('#jobs tr.job').filter({ hasText: payload }).first();
  const sameCells = (await reloaded.locator('td').allInnerTexts()).map((t) => t.trim());
  await ah.moveTo(reloaded);
  await ah.pause(SHORT_READ);
  const after = (await (await fetch(`${forward.url}/api/checks`)).json()).cache.lookups ?? {};
  const refilled = (after['miss-refilled'] ?? 0) > (before['miss-refilled'] ?? 0);
  const keyBack = (await redis('EXISTS', cacheKey)) === '1';
  rec.record('cache.refill', 'CACHE', 'private-read', refilled && keyBack, `miss-refilled ${before['miss-refilled'] ?? 0} -> ${after['miss-refilled'] ?? 0}; key refilled in Redis`);
  rec.record('job.reload-same-result', 'JOB', 'ui', sameCells.join('|') === cells.join('|') && sameCells[3] === expectedResult, 'the reloaded row shows the same job and result (Aurora fallback)');
  const labelled = (await json(['-n', ns, 'get', 'pods', '-l', `run-id=${input.runId},managed-by=orchestrator-verification,app.kubernetes.io/component=technical-probe`, '-o', 'json'])).items.length;
  const exactRemaining = await Promise.all(pods.created.map(async (name) => {
    const r = await kubectl(['-n', ns, 'get', 'pod', name, '-o', 'name']);
    return r.code === 0 || !/notfound/i.test(r.stderr);
  }));
  rec.record('technical.pods-owned-and-removed', 'CACHE', 'private-read', pods.created.length > 0 && pods.removed.length === pods.created.length && pods.leftover.length === 0 && labelled === 0 && exactRemaining.every((exists) => !exists),
    `${pods.created.length} run-labelled technical client Pod(s) created (exact names recorded in the manifest) and ${pods.removed.length} verified absent; ${labelled} run-labelled and ${exactRemaining.filter(Boolean).length} exact-name Pod(s) remain or are indeterminate`);
  if (pods.leftover.length > 0) throw new Error('a technical client Pod is still present; cleanup needs attention');
  await s.signOut();

  // ===================== CLEANUP (independent supervisor) =====================
  phase('CLEANUP', 'Dọn dẹp: lưới an toàn độc lập hủy theo thứ tự ngược — Workload, Aurora/cache, namespace, EKS, VPC — rồi truy vấn từng ID vật lý và thẻ của lần chạy');
  forward.stop();
  v2ctl(input, 'supervisor-finish', '-private', input.privateDir);
  let waited = '';
  try {
    waited = v2ctl(input, 'supervisor-wait', '-private', input.privateDir, '-timeout', input.cleanupWaitDuration);
  } catch (error) {
    rec.record('cleanup.supervisor-clean', 'CLEANUP', 'private-read', false, safeError(error));
  }
  rec.record('cleanup.supervisor-clean', 'CLEANUP', 'private-read', /"state": "CLEAN"/.test(waited), 'supervisor reached CLEAN (each layer verified absent by exact IDs and the run tag)');
  let verify = '';
  try { verify = v2ctl(input, 'aws-verify', '-config', input.configFile, '-manifest', input.manifestFile, '-layer', 'aux'); } catch (error) { verify = `FAIL ${safeError(error)}`; }
  rec.record('cleanup.aws-absent', 'CLEANUP', 'private-read', !verify.includes('FAIL') && taggedArns().length === 0, 'no resource with the run tag remains; VPC, EKS, Aurora, cache and network objects are verified absent');
  await page.waitForTimeout(5000);
  writeFileSync(`${ctx.evidenceDir}/outcome.json`, JSON.stringify({ jobId, cacheKey, scenario }, null, 2));
}
