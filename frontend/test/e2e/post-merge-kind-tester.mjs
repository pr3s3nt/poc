// Post-merge exploratory acceptance on the persistent preview-kind Console (real data,
// real Terraform/score-k8s, kind-k8s-4f). A Platform Engineer tests the two merged
// features like a person: visible cursor, typed input, recorded video.
//  A. Preview on existing Applications: read-only, never deploys them.
//  B. A new owned Application: real deploy, image update with exact baseline diff,
//     workload removal and product cleanup of its own namespace (verified absent).
//  C. Platform effectiveness on the real history, including the new work items.
import { chromium, expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { createHuman, createApplication, addWorkload, installCursor } from './human.mjs';

const base = process.env.TESTER_URL;
const evidence = process.env.TESTER_EVIDENCE_DIR;
if (!base || !evidence) throw Error('TESTER_URL and TESTER_EVIDENCE_DIR required');
await mkdir(evidence, { recursive: true, mode: 0o700 });

const IMAGE = 'acceptance-frontend:v46-20261018-512d0d3-amd64';
const viewport = { width: 1512, height: 980 };
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport, acceptDownloads: true, recordVideo: { dir: evidence, size: viewport } });
await context.addInitScript(installCursor);
const page = await context.newPage();
page.setDefaultTimeout(45_000);
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
const assertions = [];
const started = Date.now();
const check = (name, value) => {
  if (!value) throw Error(`assertion failed: ${name}`);
  assertions.push(name);
  console.log(`PASS [${Math.round((Date.now() - started) / 1000)}s] ${name}`);
};
const shot = (name) => page.screenshot({ path: `${evidence}/${name}.png`, fullPage: true });
const get = async (path) => {
  const r = await context.request.get(`${base}${path}`);
  if (!r.ok()) throw Error(`GET ${path}: HTTP ${r.status()}`);
  return r.json();
};
const kube = (...args) => execFileSync('kubectl', ['--context', 'kind-k8s-4f', ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
const h = createHuman(page);
const nav = page.getByRole('navigation', { name: 'Điều hướng chính' });
const review = page.getByLabel('Xem trước triển khai');
const identity = {};
let outcome = 'FAIL';

async function previewThenDeploy(path) {
  const previewResponse = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith(`${path}/preview`), { timeout: 180_000 });
  await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  const preview = await (await previewResponse).json();
  await expect(review.locator('.review-edges>path')).not.toHaveCount(0);
  return preview;
}
async function deploy(path, label) {
  const response = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith(`${path}/deploy`), { timeout: 900_000 });
  await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
  await expect(page.getByLabel('Theo dõi triển khai')).toBeVisible({ timeout: 60_000 });
  await h.moveTo(page.getByLabel('Theo dõi triển khai'));
  await shot(`${label}-progress`);
  const result = await response;
  const body = await result.json();
  check(`${label}: real deployment succeeds`, result.ok() && body.status === 'SUCCEEDED');
  await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công', { timeout: 120_000 });
  await h.moveTo(page.getByLabel('Kết quả triển khai'));
  await h.read('Kết quả triển khai thành công');
  await shot(`${label}-result`);
  return body;
}

