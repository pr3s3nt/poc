// Scenario runner for the Web Console refactor (refactor.md). Invoked by
// backend/test/integration/refactor-ui-local.sh against a fake-adapter backend
// with temporary JSON state. Registry keys are task IDs (Txx); a task that
// has no scenario here is rejected before any process starts:
//   node refactor-local.mjs --check <Txx>
// Every product step is a UI action; API reads only assert results. Marks,
// captions, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { appendFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { assertTablesComplete, locators } from './locators.mjs';
import { launchHeaded, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const SHORT_READ = 2000;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, never shown: the field is masked

// Registry: scenario id -> async ({ page, h, ui, caption, mark, check, baseURL, fail }).
export const SCENARIOS = {
  async T01({ page, h, ui, caption, mark, check, baseURL, fail }) {
    assertTablesComplete();
    check('locator tables: every entry has a Vietnamese and a current label');

    caption('Bước 1: Mở trang đăng nhập Web Console; ô mật khẩu luôn bị che');
    await h.showCursor();
    await expectVisibleCursor(page);
    await expect(ui('signIn', 'username')).toBeVisible();
    await expect(ui('signIn', 'password')).toHaveAttribute('type', 'password');
    check('sign-in page: username field visible, password field type=password');
    mark('sign-in-page');
    await h.read('Mở trang đăng nhập Web Console; ô mật khẩu luôn bị che');

    caption('Bước 2: Kỹ sư nền tảng nhập tài khoản thử nghiệm cục bộ rồi bấm đăng nhập');
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await expect(ui('signIn', 'password')).toHaveAttribute('type', 'password');
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/applications`);
    const cookie = (await page.context().cookies()).find((item) => item.name === 'orchestrator_session');
    expect(cookie?.httpOnly, 'session cookie is HttpOnly').toBe(true);
    check('sign-in: landed on /ui/applications, session cookie HttpOnly');
    mark('applications');
    await h.pause(SHORT_READ);

    if (process.env.ORCH_REFACTOR_INJECT_FAILURE === 'after-sign-in') {
      caption('Kiểm tra đường lỗi có chủ đích: assertion sau đây cố ý thất bại');
      await h.pause(SHORT_READ);
      await fail('injected failure after sign-in');
    }

    const nav = ui('shell', 'nav');
    await expect(nav).toBeVisible();
    const pages = [
      ['navResourceTypes', 'resourceTypes', 'resource-types', 'Bước 3: Mở mục Loại tài nguyên trên thanh điều hướng'],
      ['navResourceDefinitions', 'resourceDefinitions', 'resource-definitions', 'Bước 4: Mở mục Cấu hình tài nguyên'],
      ['navConnections', 'connections', 'connections', 'Bước 5: Mở mục Kết nối'],
      ['navSecretStores', 'secretStores', 'secret-stores', 'Bước 6: Mở mục Kho bí mật'],
    ];
    for (const [link, group, slug, text] of pages) {
      caption(text);
      await h.click(ui('shell', link));
      await expect(ui(group, 'heading')).toBeVisible();
      await expect(page).toHaveURL(new RegExp(`/ui/platform/${slug}$`));
      check(`navigation: ${slug} heading visible and URL matches`);
      mark(slug);
      await h.read(text);
    }

    caption('Bước 7: Quay lại danh sách ứng dụng bằng thanh điều hướng');
    await h.click(ui('shell', 'navApplications'));
    await expect(ui('applications', 'heading')).toBeVisible();
    mark('applications-again');
    await h.pause(SHORT_READ);

    caption('Bước 8: Đăng xuất; phiên bị thu hồi và API trả 401');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/sign-in`);
    expect((await page.request.get(`${baseURL}/api/v1/applications`)).status()).toBe(401);
    check('sign-out: sign-in page shown, GET /api/v1/applications returns 401');
    mark('signed-out');
    await h.pause(3000);
  },

  // T02: Cấu hình tài nguyên form. Platform Engineer, fake adapters, seeded
  // READY connections (internal-cluster KUBERNETES, aws-account AWS).
  async T02({ page, h, ui, caption, mark, check, baseURL }) {
    const definitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const formAlert = () => page.locator('form .form-error[role="alert"]');
    const warning = () => page.getByRole('note');

    caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    mark('applications');

    caption('Bước 2: Mở trang Cấu hình tài nguyên; danh sách không có cấu hình workload');
    await h.click(ui('shell', 'navResourceDefinitions'));
    await expect(ui('resourceDefinitions', 'heading')).toBeVisible();
    await expect(page.locator('.catalog-entry').filter({ hasText: 'vpc-aws' })).toContainText('vpc · Dùng chung · Tạo trên AWS (Terraform)');
    await expect(page.locator('.catalog-entry').filter({ hasText: /workload/ })).toHaveCount(0);
    check('page: Vietnamese list, seeded vpc-aws shown, no workload entry');
    mark('definitions-page');
    await h.read('Danh sách cấu hình, không có workload');

    caption('Bước 3: Nhập ID và chọn Loại tài nguyên vpc; cách tạo, module và phạm vi tự điền, bị khóa');
    await h.type(ui('resourceDefinitions', 'id'), 'vpc-t02');
    await h.choose(ui('resourceDefinitions', 'type'), 'vpc');
    await expect(ui('resourceDefinitions', 'driver')).toHaveValue('terraform');
    await expect(ui('resourceDefinitions', 'driver')).toBeDisabled();
    await expect(ui('resourceDefinitions', 'scope')).toHaveValue('aws-eks');
    await expect(ui('resourceDefinitions', 'scope')).toBeDisabled();
    await expect(page.locator('code', { hasText: /^vpc$/ })).toBeVisible();
    const awsValues = await ui('resourceDefinitions', 'awsConnection').locator('option').evaluateAll((options) => options.map((option) => option.value));
    expect(awsValues, 'only READY AWS connections').toEqual(['', 'aws-account']);
    check('vpc: terraform driver, aws-eks scope locked, module vpc, AWS dropdown lists only aws-account');
    mark('vpc-controls');
    await h.moveTo(ui('resourceDefinitions', 'driver'));
    await h.read('Terraform, aws-eks và module vpc được tự điền');

    caption('Bước 4: Đổi sang k8s-namespace, chọn Dùng chung để thấy cảnh báo trùng mức ưu tiên');
    await h.choose(ui('resourceDefinitions', 'type'), 'k8s-namespace');
    await expect(ui('resourceDefinitions', 'driver')).toHaveValue('kubernetes');
    await h.choose(ui('resourceDefinitions', 'scope'), 'Dùng chung');
    await expect(warning()).toContainText('trùng mức ưu tiên');
    await expect(ui('resourceDefinitions', 'id')).toHaveValue('vpc-t02');
    check('k8s-namespace: kubernetes driver, shared scope warning shown, ID kept');
    mark('shared-warning');
    await h.moveTo(warning());
    await h.read('Cảnh báo cấu hình dùng chung có thể trùng mức ưu tiên');

    caption('Bước 5: Mở phần nâng cao; Kết nối riêng cho cluster mặc định để trống');
    await expect(ui('resourceDefinitions', 'variables')).not.toBeVisible();
    await expect(ui('resourceDefinitions', 'kubeConnection')).not.toBeVisible();
    await h.click(ui('resourceDefinitions', 'showAdvanced'));
    await expect(ui('resourceDefinitions', 'variables')).toBeVisible();
    await expect(ui('resourceDefinitions', 'kubeConnection')).toBeVisible();
    await expect(ui('resourceDefinitions', 'kubeConnection')).toHaveValue('');
    await h.choose(ui('resourceDefinitions', 'kubeConnection'), 'internal-cluster');
    await expect(ui('resourceDefinitions', 'kubeConnection')).toHaveValue('internal-cluster');
    await h.choose(ui('resourceDefinitions', 'kubeConnection'), 'Dùng Kết nối của Môi trường (mặc định)');
    await expect(ui('resourceDefinitions', 'kubeConnection')).toHaveValue('');
    await h.type(ui('resourceDefinitions', 'variables'), '{"name":"keep-me"}', { replace: true, delay: 30 });
    await h.click(ui('resourceDefinitions', 'hideAdvanced'));
    await expect(ui('resourceDefinitions', 'variables')).not.toBeVisible();
    await expect(ui('resourceDefinitions', 'kubeConnection')).not.toBeVisible();
    await h.click(ui('resourceDefinitions', 'showAdvanced'));
    await expect(ui('resourceDefinitions', 'variables')).toBeVisible();
    await expect(ui('resourceDefinitions', 'variables')).toHaveValue('{"name":"keep-me"}');
    await expect(ui('resourceDefinitions', 'kubeConnection')).toBeVisible();
    check('advanced: hidden until opened, collapse/show toggles visibility and keeps values');
    check('advanced: Kubernetes connection override default empty, explicit choice possible');
    mark('kube-override');
    await h.read('Kết nối riêng chỉ là tùy chọn nâng cao');

    caption('Bước 6: Quay lại vpc, chọn Kết nối AWS và nhập tham số sai kiểu; máy chủ từ chối và form được giữ');
    await h.choose(ui('resourceDefinitions', 'type'), 'vpc');
    await h.choose(ui('resourceDefinitions', 'awsConnection'), 'aws-account');
    const badVars = '{"name":"${context.infra.resourceName}","cidr":2}';
    await h.type(ui('resourceDefinitions', 'variables'), badVars, { replace: true, delay: 30 });
    await h.click(ui('resourceDefinitions', 'submit'));
    await expect(formAlert()).toContainText('Máy chủ từ chối cấu hình này');
    await expect(ui('resourceDefinitions', 'id')).toHaveValue('vpc-t02');
    await expect(ui('resourceDefinitions', 'variables')).toHaveValue(badVars);
    expect((await definitions()).some((item) => item.key === 'vpc-t02')).toBe(false);
    check('safe failure: Vietnamese message, form kept, nothing registered');
    mark('bad-json-rejected');
    await h.moveTo(formAlert());
    await h.read('Máy chủ từ chối; dữ liệu đã nhập vẫn còn');

    caption('Bước 7: Sửa tham số rồi thử ID đã tồn tại vpc-aws; báo trùng ID bằng tiếng Việt');
    const goodVars = '{"name":"${context.infra.resourceName}","cidr":"10.20.0.0/16"}';
    await h.type(ui('resourceDefinitions', 'variables'), goodVars, { replace: true, delay: 30 });
    await h.type(ui('resourceDefinitions', 'id'), 'vpc-aws', { replace: true });
    await h.click(ui('resourceDefinitions', 'submit'));
    await expect(formAlert()).toContainText('ID cấu hình đã tồn tại');
    await expect(ui('resourceDefinitions', 'variables')).toHaveValue(goodVars);
    check('duplicate 409: Vietnamese message, form kept');
    mark('duplicate-rejected');
    await h.moveTo(formAlert());
    await h.read('ID đã tồn tại; đổi ID rồi thử lại');

    caption('Bước 8: Đổi ID mới và đăng ký thành công qua giao diện');
    await h.type(ui('resourceDefinitions', 'id'), 'vpc-t02', { replace: true });
    await h.click(ui('resourceDefinitions', 'submit'));
    await expect(page.getByRole('status')).toHaveText('Đã đăng ký cấu hình tài nguyên vpc-t02.');
    const entry = page.locator('.catalog-entry').filter({ has: page.getByText('vpc-t02', { exact: true }) });
    await expect(entry).toContainText('vpc · AWS · Tạo trên AWS (Terraform) · 1 điều kiện áp dụng');
    const vpc = (await definitions()).find((item) => item.key === 'vpc-t02');
    expect(vpc).toMatchObject({ resourceType: 'vpc', executionProfile: 'aws-eks', driverType: 'terraform', connectionKey: 'aws-account' });
    check('registered vpc-t02: payload aws-eks, terraform, connection aws-account, listed in UI');
    mark('vpc-registered');
    await h.moveTo(entry);
    await h.read('Đã đăng ký vpc-t02');

    caption('Bước 9: Đăng ký k8s-namespace dùng chung; payload gửi phạm vi rỗng và Kết nối trống');
    await h.type(ui('resourceDefinitions', 'id'), 'namespace-t02');
    await h.choose(ui('resourceDefinitions', 'type'), 'k8s-namespace');
    // The earlier Dùng chung choice is kept on purpose; choose both options through the UI.
    await expect(ui('resourceDefinitions', 'scope')).toHaveValue('');
    await h.choose(ui('resourceDefinitions', 'scope'), 'Cluster nội bộ (internal-k8s)');
    await expect(ui('resourceDefinitions', 'scope')).toHaveValue('internal-k8s');
    await h.choose(ui('resourceDefinitions', 'scope'), 'Dùng chung');
    await h.type(ui('resourceDefinitions', 'variables'), '{"name":"t02"}', { replace: true });
    await h.click(ui('resourceDefinitions', 'submit'));
    await expect(page.getByRole('status')).toHaveText('Đã đăng ký cấu hình tài nguyên namespace-t02.');
    const shared = (await definitions()).find((item) => item.key === 'namespace-t02');
    expect(shared).toMatchObject({ resourceType: 'k8s-namespace', driverType: 'kubernetes' });
    // The API omits empty fields: an absent profile/connection is the empty value.
    expect(shared.executionProfile ?? '').toBe('');
    expect(shared.connectionKey ?? '').toBe('');
    check('registered namespace-t02: shared profile "" and empty connection');
    mark('shared-registered');
    await h.pause(SHORT_READ);

    caption('Bước 10: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
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
  context.setDefaultTimeout(20_000);
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
    startedAt = Date.now();
    await page.waitForTimeout(1500);
    const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
    const fail = async (message) => { appendFileSync(assertionsPath, `FAIL ${message}\n`); await expect(message, message).toBe(''); };
    await scenario({ page, h, ui: locators(page), caption: captions.caption, mark, check, baseURL, fail });
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
    // Error text never contains the seeded password: it is typed only into a masked field.
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
