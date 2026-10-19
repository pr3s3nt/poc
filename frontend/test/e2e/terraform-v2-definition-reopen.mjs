// Read-only SQL reopen check of the Definition-driven AWS scenarios (V05, A02, A03) after cleanup and a product restart on the same
// database. The authored catalog (definition-catalog.json, the file the scenario typed), the AWS account, the absence of any
// Environment connection and the DESTROYED history of every owned resource are read back through the product API. Nothing is written.
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { FIXTURE_PASSWORD } from './terraform-v2-steps.mjs';

const candidates = [resolve(process.env.ORCH_REPO_ROOT ?? process.cwd(), 'backend/test/integration/v2run/testdata/definition-catalog.json'), resolve(process.cwd(), 'backend/test/integration/v2run/testdata/definition-catalog.json'), resolve(process.cwd(), '../backend/test/integration/v2run/testdata/definition-catalog.json')];
const catalogPath = candidates.find(existsSync);
if (!catalogPath) throw new Error('the Definition catalog file is unavailable');
const catalog = JSON.parse(readFileSync(catalogPath, 'utf8'));
const sorted = (items) => [...items].sort();
const equal = (a, b) => JSON.stringify(sorted(a)) === JSON.stringify(sorted(b));
const array = (value, label) => {
  if (!Array.isArray(value)) throw new Error(`${label} response shape is invalid`);
  return value;
};

/** The (resource type, module) pairs each scenario owns: exactly one instance of each, all DESTROYED after cleanup. */
export const EXPECTED_RESOURCES = {
  V05: [['vpc', 'vpc'], ['k8s-cluster', 'eks'], ['k8s-namespace', 'k8s-namespace'], ['postgres', 'postgres-k8s'], ['redis', 'redis-k8s']],
  A02: [['vpc', 'vpc'], ['k8s-cluster', 'eks'], ['k8s-namespace', 'k8s-namespace'], ['postgres', 'aurora'], ['redis', 'elasticache']],
  A03: [['vpc', 'vpc'], ['k8s-cluster', 'eks'], ['k8s-namespace', 'k8s-namespace'], ['postgres', 'aurora'], ['redis', 'redis-k8s']],
};
// The current resource-list API omits clusters. This reader checks its existing
// projection without changing public deletion eligibility. Independent SQL below
// proves every owned instance, including EKS, survives as DESTROYED history.
export const expectedAPIResources = (scenario) => EXPECTED_RESOURCES[scenario].filter(([type]) => type !== 'k8s-cluster');
export const expectedDefinitionKeys = (scenario) => [...catalog.definitions.common, ...catalog.definitions[scenario]].map((d) => d.id).concat(['render-eks']);

