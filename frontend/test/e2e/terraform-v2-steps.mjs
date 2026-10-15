// UI building blocks of the real Terraform v2 acceptance scenarios (I04-10).
// Every business step here is a Web Console action; API reads only assert
// results. The labels are the Vietnamese strings of the Console and are
// checked against frontend/src by terraform-v2-labels.test.mjs.
import { expect } from '@playwright/test';
import { setEnvironmentConnections } from './human.mjs';

export const SHORT_READ = 2000;
export const FIXTURE_PASSWORD = 'test-password'; // fixed synthetic fixture account of the isolated run, typed into a masked field

// Resource Types the catalog-free run registers through the Console. The
// reserved virtual types (environment, service) are not among them.
export const TYPES = [
  { key: 'workload', inputs: [], outputs: [] },
  { key: 'k8s-cluster', inputs: [], outputs: [['name', 'string', true, false], ['endpoint', 'string', false, false], ['kubeContext', 'string', true, false], ['kubeconfig', 'string', false, false]] },
  { key: 'k8s-namespace', inputs: [], outputs: [['name', 'string', true, false]] },
  { key: 'postgres', inputs: [['database', 'string', true], ['username', 'string', true]], outputs: [['host', 'string', true, false], ['port', 'number', true, false], ['database', 'string', true, false], ['username', 'string', true, false], ['password', 'string', true, true]] },
  { key: 'redis', inputs: [], outputs: [['host', 'string', true, false], ['port', 'number', true, false], ['username', 'string', true, false], ['password', 'string', true, true], ['tls', 'bool', true, false]] },
];

export const ACCEPTANCE = { database: 'acceptance', username: 'app', dbId: 'acceptance-db', cacheId: 'acceptance-cache', alias: { db: 'db', cache: 'cache' } };

// Shared with the post-restart verifier so its expected catalog cannot drift
// from the definitions authored through the UI.
export const K8S_ACCEPTANCE_DEFINITIONS = [
  { id: 'namespace-k8s-module', type: 'k8s-namespace', module: 'k8s-namespace', scope: 'Cluster nội bộ', presetName: 'name' },
  { id: 'pg-in-cluster', type: 'postgres', module: 'postgres-k8s', scope: 'Cluster nội bộ', input: true, presetNamespace: true, klass: 'in-cluster' },
  { id: 'redis-in-cluster', type: 'redis', module: 'redis-k8s', scope: 'Cluster nội bộ', presetNamespace: true, klass: 'in-cluster' },
];

// The shared catalog stores JSON-encoded typed values for planner tests. The
// form accepts the underlying scalar as text, not its JSON-quoted encoding.
export function decodeCatalogOverride(encoded, substitutions = {}) {
  const expanded = Object.entries(substitutions).reduce((value, [key, replacement]) => value.replaceAll(`@${key}@`, replacement), encoded);
  return JSON.parse(expanded);
}

export function formOverrideText(value) {
  return typeof value === 'string' ? value : JSON.stringify(value);
}

// The Redis Definition carries one fixture annotation that references the
// shared PostgreSQL host (a non-secret output). It makes the graph
// namespace <- PostgreSQL <- Redis, so the deletion step has BOTH a dependency
// to select and a candidate to leave unchecked; it is a test graph, not a
// product requirement.
export const REDIS_ANNOTATION = `{"example.test/postgres-host": "\${resources['postgres.in-cluster#shared.${ACCEPTANCE.dbId}'].outputs.host}"}`;

