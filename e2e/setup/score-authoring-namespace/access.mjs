import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';

// Optional, explicitly enabled development setup. It only grants the existing
// Fleet subject bounded rights in the product-created, receipt-owned namespace.
export async function ensureScoreNamespaceAccess(page, consoleConfig, receipt, save) {
 const kubeconfig = process.env.ORCH_SCORE_KUBECONFIG;
 if (!kubeconfig) return;
 const context = process.env.ORCH_SCORE_CONTEXT;
 assert(context, 'ORCH_SCORE_CONTEXT is required with namespace setup');
 const response = await page.request.get(consoleConfig.baseURL + '/api/v1/applications/' + receipt.appId);
 assert(response.ok(), 'Cannot read receipt-owned Application');
 const { application } = await response.json();
 assert.equal(application.name, receipt.name, 'Application receipt mismatch');
 const config = {
  runtime: { profile: 'dev-test', clusterKubeconfigFile: kubeconfig, timeoutMs: 300_000 },
  application: { environment: 'staging' },
  platform: { account: { context }, delivery: { namespaceAccess: { enabled: true, rulesFile: fileURLToPath(new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json', import.meta.url)) } } },
 };
 const state = receipt.namespaceAccess ?? { deliverySubject: { name: process.env.ORCH_SCORE_FLEET_ACCOUNT ?? 'orchestrator-delivery', namespace: process.env.ORCH_SCORE_FLEET_NAMESPACE ?? 'cattle-fleet-system' } };
 const runtime = { state, save() { receipt.namespaceAccess = state; save(); }, record(step, status) { console.log(status, step); } };
 await ensureNamespaceAccess({ page }, config, application, runtime);
 const namespace = application.environments.find(x => x.key === 'staging').namespaceIdentity;
 const args = ['--kubeconfig', kubeconfig, '--context', context, '-n', namespace];
 const identities = {};
 for (const kind of ['role', 'rolebinding']) {
  const object = JSON.parse(execFileSync('kubectl', [...args, 'get', kind, 'bootstrap-fleet-access', '-o', 'json'], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }));
  assert.equal(object.metadata.labels['bootstrap.orchestrator.dev/application'], receipt.appId);
  identities[kind] = object.metadata.uid;
 }
 if (state.rbacIdentities) assert.deepEqual(identities, state.rbacIdentities, 'Setup RBAC identity changed');
 state.rbacIdentities = identities; state.namespace = namespace; runtime.save();
}
