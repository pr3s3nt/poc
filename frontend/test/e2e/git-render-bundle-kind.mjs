// Real end-user acceptance of Git-owned manifest rendering (ADR-021, RB-E01..RB-E10).
// Run only by backend/test/integration/git-render-bundle-kind.sh, which owns the kind
// cluster, the Compose project, the workload Vault and the fixture Git ref. Every
// catalog/Definition/config/workload/Preview/Deploy/cleanup action is a browser action;
// API, kubectl, Docker and read-only database queries are independent checks only.
import { chromium, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { appendFileSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import YAML from 'yaml';
import { createHuman, createApplication } from './human.mjs';
import { makeKit, technical } from './full-app-kit.mjs';
import {
  FULL_APP_TYPES, onboardInternalAccount, registerTypesUi, registerExistingCluster, registerK8sModuleDefinition,
  configureEnvironment, openFrontend, reconnectFrontend, waitRowsPass, submitJob, CHECK_ROWS,
} from './full-app-flows.mjs';

const env = (name) => {
  const value = process.env[name];
  if (!value) throw new Error(`${name} is required`);
  return value;
};
const baseURL = env('GRB_URL');
const evidenceDir = env('GRB_EVIDENCE');
const privateDir = env('GRB_PRIVATE');
const runId = env('GRB_RUN_ID');
const source = { url: env('GRB_GIT_URL'), path: env('GRB_GIT_PATH'), refA: env('GRB_REF_A'), refB: env('GRB_REF_B'), commitA: env('GRB_COMMIT_A'), commitB: env('GRB_COMMIT_B') };
const backendContainer = env('GRB_BACKEND_CONTAINER');
const composeProject = env('GRB_COMPOSE_PROJECT');
const imageTag = env('GRB_IMAGE_TAG');
const v2ctlPath = env('GRB_V2CTL');
const dbQuery = env('GRB_DB_QUERY');
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(baseURL)) throw new Error('GRB_URL must be the run-owned loopback Console');

const shots = `${evidenceDir}/screenshots`;
mkdirSync(shots, { recursive: true, mode: 0o700 });
const forwarders = [];
const input = {
  runId, privateDir, applicationName: `Git Render ${runId}`,
  account: { name: `kind-${runId}`, kubeconfigFile: env('GRB_INTERNAL_KUBECONFIG') },
  kube: { kubectl: 'kubectl', kubeconfig: env('GRB_HOST_KUBECONFIG'), context: env('GRB_CONTEXT') },
  v2ctl: v2ctlPath,
};
const browser = await chromium.launch({ headless: false, args: ['--window-size=1512,1000'] });
const context = await browser.newContext({ viewport: { width: 1512, height: 960 } });
const page = await context.newPage();
page.setDefaultTimeout(60_000);
const browserErrors = [];
page.on('pageerror', (error) => browserErrors.push(error.message.slice(0, 200)));
page.on('console', (message) => { if (message.type() === 'error' && !/Failed to load resource/.test(message.text())) browserErrors.push(message.text().slice(0, 200)); });
const inspectCalls = [];
page.on('request', (request) => { if (new URL(request.url()).pathname === '/api/v1/render-bundles/inspect') inspectCalls.push(Date.now()); });
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
const docker = (...args) => execFileSync('docker', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const sql = (query) => execFileSync(dbQuery, [query], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
const backendIdentity = () => {
  const [image, startedAt, labels] = [docker('inspect', '-f', '{{.Image}}', backendContainer), docker('inspect', '-f', '{{.State.StartedAt}}', backendContainer), docker('inspect', '-f', '{{index .Config.Labels "com.docker.compose.project"}}', backendContainer)];
  if (labels !== composeProject) throw new Error('backend container is not owned by this run');
  return { image, startedAt };
};
const facts = { runId, source, assertions: results };
const saveFacts = () => writeFileSync(`${evidenceDir}/facts.json`, JSON.stringify(facts, null, 2));

let kit;
let tech;
let front;
const deployments = () => tech.json(['-n', input.namespace, 'get', 'deployment', '-o', 'json']);
const podTemplate = (deployment) => deployment.spec.template;
async function workloadPolicy() {
  const list = (await deployments()).items;
  const out = {};
  for (const d of list) {
    out[d.metadata.name] = {
      annotation: podTemplate(d).metadata.annotations?.['example.com/render-policy'] ?? null,
      normalizedBy: podTemplate(d).metadata.annotations?.['example.com/normalized-by'] ?? null,
      team: d.metadata.labels?.['example.com/team'] ?? null,
      securityContext: podTemplate(d).spec.securityContext ?? null,
      revisionHistoryLimit: d.spec.revisionHistoryLimit,
      replicas: d.spec.replicas,
      resources: Object.fromEntries(podTemplate(d).spec.containers.map((c) => [c.name, c.resources])),
      images: Object.fromEntries(podTemplate(d).spec.containers.map((c) => [c.name, c.image])),
      ports: Object.fromEntries(podTemplate(d).spec.containers.map((c) => [c.name, (c.ports ?? []).map((p) => p.containerPort)])),
    };
  }
  return out;
}
async function identities() {
  const ns = await tech.namespaceUid();
  const sets = (await tech.statefulsets()).items.map((s) => [s.metadata.name, s.metadata.uid]).sort();
  const pvcs = (await tech.json(['-n', input.namespace, 'get', 'pvc', '-o', 'json'])).items.map((p) => [p.metadata.name, p.metadata.uid, p.spec.volumeName]).sort();
  const instances = sql(`SELECT i.descriptor||'|'||i.id||'|'||i.status FROM terraform_resource_instances i JOIN applications a ON a.id=i.application_id WHERE a.application_key='${input.applicationKey}' ORDER BY 1`);
  const states = sql(`SELECT i.descriptor||'|'||s.lineage FROM terraform_states s JOIN terraform_resource_instances i ON i.id=s.resource_instance_id JOIN applications a ON a.id=i.application_id WHERE a.application_key='${input.applicationKey}' ORDER BY 1`);
  return { namespaceUid: ns, statefulSets: sets, pvcs, instances: instances.split('\n'), stateLineages: states.split('\n') };
}
const jobRows = (deployment) => tech.psql(deployment, 'SELECT id||\':\'||payload||\':\'||status FROM jobs ORDER BY id');
async function backendDeployment() {
  return (await deployments()).items.find((d) => d.metadata.name === 'backend');
}

async function signInAs(username) {
  if (await page.getByRole('button', { name: 'Đăng xuất', exact: true }).isVisible().catch(() => false)) await kit.signOut();
  await kit.signIn(username);
}

async function openTemplates() {
  await h.click(kit.nav('Mẫu dựng ứng dụng'));
  await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
}

async function inspect(ref, commit) {
  await h.type(page.getByLabel('Tag hoặc commit', { exact: true }), ref, { replace: true });
  const response = kit.page.waitForResponse((r) => new URL(r.url()).pathname === '/api/v1/render-bundles/inspect', { timeout: 180_000 });
  await h.click(page.getByRole('button', { name: 'Kiểm tra bundle', exact: true }));
  const reply = await response;
  const body = await reply.json();
  if (!reply.ok()) throw new Error(`inspect HTTP ${reply.status()} ${body.code}`);
  const bundle = body.renderBundle;
  await expect(page.getByRole('status').filter({ hasText: `Đã xác minh: standard-workload · commit ${commit.slice(0, 12)}` })).toBeVisible();
  const raw = JSON.stringify(body);
  ui(`RB-E${ref === source.refA ? '02' : '06'}.inspect`, reply.status() === 200 && bundle.source.revision === commit && bundle.source.path === source.path && bundle.source.url === source.url
    && /^[0-9a-f]{64}$/.test(bundle.source.contentSha256) && !/cache|binary|token|password/i.test(raw),
  `ref ${ref} -> ${commit}; content ${bundle.source.contentSha256}; no cache path or credential in the reply`);
  return bundle;
}

async function saveTemplate(expectedVersion) {
  const response = page.waitForResponse((r) => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1/resource-definitions/git-workload');
  await h.click(page.getByRole('button', { name: 'Lưu thay đổi', exact: true }));
  const reply = await response;
  if (!reply.ok()) throw new Error(`template save HTTP ${reply.status()}`);
  await expect(page.getByRole('status').filter({ hasText: `Đã lưu mẫu dựng ứng dụng git-workload (phiên bản ${expectedVersion})` })).toBeVisible();
}

async function storedTemplate() {
  return (await kit.get('/resource-definitions')).resourceDefinitions.find((d) => d.key === 'git-workload');
}

async function openApp() {
  await h.click(kit.nav('Ứng dụng'));
  await kit.openApplication();
}

async function previewDeploy(label, expectedWorkloads, expectations) {
  const { preview } = await kit.pendingPreview(expectedWorkloads);
  writeFileSync(`${evidenceDir}/${label}-preview.json`, JSON.stringify(sanitizePreview(preview), null, 2));
  await expectations(preview);
  await shot(`${label}-preview`);
  const deploy = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(`${kit.appPath}/deploy`), { timeout: 1_200_000 });
  await kit.deployAndWait(1_200_000);
  const result = await (await deploy).json();
  if (result.status !== 'SUCCEEDED') throw new Error(`${label} deploy status ${result.status}`);
  await shot(`${label}-deployed`);
  return { preview, result };
}

// Keep only the allowlisted review projection (no literal env values) for shared evidence.
function sanitizePreview(preview) {
  return {
    changes: (preview.changes ?? []).map((c) => ({ workloadId: c.workloadId, action: c.action, bundle: c.rendering?.bundle?.version, revision: c.rendering?.bundle?.source?.revision, inputsHash: c.rendering?.inputsHash })),
    nodes: (preview.review?.nodes ?? []).map((n) => ({ id: n.id, kind: n.kind, intent: n.intent, renderSource: n.renderSource })),
    manifests: (preview.review?.artifacts ?? []).filter((a) => a.kind === 'kubernetes-manifest' && a.path.startsWith('Deployment/')).map((a) => ({ path: a.path, before: a.before?.text ?? a.before?.availability, after: a.after?.text ?? a.after?.availability, unknown: a.after?.unknownPaths, provenance: a.after?.provenance })),
  };
}
const manifestOf = (preview, workload) => (preview.review?.artifacts ?? []).find((a) => a.kind === 'kubernetes-manifest' && a.path === `Deployment/${workload}`);

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

try {
  // ---- RB-E01: PE onboarding through the Console --------------------------------------------
  await page.goto(`${baseURL}/ui/sign-in`);
  kit = makeKit({ page, context, h, input, baseURL, evidenceDir, rec, caption() {}, phase() {}, forwarders });
  await kit.signIn('platform-engineer');
  for (const [path, key] of [['driver-accounts', 'accounts'], ['resource-types', 'resourceTypes'], ['resource-definitions', 'resourceDefinitions']]) {
    check(`RB-E01.empty-${path}`, (await kit.get(`/${path}`))[key].length === 0, `${path} starts empty`);
  }
  const listed = (await kit.get('/render-bundles')).renderBundles;
  check('RB-E01.no-git-bundles', listed.every((b) => !b.source), `${listed.length} installed bundle(s), no Git record before inspection`);
  facts.backendBefore = backendIdentity();
  saveFacts();
  await onboardInternalAccount(kit);
  await registerTypesUi(kit, FULL_APP_TYPES);
  const storeKey = await kit.platform.registerSecretStore({ name: 'Run Workload Vault', backendAddress: env('GRB_VAULT_ADDR'), workloadAddress: env('GRB_VAULT_ADDR'), mount: 'kv', authMount: 'kubernetes' }, readFileSync(env('GRB_WORKLOAD_VAULT_TOKEN_FILE'), 'utf8').trim());
  await registerExistingCluster(kit, { id: 'run-cluster', accountName: input.account.name, appKey: '' });
  for (const spec of [
    { id: 'run-namespace', type: 'k8s-namespace', module: 'k8s-namespace', presetName: 'name', clusterClass: 'internal' },
    { id: 'run-postgres', type: 'postgres', module: 'postgres-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
    { id: 'run-redis', type: 'redis', module: 'redis-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
  ]) {
    const stored = await registerK8sModuleDefinition(kit, spec);
    ui(`RB-E01.definition-${spec.id}`, stored?.driverType === 'terraform' && stored.driverInputs.values.provider_bindings.kubernetes.resource_ref === 'k8s-cluster.internal#environments.@app.@env', `${spec.id} verified Terraform module ${spec.module} bound to the run cluster`);
  }
  await shot('E01-catalog');

  // ---- RB-E02: Git template A through the new source UI --------------------------------------
  await openTemplates();
  await expect(page.getByRole('button', { name: 'Thêm từ Git', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await h.type(page.getByLabel('ID mẫu', { exact: true }), 'git-workload');
  await h.type(page.getByLabel('Tên hiển thị', { exact: true }), 'Workload từ Git');
  await h.type(page.getByLabel('Git HTTPS URL', { exact: true }), source.url);
  await h.type(page.getByLabel('Thư mục bundle', { exact: true }), source.path);
  const bundleA = await inspect(source.refA, source.commitA);
  facts.bundleA = { id: bundleA.id, revision: bundleA.source.revision, contentSha256: bundleA.source.contentSha256 };
  await h.type(page.getByLabel('Team', { exact: true }), 'payments');
  await expect(page.getByText('Mặc định: 1.', { exact: false })).toBeVisible();
  // Form -> JSON -> form keeps the typed inputs exactly.
  await h.click(page.getByRole('button', { name: 'Dùng gợi ý namespace', exact: true }));
  await h.click(page.getByRole('button', { name: 'Dùng gợi ý cluster', exact: true }));
  await h.click(page.getByRole('button', { name: 'JSON nâng cao', exact: true }));
  const json = JSON.parse(await page.getByLabel('JSON nâng cao của mẫu dựng ứng dụng').inputValue());
  ui('RB-E02.json-roundtrip', json.variables.render_bundle === bundleA.id && JSON.stringify(json.variables.render_inputs) === '{"team":"payments"}', 'advanced JSON holds the verified UUID and the typed inputs only');
  await h.click(page.getByRole('button', { name: 'Biểu mẫu', exact: true }));
  await expect(page.getByLabel('Team', { exact: true })).toHaveValue('payments');
  await shot('E02-template-a');
  const created = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/resource-definitions');
  await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }));
  const createdReply = await created;
  const createdRaw = await createdReply.text();
  await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký mẫu dựng ứng dụng git-workload.' })).toBeVisible();
  let template = await storedTemplate();
  ui('RB-E02.saved', createdReply.status() === 201 && template.driverInputs.values.variables.render_bundle === bundleA.id && template.driverInputs.values.variables.render_inputs.team === 'payments'
    && !/token|password|kubeconfig/i.test(createdRaw), `template v${template.version} pins ${bundleA.id}`);
  const inspectsBeforeReopen = inspectCalls.length;
  await h.click(page.getByRole('button', { name: 'Sửa mẫu git-workload', exact: true }));
  await expect(page.getByLabel('Bundle dựng ứng dụng')).toHaveValue(bundleA.id);
  await expect(page.getByLabel('Team', { exact: true })).toHaveValue('payments');
  ui('RB-E02.reopen', inspectCalls.length === inspectsBeforeReopen, 'reopened the saved pin and inputs without a new inspection');
  await shot('E02-reopened');
  await h.click(page.getByRole('button', { name: 'Hủy sửa', exact: true }));

  // ---- RB-E03: Developer Application, Secret Store, config and three workloads ----------------
  await signInAs('developer');
  input.applicationKey = await createApplication(h, input.applicationName, `gr${runId}`.slice(0, 20));
  input.namespace = `app-${input.applicationKey}-staging`;
  kit = makeKit({ page, context, h, input, baseURL, evidenceDir, rec, caption() {}, phase() {}, forwarders });
  tech = technical(input);
  facts.application = { key: input.applicationKey, namespace: input.namespace };
  saveFacts();
  execFileSync(v2ctlPath, ['v04-scores', '-out', `${privateDir}/scores`, '-application', input.applicationKey, '-run-id', runId,
    '-frontend', `acceptance-frontend:${imageTag}`, '-backend', `acceptance-backend:${imageTag}`, '-worker', `acceptance-worker:${imageTag}`], { stdio: 'pipe' });
  const scores = JSON.parse(readFileSync(`${privateDir}/scores/scores.json`, 'utf8'));
  // Precedence fixtures: backend explicit CPU request, worker memory limit only, frontend nothing.
  const adjust = (name, resources) => {
    const doc = YAML.parse(readFileSync(`${privateDir}/scores/${name}.yaml`, 'utf8'));
    const container = Object.keys(doc.containers)[0];
    if (resources) doc.containers[container].resources = resources;
    else delete doc.containers[container].resources;
    writeFileSync(`${privateDir}/scores/${name}.yaml`, YAML.stringify(doc), { mode: 0o600 });
    return container;
  };
  const containers = { backend: adjust('backend', { requests: { cpu: '50m' } }), worker: adjust('worker', { limits: { memory: '96Mi' } }), frontend: adjust('frontend', null) };
  const secret = randomBytes(24).toString('hex');
  appendFileSync(`${privateDir}/app-secrets.txt`, `ACCEPTANCE_SECRET=${secret}\n`, { mode: 0o600 });
  const hash = createHash('sha256').update(secret).digest('hex');
  const config = scores.config.filter((c) => c.kind === 'variable').map((c) => c.name === 'ACCEPTANCE_SECRET_SHA256' ? { ...c, value: hash } : c);
  await configureEnvironment(kit, { config, secretName: 'ACCEPTANCE_SECRET', secretValue: secret, storeKey });
  for (const name of ['backend', 'worker', 'frontend']) await kit.addWorkloadFromScore(`${privateDir}/scores/${name}.yaml`, name);
  ui('RB-E03.workloads', true, 'Developer created the Application, chose the run Secret Store, saved variables, one masked secret and three Score workloads');

  // ---- RB-E04 + RB-E05: Preview A then Deploy A --------------------------------------------
  const nsAbsent = (await tech.namespaceUid()) === '';
  check('RB-E04.no-resources-before', nsAbsent, 'namespace absent before Deploy');
  const a = await previewDeploy('E04-A', 3, async (preview) => {
    const workloadNodes = preview.review.nodes.filter((n) => n.kind === 'workload');
    ui('RB-E04.provenance', workloadNodes.length === 3 && workloadNodes.every((n) => n.renderSource?.revision === source.commitA && n.renderSource.path === source.path), `all workloads pinned to ${source.commitA.slice(0, 12)}`);
    await expect(page.getByLabel('Xem trước triển khai')).toContainText(`Git ${source.path} @ ${source.commitA.slice(0, 12)}`);
    const frontendManifest = manifestOf(preview, 'frontend');
    ui('RB-E04.manifest', frontendManifest.after.text.includes('example.com/render-policy: a') && frontendManifest.after.text.includes('cpu: 20m') && frontendManifest.after.text.includes('example.com/team: payments'), 'Preview shows bundle A annotation, CPU fallback and the team label');
    const unknown = (preview.review.artifacts ?? []).filter((x) => x.kind === 'kubernetes-manifest').flatMap((x) => x.after?.unknownPaths ?? []);
    ui('RB-E04.unknown', unknown.length > 0 && !JSON.stringify(sanitizePreview(preview)).includes('orchestrator-preview-unknown'), `${unknown.length} unresolved paths stay unknown, no stand-in shown as a value`);
    check('RB-E04.preview-no-mutation', (await tech.namespaceUid()) === '', 'Preview created no namespace');
  });
  facts.deployA = a.result.id ?? a.result.deploymentId ?? null;
  const policyA = await workloadPolicy();
  facts.policyA = policyA;
  check('RB-E05.policy-a', policyA.frontend.annotation === 'a' && policyA.frontend.resources[containers.frontend].requests.cpu === '20m' && policyA.frontend.resources[containers.frontend].requests.memory === '32Mi'
    && policyA.backend.resources[containers.backend].requests.cpu === '50m' && policyA.worker.resources[containers.worker].requests.memory === '96Mi' && policyA.worker.resources[containers.worker].limits.memory === '96Mi'
    && policyA.frontend.team === 'payments' && policyA.frontend.securityContext === null, 'live Deployments: A fallback 20m/32Mi, explicit request and same-field limit kept, A annotation, no securityContext');
  const backend = await backendDeployment();
  const pwd = tech.envOf(backend, 'PGPASSWORD');
  check('RB-E05.secret-ref', Boolean(pwd?.valueFrom?.secretKeyRef) && pwd.value === undefined, 'database password is a Secret reference produced by the output-only provisioner');
  const jobA = await businessFlow('E05-A');
  facts.identityA = await identities();
  facts.jobs = [jobA.id];
  saveFacts();
  check('RB-E05.infrastructure', facts.identityA.statefulSets.length >= 2 && facts.identityA.pvcs.length >= 1 && facts.identityA.instances.length >= 3, `instances ${facts.identityA.instances.length}, StatefulSets ${facts.identityA.statefulSets.length}, PVCs ${facts.identityA.pvcs.length}`);

  // ---- RB-E06: PE switches the same directory to ref B ---------------------------------------
  await signInAs('platform-engineer');
  await openTemplates();
  await h.click(page.getByRole('button', { name: 'Sửa mẫu git-workload', exact: true }));
  await h.click(page.getByRole('button', { name: 'Thêm từ Git', exact: true }));
  await expect(page.getByLabel('Git HTTPS URL', { exact: true })).toHaveValue(source.url);
  await expect(page.getByLabel('Thư mục bundle', { exact: true })).toHaveValue(source.path);
  const bundleB = await inspect(source.refB, source.commitB);
  facts.bundleB = { id: bundleB.id, revision: bundleB.source.revision, contentSha256: bundleB.source.contentSha256 };
  await expect(page.getByLabel('Team', { exact: true })).toHaveValue('payments');
  template = await storedTemplate();
  await saveTemplate(template.version + 1);
  await shot('E06-template-b');
  const afterB = backendIdentity();
  check('RB-E06.no-restart', afterB.image === facts.backendBefore.image && afterB.startedAt === facts.backendBefore.startedAt, `backend image ${afterB.image.slice(0, 19)} started ${afterB.startedAt} unchanged between A and B`);
  check('RB-E06.same-directory', bundleB.source.path === bundleA.source.path && bundleB.source.url === bundleA.source.url && bundleB.source.contentSha256 !== bundleA.source.contentSha256, 'B is the same directory at a new commit with different content');

  // ---- RB-E07: Developer Preview/Deploy B with no Score draft --------------------------------
  await signInAs('developer');
  await openApp();
  await previewDeploy('E07-B', 3, async (preview) => {
    ui('RB-E07.update', preview.changes.length === 3 && preview.changes.every((c) => c.action === 'UPDATE' && c.rendering.bundle.source.revision === source.commitB), 'three workload UPDATEs from the bundle change alone');
    const m = manifestOf(preview, 'frontend');
    ui('RB-E07.diff', m.before.text.includes('example.com/render-policy: a') && m.after.text.includes('example.com/render-policy: b') && m.before.text.includes('cpu: 20m') && m.after.text.includes('cpu: 30m') && m.after.text.includes('RuntimeDefault'), 'diff A->B: annotation, CPU fallback and securityContext');
  });
  const policyB = await workloadPolicy();
  facts.policyB = policyB;
  check('RB-E07.policy-b', policyB.frontend.annotation === 'b' && policyB.frontend.normalizedBy === 'b' && policyB.frontend.revisionHistoryLimit === 5 && policyB.frontend.resources[containers.frontend].requests.cpu === '30m'
    && policyB.frontend.securityContext?.seccompProfile?.type === 'RuntimeDefault', 'live Deployments follow B normalization, defaults and patches');
  check('RB-E07.explicit-kept', policyB.backend.resources[containers.backend].requests.cpu === '50m' && policyB.worker.resources[containers.worker].requests.memory === '96Mi'
    && JSON.stringify(policyB.backend.images) === JSON.stringify(policyA.backend.images) && JSON.stringify(policyB.frontend.ports) === JSON.stringify(policyA.frontend.ports), 'explicit request/limit, images and ports unchanged');
  const jobB = await businessFlow('E07-B');
  facts.jobs.push(jobB.id);
  const identityB = await identities();
  check('RB-E07.identity', JSON.stringify(identityB) === JSON.stringify(facts.identityA), 'namespace, StatefulSets, PVCs, Terraform instances and state lineages unchanged');

  // ---- RB-E08: refresh, restart only the owned backend, unchanged Preview is a no-op ----------
  await page.reload();
  await expect(page.getByRole('heading', { name: input.applicationName })).toBeVisible();
  const imageBeforeRestart = backendIdentity().image;
  const inspectsBeforeRestart = inspectCalls.length;
  docker('restart', backendContainer);
  await expect.poll(() => docker('inspect', '-f', '{{.State.Health.Status}}', backendContainer), { timeout: 180_000, intervals: [2000] }).toBe('healthy');
  const restarted = backendIdentity();
  check('RB-E08.restart', restarted.image === imageBeforeRestart && restarted.startedAt !== facts.backendBefore.startedAt, `same image ${restarted.image.slice(0, 19)}, new process start ${restarted.startedAt}`);
  await page.reload();
  if (await page.getByLabel('Tên đăng nhập', { exact: true }).isVisible().catch(() => false)) await kit.signIn('developer');
  await openApp();
  const idle = await responseOf(() => h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true })), `${kit.appPath}/preview`);
  ui('RB-E08.noop', (idle.changes ?? []).length === 0, 'unchanged intent after restart is a no-op Preview');
  await shot('E08-noop');
  const jobsAfterRestart = await jobRows(await backendDeployment());
  check('RB-E08.jobs', facts.jobs.every((id) => jobsAfterRestart.includes(`${id}:`)), 'persisted jobs remain');
  await signInAs('platform-engineer');
  await openTemplates();
  await h.click(page.getByRole('button', { name: 'Sửa mẫu git-workload', exact: true }));
  await expect(page.getByLabel('Bundle dựng ứng dụng')).toHaveValue(bundleB.id);
  await expect(page.getByLabel('Team', { exact: true })).toHaveValue('payments');
  const listedAfter = (await kit.get('/render-bundles')).renderBundles.filter((b) => b.source);
  ui('RB-E08.cache', inspectCalls.length === inspectsBeforeRestart && listedAfter.length === 2 && listedAfter.every((b) => b.status === 'AVAILABLE'), 'pins A and B available from the durable cache after restart without inspection');

  // ---- RB-E09: explicit re-selection of verified A --------------------------------------------
  await h.choose(page.getByLabel('Bundle dựng ứng dụng'), `standard-workload @ ${source.commitA.slice(0, 12)}`);
  await expect(page.getByLabel('Team', { exact: true })).toHaveValue('payments');
  template = await storedTemplate();
  await saveTemplate(template.version + 1);
  await shot('E09-template-a-again');
  await signInAs('developer');
  await openApp();
  await previewDeploy('E09-A', 3, async (preview) => {
    ui('RB-E09.update', preview.changes.length === 3 && preview.changes.every((c) => c.rendering.bundle.source.revision === source.commitA), 'B->A explicit re-selection is three workload UPDATEs');
  });
  const policyA2 = await workloadPolicy();
  check('RB-E09.policy-a', policyA2.frontend.annotation === 'a' && policyA2.frontend.normalizedBy === null && policyA2.frontend.resources[containers.frontend].requests.cpu === '20m' && policyA2.frontend.securityContext === null, 'live Deployments back to policy A');
  const jobA2 = await businessFlow('E09-A');
  facts.jobs.push(jobA2.id);
  check('RB-E09.identity', JSON.stringify(await identities()) === JSON.stringify(facts.identityA), 'infrastructure identities unchanged after B->A');
  saveFacts();

  // ---- RB-E10: product cleanup through the UI -------------------------------------------------
  for (const name of ['frontend', 'worker', 'backend']) {
    page.once('dialog', (dialog) => dialog.accept());
    await h.click(page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) }).getByRole('button', { name: 'Xóa', exact: true }));
    await expect(page.locator('.workload-table')).toContainText('Chờ xóa');
  }
  await previewDeploy('E10-remove', 3, async (preview) => {
    ui('RB-E10.remove-preview', preview.changes.every((c) => c.action === 'REMOVE'), 'three workload removals');
  });
  check('RB-E10.workloads-absent', (await deployments()).items.length === 0, 'no Deployment left in the run namespace');
  const scope = kit.appPath;
  for (let round = 0; round < 4; round += 1) {
    const inventory = (await kit.get(`${scope}/resources`)).resources;
    const root = inventory.find((r) => r.status === 'UNREFERENCED');
    if (!root) break;
    await page.goto(`${baseURL}/ui/applications/${input.applicationKey}`);
    const escaped = root.descriptor.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    await h.click(page.getByRole('region', { name: 'Tài nguyên của môi trường' }).getByRole('button', { name: new RegExp(escaped) }).first());
    await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
    const dialog = page.getByRole('dialog', { name: 'Xóa tài nguyên', exact: true });
    const confirmation = await dialog.getByTestId('root-name').innerText();
    for (const box of await dialog.getByRole('checkbox').all()) if (!(await box.isChecked()) && await box.isEnabled()) await box.check();
    await h.type(dialog.getByLabel('Nhập lại tên tài nguyên'), confirmation);
    await h.click(dialog.getByRole('button', { name: 'Xác nhận xóa vĩnh viễn' }));
    await expect.poll(async () => (await kit.get(`${scope}/resources`)).resources.find((r) => r.id === root.id)?.status, { timeout: 900_000, intervals: [5000] }).toBe('DESTROYED');
    await shot(`E10-cleanup-${round}`);
  }
  const remaining = (await kit.get(`${scope}/resources`)).resources.filter((r) => r.status !== 'DESTROYED' && r.resourceType !== 'k8s-cluster');
  ui('RB-E10.resources-destroyed', remaining.length === 0, 'every run resource DESTROYED through the product');
  const nsAfter = await tech.namespaceUid();
  check('RB-E10.namespace-absent', nsAfter === '', 'namespace absence proven by a successful lookup');
  check('RB-E10.no-browser-errors', browserErrors.length === 0, browserErrors.join(' | ').slice(0, 300));
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

async function responseOf(action, suffix) {
  const response = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(suffix), { timeout: 180_000 });
  await action();
  const reply = await response;
  const body = await reply.json();
  if (!reply.ok()) throw new Error(`${suffix} HTTP ${reply.status()} ${body.code}`);
  return body;
}
