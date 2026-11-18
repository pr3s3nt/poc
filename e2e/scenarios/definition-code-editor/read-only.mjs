import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { openConsole, readConsoleConfig, closeConsole } from '../../lib/console.mjs';
const { expect } = createRequire(new URL('../../../scripts/bootstrap/package.json', import.meta.url))('@playwright/test');
const { stringify } = createRequire(new URL('../../../frontend/package.json', import.meta.url))('yaml');
const config = readConsoleConfig();
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.') + '/'));
const keys = process.env.ORCH_DEFINITION_KEYS?.split(','); assert(keys?.length && keys.every(Boolean));
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const labels = JSON.parse(readFileSync(new URL('../../../frontend/public/locales/vi.json', import.meta.url)));
const session = await openConsole(config); const { page } = session;
const get = async path => { const response = await page.request.get(config.baseURL + '/api/v1' + path); assert(response.ok(), path); return response.json(); };
const result = { checked: [] };
const save = () => writeFileSync(join(evidence, 'read-only.json'), JSON.stringify(result, null, 2), { mode: 0o600 });
try {
 const before = (await get('/resource-definitions')).resourceDefinitions;
 const applications = (await get('/applications')).applications;
 for (const key of keys) {
  const stored = before.find(d => d.key === key); assert(stored);
  await page.goto(config.baseURL + '/ui/platform/resource-definitions');
  await page.locator(`[data-definition="${key}"]`).waitFor({ state: 'visible' });
  await page.getByRole('button', { name: labels['platform.resourceDefinitions.editDefinitionKey'].replace('{key}', key), exact: true }).click();
  const code = page.getByRole('tab', { name: 'YAML/JSON', exact: true });
  const form = page.getByRole('tab', { name: 'Biểu mẫu', exact: true });
  const text = page.getByRole('textbox', { name: 'Nội dung YAML/JSON', exact: true });
  await code.click(); await expect(code).toHaveAttribute('aria-selected', 'true');
  const doc = JSON.parse(await text.inputValue()); assert.equal(doc.key, key); assert.equal(doc.resourceType, stored.resourceType);
  assert.deepEqual(doc.driverInputs, stored.driverInputs); assert.equal(doc.driverType, stored.driverType);
  assert.equal(doc.driverAccount, stored.driverAccount); assert.deepEqual(doc.criteria, stored.criteria);
  assert.deepEqual(doc.provision ?? {}, stored.provision ?? {});
  await text.fill(stringify(doc)); await form.click();
  await expect(form).toHaveAttribute('aria-selected', 'true', { timeout: 120000 });
  await code.click(); assert.deepEqual(JSON.parse(await text.inputValue()), doc);
  await page.screenshot({ path: join(evidence, key + '-roundtrip.png'), fullPage: true });
  result.checked.push({ key, driver: stored.driverType, version: stored.version }); save();
 }
 assert.deepEqual((await get('/resource-definitions')).resourceDefinitions, before);
 assert.deepEqual((await get('/applications')).applications, applications);
 result.passed = true; result.at = new Date().toISOString(); save();
 console.log('PASS: existing Definitions form↔YAML↔JSON round-trip without Save; exact catalog/application snapshots unchanged');
} finally { await closeConsole(session, config); }
