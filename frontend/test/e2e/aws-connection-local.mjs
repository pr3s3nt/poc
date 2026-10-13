// I04-06 headed recording: PE/Admin AWS access-key onboarding through the Web
// Console. Invoked by backend/test/integration/aws-connection-local.sh, which
// serves the Console from backend/test/integration/awsconnserver: PostgreSQL
// Console store, Vault KV v2 Connection credential store and the product's
// real read-only awsaccess verifier whose HTTP transport points at a
// deterministic FAKE AWS. A passing run proves the Console flow and the safe
// result handling; it is NOT evidence that real AWS accepts a key (that is
// V2-A01/A02/A03). Every product step is a UI action; API reads only assert
// results and the 403 for a Developer is an explicit negative assertion.
//   node aws-connection-local.mjs --check I04-06
//   node aws-connection-local.mjs --scenario I04-06 --headed --captions vi --evidence DIR
// Marks, captions, assertions and the video are written also when a step fails.
// Key material: the success secret is random per run and lives only in this
// process and the masked fields; it is never captioned, logged or asserted
// into a file.
import { expect } from '@playwright/test';
import { randomBytes } from 'node:crypto';
import { appendFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { assertNoSecretLeaks } from './secret-scan.mjs';
import { launchHeaded, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const SHORT_READ = 2000;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, typed into a masked field

// Fixture access key IDs understood by awsconnserver's FAKE AWS (see its doc).
const KEYS = {
  success: 'AKIAFAKESUCCESS00001',
  rejected: 'AKIAFAKEREJECTED0001',
  noPermissions: 'AKIAFAKENOPERMS00001',
  noSimulate: 'AKIAFAKENOSIMULATE01',
};
const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789/+';
const randomSecret = () => Array.from(randomBytes(40), (byte) => ALPHABET[byte % ALPHABET.length]).join('');

export const SCENARIOS = {
  async 'I04-06'({ page, h, caption, mark, check, baseURL, fail, evidenceDir }) {
    const successSecret = randomSecret();
    const otherSecret = randomSecret();
    const allSecrets = [successSecret, otherSecret, ...Object.values(KEYS)];
    // The runner compares stored and logged data with these values after the
    // run. They go to a private 0600 file outside the evidence, never to it.
    if (process.env.ORCH_E2E_PRIVATE_DIR) {
      writeFileSync(join(process.env.ORCH_E2E_PRIVATE_DIR, 'secret-values'), `${successSecret}\n${otherSecret}\n`, { mode: 0o600 });
    }
    const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
    const alert = () => page.locator('form .form-error[role="alert"]');
    const nameField = () => page.getByLabel('Tên kết nối', { exact: true });
    const keyField = () => page.getByLabel('Access key ID', { exact: true });
    const secretField = () => page.getByLabel('Secret access key', { exact: true });
    const regionField = () => page.getByLabel('Vùng được phép', { exact: true });
    const save = () => page.getByRole('button', { name: 'Kiểm tra và lưu', exact: true });
    const connections = async () => (await (await page.request.get(`${baseURL}/api/v1/connections`)).json()).connections;

    // Every registration reply is recorded (status and code only) and scanned
    // for key material before any assertion uses it.
    const replies = [];
    page.on('response', async (response) => {
      if (response.request().method() !== 'POST' || new URL(response.url()).pathname !== '/api/v1/connections/aws') return;
      const text = await response.text().catch(() => '');
      if (allSecrets.some((value) => text.includes(value)) || /secretRef|kv2:\/\//.test(text)) {
        appendFileSync(`${evidenceDir}/assertions.txt`, 'FAIL a registration reply carried key material or a secret reference\n');
        replies.push({ status: response.status(), leaked: true });
        return;
      }
      let code;
      try { code = JSON.parse(text).code; } catch { code = undefined; }
      replies.push({ status: response.status(), code, leaked: false });
    });
    const posts = () => replies.length;

    async function fillAws({ name, key, secret, regions }) {
      await h.type(nameField(), name, { replace: true });
      await h.type(keyField(), key, { replace: true });
      await h.type(secretField(), secret, { replace: true });
      await h.type(regionField(), regions, { replace: true });
    }
    const submitAndWait = async () => {
      const before = replies.length;
      await h.click(save());
      await expect.poll(() => replies.length, { timeout: 60_000 }).toBeGreaterThan(before);
      return replies[replies.length - 1];
    };

    // 1. Developer is refused (negative case, before any PE step).
    caption('Bước 1: Nhà phát triển đăng nhập; vai trò này không có quyền quản lý Kết nối');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), 'developer');
    await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
    await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
    await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
    await expect(nav('Kết nối')).toHaveCount(0);
    check('developer: the Kết nối navigation entry is absent');
    mark('developer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 2: Mở thẳng địa chỉ trang Kết nối; Console không hiển thị biểu mẫu và API từ chối bằng 403');
    await page.goto(`${baseURL}/ui/platform/connections`);
    await expect(page.getByRole('heading', { name: 'Kết nối', level: 1, exact: true })).toHaveCount(0);
    await expect(page.getByLabel('Access key ID')).toHaveCount(0);
    const refused = await page.request.post(`${baseURL}/api/v1/connections/aws`, {
      data: { name: 'Nhà phát triển', accessKeyId: KEYS.success, secretAccessKey: otherSecret, allowedRegions: ['us-east-1'] },
    });
    expect(refused.status()).toBe(403);
    expect(await refused.text()).not.toContain(otherSecret);
    check('developer: /ui/platform/connections shows no form; POST /connections/aws returns 403 (negative assertion, not a seeded step)');
    mark('developer-refused');
    await h.pause(SHORT_READ);
    await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
    await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();

    // 3. Platform Engineer signs in.
    caption('Bước 3: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ; ô mật khẩu luôn bị che');
    await h.type(page.getByLabel('Tên đăng nhập', { exact: true }), 'platform-engineer');
    await h.type(page.getByLabel('Mật khẩu', { exact: true }), SEEDED_PASSWORD);
    await expect(page.getByLabel('Mật khẩu', { exact: true })).toHaveAttribute('type', 'password');
    await h.click(page.getByRole('button', { name: 'Đăng nhập', exact: true }));
    await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', level: 1 })).toBeVisible();
    const defaultBefore = (await (await page.request.get(`${baseURL}/api/v1/application-connections`)).json()).defaultConnectionKey;
    mark('platform-engineer-signed-in');
    await h.pause(SHORT_READ);

    // 4. Connections page and AWS form.
    caption('Bước 4: Mở Nền tảng › Kết nối; danh sách chỉ có kết nối mặc định, chưa có tài khoản AWS nào được đăng ký thật');
    await h.click(nav('Kết nối'));
    await expect(page.getByRole('heading', { name: 'Kết nối', level: 1, exact: true })).toBeVisible();
    const listBefore = await connections();
    const awsBefore = listBefore.filter((item) => item.kind === 'AWS');
    expect(listBefore.every((item) => item.status === 'READY' || item.status === 'VERIFYING')).toBe(true);
    check(`connections list loaded (${listBefore.length} records, ${awsBefore.length} pre-seeded AWS metadata)`);
    mark('connections-page');
    await h.read('Danh sách kết nối hiện có');

    caption('Bước 5: Chọn loại kết nối “Tài khoản AWS”; hai ô access key luôn bị che và không có nút hiện giá trị');
    await h.click(page.getByLabel('Tài khoản AWS', { exact: true }));
    await expect(keyField()).toHaveAttribute('type', 'password');
    await expect(secretField()).toHaveAttribute('type', 'password');
    await expect(page.getByLabel(/Hiện/)).toHaveCount(0);
    await expect(page.getByText(/backend Terraform|state backend/i)).toHaveCount(0);
    check('AWS form: access key ID and secret access key are type=password; no reveal control; no Terraform backend or host profile input');
    mark('aws-form');
    await h.moveTo(keyField());
    await h.read('Hai ô access key luôn bị che, không có nút hiện giá trị');

    // 6. Local validation: nothing is sent.
    caption('Bước 6: Nhập access key sai định dạng; Console báo lỗi ngay tại trình duyệt và không gửi gì lên máy chủ');
    await fillAws({ name: 'Thử định dạng', key: 'AKIAkhongdung', secret: otherSecret, regions: 'us-east-1' });
    await h.click(save());
    await expect(alert()).toContainText('Access key ID không đúng dạng');
    expect(posts()).toBe(0);
    await assertNoSecretLeaks(page, allSecrets, { allow: ['input[type=password]'] });
    check('invalid key format: error shown, no request was sent, key not echoed outside the masked field');
    mark('local-validation');
    await h.moveTo(alert());
    await h.read('Lỗi định dạng được báo ngay, chưa có yêu cầu nào được gửi');

    // 7. Failure: AWS rejects the key.
    caption('Bước 7: Nhập access key mà AWS từ chối; nút Kiểm tra và lưu báo không lưu gì và xóa hai ô khóa');
    await fillAws({ name: 'Thử khóa bị từ chối', key: KEYS.rejected, secret: otherSecret, regions: 'us-east-1' });
    let reply = await submitAndWait();
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_CREDENTIAL_REJECTED', leaked: false });
    await expect(alert()).toContainText('AWS từ chối access key');
    await expect(keyField()).toHaveValue('');
    await expect(secretField()).toHaveValue('');
    await expect(nameField()).toHaveValue('Thử khóa bị từ chối');
    await assertNoSecretLeaks(page, allSecrets);
    check('rejected key: 422 VERIFICATION_CREDENTIAL_REJECTED, guidance shown, both key fields cleared, name kept');
    mark('rejected-key');
    await h.moveTo(alert());
    await h.read('Khóa bị từ chối; hai ô khóa đã được xóa, tên được giữ lại');

    // 8. Failure: identity ok but capabilities denied (STS alone is not READY).
    caption('Bước 8: Khóa có danh tính hợp lệ nhưng thiếu quyền cấp phát VPC/EKS/ElastiCache/Aurora; chỉ xác minh danh tính thì chưa đủ để READY');
    await fillAws({ name: 'Thử thiếu quyền', key: KEYS.noPermissions, secret: otherSecret, regions: 'us-east-1' });
    reply = await submitAndWait();
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_PERMISSION_DENIED', leaked: false });
    await expect(alert()).toContainText('thiếu quyền');
    await assertNoSecretLeaks(page, allSecrets);
    check('missing capabilities: 422 VERIFICATION_PERMISSION_DENIED although STS identity succeeded; nothing saved');
    mark('missing-capabilities');
    await h.moveTo(alert());
    await h.read('Thiếu quyền cấp phát nên không lưu kết nối');

    // 9. Failure: capabilities cannot be proven read-only.
    caption('Bước 9: Khóa không được phép đọc chính sách của mình nên không chứng minh được quyền; hệ thống từ chối thay vì đoán');
    await fillAws({ name: 'Thử không mô phỏng được', key: KEYS.noSimulate, secret: otherSecret, regions: 'us-east-1' });
    reply = await submitAndWait();
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_CAPABILITY_UNVERIFIABLE', leaked: false });
    await expect(alert()).toContainText('iam:SimulatePrincipalPolicy');
    check('unprovable capabilities: 422 VERIFICATION_CAPABILITY_UNVERIFIABLE; fails closed, nothing saved');
    mark('unverifiable-capabilities');
    await h.moveTo(alert());
    await h.read('Không chứng minh được quyền thì không READY');

    // 10. Failure: disabled region.
    caption('Bước 10: Một vùng được phép bị tắt trên tài khoản; xác minh từng vùng nên đăng ký bị từ chối');
    await fillAws({ name: 'Thử vùng bị tắt', key: KEYS.success, secret: otherSecret, regions: 'us-east-1, ap-east-1' });
    reply = await submitAndWait();
    expect(reply).toMatchObject({ status: 422, code: 'VERIFICATION_REGION_UNAVAILABLE', leaked: false });
    await expect(alert()).toContainText('vùng được phép chưa dùng được');
    check('disabled region ap-east-1: 422 VERIFICATION_REGION_UNAVAILABLE; nothing saved');
    mark('disabled-region');
    await h.moveTo(alert());
    await h.read('Vùng bị tắt nên không lưu');
    const afterFailures = await connections();
    expect(afterFailures.filter((item) => item.kind === 'AWS')).toHaveLength(awsBefore.length);
    check('after every failed registration the Connection list has no new AWS record');

    // 11. Success.
    caption('Bước 11: Nhập khóa hợp lệ và hai vùng được phép; Kiểm tra và lưu xác minh danh tính, từng vùng và quyền cấp phát chỉ bằng lời gọi đọc');
    const connectionName = `AWS lab ${randomBytes(2).toString('hex')}`;
    const expectedKey = connectionName.toLowerCase().replace(/[^a-z0-9]+/g, '-');
    await fillAws({ name: connectionName, key: KEYS.success, secret: successSecret, regions: 'us-east-1, eu-west-1' });
    await assertNoSecretLeaks(page, allSecrets, { allow: ['input[type=password]'] });
    check('before saving: the key appears only inside the two masked fields');
    mark('valid-key-entered');
    await h.pause(SHORT_READ);
    reply = await submitAndWait();
    expect(reply).toMatchObject({ status: 201, leaked: false });
    const result = page.getByLabel('Kết quả xác minh AWS');
    await expect(result).toContainText('Đã xác minh (STS)');
    await expect(result).toContainText('Tài khoản 123456789012');
    await expect(result).toContainText('Vùng us-east-1, eu-west-1');
    await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký tài khoản AWS' })).toContainText('chưa có tài nguyên nào được tạo');
    await expect(keyField()).toHaveValue('');
    await expect(secretField()).toHaveValue('');
    await assertNoSecretLeaks(page, allSecrets);
    check('success: 201, safe result panel (account, principal, regions, verified identity/regions/capabilities), key fields cleared, no key outside any field');
    mark('registered');
    await h.moveTo(result);
    await h.read('Đăng ký thành công; kết quả chỉ có thông tin an toàn');

    // 12. List entry.
    caption('Bước 12: Danh sách kết nối hiển thị tài khoản, các vùng và bằng chứng xác minh thật; không có khóa hay tham chiếu bí mật');
    const row = page.getByRole('table', { name: 'Kết nối đã đăng ký' }).locator('tbody tr').filter({ has: page.getByText(connectionName, { exact: true }) });
    await expect(row).toContainText('us-east-1, eu-west-1');
    await expect(row).toContainText('123456789012');
    await expect(row).toContainText('Sẵn sàng');
    await expect(row).toContainText('Đã xác minh khi đăng ký');
    await h.moveTo(row);
    mark('listed');
    await h.read('Kết nối nằm trong danh sách với bằng chứng xác minh');
    const listed = (await connections()).find((item) => item.key === expectedKey);
    expect(listed).toMatchObject({ kind: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'READY' });
    expect(listed.config).toMatchObject({ accountId: '123456789012', region: 'us-east-1', allowedRegions: ['us-east-1', 'eu-west-1'] });
    expect(listed.verification).toMatchObject({ identity: 'VERIFIED', regionCheck: 'VERIFIED', capabilityCheck: 'VERIFIED' });
    const listText = JSON.stringify(listed);
    for (const value of allSecrets) expect(listText).not.toContain(value);
    expect(listText).not.toMatch(/secretRef|kv2:/);
    check('GET /connections: READY AWS_ACCESS_KEY with account, allowedRegions and three VERIFIED checks; no key, no secretRef');
    const defaultAfter = (await (await page.request.get(`${baseURL}/api/v1/application-connections`)).json()).defaultConnectionKey;
    expect(defaultAfter).toBe(defaultBefore);
    check(`registration did not change the default connection exposed by /application-connections (${defaultBefore || 'none exposed'}); the Organization default itself is asserted by the Go tests`);

    // 13. Reload: persisted state, still no key.
    caption('Bước 13: Tải lại trang; kết nối vẫn còn trong cơ sở dữ liệu và biểu mẫu trống, không đọc lại được khóa');
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Kết nối', level: 1, exact: true })).toBeVisible();
    await expect(page.getByRole('table', { name: 'Kết nối đã đăng ký' }).locator('tbody tr').filter({ has: page.getByText(connectionName, { exact: true }) })).toContainText('Sẵn sàng');
    await assertNoSecretLeaks(page, allSecrets);
    check('after reload the record is listed READY and the page holds no key material');
    mark('reloaded');
    await h.pause(SHORT_READ);

    caption('Bước 14: Đăng xuất; đây là bằng chứng giả lập AWS cục bộ, chưa phải xác minh AWS thật');
    await h.click(page.getByRole('button', { name: 'Đăng xuất', exact: true }));
    await expect(page.getByLabel('Tên đăng nhập', { exact: true })).toBeVisible();
    mark('signed-out');
    await h.pause(3000);

    writeFileSync(`${evidenceDir}/aws-registration-replies.json`, JSON.stringify({ fake: true, replies }, null, 2));
    writeFileSync(`${evidenceDir}/connection.json`, JSON.stringify({ key: expectedKey, name: connectionName }, null, 2));
    if (process.env.ORCH_I0406_INJECT_FAILURE === 'after-success') await fail('injected failure after success');
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
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'DISPLAY']) {
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
