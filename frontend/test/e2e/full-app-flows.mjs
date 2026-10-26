// Reusable UI flows of the full-application scenarios. Each takes the kit of full-app-kit.mjs.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { createHuman } from './human.mjs';
import { applyCatalogSources, provideSource } from './definition-sources.mjs';
import { createForwardManager, safeError, startPortForward, waitFor } from './terraform-v2-lib.mjs';
import { responseFromAction, SHORT_READ, kubeFlags, v2ctl } from './full-app-kit.mjs';

/** The Resource Types a catalog-free full-application run registers through the Console. */
export const FULL_APP_TYPES = [
  { key: 'workload', inputs: [], outputs: [] },
  { key: 'k8s-cluster', inputs: [], outputs: [['name', 'string', false], ['cluster', 'any', true]] },
  { key: 'k8s-namespace', inputs: [['name', 'string', false]], outputs: [['name', 'string', true]] },
  { key: 'postgres', inputs: [['database', 'string', true], ['username', 'string', true]], outputs: [['host', 'string', true], ['port', 'number', true], ['database', 'string', true], ['username', 'string', true], ['password', 'string', true, true]] },
  { key: 'redis', inputs: [], outputs: [['host', 'string', true], ['port', 'number', true], ['username', 'string', true], ['password', 'string', true, true], ['tls', 'bool', true]] },
];

