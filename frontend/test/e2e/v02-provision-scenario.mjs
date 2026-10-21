// V02P headed recording: a Platform Engineer authors the companion-resource
// ("provision") rules of a Resource Definition through the Web Console and
// proves the stored rules drive the planner. Registered as scenario V02P of
// v02-definitions-local.mjs and started by
// backend/test/integration/v02-provision-local.sh.
//
// UC-03 BR-26 / UC-06 BR-08: every rule is a companion descriptor with nested
// typed params and the two flags is_dependent / match_dependents. A reference
// edge always points consumer -> provider and the provider is scheduled first.
//
// REAL: the Web Console bundle, the product HTTP handlers, the planner, a
// disposable PostgreSQL Console store and a Vault KV v2 credential store that
// holds the fixture Internal account's kubeconfig.
// FAKE (labelled in the captions): the Kubernetes verification of the Internal
// account (deterministic token-prefix fake, awsconnserver -fake-kubernetes).
// Nothing is deployed: Terraform, a cluster, AWS and Docker are never
// contacted. The Score Preview only PLANS (graph, batches, resource intents);
// a PASS is NOT evidence that any companion resource was created.
//
// UI actions: every Definition create/edit/save, descriptor, params, flag,
// form <-> advanced JSON switch and every Score Preview. API calls are used
// only (a) for labelled fixtures (Resource Types, the fake-verified Internal
// account, its Environment connection, the namespace and companion Definitions),
// (b) to assert stored results and (c) for the labelled negative check.
import { expect } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { appendFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { LONG_TYPE_DELAY } from './human.mjs';
import { assertNoSecretLeaks } from './secret-scan.mjs';

const SHORT_READ = 2200;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, typed into a masked field
const APP = 'acceptance';
const WORKLOAD = 'api';
const CONSUMER = `workload.default#modules.${WORKLOAD}`;

const IS_DEPENDENT = 'co-provision-is-dependent';
const MATCH_DEPENDENTS = 'co-provision-match-dependents';

const kubeconfig = (tokenValue) => `apiVersion: v1
kind: Config
clusters:
- name: fake-lab-cluster
  cluster:
    server: https://fake-lab.example:6443
users:
- name: fake-lab-user
  user:
    token: ${tokenValue}
contexts:
- name: fake-lab
  context:
    cluster: fake-lab-cluster
    user: fake-lab-user
current-context: fake-lab
`;

const score = JSON.stringify({
  apiVersion: 'score.dev/v1b1', metadata: { name: WORKLOAD },
  containers: { main: { image: 'example.invalid/api:v1' } },
  resources: { cache: { type: 'redis', class: 'prov' } },
});

export async function provisionScenario({ page, h, caption, mark, check, baseURL, fail, evidenceDir }) {
  const privateDir = process.env.ORCH_E2E_PRIVATE_DIR;
  const kubeToken = `FAKEK8S-OK-${randomBytes(12).toString('hex')}`;
  const allSecrets = [kubeToken];
  // The runner compares the stored credential with this value after the run.
  writeFileSync(join(privateDir, 'secret-values'), `${kubeToken}\n`, { mode: 0o600 });
  const suffix = /^[0-9a-f]{4,12}$/.test(process.env.ORCH_V02_SUFFIX ?? '') ? process.env.ORCH_V02_SUFFIX : randomBytes(2).toString('hex');
  const ID = { parent: `redis-prov-${suffix}`, ns: `ns-lab-${suffix}`, companion: `pg-companion-${suffix}` };
  const names = { parent: `Redis có tài nguyên đi kèm ${suffix}`, internal: `Cluster lab ${suffix}` };
  const D = 'postgres.companion#shared.dep-only';
  const M = 'postgres.companion#shared.match-only';
  const B = 'postgres.companion#shared.both-flags';
  const N = 'postgres.companion#shared.plain';
  const PARENT = 'redis.prov#modules.api.externals.cache';

  // Every /api/v1 reply is scanned for key material and secret references.
  const violations = [];
  const writes = [];
  const previews = [];
  page.on('response', async (response) => {
    const url = new URL(response.url());
    if (!url.pathname.startsWith('/api/v1/')) return;
    const method = response.request().method();
    const text = await response.text().catch(() => '');
    const leaked = allSecrets.some((value) => text.includes(value)) || /secretRef|kv2:\/\//.test(text);
    if (leaked) {
      violations.push(`${method} ${url.pathname} status ${response.status()}`);
      appendFileSync(`${evidenceDir}/assertions.txt`, `FAIL ${method} ${url.pathname} (${response.status()}) carried key material or a secret reference\n`);
    }
    let body = {};
    try { body = JSON.parse(text); } catch { /* empty */ }
    if (url.pathname.startsWith('/api/v1/resource-definitions') && ['POST', 'PUT'].includes(method)) {
      writes.push({ method, path: url.pathname.replace('/api/v1/resource-definitions', '') || '/', status: response.status(), code: leaked ? undefined : body.code, key: leaked ? undefined : body.key, version: leaked ? undefined : body.version });
    }
    if (url.pathname.endsWith('/score-preview') && method === 'POST' && !leaked) {
      previews.push({ status: response.status(), code: body.code, stage: body.params?.stage, graph: body.graph, batches: body.batches });
    }
  });
  const assertNoViolations = () => expect(violations, 'a reply carried secret-bearing data').toEqual([]);
  async function sent(action) {
    const before = writes.length;
    await action();
    await expect.poll(() => writes.length, { timeout: 60_000 }).toBeGreaterThan(before);
    assertNoViolations();
    return writes[writes.length - 1];
  }

  const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
  const wrappedSelects = new Set(['Loại tài nguyên', 'Module đã kiểm chứng', 'Cluster đích của provider kubernetes']);
  const form = (name) => (wrappedSelects.has(name)
    ? page.locator(`select[aria-label="${name}"], label:has-text("${name}") > select`).first()
    : page.getByLabel(name, { exact: true }));
  const submitCreate = () => page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
  const submitSave = () => page.getByRole('button', { name: 'Lưu thay đổi', exact: true });
  const entry = (id) => page.locator(`[data-definition="${id}"]`);
  const alertBox = () => page.getByRole('alert').first();
  const provisionSection = () => page.getByRole('region', { name: 'Quy tắc tạo tài nguyên liên quan' });
  const modeForm = () => page.getByRole('button', { name: 'Quy tắc: biểu mẫu', exact: true });
  const modeJson = () => page.getByRole('button', { name: 'Quy tắc: JSON nâng cao', exact: true });
  const jsonBox = () => provisionSection().getByLabel('Quy tắc tạo tài nguyên liên quan (JSON object)');
  const descriptorOf = (n) => page.getByLabel(`Descriptor tài nguyên đi kèm ${n}`, { exact: true });
  const paramsOf = (n) => page.getByLabel(`Params tài nguyên đi kèm ${n} (JSON object)`, { exact: true });
  const dependentOf = (n) => page.getByLabel(`Tài nguyên đi kèm ${n} phụ thuộc vào tài nguyên cha (is_dependent)`, { exact: true });
  const matchOf = (n) => page.getByLabel(`Nối consumer của tài nguyên cha tới tài nguyên đi kèm ${n} (match_dependents)`, { exact: true });
  const api = async (path) => (await (await page.request.get(`${baseURL}/api/v1${path}`)).json());
  const optionLabel = async (select, prefix) => {
    const texts = await select.locator('option').allTextContents();
    const found = texts.find((text) => text.trim().startsWith(prefix));
    if (!found) throw new Error(`no option starting with "${prefix}"`);
    return found;
  };
  const choose = async (select, prefix) => h.choose(select, await optionLabel(select, prefix));
  const stored = async (id) => (await api('/resource-definitions')).resourceDefinitions.find((def) => def.key === id);
  const post = async (path, data, expected, label) => {
    const response = await page.request.post(`${baseURL}/api/v1${path}`, { data });
    expect(response.status(), label).toBe(expected);
    return response.json();
  };
  const json = (text) => JSON.parse(text);
  const typeJson = (locator, value) => h.type(locator, typeof value === 'string' ? value : JSON.stringify(value), { replace: true, delay: LONG_TYPE_DELAY });

  async function signIn(username) {
    await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), username);
    await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
    await expect(page.getByLabel('Mật khẩu', { exact: true })).toHaveAttribute('type', 'password');
    await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
    await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
  }
  async function openDefinitions() {
    await h.click(nav('Cấu hình tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-definitions`);
  }
  async function openEditor() {
    await h.click(page.getByRole('button', { name: `Sửa cấu hình ${ID.parent}`, exact: true }));
    await expect(page.getByRole('heading', { name: `Sửa cấu hình ${ID.parent}` })).toBeVisible();
  }
  // Reads every rule row of the typed provision editor back from the page.
  async function formRows() {
    const rows = [];
    for (let n = 1; await descriptorOf(n).count(); n += 1) {
      rows.push({ descriptor: await descriptorOf(n).inputValue(), params: json(await paramsOf(n).inputValue()), isDependent: await dependentOf(n).isChecked(), matchDependents: await matchOf(n).isChecked() });
    }
    return rows;
  }
  // PostgreSQL returns jsonb keys in its own order, so stored rules are compared by descriptor.
  const byDescriptor = (rows) => [...rows].sort((a, b) => a.descriptor.localeCompare(b.descriptor));
  async function rowNumber(descriptor) {
    const index = (await formRows()).findIndex((row) => row.descriptor === descriptor);
    expect(index, `rule row for ${descriptor}`).toBeGreaterThanOrEqual(0);
    return index + 1;
  }
  async function addRow(n, { descriptor, params, isDependent = false, matchDependents = false }) {
    await h.click(page.getByRole('button', { name: '+ Thêm tài nguyên đi kèm', exact: true }));
    await expect(descriptorOf(n)).toBeVisible();
    await h.type(descriptorOf(n), descriptor);
    await typeJson(paramsOf(n), params);
    if (isDependent) await h.click(dependentOf(n));
    if (matchDependents) await h.click(matchOf(n));
  }

  // The Score Preview of the Application Environment through the Console.
  async function previewScore(runId) {
    await h.click(nav('Ứng dụng'));
    await h.click(page.locator('.application-card').first());
    await h.click(page.getByRole('button', { name: 'Xem trước Score', exact: true }));
    await expect(page.getByRole('heading', { name: 'Xem trước Score', level: 1 })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'staging', exact: true })).toHaveAttribute('aria-selected', 'true');
    await h.type(page.getByLabel('ID Workload', { exact: true }), WORKLOAD);
    await h.type(page.getByLabel('ID lần chạy', { exact: true }), runId);
    await typeJson(page.getByLabel('Score sau (YAML hoặc JSON)'), score);
    const before = previews.length;
    await h.click(page.getByRole('button', { name: 'Xem trước', exact: true }));
    await expect.poll(() => previews.length, { timeout: 60_000 }).toBeGreaterThan(before);
    assertNoViolations();
    return previews[previews.length - 1];
  }
  const edgeSet = (graph) => new Set(graph.edges.map((edge) => `${edge.consumer} -> ${edge.provider} [${edge.reason}]`));
  const nodeOf = (graph, descriptor) => graph.nodes.find((node) => node.descriptor === descriptor);
  const batchIndex = (batches, descriptor) => batches.findIndex((batch) => batch.includes(descriptor));
  // Asserts the full co-provision edge matrix of one Preview: is_dependent and
  // match_dependents edges for every companion, match_dependents edges from EVERY
  // consumer of the parent (the workload and the is_dependent companions), no self
  // edge, no reverse edge, every endpoint scheduled and provider-first batches.
  function expectCoProvisionEdges(result, expected, label) {
    expect(result.status, `${label}: preview status`).toBe(200);
    const edges = edgeSet(result.graph);
    const consumers = [...new Set(result.graph.edges.filter((edge) => edge.provider === PARENT).map((edge) => edge.consumer))];
    expect(consumers, `${label}: the workload consumes the parent`).toContain(CONSUMER);
    for (const [companion, flags] of Object.entries(expected)) {
      expect(edges.has(`${companion} -> ${PARENT} [${IS_DEPENDENT}]`), `${label}: ${companion} is_dependent edge`).toBe(flags.isDependent);
      expect(edges.has(`${PARENT} -> ${companion} [${IS_DEPENDENT}]`), `${label}: ${companion} reverse edge`).toBe(false);
      expect(nodeOf(result.graph, companion).origins, `${label}: ${companion} origin`).toContain('co-provision');
      for (const consumer of consumers) {
        if (consumer === companion) {
          expect(edges.has(`${consumer} -> ${companion} [${MATCH_DEPENDENTS}]`), `${label}: self edge on ${companion}`).toBe(false);
          continue;
        }
        expect(edges.has(`${consumer} -> ${companion} [${MATCH_DEPENDENTS}]`), `${label}: match_dependents edge ${consumer} -> ${companion}`).toBe(flags.matchDependents);
      }
    }
    // The only co-provision edges are the ones derived above.
    const coEdges = result.graph.edges.filter((edge) => edge.reason === IS_DEPENDENT || edge.reason === MATCH_DEPENDENTS);
    for (const edge of coEdges) {
      expect(edge.consumer, `${label}: self edge`).not.toBe(edge.provider);
      const flags = expected[edge.reason === IS_DEPENDENT ? edge.consumer : edge.provider];
      expect(flags, `${label}: unexpected co-provision edge ${edge.consumer} -> ${edge.provider}`).toBeDefined();
    }
    // Every resource node is scheduled and every edge is provider-first.
    for (const node of result.graph.nodes.filter((item) => item.kind === 'resource')) {
      expect(batchIndex(result.batches, node.descriptor), `${label}: ${node.descriptor} is scheduled`).toBeGreaterThanOrEqual(0);
    }
    for (const edge of result.graph.edges) {
      const provider = batchIndex(result.batches, edge.provider);
      expect(provider, `${label}: provider ${edge.provider} is scheduled`).toBeGreaterThanOrEqual(0);
      const consumer = batchIndex(result.batches, edge.consumer);
      if (nodeOf(result.graph, edge.consumer).kind === 'resource') expect(consumer, `${label}: ${edge.consumer} after ${edge.provider}`).toBeGreaterThan(provider);
    }
  }

  // ---- 1. Sign in ----------------------------------------------------------
  caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ; ô mật khẩu luôn bị che');
  await h.showCursor();
  await signIn('platform-engineer');
  mark('platform-engineer-signed-in');
  await h.pause(SHORT_READ);

  // ---- 2. Labelled fixtures -------------------------------------------------
  caption('Bước 2 (chuẩn bị bằng API, không phải thao tác UI): 5 Loại tài nguyên, tài khoản Internal GIẢ LẬP, Definition namespace và PostgreSQL đi kèm. PostgreSQL và Vault thật');
  await h.moveTo(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 }));
  const field = (name, type, required = false) => ({ name, type, required });
  const redisOutputs = [field('host', 'string', true), field('port', 'number', true), field('username', 'string', true), field('password', 'string', true), field('tls', 'bool', true)];
  for (const type of [
    { key: 'workload', inputs: [], outputs: [] },
    { key: 'k8s-namespace', inputs: [], outputs: [field('name', 'string', true)] },
    { key: 'k8s-cluster', inputs: [], outputs: [field('name', 'string', true), field('endpoint', 'string'), field('kubeContext', 'string', true), field('kubeconfig', 'string')] },
    { key: 'postgres', inputs: [field('database', 'string', true), field('username', 'string', true), field('labels', 'any')], outputs: [field('host', 'string', true), field('port', 'number', true), field('database', 'string', true), field('username', 'string', true), field('password', 'string', true)] },
    { key: 'redis', inputs: [], outputs: redisOutputs },
  ]) await post('/resource-types', type, 201, `fixture type ${type.key}`);
  const internal = await post('/driver-accounts/internal', { name: names.internal, kubeconfig: kubeconfig(kubeToken), context: 'fake-lab' }, 201, 'fixture Internal account');
  expect(internal.status).toBe('READY');
  writeFileSync(`${evidenceDir}/accounts.json`, JSON.stringify({ internal: internal.key }, null, 2));
  const application = (await api('/applications')).applications.find((app) => app.key === APP);
  const staging = application.environments.find((env) => env.key === 'staging');
  expect(staging.executionProfile ?? '').toBe('');
  const connected = await page.request.put(`${baseURL}/api/v1/applications/${APP}/environments/staging/connection`, { data: { connectionKey: internal.key, expectedVersion: staging.version } });
  expect(connected.status(), 'fixture Environment connection').toBe(200);
  const modulePin = async (resourceType, module) => {
    const capability = (await api('/resource-definition-capabilities')).capabilities.find((item) => item.resourceType === resourceType && item.driverType === 'terraform' && item.module === module);
    const { sourceId, moduleId, revision, subdirectory, contentSha256 } = capability.source;
    return { source_id: sourceId, module_id: moduleId, revision, subdirectory, content_sha256: contentSha256 };
  };
  const k8sDefinition = (key, resourceType, module, pin, variables, inputs, outputs, criteria) => ({
    key, name: `Cố định ${key}`, resourceType, driverType: 'terraform', provision: {}, criteria,
    driverInputs: { values: { source: pin, variables,
      input_mapping: Object.fromEntries(inputs.map((name) => [name, { param: name }])),
      output_mapping: Object.fromEntries(outputs.map((name) => [name, name])), provider_bindings: { kubernetes: { target: 'environment' } } } },
  });
  await post('/resource-definitions', k8sDefinition(ID.ns, 'k8s-namespace', 'k8s-namespace', await modulePin('k8s-namespace', 'k8s-namespace'), { name: '${context.env.namespace}' }, [], ['name'], [{}]), 201, 'fixture namespace Definition');
  const nsOutput = "${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
  await post('/resource-definitions', k8sDefinition(ID.companion, 'postgres', 'postgres-k8s', await modulePin('postgres', 'postgres-k8s'), { namespace: nsOutput }, ['database', 'username', 'labels'], ['host', 'port', 'database', 'username', 'password'], [{ class: 'companion' }]), 201, 'fixture companion Definition');
  check('fixtures (API, not claimed as UI): Types workload, k8s-namespace, k8s-cluster, postgres (inputs database, username, labels), redis; a fake-verified READY Internal account connected to acceptance/staging; the namespace Definition and the class "companion" PostgreSQL Definition');
  mark('fixtures-ready');
  await h.pause(SHORT_READ);

  // ---- 3. Open the page and the empty provision editor -----------------------
  caption('Bước 3: Mở Nền tảng › Cấu hình tài nguyên; cấu hình mới là Redis (module redis-k8s) với class prov; tài khoản không cần vì cluster được kế thừa');
  await openDefinitions();
  await h.type(form('ID cấu hình (Definition ID)'), ID.parent);
  await h.type(form('Tên hiển thị'), names.parent);
  await choose(form('Loại tài nguyên'), 'redis');
  await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
  await choose(form('Module đã kiểm chứng'), 'redis-k8s');
  await expect(page.getByLabel('Tài khoản hạ tầng')).toHaveCount(0);
  await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
  await h.click(page.getByRole('button', { name: 'Dùng preset cho namespace', exact: true }));
  await choose(form('Cluster đích của provider kubernetes'), 'Cluster của Môi trường');
  await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
  await h.type(form('Điều kiện 1 Class'), 'prov', { replace: true });
  mark('parent-basics');

  caption('Bước 4: Mục “Tài nguyên đi kèm” giải thích: consumer là bên dùng, provider là bên được dùng, cạnh luôn đi consumer → provider; biểu mẫu không tự Xem trước hay triển khai');
  const editor = page.getByRole('region', { name: 'Tài nguyên đi kèm' });
  await expect(editor).toContainText('Reference luôn đi theo chiều consumer → provider');
  await expect(editor).toContainText('Chưa có tài nguyên đi kèm.');
  await h.moveTo(editor);
  await h.read('Mỗi quy tắc: descriptor, params JSON object và hai cờ is_dependent, match_dependents');
  check('UI explains consumer/provider and the fixed consumer -> provider direction; the editor starts with no rule and never previews or provisions by itself');

  // ---- 5. Four rules, one per flag combination -------------------------------
  caption('Bước 5: Thêm 4 tài nguyên đi kèm PostgreSQL: chỉ is_dependent, chỉ match_dependents, cả hai cờ, và không cờ; params là JSON lồng nhau có kiểu (labels là object)');
  const labels = (team) => ({ tier: 'db', team });
  const rules = [
    { descriptor: D, params: { database: 'dep', username: 'app', labels: labels('dep') }, isDependent: true },
    { descriptor: M, params: { database: 'match', username: 'app', labels: labels('match') }, matchDependents: true },
    { descriptor: B, params: { database: 'both', username: 'app', labels: labels('both') }, isDependent: true, matchDependents: true },
    { descriptor: N, params: { database: 'plain', username: 'app' } },
  ];
  for (const [index, rule] of rules.entries()) await addRow(index + 1, rule);
  await expect(page.getByText(/Đảo vai trò: tài nguyên đi kèm \(consumer\) phụ thuộc vào tài nguyên cha \(provider\)/).first()).toBeVisible();
  await expect(page.getByText(/Nối mọi consumer đang dùng tài nguyên cha tới tài nguyên đi kèm \(provider\)/).first()).toBeVisible();
  const expectedCreate = {
    [D]: { is_dependent: true, params: rules[0].params },
    [M]: { match_dependents: true, params: rules[1].params },
    [B]: { is_dependent: true, match_dependents: true, params: rules[2].params },
    [N]: { params: rules[3].params },
  };
  expect(await formRows()).toEqual(rules.map((rule) => ({ descriptor: rule.descriptor, params: rule.params, isDependent: !!rule.isDependent, matchDependents: !!rule.matchDependents })));
  check('form: four rules with their flags, descriptors and nested params; the is_dependent and match_dependents explanations are shown for every rule');
  mark('rules-added');
  await h.read('Giải thích hai cờ hiển thị ngay dưới mỗi quy tắc');

  // ---- 6. Advanced JSON round-trip ------------------------------------------
  caption('Bước 6: Chuyển sang JSON nâng cao: JSON giống hệt biểu mẫu (cờ false không được thêm); sửa JSON thành cờ false và params rỗng TƯỜNG MINH rồi quay lại biểu mẫu');
  await h.click(modeJson());
  expect(json(await jsonBox().inputValue())).toEqual(expectedCreate);
  const explicit = { ...expectedCreate, [N]: { is_dependent: false, match_dependents: false, params: {} } };
  await typeJson(jsonBox(), JSON.stringify(explicit));
  await h.click(modeForm());
  const rowsAfterJson = await formRows();
  expect(rowsAfterJson.find((row) => row.descriptor === N)).toEqual({ descriptor: N, params: {}, isDependent: false, matchDependents: false });
  expect(rowsAfterJson.find((row) => row.descriptor === B)).toMatchObject({ isDependent: true, matchDependents: true, params: rules[2].params });
  await h.click(modeJson());
  expect(json(await jsonBox().inputValue())).toEqual(explicit);
  check('advanced JSON <-> form round trip is lossless: nested params, both flags, and the explicit false / empty params of a hand-written rule survive both switches (draft level)');
  mark('json-roundtrip');
  await h.read('Cờ false và params rỗng viết tường minh được giữ qua hai lần chuyển');
  await h.click(modeForm());

  // ---- 7. Client-side policy ---------------------------------------------------
  caption('Bước 7: Biểu mẫu chặn trước khi gửi: descriptor trùng và params không phải JSON object bị từ chối; bản nháp được giữ');
  const requestsBefore = writes.length;
  await h.type(descriptorOf(4), D, { replace: true });
  await h.click(submitCreate());
  await expect(alertBox()).toContainText(`Descriptor "${D}" bị trùng`);
  await h.type(descriptorOf(4), N, { replace: true });
  await typeJson(paramsOf(4), '[1]');
  await h.click(submitCreate());
  await expect(alertBox()).toContainText(`Params của "${N}"`);
  await typeJson(paramsOf(4), rules[3].params);
  expect(writes.length).toBe(requestsBefore);
  check('client guidance: duplicate descriptor and a params array are refused before any request; nothing was sent');
  mark('client-policy');
  await h.read('Descriptor trùng và params sai kiểu bị chặn trước khi gửi');

  // ---- 8. Server policy for descriptors ------------------------------------------
  caption('Bước 8: Máy chủ vẫn kiểm tra lại: thêm một quy tắc có Loại tài nguyên chưa đăng ký; máy chủ trả 400, bản nháp được giữ và không lưu gì');
  await addRow(5, { descriptor: 'nope.default#shared.x', params: {} });
  const refused = await sent(() => h.click(submitCreate()));
  expect(refused.status).toBe(400);
  await expect(alertBox()).toContainText('Máy chủ từ chối cấu hình');
  expect(await stored(ID.parent)).toBeUndefined();
  await expect(form('ID cấu hình (Definition ID)')).toHaveValue(ID.parent);
  expect((await formRows()).length).toBe(5);
  check('server policy: an unregistered companion Type is refused with HTTP 400, nothing is stored and the whole draft is kept');
  mark('server-refusal');
  await h.read('Loại tài nguyên đi kèm chưa đăng ký bị máy chủ từ chối');
  await h.click(page.getByRole('button', { name: 'Xóa tài nguyên đi kèm 5', exact: true }));
  await expect(descriptorOf(5)).toHaveCount(0);

  // ---- 9. Create ----------------------------------------------------------------
  caption('Bước 9: Đăng ký cấu hình: một POST, phiên bản 1; máy chủ lưu descriptor, params lồng nhau và cờ true; cờ false là mặc định nên không được lưu (rule không cờ vẫn có params)');
  const created = await sent(() => h.click(submitCreate()));
  expect(created).toMatchObject({ method: 'POST', status: 201, key: ID.parent, version: 1 });
  await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${ID.parent}.` })).toBeVisible();
  const createdRecord = await stored(ID.parent);
  expect(createdRecord).toMatchObject({ name: names.parent, version: 1, resourceType: 'redis' });
  const persistedCreate = { ...expectedCreate, [N]: { params: rules[3].params } };
  expect(createdRecord.provision).toEqual(persistedCreate);
  expect(createdRecord).not.toHaveProperty('driverAccount');
  check('create: HTTP 201, version 1; stored provision = four descriptors with nested params; the submitted plain rule (explicit false flags from step 6, params database/username) is stored with its params and without false flags; default omission of empty params is proven by the HTTP regression TestV02ProvisionRegistrationPolicyHTTP');
  mark('created');
  await h.read('Đã đăng ký phiên bản 1 với bốn tài nguyên đi kèm');

  // ---- 10. Reload ----------------------------------------------------------------
  caption('Bước 10: Tải lại trang; biểu mẫu sửa đọc lại đúng từ PostgreSQL: descriptor, params lồng nhau và từng cờ');
  await page.reload();
  await expect(entry(ID.parent)).toContainText(names.parent);
  await expect(entry(ID.parent)).toContainText('phiên bản 1');
  await openEditor();
  expect(byDescriptor(await formRows())).toEqual(byDescriptor(rules.map((rule) => ({ descriptor: rule.descriptor, params: rule.params, isDependent: !!rule.isDependent, matchDependents: !!rule.matchDependents }))));
  check('reload: the edit form shows the four rules, nested params and each flag exactly as stored in PostgreSQL');
  mark('reloaded');
  await h.read('Cờ và params đọc lại đúng');

  // ---- 11. Preview #1 --------------------------------------------------------------
  caption('Bước 11: Xem trước Score (chỉ lập kế hoạch, không triển khai): Workload dùng Redis class prov; kế hoạch chứa 4 tài nguyên đi kèm với đúng cạnh consumer → provider');
  const first = await previewScore(`provision-${suffix}-1`);
  expectCoProvisionEdges(first, {
    [D]: { isDependent: true, matchDependents: false }, [M]: { isDependent: false, matchDependents: true },
    [B]: { isDependent: true, matchDependents: true }, [N]: { isDependent: false, matchDependents: false },
  }, 'preview 1');
  for (const companion of [D, M, B]) expect(nodeOf(first.graph, companion).paramKeys).toEqual(['database', 'labels', 'username']);
  expect(nodeOf(first.graph, N).paramKeys).toEqual(['database', 'username']);
  await expect(page.getByText(`${D} → ${PARENT}`)).toBeVisible();
  await expect(page.getByText(`${CONSUMER} → ${M}`)).toBeVisible();
  await expect(page.getByText(`${CONSUMER} → ${B}`)).toBeVisible();
  await h.moveTo(page.getByRole('heading', { name: 'Đồ thị tài nguyên' }));
  check('preview 1 (planner, not matching): dep-only has companion -> parent only; match-only has workload -> companion only; both has both; plain has neither; parents are scheduled first; params keys are database/labels/username');
  mark('preview-1');
  await h.read('Cạnh luôn đi từ bên dùng sang bên được dùng; bên cung cấp được tạo trước');
  await h.moveTo(page.getByRole('heading', { name: 'Thứ tự tạo tài nguyên' }));
  await h.pause(SHORT_READ);

  // ---- 12. Edit the flags (CAS) ------------------------------------------------------
  caption('Bước 12: Sửa lại: đổi cờ của hai quy tắc đầu (is_dependent ↔ match_dependents), đổi một tham số lồng nhau và xóa quy tắc không cờ; Lưu là một PUT theo phiên bản');
  await h.click(nav('Cấu hình tài nguyên'));
  await openEditor();
  const rowD = await rowNumber(D);
  const rowM = await rowNumber(M);
  const rowN = await rowNumber(N);
  await h.click(dependentOf(rowD));
  await h.click(matchOf(rowD));
  await h.click(dependentOf(rowM));
  await h.click(matchOf(rowM));
  const swappedLabels = labels('dep-đổi');
  await typeJson(paramsOf(rowD), { database: 'dep', username: 'app', labels: swappedLabels });
  await h.click(page.getByRole('button', { name: `Xóa tài nguyên đi kèm ${rowN}`, exact: true }));
  const edited = await sent(() => h.click(submitSave()));
  expect(edited).toMatchObject({ method: 'PUT', status: 200, key: ID.parent, version: 2 });
  await expect(page.getByRole('status').filter({ hasText: `Đã lưu cấu hình ${ID.parent} (phiên bản 2)` })).toBeVisible();
  const expectedV2 = {
    [D]: { match_dependents: true, params: { database: 'dep', username: 'app', labels: swappedLabels } },
    [M]: { is_dependent: true, params: rules[1].params },
    [B]: expectedCreate[B],
  };
  const editedRecord = await stored(ID.parent);
  expect(editedRecord).toMatchObject({ version: 2, name: names.parent, resourceType: 'redis' });
  expect(editedRecord.provision).toEqual(expectedV2);
  // Unrelated configuration is untouched by a provision edit.
  for (const part of ['driverInputs', 'criteria', 'driverType', 'key']) expect(editedRecord[part]).toEqual(createdRecord[part]);
  check('edit: HTTP 200, version 1 -> 2 once; flags swapped, one nested param changed, the plain rule removed; driverInputs, criteria, driver and ID are byte-for-byte unchanged');
  mark('edited');
  await h.read('Phiên bản tăng đúng một lần; cấu hình khác không đổi');

  // ---- 13. Reload + Preview #2 ----------------------------------------------------------
  caption('Bước 13: Tải lại, rồi Xem trước Score lần nữa: kế hoạch theo cấu hình MỚI — hai cờ đổi chỗ, tài nguyên không cờ biến mất, tài nguyên cha vẫn được tạo trước');
  await page.reload();
  await expect(entry(ID.parent)).toContainText('phiên bản 2');
  const second = await previewScore(`provision-${suffix}-2`);
  expectCoProvisionEdges(second, {
    [D]: { isDependent: false, matchDependents: true }, [M]: { isDependent: true, matchDependents: false }, [B]: { isDependent: true, matchDependents: true },
  }, 'preview 2');
  expect(nodeOf(second.graph, N)).toBeUndefined();
  check('preview 2 follows version 2: dep-only/match-only swapped their edges, the removed plain rule is gone from the graph, the both-flag rule keeps both edges');
  mark('preview-2');
  await h.read('Kế hoạch đọc cấu hình đã lưu, không phải bản nháp trong trình duyệt');

  // ---- 14. Restart ---------------------------------------------------------------------------
  caption('Bước 14: Khởi động lại máy chủ sản phẩm (PostgreSQL và Vault thật vẫn chạy); quy tắc phiên bản 2 vẫn còn và kế hoạch giống hệt');
  if (process.env.ORCH_E2E_RESTART_CMD) {
    const { execFileSync } = await import('node:child_process');
    execFileSync(process.env.ORCH_E2E_RESTART_CMD, [], { stdio: 'ignore' });
    mark('server-restarted');
    check('product server process restarted against the same disposable PostgreSQL and Vault');
  } else {
    await fail('ORCH_E2E_RESTART_CMD is required for the restart phase');
  }
  await page.reload();
  if (await page.getByLabel('Tên đăng nhập', { exact: true }).isVisible().catch(() => false)) {
    caption('Phiên đăng nhập hết hạn sau khởi động lại; đăng nhập lại bằng giao diện');
    await signIn('platform-engineer');
  }
  const afterRestart = await stored(ID.parent);
  expect(afterRestart).toMatchObject({ version: 2, name: names.parent });
  expect(afterRestart.provision).toEqual(expectedV2);
  const third = await previewScore(`provision-${suffix}-3`);
  expectCoProvisionEdges(third, {
    [D]: { isDependent: false, matchDependents: true }, [M]: { isDependent: true, matchDependents: false }, [B]: { isDependent: true, matchDependents: true },
  }, 'preview 3');
  expect([...edgeSet(third.graph)].sort()).toEqual([...edgeSet(second.graph)].sort());
  check('after restart: version 2 provision is intact in PostgreSQL and the planner produces the same edge set as before the restart');
  mark('restart-data-intact');
  await h.read('Dữ liệu và kế hoạch còn nguyên sau khi khởi động lại');

  // ---- 15. Cycle (supported planner policy) ------------------------------------------------------
  caption('Bước 15: Hai tài nguyên đi kèm cùng bật CẢ HAI cờ sẽ là consumer của nhau; Lưu được, nhưng Xem trước bị planner từ chối ở giai đoạn lập lịch (chu trình), không triển khai gì');
  await h.click(nav('Cấu hình tài nguyên'));
  await openEditor();
  const rowOfD = await rowNumber(D);
  await h.click(dependentOf(rowOfD));
  const cyclic = await sent(() => h.click(submitSave()));
  expect(cyclic).toMatchObject({ method: 'PUT', status: 200, version: 3 });
  const cycleResult = await previewScore(`provision-${suffix}-4`);
  expect(cycleResult).toMatchObject({ status: 400, code: 'PLANNING_REJECTED', stage: 'schedule' });
  await expect(alertBox()).toContainText('Không thể xem trước');
  check('cycle: two both-flag companions are stored (version 3) but the planner rejects the Preview with HTTP 400 PLANNING_REJECTED at stage schedule; nothing is deployed');
  mark('cycle-rejected');
  await h.read('Planner từ chối chu trình ở giai đoạn lập lịch');

  caption('Bước 16: Sửa lại để bỏ chu trình (phiên bản 4); cấu hình cuối cùng giống phiên bản 2 và Xem trước hoạt động lại');
  await h.click(nav('Cấu hình tài nguyên'));
  await openEditor();
  await h.click(dependentOf(await rowNumber(D)));
  const fixed = await sent(() => h.click(submitSave()));
  expect(fixed).toMatchObject({ method: 'PUT', status: 200, version: 4 });
  expect((await stored(ID.parent)).provision).toEqual(expectedV2);
  const fourth = await previewScore(`provision-${suffix}-5`);
  expectCoProvisionEdges(fourth, {
    [D]: { isDependent: false, matchDependents: true }, [M]: { isDependent: true, matchDependents: false }, [B]: { isDependent: true, matchDependents: true },
  }, 'preview 5');
  check('fix: version 3 -> 4 restores the version-2 provision and the Preview plans again');
  mark('cycle-fixed');
  await h.read('Planner chạy lại sau khi bỏ chu trình');

  // ---- 17. Finish -----------------------------------------------------------------------------------------
  caption('Bước 17: Đăng xuất. Đây là bằng chứng LẬP KẾ HOẠCH với PostgreSQL và Vault thật; xác minh Kubernetes là GIẢ LẬP và chưa có tài nguyên đi kèm nào được tạo');
  await assertNoSecretLeaks(page, allSecrets);
  await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
  await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();
  mark('signed-out');
  await h.pause(3000);

  assertNoViolations();
  check(`global: ${writes.length} Definition writes, ${previews.length} Score Previews and every /api/v1 reply were free of key material and secret references`);
  writeFileSync(`${evidenceDir}/definition-writes.json`, JSON.stringify({ fake: 'Kubernetes verification is a deterministic fake; Preview only plans, nothing is deployed', writes }, null, 2));
  writeFileSync(`${evidenceDir}/previews.json`, JSON.stringify({ planningOnly: true, previews: previews.map((p) => ({ status: p.status, code: p.code, stage: p.stage, edges: p.graph ? [...edgeSet(p.graph)].sort() : undefined, batches: p.batches })) }, null, 2));
  if (process.env.ORCH_V02_INJECT_FAILURE === 'after-success') await fail('injected failure after success');
}
