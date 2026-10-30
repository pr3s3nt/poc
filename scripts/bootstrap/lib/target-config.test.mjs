import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { readConsoleConfig } from './console.mjs';
import { loadTarget } from './target-config.mjs';
import { parseBundleConfig } from './render-bundle-config.mjs';
import { discoverAccountTarget } from './infrastructure-config.mjs';
import { ensureInfrastructureAccount } from './infrastructure-ui.mjs';

test('staging preset preloads Console, seed login, Ceph and PVC-only rendering', () => {
  const target = loadTarget({ target: 'staging', kubeconfig: '/private/cluster' }, {}, '/private/home');
  assert.equal(target.env.ORCH_PLATFORM_URL, 'https://devportal.stg.exampledevops.com');
  assert.equal(target.env.ORCH_PLATFORM_USERNAME, 'platform-engineer');
  assert.equal(target.env.ORCH_PLATFORM_PASSWORD_FILE, undefined);
  assert.equal(readConsoleConfig(target.env, { defaultPassword: target.defaultPassword }).password, 'test-password');
  assert.equal(target.env.ORCH_KUBECONFIG_FILE, '/private/cluster');
  assert.equal(target.env.ORCH_HEADLESS, '1');
  const config = parseBundleConfig(target.raw);
  assert.equal(config.account.discover, true);
  const ambiguous = structuredClone(target.raw);
  ambiguous.account.context = 'must-not-combine-with-discovery';
  assert.throws(() => parseBundleConfig(ambiguous));
  assert.equal(config.resourceDefinitions.find(d => d.resourceType === 'persistent-storage').variables.storage_class.literal, 'rook-ceph-block');
  assert.deepEqual(config.bundles[0].criteria, [{ env_type: 'staging', class: 'pvc' }]);
  assert.equal(config.bundles[0].source.ref, '1182cb5ff275451ebfa8b711bc07effd1e120fcf');
  assert.equal(Object.keys(target.env).some(k => k === 'ORCH_PLATFORM_PASSWORD'), false);
});
test('dev preset retains exact kind target; explicit environment overrides Console auth defaults', () => {
  const target = loadTarget({ target: 'dev' }, { ORCH_PLATFORM_URL: 'https://override.example', ORCH_PLATFORM_USERNAME: 'admin', ORCH_PLATFORM_PASSWORD_FILE: '/private/password', ORCH_HEADLESS: '0' }, '/private/home');
  assert.equal(target.env.ORCH_PLATFORM_URL, 'https://override.example');
  assert.equal(target.env.ORCH_PLATFORM_PASSWORD_FILE, '/private/password');
  assert.equal(target.env.ORCH_HEADLESS, '0');
  assert.equal(parseBundleConfig(target.raw).account.context, 'kind-k8s-4f');
  assert.throws(() => loadTarget({ target: 'prod' }, {}));
  assert.throws(() => loadTarget({ target: 'dev', config: 'anything' }, {}));
});
test('discovery accepts exactly one HTTPS context and rejects ambiguity or embedded credentials', () => {
  assert.deepEqual(discoverAccountTarget('k8s-4f', [{ name: 'staging', endpoint: 'https://cluster.example:6443' }]), { name: 'k8s-4f', context: 'staging', endpoint: 'https://cluster.example:6443' });
  for (const contexts of [[], [{ name: 'a', endpoint: 'https://a' }, { name: 'b', endpoint: 'https://b' }], [{ name: 'a', endpoint: 'http://a' }], [{ name: 'a', endpoint: 'https://user:private@a' }]]) assert.throws(() => discoverAccountTarget('k8s-4f', contexts));
});
test('discover preset reuses an existing READY account without reading kubeconfig or mutation; rejects unhealthy accounts', async () => {
  const account = { key: 'opaque-id', name: 'k8s-4f', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', config: { kubeContext: 'staging', endpoint: 'https://staging.example:6443' } };
  const input = { account: { name: 'k8s-4f', discover: true }, get: async () => ({ accounts: [account] }), kubeconfigFile: '/must-not-be-read', act: () => { throw Error('must not mutate'); } };
  assert.equal(await ensureInfrastructureAccount(input), account);
  await assert.rejects(() => ensureInfrastructureAccount({ ...input, get: async () => ({ accounts: [{ ...account, status: 'FAILED' }] }) }));
});

test('explicit password file wins over seed default; invalid override never falls back silently', () => {
  const dir = mkdtempSync(join(tmpdir(), 'orch-bootstrap-auth-'));
  const passwordFile = join(dir, 'password');
  writeFileSync(passwordFile, 'changed-login-password', { mode: 0o600 });
  const target = loadTarget({ target: 'staging' }, { ORCH_PLATFORM_PASSWORD_FILE: passwordFile });
  assert.equal(readConsoleConfig(target.env, { defaultPassword: target.defaultPassword }).password, 'changed-login-password');
  assert.throws(() => readConsoleConfig({ ...target.env, ORCH_PLATFORM_PASSWORD_FILE: join(dir, 'missing') }, { defaultPassword: target.defaultPassword }));
  const custom = loadTarget({ config: new URL('../config.example.json', import.meta.url) }, { ORCH_PLATFORM_URL: 'http://localhost:3001', ORCH_PLATFORM_USERNAME: 'platform-engineer' });
  assert.throws(() => readConsoleConfig(custom.env, { defaultPassword: custom.defaultPassword }), /ORCH_PLATFORM_PASSWORD_FILE/);
});
