// V02 headed recording: Platform Engineer authors, edits and checks Resource
// Definitions through the Web Console: Name + Driver Account + literal region,
// the five-field AND/OR criteria, the server-side matching preview (unique
// winner, tie, no match, unusable winner account, unconfigured Environment),
// a version-CAS edit that keeps the ID and Resource Type, a stale 409 review,
// reload and a product-server restart. Invoked by
// backend/test/integration/v02-definitions-local.sh.
//
// REAL: the Web Console bundle, the product HTTP handlers and services, a
// disposable PostgreSQL database (Console store) and a Vault KV v2 credential
// store holding the two fixture accounts' credentials.
// FAKE (labelled in the captions that depend on it): the AWS HTTP endpoints
// (deterministic awsfake behind the product's real read-only verifier). No
// Terraform runs, no cloud, cluster or Docker is contacted, and a PASS is NOT
// evidence that a real AWS account accepts the credential or that a plan can be
// executed (V02 does not run live AWS).
//
// UI actions: every Definition create/edit/save, the account and region choice,
// every criteria edit and every matching check. API calls are used only (a) to
// prepare labelled fixtures the scenario does not claim as UI operations
// (Resource Types, the two fixture accounts and three competing Definitions),
// (b) to assert stored results, (c) for the explicit negative 403/400 checks and
// (d) for the labelled "another operator" write that makes the UI copy stale.
//   node v02-definitions-local.mjs --check V02|V02P
//   node v02-definitions-local.mjs --scenario V02|V02P --headed --captions vi --evidence DIR
// Marks, captions, assertions and the video are written also when a step fails.
// Credentials are random per run, live only in this process, private 0600 files
// and never in a caption, a reply that the page keeps or the evidence.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { assertNoSecretLeaks } from './secret-scan.mjs';
import { launchHeaded, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';
import { provisionScenario } from './v02-provision-scenario.mjs';

const SHORT_READ = 2200;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, typed into a masked field

// Fixture access key IDs understood by awsconnserver's FAKE AWS (distinct AWS accounts).
const KEYS = { accountA: 'AKIAFAKESUCCESS00001', accountB: 'AKIAFAKEOTHERACCT001' };
const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/+';
const randomSecret = () => Array.from(randomBytes(40), (byte) => ALPHABET[byte % ALPHABET.length]).join('');

const APP = 'acceptance';
const APP_LABEL = 'Acceptance Application (acceptance)';

export const SCENARIOS = {
  async V02({ page, h, caption, mark, check, baseURL, fail, evidenceDir }) {
    const privateDir = process.env.ORCH_E2E_PRIVATE_DIR;
    const secrets = { accountA: randomSecret(), accountB: randomSecret() };
    const allSecrets = [...Object.values(secrets), ...Object.values(KEYS)];
    // The runner compares stored/logged data with these values after the run.
    writeFileSync(join(privateDir, 'secret-values'), `${secrets.accountA}\n${secrets.accountB}\n`, { mode: 0o600 });
    const suffix = /^[0-9a-f]{4,12}$/.test(process.env.ORCH_V02_SUFFIX ?? '') ? process.env.ORCH_V02_SUFFIX : randomBytes(2).toString('hex');
    const names = {
      accountA: `AWS lab A ${suffix}`, accountB: `AWS lab B ${suffix}`,
      lab: `VPC lab ${suffix}`, labRenamed: `VPC lab đã đổi tên ${suffix}`, rival: `VPC do người khác đổi ${suffix}`,
      namespace: `Namespace lab ${suffix}`, inherit: `Namespace kế thừa ${suffix}`, eks: `Cluster EKS lab ${suffix}`,
      tpl: `Mẫu dựng lab ${suffix}`, tplRenamed: `Mẫu dựng lab đã đổi tên ${suffix}`,
    };
    const ID = { lab: `vpc-lab-${suffix}`, wild: `fx-wildcard-${suffix}`, fast: `fx-fast-${suffix}`, staging: `fx-staging-${suffix}`, ns: `ns-lab-${suffix}`, nsInherit: `ns-inherit-${suffix}`, eks: `eks-lab-${suffix}`, tpl: `tpl-lab-${suffix}` };

    // Every /api/v1 reply (also GET and also replies the scenario never waits
    // on) is scanned for key material and secret references. A hit is a hard
    // failure: recorded, thrown by the next wait and re-checked before PASS.
    const violations = [];
    const writes = [];
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
      if (!url.pathname.startsWith('/api/v1/resource-definitions') || !['POST', 'PUT'].includes(method)) return;
      let body = {};
      try { body = JSON.parse(text); } catch { /* empty */ }
      writes.push({ method, path: url.pathname.replace('/api/v1/resource-definitions', '') || '/', status: response.status(), code: leaked ? undefined : body.code, key: leaked ? undefined : body.key, version: leaked ? undefined : body.version });
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
    const wrappedSelects = new Set(['Bundle dựng ứng dụng', 'Loại tài nguyên', 'Module đã kiểm chứng', 'Tài khoản hạ tầng', 'Cluster đích của provider kubernetes', 'Class cluster EKS của provider kubernetes', 'Ứng dụng', 'Môi trường (tùy chọn)', 'Ứng dụng kiểm tra', 'Môi trường kiểm tra']);
    const form = (name) => (wrappedSelects.has(name)
      ? page.locator(`select[aria-label="${name}"], label:has-text("${name}") > select`).first()
      : page.getByLabel(name, { exact: true }));
    const submitCreate = () => page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
    const submitSave = () => page.getByRole('button', { name: 'Lưu thay đổi', exact: true });
    const entry = (id) => page.locator(`[data-definition="${id}"]`);
    const alertBox = () => page.getByRole('alert').first();
    // The textarea sits inside its label, so the label text also holds the JSON: match by substring.
    const jsonVariables = () => page.getByLabel('Tham số cấu hình (JSON object)');
    const resultRegion = () => page.getByRole('region', { name: 'Kết quả matching chính xác' });
    const runPreview = () => page.getByRole('button', { name: 'Kiểm tra matching', exact: true });
    const api = async (path) => (await (await page.request.get(`${baseURL}/api/v1${path}`)).json());
    const optionLabel = async (select, prefix) => {
      const texts = await select.locator('option').allTextContents();
      const found = texts.find((text) => text.trim().startsWith(prefix));
      if (!found) throw new Error(`no option starting with "${prefix}"`);
      return found;
    };
    const choose = async (select, prefix) => h.choose(select, await optionLabel(select, prefix));
    const scan = (options) => assertNoSecretLeaks(page, allSecrets, options);
    const stored = async (id) => (await api('/resource-definitions')).resourceDefinitions.find((def) => def.key === id);
    const fixtureAccount = async (name, keyId, secret) => {
      const response = await page.request.post(`${baseURL}/api/v1/driver-accounts/aws`, { data: { name, accessKeyId: keyId, secretAccessKey: secret, verificationRegions: ['us-east-1'] } });
      expect(response.status(), 'fixture account creation').toBe(201);
      const created = await response.json();
      expect(created.status).toBe('READY');
      return created;
    };
    const modulePin = async (resourceType) => {
      const capability = (await api('/resource-definition-capabilities')).capabilities.find((item) => item.resourceType === resourceType && item.driverType === 'terraform');
      const { sourceId, moduleId, revision, subdirectory, contentSha256 } = capability.source;
      return { source_id: sourceId, module_id: moduleId, revision, subdirectory, content_sha256: contentSha256 };
    };
    const vpcBody = (pin, { id, name, account, region, criteria }) => ({
      key: id, name, resourceType: 'vpc', driverType: 'terraform', driverAccount: account, provision: {}, criteria,
      driverInputs: { values: { source: pin, variables: { name: '${context.infra.resourceName}', region }, input_mapping: {},
        output_mapping: { id: 'id', cidr: 'cidr', subnetIds: 'subnetIds', privateSubnetIds: 'privateSubnetIds' }, provider_bindings: { aws: { target: 'account' } } } },
    });
    async function signIn(username) {
      await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), username);
      await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
      await expect(page.getByLabel('Mật khẩu', { exact: true })).toHaveAttribute('type', 'password');
      await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
      await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
    }
    async function signOut() {
      await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
      await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();
    }
    async function openDefinitions() {
      await h.click(nav('Cấu hình tài nguyên'));
      await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
      await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-definitions`);
    }
    // The shared context of the matching check. Environment keys are real IDs.
    async function setContext({ resource, klass }) {
      await choose(form('Ứng dụng kiểm tra'), APP_LABEL);
      await choose(form('Môi trường kiểm tra'), 'Staging');
      await h.type(form('ID tài nguyên kiểm tra (res_id)'), resource, { replace: true });
      await h.type(form('Class kiểm tra'), klass, { replace: true });
    }
    async function checkMatching(expectedStatusText) {
      await h.click(runPreview());
      await expect(resultRegion()).toBeVisible();
      await expect(resultRegion()).toContainText(expectedStatusText);
      assertNoViolations();
      await h.moveTo(resultRegion());
    }

    // ---- 1. Developer: no authoring UI, API refuses ---------------------
    caption('Bước 1: Nhà phát triển đăng nhập; vai trò này không có quyền đăng ký hay sửa Cấu hình tài nguyên');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signIn('developer');
    await expect(nav('Cấu hình tài nguyên')).toHaveCount(0);
    check('developer: the "Cấu hình tài nguyên" navigation entry is absent');
    mark('developer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 2: Mở thẳng địa chỉ trang; Console không hiển thị biểu mẫu và API từ chối bằng 403 cho cả đăng ký lẫn sửa (kiểm tra phủ định)');
    await page.goto(`${baseURL}/ui/platform/resource-definitions`);
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toHaveCount(0);
    await expect(page.getByLabel('Tên hiển thị', { exact: true })).toHaveCount(0);
    for (const request of [
      () => page.request.get(`${baseURL}/api/v1/resource-definitions`),
      () => page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: { key: 'dev-attempt', name: 'x', resourceType: 'vpc', driverType: 'terraform', criteria: [{}] } }),
      () => page.request.put(`${baseURL}/api/v1/resource-definitions/dev-attempt`, { data: { key: 'dev-attempt', name: 'x', resourceType: 'vpc', driverType: 'terraform', criteria: [{}], version: 1 } }),
    ]) expect((await request()).status()).toBe(403);
    check('developer: no form on /ui/platform/resource-definitions; GET, POST and PUT /resource-definitions return 403 (negative assertion, not a seeded step)');
    mark('developer-refused');
    await h.pause(SHORT_READ);
    await signOut();

    // ---- 3. Platform Engineer + labelled fixtures -----------------------
    caption('Bước 3: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ; ô mật khẩu luôn bị che');
    await signIn('platform-engineer');
    mark('platform-engineer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 4 (chuẩn bị bằng API, không phải thao tác UI): hai tài khoản AWS GIẢ LẬP, bốn Loại tài nguyên, ba đối thủ vpc và một cấu hình EKS. AWS giả lập; Vault và PostgreSQL thật');
    await h.moveTo(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 }));
    for (const type of [
      { key: 'vpc', inputs: [], outputs: [{ name: 'id', type: 'string', required: true }, { name: 'cidr', type: 'string' }, { name: 'subnetIds', type: 'any' }, { name: 'privateSubnetIds', type: 'any' }] },
      { key: 'k8s-namespace', inputs: [], outputs: [{ name: 'name', type: 'string', required: true }] },
      { key: 'workload', inputs: [], outputs: [] },
      { key: 'k8s-cluster', inputs: [], outputs: [{ name: 'name', type: 'string', required: true }, { name: 'endpoint', type: 'string' }, { name: 'kubeContext', type: 'string', required: true }, { name: 'kubeconfig', type: 'string' }] },
    ]) expect((await page.request.post(`${baseURL}/api/v1/resource-types`, { data: type })).status()).toBe(201);
    const accountA = await fixtureAccount(names.accountA, KEYS.accountA, secrets.accountA);
    const accountB = await fixtureAccount(names.accountB, KEYS.accountB, secrets.accountB);
    expect(accountA.key).not.toBe(accountB.key);
    writeFileSync(`${evidenceDir}/accounts.json`, JSON.stringify({ aws: accountA.key, awsSecond: accountB.key }, null, 2));
    const pin = await modulePin('vpc');
    for (const fixture of [
      { id: ID.wild, name: 'Đối thủ wildcard', criteria: [{}] },
      { id: ID.fast, name: 'Đối thủ class fast', criteria: [{ class: 'fast' }] },
      { id: ID.staging, name: 'Đối thủ loại môi trường staging', criteria: [{ env_type: 'staging' }] },
    ]) {
      const response = await page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: vpcBody(pin, { ...fixture, account: accountA.key, region: 'us-east-1' }) });
      expect(response.status(), `fixture definition ${fixture.id}`).toBe(201);
    }
    // The cluster class the Kubernetes provider can inherit exists only when a verified EKS Definition declares it (fixture, API).
    const eksPin = await modulePin('k8s-cluster');
    const eksResponse = await page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: {
      key: ID.eks, name: names.eks, resourceType: 'k8s-cluster', driverType: 'terraform', driverAccount: accountA.key, provision: {}, criteria: [{ class: 'eks' }],
      driverInputs: { values: { source: eksPin, input_mapping: {}, output_mapping: { name: 'name', endpoint: 'endpoint', kubeContext: 'kubeContext', kubeconfig: 'kubeconfig' }, provider_bindings: { aws: { target: 'account' } },
        variables: { name: '${context.infra.resourceName}', region: 'us-east-1', subnet_ids: "${resources['vpc.default#@infra'].outputs.subnetIds}", kubernetes_version: '1.35', node_instance_type: 't4g.medium', node_capacity_type: 'SPOT', node_ami_type: 'AL2023_ARM_64_STANDARD', node_disk_size: 20, ebs_csi_driver: true, public_access_cidrs: ['203.0.113.10/32'] } } },
    } });
    expect(eksResponse.status(), 'fixture EKS definition').toBe(201);
    check(`fixtures (API, not claimed as UI): 2 fake-verified READY AWS accounts, Types vpc, k8s-namespace, workload and k8s-cluster, 3 competing vpc Definitions (wildcard 0, env_type 1, class 16) and one EKS Definition that publishes the cluster class "eks"`);
    const environment = (await api(`/applications/${APP}`)).environments?.find((env) => env.key === 'staging') ?? (await api('/applications')).applications.find((app) => app.key === APP).environments.find((env) => env.key === 'staging');
    expect(environment.executionProfile ?? '').toBe('');
    check('the matching context Environment (acceptance/staging) has NO execution profile or Connection: it is UNCONFIGURED');
    mark('fixtures-ready');
    await h.pause(SHORT_READ);

    // ---- 5. Open the page, explain ID vs res_id and the rules -----------
    caption('Bước 5: Mở Nền tảng › Cấu hình tài nguyên; giải thích ID cấu hình (Definition ID) khác ID tài nguyên res_id và quy tắc AND/OR, trọng số, wildcard');
    await openDefinitions();
    await expect(page.getByText(/ID cấu hình là tên kỹ thuật của cấu hình này/)).toBeVisible();
    await h.moveTo(page.getByText(/ID cấu hình là tên kỹ thuật của cấu hình này/));
    await h.read('ID cấu hình khác ID tài nguyên res_id dùng trong điều kiện áp dụng');
    const rules = page.getByRole('note', { name: 'Quy tắc so khớp' });
    await expect(rules).toContainText('AND trong một dòng, OR giữa các dòng');
    await expect(rules).toContainText('env_type 1 · app_id 2 · env_id 4 · res_id 8 · class 16');
    await expect(rules).toContainText('Dòng trống {} là wildcard tường minh');
    await expect(rules).toContainText('Không có điều kiện (danh sách trống) bị từ chối');
    await h.moveTo(rules);
    check('UI explains: Definition ID vs res_id, AND within a row / OR across rows, weights 1/2/4/8/16, explicit wildcard {} vs no criteria rejected');
    await expect(page.getByLabel('Phạm vi triển khai')).toHaveCount(0);
    await expect(page.getByRole('group', { name: /Xem trước phạm vi/ })).toHaveCount(0);
    check('no profile selector and no browser-only scope preview exist');
    await h.read('Không có bộ chọn phạm vi triển khai và không có xem trước phạm vi chạy trong trình duyệt');
    await h.pause(SHORT_READ);

    // ---- 6. Form guidance and server schema error -------------------------
    caption('Bước 6: Biểu mẫu chặn trước khi gửi: thiếu ID, thiếu Tên; chọn Loại vpc thì bắt buộc chọn tài khoản AWS sẵn sàng và vùng literal hợp lệ');
    await h.click(submitCreate());
    await expect(alertBox()).toContainText('Nhập ID cấu hình.');
    await h.type(form('ID cấu hình (Definition ID)'), ID.lab);
    await h.click(submitCreate());
    await expect(alertBox()).toContainText('Nhập Tên hiển thị của cấu hình.');
    await h.type(form('Tên hiển thị'), names.lab);
    await choose(form('Loại tài nguyên'), 'vpc');
    await expect(form('Driver Type')).toHaveValue('Terraform (module đã kiểm chứng)');
    check('Driver Type comes from server metadata (read-only) after choosing the Resource Type');
    await expect(page.getByRole('region', { name: 'Tài khoản và nguồn phụ thuộc' })).toContainText('Cấu hình không lấy tài khoản hay vùng từ Môi trường');
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Dùng preset cho name', exact: true }));
    await h.click(submitCreate());
    await expect(alertBox()).toContainText('Chọn tài khoản AWS sẵn sàng');
    const accountSelect = form('Tài khoản hạ tầng');
    const optionTexts = await accountSelect.locator('option').allTextContents();
    expect(optionTexts.filter((text) => text.includes(names.accountA) || text.includes(names.accountB)).length).toBe(2);
    expect(optionTexts.some((text) => /INTERNAL|internal-cluster|Cluster nội bộ/.test(text))).toBe(false);
    check('the account selector offers only the two READY AWS accounts (no internal or unverified account)');
    await choose(accountSelect, names.accountA);
    await h.click(submitCreate());
    await expect(alertBox()).toContainText('Nhập vùng AWS là một chuỗi literal hợp lệ');
    await h.type(form('Vùng AWS'), 'mars-1');
    await h.click(submitCreate());
    await expect(alertBox()).toContainText('Nhập vùng AWS là một chuỗi literal hợp lệ');
    check('client guidance: missing account, missing region and malformed region are refused before any request');
    mark('guidance-shown');
    await h.read('Thiếu tài khoản và vùng sai định dạng đều bị chặn trước khi gửi');

    caption('Bước 7: Lỗi schema từ máy chủ: nhập tham số lạ bằng JSON nâng cao; máy chủ trả 400, giữ nguyên bản nháp và nêu trường liên quan, không lộ giá trị');
    await h.type(form('Vùng AWS'), 'ap-southeast-1', { replace: true });
    await h.click(page.getByRole('button', { name: 'Tham số: JSON nâng cao', exact: true }));
    await h.type(jsonVariables(), '{"name":"${context.infra.resourceName}","not_a_module_variable":1}', { replace: true, delay: 20 });
    const rejected = await sent(() => h.click(submitCreate()));
    expect(rejected.status).toBe(400);
    await expect(alertBox()).toContainText('Máy chủ từ chối cấu hình');
    await expect(form('ID cấu hình (Definition ID)')).toHaveValue(ID.lab);
    await expect(form('Vùng AWS')).toHaveValue('ap-southeast-1');
    expect(await stored(ID.lab)).toBeUndefined();
    check('server schema error: HTTP 400, nothing stored, ID, Name, account and region kept in the form');
    mark('schema-error');
    await h.read('Bản nháp được giữ nguyên sau lỗi schema');
    await h.type(jsonVariables(), '{"name":"${context.infra.resourceName}"}', { replace: true, delay: 20 });
    await h.click(page.getByRole('button', { name: 'Tham số: biểu mẫu', exact: true }));

    // ---- 8. Matching preview before saving --------------------------------
    caption('Bước 8: Điều kiện “Theo ứng dụng” và kiểm tra matching TRÊN MÁY CHỦ; Môi trường chưa cấu hình kết nối vẫn kiểm tra được; trình duyệt không tự chọn cấu hình thắng');
    await h.click(page.getByRole('radio', { name: /Theo ứng dụng/ }));
    await choose(form('Ứng dụng'), APP_LABEL);
    await setContext({ resource: 'network', klass: 'fast' });
    mark('context-set');
    await checkMatching('Có đúng một cấu hình thắng');
    const winner = page.getByRole('group', { name: 'Cấu hình thắng' });
    await expect(winner).toContainText('Đối thủ class fast');
    await expect(winner).toContainText(`ID ${ID.fast}`);
    await expect(winner).toContainText('điểm 16');
    await expect(winner).toContainText(`Tài khoản: ${names.accountA}`);
    await expect(winner).toContainText('vùng hiệu lực: us-east-1');
    check('M1 specificity: candidate app_id=acceptance (2) loses to class=fast (16); winner shows Name, ID, version, account A and its region; env UNCONFIGURED');
    await h.read('Cấu hình có điểm cao nhất duy nhất thắng và nêu rõ tài khoản và vùng');

    caption('Bước 9: Đổi sang “Tùy chỉnh nâng cao”: một dòng class=fast cùng điểm 16 với đối thủ nên HÒA điểm, khác với KHÔNG khớp');
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await expect(resultRegion()).toBeVisible(); // the same candidate: switching the mode alone does not change the request
    await h.type(form('Điều kiện 1 Class'), 'fast', { replace: true });
    await expect(resultRegion()).toHaveCount(0);
    check('editing a criterion drops the previous matching result');
    await h.type(form('Điều kiện 1 ID ứng dụng'), '', { replace: true });
    await checkMatching('Hòa điểm');
    await expect(resultRegion()).toContainText(ID.fast);
    await expect(resultRegion()).not.toContainText('Có đúng một cấu hình thắng');
    check('M2 tie: unique-maximum rule gives AMBIGUOUS with both tied Definitions listed and no winner');
    await h.read('Hòa điểm cao nhất nên chưa có cấu hình thắng');

    caption('Bước 10: AND trong một dòng: thêm app_id=acceptance vào cùng dòng, điểm 16+2=18 vượt đối thủ; cấu hình đang nhập thắng');
    await h.type(form('Điều kiện 1 ID ứng dụng'), APP, { replace: true });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText(`ID ${ID.lab}`);
    await expect(winner).toContainText('điểm 18');
    await expect(winner).toContainText(`Tài khoản: ${names.accountA}`);
    await expect(winner).toContainText('vùng hiệu lực: ap-southeast-1');
    check('M3 AND: class=fast AND app_id=acceptance (18) is the unique winner with account A and region ap-southeast-1');

    caption('Bước 11: OR giữa các dòng: dòng 1 (class=fast VÀ env_id=production) không khớp Môi trường staging, dòng 2 (app_id=acceptance) khớp với điểm 2 nên đối thủ class fast thắng');
    await h.click(page.getByRole('button', { name: '+ Thêm điều kiện', exact: true }));
    await h.type(form('Điều kiện 2 ID ứng dụng'), APP, { replace: true });
    await h.type(form('Điều kiện 1 ID môi trường'), 'production', { replace: true });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText(ID.fast);
    await expect(resultRegion()).toContainText('điều kiện tốt nhất: app_id=acceptance');
    check('M4 OR/AND: row 1 fails on env_id, row 2 scores 2, so the class=fast fixture (16) wins; the candidate reports its best criterion app_id=acceptance');
    await h.read('Mỗi cấu hình lấy điểm của dòng khớp tốt nhất');
    await h.click(page.getByRole('button', { name: 'Xóa điều kiện 2', exact: true }));
    await h.type(form('Điều kiện 1 ID môi trường'), '', { replace: true });

    caption('Bước 12: Mọi thay đổi bản nháp đều làm kết quả cũ biến mất: sửa vùng AWS thì kết quả matching bị bỏ');
    await checkMatching('Có đúng một cấu hình thắng');
    await h.type(form('Vùng AWS'), 'ap-southeast-2', { replace: true });
    await expect(resultRegion()).toHaveCount(0);
    check('editing the region invalidates the displayed matching result (it can not be reused as a Save token)');
    await h.type(form('Vùng AWS'), 'ap-southeast-1', { replace: true });

    caption('Bước 13: Winner dùng vùng không hợp lệ: máy chủ báo xung đột tài khoản/vùng của CHÍNH cấu hình thắng và không chọn cấu hình khác thay thế');
    await h.type(form('Vùng AWS'), 'mars-1', { replace: true });
    await checkMatching('tài khoản hoặc vùng không dùng được');
    await expect(resultRegion()).toContainText(`Xung đột tài khoản của cấu hình thắng ${ID.lab}`);
    await expect(resultRegion()).toContainText('vùng AWS không hợp lệ');
    await expect(resultRegion()).toContainText('Hệ thống không chọn cấu hình thay thế');
    check('M5 unusable winner: CONNECTION_CONFLICT (REGION_INVALID) on the unique winner; the 16-point fixture is NOT promoted');
    await choose(accountSelect, 'Chọn tài khoản');
    await h.type(form('Vùng AWS'), 'ap-southeast-1', { replace: true });
    await checkMatching('tài khoản hoặc vùng không dùng được');
    await expect(resultRegion()).toContainText('cấu hình chưa chọn tài khoản');
    check('M6 unusable winner: no account selected is reported as ACCOUNT_MISSING, again without fallback');
    mark('conflict-shown');
    await h.read('Không có cấu hình thay thế khi winner có tài khoản không dùng được');
    await choose(accountSelect, names.accountA);

    caption('Bước 14: Wildcard tường minh: chế độ “Mọi nơi” gửi đúng criteria [{}] (điểm 0); danh sách điều kiện rỗng không bao giờ được gửi');
    await h.click(page.getByRole('radio', { name: /Mọi nơi/ }));
    // Leaving advanced rows for the wildcard mode drops them, so the editor asks first.
    await h.click(page.getByRole('button', { name: 'Bỏ điều kiện và chuyển chế độ', exact: true }));
    await expect(page.getByRole('radio', { name: /Mọi nơi/ })).toBeChecked();
    const wildcardRequest = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/resource-definitions/matching-preview');
    await h.click(runPreview());
    const sentWildcard = (await wildcardRequest).postDataJSON();
    expect(sentWildcard.definition.criteria).toEqual([{}]);
    expect(sentWildcard.definition).not.toHaveProperty('executionProfile');
    expect(sentWildcard.definition).not.toHaveProperty('connectionKey');
    await expect(resultRegion()).toContainText(ID.fast);
    check('explicit wildcard: the request carries criteria [{}] and no retired profile/Connection field; class=fast fixture (16) still outranks it');

    // Final criteria of the first save: AND row class=fast + app_id=acceptance.
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await expect(form('Điều kiện 1 Class')).toBeVisible();
    await h.type(form('Điều kiện 1 Class'), 'fast', { replace: true });
    await h.type(form('Điều kiện 1 ID ứng dụng'), APP, { replace: true });

    // ---- 15. Save (create) ------------------------------------------------
    caption('Bước 15: Đăng ký cấu hình: một yêu cầu POST, Tên, tài khoản A, vùng ap-southeast-1 và điều kiện AND được lưu; phiên bản bắt đầu từ 1');
    const created = await sent(() => h.click(submitCreate()));
    expect(created).toMatchObject({ method: 'POST', status: 201, key: ID.lab, version: 1 });
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${ID.lab}.` })).toBeVisible();
    await expect(entry(ID.lab)).toContainText(names.lab);
    await expect(entry(ID.lab)).toContainText(`tài khoản ${names.accountA}`);
    await expect(entry(ID.lab)).toContainText('phiên bản 1');
    const createdRecord = await stored(ID.lab);
    expect(createdRecord).toMatchObject({ name: names.lab, version: 1, driverAccount: accountA.key, resourceType: 'vpc', criteria: [{ app_id: APP, class: 'fast' }] });
    expect(createdRecord.driverInputs.values.variables.region).toBe('ap-southeast-1');
    expect(createdRecord.driverInputs.values.provider_bindings).toEqual({ aws: { target: 'account' } });
    expect(createdRecord).not.toHaveProperty('executionProfile');
    expect(createdRecord).not.toHaveProperty('connectionKey');
    check('create: HTTP 201, version 1, driverAccount = account A key, region literal in driverInputs, {target: account} binding, no retired fields');
    mark('created');
    await h.read('Đã đăng ký cấu hình phiên bản 1');

    // ---- 16. Reload -------------------------------------------------------
    caption('Bước 16: Tải lại trang; danh sách và biểu mẫu sửa đọc lại đúng từ PostgreSQL');
    await page.reload();
    await expect(entry(ID.lab)).toContainText(names.lab);
    await expect(entry(ID.lab)).toContainText('phiên bản 1');
    await h.click(page.getByRole('button', { name: `Sửa cấu hình ${ID.lab}`, exact: true }));
    await expect(page.getByRole('heading', { name: `Sửa cấu hình ${ID.lab}` })).toBeVisible();
    await expect(form('ID cấu hình (Definition ID)')).toHaveAttribute('readonly', '');
    await expect(form('Loại tài nguyên')).toBeDisabled();
    await expect(form('Tên hiển thị')).toHaveValue(names.lab);
    await expect(form('Vùng AWS')).toHaveValue('ap-southeast-1');
    await expect(form('Tài khoản hạ tầng')).toHaveValue(accountA.key);
    await expect(page.getByLabel('Điều kiện 1 Class')).toHaveValue('fast');
    check('reload: list and edit form show Name, version 1, account A, region and criteria; ID and Resource Type are read-only');
    mark('reloaded');
    await h.read('ID và Loại tài nguyên không đổi khi sửa');

    // ---- 17. Edit ---------------------------------------------------------
    caption('Bước 17: Sửa toàn bộ cấu hình trừ ID và Loại: đổi Tên, đổi sang tài khoản B và vùng eu-west-1, thêm một dòng điều kiện; Lưu là một PUT theo phiên bản');
    await h.type(form('Tên hiển thị'), names.labRenamed, { replace: true });
    await choose(form('Tài khoản hạ tầng'), names.accountB);
    await h.type(form('Vùng AWS'), 'eu-west-1', { replace: true });
    await h.click(page.getByRole('button', { name: '+ Thêm điều kiện', exact: true }));
    await h.type(form('Điều kiện 2 Loại môi trường'), 'production', { replace: true });
    await setContext({ resource: 'network', klass: 'fast' });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText(`Tài khoản: ${names.accountB}`);
    await expect(winner).toContainText('vùng hiệu lực: eu-west-1');
    check('before saving, the server preview already shows the edited candidate with account B and eu-west-1 as winner');
    const edited = await sent(() => h.click(submitSave()));
    expect(edited).toMatchObject({ method: 'PUT', status: 200, key: ID.lab, version: 2 });
    await expect(page.getByRole('status').filter({ hasText: `Đã lưu cấu hình ${ID.lab} (phiên bản 2)` })).toBeVisible();
    await expect(page.getByRole('status')).toContainText('tài nguyên đã triển khai và snapshot cũ không đổi');
    const editedRecord = await stored(ID.lab);
    expect(editedRecord).toMatchObject({ name: names.labRenamed, version: 2, driverAccount: accountB.key, resourceType: 'vpc' });
    expect(editedRecord.driverInputs.values.variables.region).toBe('eu-west-1');
    expect(editedRecord.criteria).toHaveLength(2);
    check('edit: HTTP 200, version 1 -> 2 exactly once; Name, account B, region and criteria replaced; ID and Resource Type unchanged');
    mark('edited');
    await h.read('Phiên bản tăng đúng một lần');

    // ---- 18. Server-enforced immutability (negative) ----------------------
    caption('Bước 18 (kiểm tra phủ định bằng API): đổi Loại tài nguyên hoặc ID khi sửa bị máy chủ từ chối; bản đang lưu không đổi');
    const keep = await stored(ID.lab);
    const body = (change) => ({ key: ID.lab, name: keep.name, resourceType: keep.resourceType, driverType: keep.driverType, driverAccount: keep.driverAccount, driverInputs: keep.driverInputs, provision: keep.provision ?? {}, criteria: keep.criteria, version: keep.version, ...change });
    expect((await page.request.put(`${baseURL}/api/v1/resource-definitions/${ID.lab}`, { data: body({ resourceType: 'k8s-namespace' }) })).status()).toBe(400);
    expect((await page.request.put(`${baseURL}/api/v1/resource-definitions/${ID.lab}`, { data: body({ key: `${ID.lab}-moi` }) })).status()).toBe(400);
    expect((await page.request.put(`${baseURL}/api/v1/resource-definitions/${ID.lab}`, { data: body({ executionProfile: 'aws-eks' }) })).status()).toBe(400);
    expect((await page.request.put(`${baseURL}/api/v1/resource-definitions/${ID.lab}`, { data: body({ driverAccount: 'khong-ton-tai' }) })).status()).toBe(400);
    expect(await stored(ID.lab)).toMatchObject({ version: 2, resourceType: 'vpc', name: names.labRenamed });
    check('immutable ID/Resource Type, retired executionProfile and unknown account are refused with 400; version stays 2');
    await h.pause(SHORT_READ);

    // ---- 19. Stale CAS ----------------------------------------------------
    caption('Bước 19: “Người khác” sửa cùng cấu hình (ghi bằng API, có gắn nhãn) lúc bản trong UI đang mở; lưu từ UI bị 409, bản nháp được giữ và nút Lưu tắt');
    const rival = await page.request.put(`${baseURL}/api/v1/resource-definitions/${ID.lab}`, { data: body({ name: names.rival, version: 2 }) });
    expect(rival.status()).toBe(200);
    expect((await rival.json()).version).toBe(3);
    await h.type(form('Tên hiển thị'), `${names.labRenamed} (bản nháp của tôi)`, { replace: true });
    const stale = await sent(() => h.click(submitSave()));
    expect(stale.status).toBe(409);
    const review = page.getByRole('alert', { name: 'Cần tải lại cấu hình' });
    await expect(review).toContainText('Chưa có gì bị ghi đè');
    await expect(form('Tên hiển thị')).toHaveValue(`${names.labRenamed} (bản nháp của tôi)`);
    await expect(submitSave()).toBeDisabled();
    expect((await stored(ID.lab)).name).toBe(names.rival);
    check('stale 409: the other operator\'s version 3 is kept, the local draft is preserved, Save is disabled until an explicit reload and no request is replayed');
    mark('stale-conflict');
    await h.read('409: không ghi đè bản của người khác');

    caption('Bước 20: Tải lại bản mới nhất một cách tường minh, xem lại và lưu lại: phiên bản 4');
    await h.click(review.getByRole('button', { name: 'Tải lại bản mới nhất', exact: true }));
    await expect(form('Tên hiển thị')).toHaveValue(names.rival);
    await expect(submitSave()).toBeEnabled();
    await h.type(form('Tên hiển thị'), names.labRenamed, { replace: true });
    const resaved = await sent(() => h.click(submitSave()));
    expect(resaved).toMatchObject({ method: 'PUT', status: 200, version: 4 });
    expect((await stored(ID.lab)).name).toBe(names.labRenamed);
    check('explicit reload then save: version 3 -> 4');
    mark('resaved');

    // ---- 21. Kubernetes module: inherited cluster, legacy label -----------
    caption('Bước 21: Cấu hình Kubernetes (namespace) không chọn tài khoản: cluster được kế thừa, tùy chọn “Môi trường” được ghi nhãn cách cũ giữ tới V03');
    await h.click(page.getByRole('button', { name: 'Hủy sửa', exact: true }));
    await expect(form('ID cấu hình (Definition ID)')).toHaveValue('');
    await h.type(form('ID cấu hình (Definition ID)'), ID.ns);
    await h.type(form('Tên hiển thị'), names.namespace);
    await choose(form('Loại tài nguyên'), 'k8s-namespace');
    await expect(page.getByLabel('Tài khoản hạ tầng')).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Tài khoản và nguồn phụ thuộc' })).toContainText('Module này không chọn tài khoản. Truy cập cluster được kế thừa');
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Dùng preset cho name', exact: true }));
    const target = form('Cluster đích của provider kubernetes');
    await expect(target.locator('option', { hasText: 'cách cũ, giữ tới V03' })).toHaveCount(1);
    await choose(target, 'Cluster của Môi trường');
    await expect(page.getByRole('region', { name: 'Tài khoản và nguồn phụ thuộc' })).toContainText('cluster của Môi trường (cách cũ, giữ tới V03)');
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(form('Điều kiện 1 Class'), 'special', { replace: true });
    await setContext({ resource: 'network', klass: 'default' });
    await checkMatching('Không có cấu hình nào khớp (khác với hòa điểm)');
    await expect(resultRegion()).not.toContainText('Hòa điểm');
    check('M7 no match: the namespace candidate (class=special) does not match class default; the answer is NO_MATCH, distinct from a tie');
    await h.type(form('Class kiểm tra'), 'special', { replace: true });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText('Cluster kế thừa từ:');
    await expect(winner).toContainText('cách cũ, giữ tới V03');
    await expect(winner.getByText('Tài khoản:')).toHaveCount(0);
    check('Kubernetes winner shows the inherited cluster source (legacy Environment, until V03) and no account');
    const nsCreated = await sent(() => h.click(submitCreate()));
    expect(nsCreated).toMatchObject({ status: 201, version: 1, key: ID.ns });
    const nsRecord = await stored(ID.ns);
    expect(nsRecord).not.toHaveProperty('driverAccount');
    expect(nsRecord.driverInputs.values.provider_bindings).toEqual({ kubernetes: { target: 'environment' } });
    check('namespace Definition stored without a driverAccount; its legacy Environment binding is explicit');
    mark('namespace-created');


    // ---- 21b. Kubernetes module inheriting the cluster of an EKS node ------
    caption('Bước 21b: Namespace lấy cluster từ node EKS ({target: resource}): chọn “Cluster do node EKS cung cấp” và class eks, KHÔNG có ô chọn tài khoản lặp lại');
    await h.type(form('ID cấu hình (Definition ID)'), ID.nsInherit);
    await h.type(form('Tên hiển thị'), names.inherit);
    await choose(form('Loại tài nguyên'), 'k8s-namespace');
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Dùng preset cho name', exact: true }));
    await choose(form('Cluster đích của provider kubernetes'), 'Cluster do node EKS cung cấp');
    const clusterClass = form('Class cluster EKS của provider kubernetes');
    await expect(clusterClass.locator('option', { hasText: /^eks$/ })).toHaveCount(1);
    await choose(clusterClass, 'eks');
    await expect(page.getByLabel('Tài khoản hạ tầng')).toHaveCount(0);
    await expect(page.getByRole('region', { name: 'Tài khoản và nguồn phụ thuộc' })).toContainText('cluster do node EKS cung cấp (class eks)');
    check('Kubernetes module with {target: resource}: no account selector at all; the cluster class eks comes from the published EKS Definition');
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(form('Điều kiện 1 Class'), 'inherit', { replace: true });
    await setContext({ resource: 'network', klass: 'inherit' });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText(`ID ${ID.nsInherit}`);
    await expect(winner).toContainText('Cluster kế thừa từ:');
    await expect(winner).toContainText('k8s-cluster.eks#environments.@app.@env');
    await expect(winner).not.toContainText('cách cũ');
    await expect(winner.getByText('Tài khoản:')).toHaveCount(0);
    check('server preview: the winner shows the inherited cluster source k8s-cluster.eks#environments.@app.@env (not the legacy label) and no account');
    const inheritCreated = await sent(() => h.click(submitCreate()));
    expect(inheritCreated).toMatchObject({ status: 201, version: 1, key: ID.nsInherit });
    const inheritRecord = await stored(ID.nsInherit);
    expect(inheritRecord).not.toHaveProperty('driverAccount');
    expect(inheritRecord.driverInputs.values.provider_bindings).toEqual({ kubernetes: { target: 'resource', resource_ref: 'k8s-cluster.eks#environments.@app.@env' } });
    check('stored: no driverAccount and the binding {target: resource, resource_ref} (no repeated account selection)');
    mark('namespace-inherits-cluster');
    await h.read('Cluster được kế thừa từ node EKS, không chọn lại tài khoản');

    // ---- 21c. Rendering template (the current consumer): Name, version, criteria
    caption('Bước 21c: Mẫu dựng ứng dụng (score-k8s, người dùng hiện tại của cấu hình) cũng có Tên và phiên bản, không có profile: tạo, kiểm tra matching TRÊN MÁY CHỦ, rồi lưu');
    await h.click(nav('Mẫu dựng ứng dụng'));
    await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
    await expect(page.getByLabel('Phạm vi triển khai')).toHaveCount(0);
    await h.type(form('ID mẫu'), ID.tpl);
    await h.type(form('Tên hiển thị'), names.tpl);
    const bundleSelect = form('Bundle dựng ứng dụng');
    await expect(bundleSelect.locator('option:not([value=""])')).not.toHaveCount(0);
    await h.click(page.getByRole('radio', { name: /Theo ứng dụng/ }));
    // The bundle selector's label also contains "ứng dụng", so address the criteria editor's own select.
    await choose(page.locator('.criteria-editor label').filter({ hasText: /^Ứng dụng(?! kiểm tra)/ }).locator('select'), APP_LABEL);
    await setContext({ resource: 'modules.web', klass: 'default' });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText(`ID ${ID.tpl}`);
    await expect(winner).toContainText(names.tpl);
    check('template preview on the server: the unsaved template is the unique score-k8s winner (profile-free, five-field criteria, UNCONFIGURED Environment)');
    const tplCreated = await sent(() => h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true })));
    expect(tplCreated).toMatchObject({ method: 'POST', status: 201, key: ID.tpl, version: 1 });
    const tplRecord = await stored(ID.tpl);
    expect(tplRecord).toMatchObject({ name: names.tpl, version: 1, resourceType: 'workload', driverType: 'score-k8s', criteria: [{ app_id: APP }] });
    expect(tplRecord).not.toHaveProperty('executionProfile');
    expect(tplRecord).not.toHaveProperty('connectionKey');
    check('template create: HTTP 201, version 1, Name stored, no executionProfile or connectionKey');
    mark('template-created');
    await h.read('Mẫu dựng ứng dụng phiên bản 1 không có profile');

    caption('Bước 21d: Tải lại trang rồi sửa mẫu: ID không đổi, kiểm tra matching hiện phiên bản đang lưu; đổi Tên và thêm class web; Lưu là một PUT theo phiên bản');
    await page.reload();
    await expect(entry(ID.tpl)).toContainText(names.tpl);
    await expect(entry(ID.tpl)).toContainText('phiên bản 1');
    await h.click(page.getByRole('button', { name: `Sửa mẫu ${ID.tpl}`, exact: true }));
    await expect(form('ID mẫu')).toHaveAttribute('readonly', '');
    await expect(form('Tên hiển thị')).toHaveValue(names.tpl);
    await setContext({ resource: 'modules.web', klass: 'default' });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText('phiên bản 1');
    check('template edit preview keeps the stored version 1 of the same-ID entry it replaces');
    await h.type(form('Tên hiển thị'), names.tplRenamed, { replace: true });
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(form('Điều kiện 1 Class'), 'web', { replace: true });
    await h.type(form('Class kiểm tra'), 'web', { replace: true });
    await checkMatching('Có đúng một cấu hình thắng');
    await expect(winner).toContainText('điểm 18');
    const tplEdited = await sent(() => h.click(submitSave()));
    expect(tplEdited).toMatchObject({ method: 'PUT', status: 200, key: ID.tpl, version: 2 });
    expect(await stored(ID.tpl)).toMatchObject({ name: names.tplRenamed, version: 2, resourceType: 'workload', criteria: [{ app_id: APP, class: 'web' }] });
    check('template edit: version 1 -> 2 once; Name and criteria replaced; ID, Resource Type and driver unchanged');
    mark('template-edited');
    await page.reload();
    await expect(entry(ID.tpl)).toContainText(names.tplRenamed);
    await expect(entry(ID.tpl)).toContainText('phiên bản 2');
    check('template reload: the list reads Name and version 2 back from PostgreSQL');
    await h.read('Mẫu phiên bản 2 đọc lại đúng sau khi tải lại');
    await h.click(nav('Cấu hình tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();

    // ---- 22. Restart -----------------------------------------------------
    caption('Bước 22: Khởi động lại máy chủ sản phẩm (PostgreSQL và Vault thật vẫn chạy); Tên, tài khoản, vùng và phiên bản còn nguyên');
    if (process.env.ORCH_E2E_RESTART_CMD) {
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
      await openDefinitions();
    }
    await expect(entry(ID.lab)).toContainText('phiên bản 4');
    const afterRestart = await stored(ID.lab);
    expect(afterRestart).toMatchObject({ name: names.labRenamed, version: 4, driverAccount: accountB.key, resourceType: 'vpc' });
    expect(afterRestart.driverInputs.values.variables.region).toBe('eu-west-1');
    expect(afterRestart.criteria).toHaveLength(2);
    expect(await stored(ID.nsInherit)).toMatchObject({ version: 1, driverInputs: { values: { provider_bindings: { kubernetes: { target: 'resource' } } } } });
    expect(await stored(ID.tpl)).toMatchObject({ name: names.tplRenamed, version: 2, resourceType: 'workload' });
    check('after restart: Name, version 4, account B, region eu-west-1 and both criteria rows are intact (durable PostgreSQL); the inheriting namespace and rendering template v2 too');
    mark('restart-data-intact');
    await h.read('Dữ liệu còn nguyên sau khi khởi động lại');

    caption('Bước 23: Sửa lần nữa sau khởi động lại: đổi lại tài khoản A và Lưu; phiên bản mới 5 và tài khoản mới được lưu bền vững');
    await h.click(page.getByRole('button', { name: `Sửa cấu hình ${ID.lab}`, exact: true }));
    await expect(form('Tài khoản hạ tầng')).toHaveValue(accountB.key);
    await choose(form('Tài khoản hạ tầng'), names.accountA);
    await h.type(form('Vùng AWS'), 'us-east-1', { replace: true });
    const afterRestartSave = await sent(() => h.click(submitSave()));
    expect(afterRestartSave).toMatchObject({ method: 'PUT', status: 200, version: 5 });
    const final = await stored(ID.lab);
    expect(final).toMatchObject({ version: 5, driverAccount: accountA.key });
    expect(final.driverInputs.values.variables.region).toBe('us-east-1');
    check('post-restart edit: version 4 -> 5 with account A and us-east-1 persisted');
    mark('edited-after-restart');
    await h.read('Phiên bản và tài khoản mới đã lưu');

    caption('Bước 24: Đăng xuất. Đây là bằng chứng với PostgreSQL và Vault thật nhưng AWS GIẢ LẬP; chưa chạy Terraform hay hạ tầng thật');
    await scan();
    await signOut();
    mark('signed-out');
    await h.pause(3000);

    assertNoViolations();
    check(`global: ${writes.length} Definition writes and every /api/v1 reply were free of key material and secret references`);
    writeFileSync(`${evidenceDir}/definition-writes.json`, JSON.stringify({ fake: 'AWS verification is a deterministic fake; no Terraform or cloud', writes }, null, 2));
    if (process.env.ORCH_V02_INJECT_FAILURE === 'after-success') await fail('injected failure after success');
  },
  // Companion-resource (provision) authoring and planner consumption: see v02-provision-scenario.mjs.
  V02P: provisionScenario,
};

