import { assertSameResourceType, parseResourceTypes } from './lib/catalog-config.mjs';
import { ensureInfrastructureAccount } from './lib/infrastructure-ui.mjs';
import { parseCatalogConfig } from './lib/catalog-config.mjs';
import { ensureCatalog } from './lib/catalog-ui.mjs';
import { parseGitAccess } from './lib/git-access-config.mjs';
import { ensureGitAccess } from './lib/git-access-ui.mjs';
import { parseArgs } from 'node:util';
import { expect } from '@playwright/test';
import { closeConsole, openConsole, readConsoleConfig } from './lib/console.mjs';
import { loadTarget } from './lib/target-config.mjs';
import { parseInfrastructure, readSetupPacing } from './lib/infrastructure-config.mjs';

const { values } = parseArgs({ options: { config: { type: 'string' }, target: { type: 'string' }, url: { type: 'string' }, 'git-url': { type: 'string' }, 'update-git-access': { type: 'boolean' }, kubeconfig: { type: 'string' }, help: { type: 'boolean' }, ...Object.fromEntries(['step', 'account', 'type', 'definition', 'bundle'].map(phase => [`${phase}-delay-ms`, { type: 'string' }])) } });
if (values.help) {
  console.log('node scripts/bootstrap/run.mjs --target dev|staging [--url https://console.example] [--kubeconfig /private/kubeconfig] --git-url https://git.company/catalog.git [--update-git-access] OR --config /private/bootstrap.json');
  process.exit(0);
}

let session, consoleConfig, lastAction = '';
try {
  const target = loadTarget(values);
  const config = { ...target.raw, ...parseInfrastructure(target.raw), resourceTypes: parseResourceTypes(target.raw.resourceTypes ?? []) };
  parseCatalogConfig(target.raw, values);
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
  const ui = { page, baseURL: consoleConfig.baseURL, act, get, responseFor };
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
  phase = 'bundle';
  await ensureCatalog({ ...ui, source: parseCatalogConfig(target.raw, values) });
} catch (error) {
  // Raw Playwright errors can include filled values; do not log them.
  console.error(`FAIL: ${error.name === 'Error' ? error.message.split('\n')[0] : 'Browser assertion/timeout thất bại; kiểm tra Console và cấu hình'}`);
  if (lastAction) console.error(`UI action: ${lastAction}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, consoleConfig);
}
