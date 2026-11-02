import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { assertAccountTarget, parseInfrastructure, readSetupPacing } from './infrastructure-config.mjs';
import { ensureInfrastructureAccount } from './infrastructure-ui.mjs';

const example = () => JSON.parse(readFileSync(new URL('../config.example.json', import.meta.url), 'utf8'));
const ready = { key: 'opaque-account-key', name: 'k8s-4f', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', config: { endpoint: 'https://k8s-4f-control-plane:6443', kubeContext: 'kind-k8s-4f' } };

test('independent phase speeds override global speed and reject invalid values before opening browser', () => {
  assert.deepEqual(readSetupPacing({}, {}), { account: 800, type: 800, definition: 800, bundle: 800 });
  assert.deepEqual(readSetupPacing({ 'step-delay-ms': '1200', 'definition-delay-ms': '2000' }, { ORCH_STEP_DELAY_MS: '99', ORCH_TYPE_DELAY_MS: '1500' }), { account: 1200, type: 1500, definition: 2000, bundle: 1200 });
  for (const value of ['', '-1', '1.5', '10001', 'fast']) assert.throws(() => readSetupPacing({ 'type-delay-ms': value }, {}));
  assert.equal(readSetupPacing({ 'type-delay-ms': '0' }, {}).type, 0);
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