export function parseArgs(argv) {
  const args = { scenario: '', captions: '', evidence: '', headed: false };
  for (let i = 0; i < argv.length; i += 1) {
    if (argv[i] === '--scenario') args.scenario = argv[++i] ?? '';
    else if (argv[i] === '--captions') args.captions = argv[++i] ?? '';
    else if (argv[i] === '--evidence') args.evidence = argv[++i] ?? '';
    else if (argv[i] === '--headed') args.headed = true;
    else if (argv[i] === '--check') args.check = argv[++i] ?? '';
    else throw new Error(`unknown argument ${argv[i]}`);
  }
  return args;
}

export function requireScenario(id) {
  if (!Object.hasOwn(SCENARIOS, id)) throw new Error(`unsupported scenario "${id}"; supported: ${Object.keys(SCENARIOS).join(', ')}`);
  return SCENARIOS[id];
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.check !== undefined) { requireScenario(args.check); return; }
  const scenario = requireScenario(args.scenario);
  if (!args.headed) throw new Error('--headed is required: recordings use a headed browser');
  requireCaptionLocale(args.captions);
  const env = process.env;
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_PRIVATE_DIR', 'DISPLAY']) {
    if (!env[name]) throw new Error(`${name} is required`);
  }
  const baseURL = env.ORCH_E2E_URL;
  const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
  const { width, height } = screenSize(env);
  const videoPath = `${evidenceDir}/${args.scenario}-raw.mp4`;
  const assertionsPath = `${evidenceDir}/assertions.txt`;
  const check = (line) => { console.log(`ok: ${line}`); appendFileSync(assertionsPath, `PASS ${line}\n`); };

  const browser = await launchHeaded({ width, height });
  const context = await browser.newContext({ viewport: null });
  context.setDefaultTimeout(30_000);
  await context.addInitScript(installCursor);
  const marks = [];
  let recorder;
  let startedAt;
  let result = 'FAIL';
  const captions = createCaptions(() => (startedAt ? Number(((Date.now() - startedAt) / 1000).toFixed(1)) : 0));
  let finalized = false;
  // Idempotent: finalizes the MP4, marks, captions and result, then closes the
  // browser. Runs on normal end, failure and on SIGTERM/SIGINT/SIGHUP.
  const finalize = async () => {
    if (finalized) return;
    finalized = true;
    try {
      if (recorder) await stopRecording(recorder);
    } finally {
      const total = startedAt ? (Date.now() - startedAt) / 1000 : 0;
      writeFileSync(`${evidenceDir}/marks.json`, JSON.stringify(marks, null, 2));
      if (startedAt) captions.writeSrt(`${evidenceDir}/captions.srt`, total);
      writeFileSync(`${evidenceDir}/result.txt`, `${result}\n`);
      await browser.close();
    }
  };
  for (const [signal, code] of [['SIGHUP', 129], ['SIGINT', 130], ['SIGTERM', 143]]) {
    process.once(signal, () => {
      appendFileSync(assertionsPath, `FAIL interrupted by ${signal}\n`);
      finalize().catch((error) => console.error(error.message ?? error)).finally(() => process.exit(code));
    });
  }
  try {
    const page = await context.newPage();
    const h = createHuman(page);
    const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
    x11.focusBrowser();
    recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
    startedAt = recorder.spawnedAtMs;
    await page.waitForTimeout(1500);
    const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
    const fail = async (message) => { appendFileSync(assertionsPath, `FAIL ${message}\n`); await expect(message, message).toBe(''); };
    await scenario({ page, h, caption: captions.caption, mark, check, baseURL, fail, evidenceDir });
    // PASS only when no FAIL line (for example from a reply listener) exists.
    const recorded = readFileSync(assertionsPath, 'utf8');
    if (/^FAIL /m.test(recorded)) throw new Error('FAIL assertions were recorded during the run');
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
    // Error text never contains a key: key values are typed only into masked fields.
    const text = String(error?.stack ?? error);
    appendFileSync(assertionsPath, `FAIL ${text.split('\n')[0]}\n`);
    console.error(text);
    process.exitCode = 1;
  } finally {
    await finalize();
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main().catch((error) => { console.error(error.message ?? error); process.exit(2); });
}
