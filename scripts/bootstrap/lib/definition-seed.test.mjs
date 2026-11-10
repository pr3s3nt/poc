import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadDefinitionConfig, seedDefinitions, substitute } from './definition-seed.mjs';

const revision = 'a'.repeat(40), sha = 'b'.repeat(64);
function config(t, patch = {}) {
 const dir = mkdtempSync(join(tmpdir(), 'definition-seed-'));
 t.after(() => rmSync(dir, { recursive: true, force: true }));
 writeFileSync(join(dir, 'password'), 'private-password', { mode: 0o600 });
 const c = { schemaVersion: 1, console: { url: 'http://127.0.0.1:3070', username: 'pe', passwordFile: 'password', expectedOrganization: 'acme' }, repository: { url: 'https://git.internal/platform/resources', ref: revision }, parameters: { driverAccountKey: 'existing-account' }, definitions: [
  { key: 'namespace', name: 'Namespace', resourceType: 'k8s-namespace', driverType: 'terraform', modulePath: 'terraform/namespace', driverInputs: { values: { variables: { name: '${context.env.namespace}' } } }, criteria: [{}], provision: {} },
  { key: 'workload', name: 'Workload', resourceType: 'workload', driverType: 'template', templatePath: 'templates/standard', driverInputs: { values: { variables: {} } }, criteria: [{}], provision: {} },
 ], ...patch };
 const file = join(dir, 'input.yaml'); writeFileSync(file, JSON.stringify(c), { mode: 0o600 });
 return { c, file, loaded: () => loadDefinitionConfig(file) };
}
function server(c, { wrongPin = false, conflict = false, missingType = false, incompatibleType = false, role = 'PLATFORM_ENGINEER', transport = false } = {}) {
 const calls = [], definitions = conflict ? [{ ...structuredClone(c.definitions[1]), driverType: 'existing-cluster' }] : [];
 const types = missingType ? [] : [{ key: 'k8s-namespace', ...(incompatibleType ? { inputs: [{ name: 'legacy-input' }] } : {}) }, { key: 'workload' }];
 const source = { sourceId: 'server-source', moduleId: 'server-module', revision, subdirectory: 'terraform/namespace', contentSha256: sha, repositoryUrl: c.repository.url };
 const fetcher = async (raw, options) => {
  const path = new URL(raw).pathname.replace('/api/v1', '');
  const body = options.body ? JSON.parse(options.body) : undefined;
  calls.push({ method: options.method, path, body });
  assert.equal(options.redirect, 'error');
  if (transport && path.endsWith('/inspect')) throw Error('private-provider-secret');
  if (path === '/auth/sign-in') return Response.json({ user: { OrganizationKey: 'acme', Role: role } }, { headers: { 'set-cookie': 'session=private-cookie; HttpOnly' } });
  assert.equal(options.headers.Cookie, 'session=private-cookie');
  if (path === '/auth/sign-out') return new Response(null, { status: 204 });
  if (path === '/resource-types') {
   if (options.method === 'POST') { types.push(body); return Response.json(body); }
   return Response.json({ resourceTypes: types });
  }
  if (path === '/driver-accounts' && options.method === 'GET') return Response.json({ accounts: [{ key: 'existing-account', type: 'INTERNAL', status: 'READY' }] });
  if (path === '/resource-definitions') {
   if (options.method === 'POST') definitions.push(body);
   return Response.json({ resourceDefinitions: definitions });
  }
  if (path === '/terraform-modules/inspect') {
   assert.deepEqual(body, { ...c.repository, path: 'terraform/namespace', resourceType: 'k8s-namespace' });
   return Response.json({ capability: { resourceType: 'k8s-namespace', driverType: 'terraform', source: { ...source, ...(wrongPin ? { revision: 'c'.repeat(40) } : {}) } } });
  }
  if (path === '/template-sources/inspect') return Response.json({ templateSource: { id: 'exact-current-renderer-source', status: 'AVAILABLE', source: { url: c.repository.url, revision, path: 'templates/standard', contentSha256: sha } } });
  throw Error('Unexpected API request');
 };
 return { fetcher, calls, definitions, types };
}

test('first seed and unchanged rerun use server pins without writing prerequisites', async t => {
 const c = config(t).loaded(), s = server(c), report = [];
 await seedDefinitions(c, { fetcher: s.fetcher, report: x => report.push(x) });
 await seedDefinitions(c, { fetcher: s.fetcher, report: x => report.push(x) });
 assert.equal(s.calls.filter(x => x.method === 'POST' && x.path === '/resource-definitions').length, 2);
 assert.equal(s.definitions[0].driverInputs.values.source.module_id, 'server-module');
 assert.equal(s.definitions[0].driverInputs.values.variables.name, '${context.env.namespace}');
 assert.equal(s.definitions[1].driverInputs.values.variables.template_source, 'exact-current-renderer-source');
 assert.ok(report.includes('Definition workload REUSED'));
 assert.ok(!report.join().includes('private-'));
 assert.equal(s.calls.at(-1).path, '/auth/sign-out');
});

test('a later conflicting Definition prevents all Definition writes', async t => {
 const c = config(t).loaded(), s = server(c, { conflict: true });
 await assert.rejects(seedDefinitions(c, { fetcher: s.fetcher }), /CONFLICT/);
 assert.equal(s.calls.filter(x => x.method === 'POST' && x.path === '/resource-definitions').length, 0);
 assert.equal(s.calls.at(-1).path, '/auth/sign-out');
});

