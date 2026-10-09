// Registers one kubeconfig-backed Connection in a running Web Console, through
// the UI, headless. Registration only: no Definition, Application or deploy.
//
//   node test/e2e/k8s4f-register.mjs --base-url http://127.0.0.1:3001 \
//     --kubeconfig /private/path/kubeconfig [--name k8s-4f]
//
// Env fallbacks: ORCH_E2E_URL, ORCH_E2E_KUBECONFIG_FILE, ORCH_E2E_CONNECTION_NAME.
// Signs in as the seeded local platform-engineer. If the key already exists the
// record is only compared with the intended target (context and endpoint from the
// kubeconfig) and reused; it is never overwritten or deleted. Mismatch exits 1.
// Prints safe metadata only; the kubeconfig content and credentials are never
// printed, screenshotted or kept beyond this process.
import { chromium, expect } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { basename } from 'node:path';

const args = process.argv.slice(2);
const flag = (name, fallback) => {
  const i = args.indexOf(`--${name}`);
  return i >= 0 ? args[i + 1] : fallback;
};
const baseURL = (flag('base-url', process.env.ORCH_E2E_URL) ?? '').replace(/\/+$/, '');
const kubeconfigFile = flag('kubeconfig', process.env.ORCH_E2E_KUBECONFIG_FILE);
const name = flag('name', process.env.ORCH_E2E_CONNECTION_NAME ?? 'k8s-4f');
if (!baseURL || !kubeconfigFile) {
  console.error('usage: k8s4f-register.mjs --base-url URL --kubeconfig FILE [--name NAME]');
  process.exit(2);
}
const key = name.toLowerCase().replace(/[^a-z0-9]+/g, '-');
if (!/^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(key) || key === 'internal-cluster' || key !== name) {
  console.error('connection name must already be its own key (lowercase letters, digits, hyphens)');
  process.exit(2);
}

const kubeconfig = readFileSync(kubeconfigFile, 'utf8');
const secretValues = [...kubeconfig.matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)]
  .map((m) => m[1].replace(/^["']|["']$/g, '')).filter((v) => v.length >= 12);
if (!secretValues.length) {
  console.error('kubeconfig carries no embedded credential');
  process.exit(2);
}
const guard = (text, where) => {
  if (secretValues.some((v) => text.includes(v))) throw new Error(`credential material in ${where}`);
  return text;
};
const safe = (c) => ({ key: c.key, name: c.name, kind: c.kind, authenticationType: c.authenticationType, status: c.status, kubeContext: c.config?.kubeContext, endpoint: c.config?.endpoint ?? c.verification?.endpoint });

const browser = await chromium.launch();
let code = 0;
try {
  const page = await (await browser.newContext()).newPage();
  page.setDefaultTimeout(30_000);
  await page.goto(`${baseURL}/ui/sign-in`);
  await page.getByLabel('Username').fill('platform-engineer');
  await page.getByLabel('Password').fill('test-password');
  await page.getByRole('button', { name: 'Sign in' }).click();
  await expect(page.getByRole('heading', { name: 'Your applications' })).toBeVisible();
  await page.getByRole('link', { name: /Connections/ }).click();
  await expect(page.getByRole('heading', { name: 'Connections', level: 1 })).toBeVisible();

  // Intended target = the single context of the kubeconfig, as the server reads it.
  const intendedResponse = await page.request.post(`${baseURL}/api/v1/connections/kubernetes/inspect`, { data: { kubeconfig } });
  guard(await intendedResponse.text(), 'inspect response');
  if (intendedResponse.status() !== 200) throw new Error(`inspect refused: ${intendedResponse.status()}`);
  const { contexts } = await intendedResponse.json();
  if (contexts.length !== 1) throw new Error(`kubeconfig must hold exactly one context, found ${contexts.length}`);
  const intended = { kubeContext: contexts[0].name, endpoint: contexts[0].endpoint };

  const list = async () => {
    const response = await page.request.get(`${baseURL}/api/v1/connections`);
    const body = guard(await response.text(), 'connections response');
    if (response.status() !== 200) throw new Error(`connections list failed: ${response.status()}`);
    return JSON.parse(body).connections;
  };
  const verify = (c) => {
    const problems = [];
    if (c.key !== key || c.name !== name) problems.push('key/name');
    if (c.kind !== 'KUBERNETES' || c.authenticationType !== 'KUBECONFIG') problems.push('kind/authenticationType');
    if (c.status !== 'READY') problems.push('status');
    if (c.config?.kubeContext !== intended.kubeContext) problems.push('kubeContext');
    if ((c.config?.endpoint ?? c.verification?.endpoint) !== intended.endpoint) problems.push('endpoint');
    if (problems.length) throw new Error(`existing connection ${key} does not match the intended target: ${problems.join(', ')}`);
  };

  const existing = (await list()).find((c) => c.key === key);
  if (existing) {
    verify(existing);
    console.log(`REUSED ${JSON.stringify(safe(existing))}`);
  } else {
    await page.getByLabel('Connection name').fill(name);
    await page.getByLabel('Upload kubeconfig file').click();
    await page.locator('input[type="file"]').setInputFiles(kubeconfigFile);
    await expect(page.getByText(`Selected file: ${basename(kubeconfigFile)}.`, { exact: false })).toBeVisible();
    await page.getByRole('button', { name: 'Inspect kubeconfig' }).click();
    await expect(page.getByLabel('Selected destination')).toContainText(`Context ${intended.kubeContext}`);
    const registered = page.waitForResponse((r) => r.request().method() === 'POST'
      && new URL(r.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 90_000 });
    await page.getByRole('button', { name: 'Check and save' }).click();
    const response = await registered;
    const body = guard(await response.text(), 'register response');
    if (response.status() >= 300) throw new Error(`registration refused: ${response.status()} ${JSON.parse(body).code ?? ''}`);
    guard(await page.content(), 'page');
    const created = JSON.parse(body);
    if (created.verification?.verified !== true) throw new Error('registration not verified');
    const saved = (await list()).find((c) => c.key === key);
    if (!saved) throw new Error('registered connection missing from the list');
    verify(saved);
    console.log(`REGISTERED ${JSON.stringify(safe(saved))}`);
  }
} catch (error) {
  console.error(`FAIL: ${guardMessage(error)}`);
  code = 1;
} finally {
  await browser.close();
}
process.exit(code);

function guardMessage(error) {
  const text = String(error?.message ?? error).split('\n')[0];
  return secretValues.some((v) => text.includes(v)) ? 'error text withheld (credential material)' : text;
}
