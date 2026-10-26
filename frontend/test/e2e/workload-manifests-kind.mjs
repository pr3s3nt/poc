// Real end-user acceptance of the open workload object set (ADR-024, WM-E01..WM-E06).
// Run only by backend/test/integration/workload-manifests-kind.sh, which owns the kind
// cluster (Envoy Gateway + Gateway gateway-system/public), the Compose project and the
// workload Vault. Every catalog/Definition/config/workload/Preview/Deploy/cleanup action
// is a browser action; API, kubectl and read-only database queries are independent
// checks only. The application is opened by its Environment host through the Gateway.
import { chromium, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { appendFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import YAML from 'yaml';
import { createHuman, createApplication } from './human.mjs';
import { makeKit, technical } from './full-app-kit.mjs';
import { startPortForward } from './terraform-v2-lib.mjs';
import {
  FULL_APP_TYPES, onboardInternalAccount, registerTypesUi, registerExistingCluster, registerK8sModuleDefinition,
  configureEnvironment, openFrontend, reconnectFrontend, waitRowsPass, submitJob, CHECK_ROWS,
} from './full-app-flows.mjs';
import { completionType, registerCompletionModule, bindCompletion, completionSnapshot } from './terraform-completion-flows.mjs';

const env = (name) => {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required`);
  return value;
};
const baseURL = env('WM_URL');
const evidenceDir = env('WM_EVIDENCE');
const privateDir = env('WM_PRIVATE');
const runId = env('WM_RUN_ID');
const completionRef = process.env.WM_COMPLETION_REF ?? '';
const source = { url: env('WM_GIT_URL'), path: env('WM_GIT_PATH'), ref: env('WM_GIT_REF'), commit: env('WM_COMMIT') };
const baseDomain = env('WM_BASE_DOMAIN');
const [gatewayNamespace, gatewayName] = env('WM_GATEWAY').split('/');
const imageTag = env('WM_IMAGE_TAG');
const v2ctlPath = env('WM_V2CTL');
const dbQuery = env('WM_DB_QUERY');
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(baseURL)) throw new Error('WM_URL must be the run-owned loopback Console');

const shots = `${evidenceDir}/screenshots`;
mkdirSync(shots, { recursive: true, mode: 0o700 });
const forwarders = [];
const freePort = () => new Promise((resolve) => { const server = createServer(); server.listen(0, '127.0.0.1', () => { const { port } = server.address(); server.close(() => resolve(port)); }); });
const input = {
  runId, privateDir, applicationName: `Workload Manifests ${runId}`,
  account: { name: `kind-${runId}`, kubeconfigFile: env('WM_INTERNAL_KUBECONFIG') },
  kube: { kubectl: 'kubectl', kubeconfig: env('WM_HOST_KUBECONFIG'), context: env('WM_CONTEXT') },
  v2ctl: v2ctlPath,
};
input.forwardPort = await freePort();
const gatewayPort = await freePort();
// The Environment host (and every other host of the run domain) resolves to a loopback
// port-forward of the Gateway's Envoy Service, so the browser sends the real Host header.
const browser = await chromium.launch({ headless: false, args: ['--window-size=1512,1000', `--host-resolver-rules=MAP *.${baseDomain} 127.0.0.1:${gatewayPort}`] });
const context = await browser.newContext({ viewport: { width: 1512, height: 960 }, locale: 'vi-VN' });
const page = await context.newPage();
page.setDefaultTimeout(60_000);
const browserErrors = [];
page.on('pageerror', (error) => browserErrors.push(error.message.slice(0, 200)));
page.on('console', (message) => { if (message.type() === 'error' && !/Failed to load resource/.test(message.text())) browserErrors.push(message.text().slice(0, 200)); });
const started = Date.now();
const results = [];
const rec = {
  record(id, phase, src, pass, detail = '') {
    results.push({ id, phase, source: src, pass: Boolean(pass), detail: String(detail).slice(0, 400) });
    writeFileSync(`${evidenceDir}/assertions.json`, JSON.stringify(results, null, 2));
    appendFileSync(`${evidenceDir}/assertions.txt`, `${pass ? 'PASS' : 'FAIL'} ${id} [${Math.round((Date.now() - started) / 1000)}s] ${String(detail).slice(0, 300)}\n`);
    console.log(`${pass ? 'PASS' : 'FAIL'} ${id}`);
    if (!pass) throw new Error(`assertion ${id} failed`);
  },
};
const check = (id, pass, detail = '') => rec.record(id, 'READY', 'private-read', pass, detail);
const ui = (id, pass, detail = '') => rec.record(id, 'FORM', 'ui', pass, detail);
const shot = (name) => page.screenshot({ path: `${shots}/${name}.png`, fullPage: true });
const h = createHuman(page, { fast: true });
const sql = (query) => execFileSync(dbQuery, [query], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const facts = { runId, source, baseDomain, assertions: results };
const saveFacts = () => writeFileSync(`${evidenceDir}/facts.json`, JSON.stringify(facts, null, 2));

let kit;
let tech;
let front;
let completionBefore;
const owned = async (kinds) => {
  const r = await tech.kubectl(['-n', input.namespace, 'get', kinds, '-l', 'app.kubernetes.io/managed-by=orchestrator', '-o', 'json']);
  if (r.code !== 0) throw new Error(`kubectl get ${kinds} failed`);
  return JSON.parse(r.stdout).items;
};
const one = async (kind, name) => {
  const r = await tech.kubectl(['-n', input.namespace, 'get', kind, name, '-o', 'json', '--ignore-not-found']);
  if (r.code !== 0) throw new Error(`kubectl get ${kind}/${name} failed`);
  return r.stdout.trim() ? JSON.parse(r.stdout) : null;
};
const inventoryOf = (workload) => {
  const raw = sql(`SELECT COALESCE(w.target_ref->'objects','[]'::jsonb)::text FROM workload_instances w JOIN environments e ON e.id=w.environment_id JOIN applications a ON a.id=e.application_id WHERE a.application_key='${input.applicationKey}' AND w.workload_id='${workload}' ORDER BY w.observed_at DESC LIMIT 1`);
  return JSON.parse(raw || '[]').map((o) => `${o.kind}/${o.name}`).sort();
};
const condition = (object, type) => (object?.status?.conditions ?? []).find((c) => c.type === type)?.status === 'True';
// Gateway API route parents carry their conditions directly (status.parents[].conditions).
const parentAccepted = (route) => (route?.status?.parents ?? []).some((p) => (p.conditions ?? []).some((c) => c.type === 'Accepted' && c.status === 'True'));

async function signInAs(username) {
  if (await page.getByRole('button', { name: 'Đăng xuất', exact: true }).isVisible().catch(() => false)) await kit.signOut();
  await kit.signIn(username);
}

async function previewDeploy(label, expectedWorkloads, expectations) {
  const { preview } = await kit.pendingPreview(expectedWorkloads);
  writeFileSync(`${evidenceDir}/${label}-preview.json`, JSON.stringify(sanitizePreview(preview), null, 2));
  await expectations(preview);
  await shot(`${label}-preview`);
  const deploy = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(`${kit.appPath}/deploy`), { timeout: 1_200_000 });
  const [result] = await Promise.all([
    deploy.then(async (response) => {
      const report = await response.json();
      if (report.status !== 'SUCCEEDED') throw new Error(`${label} deploy status ${report.status}`);
      return report;
    }),
    kit.deployAndWait(1_200_000),
  ]);
  await shot(`${label}-deployed`);
  return { preview, result };
}

// Shared evidence keeps only artifact paths and availability (no manifest text).
function sanitizePreview(preview) {
  return {
    changes: (preview.changes ?? []).map((c) => ({ workloadId: c.workloadId, action: c.action, revision: c.rendering?.bundle?.source?.revision })),
    manifests: (preview.review?.artifacts ?? []).filter((a) => a.kind === 'kubernetes-manifest').map((a) => ({ path: a.path, after: a.after?.availability })),
  };
}
const manifestPaths = (preview) => (preview.review?.artifacts ?? []).filter((a) => a.kind === 'kubernetes-manifest').map((a) => a.path);
const artifact = (preview, path) => (preview.review?.artifacts ?? []).find((a) => a.kind === 'kubernetes-manifest' && a.path === path);

async function businessFlow(label) {
  if (!front) front = await openFrontend(kit);
  else await reconnectFrontend(front, `the frontend after ${label}`);
  await waitRowsPass(front.app, CHECK_ROWS, 300_000);
  const job = await submitJob(front.ah, front.app, `${label}-${runId}`);
  await front.app.screenshot({ path: `${shots}/${label}-app.png`, fullPage: true });
  await page.bringToFront();
  ui(`${label}.business`, job.cells.join(' ').includes('DONE'), `job ${job.id} DONE through backend -> PostgreSQL -> worker -> Redis; checks ${CHECK_ROWS.join(', ')} PASS`);
  return job;
}

// Opens the Environment host in the browser through the Gateway (HTTPRoute orch-public).
async function openThroughGateway(label, host) {
  const app = await context.newPage();
  const url = `http://${host}/`;
  await expect.poll(async () => {
    const reply = await app.goto(url).catch(() => null);
    return reply?.status() ?? 0;
  }, { timeout: 180_000, intervals: [2000] }).toBe(200);
  await expect(app.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
  await waitRowsPass(app, CHECK_ROWS, 300_000);
  await app.screenshot({ path: `${shots}/${label}-domain.png`, fullPage: true });
  ui(`${label}.domain`, new URL(app.url()).host === host, `${url} served by the frontend through Gateway ${gatewayNamespace}/${gatewayName}; checks PASS`);
  await app.close();
  await page.bringToFront();
}

const annotate = (name, mutate) => {
  const file = `${privateDir}/scores/${name}.yaml`;
  const doc = YAML.parse(readFileSync(file, 'utf8'));
  mutate(doc);
  writeFileSync(file, YAML.stringify(doc), { mode: 0o600 });
  return doc;
};

try {
  // ---- WM-E01: PE onboarding through the Console --------------------------------------------
  await page.goto(`${baseURL}/ui/sign-in`);
  kit = makeKit({ page, context, h, input, baseURL, evidenceDir, rec, caption() {}, phase() {}, forwarders });
  await kit.signIn('platform-engineer');
  for (const [path, key] of [['driver-accounts', 'accounts'], ['resource-types', 'resourceTypes'], ['resource-definitions', 'resourceDefinitions']]) {
    check(`WM-E01.empty-${path}`, (await kit.get(`/${path}`))[key].length === 0, `${path} starts empty`);
  }
  await onboardInternalAccount(kit);
  await registerTypesUi(kit, completionRef ? [...FULL_APP_TYPES, completionType] : FULL_APP_TYPES);
  const storeKey = await kit.platform.registerSecretStore({ name: 'Run Workload Vault', backendAddress: env('WM_VAULT_ADDR'), workloadAddress: env('WM_VAULT_ADDR'), mount: 'kv', authMount: 'kubernetes' }, readFileSync(env('WM_WORKLOAD_VAULT_TOKEN_FILE'), 'utf8').trim());
  await registerExistingCluster(kit, { id: 'run-cluster', accountName: input.account.name, appKey: '' });
  for (const spec of [
    { id: 'run-namespace', type: 'k8s-namespace', module: 'k8s-namespace', presetName: 'name', clusterClass: 'internal' },
    { id: 'run-postgres', type: 'postgres', module: 'postgres-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
    { id: 'run-redis', type: 'redis', module: 'redis-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
  ]) {
    const stored = await registerK8sModuleDefinition(kit, spec);
    ui(`WM-E01.definition-${spec.id}`, stored?.driverType === 'terraform', `${spec.id} verified Terraform module ${spec.module}`);
  }
  if (completionRef) {
    await registerCompletionModule(kit, completionRef);
    ui('TC-module.git-pin', true, `real HTTPS Git module ${completionRef}`);
  }

  // ---- WM-E02: production bundle from the manifest repository --------------------------------
  await h.click(kit.nav('Mẫu dựng ứng dụng'));
  await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
  await h.type(page.getByLabel('ID mẫu', { exact: true }), 'git-workload');
  await h.type(page.getByLabel('Tên hiển thị', { exact: true }), 'Workload production');
  await h.type(page.getByLabel('Git HTTPS URL', { exact: true }), source.url);
  await h.type(page.getByLabel('Thư mục bundle', { exact: true }), source.path);
  await h.type(page.getByLabel('Tag hoặc commit', { exact: true }), source.ref, { replace: true });
  const inspected = page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/render-bundles/inspect', { timeout: 180_000 });
  await h.click(page.getByRole('button', { name: 'Kiểm tra bundle', exact: true }));
  const reply = await inspected;
  const body = await reply.json();
  if (!reply.ok()) throw new Error(`inspect HTTP ${reply.status()} ${body.code}`);
  await expect(page.getByRole('status').filter({ hasText: `Đã xác minh: production-workload · commit ${source.commit.slice(0, 12)}` })).toBeVisible();
  ui('WM-E02.inspect', body.renderBundle.source.revision === source.commit && body.renderBundle.source.url === source.url, `${source.url} ${source.path} @ ${source.commit}`);
  await h.type(page.getByLabel('Team', { exact: true }), 'payments');
  await h.click(page.getByRole('button', { name: 'Dùng gợi ý namespace', exact: true }));
  await h.click(page.getByRole('button', { name: 'Dùng gợi ý cluster', exact: true }));
  await shot('E02-template');
  const created = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/resource-definitions');
  await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }));
  ui('WM-E02.saved', (await created).status() === 201, 'production template registered with the verified pin');

  // ---- WM-E03: Developer application with production annotations -----------------------------
  await signInAs('developer');
  const subdomain = `wm${runId}`.slice(0, 20);
  input.applicationKey = await createApplication(h, input.applicationName, subdomain);
  input.namespace = `app-${input.applicationKey}-staging`;
  kit = makeKit({ page, context, h, input, baseURL, evidenceDir, rec, caption() {}, phase() {}, forwarders });
  tech = technical(input);
  facts.application = { key: input.applicationKey, namespace: input.namespace };
  saveFacts();
  execFileSync(v2ctlPath, ['v04-scores', '-out', `${privateDir}/scores`, '-application', input.applicationKey, '-run-id', runId,
    '-frontend', `acceptance-frontend:${imageTag}`, '-backend', `acceptance-backend:${imageTag}`, '-worker', `acceptance-worker:${imageTag}`], { stdio: 'pipe' });
  const scores = JSON.parse(readFileSync(`${privateDir}/scores/scores.json`, 'utf8'));
  const backendAnnotations = {
    'bundle/autoscale-min': '1', 'bundle/autoscale-max': '3', 'bundle/pdb-min-available': '1',
    'bundle/migrate': '["/app", "migrate"]',
    'bundle/cron.refresh': '{"schedule": "*/30 * * * *", "command": ["/app", "migrate"]}',
    'bundle/file.app.yaml': `run: ${runId}\n`,
  };
  annotate('backend', (doc) => { doc.metadata.annotations = backendAnnotations; delete doc.replicas; });
  annotate('worker', (doc) => { doc.metadata.annotations = { 'bundle/volume.scratch': '{"path": "/var/lib/scratch", "size": "64Mi"}' }; });
  const frontendScore = annotate('frontend', (doc) => {
    const port = Object.keys(doc.service?.ports ?? {})[0];
    doc.service.publicRoutes = [{ path: '/', port }];
  });
  if (completionRef) {
    for (const name of ['backend', 'worker', 'frontend']) annotate(name, (doc) => bindCompletion(doc, name !== 'frontend'));
  }
  input.servicePort = Object.values(frontendScore.service.ports)[0].port;
  const secret = randomBytes(24).toString('hex');
  appendFileSync(`${privateDir}/app-secrets.txt`, `ACCEPTANCE_SECRET=${secret}\n`, { mode: 0o600 });
  const hash = createHash('sha256').update(secret).digest('hex');
  const config = scores.config.filter((c) => c.kind === 'variable').map((c) => c.name === 'ACCEPTANCE_SECRET_SHA256' ? { ...c, value: hash } : c);
  await configureEnvironment(kit, { config, secretName: 'ACCEPTANCE_SECRET', secretValue: secret, storeKey });
  for (const name of ['backend', 'worker', 'frontend']) await kit.addWorkloadFromScore(`${privateDir}/scores/${name}.yaml`, name);
  ui('WM-E03.workloads', true, 'three Score workloads imported; backend/worker carry bundle/* annotations, frontend a public route');

  // ---- WM-E04: Preview shows every bundle object, creates nothing ----------------------------
  check('WM-E04.no-resources-before', (await tech.namespaceUid()) === '', 'namespace absent before Deploy');
  const first = await previewDeploy('E04', 3, async (preview) => {
    const paths = manifestPaths(preview);
    const wanted = ['HorizontalPodAutoscaler/backend', 'PodDisruptionBudget/backend', 'ConfigMap/backend-files', 'Job/backend-migrate-preview', 'CronJob/backend-refresh', 'PersistentVolumeClaim/worker-scratch'];
    ui('WM-E04.objects', wanted.every((p) => paths.includes(p)), `Preview lists ${wanted.join(', ')}`);
    ui('WM-E04.redacted', !artifact(preview, 'ConfigMap/backend-files').after.text.includes(runId), 'ConfigMap data stays redacted in the review');
    check('WM-E04.preview-no-mutation', (await tech.namespaceUid()) === '', 'Preview created no namespace');
  });
  facts.deploy1 = first.result.id ?? null;

  // ---- WM-E05: Deploy: every object live, Job before rollout, domain through the Gateway -------
  const id1 = (await one('deployment', 'backend')).metadata.labels['orchestrator.io/deployment-id'];
  const job1 = await one('job', `backend-migrate-${id1.slice(0, 8)}`);
  const backendRs = (await tech.json(['-n', input.namespace, 'get', 'replicaset', '-o', 'json'])).items.filter((r) => r.metadata.ownerReferences?.[0]?.name === 'backend').map((r) => Date.parse(r.metadata.creationTimestamp));
  check('WM-E05.migration-first', condition(job1, 'Complete') && backendRs.length > 0 && Math.min(...backendRs) >= Date.parse(job1.status.completionTime) - 1000,
    `Job ${job1?.metadata.name} completed ${job1?.status.completionTime} before the backend ReplicaSet was created`);
  const hpa = await one('hpa', 'backend');
  const pdb = await one('pdb', 'backend');
  const files = await one('configmap', 'backend-files');
  const cron = await one('cronjob', 'backend-refresh');
  const pvc = await one('pvc', 'worker-scratch');
  check('WM-E05.objects', hpa?.spec.maxReplicas === 3 && pdb?.spec.minAvailable === 1 && files?.data['app.yaml'] === `run: ${runId}\n` && cron?.spec.schedule === '*/30 * * * *' && pvc?.status.phase === 'Bound',
    'HPA max 3, PDB minAvailable 1, ConfigMap app.yaml, CronJob */30, PVC Bound');
  const backend = await one('deployment', 'backend');
  const mounts = backend.spec.template.spec.containers[0].volumeMounts ?? [];
  const worker = await one('deployment', 'worker');
  check('WM-E05.mounts', mounts.some((m) => m.mountPath === '/etc/app') && worker.spec.strategy.type === 'Recreate' && (worker.spec.template.spec.containers[0].volumeMounts ?? []).some((m) => m.mountPath === '/var/lib/scratch'),
    'backend mounts the config files, worker mounts its PVC with Recreate strategy');
  const labelled = [hpa, pdb, files, cron, pvc, job1].every((o) => o.metadata.labels['orchestrator.io/workload'] && o.metadata.labels['orchestrator.io/application'] === input.applicationKey);
  check('WM-E05.ownership', labelled, 'every bundle object carries the platform ownership labels');
  const inventory1 = inventoryOf('backend');
  facts.inventory1 = inventory1;
  check('WM-E05.inventory', ['ConfigMap/backend-files', 'CronJob/backend-refresh', 'Deployment/backend', 'HorizontalPodAutoscaler/backend', `Job/backend-migrate-${id1.slice(0, 8)}`, 'PodDisruptionBudget/backend', 'Service/backend'].every((k) => inventory1.includes(k)),
    `persisted inventory ${inventory1.join(', ')}`);
  // The Gateway controller sets the route status asynchronously after the apply.
  let route = null;
  await expect.poll(async () => {
    route = await one('httproute', 'orch-public');
    return parentAccepted(route);
  }, { timeout: 120_000, intervals: [2000] }).toBe(true).catch(() => {});
  const host = route?.spec.hostnames?.[0] ?? '';
  facts.host = host;
  facts.routeStatus = (route?.status?.parents ?? []).map((p) => (p.conditions ?? []).map((c) => `${c.type}=${c.status}:${c.reason}`));
  saveFacts();
  check('WM-E05.httproute', host === `staging.${subdomain}.${baseDomain}` && route.spec.parentRefs[0].name === gatewayName && route.spec.parentRefs[0].namespace === gatewayNamespace
    && parentAccepted(route), `HTTPRoute orch-public ${host} accepted by ${gatewayNamespace}/${gatewayName}`);
  const envoy = (await tech.kubectl(['-n', 'envoy-gateway-system', 'get', 'service', '-l', `gateway.envoyproxy.io/owning-gateway-name=${gatewayName},gateway.envoyproxy.io/owning-gateway-namespace=${gatewayNamespace}`, '-o', 'jsonpath={.items[0].metadata.name}'])).stdout.trim();
  const gatewayForward = startPortForward({ ...input.kube, namespace: 'envoy-gateway-system', service: envoy, remotePort: 80, localPort: gatewayPort });
  forwarders.push(gatewayForward);
  await openThroughGateway('E05', host);
  await businessFlow('E05');
  if (completionRef) {
    completionBefore = await completionSnapshot({ kit, tech, sql, check, label: 'initial', revision: 'v1' });
    facts.completionInitial = completionBefore;
    await page.reload();
    await expect(page.getByRole('heading', { name: input.applicationName, exact: true })).toBeVisible();
    ui('TC-reload', (await kit.get(`${kit.appPath}/resources`)).resources.filter((r) => r.type === 'app-config' && r.status === 'READY').length === 2,
      'resource results remain visible after a normal Console reload');
  }

  // ---- WM-E06: update drops the CronJob; a new migration replaces the old one ---------------
  delete backendAnnotations['bundle/cron.refresh'];
  backendAnnotations['bundle/autoscale-max'] = '4';
  annotate('backend', (doc) => { doc.metadata.annotations = backendAnnotations; });
  if (completionRef) {
    annotate('frontend', (doc) => bindCompletion(doc, false, 'v2'));
  }
  await kit.editWorkloadFromScore(`${privateDir}/scores/backend.yaml`, 'backend');
  if (completionRef) await kit.editWorkloadFromScore(`${privateDir}/scores/frontend.yaml`, 'frontend');
  await previewDeploy('E06', completionRef ? 2 : 1, async (preview) => {
    ui('WM-E06.update', preview.changes.length === (completionRef ? 2 : 1) && preview.changes.some((c) => c.workloadId === 'backend') && preview.changes.every((c) => c.action === 'UPDATE'), 'valid backend UPDATE; completion mode also updates the frontend private resource');
    ui('WM-E06.no-cron', !manifestPaths(preview).includes('CronJob/backend-refresh'), 'the CronJob is no longer rendered');
  });
  const id2 = (await one('deployment', 'backend')).metadata.labels['orchestrator.io/deployment-id'];
  const job2 = await one('job', `backend-migrate-${id2.slice(0, 8)}`);
  check('WM-E06.pruned', (await one('cronjob', 'backend-refresh')) === null && (await one('job', `backend-migrate-${id1.slice(0, 8)}`)) === null && condition(job2, 'Complete') && (await one('hpa', 'backend'))?.spec.maxReplicas === 4,
    `CronJob and old Job ${id1.slice(0, 8)} deleted after rollout, new Job ${id2.slice(0, 8)} complete, HPA max 4`);
  const inventory2 = inventoryOf('backend');
  facts.inventory2 = inventory2;
  check('WM-E06.inventory', !inventory2.includes('CronJob/backend-refresh') && inventory2.includes(`Job/backend-migrate-${id2.slice(0, 8)}`) && !inventory2.includes(`Job/backend-migrate-${id1.slice(0, 8)}`), `inventory ${inventory2.join(', ')}`);
  await openThroughGateway('E06', host);
  await businessFlow('E06');
  if (completionRef) facts.completionUpdate = await completionSnapshot({ kit, tech, sql, check, label: 'update', revision: 'v2', previous: completionBefore });
  saveFacts();

  // ---- WM-E07: product removal deletes every inventory object --------------------------------
  for (const name of ['frontend', 'worker', 'backend']) {
    page.once('dialog', (dialog) => dialog.accept());
    await h.click(page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) }).getByRole('button', { name: 'Xóa', exact: true }));
    await expect(page.locator('.workload-table')).toContainText('Chờ xóa');
  }
  await previewDeploy('E07-remove', 3, async (preview) => {
    ui('WM-E07.remove-preview', preview.changes.every((c) => c.action === 'REMOVE'), 'three workload removals');
  });
  const left = (await owned('deployment,statefulset,service,configmap,job,cronjob,hpa,pdb,pvc')).filter((o) => o.metadata.labels?.['orchestrator.io/workload']);
  check('WM-E07.objects-absent', left.length === 0, left.map((o) => `${o.kind}/${o.metadata.name}`).join(', ') || 'no workload-owned object left');
  check('WM-E07.route-absent', (await one('httproute', 'orch-public')) === null, 'HTTPRoute removed with the last public path');
  await signInAs('platform-engineer');
  const scope = kit.appPath;
  for (let round = 0; round < 6; round += 1) {
    const roots = (await kit.get(`${scope}/resources`)).resources.filter((r) => r.status === 'UNREFERENCED')
      .sort((a, b) => Number(a.type === 'k8s-namespace') - Number(b.type === 'k8s-namespace'));
    if (roots.length === 0) break;
    let deleted = null;
    for (const root of roots) {
      await page.goto(`${baseURL}/ui/applications/${input.applicationKey}`);
      const escaped = root.descriptor.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
      await h.click(page.getByRole('region', { name: 'Tài nguyên của môi trường' }).getByRole('button', { name: new RegExp(escaped) }).first());
      const action = page.getByRole('button', { name: 'Xóa tài nguyên', exact: true });
      await expect(action).toBeVisible();
      if (!(await action.isEnabled())) continue;
      await h.click(action);
      const dialog = page.getByRole('dialog', { name: 'Xóa tài nguyên', exact: true });
      const confirmation = await dialog.getByTestId('root-name').innerText();
      await h.type(dialog.getByLabel('Nhập lại tên tài nguyên'), confirmation);
      await h.click(dialog.getByRole('button', { name: 'Xác nhận xóa vĩnh viễn' }));
      const outcome = await expect.poll(async () => {
        const detail = (await kit.get(`${scope}/resources/${root.id}`)).resource ?? {};
        if (detail.status === 'DESTROYED') return 'DESTROYED';
        if (['FAILED', 'SUCCEEDED', 'INTERRUPTED'].includes(detail.cleanup?.status)) return `CLEANUP_${detail.cleanup.status}`;
        return 'RUNNING';
      }, { timeout: 900_000, intervals: [5000] }).not.toBe('RUNNING').then(async () => (await kit.get(`${scope}/resources/${root.id}`)).resource);
      if (outcome.status !== 'DESTROYED') {
        const objects = await tech.kubectl(['-n', input.namespace, 'get', 'all,secret,serviceaccount,configmap,pvc,vaultstaticsecret,vaultauth,vaultconnection', '-o', 'custom-columns=KIND:.kind,NAME:.metadata.name', '--no-headers']);
        facts.cleanupFailure = { descriptor: root.descriptor, status: outcome.status, cleanup: outcome.cleanup, remainingObjects: objects.stdout.split('\n').filter(Boolean) };
        saveFacts();
        await shot(`E07-cleanup-${round}-failed`);
        ui('WM-E07.cleanup', false, `${root.descriptor} cleanup ${outcome.cleanup?.status}`);
      }
      deleted = root;
      break;
    }
    if (!deleted) throw new Error('no unreferenced resource could be deleted');
  }
  const remaining = (await kit.get(`${scope}/resources`)).resources.filter((r) => r.status !== 'DESTROYED' && r.type !== 'k8s-cluster');
  ui('WM-E07.resources-destroyed', remaining.length === 0, 'every run resource DESTROYED through the product');
  check('WM-E07.namespace-absent', (await tech.namespaceUid()) === '', 'namespace absence proven by a successful lookup');
  check('WM-E07.no-browser-errors', browserErrors.length === 0, browserErrors.join(' | ').slice(0, 300));
  kit.assertNoViolations();
  facts.outcome = 'PASS';
} catch (error) {
  facts.outcome = 'FAIL';
  facts.error = String(error?.message ?? error).split('\n')[0].slice(0, 300);
  writeFileSync(`${privateDir}/failure.txt`, String(error?.stack ?? error), { mode: 0o600 });
  await shot('failure').catch(() => {});
  console.error(`scenario failed: ${facts.error}`);
  process.exitCode = 1;
} finally {
  for (const forward of forwarders) forward.stop?.();
  facts.browserErrors = browserErrors;
  facts.durationSeconds = Math.round((Date.now() - started) / 1000);
  saveFacts();
  await browser.close();
}
