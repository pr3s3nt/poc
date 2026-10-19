// Read-only SQL reopen check of the V04 run after the final product restart on the same PostgreSQL database:
// the authored catalog, the Internal account, the deployment history (including the retained FAILED negative) and
// the DESTROYED resources are read back through the product API. Nothing is written.
import { FIXTURE_PASSWORD } from './terraform-v2-steps.mjs';
import { FULL_APP_TYPES } from './full-app-flows.mjs';
import { catalogIds } from './v04-internal-full.mjs';

const sorted = (items) => [...items].sort();
const same = (actual, expected) => JSON.stringify(sorted(actual)) === JSON.stringify(sorted(expected));
const array = (value, label) => {
  if (!Array.isArray(value)) throw new Error(`${label} response shape is invalid`);
  return value;
};

// reached: the assertion IDs the scenario recorded. A run that failed early is read back in PARTIAL mode so the check
// never blames the product for a step that never ran; --verify-evidence still enforces every mandatory assertion.
export async function verifyReopen({ base, input, accountKey, fetchImpl = fetch, reached = null, historicalNegatives = false }) {
  const has = (id) => reached === null || reached.has(id);
  const complete = reached === null || reached.has('cleanup.namespaces-absent');
  const origin = new URL(base).origin;
  const signIn = await fetchImpl(`${origin}/api/v1/auth/sign-in`, {
    method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ username: 'platform-engineer', password: FIXTURE_PASSWORD }),
  });
  if (!signIn.ok) throw new Error('sign-in after reopen failed');
  const cookie = (signIn.headers.getSetCookie?.() ?? []).map((value) => value.split(';')[0]).join('; ');
  if (!cookie) throw new Error('sign-in after reopen did not establish a session');
  const get = async (path) => {
    const response = await fetchImpl(`${origin}/api/v1${path}`, { headers: { cookie } });
    if (!response.ok) throw new Error(`reopen read failed (${response.status})`);
    return response.json();
  };
  const ids = catalogIds(input);
  const types = array((await get('/resource-types')).resourceTypes, 'resource types').map((t) => t?.key);
  if (!same(types, FULL_APP_TYPES.map((t) => t.key))) throw new Error('resource type catalog changed after reopen');
  const definitions = array((await get('/resource-definitions')).resourceDefinitions, 'resource definitions');
  const keys = definitions.map((d) => d?.key);
  const expectedKeys = [ids.cluster, ids.namespace, ids.postgres, ids.redis, ids.template];
  if (historicalNegatives && has('negative.e01-definition-pinned')) expectedKeys.push(`postgres-invalid-${input.runId}`);
  if (historicalNegatives && has('negative.failure-definition-pinned')) expectedKeys.push(`redis-readiness-${input.runId}`);
  if (complete ? !same(keys, expectedKeys) : keys.some((key) => !expectedKeys.includes(key))) throw new Error('resource definition catalog changed after reopen');
  const cluster = definitions.find((d) => d.key === ids.cluster);
  if (cluster.driverType !== 'existing-cluster' || cluster.driverAccount !== accountKey || (complete ? cluster.version !== 2 : ![1, 2].includes(cluster.version))) throw new Error('the cluster Definition (account, version) changed after reopen');
  const template = definitions.find((d) => d.key === ids.template);
  if (has('ui.definition.workload') && (!template || !template.provision || Object.keys(template.provision).length !== 1)) throw new Error('the companion rule was lost after reopen');
  const account = array((await get('/driver-accounts')).accounts, 'accounts').find((a) => a?.key === accountKey);
  if (!account || account.status !== 'READY' || account.type !== 'INTERNAL') throw new Error('the Internal account is not READY after reopen');
  const scope = `/applications/${encodeURIComponent(input.applicationKey)}/environments/staging`;
  const history = array((await get(`${scope}/deployments`)).deployments, 'deployments');
  const succeeded = history.filter((d) => d?.status === 'SUCCEEDED');
  const failed = history.filter((d) => d?.status === 'FAILED');
  // deploy, frontend update, wrong config, restored config, post-restart update, backend removal, worker+frontend removal.
  const operationAssertions = historicalNegatives ? ['ready.app', 'update.frontend-rollout', 'config.truthful-fail', 'config.recovers', 'restart.operation-after', 'cleanup.shared-kept-while-consumer', 'cleanup.workloads-removed'] : ['ready.app', 'update.frontend-rollout', 'update.config-applied', 'restart.operation-after', 'cleanup.shared-kept-while-consumer', 'cleanup.workloads-removed'];
  const wantSucceeded = reached === null ? operationAssertions.length : operationAssertions.filter((id) => reached.has(id)).length;
  if (succeeded.length < wantSucceeded) throw new Error('the deployment history is incomplete after reopen');
  for (const [assertion, workload] of [['negative.e01-consumer-blocked', 'probe-e01'], ['negative.dependency-failure-blocks-consumer', 'probe']]) {
    if (!historicalNegatives || !has(assertion)) continue;
    const record = failed.find((d) => d.workloadId === workload);
    if (!record?.id) throw new Error(`the retained FAILED ${workload} deployment is missing after reopen`);
    const detail = await get(`${scope}/deployments/${record.id}`);
    if (!Array.isArray(detail.blocked) || !detail.blocked.some((b) => b.consumer === `workload.default#modules.${workload}`)) throw new Error(`the blocked-consumer evidence of ${workload} was lost after reopen`);
  }
  const environment = array((await get('/applications')).applications, 'applications').find((a) => a?.key === input.applicationKey)?.environments?.find((e) => e?.key === 'staging');
  if (!environment || environment.connectionKey) throw new Error('the Environment gained a deployment Connection');
  const resources = array((await get(`${scope}/resources`)).resources, 'resources');
  const live = resources.filter((r) => ['READY', 'UNREFERENCED', 'PROVISIONING', 'DESTROYING'].includes(r?.status));
  if (complete && live.length !== 0) throw new Error('a cleaned-up resource is still active after reopen');
  return `${complete ? '' : 'PARTIAL run (the scenario stopped early; only the reached steps were read back): '}reopen ok: ${types.length} types, ${keys.length} definitions (cluster v${cluster.version}), account ${accountKey} READY, ${succeeded.length} succeeded and ${failed.length} retained failed deployments, ${complete ? 'all resources destroyed' : `${live.length} resource(s) still active (not yet cleaned up by this stopped run)`}`;
}

async function main() {
  const [base, inputPath, accountKey, assertionsPath] = process.argv.slice(2);
  if (!base || !inputPath || !accountKey) throw new Error('usage: v04-reopen.mjs BASE INPUT.json ACCOUNT_KEY [ASSERTIONS.json]');
  const { readFileSync } = await import('node:fs');
  const reached = assertionsPath ? new Set(JSON.parse(readFileSync(assertionsPath, 'utf8')).filter((a) => a.pass).map((a) => a.id)) : null;
  console.log(await verifyReopen({ base, input: JSON.parse(readFileSync(inputPath, 'utf8')), accountKey, reached }));
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => { console.error(String(error?.message ?? error).split('\n')[0]); process.exit(1); });
}
