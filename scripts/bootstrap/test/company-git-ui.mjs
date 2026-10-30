// Explicit browser check against an existing local Console. Reads real public
// metadata; ALL catalog writes/inspects are intercepted as fixtures, never sent
// to the backend. No Kubernetes, cloud, DB reset or provider work.
import { expect } from '@playwright/test';
import { loadTarget } from '../lib/target-config.mjs';
import { readConsoleConfig, openConsole, closeConsole } from '../lib/console.mjs';
import { ensureResourceDefinitions } from '../lib/infrastructure-ui.mjs';
import { desiredInfrastructureDefinition } from '../lib/infrastructure-config.mjs';
import { desiredDefinition, assertSameDefinition, parseBundleConfig } from '../lib/render-bundle-config.mjs';
import { updateRenderDefinitionSource } from '../lib/definition-update-ui.mjs';
const target = loadTarget({ target: 'dev' });
const config = readConsoleConfig(target.env, { defaultPassword: target.defaultPassword });
if (!['localhost', '127.0.0.1'].includes(new URL(config.baseURL).hostname)) throw Error('Local read-only Console fixture check only');
const session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
const { page, context } = session;
page.setDefaultTimeout(12_000);
const read = async path => {
  const response = await context.request.get(`${config.baseURL}/api/v1${path}`);
  if (!response.ok()) throw Error(`GET ${path}: HTTP ${response.status()}`);
  return response.json();
};
const act = (locator, method, ...args) => locator[method](...args);
let handler;
try {
  const capabilities = await read('/resource-definition-capabilities');
  const originalDefinitions = await read('/resource-definitions');
  const bundleList = await read('/render-bundles');
  const seed = parseBundleConfig(target.raw);
  const oldCap = capabilities.capabilities.find(c => c.module === 'postgres-k8s' && c.source.sourceId === 'platform');
  if (!oldCap) throw Error('Installed postgres-k8s metadata required');
  const newCap = structuredClone(oldCap);
  newCap.source = { ...newCap.source, sourceId: 'company-fixture', repositoryUrl: 'https://git.company.example/catalog.git', revision: 'b'.repeat(40), subdirectory: 'backend/terraform-modules/postgres-k8s', contentSha256: 'c'.repeat(64) };
  const item = structuredClone(seed.resourceDefinitions.find(d => d.module === 'postgres-k8s'));
  item.key = 'bootstrap-ui-fixture-postgres'; item.name = 'Bootstrap fixture PostgreSQL';
  delete item.module;
  item.source = { url: newCap.source.repositoryUrl, path: newCap.source.subdirectory, ref: newCap.source.revision };
  const desired = desiredInfrastructureDefinition(item, newCap);
  const existing = { ...desiredInfrastructureDefinition(item, oldCap), version: 3 };
  const oldBundle = bundleList.renderBundles.find(b => b.source?.kind === 'git');
  if (!oldBundle) throw Error('One registered Git bundle metadata required');
  const newBundle = structuredClone(oldBundle);
  newBundle.id = 'aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee'; newBundle.source.url = item.source.url;
  newBundle.source.revision = item.source.ref; newBundle.version = 'b'.repeat(12);
  const bundleItem = { ...seed.bundles[0], key: 'bootstrap-ui-fixture-renderer', name: 'Bootstrap fixture renderer' };
  const renderDesired = desiredDefinition(bundleItem, newBundle.id);
  const renderExisting = { ...desiredDefinition(bundleItem, oldBundle.id), version: 2 };
  let definitions = [...originalDefinitions.resourceDefinitions, existing, renderExisting];
  let puts = 0, rejectNext = false;
  const get = async path => {
    if (path === '/resource-definitions') return { resourceDefinitions: structuredClone(definitions) };
    return read(path);
  };
  handler = async route => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace('/api/v1', '');
    const json = (body, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    if (request.method() === 'GET') {
      if (path === '/resource-definitions') return json({ resourceDefinitions: definitions });
      if (path === '/resource-definition-capabilities') return json({ capabilities: [...capabilities.capabilities, newCap] });
      if (path === '/render-bundles') return json({ renderBundles: [...bundleList.renderBundles, newBundle] });
      if (path.startsWith('/render-bundles/')) return json({ renderBundle: path.endsWith(newBundle.id) ? newBundle : oldBundle });
      return route.fallback(); // read-only public product metadata
    }
    if (request.method() === 'POST' && path === '/terraform-modules/inspect') return json({ capability: newCap });
    if (request.method() === 'PUT' && path.startsWith('/resource-definitions/bootstrap-ui-fixture-')) {
      puts++;
      if (rejectNext) { rejectNext = false; return json({ code: 'STALE_VERSION', message: 'fixture conflict' }, 409); }
      const body = request.postDataJSON();
      const previous = definitions.find(d => d.key === body.key);
      expect(body.version).toBe(previous.version);
      assertSameDefinition(body, body.key === desired.key ? desired : renderDesired);
      const saved = { ...body, version: body.version + 1 };
      definitions = definitions.map(d => d.key === body.key ? saved : d);
      return json(saved);
    }
    return json({ code: 'FIXTURE_BLOCKS_ALL_OTHER_WRITES' }, 501);
  };
  await page.route('**/api/v1/**', handler);
  const input = { page, baseURL: config.baseURL, definitions: [item], act, get, updateExisting: true, responseFor: (path, method) => page.waitForResponse(r => r.request().method() === method && new URL(r.url()).pathname === `/api/v1${path}`) };
  await ensureResourceDefinitions(input);
  expect(puts).toBe(1);
  expect(definitions.find(d => d.key === item.key).version).toBe(4);
  await ensureResourceDefinitions(input);
  expect(puts).toBe(1);
  console.log('PASS actual product UI: installed→company Git module update, CAS payload, matching rerun skips PUT');
  await page.goto(`${config.baseURL}/ui/platform/rendering-templates`);
  await updateRenderDefinitionSource({ page, baseURL: config.baseURL, act, existing: renderExisting, desired: renderDesired, bundleId: newBundle.id });
  expect(puts).toBe(2);
  expect(definitions.find(d => d.key === renderExisting.key).version).toBe(3);
  console.log('PASS actual product UI: renderer pin update preserves criteria/placement');
  // The server rejects a version race once; the script must stop without retry.
  definitions = definitions.map(d => d.key === existing.key ? structuredClone(existing) : d);
  rejectNext = true;
  await expect(ensureResourceDefinitions(input)).rejects.toThrow(/409/);
  expect(puts).toBe(3);
  expect(definitions.find(d => d.key === existing.key).version).toBe(3);
  console.log('PASS CAS conflict stops with one PUT, no retry or partial fixture commit');
} catch(error) {
  console.error('Fixture UI:', (await page.locator('body').innerText()).slice(-4000));
  await page.screenshot({path:'/tmp/bootstrap-company-git-ui-failure.png',fullPage:true});
  throw error;
} finally {
  if (handler) await page.unroute('**/api/v1/**', handler);
  await closeConsole(session, config);
}
