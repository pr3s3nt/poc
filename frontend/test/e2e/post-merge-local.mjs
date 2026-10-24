// Post-merge browser check of features/preview + features/manager on main.
// Runs against the disposable loopback fixture (backend/test/fixtures/deployment-review):
// product HTTP/UI and the installed score-k8s renderer, fake execution adapters only.
// A Platform Engineer works like a normal user with a visible cursor and typed input;
// the whole session is recorded as video. No cluster, cloud or retained data is touched.
import { chromium, expect } from '@playwright/test';
import { mkdir, readFile, writeFile } from 'node:fs/promises';
import { createHuman, createApplication, addWorkload, installCursor } from './human.mjs';

const base = process.env.POST_MERGE_BASE_URL;
const evidence = process.env.POST_MERGE_EVIDENCE_DIR;
if (!base || !evidence) throw Error('POST_MERGE_BASE_URL and POST_MERGE_EVIDENCE_DIR required');
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(base)) throw Error('Loopback fixture only');
await mkdir(evidence, { recursive: true });

const viewport = { width: 1512, height: 980 };
const browser = await chromium.launch({ headless: true });
const context = await browser.newContext({ viewport, acceptDownloads: true, recordVideo: { dir: evidence, size: viewport } });
await context.addInitScript(installCursor);
const page = await context.newPage();
const errors = [];
page.on('pageerror', (e) => errors.push(e.message));
const assertions = [];
const check = (name, value) => { if (!value) throw Error(`assertion failed: ${name}`); assertions.push(name); console.log(`PASS ${name}`); };
const shot = (name) => page.screenshot({ path: `${evidence}/${name}.png`, fullPage: true });
const h = createHuman(page);
const stamp = Date.now().toString().slice(-8);
let outcome = 'FAIL';

