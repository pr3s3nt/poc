// Run only from the owned-kind harness. Catalog setup and application changes
// use Console controls; the existing workload form scenario supplies the flow.
import { chromium, expect } from '@playwright/test';
import { readFileSync, writeFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { registerHooks } from 'node:module';
import { readConsoleConfig, signInContext, signOutContext } from '../../lib/console.mjs';
// Reused legacy helpers resolve dependencies from frontend/node_modules. Route
// their Playwright import to this runner's locked instance, avoiding two test
// runtimes in one Node process when both package directories are installed.
const playwrightURL = import.meta.resolve('@playwright/test');
registerHooks({ resolve(specifier, context, nextResolve) {
  return nextResolve(specifier === '@playwright/test' ? playwrightURL : specifier, context);
} });
const [{ createHuman }, { makeKit }, {
  FULL_APP_TYPES, onboardInternalAccount, registerTypesUi, registerExistingCluster,
  registerK8sModuleDefinition, registerWorkloadTemplate,
}] = await Promise.all([
  import('../../../frontend/test/e2e/human.mjs'),
  import('../../../frontend/test/e2e/full-app-kit.mjs'),
  import('../../../frontend/test/e2e/full-app-flows.mjs'),
]);

const required = name => {
  if (!process.env[name]) throw Error(`${name} is required`);
  return process.env[name];
};
const config = readConsoleConfig();
const evidenceDir = required('WM_EVIDENCE');
const privateDir = required('WM_PRIVATE');
const runId = required('WM_RUN_ID');
const browser = await chromium.launch({ headless: config.headless });
const { page, context } = await signInContext(browser, config, { role: 'PLATFORM_ENGINEER' });
page.setDefaultTimeout(60_000);
const browserErrors = [];
page.on('pageerror', error => browserErrors.push(error.message));
const input = {
  runId, privateDir,
  account: { name: `kind-${runId}`, kubeconfigFile: required('WM_INTERNAL_KUBECONFIG') },
  kube: { kubectl: 'kubectl', kubeconfig: required('WM_HOST_KUBECONFIG'), context: required('WM_CONTEXT') },
  v2ctl: required('WM_V2CTL'),
};
const assertions = [];
const rec = { record(id, phase, source, pass, detail) {
  assertions.push({ id, phase, source, pass, detail });
  writeFileSync(`${evidenceDir}/setup-assertions.json`, JSON.stringify(assertions, null, 2));
  if (!pass) throw Error(`Setup assertion ${id} failed`);
} };
let storeKey;
try {
  const kit = makeKit({ page, context, h: createHuman(page, { fast: true }), input,
    baseURL: config.baseURL, evidenceDir, rec, caption() {}, phase() {}, forwarders: [] });
  await onboardInternalAccount(kit);
  await registerTypesUi(kit, FULL_APP_TYPES);
  storeKey = await kit.platform.registerSecretStore({ name: 'Run Workload Vault',
    backendAddress: required('WM_VAULT_ADDR'), workloadAddress: required('WM_VAULT_ADDR'),
    mount: 'kv', authMount: 'kubernetes',
  }, readFileSync(required('WM_WORKLOAD_VAULT_TOKEN_FILE'), 'utf8').trim());
  await registerExistingCluster(kit, { id: 'run-cluster', accountName: input.account.name, appKey: '' });
  for (const spec of [
    { id: 'run-namespace', type: 'k8s-namespace', module: 'k8s-namespace', presetName: 'name', clusterClass: 'internal' },
    { id: 'run-postgres', type: 'postgres', module: 'postgres-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
    { id: 'run-redis', type: 'redis', module: 'redis-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
  ]) await registerK8sModuleDefinition(kit, spec);
  await registerWorkloadTemplate(kit, { id: 'run-renderer', bundle: 'score-k8s-internal-v1' });
  expect(browserErrors).toEqual([]);
  await page.screenshot({ path: `${evidenceDir}/catalog.png`, fullPage: true });
} finally {
  await signOutContext(context, config);
  await browser.close();
}
const result = spawnSync(process.execPath, ['e2e/scenarios/workload-env-form/run.mjs'], {
  cwd: new URL('../../../', import.meta.url), stdio: 'inherit',
  env: { ...process.env, ORCH_PLATFORM_USERNAME: 'developer',
    ORCH_ENV_RUN: `merge-${runId}`, ORCH_EVIDENCE_DIR: `${evidenceDir}/form`,
    ORCH_ACCEPTANCE_IMAGE_TAG: required('WM_IMAGE_TAG'),
    ORCH_KUBE_CONTEXT: required('WM_CONTEXT'), ORCH_SECRET_STORE_KEY: storeKey,
    ORCH_DELETE_APPLICATION: '1', ORCH_SECRET_SCAN_FILE: `${privateDir}/app-secrets.txt`,
  },
});
if (result.error) throw result.error;
process.exitCode = result.status ?? 1;
