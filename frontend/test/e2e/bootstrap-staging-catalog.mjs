// Operator tool: UI writes only; no kubeconfig, deployment, account creation or deletion.
import { chromium, expect } from '@playwright/test';
import { readFileSync, statSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
import { isDeepStrictEqual } from 'node:util';
import { createHuman } from './human.mjs';
import { registerExistingCluster, registerK8sModuleDefinition, registerWorkloadTemplate } from './full-app-flows.mjs';

class OperatorError extends Error {}

export const CLUSTER = 'k8s-cluster.internal#environments.@app.@env';
export const NS = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
export const BUNDLE = 'score-k8s-internal-v1';
export const specs = [
  { id: 'staging-k8s-4f-cluster', type: 'k8s-cluster', driver: 'existing-cluster', klass: 'internal' },
  { id: 'staging-k8s-4f-namespace', name: 'Namespace Kubernetes staging', type: 'k8s-namespace', module: 'k8s-namespace', presetName: 'name' },
  { id: 'staging-k8s-4f-postgres', name: 'PostgreSQL Kubernetes staging', type: 'postgres', module: 'postgres-k8s', presetNamespace: true, klass: 'in-cluster' },
  { id: 'staging-k8s-4f-redis', name: 'Redis Kubernetes staging', type: 'redis', module: 'redis-k8s', presetNamespace: true, klass: 'in-cluster' },
  { id: 'staging-k8s-4f-renderer', type: 'workload', driver: 'score-k8s' },
];

const sorted = (value) => Array.isArray(value) ? value.map(sorted) : value && typeof value === 'object'
  ? Object.fromEntries(Object.entries(value).filter(([,v]) => v !== undefined && v !== '').sort(([a],[b]) => a.localeCompare(b)).map(([k,v]) => [k,sorted(v)])) : value;
export function expectedDefinition(spec, capability, accountKey) {
  const driverType = spec.driver ?? 'terraform';
  let values = { variables: {} };
  if (driverType === 'terraform') {
    const pin = capability.source;
    values = {
      source: { source_id: pin.sourceId, module_id: pin.moduleId, revision: pin.revision, subdirectory: pin.subdirectory, content_sha256: pin.contentSha256 },
      variables: spec.presetName ? { name: '${context.env.namespace}' } : { namespace: NS },
      input_mapping: spec.type === 'postgres' ? { database: { param: 'database' }, username: { param: 'username' } } : {},
      output_mapping: Object.fromEntries(capability.outputs.map(o => [o.name, o.name])),
      provider_bindings: { kubernetes: { target: 'resource', resource_ref: CLUSTER } },
    };
  } else if (driverType === 'score-k8s') {
    values.variables = { render_bundle: BUNDLE, namespace: NS, cluster: `\${resources['${CLUSTER}'].outputs.cluster}` };
  }
  return { resourceType: spec.type, driverType, driverAccount: driverType === 'existing-cluster' ? accountKey : undefined,
    driverInputs: { values }, criteria: [{ env_type: 'staging', ...(spec.klass ? { class: spec.klass } : {}) }] };
}

export function assertMatches(actual, expected, id) {
  const projected = { resourceType: actual.resourceType, driverType: actual.driverType, driverAccount: actual.driverAccount,
    driverInputs: actual.driverInputs, criteria: actual.criteria };
  // Empty companion maps carry no behavior; nonempty rules are a conflict.
  if (projected.driverInputs?.provision && Object.keys(projected.driverInputs.provision).length === 0) {
    projected.driverInputs = { ...projected.driverInputs }; delete projected.driverInputs.provision;
  }
  if (actual.provision && Object.keys(actual.provision).length) throw new OperatorError(`Conflicting companion rules: ${id}`);
  if (!isDeepStrictEqual(sorted(projected), sorted(expected))) throw new OperatorError(`Conflicting configuration: ${id}; review in Console (nothing overwritten)`);
}

export async function bootstrapCatalog(page, baseURL) {
  const get = async path => {
    const response = await page.request.get(`${baseURL}/api/v1${path}`);
    if (!response.ok()) throw new OperatorError(`GET ${path} returned HTTP ${response.status()}`);
    return response.json();
  };
  const accounts = (await get('/driver-accounts')).accounts;
  const account = accounts?.find(a => a.key === 'k8s-4f');
  if (!account || account.type !== 'INTERNAL' || account.status !== 'READY') throw new OperatorError('Register Internal Driver Account k8s-4f and verify it to READY first');
  const types = (await get('/resource-types')).resourceTypes;
  for (const spec of specs) {
    const type = types?.find(t => t.key === spec.type);
    if (!type) throw new OperatorError(`Missing seeded Type ${spec.type}`);
    if (['postgres','redis'].includes(spec.type) && !type.outputs?.some(o => o.name === 'password' && o.secret)) throw new OperatorError(`Type ${spec.type} must mark password as secret`);
  }
  const bundles = (await get('/render-bundles')).renderBundles;
  if (!bundles?.some(b => b.id === BUNDLE && b.status === 'AVAILABLE')) throw new OperatorError(`Installed bundle ${BUNDLE} unavailable; configure score-k8s in backend`);
  const capabilities = (await get('/resource-definition-capabilities')).capabilities;
  const pins = new Map();
  for (const spec of specs.filter(s => s.module)) {
    const candidates = capabilities.filter(c => c.resourceType === spec.type && c.driverType === 'terraform' && c.module === spec.module && c.source);
    if (candidates.length !== 1) throw new OperatorError(`Expected exactly one verified module ${spec.module}; resolve catalog ambiguity first`);
    pins.set(spec.id, candidates[0]);
  }
  // Complete conflict preflight BEFORE any write; a partial previous run can resume.
  const definitions = (await get('/resource-definitions')).resourceDefinitions;
  for (const spec of specs) {
    const existing = definitions.find(d => d.key === spec.id);
    if (existing) assertMatches(existing, expectedDefinition(spec, pins.get(spec.id), account.key), spec.id);
    const rival = definitions.find(d => d.key !== spec.id && d.resourceType === spec.type && d.criteria?.some(c =>
      (!c.env_type || c.env_type === 'staging') && (!c.class || c.class === (spec.klass ?? 'default'))));
    if (rival) throw new OperatorError(`Overlapping catalog entry ${rival.key}; review matching before running bootstrap`);
  }
  const h = createHuman(page, { fast: true });
  const wrapped = new Set(['Loại tài nguyên','Module đã kiểm chứng','Tài khoản hạ tầng','Bundle dựng ứng dụng','Cách cung cấp cluster']);
  const form = (name, scope = page) => wrapped.has(name)
    ? scope.locator(`select[aria-label="${name}"]`).or(scope.locator('label').filter({ hasText: new RegExp(`^\\s*${name}`) }).locator(':scope > select')).first()
    : scope.getByLabel(name, { exact: true });
  const choose = async (select, prefix) => {
    await expect.poll(async () => (await select.locator('option').allTextContents()).filter(s => s.trim().startsWith(prefix)).length, { timeout: 30_000 }).toBe(1);
    const label = (await select.locator('option').allTextContents()).find(s => s.trim().startsWith(prefix));
    await select.selectOption({ label });
  };
  const kit = { page, h, form, choose, get, nav: name => page.getByRole('link', { name, exact: true }),
    rec: { record(_id,_phase,_source,pass) { if (!pass) throw new OperatorError('Catalog registration assertion failed'); } }, mark() {} };
  for (const spec of specs) {
    if (definitions.some(d => d.key === spec.id)) { console.log(`Retained ${spec.id}`); continue; }
    await page.goto(`${baseURL}/ui/platform/${spec.type === 'workload' ? 'rendering-templates' : 'resource-definitions'}`);
    if (spec.type === 'k8s-cluster') await registerExistingCluster(kit, { id: spec.id, accountName: account.name, envType: 'staging' });
    else if (spec.type === 'workload') await registerWorkloadTemplate(kit, { id: spec.id, bundle: BUNDLE, envType: 'staging' });
    else {
      await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
      await registerK8sModuleDefinition(kit, { ...spec, clusterClass: 'internal', envType: 'staging' });
    }
    const stored = (await get('/resource-definitions')).resourceDefinitions.find(d => d.key === spec.id);
    if (!stored) throw new OperatorError(`Registration not confirmed: ${spec.id}; inspect Console before retry`);
    assertMatches(stored, expectedDefinition(spec, pins.get(spec.id), account.key), spec.id);
    console.log(`Created ${spec.id}`);
  }
}

async function main() {
  const baseURL = (process.env.ORCH_PLATFORM_URL ?? '').replace(/\/$/, '').replace(/\/ui\/?$/, '');
  const url = new URL(baseURL);
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw new OperatorError('ORCH_PLATFORM_URL must be the platform origin');
  const username = process.env.ORCH_PLATFORM_USERNAME;
  const passwordFile = process.env.ORCH_PLATFORM_PASSWORD_FILE;
  if (!username || !passwordFile) throw new OperatorError('Set ORCH_PLATFORM_USERNAME and ORCH_PLATFORM_PASSWORD_FILE');
  if (!statSync(passwordFile).isFile() || (statSync(passwordFile).mode & 0o077)) throw new OperatorError('Password file must be private (0400 or 0600)');
  const password = readFileSync(passwordFile, 'utf8').replace(/\r?\n$/, '');
  if (!password) throw new OperatorError('Password file is empty');
  const browser = await chromium.launch({ headless: process.env.ORCH_HEADLESS === '1' });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await context.addInitScript(() => { globalThis.localStorage.setItem('orchestrator.locale', 'vi'); });
    const page = await context.newPage();
    page.setDefaultTimeout(30_000);
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.getByLabel('Tên đăng nhập', { exact: true }).fill(username);
    await page.getByLabel('Mật khẩu', { exact: true }).fill(password);
    await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
    await expect(page.getByRole('link', { name: 'Tài khoản hạ tầng', exact: true })).toBeVisible();
    await bootstrapCatalog(page, baseURL);
    console.log('Staging catalog ready. No Application or workload was deployed.');
  } finally { await browser.close(); }
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    // Never print Playwright call logs, response bodies, password or provider errors.
    if (error instanceof OperatorError) console.error(error.message);
    console.error('Catalog bootstrap failed. Review Console/account/modules/bundle and rerun; existing configuration is preserved.');
    process.exitCode = 1;
  });
}
