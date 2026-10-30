import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { assertAccountTarget, desiredInfrastructureDefinition, parseInfrastructure, readSetupPacing } from './infrastructure-config.mjs';
import { ensureInfrastructureAccount } from './infrastructure-ui.mjs';
import { assertSameDefinition, parseBundleConfig } from './render-bundle-config.mjs';

const example = () => JSON.parse(readFileSync(new URL('../config.example.json', import.meta.url), 'utf8'));
const ready = { key: 'opaque-account-key', name: 'k8s-4f', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', config: { endpoint: 'https://k8s-4f-control-plane:6443', kubeContext: 'kind-k8s-4f' } };

test('independent phase speeds override global speed and reject invalid values before opening browser', () => {
  assert.deepEqual(readSetupPacing({}, {}), { account: 800, type: 800, definition: 800, bundle: 800 });
  assert.deepEqual(readSetupPacing({ 'step-delay-ms': '1200', 'definition-delay-ms': '2000' }, { ORCH_STEP_DELAY_MS: '99', ORCH_TYPE_DELAY_MS: '1500' }), { account: 1200, type: 1500, definition: 2000, bundle: 1200 });
  for (const value of ['', '-1', '1.5', '10001', 'fast']) assert.throws(() => readSetupPacing({ 'type-delay-ms': value }, {}));
  assert.equal(readSetupPacing({ 'type-delay-ms': '0' }, {}).type, 0);
});

test('example config defines seven Types and the account/cluster/namespace/storage/database dependencies', () => {
  const config = parseBundleConfig(example());
  assert.equal(config.resourceTypes.length, 7);
  assert.deepEqual(config.resourceDefinitions.map(item => item.resourceType), ['k8s-cluster', 'k8s-namespace', 'persistent-storage', 'postgres', 'redis', 'mysql']);
  assert.doesNotThrow(() => assertAccountTarget(ready, config.account));
});

test('account reuse checks target, context, READY, authentication and account type', () => {
  const expected = example().account;
  for (const patch of [{ status: 'FAILED' }, { type: 'AWS' }, { authenticationType: 'TOKEN' }, { config: { ...ready.config, endpoint: 'https://other-cluster:6443' } }, { config: { ...ready.config, kubeContext: 'different-context' } }]) assert.throws(() => assertAccountTarget({ ...ready, ...patch }, expected), /không thay/);
});

test('matching READY account needs neither local kubeconfig nor UI mutation', async () => {
  const config = example();
  const result = await ensureInfrastructureAccount({ account: config.account, get: async () => ({ accounts: [ready] }), kubeconfigFile: '/does-not-exist', act: () => { throw new Error('must not mutate'); } });
  assert.equal(result, ready);
});

test('Terraform desired config pins source and preserves mappings, provider target and typed sources', () => {
  const item = parseInfrastructure(example()).resourceDefinitions[2];
  const capability = { resourceType: 'persistent-storage', driverType: 'terraform', source: { sourceId: 'git', moduleId: 'storage', revision: 'a'.repeat(40), subdirectory: 'storage', contentSha256: 'b'.repeat(64) }, providerRequirements: [{ name: 'kubernetes', binding: 'kubernetes' }] };
  const def = desiredInfrastructureDefinition(item, capability);
  assert.equal(def.driverInputs.values.variables.namespace, "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}");
  assert.deepEqual(def.driverInputs.values.input_mapping, { storage: { param: 'storage' } });
  assert.deepEqual(def.driverInputs.values.output_mapping, { claimName: 'claimName', namespace: 'namespace' });
  assert.deepEqual(def.driverInputs.values.provider_bindings, { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.internal#environments.@app.@env' } });
  const changed = structuredClone(def); changed.driverInputs.values.source.revision = 'c'.repeat(40);
  assert.throws(() => assertSameDefinition(changed, def), /không ghi đè/);
});

test('existing cluster Definition uses actual account key and detects a different account on rerun', () => {
  const item = parseInfrastructure(example()).resourceDefinitions[0];
  const def = desiredInfrastructureDefinition(item, undefined, ready);
  assert.equal(def.driverAccount, ready.key);
  assert.deepEqual(def.driverInputs, { values: { variables: {} } });
  assert.throws(() => assertSameDefinition({ ...def, driverAccount: 'other' }, def), /driverAccount/);
});

test('rejects ambiguous sources, unknown keys, JSON literal values and duplicate Definition IDs', () => {
  for (const mutate of [
    raw => { raw.account.endpoint = 'https://user:secret@host'; },
    raw => { raw.resourceDefinitions.push(structuredClone(raw.resourceDefinitions[0])); },
    raw => { raw.resourceDefinitions[1].source = raw.resourceDefinitions[2].source; },
    raw => { raw.resourceDefinitions[1].variables = { name: { literal: {} } }; },
    raw => { raw.resourceDefinitions[1].criteria = []; },
    raw => { raw.resourceDefinitions[2].source.ref = 'main'; },
    raw => { raw.resourceDefinitions[2].driverInputs = {}; },
    raw => { raw.resourceDefinitions[0].variables = {}; },
    raw => { raw.resourceDefinitions[0].key = raw.bundles[0].key; },
  ]) { const raw = example(); mutate(raw); assert.throws(() => parseBundleConfig(raw)); }
});
