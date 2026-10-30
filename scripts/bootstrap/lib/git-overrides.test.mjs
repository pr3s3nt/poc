import { test } from 'node:test';
import assert from 'node:assert/strict';
import { loadTarget } from './target-config.mjs';
import { applyGitOverrides } from './git-overrides.mjs';
import { parseBundleConfig } from './render-bundle-config.mjs';
import { desiredInfrastructureDefinition } from './infrastructure-config.mjs';
import { definitionAction } from './definition-action.mjs';
import { validateUpdatePayload } from './definition-update-ui.mjs';

const url = 'https://git.company.example/team/catalog.git';
const ref = 'b'.repeat(40);
const preset = () => loadTarget({ target: 'staging' }, {}).raw;

test('company Git converts installed modules and rewrites Git sources without changing other seed data', () => {
  const raw = preset();
  const result = applyGitOverrides(raw, { 'git-url': url, 'git-ref': ref });
  parseBundleConfig(result);
  for (const [i, item] of result.resourceDefinitions.entries()) {
    const old = raw.resourceDefinitions[i];
    if (item.driverType === 'existing-cluster') { assert.deepEqual(item, old); continue; }
    assert.equal(item.module, undefined);
    assert.equal(item.source.url, url);
    assert.equal(item.source.ref, ref);
    assert.equal(item.source.path, old.source?.path ?? `backend/terraform-modules/${old.module}`);
    const { source, module, ...rest } = item;
    const { source: oldSource, module: oldModule, ...oldRest } = old;
    assert.deepEqual(rest, oldRest);
  }
  assert.deepEqual(result.resourceTypes, raw.resourceTypes);
  assert.deepEqual(result.account, raw.account);
  assert.equal(result.bundles[0].source.path, raw.bundles[0].source.path);
  assert.equal(result.bundles[0].source.url, url);
  assert.equal(result.bundles[0].source.ref, ref);
  assert.deepEqual(result.bundles[0].criteria, raw.bundles[0].criteria);
  assert.equal(raw.resourceDefinitions.some(d => d.module === 'postgres-k8s'), true);
});
test('no Git override keeps old config; ref-only does not convert installed modules; unsafe or ambiguous overrides fail early', () => {
  const raw = preset();
  assert.deepEqual(applyGitOverrides(raw, {}), raw);
  assert.equal(applyGitOverrides(raw, { 'git-ref': ref }).resourceDefinitions.find(d => d.resourceType === 'postgres').module, 'postgres-k8s');
  for (const options of [{ 'git-url': url }, { 'git-url': 'http://unsafe.example', 'git-ref': ref }, { 'git-url': 'https://user:secret@git.example/repo', 'git-ref': ref }, { 'git-ref': 'main' }, { 'git-ref': '' }]) assert.throws(() => applyGitOverrides(raw, options));
  raw.resourceDefinitions.find(d => d.module).module = 'unknown-layout';
  assert.throws(() => applyGitOverrides(raw, { 'git-url': url, 'git-ref': ref }), /đường dẫn/);
});
function definitions() {
  const item = parseBundleConfig(preset()).resourceDefinitions.find(d => d.resourceType === 'persistent-storage');
  const capability = { resourceType: item.resourceType, driverType: 'terraform', source: { sourceId: 'company', moduleId: 'storage', revision: ref, subdirectory: item.source.path, contentSha256: 'c'.repeat(64) }, providerRequirements: [{ name: 'kubernetes', binding: 'kubernetes' }] };
  const desired = desiredInfrastructureDefinition(item, capability);
  const existing = structuredClone(desired);
  existing.version = 3;
  existing.driverInputs.values.source.revision = 'a'.repeat(40);
  return { existing, desired };
}
test('source updates require opt-in and CAS version; criteria/mapping/account changes are refused', () => {
  const { existing, desired } = definitions();
  assert.equal(definitionAction(undefined, desired), 'create');
  assert.equal(definitionAction({ ...desired, version: 3 }, desired, true), 'reuse');
  assert.throws(() => definitionAction(existing, desired));
  assert.equal(definitionAction(existing, desired, true), 'update');
  for (const mutate of [d => { d.criteria = [{}]; }, d => { d.name += ' changed'; }, d => { d.driverAccount = 'different'; }, d => { d.driverInputs.values.variables.storage_class = 'different'; }, d => { d.driverInputs.values.input_mapping = {}; }]) {
    const changed = structuredClone(desired); mutate(changed);
    assert.throws(() => definitionAction(existing, changed, true), /ngoài Git/);
  }
  assert.throws(() => definitionAction({ ...existing, version: undefined }, desired, true), /CAS/);
});
test('renderer update allows only bundle pin, preserves placement and inputs', () => {
  const desired = { key: 'renderer', name: 'Renderer', resourceType: 'workload', driverType: 'score-k8s', driverInputs: { values: { variables: { render_bundle: 'new', namespace: 'ns', cluster: 'cluster', render_inputs: {} } } }, criteria: [{}] };
  const existing = structuredClone(desired); existing.version = 2;
  existing.driverInputs.values.variables.render_bundle = 'old';
  assert.equal(definitionAction(existing, desired, true), 'update');
  const changed = structuredClone(desired); changed.driverInputs.values.variables.namespace = 'other';
  assert.throws(() => definitionAction(existing, changed, true));
});
test('guard blocks stale UI version or a changed payload before PUT reaches backend', () => {
  const { existing, desired } = definitions();
  assert.doesNotThrow(() => validateUpdatePayload({ ...desired, version: existing.version }, existing, desired));
  assert.throws(() => validateUpdatePayload({ ...desired, version: existing.version + 1 }, existing, desired), /version UI/);
  assert.throws(() => validateUpdatePayload({ ...desired, name: 'unexpected', version: existing.version }, existing, desired), /không gửi/);
});
