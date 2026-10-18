// Read-only SQL reopen check of the V03 run after the final product restart on the same
// PostgreSQL database: the authored catalog, the Internal account, the deployment history and the
// cleaned-up resources are all read back through the product API. Nothing is written.
import { FIXTURE_PASSWORD } from './terraform-v2-steps.mjs';
import { V03_TYPES, catalogIds } from './v03-internal-frontend.mjs';

const sorted = (items) => [...items].sort();
const same = (actual, expected) => JSON.stringify(sorted(actual)) === JSON.stringify(sorted(expected));
const array = (value, label) => {
  if (!Array.isArray(value)) throw new Error(`${label} response shape is invalid`);
  return value;
};

// reached: the assertion IDs the scenario recorded (assertions.json). A run that failed early is read back in PARTIAL mode: only what the
// scenario actually reached is required, so the check never blames the product for a step that never ran. The mandatory success assertions
// are enforced by --verify-evidence and are not replaced by this check.
export async function verifyReopen({ base, input, accountKey, fetchImpl = fetch, reached = null }) {
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
  if (!same(types, V03_TYPES.map((t) => t.key))) throw new Error('resource type catalog changed after reopen');
  const definitions = array((await get('/resource-definitions')).resourceDefinitions, 'resource definitions');
  const keys = definitions.map((d) => d?.key);
  const expectedKeys = [ids.cluster, ids.namespace, ids.template, ...(has('negative.tied-cluster-definitions.matcher') ? [ids.rivalA, ids.rivalB] : [])];
  if (complete ? !same(keys, expectedKeys) : keys.some((key) => ![ids.cluster, ids.namespace, ids.template, ids.rivalA, ids.rivalB].includes(key))) throw new Error('resource definition catalog changed after reopen');
  if (keys.includes('builtin-existing-cluster')) throw new Error('a builtin cluster Definition was admitted');
  const cluster = definitions.find((d) => d.key === ids.cluster);
  if (cluster.driverType !== 'existing-cluster' || cluster.driverAccount !== accountKey || (complete ? cluster.version !== 2 : ![1, 2].includes(cluster.version))) throw new Error('the cluster Definition (account, version) changed after reopen');
  const template = definitions.find((d) => d.key === ids.template);
  if (has('ui.definition.workload') && (!template || !template.provision || Object.keys(template.provision).length !== 1)) throw new Error('the companion rule was lost after reopen');
  const account = array((await get('/driver-accounts')).accounts, 'accounts').find((a) => a?.key === accountKey);
  if (!account || account.status !== 'READY' || account.type !== 'INTERNAL') throw new Error('the Internal account is not READY after reopen');
  const scope = `/applications/${encodeURIComponent(input.applicationKey)}/environments/staging`;
  const history = array((await get(`${scope}/deployments`)).deployments, 'deployments');
  const succeeded = history.filter((d) => d?.status === 'SUCCEEDED');
  const wantSucceeded = reached === null ? 4 : ['ready.workload', 'update.rollout', 'restart.operation-after', 'cleanup.workload-removed'].filter((id) => reached.has(id)).length;
  if (succeeded.length < wantSucceeded) throw new Error('the deployment history (deploy, update, post-restart update, removal) is incomplete after reopen');
  const environment = array((await get('/applications')).applications, 'applications').find((a) => a?.key === input.applicationKey)?.environments?.find((e) => e?.key === 'staging');
  if (!environment || environment.connectionKey) throw new Error('the Environment gained a deployment Connection');
  const resources = array((await get(`${scope}/resources`)).resources, 'resources');
  const live = resources.filter((r) => ['READY', 'UNREFERENCED', 'PROVISIONING', 'DESTROYING'].includes(r?.status) && /k8s-namespace/.test(r?.descriptor ?? ''));
  if (complete && live.length !== 0) throw new Error('a cleaned-up namespace is still active after reopen');
  return `${complete ? '' : 'PARTIAL run (the scenario stopped early; only the reached steps were read back): '}reopen ok: ${types.length} types, ${keys.length} definitions (cluster v${cluster.version}), account ${accountKey} READY, ${succeeded.length} succeeded deployments, ${complete ? 'namespaces destroyed' : `${live.length} namespace resource(s) still active (not yet cleaned up by this stopped run)`}`;
}

async function main() {
  const [base, inputPath, accountKey, assertionsPath] = process.argv.slice(2);
  if (!base || !inputPath || !accountKey) throw new Error('usage: v03-reopen.mjs BASE INPUT.json ACCOUNT_KEY');
  const { readFileSync } = await import('node:fs');
  const reached = assertionsPath ? new Set(JSON.parse(readFileSync(assertionsPath, 'utf8')).filter((a) => a.pass).map((a) => a.id)) : null;
  console.log(await verifyReopen({ base, input: JSON.parse(readFileSync(inputPath, 'utf8')), accountKey, reached }));
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => { console.error(String(error?.message ?? error).split('\n')[0]); process.exit(1); });
}