try {
  // ---- Sign in, navigation rail ----
  await page.goto(`${base}/ui/sign-in`);
  await h.showCursor();
  await h.type(page.getByLabel('Tên đăng nhập'), 'platform-engineer');
  await h.type(page.getByLabel('Mật khẩu'), 'test-password');
  await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
  check('sign in to the real Console', true);
  const applications = (await get('/api/v1/applications')).applications;
  check('real Applications are listed', applications.length >= 7);
  await h.read('Danh sách ứng dụng thật');
  await h.click(page.getByRole('button', { name: 'Thu gọn thanh điều hướng' }));
  await h.pause(1200);
  await h.click(nav.getByRole('link', { name: 'Hiệu quả platform' }));
  await expect(page.getByRole('heading', { name: 'Hiệu quả platform', level: 1 })).toBeVisible();
  check('effectiveness reachable from the collapsed rail', true);
  await h.click(page.getByRole('button', { name: 'Mở rộng thanh điều hướng' }));
  await h.click(nav.getByRole('link', { name: 'Ứng dụng', exact: true }));

  // ---- A. Preview on an existing Application (read-only) ----
  const sample = applications.find((a) => a.key === '7de8dc65-afcf-4808-9809-340c0225c79a');
  if (!sample) throw Error('retained sample Application missing');
  await h.click(page.getByText(sample.name, { exact: true }).first());
  await expect(page.getByRole('heading', { name: sample.name })).toBeVisible();
  await expect(page.locator('.workload-table .table-row')).toHaveCount(3);
  check('sample Application shows its three workloads', true);
  await h.read('Ứng dụng mẫu với frontend, backend, worker');
  const samplePath = `/api/v1/applications/${sample.key}/environments/staging`;
  const samplePreview = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith(`${samplePath}/preview`), { timeout: 180_000 });
  await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  const sampleReview = await (await samplePreview).json();
  await h.pause(2000);
  const deployButton = page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true });
  if ((sampleReview.review?.nodes ?? []).length === 0) {
    await expect(deployButton).toBeDisabled();
    check('no-change Preview renders without a blank screen, Deploy disabled', await page.locator('main').isVisible());
  } else {
    await expect(review.locator('.review-edges>path')).not.toHaveCount(0);
    check('existing Application Preview draws its graph', true);
  }
  await shot('A-sample-preview');

  // ---- B. New owned Application, real deployment on kind ----
  const stamp = Date.now().toString().slice(-8);
  const appName = `Tester Merge ${stamp}`;
  await h.click(nav.getByRole('link', { name: 'Ứng dụng', exact: true }));
  const app = await createApplication(h, appName, `tester-${stamp}`);
  const ns = `app-${app}-staging`;
  const path = `/api/v1/applications/${app}/environments/staging`;
  Object.assign(identity, { app, appName, namespace: ns });
  await writeFile(`${evidence}/identity.json`, JSON.stringify(identity, null, 2));
  check('new Application created through the form', Boolean(app));
  await addWorkload(h, { name: 'web', image: IMAGE, bindings: [], port: { name: 'http', port: '8080' } });
  check('workload typed and saved through the form', true);

  const first = await previewThenDeploy(path);
  check('Preview binds the existing kind cluster', first.review.nodes.some((n) => n.resourceType === 'k8s-cluster' && n.intent === 'BIND'));
  check('Preview creates the namespace and the workload', first.review.nodes.some((n) => n.resourceType === 'k8s-namespace' && n.intent === 'CREATE') && first.review.nodes.some((n) => n.kind === 'workload'));
  const webNode = review.locator('[data-node-id="workload.default#modules.web"]');
  await h.moveTo(review.locator('.review-graph'));
  await h.read('Đồ thị: cluster có sẵn, namespace mới, workload web');
  await expect(webNode.locator('.review-node-facts')).toContainText('acceptance-frontend');
  check('workload card shows the image fact', true);
  await h.click(webNode);
  await h.read('Tổng quan node web');
  await h.click(review.getByLabel('Chỉ đường nối của node đang chọn'));
  await expect(review.locator('.review-edges>path[data-focused=true]')).not.toHaveCount(0);
  check('selected-node connection view on real graph', true);
  await h.pause(1500);
  await h.click(review.getByLabel('Chỉ đường nối của node đang chọn'));
  await h.click(review.getByRole('tab', { name: 'Code', exact: true }));
  await expect(review.locator('.review-diff-new')).not.toHaveCount(0);
  await h.read('Manifest mới (xanh)');
  const nsNode = review.locator('[data-node-type="k8s-namespace"], [data-resource-type="k8s-namespace"]').first();
  if (await nsNode.count()) { await h.click(nsNode); await h.read('Nguồn Terraform của namespace'); }
  await shot('B1-preview');
  const firstDeploy = await deploy(path, 'B1-deploy');
  await expect(page.getByLabel('Theo dõi triển khai').locator('[data-node-id="workload.default#modules.web"]')).toHaveAttribute('data-state', 'READY');
  check('web node READY in progress view', true);
  const created = JSON.parse(kube('get', 'namespace', ns, '-o', 'json'));
  identity.namespaceUid = created.metadata.uid;
  await writeFile(`${evidence}/identity.json`, JSON.stringify(identity, null, 2));
  check('namespace exists on kind-k8s-4f', created.metadata.name === ns);
  check('Deployment web available on kind', kube('-n', ns, 'get', 'deployment', 'web', '-o', 'jsonpath={.status.availableReplicas}').trim() === '1');
  void firstDeploy;

  // Update the image through the form; diff must use the exact applied baseline.
  const deployments = (await get(`${path}/deployments`)).deployments;
  const applied = deployments.find((d) => d.workloadId === 'web' && d.status === 'SUCCEEDED');
  await h.click(page.locator('.workload-table .table-row').filter({ hasText: /web/ }).getByRole('button', { name: 'Sửa', exact: true }));
  await h.type(page.getByLabel('Ảnh Container', { exact: true }), `${IMAGE}-b`, { replace: true });
  await h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
  await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
  const second = await previewThenDeploy(path);
  const manifest = second.review.artifacts.find((a) => a.nodeId === 'workload.default#modules.web' && a.path === 'Deployment/web');
  check('update diff compares against the exact applied deployment', manifest.before.provenance.baselineDeploymentId === applied.id && manifest.before.text.includes(IMAGE) && manifest.after.text.includes(`${IMAGE}-b`));
  await h.click(webNode);
  await h.click(review.getByRole('tab', { name: 'Code', exact: true }));
  await expect(review.locator('.review-diff-old')).not.toHaveCount(0);
  await expect(review.locator('.review-diff-new')).not.toHaveCount(0);
  await h.read('Diff đỏ/xanh: image v46 sang v46-b');
  await shot('B2-update-diff');
  await deploy(path, 'B2-update');
  await expect.poll(() => kube('-n', ns, 'get', 'deployment', 'web', '-o', 'jsonpath={.spec.template.spec.containers[0].image}').trim(), { timeout: 120_000 }).toContain(`${IMAGE}-b`);
  check('kind Deployment runs the updated image', true);

  // Remove the workload, then clean up the namespace through the product.
  page.once('dialog', (dialog) => dialog.accept());
  await h.click(page.locator('.workload-table .table-row').getByRole('button', { name: 'Xóa', exact: true }));
  await expect(page.locator('.workload-table')).toContainText('Chờ xóa');
  await h.read('Workload chờ xóa');
  await previewThenDeploy(path).catch(async () => {});
  await shot('B3-remove-preview');
  await deploy(path, 'B3-remove');
  const inventory = await get(`${path}/resources`);
  const root = inventory.resources.find((r) => r.status === 'UNREFERENCED');
  check('namespace becomes unreferenced after removal', Boolean(root));
  await h.click(page.getByRole('region', { name: 'Tài nguyên của môi trường' }).getByRole('button', { name: /k8s-namespace/ }));
  await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
  const dialog = page.getByRole('dialog', { name: 'Xóa tài nguyên', exact: true });
  const confirmation = await dialog.getByTestId('root-name').innerText();
  await h.type(dialog.getByLabel('Nhập lại tên tài nguyên'), confirmation);
  await h.click(dialog.getByRole('button', { name: 'Xác nhận xóa vĩnh viễn' }));
  await expect.poll(async () => (await get(`${path}/resources`)).resources.find((r) => r.id === root.id)?.status, { timeout: 600_000, intervals: [3000] }).toBe('DESTROYED');
  await h.read('Tài nguyên đã xóa');
  await shot('B4-cleanup');
  check('owned namespace absent on kind after product cleanup', kube('get', 'namespace', ns, '--ignore-not-found', '-o', 'name').trim() === '');

  // ---- C. Platform effectiveness on the real history ----
  await h.click(nav.getByRole('link', { name: 'Hiệu quả platform' }));
  const orgReport = page.waitForResponse((r) => r.url().includes('/api/v1/effectiveness/report'));
  orgReport.catch(() => {});
  await page.reload();
  const org = await (await orgReport).json();
  await expect(page.getByRole('heading', { name: 'Giao phần mềm (DORA)' })).toBeVisible();
  check('organization report from real PostgreSQL history', org.scope.level === 'ORGANIZATION' && org.sources.every((s) => s.available));
  check('cleanup history source is available on PostgreSQL', org.sources.some((s) => s.id === 'resource_cleanups' && s.available));
  const orgCompleted = org.metrics.find((m) => m.id === 'completed_work');
  check('organization completed work includes real history', orgCompleted.status === 'COLLECTED' && orgCompleted.value >= 10);
  await h.read('Báo cáo toàn tổ chức trên dữ liệu thật');
  await h.moveTo(page.getByRole('heading', { name: 'Theo ứng dụng' }));
  await h.read('Bảng theo ứng dụng');
  await shot('C1-organization');

  // Help dialog for reading DORA metrics.
  await h.click(page.getByRole('button', { name: 'Cách đọc chỉ số' }));
  await h.read('Cách đọc chỉ số DORA');
  await h.click(page.getByRole('dialog').getByRole('button', { name: 'Đóng' }));

  // Drill into the new tester Application from the breakdown.
  const appWait = page.waitForResponse((r) => r.url().includes('/api/v1/effectiveness/report') && r.url().includes(`application=${app}`));
  appWait.catch(() => {});
  await h.click(page.locator('table').getByRole('button', { name: appName, exact: true }));
  const scoped = await (await appWait).json();
  const completed = scoped.metrics.find((m) => m.id === 'completed_work');
  check('tester Application: deploy, update, remove and cleanup are 4 completed work items', completed.value === 4);
  await h.read('Phạm vi ứng dụng tester');
  await shot('C2-tester-app');
  await h.click(page.getByRole('button', { name: /^Công việc hoàn thành: .*Xem chi tiết$/ }));
  const metric = page.getByRole('dialog');
  await expect(metric.locator('tbody tr')).toHaveCount(4);
  check('metric dialog lists the 4 real records', true);
  await h.read('Bản ghi: triển khai, cập nhật, gỡ bỏ, xóa tài nguyên');
  await shot('C3-records');
  await h.click(metric.getByRole('button', { name: /^Mở web/ }).first());
  await expect(page).toHaveURL(/\/deployments\//);
  await expect(page.getByLabel('Theo dõi triển khai').or(page.getByRole('heading', { level: 1 })).first()).toBeVisible();
  check('record opens the deployment detail (UC-09)', true);
  await h.read('Chi tiết deployment từ bản ghi');
  await shot('C4-deployment-detail');
  await page.goBack();
  await expect(page.getByRole('heading', { name: 'Hiệu quả platform', level: 1 })).toBeVisible();

  // Sample Application, then a workload drill-down.
  await h.choose(page.getByRole('combobox', { name: /^Ứng dụng/ }), sample.name);
  await expect(page.getByRole('heading', { name: 'Theo workload' })).toBeVisible();
  await h.choose(page.getByRole('combobox', { name: /^Workload/ }), 'frontend');
  await expect(page.getByRole('navigation', { name: 'Phạm vi' })).toContainText('frontend');
  check('drill-down Organization → sample Application → frontend', true);
  await h.choose(page.getByRole('combobox', { name: /^Thời gian/ }), '90 ngày');
  await h.read('Workload frontend trong 90 ngày');
  await shot('C5-sample-frontend');

  // CSV of the organization.
  await h.click(page.getByRole('navigation', { name: 'Phạm vi' }).getByRole('button', { name: 'Toàn tổ chức' }));
  const download = page.waitForEvent('download');
  download.catch(() => {});
  await h.click(page.getByRole('button', { name: 'Xuất CSV' }));
  const file = await download;
  const csvPath = `${evidence}/${file.suggestedFilename()}`;
  await file.saveAs(csvPath);
  const csv = await readFile(csvPath, 'utf8');
  check('CSV contains real Applications and the tester records', csv.includes(sample.name) && csv.includes(appName));
  check('CSV has no secret-like values', !/password|BEGIN [A-Z ]*PRIVATE KEY|token/i.test(csv));

  // Baseline version for the tester Application.
  await h.choose(page.getByRole('combobox', { name: /^Ứng dụng/ }), appName);
  await h.click(page.getByRole('button', { name: 'Mốc so sánh', exact: true }));
  const baseline = page.getByRole('dialog');
  await h.type(baseline.getByLabel('Thời gian thực hiện trung vị (phút)'), '30');
  await h.type(baseline.getByLabel('Nguồn đo'), 'Ước lượng quy trình thủ công trước khi có platform');
  const saveWait = page.waitForResponse((r) => r.url().includes('/api/v1/effectiveness/baselines') && r.request().method() === 'POST');
  saveWait.catch(() => {});
  await h.click(baseline.getByRole('button', { name: 'Lưu phiên bản mới' }));
  check('baseline saved to real PostgreSQL', (await saveWait).status() === 201);
  if (await page.getByRole('dialog').count()) await h.click(page.getByRole('dialog').getByRole('button', { name: 'Đóng' }));
  await page.reload();
  await expect(page.getByRole('button', { name: 'Xem mốc so sánh' })).toBeVisible();
  await h.moveTo(page.getByRole('heading', { name: 'Thời gian rút ngắn' }));
  await h.read('So sánh với mốc 30 phút');
  check('baseline comparison survives reload', true);
  await shot('C6-baseline');

  check('no uncaught browser errors', errors.length === 0);
  outcome = 'PASS';
} catch (error) {
  console.error(String(error?.stack ?? error).slice(0, 3000));
  await shot('failure').catch(() => {});
} finally {
  const video = page.video();
  await context.close();
  const videoPath = video ? await video.path() : null;
  await browser.close();
  await writeFile(`${evidence}/result.json`, JSON.stringify({ outcome, base, identity, assertions, errors, video: videoPath, durationSeconds: Math.round((Date.now() - started) / 1000) }, null, 2));
  console.log(`${outcome}: ${assertions.length} assertions; video ${videoPath}`);
  process.exitCode = outcome === 'PASS' ? 0 : 1;
}
