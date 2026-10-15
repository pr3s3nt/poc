// Read-only SQL reopen check after cleanup and product restart. Catalog entries
// come from the same UI definitions/AWS fixture used by the scenarios.
import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { K8S_ACCEPTANCE_DEFINITIONS, TYPES, FIXTURE_PASSWORD } from './terraform-v2-steps.mjs';

const catalogCandidates = [resolve(process.env.ORCH_REPO_ROOT ?? process.cwd(), 'backend/test/integration/v2run/testdata/aws-catalog.json'), resolve(process.cwd(), 'backend/test/integration/v2run/testdata/aws-catalog.json'), resolve(process.cwd(), '../backend/test/integration/v2run/testdata/aws-catalog.json')];
const awsCatalogPath = catalogCandidates.find(existsSync);
if (!awsCatalogPath) throw new Error('AWS acceptance catalog file is unavailable');
const awsCatalog = JSON.parse(readFileSync(awsCatalogPath, 'utf8'));
const sorted = (items) => [...items].sort();
const equal = (actual, expected) => JSON.stringify(sorted(actual)) === JSON.stringify(sorted(expected));
const expectArray = (value, label) => {
  if (!Array.isArray(value)) throw new Error(`${label} response shape is invalid`);
  return value;
};

export async function verifyReopen({ base, scenario, applicationKey, connectionKey, fetchImpl = fetch }) {
  if (!['A01', 'E01', 'E02', 'A02', 'A03'].includes(scenario) || !applicationKey || !connectionKey) throw new Error('scenario scope is incomplete');
  const origin = new URL(base).origin;
  const signIn = await fetchImpl(`${origin}/api/v1/auth/sign-in`, {
    method: 'POST', headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ username: 'platform-engineer', password: FIXTURE_PASSWORD }),
  });
  if (!signIn.ok) throw new Error('sign-in after reopen failed');
  const cookie = (signIn.headers.getSetCookie?.() ?? []).map((value) => value.split(';')[0]).join('; ');
  if (!cookie) throw new Error('sign-in after reopen did not establish a session');
  const get = async (path) => {
    const response = await fetchImpl(`${origin}/api/v1${path}`, { headers: { cookie } });
    if (!response.ok) throw new Error(`reopen read failed (${response.status})`);
    try { return await response.json(); } catch { throw new Error('reopen response was not valid JSON'); }
  };
  const typesDoc = await get('/resource-types');
  const types = expectArray(typesDoc.resourceTypes, 'resource types').map((item) => item?.key);
  const aws = scenario === 'A02' || scenario === 'A03';
  const expectedTypes = aws ? awsCatalog.types.map((item) => item.key) : TYPES.map((item) => item.key);
  if (types.some((key) => typeof key !== 'string') || !equal(types, expectedTypes)) throw new Error('resource type catalog changed after reopen');
  const definitionsDoc = await get('/resource-definitions');
  const definitions = expectArray(definitionsDoc.resourceDefinitions, 'resource definitions').map((item) => item?.key);
  const expectedDefinitions = aws
    ? [...awsCatalog.definitions.common, ...awsCatalog.definitions[scenario]].map((item) => item.id)
    : K8S_ACCEPTANCE_DEFINITIONS.map((item) => item.id);
  if (definitions.some((key) => typeof key !== 'string') || !equal(definitions, expectedDefinitions)) throw new Error('scenario resource definition catalog changed after reopen');
  const capabilities = expectArray((await get('/resource-definition-capabilities')).capabilities, 'definition capabilities');
  const expectedModules = aws
    ? [...awsCatalog.definitions.common, ...awsCatalog.definitions[scenario]].map((item) => item.module)
    : K8S_ACCEPTANCE_DEFINITIONS.map((item) => item.module);
  for (const module of new Set(expectedModules)) {
    if (!capabilities.some((capability) => capability?.driverType === 'terraform' && capability.module === module && typeof capability.source?.contentSha256 === 'string' && /^[a-f0-9]{64}$/i.test(capability.source.contentSha256))) {
      throw new Error(`verified module pin ${module} is missing after reopen`);
    }
  }
  const connections = expectArray((await get('/connections')).connections, 'connections');
  const expectedKind = aws ? 'AWS' : 'KUBERNETES';
  if (!connections.some((connection) => connection?.key === connectionKey && connection.kind === expectedKind && connection.status === 'READY')) throw new Error('scenario Connection is not READY after reopen');

  const scope = `/applications/${encodeURIComponent(applicationKey)}/environments/staging`;
  const resources = expectArray((await get(`${scope}/resources`)).resources, 'resources');
  const deployments = expectArray((await get(`${scope}/deployments`)).deployments, 'deployments');
  const expectedResources = scenario === 'A01'
    ? [['k8s-namespace', 'k8s-namespace'], ['postgres', 'postgres-k8s'], ['redis', 'redis-k8s']]
    : scenario === 'E02'
      ? [['k8s-namespace', 'k8s-namespace'], ['redis', 'redis-k8s']]
      : scenario === 'A02'
        ? [['vpc', 'vpc'], ['k8s-cluster', 'eks'], ['k8s-namespace', 'k8s-namespace'], ['postgres', 'aurora'], ['redis', 'elasticache']]
        : scenario === 'A03'
          ? [['vpc', 'vpc'], ['k8s-cluster', 'eks'], ['k8s-namespace', 'k8s-namespace'], ['postgres', 'aurora'], ['redis', 'redis-k8s']]
          : [];
  if (scenario !== 'E01' && resources.length !== expectedResources.length) throw new Error(`${scenario} resource history count does not match its owned plan`);
  for (const [type, module] of expectedResources) {
    const matches = resources.filter((resource) => resource?.type === type && resource.module === module);
    if (matches.length !== 1 || !matches[0].id || !matches[0].resourceId) throw new Error(`${scenario} ${type}/${module} ownership history is missing, duplicated, or incomplete`);
    if (matches[0].status !== 'DESTROYED') throw new Error(`${scenario} ${type}/${module} is not DESTROYED after cleanup`);
  }
  if (scenario === 'E01') {
    if (resources.length !== 0 || deployments.length !== 0) throw new Error('E01 unexpectedly persisted resource instances or deployments');
  } else if (scenario === 'E02') {
    const redis = resources.find((resource) => resource?.type === 'redis' && resource.module === 'redis-k8s');
    const detail = await get(`${scope}/resources/${encodeURIComponent(redis.id)}`);
    if (detail?.resource?.status !== 'DESTROYED' || detail.resource.module?.id !== 'redis-k8s' || !detail.resource.module.revision) throw new Error('E02 destroyed resource or module provenance is missing after cleanup');
    let retainedFailure = false;
    for (const deployment of deployments) {
      if (!deployment?.id) throw new Error('E02 deployment history shape is invalid');
      const history = await get(`${scope}/deployments/${encodeURIComponent(deployment.id)}`);
      retainedFailure ||= expectArray(history.resourceProgress, 'E02 deployment progress').some((attempt) => attempt?.status === 'FAILED' && attempt.moduleId === 'redis-k8s' && attempt.resourceInstanceId === redis.id);
    }
    if (!retainedFailure) throw new Error('E02 failed Terraform attempt history is missing after cleanup');
  }
  return { scenario, types: types.length, definitions: definitions.length, resources: resources.length, deployments: deployments.length };
}

if (process.argv[1] && import.meta.url.startsWith('file:') && fileURLToPath(import.meta.url) === process.argv[1]) {
  const [base, scenario, applicationKey, connectionKey] = process.argv.slice(2);
  try {
    const result = await verifyReopen({ base, scenario, applicationKey, connectionKey });
    console.log(`sql-reopen ok: ${result.scenario}, ${result.types} types, ${result.definitions} definitions, ${result.resources} resources and ${result.deployments} deployments served after restart`);
  } catch (error) {
    console.error(String(error.message ?? error));
    process.exit(1);
  }
}
