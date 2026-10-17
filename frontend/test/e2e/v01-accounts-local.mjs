// V01 headed recording: Platform Engineer manages infrastructure accounts
// (AWS and Internal) through the Web Console: create -> detail -> rename ->
// stale update -> read-only reverify (success and failure) -> same-target
// credential replacement (accepted and refused) -> reload -> server restart ->
// reverify from the durable credential. Invoked by
// backend/test/integration/v01-accounts-local.sh.
//
// REAL: the Web Console bundle, the product HTTP handlers and services, a
// disposable PostgreSQL database and the Vault KV v2 credential store.
// FAKE (labelled in every caption that depends on it): the AWS HTTP endpoints
// (deterministic awsfake behind the product's real read-only verifier) and the
// Kubernetes verification (deterministic token-prefix fake). A PASS here is NOT
// evidence that real AWS or a real cluster accepts a credential.
// Every product step is a UI action. API calls are used only (a) to assert
// results, (b) for the explicit negative 403 of the Developer and (c) for the
// labelled "another operator" write that makes the UI copy stale.
//   node v01-accounts-local.mjs --check V01
//   node v01-accounts-local.mjs --scenario V01 --headed --captions vi --evidence DIR
// Marks, captions, assertions and the video are written also when a step fails.
// Credentials are random per run, live only in this process, private 0600
// files and masked fields, and are never captioned or logged.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { randomBytes } from 'node:crypto';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { assertNoSecretLeaks } from './secret-scan.mjs';
import { launchHeaded, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const SHORT_READ = 2000;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, typed into a masked field

// Fixture access key IDs understood by awsconnserver's FAKE AWS.
const KEYS = {
  success: 'AKIAFAKESUCCESS00001',
  rotated: 'AKIAFAKESUCCESS00002',
  rejected: 'AKIAFAKEREJECTED0001',
  otherAccount: 'AKIAFAKEOTHERACCT001',
};
const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/+';
const randomSecret = () => Array.from(randomBytes(40), (byte) => ALPHABET[byte % ALPHABET.length]).join('');
const token = (prefix) => `${prefix}-${randomBytes(12).toString('hex')}`;
const kubeconfig = ({ server, tokenValue, context = 'fake-lab' }) => `apiVersion: v1
kind: Config
clusters:
- name: ${context}-cluster
  cluster:
    server: ${server}
users:
- name: ${context}-user
  user:
    token: ${tokenValue}
contexts:
- name: ${context}
  context:
    cluster: ${context}-cluster
    user: ${context}-user
current-context: ${context}
`;

export const SCENARIOS = {
  async V01({ page, h, caption, mark, check, baseURL, fail, evidenceDir }) {
    const privateDir = process.env.ORCH_E2E_PRIVATE_DIR;
    const secrets = { awsFirst: randomSecret(), awsRotated: randomSecret(), awsOther: randomSecret(), awsRejected: randomSecret() };
    const tokens = {
      first: token('FAKEK8S-OK'), rotated: token('FAKEK8S-OK-ROT'), rejected: token('FAKEK8S-REJECTED'),
      noPerms: token('FAKEK8S-NOPERMS'), other: token('FAKEK8S-OK-OTHER'),
    };
    const allSecrets = [...Object.values(secrets), ...Object.values(tokens), ...Object.values(KEYS)];
    const files = {
      first: join(privateDir, 'kube-first.yaml'), rotated: join(privateDir, 'kube-rotated.yaml'), rejected: join(privateDir, 'kube-rejected.yaml'),
      noPerms: join(privateDir, 'kube-noperms.yaml'), otherEndpoint: join(privateDir, 'kube-other-endpoint.yaml'),
    };
    const server = 'https://fake-lab.example:6443';
    writeFileSync(files.first, kubeconfig({ server, tokenValue: tokens.first }), { mode: 0o600 });
    writeFileSync(files.rotated, kubeconfig({ server, tokenValue: tokens.rotated }), { mode: 0o600 });
    writeFileSync(files.rejected, kubeconfig({ server, tokenValue: tokens.rejected }), { mode: 0o600 });
    writeFileSync(files.noPerms, kubeconfig({ server, tokenValue: tokens.noPerms }), { mode: 0o600 });
    writeFileSync(files.otherEndpoint, kubeconfig({ server: 'https://fake-elsewhere.example:6443', tokenValue: tokens.other }), { mode: 0o600 });
    // The runner compares stored/logged data with these values after the run.
    writeFileSync(join(privateDir, 'secret-values'), `${secrets.awsFirst}\n${secrets.awsRotated}\n${secrets.awsOther}\n${secrets.awsRejected}\n${Object.values(tokens).join('\n')}\n`, { mode: 0o600 });

    // The runner may fix the suffix so it knows the two account keys in advance (fault checks).
    const suffix = /^[0-9a-f]{4,12}$/.test(process.env.ORCH_V01_SUFFIX ?? '') ? process.env.ORCH_V01_SUFFIX : randomBytes(2).toString('hex');
    const awsName = `AWS lab ${suffix}`;
    const awsRenamed = `AWS lab đã đổi tên ${suffix}`;
    const internalName = `Cụm lab ${suffix}`;
    const internalRenamed = `Cụm lab đã đổi tên ${suffix}`;
    const rivalName = `AWS do người khác đổi ${suffix}`;

    // Every account API reply is recorded (method, normalized path, status,
    // code and safe ids) and scanned for key material first.
    const replies = [];
    const accounts = {};
    // Every /api/v1 reply (also GET and also replies the scenario never waits
    // on) is scanned for key material, secret references and the legacy
    // allowed-region policy. A hit is a hard failure: it is recorded, thrown by
    // the next wait and re-checked before PASS.
    const violations = [];
    page.on('response', async (response) => {
      const url = new URL(response.url());
      if (!url.pathname.startsWith('/api/v1/')) return;
      const method = response.request().method();
      const text = await response.text().catch(() => '');
      const leaked = allSecrets.some((value) => text.includes(value)) || /secretRef|kv2:\/\/|allowedRegions/.test(text) && url.pathname.startsWith('/api/v1/driver-accounts');
      if (leaked) {
        violations.push(`${method} ${url.pathname.replace(/[a-z0-9-]{3,}$/, ':id')} status ${response.status()}`);
        appendFileSync(`${evidenceDir}/assertions.txt`, `FAIL ${method} ${url.pathname} (${response.status()}) carried key material, a secret reference or an allowed-region policy\n`);
      }
      if (!url.pathname.startsWith('/api/v1/driver-accounts') || method === 'GET') return;
      let body = {};
      try { body = JSON.parse(text); } catch { /* empty */ }
      replies.push({
        method, path: url.pathname.replace(/^\/api\/v1\/driver-accounts\/?/, '').replace(/^[^/]+(?=\/|$)/, (segment) => (['aws', 'internal'].includes(segment) ? segment : ':key')),
        status: response.status(), code: leaked ? undefined : body.code, key: leaked ? undefined : body.key, version: leaked ? undefined : body.version, leaked,
      });
    });
    const assertNoViolations = () => expect(violations, 'a reply carried secret-bearing data').toEqual([]);
    async function sent(action) {
      const before = replies.length;
      await action();
      await expect.poll(() => replies.length, { timeout: 60_000 }).toBeGreaterThan(before);
      assertNoViolations();
      return replies[replies.length - 1];
    }

    const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
    const create = () => page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    const detail = (name) => page.getByRole('region', { name: `Chi tiết tài khoản ${name}` });
    const replaceGroup = (name) => detail(name).getByRole('group', { name: 'Thay credential' });
    const createAlert = () => create().locator('.form-error[role="alert"]');
    const table = () => page.getByRole('table', { name: 'Tài khoản hạ tầng' });
    const row = (name) => table().locator('tbody tr').filter({ has: page.getByRole('button', { name: `Xem ${name}`, exact: true }) });
    const api = async (path) => (await (await page.request.get(`${baseURL}/api/v1${path}`)).json());
    const account = async (key) => api(`/driver-accounts/${key}`);
    const chooseFile = async (scope, file) => {
      const chooser = page.waitForEvent('filechooser');
      chooser.catch(() => {});
      await h.click(scope.locator('input[type="file"]'));
      await (await chooser).setFiles(file);
      await expect(scope.getByText('Đã chọn file:', { exact: false })).toBeVisible();
      await expect(scope.getByLabel('Đích đã chọn')).toBeVisible();
      await h.pause(800);
    };
    const open = async (name) => {
      await h.click(page.getByRole('button', { name: `Xem ${name}`, exact: true }));
      await expect(detail(name)).toBeVisible();
    };
    const scan = (options) => assertNoSecretLeaks(page, allSecrets, options);

    // ---- 1. Developer: no management UI, API refuses ---------------------
    caption('Bước 1: Nhà phát triển đăng nhập; vai trò này không có quyền quản lý tài khoản hạ tầng');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), 'developer');
    await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
    await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
    await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
    await expect(nav('Tài khoản hạ tầng')).toHaveCount(0);
    check('developer: the "Tài khoản hạ tầng" navigation entry is absent');
    mark('developer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 2: Mở thẳng địa chỉ trang; Console không hiển thị biểu mẫu và API từ chối bằng 403 (kiểm tra phủ định)');
    await page.goto(`${baseURL}/ui/platform/driver-accounts`);
    await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toHaveCount(0);
    await expect(page.getByRole('group', { name: 'Đăng ký tài khoản mới' })).toHaveCount(0);
    for (const request of [
      () => page.request.get(`${baseURL}/api/v1/driver-accounts`),
      () => page.request.post(`${baseURL}/api/v1/driver-accounts/aws`, { data: { name: 'Nhà phát triển', accessKeyId: KEYS.success, secretAccessKey: secrets.awsOther, verificationRegions: ['us-east-1'] } }),
    ]) {
      const refused = await request();
      expect(refused.status()).toBe(403);
      expect(await refused.text()).not.toContain(secrets.awsOther);
    }
    check('developer: /ui/platform/driver-accounts shows no form; GET and POST /driver-accounts return 403 (negative assertion, not a seeded step)');
    mark('developer-refused');
    await h.pause(SHORT_READ);
    await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
    await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();

    // ---- 3. Platform Engineer --------------------------------------------
    caption('Bước 3: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ; ô mật khẩu luôn bị che');
    await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), 'platform-engineer');
    await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
    await expect(page.getByLabel('Mật khẩu', { exact: true })).toHaveAttribute('type', 'password');
    await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
    await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
    mark('platform-engineer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 4: Mở Nền tảng › Tài khoản hạ tầng; bản ghi khởi tạo chưa xác minh không được gắn nhãn Sẵn sàng');
    await h.click(nav('Tài khoản hạ tầng'));
    await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/driver-accounts`);
    const seeded = row('internal-cluster');
    await expect(seeded).toContainText('Chưa xác minh');
    await expect(seeded).not.toContainText('Sẵn sàng');
    await expect(page.getByText(/Môi trường:|Gán cho môi trường|Phạm vi áp dụng/)).toHaveCount(0);
    check('list: the unverified seed marker shows "Chưa xác minh", not READY; no Environment assignment or applicability control exists');
    mark('accounts-page');
    await h.moveTo(seeded);
    await h.read('Bản ghi khởi tạo chưa xác minh được hiển thị trung thực');
    const listBefore = (await api('/driver-accounts')).accounts;

    // ---- 5. Internal: failures then success ------------------------------
    caption('Bước 5: Đăng ký tài khoản Internal; kubeconfig được chọn từ file và Console đọc context, không truy cập cluster');
    await h.type(create().getByLabel('Tên tài khoản', { exact: true }), internalName);
    await chooseFile(create(), files.rejected);
    await expect(create().getByLabel('Đích đã chọn')).toContainText('https://fake-lab.example:6443');
    await expect(create().getByLabel('Đích đã chọn')).not.toContainText(tokens.rejected);
    check('internal: file upload inspected; summary shows context, cluster and endpoint only (no token)');
    mark('internal-file-chosen');
    await h.read('Chỉ hiển thị context, cluster và endpoint');

    caption('Bước 6: Cụm giả lập từ chối credential; không lưu tài khoản nào (xác minh Kubernetes là GIẢ LẬP)');
    let reply = await sent(() => h.click(create().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })));
    expect(reply).toMatchObject({ method: 'POST', path: 'internal', status: 422, code: 'VERIFICATION_CREDENTIAL_REJECTED', leaked: false });
    await expect(createAlert()).toContainText('thông tin xác thực bị từ chối');
    check('internal rejected credential: 422 VERIFICATION_CREDENTIAL_REJECTED, nothing saved (fake Kubernetes verification)');
    mark('internal-rejected');
    await h.moveTo(createAlert());
    await h.read('Credential bị từ chối nên không lưu');

    caption('Bước 7: Credential xác thực được nhưng thiếu quyền tạo tài nguyên; chưa đủ để Sẵn sàng');
    await chooseFile(create(), files.noPerms);
    reply = await sent(() => h.click(create().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })));
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_PERMISSION_DENIED', leaked: false });
    await expect(createAlert()).toContainText('thiếu quyền');
    expect((await api('/driver-accounts')).accounts).toHaveLength(listBefore.length);
    check('internal missing permissions: 422 VERIFICATION_PERMISSION_DENIED; the account list is unchanged');
    mark('internal-no-permissions');
    await h.moveTo(createAlert());
    await h.read('Thiếu quyền nên không lưu');

    caption('Bước 8: Chọn kubeconfig hợp lệ và lưu; hệ thống xác minh (giả lập), ghi credential vào Vault thật và tạo tài khoản phiên bản 1');
    await chooseFile(create(), files.first);
    reply = await sent(() => h.click(create().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })));
    expect(reply).toMatchObject({ method: 'POST', path: 'internal', status: 201, leaked: false, version: 1 });
    accounts.internal = reply.key;
    await expect(detail(internalName)).toBeVisible();
    await expect(detail(internalName)).toContainText('Internal (cluster có sẵn)');
    await expect(detail(internalName)).toContainText('Kubeconfig');
    await expect(detail(internalName)).toContainText('https://fake-lab.example:6443');
    await expect(detail(internalName)).toContainText('phiên bản 1');
    await expect(page).toHaveURL(new RegExp(`/ui/platform/driver-accounts\\?account=${accounts.internal}$`));
    await scan();
    check('internal created: 201, version 1, type Internal apart from authentication Kubeconfig, detail shown, no credential anywhere in the page');
    mark('internal-created');
    if (process.env.ORCH_V01_INJECT_FAILURE === 'after-first-create') await fail('injected failure after the first account was created');
    await h.moveTo(detail(internalName));
    await h.read('Tài khoản Internal đã được tạo; loại hạ tầng tách khỏi phương thức xác thực');

    // ---- 9. AWS: local validation, failures, success -----------------------
    const aws = () => create();
    const fillAws = async ({ name, key, secret, regions }) => {
      await h.type(aws().getByLabel('Tên tài khoản', { exact: true }), name, { replace: true });
      await h.type(aws().getByLabel('Access key ID', { exact: true }), key, { replace: true });
      await h.type(aws().getByLabel('Secret access key', { exact: true }), secret, { replace: true });
      await h.type(aws().getByLabel('Vùng kiểm tra', { exact: true }), regions, { replace: true });
    };
    caption('Bước 9: Chọn loại hạ tầng AWS; hai ô access key luôn bị che; “Vùng kiểm tra” chỉ là bằng chứng kiểm tra, không phải vùng triển khai');
    await h.click(aws().getByLabel('AWS', { exact: true }));
    await expect(aws().getByLabel('Access key ID', { exact: true })).toHaveAttribute('type', 'password');
    await expect(aws().getByLabel('Secret access key', { exact: true })).toHaveAttribute('type', 'password');
    await expect(aws().getByText('không phải phạm vi áp dụng của tài khoản', { exact: false })).toBeVisible();
    await expect(page.getByLabel(/Vùng được phép/)).toHaveCount(0);
    check('AWS form: masked key fields, "Vùng kiểm tra" label (evidence only), no allowed-region or Environment control');
    mark('aws-form');
    await h.read('Vùng kiểm tra không phải vùng triển khai');

    caption('Bước 10: Access key sai định dạng bị Console chặn ngay trong trình duyệt, chưa gửi gì lên máy chủ');
    const postsBefore = replies.length;
    await fillAws({ name: awsName, key: 'AKIAkhongdung', secret: secrets.awsOther, regions: 'us-east-1' });
    await h.click(aws().getByRole('button', { name: 'Kiểm tra và lưu', exact: true }));
    await expect(createAlert()).toContainText('Access key ID không đúng dạng');
    expect(replies.length).toBe(postsBefore);
    await scan({ allow: ['input[type=password]'] });
    check('AWS invalid key format: error shown, no request sent, key not echoed outside the masked field');
    mark('aws-local-validation');
    await h.read('Lỗi định dạng được báo ngay');

    caption('Bước 11: AWS (giả lập) từ chối access key; hai ô khóa được xóa, tên được giữ lại');
    await fillAws({ name: awsName, key: KEYS.rejected, secret: secrets.awsRejected, regions: 'us-east-1' });
    reply = await sent(() => h.click(aws().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })));
    expect(reply).toMatchObject({ path: 'aws', status: 422, code: 'VERIFICATION_CREDENTIAL_REJECTED', leaked: false });
    await expect(createAlert()).toContainText('thông tin xác thực bị từ chối');
    await expect(aws().getByLabel('Access key ID', { exact: true })).toHaveValue('');
    await expect(aws().getByLabel('Secret access key', { exact: true })).toHaveValue('');
    await expect(aws().getByLabel('Tên tài khoản', { exact: true })).toHaveValue(awsName);
    await scan();
    check('AWS rejected key: 422, guidance shown, both key fields cleared, name kept');
    mark('aws-rejected');
    await h.read('Khóa bị từ chối; hai ô khóa đã xóa');

    caption('Bước 12: Một vùng kiểm tra bị tắt trên tài khoản (giả lập); xác minh từng vùng nên bị từ chối');
    await fillAws({ name: awsName, key: KEYS.success, secret: secrets.awsFirst, regions: 'us-east-1, ap-east-1' });
    reply = await sent(() => h.click(aws().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })));
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_REGION_UNAVAILABLE', leaked: false });
    await expect(createAlert()).toContainText('vùng được phép chưa dùng được');
    mark('aws-disabled-region');
    await h.read('Vùng bị tắt nên không lưu');

    caption('Bước 13: Nhập khóa hợp lệ và hai vùng kiểm tra; lưu xác minh danh tính, từng vùng và quyền chỉ bằng lời gọi đọc');
    await fillAws({ name: awsName, key: KEYS.success, secret: secrets.awsFirst, regions: 'us-east-1, eu-west-1' });
    await scan({ allow: ['input[type=password]'] });
    mark('aws-valid-entered');
    await h.pause(SHORT_READ);
    reply = await sent(() => h.click(aws().getByRole('button', { name: 'Kiểm tra và lưu', exact: true })));
    expect(reply).toMatchObject({ method: 'POST', path: 'aws', status: 201, leaked: false, version: 1 });
    accounts.aws = reply.key;
    await expect(detail(awsName)).toBeVisible();
    await expect(detail(awsName)).toContainText('123456789012');
    await expect(detail(awsName)).toContainText('us-east-1, eu-west-1');
    await expect(detail(awsName)).toContainText('Access key');
    await expect(aws().getByLabel('Access key ID', { exact: true })).toHaveValue('');
    await scan();
    const created = await account(accounts.aws);
    expect(created).toMatchObject({ type: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'READY', version: 1 });
    expect(created.config.verificationRegions).toEqual(['us-east-1', 'eu-west-1']);
    expect(JSON.stringify(created)).not.toMatch(/allowedRegions|secretRef|region"/);
    check('AWS created: 201, version 1, verificationRegions evidence, no allowedRegions/default region/secretRef in the DTO, keys cleared');
    mark('aws-created');
    await h.moveTo(detail(awsName));
    await h.read('Tài khoản AWS đã được tạo ở phiên bản 1');

    // ---- 14. Rename + stale ----------------------------------------------
    const renameField = (name) => detail(name).getByLabel('Tên tài khoản', { exact: true });
    caption('Bước 14: Đổi tên tài khoản Internal; chỉ tên thay đổi, ID, đích, credential và kết quả xác minh được giữ nguyên');
    await open(internalName);
    const internalBefore = await account(accounts.internal);
    await h.type(renameField(internalName), internalRenamed, { replace: true });
    reply = await sent(() => h.click(detail(internalName).getByRole('button', { name: 'Lưu tên', exact: true })));
    expect(reply).toMatchObject({ method: 'PUT', path: ':key', status: 200, version: 2 });
    await expect(detail(internalRenamed)).toContainText('phiên bản 2');
    const internalAfter = await account(accounts.internal);
    expect(internalAfter).toMatchObject({ id: internalBefore.id, key: internalBefore.key, config: internalBefore.config, version: 2, name: internalRenamed });
    check('internal rename: version 1 -> 2, id/key/config preserved');
    mark('internal-renamed');
    await h.read('Đổi tên thành công, phiên bản tăng một lần');

    caption('Bước 15: Một người vận hành khác (mô phỏng bằng API, không phải thao tác UI) đổi tên tài khoản AWS trước khi bạn lưu');
    await open(awsName);
    await h.type(renameField(awsName), awsRenamed, { replace: true });
    const rival = await page.request.put(`${baseURL}/api/v1/driver-accounts/${accounts.aws}`, { data: { name: rivalName, version: 1 } });
    expect(rival.status()).toBe(200);
    check('simulated other operator renamed the AWS account through the API (setup for the stale case; not a UI step)');
    mark('rival-write');
    caption('Bước 16: Lưu bằng phiên bản cũ bị từ chối 409; bản nháp của bạn được giữ, Console không tự ghi đè hay gửi lại');
    reply = await sent(() => h.click(detail(awsName).getByRole('button', { name: 'Lưu tên', exact: true })));
    expect(reply).toMatchObject({ method: 'PUT', status: 409, code: 'STALE_VERSION', leaked: false });
    await expect(detail(awsName).getByRole('alert')).toContainText('phiên bản không còn khớp');
    await expect(renameField(awsName)).toHaveValue(awsRenamed);
    expect((await account(accounts.aws)).name).toBe(rivalName);
    check('stale rename: 409 STALE_VERSION, draft kept, the other operator\'s name was not overwritten');
    mark('stale-409');
    await h.moveTo(detail(awsName).getByRole('alert'));
    await h.read('Xung đột phiên bản; bản nháp được giữ');
    await h.click(detail(awsName).getByRole('button', { name: 'Tải lại bản mới nhất', exact: true }));
    // The region label now carries the other operator's name (loaded copy).
    await expect(detail(rivalName).getByText('phiên bản hiện tại 2', { exact: false })).toBeVisible();
    await expect(renameField(rivalName)).toHaveValue(awsRenamed);
    caption('Bước 17: Sau khi tải lại bản mới (phiên bản 2) bạn tự lưu lại bản nháp; lần này thành công');
    reply = await sent(() => h.click(detail(rivalName).getByRole('button', { name: 'Lưu tên', exact: true })));
    expect(reply).toMatchObject({ method: 'PUT', status: 200, version: 3 });
    await expect(detail(awsRenamed)).toContainText('phiên bản 3');
    check('explicit reload then explicit save: version 3, no automatic replay');
    mark('stale-resolved');
    await h.read('Đã lưu sau khi tải lại');

    // ---- 18. Reverify ------------------------------------------------------
    caption('Bước 18: Kiểm tra kết nối Internal bằng credential lưu riêng; thành công chỉ khi vượt qua kiểm tra, phiên bản tăng một lần');
    await open(internalRenamed);
    await h.click(detail(internalRenamed).getByRole('button', { name: 'Kiểm tra kết nối', exact: true }));
    await expect(detail(internalRenamed).getByText('Kiểm tra kết nối thành công', { exact: false })).toBeVisible();
    await expect(detail(internalRenamed)).toContainText('phiên bản 3');
    await expect(detail(internalRenamed)).toContainText('không phải tình trạng kết nối hiện tại');
    check('internal reverify: success from the stored credential, version 2 -> 3, UI says READY is the last successful check (fake Kubernetes)');
    mark('internal-verified');
    await h.read('Kiểm tra lại thành công');

    caption('Bước 19: Kiểm tra lại AWS với một vùng bị tắt (giả lập): lần kiểm tra vừa thất bại hiển thị riêng, bản ghi cũ giữ nguyên');
    await open(awsRenamed);
    const awsBeforeFail = await account(accounts.aws);
    const verifyRegions = detail(awsRenamed).getByLabel('Vùng kiểm tra', { exact: true }).first();
    await h.type(verifyRegions, 'us-east-1, ap-east-1', { replace: true });
    reply = await sent(() => h.click(detail(awsRenamed).getByRole('button', { name: 'Kiểm tra kết nối', exact: true })));
    expect(reply).toMatchObject({ method: 'POST', path: ':key/verify', status: 422, code: 'VERIFICATION_REGION_UNAVAILABLE', leaked: false });
    await expect(detail(awsRenamed).getByRole('alert', { name: 'Lần kiểm tra vừa thất bại' })).toContainText('lần kiểm tra thành công gần nhất');
    expect(await account(accounts.aws)).toEqual(awsBeforeFail);
    check('aws reverify failure: 422, shown apart from the last successful READY, committed record byte-identical (version 3)');
    mark('aws-verify-failed');
    await h.moveTo(detail(awsRenamed).getByRole('alert', { name: 'Lần kiểm tra vừa thất bại' }));
    await h.read('Lần kiểm tra thất bại, bản ghi cũ không đổi');
    await h.type(verifyRegions, 'us-east-1, eu-west-1', { replace: true });
    await h.click(detail(awsRenamed).getByRole('button', { name: 'Kiểm tra kết nối', exact: true }));
    await expect(detail(awsRenamed).getByText('Kiểm tra kết nối thành công', { exact: false })).toBeVisible();
    await expect(detail(awsRenamed)).toContainText('phiên bản 4');
    mark('aws-verified');

    // ---- 20. Credential replacement: AWS -----------------------------------
    const awsKeys = () => replaceGroup(awsRenamed);
    const fillReplace = async (key, secret) => {
      await h.type(awsKeys().getByLabel('Access key ID', { exact: true }), key, { replace: true });
      await h.type(awsKeys().getByLabel('Secret access key', { exact: true }), secret, { replace: true });
    };
    const submitReplace = (group) => sent(() => h.click(group.getByRole('button', { name: 'Xác minh và thay credential', exact: true })));
    caption('Bước 20: Thay credential AWS bằng khóa của TÀI KHOẢN AWS KHÁC bị từ chối; credential cũ vẫn được giữ');
    await fillReplace(KEYS.otherAccount, secrets.awsOther);
    reply = await submitReplace(awsKeys());
    expect(reply).toMatchObject({ method: 'PUT', path: ':key/credentials', status: 422, code: 'ACCOUNT_TARGET_MISMATCH', leaked: false });
    await expect(awsKeys().getByRole('alert')).toContainText('không thuộc cùng đích');
    await expect(awsKeys().getByLabel('Secret access key', { exact: true })).toHaveValue('');
    expect(await account(accounts.aws)).toMatchObject({ version: 4 });
    check('aws replacement to another account: 422 ACCOUNT_TARGET_MISMATCH, version unchanged, fields cleared');
    mark('aws-replace-mismatch');
    await h.read('Khác tài khoản AWS nên bị từ chối');

    caption('Bước 21: Khóa mới bị AWS (giả lập) từ chối; credential cũ vẫn dùng được và không có đối tượng mới nào còn lại trong Vault');
    await fillReplace(KEYS.rejected, secrets.awsRejected);
    reply = await submitReplace(awsKeys());
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_CREDENTIAL_REJECTED', leaked: false });
    expect(await account(accounts.aws)).toMatchObject({ version: 4 });
    mark('aws-replace-rejected');
    await h.read('Khóa mới bị từ chối');

    caption('Bước 22: Nhập khóa mới rồi nhấn Hủy; hai ô khóa được xóa và không gửi gì');
    await h.type(awsKeys().getByLabel('Access key ID', { exact: true }), KEYS.rotated, { replace: true });
    await h.type(awsKeys().getByLabel('Secret access key', { exact: true }), secrets.awsRotated, { replace: true });
    const beforeCancel = replies.length;
    await h.click(awsKeys().getByRole('button', { name: 'Hủy', exact: true }));
    await expect(awsKeys().getByLabel('Access key ID', { exact: true })).toHaveValue('');
    await expect(awsKeys().getByLabel('Secret access key', { exact: true })).toHaveValue('');
    expect(replies.length).toBe(beforeCancel);
    await scan();
    mark('aws-replace-cancelled');
    await h.read('Hủy thì credential bị xóa khỏi form');

    caption('Bước 23: Thay bằng khóa mới của CÙNG tài khoản AWS; xác minh xong mới lưu đối tượng mới, đối tượng cũ vẫn được giữ');
    await fillReplace(KEYS.rotated, secrets.awsRotated);
    reply = await submitReplace(awsKeys());
    expect(reply).toMatchObject({ method: 'PUT', status: 200, version: 5, leaked: false });
    await expect(awsKeys().getByText('Đã thay credential', { exact: false })).toBeVisible();
    await expect(awsKeys().getByLabel('Access key ID', { exact: true })).toHaveValue('');
    await expect(detail(awsRenamed)).toContainText('phiên bản 5');
    await scan();
    check('aws replacement of the same account: 200, version 4 -> 5, keys cleared, nothing echoed');
    mark('aws-replaced');
    await h.read('Thay credential thành công');

    // ---- 24. Credential replacement: Internal -------------------------------
    const kubeGroup = () => replaceGroup(internalRenamed);
    await open(internalRenamed);
    caption('Bước 24: Thay kubeconfig Internal bằng file của endpoint KHÁC bị từ chối; credential cũ được giữ');
    await chooseFile(kubeGroup(), files.otherEndpoint);
    reply = await submitReplace(kubeGroup());
    expect(reply).toMatchObject({ method: 'PUT', path: ':key/credentials', status: 422, code: 'ACCOUNT_TARGET_MISMATCH', leaked: false });
    await expect(kubeGroup().getByRole('alert')).toContainText('không thuộc cùng đích');
    expect(await account(accounts.internal)).toMatchObject({ version: 3 });
    check('internal replacement to another endpoint: 422 ACCOUNT_TARGET_MISMATCH, version unchanged');
    mark('internal-replace-mismatch');
    await h.read('Khác endpoint nên bị từ chối');

    caption('Bước 25: Cùng cluster nhưng credential thiếu quyền (giả lập); không lưu');
    await chooseFile(kubeGroup(), files.noPerms);
    reply = await submitReplace(kubeGroup());
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_PERMISSION_DENIED', leaked: false });
    expect(await account(accounts.internal)).toMatchObject({ version: 3 });
    mark('internal-replace-no-permissions');
    await h.read('Thiếu quyền nên giữ credential cũ');

    caption('Bước 26: Cùng cluster, context, endpoint và TLS với credential mới hợp lệ; thay thành công');
    await chooseFile(kubeGroup(), files.rotated);
    reply = await submitReplace(kubeGroup());
    expect(reply).toMatchObject({ method: 'PUT', status: 200, version: 4, leaked: false });
    await expect(kubeGroup().getByText('Đã thay credential', { exact: false })).toBeVisible();
    await expect(detail(internalRenamed)).toContainText('phiên bản 4');
    await scan();
    check('internal replacement of the same target: 200, version 3 -> 4, document cleared');
    mark('internal-replaced');
    await h.read('Thay kubeconfig thành công');

    // ---- 27. Reload --------------------------------------------------------
    caption('Bước 27: Tải lại trang; dữ liệu và phiên bản còn nguyên, chi tiết mở lại theo địa chỉ, biểu mẫu trống');
    await open(awsRenamed);
    await page.reload();
    await expect(detail(awsRenamed)).toContainText('phiên bản 5');
    await expect(row(internalRenamed)).toContainText('Sẵn sàng');
    await expect(replaceGroup(awsRenamed).getByLabel('Access key ID', { exact: true })).toHaveValue('');
    await scan();
    check('reload: names, versions and READY status persisted; detail reopened from ?account=; no key material in the page');
    mark('reloaded');
    await h.read('Dữ liệu còn nguyên sau khi tải lại');

    // ---- 28. Restart ------------------------------------------------------
    caption('Bước 28: Khởi động lại máy chủ sản phẩm (PostgreSQL và Vault thật vẫn chạy); dữ liệu bền vững');
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
      await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), 'platform-engineer');
      await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
      await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
      await page.goto(`${baseURL}/ui/platform/driver-accounts?account=${accounts.aws}`);
    }
    await expect(detail(awsRenamed)).toContainText('phiên bản 5');
    const afterRestart = await account(accounts.internal);
    expect(afterRestart).toMatchObject({ name: internalRenamed, version: 4, status: 'READY' });
    check('after restart: both accounts, names, versions and statuses are unchanged (durable PostgreSQL)');
    mark('restart-data-intact');
    await h.read('Dữ liệu còn nguyên sau khi khởi động lại');

    caption('Bước 29: Kiểm tra kết nối AWS sau khởi động lại; credential mới đọc lại từ Vault thật và vượt qua kiểm tra (AWS GIẢ LẬP)');
    await h.click(detail(awsRenamed).getByRole('button', { name: 'Kiểm tra kết nối', exact: true }));
    await expect(detail(awsRenamed).getByText('Kiểm tra kết nối thành công', { exact: false })).toBeVisible();
    await expect(detail(awsRenamed)).toContainText('phiên bản 6');
    check('aws reverify after restart: the stored credential was read back from Vault; version 5 -> 6');
    mark('aws-verified-after-restart');
    await h.read('Credential lưu bền vững và dùng lại được');

    caption('Bước 30: Đăng xuất. Đây là bằng chứng với PostgreSQL và Vault thật nhưng AWS/Kubernetes GIẢ LẬP, chưa phải xác minh với hạ tầng thật');
    await scan();
    await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
    await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();
    mark('signed-out');
    await h.pause(3000);

    if (process.env.ORCH_V01_INJECT_FAILURE === 'leak-reply') {
      // Debug only: a stubbed list reply that carries a secret must fail the run.
      await page.route('**/api/v1/driver-accounts', (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ accounts: [], note: secrets.awsFirst }) }));
      await page.evaluate(() => fetch('/api/v1/driver-accounts').then((r) => r.text()));
      await h.pause(1000);
    }
    assertNoViolations();
    expect(replies.some((reply) => reply.leaked)).toBe(false);
    check(`global: ${replies.length} mutating replies and every /api/v1 reply were free of key material, secret references and allowed-region policy`);
    writeFileSync(`${evidenceDir}/account-replies.json`, JSON.stringify({ fake: 'AWS and Kubernetes verification are deterministic fakes', replies }, null, 2));
    writeFileSync(`${evidenceDir}/accounts.json`, JSON.stringify({ aws: accounts.aws, internal: accounts.internal }, null, 2));
    if (process.env.ORCH_V01_INJECT_FAILURE === 'after-success') await fail('injected failure after success');
  },
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
