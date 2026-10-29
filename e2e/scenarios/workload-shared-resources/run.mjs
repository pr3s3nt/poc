// Disposable product UI + real HTTP handlers, fake execution adapters only.
import { expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import { readConsoleConfig, openConsole, closeConsole } from '../../lib/console.mjs';

const config = readConsoleConfig();
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(config.baseURL)) throw Error('Disposable loopback fixture only');
const evidence = process.env.ORCH_EVIDENCE_DIR;
if (!evidence) throw Error('ORCH_EVIDENCE_DIR outside Git required');
await mkdir(evidence, { recursive: true });
const consoleSession = await openConsole(config, { role: 'DEVELOPER' });
const { page, context } = consoleSession;
const errors = [], assertions = [];
page.on('pageerror', error => errors.push(error.message));
const check = name => { assertions.push(name); console.log(`PASS ${name}`); };
const labels = {
  vi: { source: 'Nguồn tài nguyên', add: '+ Thêm tài nguyên', save: 'Lưu draft Workload', alias: 'Alias tài nguyên', type: 'Loại tài nguyên', image: 'Ảnh Container', name: 'Tên Workload', database: 'Tham số database', username: 'Tham số username', advanced: 'Hiện nâng cao tài nguyên db', private: 'Chỉ dùng cho workload này' },
  en: { source: 'Resource source', add: '+ Add resource', save: 'Save Workload draft', alias: 'Resource alias', type: 'Resource type', image: 'Container image', name: 'Workload name', database: 'Parameter database', username: 'Parameter username' },
};
async function createApp(name) {
  await page.goto(`${config.baseURL}/ui/applications`);
  await page.getByRole('button', { name: '+ Tạo ứng dụng', exact: true }).first().click();
  await page.getByLabel('Tên ứng dụng', { exact: true }).fill(name);
  await page.getByLabel('Subdomain', { exact: true }).fill(name);
  await page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }).click();
  await expect(page.getByRole('heading', { name, exact: true })).toBeVisible();
  return new URL(page.url()).pathname.split('/').at(-1);
}
async function editor(app, id = 'new', env = 'staging') {
  await page.goto(`${config.baseURL}/ui/applications/${app}/environments/${env}/workloads/${id}`);
}
async function save(l) {
  const reply = page.waitForResponse(r => r.request().method() === 'PUT' && /\/workloads\//.test(r.url()));
  await page.getByRole('button', { name: l.save, exact: true }).click();
  expect((await reply).status()).toBe(200);
}
async function list(app, env = 'staging') {
  const response = await context.request.get(`${config.baseURL}/api/v1/applications/${app}/environments/${env}/workloads`);
  expect(response.status()).toBe(200);
  return (await response.json()).workloads;
}
try {
  const run = `shared-${Date.now()}`;
  const app = await createApp(run);
  const vi = labels.vi;
  await editor(app);
  await page.getByLabel(vi.name, { exact: true }).fill('api');
  await page.getByLabel(vi.image, { exact: true }).fill('example.invalid/api:v1');
  await page.getByRole('button', { name: vi.add, exact: true }).click();
  await page.getByLabel(vi.alias, { exact: true }).fill('db');
  await page.getByLabel(vi.type, { exact: true }).selectOption('postgres');
  await page.getByLabel(vi.database, { exact: true }).fill('appdb');
  await page.getByLabel(vi.username, { exact: true }).fill('appuser');
  await expect(page.getByLabel(/ID dùng chung|Shared ID/)).toHaveCount(0);
  await save(vi);
  const db = (await list(app)).find(w => w.id === 'api').score.resources.db;
  expect(db.id).toMatch(/^resource-/);
  check('New resource automatically shared, no manual ID');

  await editor(app);
  await page.getByLabel(vi.name, { exact: true }).fill('worker');
  await page.getByLabel(vi.image, { exact: true }).fill('example.invalid/worker:v1');
  await page.getByRole('button', { name: vi.add, exact: true }).click();
  await page.getByLabel(vi.alias, { exact: true }).fill('store');
  await page.getByLabel(vi.source, { exact: true }).selectOption(`shared:${db.id}`);
  await expect(page.getByLabel(vi.type, { exact: true })).toBeDisabled();
  await expect(page.getByLabel(vi.database, { exact: true })).toBeDisabled();
  await save(vi);
  expect((await list(app)).find(w => w.id === 'worker').score.resources.store).toEqual(db);
  check('Worker reuses API database with a different local alias');

  for (const locale of ['vi', 'en']) {
    await page.evaluate(locale => localStorage.setItem('orchestrator.locale', locale), locale);
    const l = labels[locale];
    for (const [width, height] of [[1512, 980], [1280, 900]]) {
      await page.setViewportSize({ width, height });
      await editor(app, 'worker');
      await page.getByLabel(l.image, { exact: true }).waitFor();
      await expect(page.getByLabel(/ID dùng chung|Shared ID/)).toHaveCount(0);
      await expect(page.getByLabel(l.source).locator('optgroup option')).toHaveCount(1);
      for (const collapsed of [false, true]) {
        await page.evaluate(collapsed => localStorage.setItem('orchestrator.sidebar.collapsed', String(collapsed)), collapsed);
        await page.reload();
        await page.getByLabel(l.image, { exact: true }).waitFor();
        expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
        await page.getByLabel(l.source).scrollIntoViewIfNeeded();
        await page.screenshot({ path: `${evidence}/${locale}-${width}-${collapsed ? 'collapsed' : 'expanded'}.png` });
      }
    }
  }
  check('VI/EN, both desktop sizes and sidebar states have no page overflow');
  await page.evaluate(() => localStorage.setItem('orchestrator.locale', 'vi'));
  await editor(app, 'api');
  await page.getByLabel(vi.image, { exact: true }).fill('example.invalid/api:v2');
  await save(vi);
  expect((await list(app)).find(w => w.id === 'api').score.resources.db).toEqual(db);
  check('Edit and save preserve original shared identity');

  await editor(app, 'new', 'production');
  await page.getByRole('button', { name: vi.add, exact: true }).click();
  await expect(page.getByLabel(vi.source).locator('optgroup option')).toHaveCount(0);
  const other = await createApp(`${run}-other`);
  await editor(other);
  await page.getByRole('button', { name: vi.add, exact: true }).click();
  await expect(page.getByLabel(vi.source).locator('optgroup option')).toHaveCount(0);
  check('Other app and production cannot select staging resources');

  await page.getByLabel(vi.name, { exact: true }).fill('private');
  await page.getByLabel(vi.image, { exact: true }).fill('example.invalid/private:v1');
  await page.getByLabel(vi.alias, { exact: true }).fill('db');
  await page.getByLabel(vi.type, { exact: true }).selectOption('postgres');
  await page.getByLabel(vi.database, { exact: true }).fill('private');
  await page.getByLabel(vi.username, { exact: true }).fill('appuser');
  await page.getByRole('button', { name: vi.advanced, exact: true }).click();
  await page.getByLabel(vi.private, { exact: true }).check();
  await save(vi);
  expect((await list(other))[0].score.resources.db).not.toHaveProperty('id');
  check('Advanced private scope omits shared ID');
  expect(errors).toEqual([]);
  await writeFile(`${evidence}/assertions.json`, JSON.stringify({ assertions, errors }, null, 2));
} finally {
  await closeConsole(consoleSession, config);
}
