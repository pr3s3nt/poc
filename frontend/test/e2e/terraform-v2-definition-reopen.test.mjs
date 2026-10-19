import { afterEach, describe, expect, it } from 'vitest';
import { createServer } from 'node:http';
import { once } from 'node:events';
import { EXPECTED_RESOURCES, expectedDefinitionKeys, verifyDefinitionReopen } from './terraform-v2-definition-reopen.mjs';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const catalog = JSON.parse(readFileSync(resolve(process.cwd(), '../backend/test/integration/v2run/testdata/definition-catalog.json'), 'utf8'));
// Scenarios present in the catalog (V05 at the V05 gate; V05, A02 and A03 once V06 adds the managed ones).
const SCENARIOS = Object.keys(catalog.definitions).filter((key) => key !== 'common');
const LAST = SCENARIOS[SCENARIOS.length - 1];
let server; let base; let fx;
async function start() {
  server = createServer((req, res) => {
    if (req.method === 'POST' && req.url === '/api/v1/auth/sign-in') { res.writeHead(200, { 'set-cookie': 'session=fx; Path=/', 'content-type': 'application/json' }); res.end('{}'); return; }
    if (req.headers.cookie !== 'session=fx') { res.writeHead(401); res.end('{}'); return; }
    const path = req.url;
    let body;
    if (path === '/api/v1/resource-types') body = { resourceTypes: fx.types };
    else if (path === '/api/v1/resource-definitions') body = { resourceDefinitions: fx.definitions };
    else if (path === '/api/v1/resource-definition-capabilities') body = { capabilities: fx.modules.map((module) => ({ driverType: 'terraform', module, source: { contentSha256: 'a'.repeat(64) } })) };
    else if (path === '/api/v1/driver-accounts') body = { accounts: fx.accounts };
    else if (path === '/api/v1/applications') body = { applications: [{ key: 'app-key', environments: [{ key: 'staging', ...fx.environment }] }] };
    else if (path.endsWith('/resources')) body = { resources: fx.resources };
    else if (path.endsWith('/deployments')) body = { deployments: fx.deployments };
    else { res.writeHead(404); res.end('{}'); return; }
    res.writeHead(200, { 'content-type': 'application/json' }); res.end(JSON.stringify(body));
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  base = `http://127.0.0.1:${server.address().port}`;
}
afterEach(async () => { if (server) { server.close(); await once(server, 'close'); server = undefined; } });

function setup(scenario) {
  const entries = [...catalog.definitions.common, ...catalog.definitions[scenario]];
  fx = {
    types: catalog.types.map((t) => ({ key: t.key })),
    definitions: expectedDefinitionKeys(scenario).map((key) => ({ key, driverType: entries.find((e) => e.id === key)?.provider === 'aws' ? 'terraform' : 'other', driverAccount: entries.find((e) => e.id === key)?.provider === 'aws' ? 'acct' : undefined })),
    modules: entries.map((e) => e.module),
    accounts: [{ key: 'acct', type: 'AWS', status: 'READY' }], environment: {},
    instances: EXPECTED_RESOURCES[scenario].map(([type, module], i) => ({ id: `row${i}`, descriptor: `${type}.default#shared.res${i}`, module, status: 'DESTROYED', stateKey12: '123456789abc', createdByRun: 'pending-fixture', scope: 'SHARED' })),
    resources: EXPECTED_RESOURCES[scenario].map(([type, module], i) => ({ id: `row${i}`, resourceId: `res${i}`, type, module, status: 'DESTROYED' })).filter((r) => r.type !== 'k8s-cluster'),
    deployments: [{ id: 'a', status: 'SUCCEEDED' }, { id: 'b', status: 'SUCCEEDED' }, { id: 'c', status: 'SUCCEEDED' }],
  };
}
const args = (scenario) => ({ base, scenario, applicationKey: 'app-key', accountKey: 'acct', persistedInstances: fx.instances });

describe('Definition-driven AWS SQL reopen', () => {
  it.each(SCENARIOS)('accepts the exact %s catalog and post-cleanup history', async (scenario) => {
    setup(scenario); await start();
    await expect(verifyDefinitionReopen(args(scenario))).resolves.toMatchObject({ scenario, resources: 5, apiResources: 4 });
  });
  it('rejects an Environment that gained a Connection or an execution profile', async () => {
    setup('V05'); fx.environment = { connectionKey: 'x' }; await start();
    await expect(verifyDefinitionReopen(args('V05'))).rejects.toThrow('gained a deployment Connection');
  });
  it('rejects a lost Driver Account, an unready account and a changed catalog', async () => {
    setup('V05'); fx.definitions.find((d) => d.driverType === 'terraform').driverAccount = 'other'; await start();
    await expect(verifyDefinitionReopen(args('V05'))).rejects.toThrow('lost its own Driver Account');
    await new Promise((r) => server.close(r)); server = undefined;
    setup('V05'); fx.accounts[0].status = 'VERIFYING'; await start();
    await expect(verifyDefinitionReopen(args('V05'))).rejects.toThrow('not READY');
    await new Promise((r) => server.close(r)); server = undefined;
    setup(LAST); fx.definitions.pop(); await start();
    await expect(verifyDefinitionReopen(args(LAST))).rejects.toThrow('definition catalog changed');
  });
  it('rejects live, missing or duplicated owned resources and an incomplete deployment history', async () => {
    setup(LAST); fx.resources[1].status = 'READY'; await start();
    await expect(verifyDefinitionReopen(args(LAST))).rejects.toThrow('is not DESTROYED');
    await new Promise((r) => server.close(r)); server = undefined;
    setup(LAST); fx.resources.push({ ...fx.resources[0] }); await start();
    await expect(verifyDefinitionReopen(args(LAST))).rejects.toThrow('resource history count');
    await new Promise((r) => server.close(r)); server = undefined;
    setup('V05'); fx.deployments = fx.deployments.slice(0, 2); await start();
    await expect(verifyDefinitionReopen(args('V05'))).rejects.toThrow('deployment history');
  });
  it('is strict about its arguments', async () => {
    await expect(verifyDefinitionReopen({ base: 'http://x', scenario: 'A01', applicationKey: 'a', accountKey: 'k' })).rejects.toThrow('incomplete');
  });
});
