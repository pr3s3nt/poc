import assert from 'node:assert/strict';
import { test } from 'vitest';
import { verifyReopen } from './v03-reopen.mjs';
import { V03_TYPES, catalogIds } from './v03-internal-frontend.mjs';

const input = { runId: 'v03-1018-ab12', applicationKey: 'acc-v03-1018-ab12' };
const ids = catalogIds(input);

function server(overrides = {}) {
  const data = {
    '/resource-types': { resourceTypes: V03_TYPES.map((t) => ({ key: t.key })) },
    '/resource-definitions': { resourceDefinitions: [
      { key: ids.cluster, driverType: 'existing-cluster', driverAccount: 'lab', version: 2 },
      { key: ids.namespace }, { key: ids.template, provision: { 'k8s-namespace.default#environments.@app.@env-aux': {} } }, { key: ids.rivalA }, { key: ids.rivalB },
    ] },
    '/driver-accounts': { accounts: [{ key: 'lab', status: 'READY', type: 'INTERNAL' }] },
    [`/applications/${input.applicationKey}/environments/staging/deployments`]: { deployments: [{ status: 'SUCCEEDED' }, { status: 'SUCCEEDED' }, { status: 'SUCCEEDED' }, { status: 'SUCCEEDED' }, { status: 'FAILED' }] },
    '/applications': { applications: [{ key: input.applicationKey, environments: [{ key: 'staging' }] }] },
    [`/applications/${input.applicationKey}/environments/staging/resources`]: { resources: [{ descriptor: 'k8s-namespace.default#x', status: 'DESTROYED' }] },
    ...overrides,
  };
  return async (url) => {
    const path = new URL(url).pathname.replace('/api/v1', '');
    if (path === '/auth/sign-in') return { ok: true, headers: { getSetCookie: () => ['session=abc; Path=/'] } };
    if (!(path in data)) return { ok: false, status: 404 };
    return { ok: true, json: async () => data[path] };
  };
}

test('a complete reopened state passes', async () => {
  const out = await verifyReopen({ base: 'http://x/ui', input, accountKey: 'lab', fetchImpl: server() });
  assert.match(out, /reopen ok/);
  assert.match(out, /namespaces destroyed/);
});

test('a changed catalog, a lost companion rule, a Connection on the Environment or a live namespace fail', async () => {
  const run = (overrides) => verifyReopen({ base: 'http://x/ui', input, accountKey: 'lab', fetchImpl: server(overrides) });
  await assert.rejects(run({ '/resource-types': { resourceTypes: [{ key: 'workload' }] } }), /resource type catalog changed/);
  await assert.rejects(run({ '/resource-definitions': { resourceDefinitions: [{ key: 'builtin-existing-cluster' }] } }), /catalog changed/);
  await assert.rejects(run({ '/driver-accounts': { accounts: [{ key: 'lab', status: 'REJECTED', type: 'INTERNAL' }] } }), /not READY/);
  await assert.rejects(run({ '/applications': { applications: [{ key: input.applicationKey, environments: [{ key: 'staging', connectionKey: 'lab' }] }] } }), /gained a deployment Connection/);
  await assert.rejects(run({ [`/applications/${input.applicationKey}/environments/staging/resources`]: { resources: [{ descriptor: 'k8s-namespace.default#x', status: 'READY' }] } }), /still active/);
  await assert.rejects(run({ [`/applications/${input.applicationKey}/environments/staging/deployments`]: { deployments: [{ status: 'SUCCEEDED' }] } }), /history .* is incomplete/);
});

test('an early-stopped run is read back in PARTIAL mode for what it reached, never blaming a step that did not run', async () => {
  const reached = new Set(['ui.account.ready', 'ui.definition.workload', 'ready.workload']);
  const partial = {
    '/resource-definitions': { resourceDefinitions: [{ key: ids.cluster, driverType: 'existing-cluster', driverAccount: 'lab', version: 1 }, { key: ids.namespace }, { key: ids.template, provision: { 'k8s-namespace.default#environments.@app.@env-aux': {} } }] },
    [`/applications/${input.applicationKey}/environments/staging/deployments`]: { deployments: [{ status: 'SUCCEEDED' }] },
    [`/applications/${input.applicationKey}/environments/staging/resources`]: { resources: [{ descriptor: 'k8s-namespace.default#x', status: 'READY' }] },
  };
  const out = await verifyReopen({ base: 'http://x/ui', input, accountKey: 'lab', fetchImpl: server(partial), reached });
  assert.match(out, /PARTIAL run/);
  assert.match(out, /1 namespace resource\(s\) still active/);
  assert.doesNotMatch(out, /namespaces destroyed/);
  // Partial mode still refuses a foreign definition, a lost account or a Connection on the Environment.
  await assert.rejects(verifyReopen({ base: 'http://x/ui', input, accountKey: 'lab', reached, fetchImpl: server({ ...partial, '/resource-definitions': { resourceDefinitions: [{ key: 'stranger' }] } }) }), /catalog changed/);
  await assert.rejects(verifyReopen({ base: 'http://x/ui', input, accountKey: 'lab', reached, fetchImpl: server({ ...partial, '/applications': { applications: [{ key: input.applicationKey, environments: [{ key: 'staging', connectionKey: 'lab' }] }] } }) }), /gained a deployment Connection/);
  // The complete (strict) read still demands everything.
  await assert.rejects(verifyReopen({ base: 'http://x/ui', input, accountKey: 'lab', reached: new Set(['cleanup.namespaces-absent']), fetchImpl: server(partial) }), /changed after reopen/);
});
