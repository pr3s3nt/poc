import { afterEach, describe, expect, it } from 'vitest';
import { createServer } from 'node:http';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { once } from 'node:events';
import { verifyReopen } from './terraform-v2-reopen.mjs';
import { applyOverrideValue, decodeCatalogOverride, formOverrideText } from './terraform-v2-steps.mjs';

const awsCatalog = JSON.parse(readFileSync(resolve(process.cwd(), '../backend/test/integration/v2run/testdata/aws-catalog.json'), 'utf8'));

function catalogFormValue(definitionID, variable, substitutions = {}) {
  const entries = [...awsCatalog.definitions.common, ...awsCatalog.definitions.A02, ...awsCatalog.definitions.A03];
  const definition = entries.find((item) => item.id === definitionID);
  const encoded = definition?.overrides?.find(([name]) => name === variable)?.[1];
  if (encoded === undefined) throw new Error(`missing catalog override ${definitionID}.${variable}`);
  return formOverrideText(decodeCatalogOverride(encoded, substitutions));
}

let server;
let base;
let fixture = {};
async function start() {
  server = createServer((req, res) => {
    if (req.method === 'POST' && req.url === '/api/v1/auth/sign-in') {
      res.writeHead(200, { 'set-cookie': 'session=fixture; Path=/; HttpOnly', 'content-type': 'application/json' }); res.end('{}'); return;
    }
    if (req.headers.cookie !== 'session=fixture') { res.writeHead(401); res.end('{}'); return; }
    const path = req.url;
    if (path.startsWith('/api/v1/applications/') && !path.startsWith(`/api/v1/applications/${fixture.appKey}/`)) { res.writeHead(404); res.end('{}'); return; }
    if (fixture.failPath === path) { res.writeHead(503); res.end('{"detail":"private server response"}'); return; }
    let body;
    if (path === '/api/v1/resource-types') body = { resourceTypes: fixture.types };
    else if (path === '/api/v1/resource-definitions') body = { resourceDefinitions: fixture.definitions.map((key) => ({ key })) };
    else if (path === '/api/v1/resource-definition-capabilities') body = { capabilities: fixture.modules.map((module) => ({ driverType: 'terraform', module, source: { contentSha256: 'a'.repeat(64) } })) };
    else if (path === '/api/v1/connections') body = { connections: [{ key: fixture.connectionKey, kind: fixture.kind, status: 'READY' }] };
    else if (path.endsWith('/resources')) body = { resources: fixture.resources };
    else if (path.endsWith('/deployments')) body = { deployments: fixture.deployments };
    else if (path.includes('/resources/')) body = { resource: fixture.resourceDetail };
    else if (path.includes('/deployments/')) body = { resourceProgress: fixture.progress };
    else { res.writeHead(404); res.end('{}'); return; }
    res.writeHead(200, { 'content-type': 'application/json' }); res.end(fixture.malformedPath === path ? '{' : JSON.stringify(body));
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  base = `http://127.0.0.1:${server.address().port}`;
}
afterEach(async () => { if (server) { server.close(); await once(server, 'close'); server = undefined; } });

function setup(scenario) {
  const aws = scenario === 'A02' || scenario === 'A03';
  fixture = {
    types: aws ? ['workload', 'k8s-cluster', 'vpc', 'k8s-namespace', 'postgres', 'redis'].map((key) => ({ key })) : ['workload', 'k8s-cluster', 'k8s-namespace', 'postgres', 'redis'].map((key) => ({ key })),
    definitions: aws ? ['vpc-aws', 'cluster-aws-eks', 'namespace-eks-module', 'pg-aurora', scenario === 'A02' ? 'redis-elasticache' : 'redis-eks-in-cluster', 'render-aws'] : [...(scenario === 'A01' || scenario === 'E02' ? ['builtin-existing-cluster'] : []), 'namespace-k8s-module', 'pg-in-cluster', 'redis-in-cluster', 'render-internal'],
    modules: aws ? ['vpc', 'eks', 'k8s-namespace', 'aurora', scenario === 'A02' ? 'elasticache' : 'redis-k8s'] : ['k8s-namespace', 'postgres-k8s', 'redis-k8s'],
    appKey: 'app-key', connectionKey: 'conn-key', kind: aws ? 'AWS' : 'KUBERNETES', resources: [], deployments: [], progress: [],
  };
  const row = (id, type, module, status = 'DESTROYED') => ({ id: `row-${id}`, resourceId: id, type, module, status });
  if (scenario === 'A01') fixture.resources = [row('ns', 'k8s-namespace', 'k8s-namespace'), row('db', 'postgres', 'postgres-k8s'), row('cache', 'redis', 'redis-k8s')];
  if (scenario === 'E02') {
    fixture.resources = [row('ns', 'k8s-namespace', 'k8s-namespace'), row('r-redis', 'redis', 'redis-k8s')];
    fixture.resourceDetail = { status: 'DESTROYED', module: { id: 'redis-k8s', revision: 'sha-1' }, lastAttempt: { status: 'SUCCEEDED' } };
    fixture.deployments = [{ id: 'd-failed' }]; fixture.progress = [{ status: 'FAILED', moduleId: 'redis-k8s', resourceInstanceId: 'row-r-redis' }];
  }
  if (scenario === 'A02') fixture.resources = [row('vpc', 'vpc', 'vpc'), row('eks', 'k8s-cluster', 'eks'), row('ns', 'k8s-namespace', 'k8s-namespace'), row('db', 'postgres', 'aurora'), row('cache', 'redis', 'elasticache')];
  if (scenario === 'A03') fixture.resources = [row('vpc', 'vpc', 'vpc'), row('eks', 'k8s-cluster', 'eks'), row('ns', 'k8s-namespace', 'k8s-namespace'), row('db', 'postgres', 'aurora'), row('cache', 'redis', 'redis-k8s')];
}

describe('Terraform v2 SQL reopen acceptance checks', () => {
  it('uses native choices for booleans and raw insertion for other typed overrides', async () => {
    const actions = [];
    const h = {
      choose: async (_field, label) => actions.push(['choose', label]),
      paste: async (_field, text) => actions.push(['paste', text]),
    };
    const field = {};
    const decoded = (definition, variable, substitutions) => {
      const entries = [...awsCatalog.definitions.common, ...awsCatalog.definitions.A02, ...awsCatalog.definitions.A03];
      const entry = entries.find((item) => item.id === definition)?.overrides?.find(([name]) => name === variable)?.[1];
      if (entry === undefined) throw new Error(`missing catalog override ${definition}.${variable}`);
      return decodeCatalogOverride(entry, substitutions);
    };

    await applyOverrideValue(h, field, decoded('cluster-aws-eks', 'ebs_csi_driver'));
    await applyOverrideValue(h, field, false);
    await applyOverrideValue(h, field, decoded('cluster-aws-eks', 'node_capacity_type'));
    await applyOverrideValue(h, field, decoded('cluster-aws-eks', 'node_disk_size'));
    await applyOverrideValue(h, field, decoded('cluster-aws-eks', 'public_access_cidrs', { EGRESS: '198.51.100.7/32' }));
    await applyOverrideValue(h, field, decoded('cluster-aws-eks', 'name'));

    expect(actions).toEqual([
      ['choose', 'true'], ['choose', 'false'], ['paste', 'SPOT'], ['paste', '20'],
      ['paste', '["198.51.100.7/32"]'], ['paste', '${context.infra.resourceName}'],
    ]);
  });

  it('turns JSON-typed AWS catalog entries into the raw typed form values', () => {
    expect(catalogFormValue('cluster-aws-eks', 'node_instance_type', { NODE_TYPE: 't4g.medium' })).toBe('t4g.medium');
    expect(catalogFormValue('cluster-aws-eks', 'node_capacity_type')).toBe('SPOT');
    expect(catalogFormValue('cluster-aws-eks', 'node_disk_size')).toBe('20');
    expect(catalogFormValue('cluster-aws-eks', 'ebs_csi_driver')).toBe('true');
    expect(catalogFormValue('cluster-aws-eks', 'public_access_cidrs', { EGRESS: '198.51.100.7/32' })).toBe('["198.51.100.7/32"]');
    expect(catalogFormValue('cluster-aws-eks', 'name')).toBe('${context.infra.resourceName}');
    expect(catalogFormValue('redis-eks-in-cluster', 'storage_class')).toBe('gp2');
  });
  it('keeps E02 image references raw and preserves the intentional invalid-map string', () => {
    const badImage = 'redis:7.4.7-alpine@sha256:' + 'a'.repeat(64);
    expect(formOverrideText(badImage)).toBe(badImage);
    expect(formOverrideText(JSON.stringify('not-a-map'))).toBe('"not-a-map"');
  });
  it.each(['A01', 'E01', 'E02', 'A02', 'A03'])('accepts exact %s catalog and post-cleanup history', async (scenario) => {
    setup(scenario); await start();
    await expect(verifyReopen({ base, scenario, applicationKey: 'app-key', connectionKey: 'conn-key' })).resolves.toMatchObject({ scenario });
  });
  it('fails closed on non-2xx reads without echoing response bodies', async () => {
    setup('A01'); fixture.failPath = '/api/v1/applications/app-key/environments/staging/resources'; await start();
    await expect(verifyReopen({ base, scenario: 'A01', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('reopen read failed (503)');
  });
  it('rejects malformed JSON and a missing expected connection', async () => {
    setup('E01'); fixture.malformedPath = '/api/v1/resource-types'; await start();
    await expect(verifyReopen({ base, scenario: 'E01', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('valid JSON');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('E01'); fixture.connectionKey = 'different'; await start();
    await expect(verifyReopen({ base, scenario: 'E01', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('Connection is not READY');
  });
  it('rejects malformed catalog shapes and extra or missing definitions', async () => {
    setup('A02'); fixture.types = {}; await start();
    await expect(verifyReopen({ base, scenario: 'A02', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('resource types response shape');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('A02'); fixture.definitions.pop(); await start();
    await expect(verifyReopen({ base, scenario: 'A02', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('resource definition catalog changed');
  });
  it('rejects wrong application scope and incomplete E02 history', async () => {
    setup('A01'); await start();
    await expect(verifyReopen({ base, scenario: 'A01', applicationKey: 'wrong-app', connectionKey: 'conn-key' })).rejects.toThrow('reopen read failed (404)');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('E02'); await start();
    fixture.progress = [];
    await expect(verifyReopen({ base, scenario: 'E02', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('failed Terraform attempt history');
    fixture.progress = [{ status: 'FAILED', moduleId: 'redis-k8s', resourceInstanceId: 'other-resource' }];
    await expect(verifyReopen({ base, scenario: 'E02', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('failed Terraform attempt history');
  });
  it('rejects missing, duplicate, unexpected, or live owned resources and E01 deployment records', async () => {
    setup('A01'); fixture.resources.pop(); await start();
    await expect(verifyReopen({ base, scenario: 'A01', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('resource history count');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('A01'); fixture.resources.push({ ...fixture.resources[0] }); await start();
    await expect(verifyReopen({ base, scenario: 'A01', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('resource history count');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('A02'); fixture.resources = []; await start();
    await expect(verifyReopen({ base, scenario: 'A02', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('resource history count');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('A03'); fixture.resources[0].status = 'READY'; await start();
    await expect(verifyReopen({ base, scenario: 'A03', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('is not DESTROYED');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('A03'); fixture.resources[0].type = 'foreign'; await start();
    await expect(verifyReopen({ base, scenario: 'A03', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('ownership history is missing');
    await new Promise((resolve) => server.close(resolve)); server = undefined;
    setup('E01'); fixture.deployments = [{ id: 'unexpected' }]; await start();
    await expect(verifyReopen({ base, scenario: 'E01', applicationKey: 'app-key', connectionKey: 'conn-key' })).rejects.toThrow('E01 unexpectedly persisted');
  });
});
