import { preserveConsoleVariables } from './lib/config-diff.mjs';
import { parseGitAccess } from './lib/git-access-config.mjs';
import { ensureGitAccess } from './lib/git-access-ui.mjs';
import { parseArgs } from 'node:util';
import { expect } from '@playwright/test';
import { closeConsole, openConsole, readConsoleConfig } from './lib/console.mjs';
import { assertSameDefinition, assertSameResourceType, desiredDefinition, parseBundleConfig } from './lib/render-bundle-config.mjs';
import { updateRenderDefinitionSource } from './lib/definition-update-ui.mjs';
import { definitionAction } from './lib/definition-action.mjs';
import { loadTarget } from './lib/target-config.mjs';
import { readSetupPacing } from './lib/infrastructure-config.mjs';
import { ensureInfrastructureAccount, ensureResourceDefinitions } from './lib/infrastructure-ui.mjs';

const { values } = parseArgs({ options: { config: { type: 'string' }, target: { type: 'string' }, url: { type: 'string' }, 'git-url': { type: 'string' }, 'git-ref': { type: 'string' }, 'update-existing': { type: 'boolean' }, 'update-git-access': { type: 'boolean' }, kubeconfig: { type: 'string' }, help: { type: 'boolean' }, ...Object.fromEntries(['step', 'account', 'type', 'definition', 'bundle'].map(phase => [`${phase}-delay-ms`, { type: 'string' }])) } });
if (values.help) {
  console.log('node scripts/bootstrap/run.mjs --target dev|staging [--url https://console.example] [--kubeconfig /private/kubeconfig] [--git-url https://git.company/catalog.git --git-ref FULL_COMMIT] [--update-existing] [--update-git-access] OR --config /path/catalog.json [--step-delay-ms 800] [--type-delay-ms 1000] [--definition-delay-ms 1500] [--account-delay-ms 800] [--bundle-delay-ms 800]\nConsole: ORCH_PLATFORM_URL, ORCH_PLATFORM_USERNAME, ORCH_PLATFORM_PASSWORD_FILE; presets mặc định ORCH_HEADLESS=1\nKubeconfig (chỉ cần khi account chưa có): ORCH_KUBECONFIG_FILE');
  process.exit(0);
}

