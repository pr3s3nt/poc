import { readFileSync } from 'node:fs';
import { parseArgs } from 'node:util';
import { expect } from '@playwright/test';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';
import { assertSameDefinition, desiredDefinition, parseBundleConfig } from '../../lib/render-bundle-config.mjs';

const { values } = parseArgs({ options: { config: { type: 'string' }, help: { type: 'boolean' } } });
if (values.help) {
  console.log('node e2e/setup/register-render-bundles/run.mjs --config /path/catalog.json\nConsole: ORCH_PLATFORM_URL, ORCH_PLATFORM_USERNAME, ORCH_PLATFORM_PASSWORD_FILE, ORCH_HEADLESS=1');
  process.exit(0);
}

let session, consoleConfig;
try {
  if (!values.config) throw new Error('Thiếu --config; xem config.example.json cạnh script');
  const config = parseBundleConfig(JSON.parse(readFileSync(values.config, 'utf8')));
  consoleConfig = readConsoleConfig();
  console.log(`Console: ${consoleConfig.baseURL}`);
  session = await openConsole(consoleConfig);
  if (!['PLATFORM_ENGINEER', 'ADMIN'].includes(session.user.role)) throw new Error('Cần tài khoản Platform Engineer hoặc Admin');
  const { page } = session;
  const get = async path => {
    const reply = await page.request.get(`${consoleConfig.baseURL}/api/v1${path}`);
    if (!reply.ok()) throw new Error(`GET ${path}: HTTP ${reply.status()}`);
    return reply.json();
  };
  const responseFor = (path, method) => page.waitForResponse(reply => reply.request().method() === method && new URL(reply.url()).pathname === `/api/v1${path}`, { timeout: 180_000 });
  const checkedBundleIds = new Set();
  for (const item of config.bundles) {
    await page.goto(`${consoleConfig.baseURL}/ui/platform/rendering-templates`);
    const form = page.getByRole('form', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true });
    await form.getByLabel('ID mẫu', { exact: true }).fill(item.key);
    await form.getByLabel('Tên hiển thị', { exact: true }).fill(item.name);
    await form.getByLabel('Git HTTPS URL', { exact: true }).fill(item.source.url);
    await form.getByLabel('Thư mục bundle', { exact: true }).fill(item.source.path);
    await form.getByLabel('Tag hoặc commit', { exact: true }).fill(item.source.ref);
    const inspectedReply = responseFor('/render-bundles/inspect', 'POST');
    await form.getByRole('button', { name: 'Kiểm tra bundle', exact: true }).click();
    const inspected = await inspectedReply;
    if (!inspected.ok()) throw new Error(`${item.key}: inspect thất bại HTTP ${inspected.status()}; xem lỗi trên Console`);
    const { renderBundle } = await inspected.json();
    if (renderBundle?.status !== 'AVAILABLE' || !renderBundle.source || renderBundle.source.url !== item.source.url || renderBundle.source.path !== item.source.path || !/^[0-9a-f]{40}$/.test(renderBundle.source.revision)) throw new Error(`${item.key}: pin Git không hợp lệ`);
    if (/^[0-9a-f]{40}$/.test(item.source.ref) && renderBundle.source.revision !== item.source.ref) throw new Error(`${item.key}: commit không khớp`);
    checkedBundleIds.add(renderBundle.id);
    const desired = desiredDefinition(item, renderBundle.id);
    const definitions = (await get('/resource-definitions')).resourceDefinitions;
    if (!Array.isArray(definitions)) throw new Error('Danh sách Definitions không hợp lệ');
    const existing = definitions.find(definition => definition.key === item.key);
    if (existing) {
      assertSameDefinition(existing, desired);
      console.log(`PASS: dùng lại ${item.key}, cấu hình và Git pin khớp`);
      continue;
    }
    await form.getByRole('button', { name: 'JSON nâng cao', exact: true }).click();
    await form.getByLabel('JSON nâng cao của mẫu dựng ứng dụng', { exact: true }).fill(JSON.stringify({ variables: desired.driverInputs.values.variables, provision: item.provision }, null, 2));
    // Advanced criteria retain all five supported fields and OR rows.
    await form.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }).check();
    const labels = { env_type: 'Loại môi trường', app_id: 'ID ứng dụng', env_id: 'ID môi trường', res_id: 'ID tài nguyên', class: 'Class' };
    for (const [index, row] of item.criteria.entries()) {
      if (index > 0) await form.getByRole('button', { name: '+ Thêm điều kiện', exact: true }).click();
      for (const [key, value] of Object.entries(row)) await form.getByLabel(`Điều kiện ${index + 1} ${labels[key]}`, { exact: true }).fill(value);
    }
    const savedReply = responseFor('/resource-definitions', 'POST');
    await form.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }).click();
    const saved = await savedReply;
    if (!saved.ok()) throw new Error(`${item.key}: đăng ký thất bại HTTP ${saved.status()}; không tự retry POST, chạy lại để kiểm tra kết quả`);
    const persisted = (await get('/resource-definitions')).resourceDefinitions.find(definition => definition.key === item.key);
    if (!persisted) throw new Error(`${item.key}: không tìm thấy cấu hình sau đăng ký`);
    assertSameDefinition(persisted, desired);
    await expect(page.locator('[data-definition]').filter({ has: page.getByRole('button', { name: `Sửa mẫu ${item.key}`, exact: true }) })).toBeVisible();
    console.log(`PASS: đăng ký ${item.key}, commit ${renderBundle.source.revision}`);
  }

  const apps = (await get('/applications')).applications;
  if (!Array.isArray(apps)) throw new Error('Danh sách Applications không hợp lệ');
  const app = config.verify.applicationKey ? apps.find(app => app.key === config.verify.applicationKey) : apps.find(app => app.environments?.some(env => env.key === config.verify.environment));
  if (!app) throw new Error('Bundle đã đăng ký; chưa có Application để kiểm tra picker. Tạo Application, đặt verify.applicationKey rồi chạy lại.');
  const env = config.verify.environment;
  const { templates } = await get(`/applications/${encodeURIComponent(app.key)}/environments/${env}/workload-templates`);
  if (!Array.isArray(templates)) throw new Error('Danh sách workload templates không hợp lệ');
  const expected = config.bundles.flatMap(item => item.expectedTemplates);
  for (const pin of expected) if (!templates.some(template => template.id === pin.id && template.version === pin.version)) throw new Error(`Catalog thiếu ${pin.id}@${pin.version}`);
  // Cache availability is distinct from successful metadata registration.
  for (const id of checkedBundleIds) {
    const { renderBundle } = await get(`/render-bundles/${encodeURIComponent(id)}`);
    if (!renderBundle || renderBundle.status !== 'AVAILABLE') throw new Error('Bundle đã lưu nhưng không AVAILABLE');
  }
  if (templates.length === 0) throw new Error('Catalog rỗng: bundle phải khai báo workloadTemplates trong bundle.yaml');
  await page.goto(`${consoleConfig.baseURL}/ui/applications/${encodeURIComponent(app.key)}?environment=${env}`);
  await page.getByRole('button', { name: '+ Thêm Workload', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Chọn loại workload', exact: true });
  await expect(dialog).toBeVisible();
  for (const pin of expected) {
    const template = templates.find(template => template.id === pin.id && template.version === pin.version);
    const label = template.labels.vi ?? template.labels.en ?? template.id;
    await expect(dialog.getByRole('button').filter({ hasText: label }).filter({ hasText: template.version })).toHaveCount(1);
  }
  await expect(dialog.locator('.application-card')).toHaveCount(templates.length);
  console.log(`PASS: Thêm Workload hiển thị ${templates.length} mẫu; không tạo workload hoặc deploy`);
} catch (error) {
  // Raw Playwright errors can include filled values; do not log them.
  console.error(`FAIL: ${error.name === 'Error' ? error.message.split('\n')[0] : 'Browser assertion/timeout thất bại; kiểm tra Console và cấu hình'}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, consoleConfig);
}
