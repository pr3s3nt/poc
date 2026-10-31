import assert from 'node:assert/strict';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';

// This runner changes only the explicitly named Environment and retains the
// resulting release. It never registers sources, deletes resources or rolls back.
function required(name) {
  const value = (process.env[name] ?? '').trim();
  if (!value) throw new Error(`Missing ${name}`);
  return value;
}
const app = required('ORCH_CATALOG_APP');
const source = required('ORCH_CATALOG_SOURCE');
const from = required('ORCH_CATALOG_FROM');
const to = required('ORCH_CATALOG_TO');
if (process.env.ORCH_CATALOG_DEPLOY !== '1') throw new Error('ORCH_CATALOG_DEPLOY=1 requires an explicitly authorized isolated deployment');
if (from === to) throw new Error('Source and target versions must differ');
const config = readConsoleConfig();
let session;
try {
  session = await openConsole(config);
  const { page } = session;
  const base = `/api/v1/applications/${encodeURIComponent(app)}/environments`;
  async function read(path) {
    const response = await page.request.get(`${config.baseURL}${path}`);
    assert.equal(response.status(), 200, 'Scoped verification read failed');
    return response.json();
  }
  const staging = await read(`${base}/staging/catalog`);
  const production = await read(`${base}/production/catalog`);
  assert.equal(staging.catalog.applied?.sourceId, source);
  assert.equal(staging.catalog.applied?.tag, from);
  assert.equal(production.catalog.applied?.sourceId, source);
  assert.equal(production.catalog.applied?.tag, from);
  assert.ok(staging.releases.some(release => release.sourceId === source && release.tag === to), 'Target release is not ready');
  const beforeResources = await read(`${base}/staging/resources`);
  const originalIDs = (beforeResources.resources ?? []).map(resource => resource.id).filter(Boolean).sort();
  assert.ok(originalIDs.length, 'Prepare an occupied Environment to verify identity retention');
  await page.goto(`${config.baseURL}/ui/applications/${encodeURIComponent(app)}/settings?environment=staging`);
  const panel = page.getByRole('region', { name: 'Catalog', exact: true });
  await panel.getByRole('combobox').selectOption(JSON.stringify([source, to]));
  await panel.getByRole('button', { name: 'Lưu lựa chọn', exact: true }).click();
  await panel.getByRole('status').filter({ hasText: 'Đã lưu mục tiêu.' }).waitFor();
  const desired = await read(`${base}/staging/catalog`);
  assert.equal(desired.catalog.desired.tag, to);
  assert.equal(desired.catalog.applied.tag, from, 'Save must not activate the release');
  await panel.getByRole('button', { name: 'Xem thay đổi', exact: true }).click();
  const previewResponse = page.waitForResponse(response => response.url().endsWith('/staging/preview') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
  const preview = await (await previewResponse).json();
  assert.equal(preview.catalogPending, true);
  assert.ok(preview.catalogPin);
  const deployResponse = page.waitForResponse(response => response.url().endsWith('/staging/deploy') && response.request().method() === 'POST', { timeout: 45 * 60_000 });
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  const response = await deployResponse;
  assert.equal(response.status(), 200, 'Deploy did not return a successful report');
  const report = await response.json();
  assert.equal(report.status, 'SUCCEEDED', 'Partial/unknown outcomes require review; runner never blindly retries');
  const applied = await read(`${base}/staging/catalog`);
  assert.equal(applied.catalog.status, 'APPLIED');
  assert.equal(applied.catalog.applied.tag, to);
  const afterProduction = await read(`${base}/production/catalog`);
  assert.deepEqual(afterProduction.catalog, production.catalog, 'Staging upgrade changed production selection');
  const afterResources = await read(`${base}/staging/resources`);
  const currentIDs = new Set((afterResources.resources ?? []).map(resource => resource.id));
  assert.ok(originalIDs.every(id => currentIDs.has(id)), 'Existing resource identity was lost');
  console.log('PASS: staging catalog upgraded through UI Preview/Deploy; production pins and resource identities retained. Workload data checks remain application-specific.');
} finally {
  if (session) await closeConsole(session, config);
}
