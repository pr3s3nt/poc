// Real retained kind smoke: UI onboarding/Preview/Deploy, no fake adapters.
// Explicit private inputs and a NEW evidence directory; never resets a stack.
// Retains its sample Application and runtime for the operator to use.
import { chromium, expect } from '@playwright/test';
import { mkdirSync, readFileSync, writeFileSync, appendFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { resolve } from 'node:path';
import { createHuman, createApplication } from './human.mjs';
import { makeKit } from './full-app-kit.mjs';
import {
  FULL_APP_TYPES, onboardInternalAccount, registerTypesUi, registerExistingCluster,
  registerK8sModuleDefinition, registerWorkloadTemplate, configureEnvironment,
} from './full-app-flows.mjs';

const baseURL = process.env.PREVIEW_URL;
const evidenceDir = process.env.PREVIEW_EVIDENCE;
const kubeconfigFile = process.env.PREVIEW_KUBECONFIG;
const tokenFile = process.env.PREVIEW_WORKLOAD_VAULT_TOKEN;
const ctl = process.env.PREVIEW_V2CTL;
if (!/^http:\/\/(?:127\.0\.0\.1|100(?:\.\d{1,3}){3}):18094$/.test(baseURL ?? '') || !evidenceDir || !kubeconfigFile || !tokenFile || !ctl) {
  throw new Error('Explicit PREVIEW_URL (loopback/Tailscale port 18094), EVIDENCE, KUBECONFIG, WORKLOAD_VAULT_TOKEN and V2CTL required');
}
const repoDir = resolve(import.meta.dirname, '../../..');
if (resolve(evidenceDir) === repoDir || resolve(evidenceDir).startsWith(`${repoDir}/`)) throw new Error('Evidence must be outside the repository');
mkdirSync(evidenceDir, { mode: 0o700 });
mkdirSync(`${evidenceDir}/private`, { mode: 0o700 });
const browser = await chromium.launch({ channel: 'chrome', headless: true });
const context = await browser.newContext({ viewport: { width: 1512, height: 1100 } });
const page = await context.newPage();
page.setDefaultTimeout(45_000);
const resumed = process.env.PREVIEW_RESUME ? JSON.parse(readFileSync(process.env.PREVIEW_RESUME, 'utf8')) : null;
const runId = resumed?.runId ?? `preview-${Date.now().toString().slice(-9)}`;
const input = {
  runId, privateDir: `${evidenceDir}/private`, applicationName: `Kind Preview ${runId}`,
  account: { name: 'kind-k8s-4f', kubeconfigFile },
  kube: { kubectl: 'kubectl', context: 'kind-k8s-4f' },
};
const h = createHuman(page, { fast: true });
const rec = { record(id, phase, source, pass) {
  if (!pass) throw new Error(`Assertion ${id} failed`);
  appendFileSync(`${evidenceDir}/assertions.txt`, `PASS ${id} (${phase}/${source})\n`);
  console.log(`PASS ${id}`);
} };
let kit;
try {
  await page.goto(`${baseURL}/ui/sign-in`);
  await page.getByLabel('Tên đăng nhập').fill('platform-engineer');
  await page.getByLabel('Mật khẩu').fill('test-password');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
  if (!resumed) {
    for (const [path, key] of [['driver-accounts', 'accounts'], ['resource-types', 'resourceTypes'], ['resource-definitions', 'resourceDefinitions']]) {
      const response = await context.request.get(`${baseURL}/api/v1/${path}`);
      expect(response.ok()).toBe(true);
      expect((await response.json())[key]).toHaveLength(0);
    }
  }
  input.applicationKey = resumed?.applicationKey ?? await createApplication(h, input.applicationName, runId);
  if (resumed && resumed.applicationName !== input.applicationName) throw new Error('Resume Application identity mismatch');
  writeFileSync(`${evidenceDir}/application.json`, JSON.stringify({ ...input, privateDir: undefined, account: undefined, kube: undefined }, null, 2));
  kit = makeKit({ page, context, h, input, baseURL, evidenceDir, rec, caption() {}, phase() {}, forwarders: [] });
  expect((await kit.get(`${kit.appPath}/deployments`)).deployments).toHaveLength(0);
  if (!resumed) {
    await onboardInternalAccount(kit);
    await registerTypesUi(kit, FULL_APP_TYPES);
  } else {
    const account = (await kit.get('/driver-accounts')).accounts.find((a) => a.key === input.account.name);
    expect(account?.status).toBe('READY');
    expect(account?.config?.endpoint).toBe('https://k8s-4f-control-plane:6443');
    const app = await kit.get(`/applications/${input.applicationKey}`);
    expect(app.application.name).toBe(input.applicationName);
  }
  const defs = (await kit.get('/resource-definitions')).resourceDefinitions;
  if (!defs.some((d) => d.key === 'kind-cluster')) await registerExistingCluster(kit, { id: 'kind-cluster', accountName: input.account.name, appKey: '' });
  else {
    expect(resumed).toBeTruthy();
    await page.getByRole('link', { name: /Cấu hình tài nguyên$/ }).click();
    await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  }
  for (const spec of [
    { id: 'kind-namespace', type: 'k8s-namespace', module: 'k8s-namespace', presetName: 'name', clusterClass: 'internal' },
    { id: 'kind-postgres', type: 'postgres', module: 'postgres-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
    { id: 'kind-redis', type: 'redis', module: 'redis-k8s', presetNamespace: true, klass: 'in-cluster', clusterClass: 'internal' },
  ]) {
    const existing = defs.find((d) => d.key === spec.id);
    if (existing) {
      expect(resumed).toBeTruthy();
      expect(existing.resourceType).toBe(spec.type);
      expect(existing.driverInputs.values.provider_bindings.kubernetes.resource_ref).toBe('k8s-cluster.internal#environments.@app.@env');
    } else await registerK8sModuleDefinition(kit, spec);
  }
  if (!defs.some((d) => d.key === 'kind-workload')) await registerWorkloadTemplate(kit, { id: 'kind-workload', bundle: 'score-k8s-internal-v1' });
  const stores = (await kit.get('/secret-stores')).secretStores;
  const existingStore = stores.find((s) => s.key === 'kind-workload-vault');
  let storeKey;
  if (existingStore) {
    expect(resumed).toBeTruthy();
    expect(existingStore.status).toBe('READY');
    expect(existingStore.backendAddress).toBe('http://k8s-4f-workload-vault:8200');
    expect(existingStore.workloadAddress).toBe('http://k8s-4f-workload-vault:8200');
    storeKey = existingStore.key;
  } else storeKey = await kit.platform.registerSecretStore({ name: 'Kind Workload Vault', backendAddress: 'http://k8s-4f-workload-vault:8200', workloadAddress: 'http://k8s-4f-workload-vault:8200', mount: 'kv', authMount: 'kubernetes' }, readFileSync(tokenFile, 'utf8').trim());
  const imageTag = 'v46-20261018-512d0d3-amd64';
  execFileSync(ctl, ['v04-scores', '-out', `${evidenceDir}/scores`, '-application', input.applicationKey, '-run-id', runId,
    '-frontend', `acceptance-frontend:${imageTag}`, '-backend', `acceptance-backend:${imageTag}`, '-worker', `acceptance-worker:${imageTag}`], { stdio: 'pipe' });
  const scores = JSON.parse(readFileSync(`${evidenceDir}/scores/scores.json`, 'utf8'));
  const secret = randomBytes(24).toString('hex');
  const hash = createHash('sha256').update(secret).digest('hex');
  const config = scores.config.filter((c) => c.kind === 'variable').map((c) => c.name === 'ACCEPTANCE_SECRET_SHA256' ? { ...c, value: hash } : c);
  await page.goto(`${baseURL}/ui/applications/${input.applicationKey}`);
  await configureEnvironment(kit, { config, secretName: 'ACCEPTANCE_SECRET', secretValue: secret, storeKey });
  for (const name of ['backend', 'worker', 'frontend']) await kit.addWorkloadFromScore(`${evidenceDir}/scores/${name}.yaml`, name);
  const responsePromise = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/preview'), { timeout: 120_000 });
  await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
  const response = await responsePromise;
  if (!response.ok()) throw new Error(`Preview HTTP ${response.status()}: ${(await response.json()).code}`);
  const preview = await response.json();
  writeFileSync(`${evidenceDir}/preview.json`, JSON.stringify(preview));
  await page.screenshot({ path: `${evidenceDir}/preview.png`, fullPage: true });
  expect(preview.review.nodes.filter((n) => n.kind === 'workload')).toHaveLength(3);
  const deployResponse = page.waitForResponse((r) => r.request().method() === 'POST' && r.url().endsWith('/deploy'), { timeout: 900_000 });
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  const deployed = await deployResponse;
  const result = await deployed.json();
  writeFileSync(`${evidenceDir}/deploy.json`, JSON.stringify(result));
  if (!deployed.ok() || result.status !== 'SUCCEEDED') throw new Error(`Deploy HTTP ${deployed.status()} status ${result.status} code ${result.code ?? ''}`);
  await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công');
  await page.screenshot({ path: `${evidenceDir}/ready.png`, fullPage: true });
  kit.assertNoViolations();
  rec.record('kind.real-deploy', 'READY', 'ui', true);
  console.log(JSON.stringify({ app: input.applicationKey, runId, status: result.status, url: `${baseURL}/ui/applications/${input.applicationKey}` }));
} catch (error) {
  // Never echo CLI stderr, request bodies, tokens or kubeconfig content.
  await page.screenshot({ path: `${evidenceDir}/failure.png`, fullPage: true }).catch(() => {});
  writeFileSync(`${evidenceDir}/private/failure.txt`, String(error.stack), { mode: 0o600 });
  // Playwright assertion details can include the supplied expected value.
  // Keep those private when onboarding handles a token or kubeconfig.
  console.error(`Live verification failed; inspect ${evidenceDir}/private/failure.txt`);
  process.exitCode = 1;
} finally {
  await browser.close();
}
