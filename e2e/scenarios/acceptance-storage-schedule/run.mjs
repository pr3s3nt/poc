import { parseArgs } from 'node:util';
import { createHash } from 'node:crypto';
import { chromium, expect } from '@playwright/test';

const { values } = parseArgs({ options: { url: { type: 'string' }, value: { type: 'string' }, 'run-id': { type: 'string' }, write: { type: 'boolean' }, 'wait-ms': { type: 'string', default: '180000' }, 'delay-ms': { type: 'string', default: '800' }, headless: { type: 'boolean' }, help: { type: 'boolean' } } });
if (values.help) {
  console.log('node e2e/scenarios/acceptance-storage-schedule/run.mjs --url http://localhost:18481 --run-id <scope> --value <public-test-value> [--write] [--headless] [--delay-ms 800] [--wait-ms 180000]');
  process.exit(0);
}
let browser;
try {
  const url = new URL(values.url);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || !['', '/'].includes(url.pathname)) throw new Error('url phải là origin acceptance app, không phải Console');
  if (!values.value?.trim() || Buffer.byteLength(values.value) > 4096 || !/^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/.test(values['run-id'] ?? '')) throw new Error('Cần --run-id và --value công khai (1..4096 bytes)');
  const wait = Number(values['wait-ms']), delay = Number(values['delay-ms']);
  if (!Number.isInteger(wait) || wait < 1000 || wait > 900000 || !Number.isInteger(delay) || delay < 0 || delay > 10000) throw new Error('wait-ms 1000..900000; delay-ms 0..10000');
  browser = await chromium.launch({ headless: values.headless ?? false });
  const page = await browser.newPage();
  const errors = []; page.on('pageerror', error => errors.push(error.message));
  await page.goto(url.origin);
  const before = await page.request.get(`${url.origin}/api/scheduled-runs`);
  if (!before.ok()) throw new Error('Không đọc được lịch sử CronJob');
  const baseline = await before.json();
  if (!baseline.configured || baseline.runId !== values['run-id'] || !Array.isArray(baseline.runs)) throw new Error('Run scope backend không khớp');
  const previous = new Set(baseline.runs.map(row => row.invocation));
  if (values.write) {
    const input = page.getByLabel('Test value', { exact: true });
    await input.scrollIntoViewIfNeeded(); await input.fill('');
    await input.pressSequentially(values.value, { delay: Math.min(45, Math.floor(delay / 20)) });
    await page.waitForTimeout(delay);
    await page.getByRole('button', { name: 'Write to volume', exact: true }).click();
  }
  const hash = createHash('sha256').update(values.value).digest('hex');
  await expect(page.locator('#storage-value')).toHaveText(values.value);
  await expect(page.locator('#storage-hash')).toHaveText(hash);
  await expect(page.locator('#schedule-run')).toHaveText(values['run-id']);
  console.log('PASS: stored test value and hash match; waiting for a new real scheduled receipt');
  // Observe actual scheduled execution; no Job creation or fake receipt.
  await expect.poll(async () => {
    await page.reload();
    const rows = await page.locator('#scheduled-runs tr[data-invocation]').evaluateAll(rows => rows.map(row => ({ invocation: row.getAttribute('data-invocation'), text: row.textContent })));
    return rows.some(row => !previous.has(row.invocation) && row.text.includes(hash));
  }, { timeout: wait, intervals: [3000], message: 'Chưa có receipt mới đúng hash; kiểm tra lịch/Job logs trên Console' }).toBe(true);
  await expect(page.locator('#storage-value')).toHaveText(values.value);
  await expect(page.locator('#storage-hash')).toHaveText(hash);
  if (errors.length) throw new Error('Browser có lỗi runtime');
  console.log('PASS: a new invocation committed the expected file hash to PostgreSQL');
  console.log('Limit: this observes the deployed app; use PVC/Pod UID evidence to prove Kubernetes retention.');
} catch (error) {
  console.error(`FAIL: ${error.message.split('\n')[0]}`);
  process.exitCode = 1;
} finally { if (browser) await browser.close(); }