let session, consoleConfig, lastAction = '';
try {
  const target = loadTarget(values);
  const config = parseBundleConfig(target.raw);
  const gitAccess = parseGitAccess(target.raw, values);
  if (values['update-git-access'] && !gitAccess) throw new Error('--update-git-access cần gitAccess đang bật trong cấu hình');
  if (gitAccess && (process.env.DEBUG || process.env.NODE_DEBUG || process.env.PWDEBUG && process.env.PWDEBUG !== '0')) throw new Error('Git access: tắt DEBUG, NODE_DEBUG và PWDEBUG trước khi nhập token');
  const pacing = readSetupPacing(values, target.env);
  let phase = 'account';
  consoleConfig = readConsoleConfig(target.env, { defaultPassword: target.defaultPassword });
  console.log(`Console: ${consoleConfig.baseURL}`);
  session = await openConsole(consoleConfig);
  if (!['PLATFORM_ENGINEER', 'ADMIN'].includes(session.user.role)) throw new Error('Cần tài khoản Platform Engineer hoặc Admin');
  const { page } = session;
  const act = async (locator, method, ...args) => {
    lastAction = `${phase}: ${method} ${locator.toString()}`;
    const stepDelay = pacing[phase];
    const visible = method === 'setInputFiles' ? locator.locator('..') : locator;
    await visible.scrollIntoViewIfNeeded();
    await visible.hover();
    await page.waitForTimeout(stepDelay);
    if (method === 'fill') {
      await locator.fill('');
      await locator.pressSequentially(args[0], { delay: Math.min(45, Math.floor(stepDelay / 20)) });
    } else await locator[method](...args);
    await page.waitForTimeout(stepDelay);
  };
  const get = async path => {
    const reply = await page.request.get(`${consoleConfig.baseURL}/api/v1${path}`);
    if (!reply.ok()) throw new Error(`GET ${path}: HTTP ${reply.status()}`);
    return reply.json();
  };
  const responseFor = (path, method) => page.waitForResponse(reply => reply.request().method() === method && new URL(reply.url()).pathname === `/api/v1${path}`, { timeout: 180_000 });
  const readTypes = async () => {
    const { resourceTypes } = await get('/resource-types');
    if (!Array.isArray(resourceTypes)) throw new Error('Danh sách Resource Types không hợp lệ');
    return resourceTypes;
  };
  // Check existing schemas before any create, including before bundle inspection.
  const existingTypes = await readTypes();
  for (const type of config.resourceTypes) {
    const existing = existingTypes.find(item => item.key === type.key);
    if (existing) assertSameResourceType(existing, type);
  }
  const ui = { page, baseURL: consoleConfig.baseURL, act, get, responseFor, updateExisting: values['update-existing'] === true };
  await ensureGitAccess({ ...ui, desired: gitAccess, update: values['update-git-access'] === true });
  await ensureInfrastructureAccount({ ...ui, account: config.account, kubeconfigFile: target.env.ORCH_KUBECONFIG_FILE });
  phase = 'type';
  for (const type of config.resourceTypes) {
    if (existingTypes.some(item => item.key === type.key)) {
      console.log(`PASS: dùng lại Resource Type ${type.key}, schema khớp`);
      continue;
    }
    console.log(`UI: đăng ký Resource Type ${type.key}`);
    await page.goto(`${consoleConfig.baseURL}/ui/platform/resource-types`);
    const form = page.locator('form').filter({ has: page.getByLabel('ID loại tài nguyên', { exact: true }) });
    await act(form.getByLabel('ID loại tài nguyên', { exact: true }), 'fill', type.key);
    for (const [kind, label, noun] of [['inputs', 'Đầu vào', 'tham số đầu vào'], ['outputs', 'Đầu ra', 'tham số đầu ra']]) {
      for (const [index, field] of type[kind].entries()) {
        await act(form.getByRole('button', { name: `+ Thêm ${noun}`, exact: true }), 'click');
        const prefix = `${label} ${index + 1}`;
        await act(form.getByLabel(`${prefix} tên`, { exact: true }), 'fill', field.name);
        await act(form.getByLabel(`${prefix} kiểu`, { exact: true }), 'selectOption', field.type);
        await act(form.getByLabel(`${prefix} bắt buộc`, { exact: true }), 'setChecked', field.required);
        if (kind === 'outputs') await act(form.getByLabel(`${prefix} bí mật`, { exact: true }), 'setChecked', field.secret);
      }
    }
    const reply = responseFor('/resource-types', 'POST');
    await act(form.getByRole('button', { name: 'Đăng ký loại tài nguyên', exact: true }), 'click');
    const saved = await reply;
    if (!saved.ok()) throw new Error(`${type.key}: đăng ký Resource Type thất bại HTTP ${saved.status()}; kiểm tra Console trước khi chạy lại`);
    const persisted = (await readTypes()).find(item => item.key === type.key);
    if (!persisted) throw new Error(`${type.key}: không tìm thấy Resource Type sau đăng ký`);
    assertSameResourceType(persisted, type);
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký loại tài nguyên ${type.key}.` })).toBeVisible();
    console.log(`PASS: đăng ký Resource Type ${type.key}, inputs/outputs/flags khớp`);
  }
  phase = 'definition';
  await ensureResourceDefinitions({ ...ui, definitions: config.resourceDefinitions });
  phase = 'bundle';
  const checkedBundleIds = new Set();
  for (const item of config.bundles) {
    console.log(`UI: mở và kiểm tra Git bundle ${item.key}`);
    await page.goto(`${consoleConfig.baseURL}/ui/platform/rendering-templates`);
    const form = page.getByRole('form', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true });
    await act(form.getByLabel('ID mẫu', { exact: true }), 'fill', item.key);
    await act(form.getByLabel('Tên hiển thị', { exact: true }), 'fill', item.name);
    await act(form.getByLabel('Git HTTPS URL', { exact: true }), 'fill', item.source.url);
    await act(form.getByLabel('Thư mục bundle', { exact: true }), 'fill', item.source.path);
    await act(form.getByLabel('Tag hoặc commit', { exact: true }), 'fill', item.source.ref);
    const inspectedReply = responseFor('/render-bundles/inspect', 'POST');
    await act(form.getByRole('button', { name: 'Kiểm tra bundle', exact: true }), 'click');
    const inspected = await inspectedReply;
    if (!inspected.ok()) throw new Error(`${item.key}: inspect thất bại HTTP ${inspected.status()}; xem lỗi trên Console`);
    const { renderBundle } = await inspected.json();
    if (renderBundle?.status !== 'AVAILABLE' || !renderBundle.source || renderBundle.source.url !== item.source.url || renderBundle.source.path !== item.source.path || !/^[0-9a-f]{40}$/.test(renderBundle.source.revision)) throw new Error(`${item.key}: pin Git không hợp lệ`);
    if (/^[0-9a-f]{40}$/.test(item.source.ref) && renderBundle.source.revision !== item.source.ref) throw new Error(`${item.key}: commit không khớp`);
    checkedBundleIds.add(renderBundle.id);
    let desired = desiredDefinition(item, renderBundle.id);
    const definitions = (await get('/resource-definitions')).resourceDefinitions;
    if (!Array.isArray(definitions)) throw new Error('Danh sách Definitions không hợp lệ');
    const existing = definitions.find(definition => definition.key === item.key);
    desired = preserveConsoleVariables(existing, desired, values['update-existing'] === true, console.warn);
    const action = definitionAction(existing, desired, values['update-existing'] === true);
    if (action === 'reuse') {
      console.log(`PASS: dùng lại ${item.key}, cấu hình và Git pin khớp`);
      continue;
    }
    if (action === 'update') {
      await updateRenderDefinitionSource({ page, baseURL: consoleConfig.baseURL, act, existing, desired, bundleId: renderBundle.id });
      const persisted = (await get('/resource-definitions')).resourceDefinitions.find(def => def.key === item.key);
      if (!persisted || persisted.version !== existing.version + 1) throw new Error(`${item.key}: không xác minh được version CAS sau update`);
      assertSameDefinition(persisted, desired);
      console.log(`PASS: cập nhật source ${item.key}, version ${existing.version} → ${persisted.version}`);
      continue;
    }
    await act(form.getByLabel('Tham chiếu namespace', { exact: true }), 'fill', item.placement.namespace);
    await act(form.getByLabel('Tham chiếu cluster', { exact: true }), 'fill', item.placement.cluster);
    for (const [key, value] of Object.entries(item.renderInputs)) {
      const schema = renderBundle.inputsSchema?.properties?.[key];
      if (!schema) throw new Error(`${item.key}: render input ${key} không có trong schema`);
      const control = form.locator('[data-input]').filter({ has: page.getByLabel(schema.title || key, { exact: true }) }).getByLabel(schema.title || key, { exact: true });
      if (Array.isArray(schema.enum)) await act(control, 'selectOption', JSON.stringify(value));
      else if (schema.type === 'boolean') {
        // A false default still needs an explicit user change so the authored
        // false is preserved in the saved Definition.
        await act(control, 'setChecked', !value);
        await act(control, 'setChecked', value);
      } else {
        if (value === '') await act(control, 'fill', ' ');
        await act(control, 'fill', String(value));
      }
    }
    // Advanced criteria retain all five supported fields and OR rows.
    await act(form.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }), 'check');
    const labels = { env_type: 'Loại môi trường', app_id: 'ID ứng dụng', env_id: 'ID môi trường', res_id: 'ID tài nguyên', class: 'Class' };
    for (const [index, row] of item.criteria.entries()) {
      if (index > 0) await act(form.getByRole('button', { name: '+ Thêm điều kiện', exact: true }), 'click');
      for (const [key, value] of Object.entries(row)) await act(form.getByLabel(`Điều kiện ${index + 1} ${labels[key]}`, { exact: true }), 'fill', value);
    }
    const savedReply = responseFor('/resource-definitions', 'POST');
    await act(form.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }), 'click');
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
  if (!app && config.verify.applicationKey) throw new Error('Không tìm thấy Application kiểm tra đã chỉ định');
  if (!app) console.log('PASS: seed đã lưu; chưa có Application nên bỏ kiểm tra picker/volume mount, không tự tạo app');
  else {
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
    await act(page.getByRole('button', { name: '+ Thêm Workload', exact: true }), 'click');
    const dialog = page.getByRole('dialog', { name: 'Chọn loại workload', exact: true });
    await expect(dialog).toBeVisible();
    for (const pin of expected) {
      const template = templates.find(template => template.id === pin.id && template.version === pin.version);
      const label = template.labels.vi ?? template.labels.en ?? template.id;
      await expect(dialog.getByRole('button').filter({ hasText: label }).filter({ hasText: template.version })).toHaveCount(1);
    }
    await expect(dialog.locator('.application-card')).toHaveCount(templates.length);
    console.log(`PASS: Thêm Workload hiển thị ${templates.length} mẫu; không tạo workload hoặc deploy`);
    if (config.verify.volumeMount) {
      const check = config.verify.volumeMount;
      const template = templates.find(item => item.id === check.templateId && item.version === check.templateVersion);
      if (!template) throw new Error('Không tìm thấy template để kiểm tra volume mount');
      const type = (await readTypes()).find(item => item.key === check.resourceType);
      if (!type || !['claimName', 'namespace'].every(name => type.outputs?.some(field => field.name === name && field.type === 'string' && !field.secret))) throw new Error(`${check.resourceType}: thiếu storage outputs claimName/namespace dạng string công khai`);
      await act(dialog.getByRole('button').filter({ hasText: template.labels.vi ?? template.labels.en ?? template.id }).filter({ hasText: template.version }), 'click');
      const addMount = page.getByRole('button', { name: 'Thêm volume mount', exact: true });
      await expect(addMount).toBeEnabled();
      await act(addMount, 'click');
      const storageDialog = page.getByRole('dialog', { name: 'Thêm tài nguyên lưu trữ', exact: true });
      await act(storageDialog.getByLabel('Alias tài nguyên', { exact: true }), 'fill', 'data');
      await act(storageDialog.getByLabel('Loại tài nguyên', { exact: true }), 'selectOption', check.resourceType);
      await act(storageDialog.getByRole('button', { name: 'Thêm và chọn', exact: true }), 'click');
      await expect(page.getByLabel('Mount path', { exact: true })).toHaveValue('/data');
      console.log(`PASS: ${check.templateId} thêm volume mount từ ${check.resourceType}; chỉ sửa form local, không lưu draft`);
    }
  }
} catch (error) {
  // Raw Playwright errors can include filled values; do not log them.
  console.error(`FAIL: ${error.name === 'Error' ? error.message.split('\n')[0] : 'Browser assertion/timeout thất bại; kiểm tra Console và cấu hình'}`);
  if (lastAction) console.error(`UI action: ${lastAction}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, consoleConfig);
}
