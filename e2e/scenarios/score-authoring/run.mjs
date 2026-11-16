import { createRequire } from 'node:module';
// Use the same Playwright instance as the shared Console helper.
const { expect } = createRequire(new URL('../../../scripts/bootstrap/package.json', import.meta.url))('@playwright/test');
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { openConsole, readConsoleConfig, closeConsole } from '../../lib/console.mjs';

const config = readConsoleConfig();
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.') + '/'), 'Evidence must be outside Git');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const receiptFile = join(evidence, 'receipt.json');
const receipt = existsSync(receiptFile) ? JSON.parse(readFileSync(receiptFile, 'utf8')) : { name: 'score83-' + Date.now().toString(36) };
const save = () => writeFileSync(receiptFile, JSON.stringify(receipt, null, 2), { mode: 0o600 });
const phase = process.env.ORCH_SCORE_PHASE ?? 'author';
const session = await openConsole(config, {});
const { page } = session;
page.setDefaultTimeout(30_000);
const api = async (path, data) => {
 const response = data === undefined ? await page.request.get(config.baseURL + '/api/v1' + path) : await page.request.post(config.baseURL + '/api/v1' + path, { data, timeout: 600_000 });
 if (!response.ok()) throw new Error(`HTTP ${response.status()} ${path}: ${(await response.json()).code ?? 'unknown'}`);
 return response.json();
};
const fixture = name => fileURLToPath(new URL(name, import.meta.url));
try {
 if (phase === 'author') {
  if (!receipt.appId) {
  await page.goto(config.baseURL + '/ui/applications/new');
  await page.getByLabel('Tên ứng dụng', { exact: true }).fill(receipt.name);
  const creation = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/applications');
  await page.getByRole('button', { name: 'Tạo ứng dụng', exact: true }).click();
  const response = await creation;
  assert(response.ok(), 'Application create failed');
  const created = await response.json();
  receipt.appId = created.application.key ?? created.application.id;
  assert(receipt.appId); save();
  } else {
   const existing = await api(`/applications/${receipt.appId}`);
   assert.equal(existing.application.name, receipt.name, 'Receipt does not own this Application');
  }
  const base = `/applications/${receipt.appId}/environments/staging`;
  for (const [name, workload] of [['deployment', 'database-client'], ['cronjob', 'scheduled-report']]) {
   if (receipt.saved?.includes(workload)) continue;
   await page.goto(config.baseURL + '/ui' + base + '/workloads/new');
   await page.getByRole('heading', { name: 'Thông tin cơ bản' }).waitFor();
   if (name === 'deployment') await page.screenshot({ path: join(evidence, 'form-reference.png'), fullPage: true });
   await page.getByRole('button', { name: 'Nhập Workload', exact: true }).click();
   await page.getByLabel('File extension (tùy chọn)', { exact: true }).setInputFiles(fixture(`${name}.extension.yaml`));
   const parsed = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/workloads/parse'));
   await page.getByLabel('File Workload', { exact: true }).setInputFiles(fixture(`${name}.yaml`));
   const parseResponse = await parsed;
   if (!parseResponse.ok()) throw new Error('Score import HTTP ' + parseResponse.status() + ' ' + (await parseResponse.json()).code);
   const normalized = (await parseResponse.json()).workload;
   assert.equal(normalized.metadata.name, workload);
   assert.equal(normalized.workloadExecution.template.id, name === 'deployment' ? 'backend' : 'scheduled');
   assert.match(normalized.workloadExecution.template.sha256, /^[0-9a-f]{64}$/);
   if (name === 'deployment') {
    assert.equal(normalized.resources.db.id, 'score-db');
    assert.equal(normalized.containers.main.variables.PGHOST, '${resources.db.host}');
   } else assert.equal(normalized.workloadExecution.schedule.expression, '* * * * *');
   await expect(page.getByRole('button', { name: 'Lưu draft Workload', exact: true })).toBeEnabled();
   const downloads = [];
   const download = async d => { const path = join(evidence, name + '-' + d.suggestedFilename()); await d.saveAs(path); downloads.push({ name: d.suggestedFilename(), path }); };
   page.on('download', download);
   await page.getByRole('button', { name: 'Tải Score + extension', exact: true }).click();
   await expect.poll(() => downloads.length).toBe(2);
   page.off('download', download);
   const roundtrip = await api(base + '/workloads/parse', { content: readFileSync(downloads.find(x => x.name === 'score.yaml').path, 'utf8'), extensions: readFileSync(downloads.find(x => x.name === 'humanitec.score.yaml').path, 'utf8') });
   assert.deepEqual(roundtrip.workload, normalized, 'Export/reimport changed workload intent or template pin');
   await page.screenshot({ path: join(evidence, name + '-import.png'), fullPage: true });
   const saved = page.waitForResponse(r => r.request().method() === 'PUT' && r.url().endsWith('/workloads/' + workload));
   await page.getByRole('button', { name: 'Lưu draft Workload', exact: true }).click();
   assert((await saved).ok(), 'Save draft failed');
   receipt.saved = [...(receipt.saved ?? []), workload]; save();
  }
  const workloads = await api(base + '/workloads');
  assert.equal(workloads.workloads.length, 2);
  assert(workloads.workloads.every(x => x.state === 'PENDING_UPSERT'));
  receipt.allSaved = true; save();
  console.log('PASS: real Console Score + extension import, pinned profiles, export/reimport and two saved drafts');
 } else if (phase === 'deploy') {
  assert(receipt.allSaved && !receipt.deployStarted, 'Require saved drafts and no prior deployment submission');
  const base = `/applications/${receipt.appId}/environments/staging`;
  await page.goto(config.baseURL + '/ui/applications/' + receipt.appId);
  const preview = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/environments/staging/preview'), { timeout: 180_000 });
  await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
  const response = await preview;
  if (!response.ok()) throw new Error('Preview HTTP ' + response.status() + ' ' + (await response.json()).code);
  const body = await response.json(); assert.deepEqual(body.changes.map(x => x.workloadId).sort(), ['database-client', 'scheduled-report']);
  receipt.preview = { workloads: body.changes.map(x => x.workloadId), tokenHashPresent: !!body.token }; save();
  await page.screenshot({ path: join(evidence, 'preview.png'), fullPage: true });
  const deployment = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/environments/staging/deploy'), { timeout: 600_000 });
  receipt.deployStarted = true; save();
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  const deployed = await deployment;
  receipt.deployHTTP = deployed.status();
  const report = await deployed.json(); receipt.report = report; save();
  assert(deployed.ok() && report.status === 'SUCCEEDED', 'Environment deployment did not succeed');
  await page.screenshot({ path: join(evidence, 'deployed.png'), fullPage: true });
  const workloads = await api(base + '/workloads');
  assert.equal(workloads.workloads.length, 2);
  assert(workloads.workloads.every(x => x.ready && !x.state));
  receipt.deployed = true; save();
  console.log('PASS: one Preview and one Environment Deploy for both Score workloads');
 } else throw new Error('Unknown ORCH_SCORE_PHASE');
} catch (error) {
 receipt.lastError = error.message; save();
 await page.screenshot({ path: join(evidence, phase + '-failure.png'), fullPage: true }).catch(() => {});
 throw error;
} finally { await closeConsole(session, config); }
