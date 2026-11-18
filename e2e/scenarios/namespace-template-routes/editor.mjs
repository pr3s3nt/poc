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
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const prefix = process.env.ORCH_ROUTE_DEFINITION_PREFIX;
const legacyPrefix = process.env.ORCH_ROUTE_LEGACY_PREFIX;
assert(prefix && legacyPrefix && prefix !== legacyPrefix, 'Explicit separate review and legacy prefixes required');
const labels = JSON.parse(readFileSync(new URL('../../../frontend/public/locales/vi.json', import.meta.url)));
const session = await openConsole(config); const { page } = session;
const result = { checked: [], consoleURL: config.baseURL + '/ui/platform/resource-definitions' };
const save = () => writeFileSync(join(evidence, 'editor.json'), JSON.stringify(result, null, 2), { mode: 0o600 });
const get = async path => { const response = await page.request.get(config.baseURL + '/api/v1' + path); assert(response.ok(), path); return response.json(); };
const code = () => page.getByRole('tab', { name: 'YAML/JSON', exact: true });
const form = () => page.getByRole('tab', { name: 'Biểu mẫu', exact: true });
const text = () => page.getByRole('textbox', { name: 'Nội dung YAML/JSON', exact: true });
let deployments = 0;
page.on('request', request => { if (request.method() !== 'GET' && /\/deploy(?:ments)?(?:\/|$)/.test(new URL(request.url()).pathname)) deployments++; });
try {
 const applications = (await get('/applications')).applications;
 const allBefore = (await get('/resource-definitions')).resourceDefinitions;
 for (const [scope, editable] of [[legacyPrefix, false], [prefix, true]]) {
  for (const [suffix, label, field, name] of [['gateway', 'Nhóm cluster', 'cluster', 'HTTPRoute Gateway Echo'], ['endpoint', 'Nhóm Gateway', 'gateway', 'HTTPRoute Endpoint qua Gateway']]) {
   const key = scope + '-' + suffix;
   const before = allBefore.find(d => d.key === key); assert(before);
   assert.equal(before.driverType, suffix === 'endpoint' ? 'endpoint' : editable ? 'echo' : 'existing-gateway');
   const history = (await get('/resource-definitions/' + key + '/versions')).history;
   await page.goto(result.consoleURL);
   await page.locator(`[data-definition="${key}"]`).waitFor({ state: 'visible' });
   await page.getByRole('button', { name: labels['platform.resourceDefinitions.editDefinitionKey'].replace('{key}', key), exact: true }).click();
   const reference = (before.driverType === 'echo' ? before.driverInputs.values.variables.gateway : before.driverInputs.values.variables)[field];
   const className = /^\$\{resources\['[^.]+\.([^#]+)#environments\.@app\.@env'\]\.outputs\.[^}]+\}$/.exec(reference)?.[1]; assert(className);
   await expect(page.getByRole('combobox', { name: label, exact: true })).toHaveValue(className);
   if (editable) await page.getByLabel('Tên hiển thị', { exact: true }).fill(name + ' – UI');
   await code().click(); await expect(code()).toHaveAttribute('aria-selected', 'true');
   const generated = JSON.parse(await text().inputValue()); assert(!('version' in generated));
   assert.deepEqual(generated.driverInputs, before.driverInputs);
   assert.equal(generated.driverType, before.driverType);
   const box = await text().boundingBox(), panel = await page.getByRole('tabpanel', { name: 'YAML/JSON', exact: true }).boundingBox();
   assert(box && panel && box.width > panel.width * 0.8);
   if (editable) assert.equal(generated.name, name + ' – UI');
   await text().fill(stringify({ ...generated, name: editable ? name : before.name }));
   await form().click(); await expect(form()).toHaveAttribute('aria-selected', 'true', { timeout: 120000 });
   await expect(page.getByLabel('Tên hiển thị', { exact: true })).toHaveValue(editable ? name : before.name);
   await expect(page.getByRole('combobox', { name: label, exact: true })).toHaveValue(className);
   await page.screenshot({ path: join(evidence, key + '-form.png'), fullPage: true });
   if (editable) {
    const response = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1/resource-definitions/' + key);
    await page.getByRole('button', { name: 'Lưu thay đổi', exact: true }).click(); const saved = await response; assert(saved.ok());
    const first = await saved.json(); assert.equal(first.version, before.version + 1);
    await expect(page.getByRole('button', { name: 'Lưu thay đổi', exact: true })).toBeEnabled();
    await code().click(); const again = JSON.parse(await text().inputValue()); assert.equal(again.name, name);
    await text().fill(JSON.stringify(again, null, 2));
    const secondResponse = page.waitForResponse(r => r.request().method() === 'PUT' && new URL(r.url()).pathname === '/api/v1/resource-definitions/' + key);
    await page.getByRole('button', { name: 'Lưu thay đổi', exact: true }).click(); const reply = await secondResponse; assert(reply.ok());
    const second = await reply.json(); assert.equal(second.version, before.version + 2);
    assert.deepEqual(second.driverInputs, before.driverInputs); assert.equal(second.driverType, before.driverType);
    assert.equal(second.driverAccount, before.driverAccount); assert.deepEqual(second.criteria, before.criteria);
    assert.deepEqual(second.provision ?? {}, before.provision ?? {});
    const afterHistory = (await get('/resource-definitions/' + key + '/versions')).history;
    assert.equal(afterHistory.activeId, history.activeId); assert.equal(afterHistory.versions.length, history.versions.length + 2);
   } else {
    assert.deepEqual((await get('/resource-definitions')).resourceDefinitions.find(d => d.key === key), before);
    await code().click();
   }
   await page.screenshot({ path: join(evidence, key + '-code.png'), fullPage: true });
   result.checked.push({ key, driver: before.driverType, saved: editable, originalVersion: before.version, activeRevisionUnchanged: true }); save();
  }
 }
 const after = (await get('/resource-definitions')).resourceDefinitions;
 assert.equal(after.length, allBefore.length, 'Unexpected new Definitions');
 for (const before of allBefore.filter(d => ![prefix + '-gateway', prefix + '-endpoint'].includes(d.key))) assert.deepEqual(after.find(d => d.key === before.key), before);
 assert.deepEqual((await get('/applications')).applications, applications); assert.equal(deployments, 0);
 result.passed = true; result.at = new Date().toISOString(); save();
 console.log('PASS: integrated Echo Gateway/Endpoint form↔YAML/JSON saves; legacy round-trip; unchanged pins, active revisions and other Definitions; no deployment');
} finally { await closeConsole(session, config); }
