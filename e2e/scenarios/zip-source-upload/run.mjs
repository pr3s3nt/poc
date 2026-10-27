import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { cpSync, mkdirSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect } from '@playwright/test';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';

// This scenario writes immutable catalog records. Use only a disposable local
// platform; destroying that owned platform is the cleanup boundary.
const config = readConsoleConfig();
assert.equal(process.env.ORCH_ZIP_DISPOSABLE, '1', 'Requires ORCH_ZIP_DISPOSABLE=1');
assert.ok(['127.0.0.1', 'localhost', '[::1]'].includes(new URL(config.baseURL).hostname), 'Requires a loopback disposable platform');
const root = fileURLToPath(new URL('../../../', import.meta.url));
const work = mkdtempSync(join(tmpdir(), 'zip-source-e2e-'));
const runID = `zip-${Date.now()}`;
const evidence = process.env.ORCH_ZIP_EVIDENCE;
let session;
try {
  mkdirSync(join(work, 'modules'), { recursive: true });
  mkdirSync(join(work, 'bundles'), { recursive: true });
  cpSync(join(root, 'backend/internal/adapters/terraform/modules/k8s-namespace'), join(work, 'modules/namespace'), { recursive: true });
  cpSync(join(root, 'backend/render-bundles/standard'), join(work, 'bundles/standard'), { recursive: true });
  const archive = join(work, 'source.zip');
  execFileSync('zip', ['-q', '-r', archive, 'modules', 'bundles'], { cwd: work });
  session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
  const { page } = session;
  const gitInspections = [];
  page.on('request', (req) => { if (/\/(render-bundles|terraform-modules)\/inspect$/.test(new URL(req.url()).pathname)) gitInspections.push(req.url()); });
  const upload = async (path, endpoint) => {
    await page.getByRole('button', { name: 'Tải ZIP', exact: true }).click();
    await page.getByLabel('Tệp ZIP', { exact: true }).setInputFiles(archive);
    await page.getByLabel('Thư mục trong ZIP', { exact: true }).fill(path);
    const pending = page.waitForResponse((r) => new URL(r.url()).pathname === `/api/v1/${endpoint}/upload`);
    await page.getByRole('button', { name: 'Tải lên và kiểm tra', exact: true }).click();
    const response = await pending;
    assert.equal(response.status(), 200, `${endpoint} upload`);
    await expect(page.getByText('Đã xác minh ZIP.', { exact: true })).toBeVisible();
    return response.json();
  };
  const screenshots = async (name) => {
    if (!evidence) return;
    mkdirSync(evidence, { recursive: true });
    for (const width of [1512, 1280]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.screenshot({ path: join(evidence, `${name}-${width}.png`), fullPage: true });
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, `${name} overflows at ${width}`);
    }
  };
  await page.goto(`${config.baseURL}/ui/platform/resource-definitions`);
  await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  await page.getByRole('combobox', { name: /^Loại tài nguyên/ }).selectOption('k8s-namespace');
  const { capability } = await upload('modules/namespace', 'terraform-modules');
  assert.ok(capability.source.sourceId.startsWith('zip-'));
  assert.match(capability.source.revision, /^[a-f0-9]{64}$/);
  assert.equal(capability.source.subdirectory, 'modules/namespace');
  await screenshots('module-upload');
  const moduleList = await page.request.get(`${config.baseURL}/api/v1/resource-definition-capabilities`);
  assert.ok((await moduleList.json()).capabilities.some((item) => item.source?.sourceId === capability.source.sourceId));
  await page.goto(`${config.baseURL}/ui/platform/rendering-templates`);
  const { renderBundle } = await upload('bundles/standard', 'render-bundles');
  assert.equal(renderBundle.source.kind, 'zip');
  assert.equal(renderBundle.source.url, `zip:sha256:${renderBundle.source.revision}`);
  await page.getByLabel('ID mẫu', { exact: true }).fill(runID);
  await page.getByLabel('Tên hiển thị', { exact: true }).fill('ZIP source E2E');
  await page.getByRole('button', { name: 'Dùng gợi ý namespace', exact: true }).click();
  await page.getByRole('button', { name: 'Dùng gợi ý cluster', exact: true }).click();
  await screenshots('bundle-upload');
  const saved = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/resource-definitions');
  await page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }).click();
  assert.equal((await saved).status(), 201, 'save uploaded bundle Definition');
  await page.reload();
  const row = page.locator('.catalog-entry').filter({ hasText: runID });
  await expect(row).toBeVisible();
  await row.getByRole('button', { name: `Sửa mẫu ${runID}`, exact: true }).click();
  const detail = await page.request.get(`${config.baseURL}/api/v1/render-bundles/${renderBundle.id}`);
  assert.deepEqual((await detail.json()).renderBundle.source, renderBundle.source);
  assert.equal(gitInspections.length, 0, 'ZIP source was sent to Git inspection');
  console.log('PASS: browser ZIP module/bundle upload, real inspection, catalog pins, template save/reopen, no Git inspection; no deployment');
} catch (error) {
  if (session && evidence) { mkdirSync(evidence, { recursive: true }); await session.page.screenshot({ path: join(evidence, `failure-${runID}.png`), fullPage: true }).catch(() => {}); }
  throw error;
} finally {
  if (session) await closeConsole(session, config);
  rmSync(work, { recursive: true, force: true });
}
