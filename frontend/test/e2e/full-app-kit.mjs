// Shared building blocks of the full-application acceptance scenarios (V04 existing cluster, V05 EKS,
// V06 managed A02/A03). Every business step is a Web Console or acceptance-app UI action. The only
// non-UI access is (a) the labelled read-only technical reads (kubectl through the private host handle,
// psql/redis-cli INSIDE the run's own pods with the password on stdin only) and (b) the labelled API setup
// calls, each recorded in setup-api-calls.txt. Nothing here creates a job, a Workload or a resource by API.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { appendFileSync, readFileSync } from 'node:fs';
import { kubectlRunner, pickByImage } from './terraform-v2-lib.mjs';
import { steps as v2steps } from './terraform-v2-steps.mjs';

export const SHORT_READ = 2000;
export const FIXTURE_PASSWORD = 'test-password'; // fixed synthetic fixture account of the isolated run, typed into a masked field
// Credential MATERIAL only (never a word such as "kubeconfig" or "terraform" that the UI legitimately shows).
export const FORBIDDEN_IN_PAGE = [/AKIA[0-9A-Z]{16}/, /vault:\/\//i, /secretRef/i, /BEGIN [A-Z ]*PRIVATE KEY/, /client-key-data/i, /client-certificate-data/i, /hvs\.[A-Za-z0-9_-]{20,}/];

/** Exact-text pattern source for a descriptor: `x` must not match `x-aux`. */
export const exactTextPattern = (text) => `^${String(text).replace(/[.*+?^${}()|[\]\\#]/g, '\\$&')}$`;

// Attach both handlers before a human action can wait or fail. A response timeout
// must reach the scenario catch/finalizer instead of crashing Node unhandled.
export async function responseFromAction(page, predicate, action, options) {
  const [response] = await Promise.all([page.waitForResponse(predicate, options), Promise.resolve().then(action)]);
  return response;
}

export async function requiredJSON(response, path) {
  if (!response.ok()) throw new Error(`required GET ${path} returned HTTP ${response.status()}`);
  const body = await response.json();
  if (!body || typeof body !== 'object' || Array.isArray(body)) throw new Error(`required GET ${path} returned a malformed object`);
  return body;
}

export function namespaceLookupResult(result) {
  if (result.code === 0) {
    const uid = result.stdout.trim();
    if (!uid) throw new Error('namespace UID response was empty');
    return uid;
  }
  if (/\bnotfound\b/i.test(result.stderr)) return '';
  throw new Error('namespace lookup failed; absence is not proven');
}


export function v2ctl(input, ...args) {
  return execFileSync(input.v2ctl, args, { encoding: 'utf8', env: { PATH: process.env.PATH }, stdio: ['ignore', 'pipe', 'pipe'] });
}
export const kubeFlags = (input) => ['-kubectl', input.kube.kubectl, '-kubeconfig', input.kube.kubeconfig, '-context', input.kube.context];

/**
 * Private technical reads of the run's own namespace. The database/Redis password is read from the Secret the
 * Pod uses and travels on stdin only: never in an argv, a log, an error text or evidence.
 */
export function technical(input) {
  const kubectl = kubectlRunner(input.kube);
  const ns = input.namespace;
  const json = async (args) => {
    const r = await kubectl(args);
    if (r.code !== 0) throw new Error(`kubectl ${args[0]} failed`);
    return JSON.parse(r.stdout);
  };
  const secretValue = async (name, key) => {
    const r = await kubectl(['-n', ns, 'get', 'secret', name, '-o', `jsonpath={.data.${key}}`]);
    if (r.code !== 0 || !r.stdout) throw new Error('secret value is not readable');
    return Buffer.from(r.stdout, 'base64').toString('utf8');
  };
  const envOf = (deployment, name) => (deployment.spec.template.spec.containers[0].env ?? []).find((e) => e.name === name);
  async function resolvedEnv(deployment, name) {
    const entry = envOf(deployment, name);
    if (!entry) throw new Error(`${name} is not bound`);
    if (entry.value !== undefined) return entry.value;
    const ref = entry.valueFrom?.secretKeyRef;
    if (!ref) throw new Error(`${name} has no value source`);
    return secretValue(ref.name, ref.key);
  }
  return {
    kubectl, json, envOf, resolvedEnv,
    namespaceUid: async (name = ns) => {
      const r = await kubectl(['get', 'namespace', name, '-o', 'jsonpath={.metadata.uid}']);
      return namespaceLookupResult(r);
    },
    deployments: () => json(['-n', ns, 'get', 'deployments', '-o', 'json']),
    statefulsets: () => json(['-n', ns, 'get', 'statefulsets', '-o', 'json']),
    pods: () => json(['-n', ns, 'get', 'pods', '-o', 'json']),
    async psql(deployment, sql) {
      const pg = pickByImage(await this.statefulsets(), 'postgres')[0];
      if (!pg) throw new Error('no PostgreSQL StatefulSet');
      const [user, db, password] = await Promise.all([resolvedEnv(deployment, 'PGUSER'), resolvedEnv(deployment, 'PGDATABASE'), resolvedEnv(deployment, 'PGPASSWORD')]);
      const script = 'IFS= read -r PGPASSWORD; export PGPASSWORD; exec psql -h 127.0.0.1 -U "$0" -d "$1" -tAF "|" -c "$2"';
      const r = await kubectl(['-n', ns, 'exec', '-i', `statefulset/${pg.metadata.name}`, '--', 'sh', '-c', script, user, db, sql], { timeoutMs: 60_000, input: `${password}\n` });
      if (r.code !== 0) throw new Error('psql query failed');
      return r.stdout.trim();
    },
    async redis(deployment, ...command) {
      const rd = pickByImage(await this.statefulsets(), 'redis')[0];
      if (!rd) throw new Error('no Redis StatefulSet');
      const [user, password] = await Promise.all([resolvedEnv(deployment, 'REDIS_USERNAME'), resolvedEnv(deployment, 'REDIS_PASSWORD')]);
      return this.redisAs(rd, user, password, ...command);
    },
    async redisAs(statefulSet, user, password, ...command) {
      const rd = typeof statefulSet === 'string' ? (await this.statefulsets()).items.find((s) => s.metadata.name === statefulSet) : statefulSet;
      if (!rd) throw new Error('Redis StatefulSet is not present');
      const script = 'IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; exec redis-cli --no-auth-warning --user "$0" "$@"';
      const r = await kubectl(['-n', ns, 'exec', '-i', `statefulset/${rd.metadata.name}`, '--', 'sh', '-c', script, user, ...command], { timeoutMs: 30_000, input: `${password}\n` });
      if (r.code !== 0) throw new Error('redis-cli failed');
      return r.stdout.trim();
    },
    async redisAsPrivateCommand(statefulSet, user, password, ...command) {
      const rd = typeof statefulSet === 'string' ? (await this.statefulsets()).items.find((s) => s.metadata.name === statefulSet) : statefulSet;
      if (!rd) throw new Error('Redis StatefulSet is not present');
      const script = 'set -o pipefail; IFS= read -r REDISCLI_AUTH; export REDISCLI_AUTH; IFS= read -r redis_command; printf "%s\\n" "$redis_command" | redis-cli --quoted-input --no-auth-warning --user "$0"';
      const r = await kubectl(['-n', ns, 'exec', '-i', `statefulset/${rd.metadata.name}`, '--', 'sh', '-c', script, user], { timeoutMs: 30_000, input: `${password}\n${command.join(' ')}\n` });
      if (r.code !== 0) throw new Error('private Redis command failed');
      return r.stdout.trim();
    },
  };
}

/** Appends a run-created secret value to the private scan input (never printed). */
export function recordAppSecret(input, name, value) {
  appendFileSync(`${input.privateDir}/app-secrets.txt`, `${name}=${value}\n`, { mode: 0o600 });
}

/** Builds the page-level helpers every full-application scenario shares. */
export function makeKit(ctx) {
  const { page, h, context, x11, caption, phase, rec, baseURL, input, forwarders, evidenceDir } = ctx;
  const mark = ctx.mark ?? (() => {});
  const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
  const get = async (path) => requiredJSON(await page.request.get(`${baseURL}/api/v1${path}`), path);
  const wrapped = new Set(['Loại tài nguyên', 'Module đã kiểm chứng', 'Tài khoản hạ tầng', 'Bundle dựng ứng dụng', 'Cách cung cấp cluster', 'Nhóm cluster của provider kubernetes', 'Tài khoản Internal của cluster']);
  // Match the beginning of a wrapping label, not text inside its <option>s:
  // a Type awaiting its first module says "chưa có module đã kiểm chứng" and
  // must not turn the Type selector into the module selector.
  const form = (name, scope = page) => (wrapped.has(name)
    ? scope.locator(`select[aria-label="${name}"]`).or(scope.locator('label')
      .filter({ hasText: new RegExp(`^\\s*${exactTextPattern(name).slice(1, -1)}`) }).locator(':scope > select')).first()
    : scope.getByLabel(name, { exact: true }));
  const alertBox = () => page.getByRole('alert').first();
  const optionLabel = async (select, prefix) => {
    let found = '';
    await expect.poll(async () => {
      const texts = await select.locator('option').allTextContents();
      found = texts.find((text) => text.trim().startsWith(prefix)) ?? '';
      return found;
    }, { message: `waiting for option starting with "${prefix}"`, timeout: 30_000 }).not.toBe('');
    return found;
  };
  const choose = async (select, prefix) => h.choose(select, await optionLabel(select, prefix));
  const noLeak = async (where) => {
    const text = await page.locator('body').innerText();
    for (const pattern of FORBIDDEN_IN_PAGE) expect(pattern.test(text), `${where}: the page shows credential-like text`).toBe(false);
  };
  // Every /api/v1 reply is scanned for credential-bearing data (also replies the scenario never waits on).
  const violations = [];
  page.on('response', async (response) => {
    const url = new URL(response.url());
    if (!url.pathname.startsWith('/api/v1/')) return;
    const text = await response.text().catch(() => '');
    if (FORBIDDEN_IN_PAGE.some((pattern) => pattern.test(text))) {
      violations.push(`${response.request().method()} ${url.pathname} ${response.status()}`);
      appendFileSync(`${evidenceDir}/assertions.txt`, `FAIL ${response.request().method()} ${url.pathname} carried credential-like data\n`);
    }
  });
  const assertNoViolations = () => expect(violations, 'a reply carried credential-bearing data').toEqual([]);
  const apiSetup = (label) => appendFileSync(`${evidenceDir}/setup-api-calls.txt`, `${label}\n`);
  const appPath = `/applications/${input.applicationKey}/environments/staging`;
  const tech = technical(input);

  async function signIn(username) {
    await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), username);
    await h.type(page.getByLabel('Mật khẩu', { exact: true }), FIXTURE_PASSWORD);
    await expect(page.getByLabel('Mật khẩu', { exact: true })).toHaveAttribute('type', 'password');
    await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
    await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
  }
  async function signOut() {
    await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
    await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();
  }
  // The AWS harness supplies a deploy() that records the Preview's Terraform run ID in the private manifest first.
  const deploy = ctx.deploy ?? (() => h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true })));
  async function backToApplication(name = input.applicationName) {
    const heading = page.getByRole('heading', { name });
    for (let i = 0; i < 5 && !(await heading.isVisible()); i += 1) await h.click(page.getByRole('button', { name: /^←/ }).first());
    await expect(heading).toBeVisible();
  }
  async function openApplication(name = input.applicationName) {
    await h.click(page.getByRole('button', { name: new RegExp(name) }).first());
    await expect(page.getByRole('heading', { name })).toBeVisible();
  }
  const variablesSection = () => page.locator('section[aria-label="Biến"]');
  async function openSettings() {
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await expect(variablesSection()).toBeVisible();
    await h.pause(800);
  }
  async function saveVariable() {
    const name = await page.getByLabel('Tên key', { exact: true }).inputValue();
    const response = await responseFromAction(page, (r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/configuration/keys/${encodeURIComponent(name)}`),
      () => h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })));
    if (!response.ok()) throw new Error(`saving variable ${name} returned HTTP ${response.status()}`);
    await expect(page.getByLabel('Tên key', { exact: true })).toHaveCount(0);
    await h.pause(600);
  }
  async function editVariable(name, value) {
    const row = variablesSection().locator('.settings-table-row').filter({ has: page.locator('strong', { hasText: new RegExp(`^${name}$`) }) });
    await h.click(row.getByRole('button', { name: 'Sửa', exact: true }));
    await h.type(page.getByLabel('Giá trị', { exact: true }), value, { replace: true });
    await saveVariable();
  }
  const leaveSettings = (name = input.applicationName) => h.click(page.getByRole('button', { name: `← ${name}` }));
  const importScore = async (file, expectName) => {
    await h.click(page.getByRole('button', { name: 'Nhập Score', exact: true }));
    const chooser = page.waitForEvent('filechooser');
    chooser.catch(() => {});
    await h.click(page.getByLabel('File Score', { exact: true }));
    await (await chooser).setFiles(file);
    await expect(page.getByText(`Đã phân tích Workload: ${expectName}`, { exact: false })).toBeVisible();
    await h.pause(SHORT_READ);
    const response = await responseFromAction(page, (r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/workloads/${expectName}`),
      () => h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true })));
    if (!response.ok()) throw new Error(`saving the Score draft ${expectName} returned HTTP ${response.status()}`);
    await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
  };
  const addWorkloadFromScore = async (file, name) => {
    await h.click(page.getByRole('button', { name: '+ Thêm Workload', exact: true }));
    await importScore(file, name);
  };
  const editWorkloadFromScore = async (file, name) => {
    const row = page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) });
    await h.click(row.getByRole('button', { name: 'Sửa', exact: true }));
    await importScore(file, name);
  };
  // The Score Preview page (UC-05) shows the plan of the SAME Score and Environment snapshot: graph edges, matches, batches and pins.
  const scorePreview = async (scoreFile, workloadId, tag) => {
    await h.click(page.getByRole('button', { name: 'Xem trước Score', exact: true }));
    await h.type(page.getByLabel('ID Workload', { exact: true }), workloadId, { replace: true });
    await h.type(page.getByLabel('ID lần chạy', { exact: true }), `${tag}-${Date.now() % 100000}`, { replace: true });
    await h.paste(page.getByLabel(/Score sau/), readFileSync(scoreFile, 'utf8'));
    const response = await responseFromAction(page, (r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith('/score-preview'),
      () => h.click(page.getByRole('button', { name: 'Xem trước', exact: true })));
    return { status: response.status(), body: await response.json().catch(() => ({})) };
  };
  const pendingPreview = async (expectedWorkloads) => {
    const previewResponse = await responseFromAction(page, (r) => r.request().method() === 'POST' && new URL(r.url()).pathname.endsWith(`${appPath}/preview`),
      () => h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true })));
    const preview = await previewResponse.json();
    if (!previewResponse.ok()) {
      const code = /^[A-Z0-9_]{1,64}$/.test(preview?.code ?? '') ? preview.code : 'UNCLASSIFIED';
      const reason = /^[a-z-]{1,64}$/.test(preview?.params?.reason ?? '') ? preview.params.reason : 'unspecified';
      throw new Error(`main Preview HTTP ${previewResponse.status()} ${code} (${reason})`);
    }
    const panel = page.getByLabel('Xem trước triển khai');
    await expect(panel).toContainText(`${expectedWorkloads} Workload bị ảnh hưởng`, { timeout: 120_000 });
    return { preview, panel };
  };
  const deployAndWait = async (timeoutMs) => {
    await deploy();
    const result = page.getByLabel('Kết quả triển khai');
    await expect(result).toContainText(/Triển khai (thành công|thất bại)/, { timeout: timeoutMs });
    await expect(result).toContainText('Triển khai thành công');
    return result;
  };
  /** Opens the newest deployment's detail page and returns the persisted detail JSON of every deployment. */
  const openLatestDeploymentDetail = async () => {
    await h.click(page.getByRole('button', { name: 'Xem tất cả', exact: true }));
    await h.click(page.locator('button.deployment-entry').first());
    const region = page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' });
    await expect(region).toBeVisible();
    await h.moveTo(region);
    await h.pause(SHORT_READ);
    return region;
  };
  const platform = v2steps({ page, h, caption, phase, rec, baseURL, input });
  return {
    ...ctx, mark, nav, get, form, alertBox, optionLabel, choose, noLeak, assertNoViolations, apiSetup, appPath, tech, signIn, signOut, deploy, backToApplication, openApplication,
    openSettings, saveVariable, editVariable, leaveSettings, importScore, addWorkloadFromScore, editWorkloadFromScore, scorePreview, pendingPreview, deployAndWait, openLatestDeploymentDetail, platform, variablesSection, context, x11, forwarders,
  };
}
