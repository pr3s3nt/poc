// Read-only SQL reopen check of the real runner: after the product restarted on
// the same PostgreSQL database it must serve the catalog the Console wrote and,
// for V2-A01, the resource statuses the run left behind. Nothing is written.
//   node terraform-v2-reopen.mjs BASE_URL SCENARIO APPLICATION_KEY
const [base, scenario, application] = process.argv.slice(2);
const FIXTURE_PASSWORD = 'test-password';
const signIn = async (username) => {
  const res = await fetch(`${base}/api/v1/auth/sign-in`, { method: 'POST', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ username, password: FIXTURE_PASSWORD }) });
  if (!res.ok) throw new Error('sign-in after reopen failed');
  return (res.headers.getSetCookie?.() ?? []).map((v) => v.split(';')[0]).join('; ');
};
const failures = [];
try {
  const cookie = await signIn('platform-engineer');
  const get = async (path) => (await fetch(`${base}/api/v1${path}`, { headers: { cookie } })).json();
  const types = (await get('/resource-types')).resourceTypes.map((t) => t.key).sort();
  const definitions = (await get('/resource-definitions')).resourceDefinitions.map((d) => d.key).sort();
  for (const key of ['k8s-cluster', 'k8s-namespace', 'postgres', 'redis', 'workload']) if (!types.includes(key)) failures.push(`type ${key} missing after reopen`);
  for (const key of ['namespace-k8s-module', 'pg-in-cluster', 'redis-in-cluster']) if (!definitions.includes(key)) failures.push(`definition ${key} missing after reopen`);
  const capabilities = (await get('/resource-definition-capabilities')).capabilities;
  if (!capabilities.some((c) => c.driverType === 'terraform' && c.module === 'redis-k8s' && c.source?.contentSha256)) failures.push('verified pins missing after reopen');
  const connections = (await get('/connections')).connections.map((c) => `${c.key}:${c.status}`);
  if (!connections.some((c) => c.endsWith(':READY'))) failures.push('no READY Connection after reopen');
  if (scenario === 'A01') {
    const resources = (await get(`/applications/${application}/environments/staging/resources`)).resources ?? [];
    const status = (type) => resources.find((r) => r.type === type)?.status;
    if (status('redis') !== 'DESTROYED' || status('postgres') !== 'DESTROYED') failures.push('deleted resources are not DESTROYED after reopen');
    if (status('k8s-namespace') === 'DESTROYED') failures.push('the retained namespace record was destroyed');
  }
  if (scenario === 'E02') {
    const resources = (await get(`/applications/${application}/environments/staging/resources`)).resources ?? [];
    if (!resources.some((r) => r.type === 'redis' && r.status !== 'DESTROYED')) failures.push('the failed Redis record (state/provenance) is missing after reopen');
  }
  if (failures.length) throw new Error(failures.join('; '));
  console.log(`sql-reopen ok: ${types.length} types, ${definitions.length} definitions, pins and connections served again after a product restart on the same PostgreSQL database`);
} catch (error) {
  console.error(String(error.message ?? error));
  process.exit(1);
}
