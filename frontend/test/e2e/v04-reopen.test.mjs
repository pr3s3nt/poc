import { describe, expect, it } from 'vitest';
import { verifyReopen } from './v04-reopen.mjs';
import { FULL_APP_TYPES } from './full-app-flows.mjs';
import { catalogIds } from './v04-internal-full.mjs';
const input = { runId: 'v04-test-1', applicationKey: 'app' };
const ids = catalogIds(input);
const scope = '/applications/app/environments/staging';
const baseDefinitions = [
  { key: ids.cluster, driverType: 'existing-cluster', driverAccount: 'lab', version: 2 },
  { key: ids.namespace }, { key: ids.postgres }, { key: ids.redis },
  { key: ids.template, provision: { companion: {} } },
];
function server({ partial = false, change = () => {} } = {}) {
  const data = {
    '/resource-types': { resourceTypes: FULL_APP_TYPES.map(({ key }) => ({ key })) },
    '/resource-definitions': { resourceDefinitions: [...baseDefinitions, { key: `postgres-invalid-${input.runId}` }, ...(!partial ? [{ key: `redis-readiness-${input.runId}` }] : [])] },
    '/driver-accounts': { accounts: [{ key: 'lab', status: 'READY', type: 'INTERNAL' }] },
    '/applications': { applications: [{ key: 'app', environments: [{ key: 'staging' }] }] },
    [`${scope}/resources`]: { resources: [{ status: partial ? 'READY' : 'DESTROYED' }] },
    [`${scope}/deployments`]: { deployments: [...Array.from({ length: 7 }, (_, i) => ({ id: `ok${i}`, status: 'SUCCEEDED' })), { id: 'e01', status: 'FAILED', workloadId: 'probe-e01' }, ...(!partial ? [{ id: 'e02', status: 'FAILED', workloadId: 'probe' }] : [])] },
    [`${scope}/deployments/e01`]: { blocked: [{ consumer: 'workload.default#modules.probe-e01' }] },
    [`${scope}/deployments/e02`]: { blocked: [{ consumer: 'workload.default#modules.probe' }] },
  };
  change(data);
  return async (url) => {
    const path = new URL(url).pathname.replace('/api/v1', '');
    if (path === '/auth/sign-in') return { ok: true, headers: { getSetCookie: () => ['session=fixture; Path=/'] } };
    return path in data ? { ok: true, json: async () => data[path] } : { ok: false, status: 404 };
  };
}
const run = (options, reached = null) => verifyReopen({ base: 'http://fixture', input, accountKey: 'lab', fetchImpl: server(options), reached, historicalNegatives: true });
describe('historical V04 reopened catalog and negative deployment histories (unit-only provenance)', () => {
  it('requires the actual seven Definitions and both retained blocked workloads after cleanup', async () => {
    await expect(run()).resolves.toMatch(/7 definitions.*7 succeeded and 2 retained failed/);
  });
  it('reads a stopped E01 run as partial without requiring an unregistered E02 fixture', async () => {
    await expect(run({ partial: true }, new Set(['negative.e01-definition-pinned', 'negative.e01-consumer-blocked']))).resolves.toMatch(/PARTIAL.*6 definitions/);
  });
  it.each(['probe-e01', 'probe'])('refuses lost blocked evidence for %s even if the other negative survives', async (workload) => {
    const id = workload === 'probe' ? 'e02' : 'e01';
    await expect(run({ change: (d) => { d[`${scope}/deployments/${id}`].blocked = []; } })).rejects.toThrow(`evidence of ${workload} was lost`);
  });
  it('still refuses missing or foreign Definitions in a complete run', async () => {
    for (const mode of ['missing', 'foreign']) {
      await expect(run({ change: (d) => { const defs = d['/resource-definitions'].resourceDefinitions; if (mode === 'missing') defs.pop(); else defs.push({ key: 'foreign' }); } })).rejects.toThrow('catalog changed');
    }
  });
  it('requires seven reached successful operations and refuses live resources after cleanup', async () => {
    await expect(run({ change: (d) => { d[`${scope}/deployments`].deployments = d[`${scope}/deployments`].deployments.filter((x) => x.id !== 'ok0'); } })).rejects.toThrow('history is incomplete');
    await expect(run({ change: (d) => { d[`${scope}/resources`].resources[0].status = 'READY'; } })).rejects.toThrow('still active');
  });
});