export async function verifyDefinitionReopen({ base, scenario, applicationKey, accountKey, persistedInstances, fetchImpl = fetch }) {
  if (!Object.hasOwn(EXPECTED_RESOURCES, scenario) || !applicationKey || !accountKey) throw new Error('scenario scope is incomplete');
  const instances = array(persistedInstances, 'technical SQL resource history').map((r) => ({ ...r, type: r?.descriptor?.split('.')[0] }));
  if (instances.length !== EXPECTED_RESOURCES[scenario].length) throw new Error(`${scenario} full SQL resource history count does not match its owned plan`);
  for (const [type, module] of EXPECTED_RESOURCES[scenario]) {
    const matches = instances.filter((r) => r.type === type && r.module === module);
    if (matches.length !== 1 || !matches[0].id || !matches[0].descriptor?.includes('#') || !matches[0].createdByRun || !matches[0].stateKey12 || !matches[0].scope || matches[0].status !== 'DESTROYED') throw new Error(`${scenario} full SQL ${type}/${module} identity or DESTROYED history is incomplete`);
  }
  if (new Set(instances.map((r) => r.id)).size !== instances.length) throw new Error('full SQL resource history has duplicate identities');
  const origin = new URL(base).origin;
  const signIn = await fetchImpl(`${origin}/api/v1/auth/sign-in`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ username: 'platform-engineer', password: FIXTURE_PASSWORD }) });
  if (!signIn.ok) throw new Error('sign-in after reopen failed');
  const cookie = (signIn.headers.getSetCookie?.() ?? []).map((value) => value.split(';')[0]).join('; ');
  if (!cookie) throw new Error('sign-in after reopen did not establish a session');
  const get = async (path) => {
    const response = await fetchImpl(`${origin}/api/v1${path}`, { headers: { cookie } });
    if (!response.ok) throw new Error(`reopen read failed (${response.status})`);
    try { return await response.json(); } catch { throw new Error('reopen response was not valid JSON'); }
  };
  const types = array((await get('/resource-types')).resourceTypes, 'resource types').map((t) => t?.key);
  if (!equal(types, catalog.types.map((t) => t.key))) throw new Error('resource type catalog changed after reopen');
  const definitions = array((await get('/resource-definitions')).resourceDefinitions, 'resource definitions');
  if (!equal(definitions.map((d) => d?.key), expectedDefinitionKeys(scenario))) throw new Error('scenario resource definition catalog changed after reopen');
  for (const def of definitions.filter((d) => d.driverType === 'terraform' && d.driverAccount)) {
    if (def.driverAccount !== accountKey) throw new Error('an AWS Definition lost its own Driver Account after reopen');
  }
  const capabilities = array((await get('/resource-definition-capabilities')).capabilities, 'definition capabilities');
  for (const module of new Set([...catalog.definitions.common, ...catalog.definitions[scenario]].map((d) => d.module))) {
    if (!capabilities.some((c) => c?.driverType === 'terraform' && c.module === module && /^[a-f0-9]{64}$/i.test(c.source?.contentSha256 ?? ''))) throw new Error(`verified module pin ${module} is missing after reopen`);
  }
  const account = array((await get('/driver-accounts')).accounts, 'accounts').find((a) => a?.key === accountKey);
  if (!account || account.type !== 'AWS' || account.status !== 'READY') throw new Error('the AWS account is not READY after reopen');
  const apps = array((await get('/applications')).applications, 'applications');
  const environment = apps.find((a) => a?.key === applicationKey)?.environments?.find((e) => e?.key === 'staging');
  if (!environment || environment.connectionKey || environment.executionProfile) throw new Error('the Environment gained a deployment Connection or execution profile');
  const scope = `/applications/${encodeURIComponent(applicationKey)}/environments/staging`;
  const resources = array((await get(`${scope}/resources`)).resources, 'resources');
  const deployments = array((await get(`${scope}/deployments`)).deployments, 'deployments');
  const expected = expectedAPIResources(scenario);
  if (resources.length !== expected.length) throw new Error(`${scenario} resource history count: expected ${expected.length} API rows, got ${resources.length}; full SQL instances ${instances.length}`);
  for (const [type, module] of expected) {
    const matches = resources.filter((r) => r?.type === type && r.module === module);
    if (matches.length !== 1 || !matches[0].id || !matches[0].resourceId) throw new Error(`${scenario} ${type}/${module} ownership history is missing, duplicated, or incomplete`);
    if (matches[0].status !== 'DESTROYED') throw new Error(`${scenario} ${type}/${module} is not DESTROYED after cleanup`);
  }
  if (!equal(resources.map((r) => r.id), instances.filter((r) => r.type !== 'k8s-cluster').map((r) => r.id))) throw new Error('API resource identities differ from the complete SQL history');
  // deploy, configuration update, post-restart operation (a refused tie leaves no deployment record).
  if (deployments.filter((d) => d?.status === 'SUCCEEDED').length < 3) throw new Error(`${scenario} deployment history (deploy, update, post-restart operation) is incomplete after reopen`);
  return { scenario, types: types.length, definitions: definitions.length, resources: instances.length, apiResources: resources.length, deployments: deployments.length };
}

if (process.argv[1] && import.meta.url.startsWith('file:') && fileURLToPath(import.meta.url) === process.argv[1]) {
  const [base, scenario, applicationKey, accountKey, inventoryPath] = process.argv.slice(2);
  try {
    const persistedInstances = JSON.parse(readFileSync(inventoryPath, 'utf8'));
    const result = await verifyDefinitionReopen({ base, scenario, applicationKey, accountKey, persistedInstances });
    console.log(`sql-reopen ok: ${result.scenario}, ${result.types} types, ${result.definitions} definitions, ${result.resources} SQL resources DESTROYED (${result.apiResources} public API rows) and ${result.deployments} deployments served after restart`);
  } catch (error) {
    console.error(String(error.message ?? error));
    process.exit(1);
  }
}