export function steps({ page, h, caption, phase, rec, baseURL, input }) {
  const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
  const get = async (path) => (await (await page.request.get(`${baseURL}/api/v1${path}`)).json());
  const wrappedSelects = new Set(['Loại tài nguyên', 'Module đã kiểm chứng', 'Phạm vi triển khai', 'Bundle dựng ứng dụng']);
  const form = (name) => (wrappedSelects.has(name)
    ? page.locator(`select[aria-label="${name}"], label:has-text("${name}") > select`).first()
    : page.getByLabel(name, { exact: true }));
  const alert = () => page.getByRole('alert').first();
  const submitDefinition = () => page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
  async function optionLabel(select, prefix) {
    const texts = await select.locator('option').allTextContents();
    const found = texts.find((text) => text.trim().startsWith(prefix));
    if (!found) throw new Error(`no option starting with "${prefix}"`);
    return found;
  }
  const choose = async (select, prefix) => h.choose(select, await optionLabel(select, prefix));
  const registered = (id) => expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${id}.` })).toBeVisible();

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
  async function override(name, value) {
    await h.click(form(`Ghi đè ${name}`));
    await h.paste(form(`Giá trị ${name}`), formOverrideText(value));
  }
  async function startDefinition(id, type, module, scope) {
    await h.type(form('ID cấu hình'), id);
    await choose(form('Loại tài nguyên'), type);
    await expect(form('Loại tài nguyên')).toHaveValue(type);
    if (module) {
      const pins = form('Module đã kiểm chứng');
      if (await pins.isEnabled()) await choose(pins, module);
      await expect(pins).toContainText(module);
    }
    if (scope) {
      const select = form('Phạm vi triển khai');
      // A module that only supports one scope pre-selects it and disables the control.
      if (await select.isEnabled()) await choose(select, scope);
      else await expect(select).toContainText(scope);
    }
  }

  // ---- Platform Engineer: Connection, Types, Definitions, template ---------
  async function registerKubernetesConnection() {
    caption('Kỹ sư nền tảng mở trang Kết nối và tải lên kubeconfig một-context của cluster k8s-4f; Console kiểm tra và lưu vào Vault với phạm vi riêng, không dùng thông tin xác thực của máy chủ');
    await h.click(nav('Kết nối'));
    await expect(page.getByRole('heading', { name: 'Kết nối', level: 1 })).toBeVisible();
    await h.type(page.getByLabel('Tên kết nối'), input.connection.name);
    await h.click(page.getByLabel('Tải lên file kubeconfig'));
    await page.locator('input[type="file"]').setInputFiles(input.connection.kubeconfigFile);
    await expect(page.getByLabel('Đích đã chọn')).toContainText(`Context ${input.kube.context}`);
    await h.pause(SHORT_READ);
    const registeredResponse = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 120_000 });
    await h.click(page.getByRole('button', { name: 'Kiểm tra và lưu' }));
    const response = await registeredResponse;
    if (response.status() >= 300) throw new Error(`Connection registration was refused (HTTP ${response.status()})`);
    const created = await response.json();
    const listed = (await get('/connections')).connections.find((c) => c.key === input.connection.name);
    const ok = created.verification?.verified === true && listed?.status === 'READY' && listed.kind === 'KUBERNETES' && listed.authenticationType === 'KUBECONFIG';
    rec.record('ui.connections.registered', 'PREPARE', 'ui', ok, `Connection ${input.connection.name} ${listed?.status ?? 'missing'} verified=${created.verification?.verified}`);
    await h.read('Kết nối Kubernetes đã được kiểm tra và đang sẵn sàng');
  }

  async function registerType(type) {
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

  async function registerTypes(wanted = TYPES) {
    caption('Danh mục bắt đầu trống: Kỹ sư nền tảng đăng ký từng loại tài nguyên bằng form — cluster, namespace, PostgreSQL, Redis và workload; mật khẩu được đánh dấu bí mật');
    await h.click(nav('Loại tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Loại tài nguyên', level: 1, exact: true })).toBeVisible();
    expect((await get('/resource-types')).resourceTypes).toHaveLength(0);
    for (const type of wanted) await registerType(type);
    const types = (await get('/resource-types')).resourceTypes;
    const redis = types.find((t) => t.key === 'redis');
    const pg = types.find((t) => t.key === 'postgres');
    const ok = types.length === wanted.length && redis?.outputs.find((o) => o.name === 'password')?.secret === true && pg?.outputs.find((o) => o.name === 'password')?.secret === true;
    rec.record('ui.types.registered', 'FORM', 'ui', ok, `${types.length} types, password outputs secret`);
    await h.read('Đã đăng ký năm loại tài nguyên');
  }

  /** @param {{id:string,type:string,module:string,scope:string,klass?:string,input?:boolean,overrides?:[string,string][],presetNamespace?:boolean,clusterTarget?:boolean}} d */
  async function registerDefinition(d) {
    await startDefinition(d.id, d.type, d.module, d.scope);
    if (d.input) await h.click(page.getByRole('button', { name: 'Ghép đầu vào theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    if (d.clusterTarget !== false) await choose(form('Cluster đích của provider kubernetes'), 'Cluster của Môi trường');
    if (d.presetName) await h.click(page.getByRole('button', { name: `Dùng preset cho ${d.presetName}`, exact: true }));
    if (d.presetNamespace) await h.click(page.getByRole('button', { name: 'Dùng preset cho namespace', exact: true }));
    for (const [name, value] of d.overrides ?? []) await override(name, value);
    if (d.klass) {
      await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
      await h.type(form('Điều kiện 1 Class'), d.klass);
    }
    await h.pause(SHORT_READ);
    await h.click(submitDefinition());
    await registered(d.id);
  }

  async function registerDefinitions({ redisOverrides = [] } = {}) {
    caption('Cấu hình tài nguyên: chỉ chọn module đã được registry xác minh; pin source, revision và SHA-256 do máy chủ công bố, không có ô nhập URL hay digest');
    await h.click(nav('Cấu hình tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
    await expect(page.getByLabel(/URL|digest|revision|source_id/i)).toHaveCount(0);
    for (const definition of K8S_ACCEPTANCE_DEFINITIONS) {
      await registerDefinition({ ...definition, ...(definition.id === 'redis-in-cluster' ? { overrides: redisOverrides } : {}) });
    }
    const defs = (await get('/resource-definitions')).resourceDefinitions.map((d) => d.key).sort();
    const capabilities = (await get('/resource-definition-capabilities')).capabilities;
    const pinned = ['k8s-namespace', 'postgres-k8s', 'redis-k8s'].every((m) => capabilities.some((c) => c.driverType === 'terraform' && c.module === m && c.source?.contentSha256));
    return { defs, pinned, capabilities };
  }

  async function registerRenderTemplate(scope = 'Cluster nội bộ', id = 'render-internal') {
    caption('Kỹ sư nền tảng đăng ký đúng một Mẫu dựng ứng dụng score-k8s cho cluster nội bộ; bundle lấy từ danh sách đã cài, không có trình dựng thay thế');
    await h.click(nav('Mẫu dựng ứng dụng'));
    await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
    await h.type(form('ID mẫu'), id);
    await choose(form('Phạm vi triển khai'), scope);
    await expect(form('Bundle dựng ứng dụng')).toHaveValue('score-k8s-internal-v1');
    await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }));
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký mẫu dựng ứng dụng ${id}.` })).toBeVisible();
  }

  // ---- Developer: application, variables, workloads -----------------------
  async function openApplication() {
    await h.click(page.getByRole('button', { name: new RegExp(input.applicationName) }).first());
    await expect(page.getByRole('heading', { name: input.applicationName })).toBeVisible();
  }

  // Walks back with the "←" buttons until the Application page is shown.
  async function backToApplication() {
    const heading = page.getByRole('heading', { name: input.applicationName });
    for (let i = 0; i < 4 && !(await heading.isVisible()); i += 1) await h.click(page.getByRole('button', { name: /^←/ }).first());
    await expect(heading).toBeVisible();
  }

  async function bindEnvironment(connectionKey = input.connection.name, profile = 'internal-k8s') {
    caption('Chọn Ứng dụng và Môi trường staging, rồi gán Kết nối trong Cài đặt môi trường; hồ sơ thực thi lấy từ loại Kết nối');
    await openApplication();
    await setEnvironmentConnections(h, input.applicationName, { staging: connectionKey });
    const env = (await get(`/applications/${input.applicationKey}`)).application?.environments?.find((e) => e.key === 'staging');
    const ok = env?.connectionKey === connectionKey && env.executionProfile === profile;
    rec.record('ui.environment.bound', 'FORM', 'ui', ok, `staging -> ${env?.connectionKey ?? 'unbound'} ${env?.executionProfile ?? ''}`);
    return env;
  }

  async function putVariables(entries) {
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await h.pause(1000);
    for (const [name, value] of entries) {
      const section = page.locator('section[aria-label="Biến"]');
      await h.click(section.getByRole('button', { name: '+ Thêm biến' }));
      await h.type(page.getByLabel('Tên key'), name);
      await h.type(page.getByLabel('Giá trị'), value);
      const saved = page.waitForResponse((r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/configuration/keys/${encodeURIComponent(name)}`));
      await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
      const response = await saved;
      if (!response.ok()) throw new Error(`saving variable ${name} returned HTTP ${response.status()}`);
      await expect(page.getByLabel('Tên key')).toHaveCount(0);
      await h.pause(600);
    }
    await h.click(page.getByRole('button', { name: `← ${input.applicationName}` }));
    await h.pause(1000);
  }

  async function addResource(index, r) {
    await h.click(page.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }));
    await h.type(page.getByLabel('Alias tài nguyên', { exact: true }).nth(index), r.alias);
    await choose(page.locator('select[aria-label="Loại tài nguyên"]').nth(index), r.type);
    const klass = r.klass ?? 'in-cluster';
    await choose(form(`Cách cung cấp (class) của ${r.alias}`), klass === 'cloud-managed' ? 'Dịch vụ managed AWS' : 'Trong cluster');
    await expect(form(`Cách cung cấp (class) của ${r.alias}`)).toHaveValue(klass);
    await h.type(form(`ID dùng chung của ${r.alias}`), r.sharedId);
    for (const [name, value] of Object.entries(r.params ?? {})) await h.type(form(`Tham số ${name}`), value);
  }

  /** @param {{name:string,image:string,resources?:object[],variables?:string[],outputs?:[string,string,string][],service?:{workload:string,port:string,name:string}|null,port?:{name:string,port:string}|null}} spec */
  async function addWorkload(spec) {
    await h.click(page.getByRole('button', { name: '+ Thêm Workload', exact: true }));
    await expect(page.getByRole('button', { name: 'Nhập bằng form' })).toBeVisible();
    await h.pause(800);
    await h.type(form('Tên Workload'), spec.name);
    await h.type(form('Ảnh Container'), spec.image);
    for (const [index, r] of (spec.resources ?? []).entries()) await addResource(index, r);
    const group = page.getByRole('group', { name: 'Biến của ứng dụng cho main', exact: true });
    for (const key of spec.variables ?? []) {
      const box = group.getByRole('checkbox', { name: key, exact: true });
      await h.click(box);
      await expect(box).toBeChecked();
    }
    const secretGroup = page.getByRole('group', { name: 'Bí mật của ứng dụng cho main', exact: true });
    for (const key of spec.secrets ?? []) {
      const box = secretGroup.getByRole('checkbox', { name: key, exact: true });
      await h.click(box);
      await expect(box).toBeChecked();
    }
    const rows = page.locator('.binding-row').filter({ has: page.getByLabel('Tên biến Container', { exact: true }) });
    let row = 0;
    for (const [containerName, alias, output] of spec.outputs ?? []) {
      await h.click(page.getByRole('button', { name: '+ Thêm nguồn khác' }));
      const r = rows.nth(row++);
      await h.type(r.getByLabel('Tên biến Container', { exact: true }), containerName);
      await h.choose(r.getByLabel('Tài nguyên được tham chiếu', { exact: true }), alias);
      await h.choose(r.getByLabel('Output của tài nguyên', { exact: true }), output.endsWith(' · secret') ? output.replace(/ · secret$/, ' · bí mật') : output);
    }
    if (spec.service) {
      await h.click(page.getByRole('button', { name: '+ Thêm nguồn khác' }));
      const r = rows.nth(row++);
      await h.type(r.getByLabel('Tên biến Container', { exact: true }), spec.service.name);
      await h.choose(r.getByLabel('Nguồn tham chiếu', { exact: true }), 'Service của Workload');
      await h.choose(r.getByLabel('Workload cung cấp Service', { exact: true }), spec.service.workload);
      await h.choose(r.getByLabel('Cổng của Service', { exact: true }), spec.service.port);
    }
    if (spec.port) {
      await h.click(page.getByRole('button', { name: '+ Thêm cổng' }));
      await h.type(page.getByLabel('Tên cổng Service', { exact: true }), spec.port.name);
      await h.type(page.getByLabel('Cổng Service', { exact: true }), spec.port.port);
    }
    await h.pause(1000);
    const saved = page.waitForResponse((r) => r.request().method() === 'PUT' && new URL(r.url()).pathname.endsWith(`/workloads/${encodeURIComponent(spec.name)}`));
    await h.click(page.getByRole('button', { name: 'Lưu draft Workload' }));
    const response = await saved;
    if (!response.ok()) throw new Error(`saving workload ${spec.name} returned HTTP ${response.status()}`);
    await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
    await h.pause(800);
  }

  /**
   * Platform Engineer: registers the EXISTING configuration Vault as a Secret
   * Store through "Kho bí mật". The scoped token is pasted into the masked
   * field and never typed visibly, echoed, or kept in the form.
   */
  async function registerSecretStore(store, token) {
    caption('Kỹ sư nền tảng vào "Kho bí mật" và đăng ký Vault cấu hình hiện có: địa chỉ backend, địa chỉ workload và token có phạm vi (dán vào ô mật khẩu, luôn bị che)');
    await h.click(nav('Kho bí mật'));
    await expect(page.getByRole('heading', { name: 'Kho bí mật', level: 1 })).toBeVisible();
    await h.pause(SHORT_READ);
    await h.type(page.getByLabel('Tên kho', { exact: true }), store.name);
    await h.type(page.getByRole('textbox', { name: 'Địa chỉ backend' }), store.backendAddress);
    await h.type(page.getByLabel('Địa chỉ workload', { exact: true }), store.workloadAddress);
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao' }));
    await h.type(page.getByLabel('Mount KV v2', { exact: true }), store.mount, { replace: true });
    await h.type(page.getByLabel('Mount xác thực Kubernetes', { exact: true }), store.authMount, { replace: true });
    await h.paste(page.getByLabel('Token Vault', { exact: true }), token);
    await expect(page.getByLabel('Token Vault', { exact: true })).toHaveAttribute('type', 'password');
    expect(await page.locator('body').innerText(), 'the Vault token must not be visible').not.toContain(token);
    const registered = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/secret-stores', { timeout: 120_000 });
    await h.click(page.getByRole('button', { name: 'Xác minh và đăng ký' }));
    const response = await registered;
    const text = await response.text();
    if (text.includes(token)) throw new Error('the registration response echoed the Vault token');
    if (response.status() !== 201) {
      let diagnostic = `HTTP ${response.status()}`;
      try {
        const body = JSON.parse(text);
        if (typeof body.code === 'string' && /^[A-Z][A-Z0-9_]{0,63}$/.test(body.code)) diagnostic += ` code=${body.code}`;
        if (typeof body.field === 'string' && /^[A-Za-z][A-Za-z0-9_.-]{0,63}$/.test(body.field)) diagnostic += ` field=${body.field}`;
      } catch { /* retain only the status when the response is not the fixed public error shape */ }
      throw new Error(`registering the secret store was refused (${diagnostic})`);
    }
    const body = JSON.parse(text);
    const key = body.key ?? body.secretStore?.key;
    if (!key) throw new Error('the registered secret store has no key');
    await expect(page.getByRole('status')).toContainText(`"${store.name}" đã sẵn sàng`);
    const row = page.locator('.settings-table-row').filter({ hasText: store.name });
    await expect(row).toContainText('Sẵn sàng');
    await h.moveTo(row);
    await h.pause(SHORT_READ);
    expect(await page.locator('body').innerText(), 'the Vault token must not be visible after registration').not.toContain(token);
    expect(await page.getByLabel('Token Vault', { exact: true }).inputValue(), 'the token field is cleared').toBe('');
    rec.record('ui.secret-store.ready', 'FORM', 'ui', true, `secret store ${key} READY; token masked and never echoed`);
    return key;
  }

  /**
   * Platform Engineer: registers the AWS account Connection through the Console
   * (V2-A02/A03). The scoped key is typed into the two masked fields only; the
   * Console verifies identity, capabilities and the allowed region, then stores
   * the key in Vault. The product never reads the host credential chain.
   */
  async function registerAwsConnection(aws) {
    caption('Kỹ sư nền tảng mở trang Kết nối, chọn “Tài khoản AWS” và nhập access key có phạm vi vào hai ô luôn bị che; Console kiểm tra danh tính, quyền và vùng trước khi lưu vào Vault');
    await h.click(nav('Kết nối'));
    await expect(page.getByRole('heading', { name: 'Kết nối', level: 1, exact: true })).toBeVisible();
    await h.click(page.getByLabel('Tài khoản AWS', { exact: true }));
    const keyField = page.getByLabel('Access key ID', { exact: true });
    const secretField = page.getByLabel('Secret access key', { exact: true });
    await expect(keyField).toHaveAttribute('type', 'password');
    await expect(secretField).toHaveAttribute('type', 'password');
    await h.type(page.getByLabel('Tên kết nối', { exact: true }), aws.name, { replace: true });
    await h.paste(keyField, aws.accessKeyId);
    await h.paste(secretField, aws.secretAccessKey);
    await h.type(page.getByLabel('Vùng được phép', { exact: true }), aws.region, { replace: true });
    expect(await page.locator('body').innerText(), 'key material must not be visible').not.toContain(aws.secretAccessKey);
    const registeredResponse = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/connections/aws', { timeout: 180_000 });
    await h.click(page.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }));
    const response = await registeredResponse;
    const text = await response.text();
    if (text.includes(aws.secretAccessKey) || text.includes(aws.accessKeyId)) throw new Error('the registration response echoed key material');
    if (response.status() >= 300) throw new Error(`AWS Connection registration was refused (HTTP ${response.status()})`);
    const listed = (await get('/connections')).connections.find((c) => c.name === aws.name || c.key === aws.name);
    const ok = listed?.status === 'READY' && listed.kind === 'AWS';
    rec.record('ui.connections.registered', 'PREPARE', 'ui', ok, `AWS Connection ${aws.name} ${listed?.status ?? 'missing'}; key fields cleared`);
    expect(await keyField.inputValue()).toBe('');
    expect(await secretField.inputValue()).toBe('');
    await h.read('Kết nối AWS đã được kiểm tra và đang sẵn sàng; hai ô khóa đã được xóa');
    return listed?.key ?? aws.name;
  }

  /**
   * Registers the Console-authored AWS Definitions of one scenario from the shared
   * catalog file (the planner test plans exactly these entries).
   * @param {{definitions:{common:object[]}}} catalog
   */
  async function registerAwsDefinitions(catalog, scenario, subs) {
    caption('Cấu hình tài nguyên AWS: VPC, EKS, namespace, Aurora và bộ nhớ đệm — chỉ chọn module đã được registry xác minh; giá trị được nhập qua form, không có ô URL hay digest');
    await h.click(nav('Cấu hình tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
    for (const d of [...catalog.definitions.common, ...catalog.definitions[scenario]]) {
      await registerDefinition({ ...d, overrides: (d.overrides ?? []).map(([name, value]) => [name, decodeCatalogOverride(value, subs)]) });
    }
    const defs = (await get('/resource-definitions')).resourceDefinitions.map((x) => x.key).sort();
    const capabilities = (await get('/resource-definition-capabilities')).capabilities;
    const wantModules = [...catalog.definitions.common, ...catalog.definitions[scenario]].map((d) => d.module);
    const pinned = wantModules.every((m) => capabilities.some((c) => c.driverType === 'terraform' && c.module === m && c.source?.contentSha256));
    return { defs, pinned, capabilities };
  }

  const cacheVariables = ['ACCEPTANCE_CACHE_APP', 'ACCEPTANCE_CACHE_ENV', 'ACCEPTANCE_CACHE_RUN', 'ACCEPTANCE_CACHE_TTL_SECONDS'];
  const dbOutputs = [['PGHOST', 'db', 'host'], ['PGPORT', 'db', 'port'], ['PGDATABASE', 'db', 'database'], ['PGUSER', 'db', 'username'], ['PGPASSWORD', 'db', 'password · secret']];
  const cacheOutputs = [['REDIS_HOST', 'cache', 'host'], ['REDIS_PORT', 'cache', 'port'], ['REDIS_USERNAME', 'cache', 'username'], ['REDIS_PASSWORD', 'cache', 'password · secret'], ['REDIS_TLS', 'cache', 'tls']];
  const sharedResourcesFor = (classes = {}) => [
    { alias: 'db', type: 'postgres', sharedId: ACCEPTANCE.dbId, klass: classes.postgres, params: { database: ACCEPTANCE.database, username: ACCEPTANCE.username } },
    { alias: 'cache', type: 'redis', sharedId: ACCEPTANCE.cacheId, klass: classes.redis },
  ];

  async function addAcceptanceWorkloads(classes, { secret = true } = {}) {
    const sharedResources = sharedResourcesFor(classes);
    caption('Nhà phát triển thêm ba Workload bằng form: backend và worker dùng chung một PostgreSQL và một Redis qua ID dùng chung; mật khẩu chỉ đi qua output bí mật');
    await addWorkload({ name: 'backend', image: input.images.backend, resources: sharedResources, variables: ['PORT', 'ACCEPTANCE_CONFIG', ...(secret ? ['ACCEPTANCE_SECRET_SHA256'] : []), ...cacheVariables], secrets: secret ? ['ACCEPTANCE_SECRET'] : [], outputs: [...dbOutputs, ...cacheOutputs], port: { name: 'http', port: '8080' } });
    await addWorkload({ name: 'worker', image: input.images.worker, resources: sharedResources, variables: ['POLL_INTERVAL_MS', ...cacheVariables], outputs: [...dbOutputs, ...cacheOutputs] });
    await addWorkload({ name: 'frontend', image: input.images.frontend, variables: ['PORT'], service: { name: 'BACKEND_URL', workload: 'backend', port: 'http' }, port: { name: 'http', port: '8080' } });
  }

  return {
    nav, get, form, alert, choose, override, startDefinition, signIn, signOut, optionLabel, submitDefinition, registered,
    registerKubernetesConnection, registerAwsConnection, registerAwsDefinitions, registerSecretStore, registerTypes, registerDefinitions, registerRenderTemplate, openApplication, backToApplication, bindEnvironment, putVariables, addWorkload,
    addAcceptanceWorkloads, cacheVariables, phase,
  };
}