/** Registers Resource Types through the Console form; returns the stored types. */
export async function registerTypesUi(kit, types) {
  const { page, h, nav, form, get } = kit;
  await h.click(nav('Loại tài nguyên'));
  await expect(page.getByRole('heading', { name: 'Loại tài nguyên', level: 1, exact: true })).toBeVisible();
  expect((await get('/resource-types')).resourceTypes).toHaveLength(0);
  for (const type of types) {
    await h.type(form('ID loại tài nguyên'), type.key);
    for (const [index, [name, kind, required]] of type.inputs.entries()) {
      await h.click(page.getByRole('button', { name: '+ Thêm tham số đầu vào', exact: true }));
      await h.type(form(`Đầu vào ${index + 1} tên`), name);
      if (kind !== 'string') await h.choose(form(`Đầu vào ${index + 1} kiểu`), kind);
      if (required) await h.click(form(`Đầu vào ${index + 1} bắt buộc`));
    }
    for (const [index, [name, kind, required, secret]] of type.outputs.entries()) {
      await h.click(page.getByRole('button', { name: '+ Thêm tham số đầu ra', exact: true }));
      await h.type(form(`Đầu ra ${index + 1} tên`), name);
      if (kind !== 'string') await h.choose(form(`Đầu ra ${index + 1} kiểu`), kind);
      if (required) await h.click(form(`Đầu ra ${index + 1} bắt buộc`));
      if (secret) await h.click(form(`Đầu ra ${index + 1} bí mật`));
    }
    await h.click(page.getByRole('button', { name: 'Đăng ký loại tài nguyên', exact: true }));
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký loại tài nguyên ${type.key}.` })).toBeVisible();
  }
  return (await get('/resource-types')).resourceTypes;
}

/** Main-flow Internal account upload. Returns the account key; no intentional failure probes. */
export async function onboardInternalAccount(kit) {
  const { page, h, nav, rec, input, noLeak, assertNoViolations, mark } = kit;
  await h.click(nav('Tài khoản hạ tầng'));
  await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toBeVisible();
  const create = () => page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
  const chooseFile = async (file) => {
    const chooser = page.waitForEvent('filechooser');
    chooser.catch(() => {});
    await h.click(create().locator('input[type="file"]'));
    await (await chooser).setFiles(file);
    await expect(create().getByLabel('Đích đã chọn')).toBeVisible();
    await h.pause(800);
  };
  const saveAccount = async () => {
    const response = await responseFromAction(page, (r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/driver-accounts/internal',
      () => h.click(create().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })), { timeout: 180_000 });
    let body = {};
    try { body = await response.json(); } catch { /* not JSON */ }
    assertNoViolations();
    return { status: response.status(), code: body.code, key: body.key, accountStatus: body.status, version: body.version };
  };
  await h.type(create().getByLabel('Tên tài khoản', { exact: true }), input.account.name, { replace: true });
  await chooseFile(input.account.kubeconfigFile);
  const account = await saveAccount();
  rec.record('ui.account.ready', 'PREPARE', 'ui', account.status === 201 && account.accountStatus === 'READY' && account.version === 1,
    `Internal account ${account.key} HTTP ${account.status} ${account.accountStatus} v${account.version}; kubeconfig never shown`);
  writeFileSync(`${input.privateDir}/owned-vault.json`, JSON.stringify({ accountKeys: [account.key] }), { mode: 0o600 });
  writeFileSync(`${input.privateDir}/account-key`, account.key, { mode: 0o600 });
  await noLeak('account page');
  await h.read('Tài khoản Internal đã sẵn sàng; thông tin xác thực không hiển thị');
  mark('account-ready');
  return account.key;
}

/** Existing-cluster Definition with its OWN Internal account (ADR-016), authored in the shared Definition form (UI3-05). */
export async function registerExistingCluster(kit, { id, accountName, appKey, klass = 'internal' }) {
  const { page, h, nav, form: scopedForm, choose, get, rec, mark } = kit;
  await h.click(nav('Cấu hình tài nguyên'));
  await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
  await h.click(page.getByRole('button', { name: 'Tạo cấu hình', exact: true }));
  const section = page.getByRole('region', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
  const form = (name) => scopedForm(name, section);
  await h.type(form('ID cấu hình (Definition ID)'), id);
  await h.type(form('Tên hiển thị'), 'Cluster k8s-4f (có sẵn)');
  await choose(form('Loại tài nguyên'), 'k8s-cluster');
  await choose(form('Cách cung cấp cluster'), 'Dùng cluster có sẵn');
  await choose(form('Tài khoản hạ tầng'), accountName);
  await h.click(section.getByRole('tab', { name: /Điều kiện áp dụng/ }));
  if (appKey) {
    await h.click(section.getByRole('button', { name: '+ Thêm điều kiện', exact: true }));
    await section.getByLabel('Trường hợp 1 thuộc tính 1').selectOption('app_id');
    await h.type(section.getByLabel('Điều kiện 1 ID ứng dụng', { exact: true }), appKey);
  }
  await h.click(section.getByRole('button', { name: '+ Thêm điều kiện', exact: true }));
  await section.getByLabel(`Trường hợp 1 thuộc tính ${appKey ? 2 : 1}`).selectOption('class');
  await h.type(section.getByLabel('Điều kiện 1 Class', { exact: true }), klass);
  await h.pause(SHORT_READ);
  await h.click(section.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }));
  await expect(section.getByRole('status')).toContainText(`Đã đăng ký cấu hình tài nguyên ${id}.`);
  const stored = (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === id);
  rec.record('ui.definition.cluster', 'FORM', 'ui', stored?.driverType === 'existing-cluster' && Boolean(stored.driverAccount) && stored.version === 1,
    `cluster Definition ${id} uses its own Internal account ${stored?.driverAccount}; no Environment connection`);
  mark('cluster-definition');
  return stored;
}

/**
 * A Terraform module Definition whose Kubernetes provider is bound to a cluster resource of class `clusterClass`.
 * spec: { id, name, type, module, klass?, presetName?, presetNamespace?, clusterClass }
 */
export async function registerK8sModuleDefinition(kit, spec) {
  const { page, h, choose, get } = kit;
  const section = page.getByRole('region', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
  await h.click(section.getByRole('tab', { name: 'Cấu hình', exact: true }));
  const form = (name) => kit.form(name, section);
  await h.type(form('ID cấu hình (Definition ID)'), spec.id);
  await h.type(form('Tên hiển thị'), spec.name ?? spec.id);
  await choose(form('Loại tài nguyên'), spec.type);
  if (spec.git) {
    await h.click(section.getByRole('button', { name: 'Thêm từ Git', exact: true }));
    await h.type(section.getByLabel('Git HTTPS URL', { exact: true }), spec.git.url);
    await h.type(section.getByLabel('Thư mục module', { exact: true }), spec.git.path);
    await h.type(section.getByLabel('Tag hoặc commit', { exact: true }), spec.git.ref, { replace: true });
    const inspected = await responseFromAction(page,
      (r) => new URL(r.url()).pathname === '/api/v1/terraform-modules/inspect',
      () => h.click(section.getByRole('button', { name: 'Kiểm tra module', exact: true })), { timeout: 180_000 });
    expect(inspected.status()).toBe(200);
  } else {
    await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
    const pins = form('Module đã kiểm chứng');
    if (await pins.isEnabled()) await choose(pins, spec.module);
  }
  const advanced = section.locator('details.definition-advanced');
  if (await advanced.getAttribute('open') === null) await h.click(advanced.locator('summary').first());
  const inputMapping = section.getByRole('button', { name: 'Ghép đầu vào theo tên trùng', exact: true });
  if (await inputMapping.isVisible()) await h.click(inputMapping);
  await h.click(section.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
  await h.choose(section.getByLabel(/^Nhóm cluster của provider /), spec.clusterClass);
  await applyCatalogSources(kit, spec);
  // Keep typed mode: JSON mode makes the cluster source read-only, and replacing its
  // object would discard the provider's namespace dependency. Values here are
  // actual JS values (unlike the JSON-encoded catalog overrides).
  for (const [name, value] of Object.entries(spec.variables ?? {})) await provideSource(kit, name, { literal: value });
  if (spec.klass) {
    await h.click(section.getByRole('tab', { name: /Điều kiện áp dụng/ }));
    await h.click(section.getByRole('button', { name: '+ Thêm điều kiện', exact: true }));
    await section.getByLabel('Trường hợp 1 thuộc tính 1').selectOption('class');
    await h.type(form('Điều kiện 1 Class'), spec.klass);
  }
  await h.pause(SHORT_READ);
  await h.click(section.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }));
  await expect(section.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${spec.id}.` })).toBeVisible();
  return (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === spec.id);
}

