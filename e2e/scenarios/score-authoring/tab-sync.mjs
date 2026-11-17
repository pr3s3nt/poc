// Real Console/API acceptance for switching form <-> Score/extension.
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { parse, stringify } from 'yaml';
import { openConsole, readConsoleConfig, closeConsole } from '../../lib/console.mjs';
const { expect } = createRequire(new URL('../../../scripts/bootstrap/package.json', import.meta.url))('@playwright/test');
const config = readConsoleConfig(), evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && evidence !== resolve('.') && !evidence.startsWith(resolve('.') + '/'), 'Private evidence outside Git required');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const receipt = { name: 'score83sync-' + Date.now().toString(36), checks: [] };
const save = () => writeFileSync(join(evidence, 'receipt.json'), JSON.stringify(receipt, null, 2), { mode: 0o600 });
const session = await openConsole(config, {}), { page } = session;
page.setDefaultTimeout(30_000);
let failure;
try {
  await page.goto(config.baseURL + '/ui/applications/new');
  await page.getByLabel('Tên ứng dụng', { exact: true }).fill(receipt.name);
  const creation = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/applications');
  await page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }).click();
  const created = await creation; assert(created.ok()); const { application } = await created.json();
  receipt.appId = application.key ?? application.id; save();
  const base = config.baseURL + `/api/v1/applications/${receipt.appId}/environments/staging/workloads`;
  await page.goto(config.baseURL + `/ui/applications/${receipt.appId}/environments/staging/workloads/new`);
  const formTab = () => page.getByRole('button', { name: 'Nhập bằng form', exact: true });
  const yamlTab = () => page.getByRole('button', { name: 'Nhập Workload', exact: true });
  const score = () => page.getByRole('textbox', { name: 'Nội dung Score (YAML/JSON)', exact: true });
  const extension = () => page.getByRole('textbox', { name: 'Nội dung extension (tùy chọn)', exact: true });
  const saveButton = () => page.getByRole('button', { name: 'Lưu draft Workload', exact: true });
  const responseFor = suffix => page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith(suffix));
  let writes = 0;
  page.on('request', r => { if (r.method() === 'PUT' && r.url().includes('/workloads/')) writes++; });
  await yamlTab().click();
  await score().fill(readFileSync(new URL('./deployment.yaml', import.meta.url), 'utf8'));
  await extension().fill(readFileSync(new URL('./deployment.extension.yaml', import.meta.url), 'utf8'));
  // Returning to form performs validation; no separate Check is required.
  const firstParse = responseFor('/workloads/parse'); await formTab().click();
  const firstResponse = await firstParse; assert(firstResponse.ok()); const initial = (await firstResponse.json()).workload;
  await expect(page.getByLabel('Ảnh Container', { exact: true })).toHaveValue(initial.containers.main.image);
  const image = 'postgres:16.14-alpine';
  await page.getByLabel('Ảnh Container', { exact: true }).fill(image);
  const exported = responseFor('/workloads/export'); await yamlTab().click(); assert((await exported).ok());
  await expect(score()).not.toHaveValue('');
  const authored = parse(await score().inputValue()); assert.equal(authored.containers.main.image, image);
  assert.equal(authored.containers.main.variables.PGHOST, '${resources.db.host}');
  authored.containers.main.variables.TAB_SYNC = 'from-yaml';
  const extra = parse(await extension().inputValue());
  // Edit the representation emitted by export; duplicate native/Humanitec fields are rejected.
  if (Object.hasOwn(extra.spec.orchestrator ?? {}, 'replicas')) extra.spec.orchestrator.replicas = 2;
  else extra.spec.deployment = { ...extra.spec.deployment, replicas: 2 };
  await score().fill(stringify(authored)); await extension().fill(stringify(extra));
  await expect(saveButton()).toBeDisabled();
  const changedParse = responseFor('/workloads/parse'); await formTab().click(); const changedResponse = await changedParse; assert(changedResponse.ok());
  const expected = (await changedResponse.json()).workload;
  await expect(page.locator('input[value="from-yaml"]')).toBeVisible();
  assert.equal(expected.replicas, 2); assert.deepEqual(expected.workloadExecution.template, initial.workloadExecution.template);
  receipt.checks.push('unsaved form image -> Score and YAML variable/extension replica -> form; template pin and resource ref retained'); save();
  const nextExport = responseFor('/workloads/export'); await yamlTab().click(); assert((await nextExport).ok());
  const validScore = await score().inputValue(), validExtension = await extension().inputValue();
  await score().fill('apiVersion: ['); const invalidParse = responseFor('/workloads/parse'); await formTab().click();
  assert.equal((await invalidParse).status(), 400); await expect(page.getByRole('alert')).toBeVisible();
  await expect(score()).toHaveValue('apiVersion: ['); await expect(saveButton()).toBeDisabled();
  await expect(page.getByRole('button', { name: 'Tải Score + extension', exact: true })).toBeDisabled();
  await yamlTab().click(); await expect(score()).toHaveValue('apiVersion: ['); assert.equal(writes, 0);
  await score().fill(validScore); await extension().fill(validExtension);
  const fixedParse = responseFor('/workloads/parse'); await formTab().click(); assert((await fixedParse).ok());
  await page.getByRole('button', { name: '+ Thêm cổng', exact: true }).click();
  const unfinishedPort = page.getByLabel('Tên cổng Service', { exact: true });
  await unfinishedPort.fill('http'); await yamlTab().click();
  await expect(page.getByRole('alert')).toContainText('Hãy sửa tên và số cổng Service');
  await expect(unfinishedPort).toHaveValue('http');
  await unfinishedPort.locator('..').getByRole('button', { name: 'Xóa', exact: true }).click();
  receipt.checks.push('unfinished Service row is retained instead of being dropped during form-to-YAML conversion');
  const saved = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith('/workloads/database-client'));
  await saveButton().click(); assert((await saved).ok()); assert.equal(writes, 1);
  receipt.checks.push('invalid text stays on YAML, stale actions blocked, no implicit Save; explicit Save after correction'); save();
  await page.goto(config.baseURL + `/ui/applications/${receipt.appId}/environments/staging/workloads/database-client`);
  const reopened = responseFor('/workloads/export'); await yamlTab().click(); assert((await reopened).ok());
  const reparsed = await page.request.post(base + '/parse', { data: { content: await score().inputValue(), extensions: await extension().inputValue() } });
  assert(reparsed.ok()); assert.deepEqual((await reparsed.json()).workload, expected);
  await page.screenshot({ path: join(evidence, 'sync-vi.png'), fullPage: true });
  await page.locator('.language-select select').selectOption('en');
  const englishExport = responseFor('/workloads/export'); await page.getByRole('button', { name: 'Import Workload', exact: true }).click(); assert((await englishExport).ok());
  await expect(page.getByRole('textbox', { name: 'Score content (YAML/JSON)', exact: true })).not.toHaveValue('');
  const englishParse = responseFor('/workloads/parse'); const englishForm = page.getByRole('button', { name: 'Form input', exact: true }); await englishForm.focus(); await englishForm.press('Enter'); assert((await englishParse).ok());
  await expect(page.getByLabel('Container image', { exact: true })).toHaveValue(image);
  await page.screenshot({ path: join(evidence, 'sync-en.png'), fullPage: true });
  receipt.checks.push('saved workload reopens with equivalent Score/extension; VI/EN and keyboard tab activation');
  receipt.status = 'PASS'; save(); console.log('PASS synchronized form/Score/extension, errors, explicit Save/reopen, VI/EN and keyboard');
} catch (error) {
  failure = error; receipt.status = 'FAILED'; receipt.error = error.message; save();
  await page.screenshot({ path: join(evidence, 'failure.png'), fullPage: true }).catch(() => {});
} finally {
  if (receipt.appId) {
    try {
      const { application } = await (await page.request.get(config.baseURL + '/api/v1/applications/' + receipt.appId)).json(); assert.equal(application.name, receipt.name);
      const preview = await page.request.post(config.baseURL + `/api/v1/applications/${receipt.appId}/deletion-preview`, { data: {} }); assert(preview.ok()); const { preview: p } = await preview.json(); assert(!p.blocked && p.token);
      const removed = await page.request.post(config.baseURL + `/api/v1/applications/${receipt.appId}/deletions`, { data: { token: p.token, confirmationName: receipt.name } }); assert(removed.ok()); const { deletion } = await removed.json(); receipt.deletionId = deletion.operationId; save();
      await expect.poll(async () => { const response = await page.request.get(config.baseURL + `/api/v1/applications/${receipt.appId}/deletions/${deletion.operationId}`); assert(response.ok()); const { deletion: d } = await response.json(); receipt.cleanup = d.status; save(); return d.status; }, { timeout: 90_000, intervals: [1000] }).toBe('SUCCEEDED');
      console.log('PASS owned synchronization test Application cleaned; no workload deployment performed');
    } catch (error) { receipt.cleanupError = error.message; save(); failure ??= error; }
  }
  await closeConsole(session, config);
}
if (failure) throw failure;
