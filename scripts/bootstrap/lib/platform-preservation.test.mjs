import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { readPrivateFile } from './console.mjs';
import { readGitToken } from './git-access-config.mjs';
import { preserveConsoleVariables, differences } from './config-diff.mjs';
import { definitionAction } from './definition-action.mjs';
import { validateUpdatePayload } from './definition-update-ui.mjs';
import { loadTarget } from './target-config.mjs';
import { parseBundleConfig } from './render-bundle-config.mjs';
import { parseInfrastructure, desiredInfrastructureDefinition, validateVariableLiterals } from './infrastructure-config.mjs';

const fixture = () => {
  const desired = { key: 'db', name: 'Database', resourceType: 'mysql', driverType: 'terraform', criteria: [{}], driverInputs: { values: { source: { revision: 'new' }, variables: { namespace: 'ns', storage_class: 'ceph' } } } };
  const existing = structuredClone(desired); existing.version = 4;
  existing.driverInputs.values.source.revision = 'old';
  Object.assign(existing.driverInputs.values.variables, { image: 'mysql@sha256:old', enabled: false, count: 0, config: { nested: ['keep'] } });
  return { existing, desired };
};
test('Windows skips POSIX mode checks for every shared private file reader; Linux keeps enforcing them', () => {
  const dir = mkdtempSync(join(tmpdir(), 'orch-windows-'));
  try {
    const file = join(dir, 'credential'); writeFileSync(file, 'fixture-token', { mode: 0o644 });
    for (const what of ['mật khẩu', 'kubeconfig', 'Git token']) assert.equal(readPrivateFile(file, what, { platform: 'win32' }), 'fixture-token');
    assert.equal(readGitToken(file, { platform: 'win32' }), 'fixture-token');
    assert.throws(() => readPrivateFile(file, 'credential', { platform: 'linux' }), /chmod/);
    assert.throws(() => readGitToken(join(dir, 'missing'), { platform: 'win32' }), error => !/0600|chmod/u.test(error.message));
    writeFileSync(file, ''); assert.throws(() => readPrivateFile(file, 'credential', { platform: 'win32' }), /rỗng/);
  } finally { rmSync(dir, { recursive: true }); }
});
test('source updates preserve Console-only variables and warn without printing their values', () => {
  const { existing, desired } = fixture(); const warnings = [];
  const merged = preserveConsoleVariables(existing, desired, true, message => warnings.push(message));
  assert.deepEqual(merged.driverInputs.values.variables, existing.driverInputs.values.variables);
  assert.equal(warnings.length, 4); assert.ok(warnings.some(message => message.includes('image')));
  assert.ok(warnings.every(message => !message.includes('sha256:old')));
  assert.equal(desired.driverInputs.values.variables.image, undefined);
  assert.equal(definitionAction(existing, merged, true), 'update');
  assert.doesNotThrow(() => validateUpdatePayload({ ...merged, version: 4 }, existing, merged));
  const dropped = structuredClone(merged); delete dropped.driverInputs.values.variables.image;
  assert.throws(() => validateUpdatePayload({ ...dropped, version: 4 }, existing, merged), /image/);
  const saved = { ...structuredClone(merged), version: 5 };
  assert.equal(definitionAction(saved, desired, true), 'reuse');
});
test('shared conflicting variables fail with exact path and both values; unrelated guards remain', () => {
  const { existing, desired } = fixture(); desired.driverInputs.values.variables.image = 'mysql@sha256:new';
  assert.throws(() => definitionAction(existing, desired, true), error => ['image', 'Console=', 'seed=', 'sha256:old', 'sha256:new'].every(text => error.message.includes(text)));
  delete desired.driverInputs.values.variables.image; desired.name = 'Changed';
  assert.throws(() => definitionAction(existing, desired, true), /name.*Database.*Changed/);
  assert.throws(() => definitionAction(existing, desired, false), /không ghi đè/);
});
test('difference errors redact nested secrets and credential-bearing URLs', () => {
  const text = differences({}, { options: { password: 'sensitive-sentinel' }, image: 'https://reader:sensitive-sentinel@git.example/repo' });
  assert.ok(!text.includes('sensitive-sentinel')); assert.ok(text.includes('<ẩn'));
});
test('malformed variable sources identify Definition and variable and show a supported format', () => {
  for (const source of ['mysql:8', { literal: [] }, { context: 'wrong' }, { resource: { type: 'mysql' } }, { literal: 'x', context: 'env.namespace' }]) {
    const raw = loadTarget({ target: 'staging' }, {}).raw;
    const item = raw.resourceDefinitions.find(item => item.resourceType === 'mysql'); item.variables.image = source;
    assert.throws(() => parseInfrastructure(raw), error => error.message.includes(item.key) && error.message.includes('image'));
  }
});
test('dev/staging MySQL Types and Definitions map digest image and existing Secret refs without password literals', () => {
  for (const target of ['dev', 'staging']) {
    const raw = loadTarget({ target }, {}).raw; const config = parseBundleConfig(raw);
    const type = config.resourceTypes.find(type => type.key === 'mysql');
    const item = config.resourceDefinitions.find(item => item.resourceType === 'mysql');
    assert.ok(type.inputs.some(field => field.name === 'image' && field.required));
    assert.equal(item.source.path, 'backend/terraform-modules/mysql');
    assert.equal(item.variables.storage_class.literal, target === 'staging' ? 'rook-ceph-block' : '');
    const capability = { resourceType: 'mysql', driverType: 'terraform', source: { sourceId: 'mysql', moduleId: 'mysql', revision: item.source.ref }, providerRequirements: [{ name: 'kubernetes', binding: 'kubernetes' }] };
    const def = desiredInfrastructureDefinition(item, capability);
    for (const name of ['image', 'auth_secret_name', 'root_password_key', 'user_password_key']) assert.deepEqual(def.driverInputs.values.input_mapping[name], { param: name });
    assert.equal(Object.hasOwn(item.variables, 'password'), false);
    assert.equal(type.outputs.some(field => field.name === 'password'), false);
  }
});

test('seed-only variables are added while Console-only values survive a source migration', () => {
  const { existing, desired } = fixture(); desired.driverInputs.values.variables.new_setting = 'new';
  const merged = preserveConsoleVariables(existing, desired, true);
  assert.equal(definitionAction(existing, merged, true), 'update');
  assert.equal(merged.driverInputs.values.variables.new_setting, 'new');
  assert.equal(merged.driverInputs.values.variables.image, 'mysql@sha256:old');
  assert.doesNotThrow(() => validateUpdatePayload({ ...merged, version: 4 }, existing, merged));
});

test('module literal type mismatch is reported before UI coercion with Definition and variable name', () => {
  assert.throws(() => validateVariableLiterals({ key: 'db', variables: { image: { literal: 123 } } }, { variables: [{ name: 'image', type: 'string' }] }), /db.*image.*yêu cầu string.*number/);
  assert.doesNotThrow(() => validateVariableLiterals({ key: 'db', variables: { enabled: { literal: false } } }, { variables: [{ name: 'enabled', type: 'bool' }] }));
});