/** Workload rendering template: literal bundle + whole namespace/cluster references (+ optional companion rule). */
export async function registerWorkloadTemplate(kit, { id, bundle, companion, clusterRef }) {
  const { page, h, nav, form, get } = kit;
  await h.click(nav('Mẫu dựng ứng dụng'));
  await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
  await h.type(form('ID mẫu'), id);
  await h.type(form('Tên hiển thị'), 'Mẫu dựng ứng dụng đầy đủ (score-k8s)');
  await expect(form('Bundle dựng ứng dụng')).toHaveValue(bundle);
  await h.click(page.getByRole('button', { name: 'Dùng gợi ý namespace', exact: true }));
  if (clusterRef) {
    // An EKS cluster the plan itself creates: the author states the reference to the cluster RESOURCE of class eks.
    await h.type(page.getByLabel('Tham chiếu cluster', { exact: true }), clusterRef, { replace: true });
  } else {
    await h.click(page.getByRole('button', { name: 'Dùng gợi ý cluster', exact: true }));
  }
  if (companion) {
    await h.click(page.getByRole('button', { name: '+ Thêm tài nguyên đi kèm', exact: true }));
    await h.type(page.getByLabel('Descriptor tài nguyên đi kèm 1', { exact: true }), companion.descriptor);
    await h.type(page.getByLabel('Params tài nguyên đi kèm 1 (JSON object)', { exact: true }), JSON.stringify(companion.params), { replace: true });
  }
  await h.pause(SHORT_READ);
  await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }));
  await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký mẫu dựng ứng dụng ${id}.` })).toBeVisible();
  return (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === id);
}

/** Environment Settings: public configuration keys and ONE secret through the registered Secret Store. */
export async function configureEnvironment(kit, { config, secretName, secretValue, storeKey }) {
  const { h, input } = kit;
  const { putKeys } = await import('./human.mjs');
  const entries = config.map((c) => ({ kind: 'variable', name: c.name, value: c.value }));
  entries.push({ kind: 'secret', name: secretName, value: secretValue, paste: true });
  await putKeys(h, input.applicationName, entries, { secretStore: storeKey });
}

/** Live technical GET through the same real browser/frontend as the UI. No HTTP cache or fallback data. */
export async function readAppJSON(page, path) {
  return page.evaluate(async (route) => {
    const response = await fetch(route, { method: 'GET', cache: 'no-store', signal: AbortSignal.timeout(15_000) });
    if (response.status !== 200) throw new Error(`acceptance frontend API returned HTTP ${response.status}`);
    return response.json();
  }, path);
}

/** Opens the real frontend through a managed port-forward (re-established after every rollout) and returns { app, ah, forwarding }. */
export async function openFrontend(kit, serviceName = 'frontend') {
  const { context, input, forwarders } = kit;
  const healthy = async (url) => fetch(`${url}/healthz`).then((r) => r.ok).catch(() => false);
  const forwarding = createForwardManager({ ...input.kube, namespace: input.namespace, service: serviceName, remotePort: input.servicePort ?? 8080, localPort: input.forwardPort }, { forwarders, start: startPortForward, healthy });
  await forwarding.connect('the frontend Service to answer through the port-forward');
  const app = await context.newPage();
  const ah = createHuman(app, { selectNative: kit.selectNative, fast: kit.h.fast === true });
  await app.goto(`${forwarding.url}/`);
  await expect(app.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
  await ah.showCursor();
  return { app, ah, forwarding };
}

/** After a rollout the old forward is dead: connect again and reload the real page. */
export async function reconnectFrontend(front, what) {
  await front.forwarding.connect(what);
  await front.app.goto(`${front.forwarding.url}/`);
  await expect(front.app.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
}

export const CHECK_ROWS = ['backend connection', 'environment', 'secret', 'database', 'cache'];

/** Reads the five check rows from the real page as [name, status] pairs. */
export const readCheckRows = (app) => app.locator('#checks tr[data-check]').evaluateAll((rows) => rows.map((row) => [row.getAttribute('data-check'), row.children[1].textContent.trim()]));

/** Waits (reloading the real page) until every given check row shows PASS. */
export async function waitRowsPass(app, rows = CHECK_ROWS, timeoutMs = 240_000) {
  for (const name of rows) {
    const row = app.locator(`[data-check="${name}"]`);
    await waitFor(async () => { await app.reload(); return (await row.innerText()).includes('PASS'); }, { timeoutMs, intervalMs: 3000, what: `the ${name} check to PASS` });
  }
}

/** Submits a job through the real form, waits for the worker, returns the persisted row cells. */
export async function submitJob(ah, app, payload) {
  await ah.type(app.locator('input[name="payload"]'), payload, { replace: true });
  await ah.click(app.getByRole('button', { name: 'Submit job' }));
  const row = app.locator('#jobs tr.job').filter({ hasText: payload }).first();
  await expect(row).toBeVisible();
  await waitFor(async () => { await app.reload(); return (await row.innerText()).includes('DONE'); }, { timeoutMs: 240_000, intervalMs: 1500, what: 'the worker to complete the job' });
  const id = Number(await row.getAttribute('data-id'));
  const cells = (await row.locator('td').allInnerTexts()).map((t) => t.trim());
  await ah.moveTo(row);
  await ah.pause(SHORT_READ);
  return { id, cells, row };
}

export const cacheKeyOf = (input, jobId) => `acceptance:v1:${input.applicationKey}:staging:${input.runId}:job:${jobId}`;

export { execFileSync, readFileSync, safeError, kubeFlags, v2ctl };

/**
 * AWS account upload (V01 UI): masked key fields, "Vùng kiểm tra" is probe evidence only. The Definitions, not the account or the
 * Environment, state the literal deployment region. Returns the stored account key.
 */
export async function onboardAwsAccount(kit, aws) {
  const { page, h, nav, get, rec, noLeak, assertNoViolations } = kit;
  await h.click(nav('Tài khoản hạ tầng'));
  await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toBeVisible();
  const create = () => page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
  await h.click(create().getByLabel('AWS', { exact: true }));
  const keyField = create().getByLabel('Access key ID', { exact: true });
  const secretField = create().getByLabel('Secret access key', { exact: true });
  await expect(keyField).toHaveAttribute('type', 'password');
  await expect(secretField).toHaveAttribute('type', 'password');
  await h.type(create().getByLabel('Tên tài khoản', { exact: true }), aws.name, { replace: true });
  await h.paste(keyField, aws.accessKeyId);
  await h.paste(secretField, aws.secretAccessKey);
  await h.type(create().getByLabel('Vùng kiểm tra', { exact: true }), aws.region, { replace: true });
  expect(await page.locator('body').innerText(), 'key material must not be visible').not.toContain(aws.secretAccessKey);
  const response = await responseFromAction(page, (r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/driver-accounts/aws',
    () => h.click(create().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })), { timeout: 180_000 });
  const text = await response.text();
  if (text.includes(aws.secretAccessKey) || text.includes(aws.accessKeyId)) throw new Error('the registration response echoed key material');
  if (response.status() !== 201) throw new Error(`AWS account registration was refused (HTTP ${response.status()})`);
  assertNoViolations();
  const body = JSON.parse(text);
  const listed = ((await get('/driver-accounts')).accounts ?? []).find((a) => a.key === body.key);
  rec.record('ui.account.ready', 'PREPARE', 'ui', listed?.status === 'READY' && listed.type === 'AWS' && listed.version === 1, `AWS account ${body.key} ${listed?.status} v${listed?.version}; key fields cleared; the verification region is evidence, not a deployment region`);
  expect(await keyField.inputValue()).toBe('');
  expect(await secretField.inputValue()).toBe('');
  await noLeak('aws account page');
  await h.read('Tài khoản AWS đã được kiểm tra và sẵn sàng; hai ô khóa đã được xóa');
  return body.key;
}

/**
 * Registers one authored catalog entry (definition-catalog.json) through the Resource Definition form. AWS entries choose the
 * Driver Account by name and type the literal region; Kubernetes entries bind their provider to the cluster RESOURCE of the
 * entry's class. A relation is only ever a resource reference or a provider binding: there is no separate dependency field.
 */
export async function registerCatalogDefinition(kit, entry, { accountName, region, subs }) {
  const { page, h, choose, get } = kit;
  const create = page.getByRole('button', { name: 'Tạo cấu hình', exact: true });
  if (await create.isVisible()) await h.click(create);
  const section = page.getByRole('region', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
  await h.click(section.getByRole('tab', { name: 'Cấu hình', exact: true }));
  const form = (name) => kit.form(name, section);
  const { decodeCatalogOverride } = await import('./terraform-v2-steps.mjs');
  await h.type(form('ID cấu hình (Definition ID)'), entry.id);
  await h.type(form('Tên hiển thị'), entry.id);
  await choose(form('Loại tài nguyên'), entry.type);
  await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
  await expect(form('Loại tài nguyên')).toHaveValue(entry.type);
  const pins = form('Module đã kiểm chứng');
  if (await pins.isEnabled()) await choose(pins, entry.module);
  await expect(pins).toContainText(entry.module);
  if (entry.provider === 'aws') {
    await choose(form('Tài khoản hạ tầng'), accountName);
    await h.type(form('Vùng AWS'), region);
  }
  const advanced = section.locator('details.definition-advanced');
  if (await advanced.getAttribute('open') === null) await h.click(advanced.locator('summary').first());
  const inputMapping = section.getByRole('button', { name: 'Ghép đầu vào theo tên trùng', exact: true });
  if (await inputMapping.isVisible()) await h.click(inputMapping);
  await h.click(section.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
  if (entry.provider === 'kubernetes') {
    await h.choose(section.getByLabel(/^Nhóm cluster của provider /), entry.clusterClass);
  }
  await applyCatalogSources(kit, entry);
  for (const [name, encoded] of entry.overrides ?? []) await provideSource(kit, name, { literal: decodeCatalogOverride(encoded, subs) });
  if (entry.klass) {
    await h.click(section.getByRole('tab', { name: /Điều kiện áp dụng/ }));
    await h.click(section.getByRole('button', { name: '+ Thêm điều kiện', exact: true }));
    await section.getByLabel('Trường hợp 1 thuộc tính 1').selectOption('class');
    await h.type(form('Điều kiện 1 Class'), entry.klass);
  }
  await h.pause(SHORT_READ);
  await h.click(section.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }));
  await expect(section.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${entry.id}.` })).toBeVisible();
  return (await get('/resource-definitions')).resourceDefinitions.find((d) => d.key === entry.id);
}