test('wrong inspection pin and unavailable prerequisites stop before creating Definitions', async t => {
 for (const options of [{ wrongPin: true }, { missingType: true }, { incompatibleType: true }, { role: 'DEVELOPER' }]) {
  const c = config(t).loaded(), s = server(c, options);
  await assert.rejects(seedDefinitions(c, { fetcher: s.fetcher }));
  assert.equal(s.calls.filter(x => x.path === '/resource-definitions' && x.method === 'POST').length, 0);
  assert.equal(s.calls.at(-1).path, '/auth/sign-out');
 }
});

test('unknown inspection outcome is never replayed and provider secrets are suppressed', async t => {
 const c = config(t).loaded(), s = server(c, { transport: true });
 await assert.rejects(seedDefinitions(c, { fetcher: s.fetcher }), e => e.message.includes('read server state') && !e.message.includes('private-provider'));
 assert.equal(s.calls.filter(x => x.path.endsWith('/inspect')).length, 1);
});

test('loader rejects mutable refs, embedded credentials, duplicate keys and source overrides', t => {
 for (const patch of [
  { repository: { url: 'https://git.internal/repo', ref: 'main' } },
  { repository: { url: 'https://token@git.internal/repo', ref: revision } },
  { accounts: [] },
 ]) assert.throws(config(t, patch).loaded);
 const f = config(t); f.c.definitions.push(f.c.definitions[0]);
 writeFileSync(f.file, JSON.stringify(f.c)); assert.throws(f.loaded, /duplicate/);
 const override = config(t); override.c.definitions[0].driverInputs.values.source = {};
 writeFileSync(override.file, JSON.stringify(override.c)); assert.throws(override.loaded, /inspection/);
 const duplicate = config(t); writeFileSync(duplicate.file, 'schemaVersion: 1\nschemaVersion: 1\n'); assert.throws(duplicate.loaded, /YAML/);
 assert.throws(() => substitute('${seed.missing}', {}), /Missing seed parameter/);
});

test('published input declares seven Definitions and just one Git URL', t => {
 const raw = readFileSync(new URL('../definitions.example.yaml', import.meta.url), 'utf8');
 const f = config(t); writeFileSync(f.file, raw.replace('https://console.company.example', 'http://127.0.0.1:3070').replace('./credentials/password', 'password'));
 const c = f.loaded();
 assert.equal(c.definitions.length, 7);
 assert.equal(c.resourceTypes.length, 7);
 assert.deepEqual(new Set(c.resourceTypes.map(t => t.key)), new Set(c.definitions.map(d => d.resourceType)));
 assert.equal(c.definitions.filter(d => d.modulePath).length, 5);
 assert.equal(c.definitions.filter(d => d.templatePath).length, 1);
 assert.equal((raw.match(/https:\/\/github.com/g) ?? []).length, 1);
});

test('missing Types are created before inspection; rerun preserves Types and Definitions', async t => {
 const c = config(t, { resourceTypes: [{ key: 'k8s-namespace', inputs: [], outputs: [] }, { key: 'workload', inputs: [], outputs: [] }] }).loaded();
 const s = server(c, { missingType: true }), report = [];
 await seedDefinitions(c, { fetcher: s.fetcher, report: x => report.push(x) });
 const createPositions = s.calls.flatMap((x,i) => x.path === '/resource-types' && x.method === 'POST' ? [i] : []);
 assert.equal(createPositions.length, 2);
 assert.ok(createPositions.every(i => i < s.calls.findIndex(x => x.path.endsWith('/inspect'))));
 await seedDefinitions(c, { fetcher: s.fetcher, report: x => report.push(x) });
 assert.equal(s.calls.filter(x => x.path === '/resource-types' && x.method === 'POST').length, 2);
 assert.equal(s.definitions.length, 2);
 assert.ok(report.includes('ResourceType workload REUSED'));
});

test('later Type conflict and invalid Definition mapping prevent every Type write', async t => {
 const c = config(t, { resourceTypes: [{ key: 'k8s-namespace', inputs: [], outputs: [] }, { key: 'workload', inputs: [], outputs: [] }] }).loaded();
 for (const mismatch of ['type','mapping']) {
  const s = server(c, { missingType: true });
  if (mismatch === 'type') s.types.push({ key: 'workload', inputs: [{ name: 'legacy', type: 'string' }], outputs: [] });
  else c.resourceTypes[0].inputs.push({ name: 'unmapped', type: 'string' });
  await assert.rejects(seedDefinitions(c, { fetcher: s.fetcher }), /CONFLICT|mappings differ/);
  assert.equal(s.calls.filter(x => x.path !== '/auth/sign-in' && x.path !== '/auth/sign-out' && x.method === 'POST').length, 0);
 }
});

test('invalid or duplicate Type contracts fail locally', t => {
 for (const resourceTypes of [null, {}, [{ key: 'db', inputs: [], outputs: [] }, { key: 'db', inputs: [], outputs: [] }], [{ key: 'db', inputs: [{ name: 'size', type: 'unsupported' }], outputs: [] }], [{ key: 'db', inputs: [], outputs: [{ name: 'password', type: 'string', secret: 'true' }] }]]) {
  const f = config(t, { resourceTypes });
  assert.throws(f.loaded);
 }
});
