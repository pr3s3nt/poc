// Focused update of an existing acceptance backend: catalog, PVC mount, write,
// UID-guarded Pod recreation and retained value/hash. No app/job/schedule setup.
import { expect, chromium } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdirSync, mkdtempSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { readConsoleConfig, signInContext, signOutContext } from '../../lib/console.mjs';

const config = readConsoleConfig();
if (!config.headless) throw Error('Set ORCH_HEADLESS=1 for this focused scenario');
const appId = process.env.ORCH_APPLICATION_ID;
if (!/^[0-9a-f-]{36}$/.test(appId ?? '')) throw Error('ORCH_APPLICATION_ID required');
const namespace = `app-${appId}-staging`;
const kubeContext = process.env.ORCH_KUBE_CONTEXT;
if (!kubeContext) throw Error('ORCH_KUBE_CONTEXT required');
const appURL = new URL(process.env.ORCH_ACCEPTANCE_URL);
if (!['http:', 'https:'].includes(appURL.protocol) || appURL.username || appURL.password || appURL.search || appURL.hash || !['', '/'].includes(appURL.pathname)) throw Error('ORCH_ACCEPTANCE_URL must be the acceptance app origin');
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
if (!process.env.ORCH_EVIDENCE_DIR) throw Error('ORCH_EVIDENCE_DIR outside Git required');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const repo = realpathSync(resolve(dirname(fileURLToPath(import.meta.url)), '../../..'));
if (!relative(repo, realpathSync(evidence)).startsWith('../')) throw Error('Evidence must stay outside Git');
const temp = mkdtempSync(join(tmpdir(), 'orch-pvc-'));
const browser = await chromium.launch({ headless: true });
const assertions = [];
let session, app;
const check = name => { assertions.push(name); console.log(`PASS ${name}`); };
const kubectl = args => execFileSync('kubectl', ['--context', kubeContext, '-n', namespace, ...args], { encoding: 'utf8', timeout: 180_000 }).trim();
const object = (kind, name) => JSON.parse(kubectl(['get', kind, name, '-o', 'json']));
const endpoint = `/applications/${appId}/environments/staging`;
let status = 'FAIL';
try {
  session = await signInContext(browser, config, { role: 'DEVELOPER', recordVideo: { dir: join(evidence, 'raw'), size: { width: 1440, height: 1000 } } });
  const { page } = session;
  const get = async path => {
    const reply = await page.request.get(`${config.baseURL}/api/v1${path}`);
    if (!reply.ok()) throw Error(`GET ${path}: HTTP ${reply.status()}`);
    return reply.json();
  };
  const initial = (await get(`${endpoint}/workloads`)).workloads;
  const backend = initial.find(w => w.id === 'backend');
  if (!backend?.score?.containers?.main) throw Error('Existing acceptance backend/main required');
  if (initial.some(w => w.id !== 'backend' && w.state)) throw Error('Unrelated workload changes pending; refusing deployment');
  const others = initial.filter(w => w.id !== 'backend');
  const identities = Object.fromEntries(['frontend', 'worker'].map(name => [name, object('deployment', name).metadata.uid]));
  const { templates } = await get(`${endpoint}/workload-templates`);
  const template = templates.find(t => t.id === 'backend' && t.version === '1.0.0');
  if (!template) throw Error('Register the standard workload catalog first');
  await page.goto(`${config.baseURL}/ui/applications/${appId}`);
  await page.getByRole('button', { name: '+ Thêm Workload', exact: true }).click();
  const picker = page.getByRole('dialog', { name: 'Chọn loại workload', exact: true });
  await expect(picker.locator('.application-card')).toHaveCount(templates.length);
  await expect(picker.getByRole('button').filter({ hasText: template.labels.vi })).toBeVisible();
  await page.screenshot({ path: join(evidence, 'catalog.png') });
  check(`Catalog shows ${templates.length} real templates`);
  await page.keyboard.press('Escape');

  const score = structuredClone(backend.score);
  const alreadyMounted = score.workloadExecution?.mounts?.some(m => m.path === '/data' && m.container === 'main');
  if (!alreadyMounted) {
    if (score.workloadExecution) throw Error('Existing custom execution requires explicit review');
    score.workloadExecution = { version: 1, kind: template.kind, template: { id: template.id, version: template.version, sha256: template.sha256 }, inputs: template.defaults };
    score.containers.main.variables.ACCEPTANCE_STORAGE_DIR = '/data';
    const file = join(temp, 'backend.json');
    writeFileSync(file, JSON.stringify(score), { mode: 0o600 });
    await page.goto(`${config.baseURL}/ui/applications/${appId}/environments/staging/workloads/backend`);
    await page.getByRole('button', { name: 'Nhập Score', exact: true }).click();
    await page.getByLabel('File Score', { exact: true }).setInputFiles(file);
    await expect(page.getByText('Đã phân tích Workload: backend', { exact: false })).toBeVisible();
    await page.getByRole('button', { name: 'Nhập bằng form', exact: true }).click();
    await page.getByRole('button', { name: 'Thêm volume mount', exact: true }).click();
    const storage = page.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ', exact: true });
    await storage.getByLabel('Tham số storage', { exact: true }).fill('1Gi');
    await storage.getByRole('button', { name: 'Thêm và chọn', exact: true }).click();
    await expect(page.getByLabel('Mount path', { exact: true })).toHaveValue('/data');
    await page.screenshot({ path: join(evidence, 'mount.png'), fullPage: true });
    const [saved] = await Promise.all([
      page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === `/api/v1${endpoint}/workloads/backend`),
      page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click(),
    ]);
    if (!saved.ok()) throw Error(`Save backend HTTP ${saved.status()}`);
    check('Backend form saves managed PVC at /data');
  }
  const draft = (await get(`${endpoint}/workloads`)).workloads;
  expect(draft.filter(w => w.id !== 'backend')).toEqual(others);
  const pending = draft.find(w => w.id === 'backend');
  expect(pending.score.containers.main.variables.ACCEPTANCE_STORAGE_DIR).toBe('/data');
  if (pending.state) {
    await page.goto(`${config.baseURL}/ui/applications/${appId}`);
    const [previewReply] = await Promise.all([
      page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === `/api/v1${endpoint}/preview`, { timeout: 120_000 }),
      page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click(),
    ]);
    const preview = await previewReply.json();
    if (!previewReply.ok()) throw Error(`Preview HTTP ${previewReply.status()}: ${preview.code ?? 'see Console'}`);
    expect(preview.changes.map(c => c.workloadId)).toEqual(['backend']);
    check('Preview affects backend only');
    const [deployReply] = await Promise.all([
      page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === `/api/v1${endpoint}/deploy`, { timeout: 600_000 }),
      page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click(),
    ]);
    const report = await deployReply.json();
    if (!deployReply.ok() || report.status !== 'SUCCEEDED') throw Error(`Backend deploy ${report.status ?? deployReply.status()}`);
    check('PVC and backend deployment succeeded');
  }
  app = await session.context.newPage();
  await app.goto(appURL.origin);
  await expect(app.getByLabel('Test value', { exact: true })).toBeVisible();
  const value = `pvc-proof-${Date.now()}`;
  const hash = createHash('sha256').update(value).digest('hex');
  await app.getByLabel('Test value', { exact: true }).fill(value);
  await app.getByRole('button', { name: 'Write to volume', exact: true }).click();
  await expect(app.locator('#storage-value')).toHaveText(value);
  await expect(app.locator('#storage-hash')).toHaveText(hash);
  check('Storage form writes and reads actual value/hash');

  const deployment = object('deployment', 'backend');
  const mount = deployment.spec.template.spec.containers.find(c => c.name === 'main').volumeMounts.find(m => m.mountPath === '/data');
  const claim = deployment.spec.template.spec.volumes.find(v => v.name === mount.name).persistentVolumeClaim.claimName;
  const pvc = object('pvc', claim);
  expect(pvc.status.phase).toBe('Bound');
  expect(pvc.metadata.namespace).toBe(namespace);
  expect(pvc.metadata.labels['app.kubernetes.io/managed-by']).toBe('orchestrator');
  const pods = JSON.parse(kubectl(['get', 'pods', '-l', 'app.kubernetes.io/name=backend', '-o', 'json'])).items.filter(p => !p.metadata.deletionTimestamp);
  expect(pods).toHaveLength(1);
  const pod = pods[0];
  const owner = pod.metadata.ownerReferences.find(o => o.controller && o.kind === 'ReplicaSet');
  if (!owner) throw Error('Backend Pod has no controlling ReplicaSet');
  const rs = object('replicaset', owner.name);
  expect(rs.metadata.uid).toBe(owner.uid);
  expect(rs.metadata.ownerReferences.some(o => o.controller && o.uid === deployment.metadata.uid)).toBe(true);
  expect(pod.metadata.labels['orchestrator.io/application']).toBe(appId);
  writeFileSync(join(evidence, 'identities.json'), JSON.stringify({ appId, namespace, kubeContext, deploymentUID: deployment.metadata.uid, pod: pod.metadata.name, podUID: pod.metadata.uid, pvc: claim, pvcUID: pvc.metadata.uid, value, hash }, null, 2));
  await expect(app.locator('#storage-process')).toHaveText(pod.metadata.name);
  check('PVC Bound and Pod controller chain verified');
  execFileSync('kubectl', ['--context', kubeContext, 'delete', '--raw', `/api/v1/namespaces/${namespace}/pods/${pod.metadata.name}`, '-f', '-'], { encoding: 'utf8', timeout: 30_000, input: JSON.stringify({ apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: pod.metadata.uid } }) });
  kubectl(['rollout', 'status', 'deployment/backend', '--timeout=180s']);
  await expect.poll(async () => {
    await app.reload();
    const process = app.locator('#storage-process');
    return await process.count() > 0 && (await process.textContent()) !== pod.metadata.name;
  }, { timeout: 180_000, intervals: [1000] }).toBe(true);
  await expect(app.locator('#storage-value')).toHaveText(value);
  await expect(app.locator('#storage-hash')).toHaveText(hash);
  expect(object('pvc', claim).metadata.uid).toBe(pvc.metadata.uid);
  check('Value/hash and PVC UID retained after backend Pod recreation');
  for (const [name, uid] of Object.entries(identities)) expect(object('deployment', name).metadata.uid).toBe(uid);
  expect((await get(`${endpoint}/workloads`)).workloads.filter(w => w.id !== 'backend')).toEqual(others);
  check('Unrelated workload specs and deployment identities unchanged');
  await app.screenshot({ path: join(evidence, 'storage-retained.png'), fullPage: true });
  status = 'PASS';
} catch (error) {
  console.error(`FAIL ${String(error.message).split('\n')[0]}`);
  process.exitCode = 1;
} finally {
  writeFileSync(join(evidence, 'result.json'), JSON.stringify({ status, appId, assertions }, null, 2));
  rmSync(temp, { recursive: true, force: true });
  try {
    if (session) {
      await session.page.screenshot({ path: join(evidence, 'console.png'), fullPage: true }).catch(() => {});
      await signOutContext(session.context, config);
      const video = session.page.video(), appVideo = app?.video();
      await session.context.close();
      await video.saveAs(join(evidence, 'console.webm'));
      await appVideo?.saveAs(join(evidence, 'application.webm'));
    }
  } finally { await browser.close(); }
}