try {
  // 1. Sign in as Platform Engineer.
  await page.goto(`${base}/ui/sign-in`);
  await h.showCursor();
  await h.type(page.getByLabel('Tên đăng nhập'), 'platform-engineer');
  await h.type(page.getByLabel('Mật khẩu'), 'test-password');
  await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
  check('platform engineer signs in', true);

  // 2. Merged navigation: preview's collapsible rail carries manager's effectiveness link.
  const nav = page.getByRole('navigation', { name: 'Điều hướng chính' });
  check('effectiveness link is in the collapsible rail', await nav.getByRole('link', { name: 'Hiệu quả platform' }).isVisible());
  await h.click(page.getByRole('button', { name: 'Thu gọn thanh điều hướng' }));
  await expect(page.getByRole('button', { name: 'Mở rộng thanh điều hướng' })).toHaveAttribute('aria-expanded', 'false');
  await h.pause(1200);
  await h.click(page.getByRole('button', { name: 'Mở rộng thanh điều hướng' }));
  await expect(page.getByRole('button', { name: 'Thu gọn thanh điều hướng' })).toHaveAttribute('aria-expanded', 'true');
  check('sidebar collapses and expands', true);

  // 3. Create an Application through the Console.
  const appName = `Post Merge QA ${stamp}`;
  const app = await createApplication(h, appName, `pm-${stamp}`);
  check('application created through the form', Boolean(app));

  // Setup (technical, via API): Definitions scoped to this Application only.
  const definitions = (await (await context.request.get(`${base}/api/v1/resource-definitions`)).json()).resourceDefinitions;
  for (const key of ['cluster-internal', 'namespace-k8s-module', 'workload-score-k8s']) {
    const original = definitions.find((d) => d.key === key);
    if (!original) throw Error(`missing fixture definition ${key}`);
    const { version, sourceFingerprint, ...draft } = original; void version; void sourceFingerprint;
    draft.key = `${key}-${app}`; draft.name = `QA ${original.name}`;
    draft.criteria = [{ app_id: app, ...(key === 'cluster-internal' ? { class: 'internal' } : {}) }];
    const res = await context.request.post(`${base}/api/v1/resource-definitions`, { data: draft });
    if (!res.ok()) throw Error(`register ${key}: HTTP ${res.status()}`);
  }
  check('application-scoped definitions registered', true);

  // 4. Add two workloads like a developer would.
  for (const name of ['api', 'web']) await addWorkload(h, { name, image: `example.invalid/${name}:v1`, bindings: [], port: { name: 'http', port: '8080' } });
  check('two workloads saved through the form', true);

  // 5. Preview: graph, safe node facts, selected-node connections, code tab.
  const previewResponse = page.waitForResponse((r) => r.url().endsWith('/preview') && r.request().method() === 'POST');
  await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  const preview = await (await previewResponse).json();
  check('preview graph has both workloads', preview.review.nodes.filter((n) => n.kind === 'workload').length === 2);
  const review = page.getByLabel('Xem trước triển khai');
  await expect(review.locator('.review-edges>path')).not.toHaveCount(0);
  await h.moveTo(review.locator('.review-graph'));
  await h.read('Đồ thị xem trước với các node và đường nối');
  const apiNode = review.locator('[data-node-id="workload.default#modules.api"]');
  await expect(apiNode.locator('.review-node-facts')).toContainText('example.invalid/api:v1');
  check('workload card shows safe image fact', true);
  await h.click(apiNode);
  await expect(apiNode).toHaveClass(/review-node-selected/);
  const focus = review.getByLabel('Chỉ đường nối của node đang chọn');
  await h.click(focus);
  await expect(focus).toBeChecked();
  await expect(review.locator('.review-edges>path[data-focused=true]')).not.toHaveCount(0);
  await h.pause(1500);
  await h.click(focus);
  check('selected-node connection view toggles', !(await focus.isChecked()));
  await h.click(review.getByRole('tab', { name: 'Code', exact: true }));
  await expect(review.locator('.review-diff-new')).not.toHaveCount(0);
  await h.read('Code diff');
  check('code tab shows new manifest lines', true);
  await shot('01-preview');

  // 6. Deploy.
  const deployResponse = page.waitForResponse((r) => r.url().endsWith('/deploy') && r.request().method() === 'POST');
  await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
  const deployed = await deployResponse;
  const report = await deployed.json();
  check('deployment succeeds (fake adapters)', deployed.ok() && report.status === 'SUCCEEDED');
  await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công');
  for (const w of ['api', 'web']) await expect(page.getByLabel('Theo dõi triển khai').locator(`[data-node-id="workload.default#modules.${w}"]`)).toHaveAttribute('data-state', 'READY');
  check('both workload nodes READY', true);
  await h.moveTo(page.getByLabel('Kết quả triển khai'));
  await h.read('Triển khai thành công');
  await shot('02-deployed');

  // 7. Platform effectiveness (UC-17) through the sidebar.
  await h.click(nav.getByRole('link', { name: 'Hiệu quả platform' }));
  await expect(page.getByRole('heading', { name: 'Hiệu quả platform', level: 1 })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Giao phần mềm (DORA)' })).toBeVisible();
  check('effectiveness report renders for the organization', true);
  await h.read('Báo cáo hiệu quả toàn tổ chức');
  const appRow = page.locator('table tr').filter({ hasText: appName });
  await expect(appRow).toHaveCount(1);
  check('organization breakdown lists the new application', true);

  const reportWait = page.waitForResponse((r) => r.url().includes('/api/v1/effectiveness/report') && r.url().includes(`application=${app}`));
  reportWait.catch(() => {});
  await h.choose(page.getByRole('combobox', { name: /^Ứng dụng/ }), appName);
  const scoped = await (await reportWait).json();
  const completed = scoped.metrics.find((m) => m.id === 'completed_work');
  check('application scope counts the two completed deployments', scoped.scope.level === 'APPLICATION' && completed.status === 'COLLECTED' && completed.value === 2);
  await expect(page.getByRole('navigation', { name: 'Phạm vi' })).toContainText(appName);
  await expect(page.getByRole('heading', { name: 'Theo workload' })).toBeVisible();
  check('drill-down shows workload breakdown', true);
  await shot('03-effectiveness-app');

  // Metric detail dialog with records linking back to deployment history.
  await h.click(page.getByRole('button', { name: /^Công việc hoàn thành: .*Xem chi tiết$/ }));
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('button', { name: 'Mở api' })).toBeVisible();
  await expect(dialog.getByRole('button', { name: 'Mở web' })).toBeVisible();
  await h.read('Chi tiết chỉ số và bản ghi');
  check('metric dialog lists api and web records', true);
  await h.click(dialog.getByRole('button', { name: 'Đóng' }));
  await expect(dialog).toHaveCount(0);

  // CSV export.
  const downloadWait = page.waitForEvent('download');
  downloadWait.catch(() => {});
  await h.click(page.getByRole('button', { name: 'Xuất CSV' }));
  const download = await downloadWait;
  const csvPath = `${evidence}/${download.suggestedFilename() || 'effectiveness.csv'}`;
  await download.saveAs(csvPath);
  const csv = await readFile(csvPath, 'utf8');
  check('CSV export contains the application and its records', csv.includes(appName) && /api/.test(csv) && /web/.test(csv));

  // Baseline version (append-only).
  await h.click(page.getByRole('button', { name: 'Mốc so sánh', exact: true }));
  const baselineDialog = page.getByRole('dialog');
  await h.type(baselineDialog.getByLabel('Thời gian thực hiện trung vị (phút)'), '45');
  await h.type(baselineDialog.getByLabel('Nguồn đo'), 'Đo thủ công quy trình cũ, 10 lần triển khai staging');
  await h.type(baselineDialog.getByLabel('Số mẫu'), '10');
  const saveWait = page.waitForResponse((r) => r.url().includes('/api/v1/effectiveness/baselines') && r.request().method() === 'POST');
  saveWait.catch(() => {});
  await h.click(baselineDialog.getByRole('button', { name: 'Lưu phiên bản mới' }));
  const saved = await saveWait;
  check('baseline version saved (201)', saved.status() === 201);
  await h.pause(1500);
  if (await page.getByRole('dialog').count()) await h.click(page.getByRole('dialog').getByRole('button', { name: 'Đóng' }));
  const baselines = await (await context.request.get(`${base}/api/v1/effectiveness/baselines?application=${app}`)).json();
  check('baseline persisted as version 1', baselines.currentVersion === 1 && baselines.baselines[0].valueSeconds === 2700);
  await page.reload();
  await expect(page.getByRole('heading', { name: 'Thời gian rút ngắn' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Xem mốc so sánh' })).toBeVisible();
  check('baseline comparison shown after reload', true);
  await h.read('So sánh với mốc');
  await shot('04-effectiveness-baseline');

  // 8. Back to the Application through the sidebar; deployment remains visible.
  await h.click(nav.getByRole('link', { name: 'Ứng dụng', exact: true }));
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
  await h.click(page.getByText(appName, { exact: true }).first());
  await expect(page.getByRole('heading', { name: appName })).toBeVisible();
  check('application reopens after visiting effectiveness', true);
  await h.pause(1500);

  check('no uncaught browser errors', errors.length === 0);
  const html = await page.content();
  check('no secret-like values in final page', !/test-password|BEGIN [A-Z ]*PRIVATE KEY/.test(html));
  outcome = 'PASS';
} catch (error) {
  console.error(error);
  await shot('failure').catch(() => {});
} finally {
  const video = page.video();
  await context.close();
  const videoPath = video ? await video.path() : null;
  await browser.close();
  await writeFile(`${evidence}/result.json`, JSON.stringify({ outcome, base, assertions, errors, video: videoPath, adapters: 'fake' }, null, 2));
  console.log(`${outcome}: ${assertions.length} assertions; video ${videoPath}`);
  process.exitCode = outcome === 'PASS' ? 0 : 1;
}
