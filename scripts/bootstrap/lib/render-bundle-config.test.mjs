import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { assertSameDefinition, assertSameResourceType, desiredDefinition, parseBundleConfig } from './render-bundle-config.mjs';

const example = () => JSON.parse(readFileSync(new URL('../config.example.json', import.meta.url), 'utf8'));

test('example pins five templates and preserves customized placement, inputs and scope', () => {
  const raw = example();
  raw.bundles[0].renderInputs = { team: 'payments', runAsNonRoot: true, defaultReplicas: 2 };
  raw.bundles[0].criteria = [{ class: 'payments', env_type: 'staging' }, { app_id: 'example' }];
  const { bundles } = parseBundleConfig(raw);
  const definition = desiredDefinition(bundles[0], 'verified-pin');
  assert.equal(bundles[0].expectedTemplates.length, 5);
  assert.equal(definition.driverInputs.values.variables.render_bundle, 'verified-pin');
  assert.deepEqual(definition.driverInputs.values.variables.render_inputs, raw.bundles[0].renderInputs);
  assert.deepEqual(definition.criteria, raw.bundles[0].criteria);
});

test('rejects duplicate keys, embedded Git credentials and unknown or incomplete configuration before connecting', () => {
  for (const mutate of [
    raw => raw.bundles.push(structuredClone(raw.bundles[0])),
    raw => { raw.bundles[0].source.url = 'https://user:password@example.com/repo.git'; },
    raw => { raw.bundles[0].source.url = 'https://example.com/repo.git?token=secret'; },
    raw => { raw.bundles[0].source.path = '../outside'; },
    raw => { delete raw.bundles[0].placement.cluster; },
    raw => { raw.bundles[0].criteria = []; },
    raw => { raw.bundles[0].criteria = [{ region: 'unknown-field' }]; },
    raw => { raw.bundles[0].criteria = [{ class: '' }]; },
    raw => { raw.bundles[0].renderInput = {}; },
    raw => { raw.verify.environment = 'typo'; },
  ]) {
    const raw = example(); mutate(raw);
    assert.throws(() => parseBundleConfig(raw));
  }
});

test('reruns accept server metadata, absent empty provision and unordered matching criteria', () => {
  const { bundles } = parseBundleConfig(example());
  const desired = desiredDefinition(bundles[0], 'verified-pin');
  desired.criteria = [{ class: 'blue', env_type: 'staging' }, { app_id: 'demo' }];
  const actual = { ...structuredClone(desired), version: 3, sourceFingerprint: 'server-owned' };
  delete actual.provision;
  actual.criteria = [{ app_id: 'demo' }, { env_type: 'staging', class: 'blue' }];
  assert.doesNotThrow(() => assertSameDefinition(actual, desired));
});

test('reruns refuse changes to source pin, render inputs, placement, name and scope', () => {
  const { bundles } = parseBundleConfig(example());
  const desired = desiredDefinition(bundles[0], 'verified-pin');
  for (const mutate of [
    actual => { actual.driverInputs.values.variables.render_bundle = 'different-pin'; },
    actual => { actual.driverInputs.values.variables.render_inputs = { team: 'changed' }; },
    actual => { actual.driverInputs.values.variables.namespace = 'different-target'; },
    actual => { actual.name = 'Changed'; },
    actual => { actual.criteria = [{ class: 'different' }]; },
    actual => { actual.resourceType = 'postgres'; },
  ]) {
    const actual = structuredClone(desired); mutate(actual);
    assert.throws(() => assertSameDefinition(actual, desired), /không ghi đè/);
  }
});

test('Resource Type config preserves typed schema and normalizes absent flags', () => {
  const raw = example();
  raw.resourceTypes[0].inputs = [{ name: 'count', type: 'number', required: true }, { name: 'enabled', type: 'bool' }, { name: 'labels', type: 'any' }];
  raw.resourceTypes[0].outputs.push({ name: 'password', type: 'string', secret: true });
  const { resourceTypes } = parseBundleConfig(raw);
  assert.equal(resourceTypes[0].inputs[1].required, false);
  assert.equal(resourceTypes[0].outputs[2].secret, true);
  assert.doesNotThrow(() => assertSameResourceType({ ...raw.resourceTypes[0], classes: ['default'], outputs: [...raw.resourceTypes[0].outputs].reverse() }, resourceTypes[0]));
});

test('Resource Type reruns reject a changed type, required/secret flag or missing output', () => {
  const { resourceTypes: [desired] } = parseBundleConfig(example());
  for (const mutate of [
    actual => { actual.outputs[0].secret = true; },
    actual => { actual.outputs[0].required = false; },
    actual => { actual.outputs[0].type = 'any'; },
    actual => { actual.outputs.pop(); },
  ]) {
    const actual = structuredClone(desired); mutate(actual);
    assert.throws(() => assertSameResourceType(actual, desired), /không ghi đè/);
  }
});

test('rejects invalid Resource Types and configurations requiring JSON UI before mutations', () => {
  for (const mutate of [
    raw => raw.resourceTypes.push(structuredClone(raw.resourceTypes[0])),
    raw => { raw.resourceTypes[0].key = 'environment'; },
    raw => { raw.resourceTypes[0].inputs.push({ name: 'storage', type: 'string' }); },
    raw => { raw.resourceTypes[0].outputs[0].type = 'boolean'; },
    raw => { raw.resourceTypes[0].outputs[0].secret = 'false'; },
    raw => { raw.bundles[0].provision = { child: { params: { count: 1 } } }; },
    raw => { raw.bundles[0].renderInputs = { nested: {} }; },
    raw => { raw.verify.volumeMount.resourceType = ''; },
  ]) {
    const raw = example(); mutate(raw);
    assert.throws(() => parseBundleConfig(raw));
  }
});

test('existing bundle-only configuration stays valid', () => {
  const raw = example(); delete raw.resourceTypes; delete raw.verify.volumeMount;
  delete raw.account; delete raw.resourceDefinitions;
  assert.deepEqual(parseBundleConfig(raw).resourceTypes, []);
});
