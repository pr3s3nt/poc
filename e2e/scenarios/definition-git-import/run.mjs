import assert from 'node:assert/strict';
import { readFile, mkdir } from 'node:fs/promises';
import { parse, stringify } from 'yaml';
import { readConsoleConfig, openConsole, closeConsole } from '../../lib/console.mjs';

const c = readConsoleConfig();
const s = await openConsole(c, {});
const { page } = s;
page.setDefaultTimeout(120000);
const key = process.env.ORCH_IMPORT_PREFIX ?? `issue79-mongodb-${Date.now()}`;
const schema = {
  key: 'mongodb',
  inputs: [
    { name: 'database', type: 'string', required: true },
    { name: 'username', type: 'string', required: false },
    { name: 'storage', type: 'string', required: false },
  ],
  outputs: ['host', 'port', 'database', 'username', 'password', 'connection'].map(name => ({ name, type: name === 'port' ? 'number' : 'string', required: true, secret: ['password', 'connection'].includes(name) })),
};
const doc = {
  key, name: 'MongoDB Git file review', resourceType: 'mongodb', driverType: 'terraform',
  driverInputs: { values: {
    source: { url: 'https://github.com/pr3s3nt/orchestrator-workload-templates', ref: 'main', path: 'terraform/mongodb' },
    variables: {
      image: 'mongo:7.0@sha256:8bb4442761a63a8dda452744082cdac9644b429807eb977b42324a63cef59ca4',
      namespace: "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}",
      username: 'app', storage: '10Gi', storage_class: '', image_pull_secret_name: '', root_username: 'admin',
    },
    input_mapping: Object.fromEntries(schema.inputs.map(f => [f.name, { param: f.name }])),
    output_mapping: Object.fromEntries(schema.outputs.map(f => [f.name, f.name])),
    provider_bindings: { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.bootstrap-dev#environments.@app.@env' } },
  } },
  criteria: [{ env_type: 'staging', class: 'in-cluster' }], provision: {},
};
const get = async path => {
  const response = await page.request.get(c.baseURL + '/api/v1' + path);
  assert(response.ok(), `GET ${path}: HTTP ${response.status()}`);
  return response.json();
};
const editor = () => page.getByLabel('Nội dung YAML/JSON', { exact: true });
const register = () => page.getByRole('tabpanel').filter({ has: editor() }).getByRole('button', { name: 'Đăng ký', exact: true });
async function check() {
  const responsePromise = page.waitForResponse(r => r.url().endsWith('/resource-definitions/import/prepare') && r.request().method() === 'POST');
  await page.getByRole('button', { name: 'Kiểm tra cấu hình', exact: true }).click();
  const response = await responsePromise;
  assert(response.ok(), `Prepare failed: HTTP ${response.status()}`);
  const prepared = await response.json();
  await page.getByRole('status').filter({ hasText: 'Cấu hình hợp lệ' }).waitFor();
  assert.match(prepared.source.revision, /^[a-f0-9]{40}$/);
  assert.equal(prepared.source.path, 'terraform/mongodb');
  await page.getByLabel('Nguồn đã lưu', { exact: true }).getByText(prepared.source.revision, { exact: true }).waitFor();
  return prepared;
}
async function download(button) {
  const [file] = await Promise.all([page.waitForEvent('download'), button.click()]);
  return readFile(await file.path(), 'utf8');
}
try {
  const types = (await get('/resource-types')).resourceTypes;
  const existing = types.find(t => t.key === schema.key);
  if (existing) {
    // Go omitempty drops false flags from reads; compare the same schema semantics.
    const inputs = existing.inputs.map(f => ({ name: f.name, type: f.type, required: f.required ?? false }));
    const outputs = existing.outputs.map(f => ({ name: f.name, type: f.type, required: f.required ?? false, secret: f.secret ?? false }));
    assert.deepEqual({ key: existing.key, inputs, outputs }, schema, 'Existing MongoDB Type differs; leave it untouched');
  } else {
    await page.goto(c.baseURL + '/ui/platform/resource-types');
    await page.getByRole('tab', { name: 'Nhập YAML/JSON', exact: true }).click();
    await editor().fill(stringify(schema)); await register().click();
    await page.getByRole('status').filter({ hasText: 'CREATED' }).waitFor();
  }
  await page.goto(c.baseURL + '/ui/platform/resource-definitions');
  await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  await page.getByRole('tab', { name: 'Nhập YAML/JSON', exact: true }).click();
  await page.getByLabel('Chọn file', { exact: true }).setInputFiles({ name: 'mongodb-definition.yaml', mimeType: 'application/yaml', buffer: Buffer.from(stringify(doc)) });
  assert(await register().isDisabled());
  const invalid = structuredClone(doc); delete invalid.driverInputs.values.output_mapping.connection;
  await editor().fill(stringify(invalid));
  await page.getByRole('button', { name: 'Kiểm tra cấu hình', exact: true }).click();
  await page.getByRole('alert').waitFor();
  assert.equal(await editor().inputValue(), stringify(invalid));
  assert(await register().isDisabled());
  await editor().fill(stringify(doc)); const prepared = await check();
  assert(!(await get('/resource-definitions')).resourceDefinitions.some(d => d.key === key), 'Check wrote a Definition');
  await editor().fill(stringify({ ...doc, name: 'Edited draft' }));
  assert(await register().isDisabled(), 'Edit did not invalidate reviewed pin');
  await editor().fill(stringify(doc)); await check(); await register().click();
  await page.getByRole('status').filter({ hasText: 'CREATED' }).waitFor();
  const saved = (await get('/resource-definitions')).resourceDefinitions.find(d => d.key === key);
  assert.equal(saved.driverInputs.values.source.revision, prepared.source.revision);
  assert.equal(saved.driverInputs.values.source.url, undefined);
  const history = await get(`/resource-definitions/${key}/versions`);
  await page.locator('.definition-breadcrumb button').click();
  const row = page.locator(`[data-definition="${key}"]`);
  const yaml = await download(row.getByRole('button', { name: 'Tải file · v1', exact: true }));
  const exported = parse(yaml);
  assert.deepEqual(exported.driverInputs.values.source, { url: doc.driverInputs.values.source.url, ref: prepared.source.revision, path: 'terraform/mongodb' });
  assert.equal(exported.version, undefined); assert.equal(exported.sourceFingerprint, undefined);
  await row.getByRole('button', { name: 'Phiên bản', exact: true }).click();
  const revisionYaml = await download(page.getByRole('button', { name: 'Tải file · r1', exact: true }));
  assert.deepEqual(parse(revisionYaml), exported);
  await page.getByRole('button', { name: 'Đóng', exact: true }).click();
  await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  await page.getByLabel('Chọn file', { exact: true }).setInputFiles({ name: 'roundtrip.yaml', mimeType: 'application/yaml', buffer: Buffer.from(yaml) });
  await check(); await register().click(); await page.getByRole('status').filter({ hasText: 'REUSED' }).waitFor();
  await editor().fill(stringify({ ...exported, name: 'Conflicting name' }));
  await check(); await register().click(); await page.getByRole('status').filter({ hasText: 'CONFLICT' }).waitFor();
  assert.deepEqual(await get(`/resource-definitions/${key}/versions`), history, 'Round-trip/conflict changed history');
  if (process.env.ORCH_EVIDENCE_DIR) {
    await mkdir(process.env.ORCH_EVIDENCE_DIR, { recursive: true });
    await page.screenshot({ path: process.env.ORCH_EVIDENCE_DIR + '/definition-git-import.png', fullPage: true });
  }
  console.log('PASS: real Git MongoDB file check/review/register, edit invalidation, no check write, portable current/revision downloads, REUSED/CONFLICT with unchanged history. Key:', key);
} finally { await closeConsole(s, c); }
