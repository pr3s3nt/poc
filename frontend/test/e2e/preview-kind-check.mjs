// Observe the retained sample, optionally perform its valid image update through
// the Console. Run again after restarting only preview-kind-backend-1.
import { chromium, expect } from '@playwright/test';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createHuman, createApplication } from './human.mjs';
import { waitRowsPass, submitJob } from './full-app-flows.mjs';
const base = process.env.PREVIEW_URL;
const sample = process.env.PREVIEW_SAMPLE_URL;
const evidence = process.env.PREVIEW_CHECK_EVIDENCE;
const owned = JSON.parse(readFileSync(process.env.PREVIEW_APPLICATION, 'utf8'));
if (!base || !sample || !evidence || !/^preview-\d+$/.test(owned.runId) || owned.applicationName !== `Kind Preview ${owned.runId}`) throw new Error('Explicit owned sample and URLs required');
mkdirSync(evidence, { mode: 0o700 });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({ viewport: { width: 1512, height: 1100 } });
const page = await context.newPage();
page.setDefaultTimeout(45_000);
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
const assertions = [];
const check = (name, pass) => { if (!pass) throw new Error(name); assertions.push(name); console.log(`PASS ${name}`); };
const prefix = `/api/v1/applications/${owned.applicationKey}/environments/staging`;
const get = async (path) => {
  const r = await context.request.get(`${base}${path}`);
  if (!r.ok()) throw new Error(`GET HTTP ${r.status()}`);
  return r.json();
};
try {
  await page.goto(`${base}/ui/sign-in`);
  await page.getByLabel('Tên đăng nhập').fill('platform-engineer');
  await page.getByLabel('Mật khẩu').fill('test-password');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
  check('owned Application identity', (await get(`/api/v1/applications/${owned.applicationKey}`)).application.name === owned.applicationName);
  const inventory = await get(`${prefix}/resources`);
  writeFileSync(`${evidence}/inventory.json`, JSON.stringify(inventory));
  check('inventory available from real state backend', Array.isArray(inventory.resources) && inventory.resources.length === 3);
  const deployments = (await get(`${prefix}/deployments`)).deployments;
  check('three real successful deployments', deployments.filter((d) => d.status === 'SUCCEEDED').length >= 3);
  const details = await Promise.all(deployments.map((d) => get(`${prefix}/deployments/${d.id}`)));
  check('applied manifests persisted for all three workloads', ['frontend', 'backend', 'worker'].every((name) => details.some((d) => d.workloads?.some((w) => w.workloadId === name && w.status === 'READY' && w.manifestReview?.version === 1))));
  await page.goto(`${base}/ui/applications/${owned.applicationKey}`);
  await expect(page.locator('.workload-table .table-row')).toHaveCount(3);
  await page.screenshot({ path: `${evidence}/console.png`, fullPage: true });
  const app = await context.newPage();
  // Service port-forward reconnects after a rollout replaces its selected Pod.
  await expect.poll(async () => {
    try { return (await context.request.get(`${sample}/healthz`, { timeout: 5000 })).ok(); }
    catch { return false; }
  }, { timeout: 60_000, intervals: [1000] }).toBe(true);
  await app.goto(sample);
  await waitRowsPass(app);
  check('real backend environment secret database cache checks', true);
  const h = createHuman(app, { fast: true });
  const payload = `preview-kind-job-${Date.now()}`;
  const job = await submitJob(h, app, payload);
  check('worker completed browser-submitted job', job.cells.some((v) => v.includes('DONE')));
  writeFileSync(`${evidence}/job.json`, JSON.stringify({ id: job.id, payload, cells: job.cells }));
  await app.screenshot({ path: `${evidence}/application.png`, fullPage: true });
  if (process.env.PREVIEW_PRIOR_JOB) {
    const prior = JSON.parse(readFileSync(process.env.PREVIEW_PRIOR_JOB, 'utf8'));
    const jobs = await (await context.request.get(`${sample}/api/jobs`)).json();
    check('previous job survives update and backend restart', jobs.jobs.some((j) => j.id === prior.id && j.payload === prior.payload && j.status === 'DONE'));
  }
  if (process.env.PREVIEW_UPDATE === '1') {
    await page.locator('.workload-table .table-row').filter({ has: page.locator('.workload-name', { hasText: /frontend$/ }) }).getByRole('button', { name: 'Sửa', exact: true }).click();
    const current = (await get(`${prefix}/workloads`)).workloads.find((w) => w.id === 'frontend').score;
    const before = 'acceptance-frontend:v46-20261018-512d0d3-amd64';
    expect([before, `${before}-b`]).toContain(current.containers.main.image);
    const updated = structuredClone(current);
    updated.containers.main.image = `${before}-b`;
    await page.getByRole('button', { name: 'Nhập Score', exact: true }).click();
    await page.getByLabel('File Score', { exact: true }).setInputFiles({ name: 'frontend-update.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(updated)) });
    await expect(page.locator('.score-preview')).toContainText(`${before}-b`);
    await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click();
    const previewResponse = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/preview'), { timeout: 120_000 });
    await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
    const preview = await (await previewResponse).json();
    const manifest = preview.review.artifacts.find((a) => a.nodeId === 'workload.default#modules.frontend' && a.path === 'Deployment/frontend');
    const original = deployments.find((d) => d.workloadId === 'frontend');
    check('update diff uses exact applied baseline', ['AVAILABLE', 'PARTIAL'].includes(manifest.before.availability) && manifest.before.provenance.baselineDeploymentId === original.id && manifest.before.text.includes(before) && manifest.after.text.includes(`${before}-b`));
    await page.getByLabel('Xem trước triển khai').locator('[data-node-id="workload.default#modules.frontend"]').click();
    await page.getByLabel('Xem trước triển khai').getByRole('tab', { name: 'Code', exact: true }).click();
    await expect(page.locator('.review-diff-old')).not.toHaveCount(0);
    await expect(page.locator('.review-diff-new')).not.toHaveCount(0);
    await page.screenshot({ path: `${evidence}/update-code.png`, fullPage: true });
    const deployResponse = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/deploy'), { timeout: 900_000 });
    await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
    const deployed = await deployResponse;
    check('real image update succeeds', deployed.ok() && (await deployed.json()).status === 'SUCCEEDED');
  }
  if (process.env.PREVIEW_CLEANUP_SMOKE === '1') {
    await page.goto(`${base}/ui/applications`);
    const consoleHuman = createHuman(page, { fast: true });
    const smoke = `cleanup-${Date.now().toString().slice(-9)}`;
    const id = await createApplication(consoleHuman, smoke, smoke);
    const ns = `app-${id}-staging`;
    const path = `/api/v1/applications/${id}/environments/staging`;
    writeFileSync(`${evidence}/cleanup-identity.json`, JSON.stringify({ app: id, namespace: ns }));
    await page.getByRole('button', { name: '+ Thêm Workload', exact: true }).click();
    await page.getByRole('button', { name: 'Nhập Score', exact: true }).click();
    const score = { apiVersion: 'score.dev/v1b1', metadata: { name: 'web' }, containers: { main: { image: 'acceptance-frontend:v46-20261018-512d0d3-amd64' } }, service: { ports: { http: { port: 8080, targetPort: 8080 } } } };
    await page.getByLabel('File Score').setInputFiles({ name: 'web.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(score)) });
    await expect(page.locator('.score-preview')).toContainText('acceptance-frontend');
    await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click();
    const deploy = async () => {
      const previewResponse = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith(`${path}/preview`), { timeout: 120_000 });
      await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
      expect((await previewResponse).ok()).toBe(true);
      const response = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith(`${path}/deploy`), { timeout: 900_000 });
      await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
      const result = await response;
      expect(result.ok()).toBe(true);
      expect((await result.json()).status).toBe('SUCCEEDED');
    };
    await deploy();
    const kube = (...args) => execFileSync('kubectl', ['--context', 'kind-k8s-4f', ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
    const original = JSON.parse(kube('get', 'namespace', ns, '-o', 'json'));
    writeFileSync(`${evidence}/cleanup-identity.json`, JSON.stringify({ app: id, namespace: ns, uid: original.metadata.uid, labels: original.metadata.labels }));
    check('cleanup smoke namespace created by real deployment', original.metadata.name === ns && Boolean(original.metadata.uid));
    page.once('dialog', (dialog) => dialog.accept());
    await page.locator('.workload-table .table-row').getByRole('button', { name: 'Xóa', exact: true }).click();
    await expect(page.locator('.workload-table')).toContainText('Chờ xóa');
    await deploy();
    const inventory = await get(`${path}/resources`);
    expect(inventory.resources).toHaveLength(1);
    const resource = inventory.resources[0];
    expect(resource.status).toBe('UNREFERENCED');
    await page.getByRole('region', { name: 'Tài nguyên của môi trường' }).getByRole('button', { name: new RegExp('k8s-namespace') }).click();
    await page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: 'Xóa tài nguyên', exact: true });
    const confirmation = await dialog.getByTestId('root-name').innerText();
    await dialog.getByLabel('Nhập lại tên tài nguyên').fill(confirmation);
    await dialog.getByRole('button', { name: 'Xác nhận xóa vĩnh viễn' }).click();
    await expect.poll(async () => (await get(`${path}/resources`)).resources.find((r) => r.id === resource.id)?.status, { timeout: 300_000, intervals: [2000] }).toBe('DESTROYED');
    check('owned namespace absent after product cleanup', kube('get', 'namespace', ns, '--ignore-not-found', '-o', 'name').trim() === '');
  }
  check('no browser page exception', errors.length === 0);
} catch (error) {
  await page.screenshot({ path: `${evidence}/failure.png`, fullPage: true }).catch(() => {});
  console.error(String(error.message).slice(0, 2000));
  process.exitCode = 1;
} finally {
  writeFileSync(`${evidence}/assertions.json`, JSON.stringify(assertions, null, 2));
  await browser.close();
}
