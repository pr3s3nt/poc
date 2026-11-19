import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { openConsole, readConsoleConfig, closeConsole } from '../../lib/console.mjs';

const config = readConsoleConfig();
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.') + '/'));
assert(process.env.ORCH_KUBECONFIG && process.env.ORCH_CONTEXT === 'kind-k8s-4f', 'Explicit read-only kind verification required');
assert(process.env.ORCH_TEMPLATE_REF && /^[a-f0-9]{40}$/.test(process.env.ORCH_TEMPLATE_REF), 'Pinned template commit required');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const receiptPath = join(evidence, 'setup.json');
const receipt = existsSync(receiptPath) ? JSON.parse(readFileSync(receiptPath)) : { definitions: [] };
const save = () => writeFileSync(receiptPath, JSON.stringify(receipt, null, 2), { mode: 0o600 });
const session = await openConsole(config); const { page } = session;
const get = async path => { const r = await page.request.get(config.baseURL + '/api/v1' + path); assert(r.ok(), 'Read failed: ' + path); return r.json(); };
const post = async (path, data) => { const r = await page.request.post(config.baseURL + '/api/v1' + path, { data, headers: { Origin: config.baseURL } }); const b = await r.json(); assert(r.ok(), `${path}: HTTP ${r.status()} ${b.code ?? ''}`); return b; };
try {
  const accounts = (await get('/driver-accounts')).accounts;
  if (!receipt.account) {
    assert(!accounts.some(a => a.name === 'Definition editor kind'), 'Unexpected preexisting trial account');
    const kubeconfig = readFileSync(process.env.ORCH_KUBECONFIG, 'utf8');
    const created = await post('/driver-accounts/internal', { name: 'Definition editor kind', kubeconfig, context: process.env.ORCH_CONTEXT });
    assert.equal(created.status, 'READY'); receipt.account = created.key; save();
  } else assert(accounts.some(a => a.key === receipt.account && a.status === 'READY'));
  const definitions = (await get('/resource-definitions')).resourceDefinitions;
  const types = (await get('/resource-types')).resourceTypes;
  if (!types.some(t => t.key === 'endpoint')) await post('/resource-types', { key: 'endpoint', inputs: [{ name: 'name', type: 'string', required: true }, { name: 'port', type: 'string', required: true }, { name: 'path', type: 'string' }], outputs: [{ name: 'hostname', type: 'string', required: true }, { name: 'url', type: 'string', required: true }] });
  const criteria = [{ class: 'definition-editor' }];
  const candidates = [
    { key: 'definition-editor-cluster', name: 'Cluster – ví dụ editor', resourceType: 'k8s-cluster', driverType: 'existing-cluster', driverAccount: receipt.account, driverInputs: { values: { variables: {} } }, provision: {}, criteria },
    { key: 'definition-editor-endpoint', name: 'Endpoint – ví dụ editor', resourceType: 'endpoint', driverType: 'endpoint', driverInputs: { values: { variables: { rootDomain: 'apps.example.test', scheme: 'http', gatewayName: 'shared-gateway', gatewayNamespace: 'gateway-system' } } }, provision: {}, criteria },
    { key: 'definition-editor-namespace', name: 'Namespace – ví dụ editor', resourceType: 'k8s-namespace', driverType: 'terraform', driverInputs: { values: { source: { url: 'https://github.com/pr3s3nt/orchestrator-workload-templates', ref: process.env.ORCH_TEMPLATE_REF, path: 'terraform/namespace' }, variables: { name: '${context.env.namespace}' }, input_mapping: {}, output_mapping: { name: 'name' }, provider_bindings: { kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.definition-editor#environments.@app.@env' } } } }, provision: {}, criteria },
  ];
  if (!receipt.templateSource) {
    const result = await post('/template-sources/inspect', { url: 'https://github.com/pr3s3nt/orchestrator-workload-templates', ref: process.env.ORCH_TEMPLATE_REF, path: 'templates/standard' });
    receipt.templateSource = result.templateSource.id; save();
  }
  candidates.push({ key: 'definition-editor-workload', name: 'Workload – ví dụ editor', resourceType: 'workload', driverType: 'template', driverInputs: { values: { variables: { template_source: receipt.templateSource, template_inputs: {}, namespace: "${resources['k8s-namespace.definition-editor#environments.@app.@env'].outputs.name}" } } }, provision: {}, criteria });
  for (const candidate of candidates) {
    if (receipt.definitions.includes(candidate.key)) { assert(definitions.some(d => d.key === candidate.key)); continue; }
    assert(!definitions.some(d => d.key === candidate.key), 'Unexpected preexisting Definition');
    const prepared = await post('/resource-definitions/import/prepare', candidate);
    await post('/resource-definitions', prepared.definition);
    receipt.definitions.push(candidate.key); save();
  }
  console.log('PASS: isolated real catalog fixtures prepared; kind account verified read-only; no Application or deployment created');
} finally { await closeConsole(session, config); }
