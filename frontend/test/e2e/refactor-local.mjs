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
