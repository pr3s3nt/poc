import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';

const config = readConsoleConfig();
const evidence = process.env.ORCH_EVIDENCE_DIR;
let session;
try {
  session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
  const { page } = session;
  const errors = [];
  page.on('pageerror', e => errors.push(e.message));
  await page.addInitScript(() => localStorage.setItem('orchestrator.locale', 'vi'));
  const get = async endpoint => {
    const response = await page.request.get(`${config.baseURL}/api/v1${endpoint}`);
    assert.equal(response.status(), 200, endpoint);
    return response.json();
  };
  const beforeApps = await get('/applications');
  const beforeDefs = await get('/resource-definitions');
  const { sources } = await get('/platform/catalog/sources');
  let choice;
  for (const source of sources) {
    const { releases } = await get(`/platform/catalog/sources/${encodeURIComponent(source.id)}/releases`);
    for (const release of releases) {
      const detail = await get(`/platform/catalog/sources/${encodeURIComponent(source.id)}/releases/${encodeURIComponent(release.tag)}`);
      const component = detail.release.components.find(c => c.kind === 'terraform-module' && c.resourceType === 'postgres') ?? detail.release.components.find(c => c.kind === 'terraform-module');
      if (component) { choice = { source: source.id, tag: release.tag, type: component.resourceType, component: component.id }; break; }
    }
    if (choice) break;
  }
  assert(choice, 'A published Catalog with a Terraform module is required');
  const modules = await get(`/platform/catalog/sources/${encodeURIComponent(choice.source)}/releases/${encodeURIComponent(choice.tag)}/modules?resourceType=${encodeURIComponent(choice.type)}`);
  const selected = modules.modules.find(m => m.componentId === choice.component);
  assert(selected?.capability.source, 'Expected a verified immutable module pin');
  const route = `${config.baseURL}/ui/platform/resource-definitions?resourceType=${encodeURIComponent(choice.type)}`;
  await page.goto(route);
  await page.getByRole('combobox', { name: 'Loại tài nguyên', exact: true }).waitFor();
  await page.getByRole('button', { name: 'Catalog', exact: true }).click();
  const sourceField = page.getByRole('combobox', { name: 'Nguồn Catalog', exact: true });
  await sourceField.selectOption(choice.source);
  const releaseField = page.getByRole('combobox', { name: 'Phiên bản Catalog', exact: true });
  assert.equal(await releaseField.inputValue(), '', 'The UI must not choose a release automatically');
  await releaseField.selectOption(choice.tag);
  await page.getByRole('combobox', { name: 'Module trong Catalog', exact: true }).selectOption(choice.component);
  await page.getByLabel('Pin module đã kiểm chứng', { exact: true }).waitFor({ state: 'attached' });
  await page.getByText('Chi tiết kỹ thuật module', { exact: true }).click();
  assert((await page.getByLabel('Pin module đã kiểm chứng', { exact: true }).innerText()).includes(selected.capability.source.revision));
  await page.getByRole('tab', { name: /^Điều kiện áp dụng/ }).click();
  await page.getByRole('tab', { name: 'Cấu hình', exact: true }).click();
  assert.equal(await page.getByRole('combobox', { name: 'Module trong Catalog', exact: true }).inputValue(), choice.component);
  if (evidence) {
    await mkdir(evidence, { recursive: true });
    await page.screenshot({ path: path.join(evidence, 'definition-catalog.png'), fullPage: true });
  }
  await page.getByRole('button', { name: 'Thêm từ Git', exact: true }).click();
  await page.getByLabel('Git HTTPS URL', { exact: true }).waitFor();
  await page.getByRole('button', { name: 'Tải ZIP', exact: true }).click();
  await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
  await page.getByRole('link', { name: 'Mẫu dựng ứng dụng', exact: true }).click();
  await page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', exact: true }).waitFor();
  assert.deepEqual(await get('/applications'), beforeApps, 'Read-only checks changed Application state');
  assert.deepEqual(await get('/resource-definitions'), beforeDefs, 'Draft inspection saved a Definition');
  assert.deepEqual(errors, [], 'Browser runtime errors');
  const summary = { url: route, choice, modulePin: selected.capability.source, assertions: 'Catalog selection, explicit release, verified pin, criteria tab, source switches, rendering route, unchanged stored records', applicationCount: beforeApps.applications.length, definitionCount: beforeDefs.resourceDefinitions.length, browserErrors: errors.length, saved: false, deployed: false };
  if (evidence) await writeFile(path.join(evidence, 'browser-summary.json'), JSON.stringify(summary, null, 2));
  console.log(JSON.stringify(summary, null, 2));
} catch (error) {
  if (session && evidence) { await mkdir(evidence, { recursive: true }); await session.page.screenshot({ path: path.join(evidence, 'failure.png'), fullPage: true }); }
  throw error;
} finally { if (session) await closeConsole(session, config); }
