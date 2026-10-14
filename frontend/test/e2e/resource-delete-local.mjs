// I04-08 headed recording: a Platform Engineer deletes an UNREFERENCED resource
// with an opt-in dependency, a Developer is denied. Invoked by
// backend/test/integration/resource-delete-local.sh, which serves the Console from
// backend/test/integration/resdelserver:
//   REAL   Web Console bundle, product HTTP handlers and services, the REAL
//          tfexec cleanup engine and encrypted state backend driving the pinned
//          terraform 1.9.8 binary, disposable PostgreSQL (Console and State
//          logins), run-owned Vault KV v2 for Connection credentials.
//   FAKE   the resources themselves: terraform_data objects of one hand-built
//          module in local state (no provider, cloud or cluster), the provider
//          binder and readiness check, and the topology, which the launcher
//          arranges before the browser starts because this scenario does not
//          deploy Workloads. One destroy-time local-exec fails on purpose (the
//          partial-failure step). A pass is NOT live proof for V2-A01/A02/A03.
// Every product step is a UI action. API reads only assert results; the three
// API mutation calls (Developer 403, wrong-name 400) are denial assertions that
// must have no effect, and the stale preview is injected by an administrator
// SQL action (touch-environment) while the dialog is open.
//   node resource-delete-local.mjs --check I04-08
//   node resource-delete-local.mjs --scenario I04-08 --headed --captions vi --evidence DIR
// Marks, captions, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { appendFileSync, readFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { launchHeaded, screenSize, startRecording, stopRecording, x11Input, nativeSelect } from './video.mjs';

const SHORT_READ = 2000;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, typed into a masked field
const FORBIDDEN_IN_PAGE = [/AKIA/, /vault:\/\//i, /tfstate/i, /kubeconfig/i, /secretRef/i, /\/var\/lib/, /\.terraform/, /recoveryPath/i];
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export const SCENARIOS = {
  async 'I04-08'({ page, h, caption, mark, check, baseURL, evidenceDir, scenarioFile }) {
    const fixture = JSON.parse(readFileSync(scenarioFile, 'utf8'));
    const ids = fixture.resources;
    const names = fixture.descriptors;
    const appId = fixture.applicationId;
    const scope = `/applications/${appId}/environments/staging`;
    const apiGet = async (path) => { const r = await page.request.get(`${baseURL}/api/v1${path}`); return { status: r.status(), body: await r.json().catch(() => ({})) }; };
    const apiPost = async (path, body) => { const r = await page.request.post(`${baseURL}/api/v1${path}`, { data: body }); return { status: r.status(), body: await r.json().catch(() => ({})) }; };
    const statusOf = async (key) => (await apiGet(`${scope}/resources/${ids[key]}`)).body.resource?.status;
    const noCleanup = async (key) => expect((await apiGet(`${scope}/resources/${ids[key]}`)).body.resource?.cleanup).toBeUndefined();
    const dialog = () => page.getByRole('dialog');
    const tick = (key) => dialog().getByRole('checkbox', { name: new RegExp(names[key].replace(/[.#]/g, '\\$&')) });
    const confirmButton = () => dialog().getByRole('button', { name: 'Xác nhận xóa vĩnh viễn', exact: true });
    const nameField = () => dialog().getByLabel('Nhập lại tên tài nguyên');
    const resourcesPanel = () => page.getByRole('region', { name: 'Tài nguyên của môi trường' });
    const progress = () => page.getByTestId('cleanup-progress');

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
    async function openApplication() {
      await h.click(page.getByRole('button', { name: /Payments/ }).first());
      await expect(page.getByRole('heading', { name: 'Payments', level: 1 })).toBeVisible();
      await expect(resourcesPanel()).toBeVisible();
    }
    async function openResource(key) {
      const entry = resourcesPanel().locator('button.deployment-entry', { hasText: names[key] });
      await h.click(entry);
      await expect(page.getByTestId('resource-name')).toHaveText(names[key]);
    }
    async function backToApplication() {
      await h.click(page.getByRole('button', { name: /^← Payments/ }));
      await expect(resourcesPanel()).toBeVisible();
    }
    async function pageIsSafe(label) {
      const text = await page.locator('body').innerText();
      for (const pattern of FORBIDDEN_IN_PAGE) expect(text, `${label}: ${pattern}`).not.toMatch(pattern);
    }

    // ---- Developer: reads, no delete ----------------------------------------
    caption('Bước 1 (FAKE nền): tài nguyên là đối tượng terraform_data do bộ khởi tạo sắp sẵn; Console, máy chủ, PostgreSQL và bộ thực thi Terraform là thật. Nhà phát triển đăng nhập');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signIn('developer');
    mark('developer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 2: Mở ứng dụng Payments (staging); danh sách Tài nguyên liệt kê cả tài nguyên không còn được Workload nào dùng (UNREFERENCED)');
    await openApplication();
    for (const key of ['root1', 'ordersNet', 'sharedNet', 'sessions', 'liveCache', 'root2', 'auditNet']) await expect(resourcesPanel()).toContainText(names[key]);
    await expect(resourcesPanel().locator('button.deployment-entry')).toHaveCount(7);
    check('Environment navigation lists all 7 resources including UNREFERENCED ones (no Workload references them)');
    mark('developer-resource-list');
    await h.moveTo(resourcesPanel());
    await h.read('Danh sách tài nguyên của môi trường staging');

    caption('Bước 3: Nhà phát triển mở chi tiết tài nguyên gốc: đọc được siêu dữ liệu an toàn, nhưng không có nút xóa');
    await openResource('root1');
    await expect(page.getByText('Chỉ Kỹ sư nền tảng hoặc Quản trị viên được xóa tài nguyên.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true })).toHaveCount(0);
    await expect(page.getByTestId('original-target')).toBeVisible();
    await pageIsSafe('developer detail');
    check('Developer reads the resource detail (original target, module, consumers) and gets no delete action');
    mark('developer-detail-no-delete');
    await h.moveTo(page.getByTestId('original-target'));
    await h.read('Nhà phát triển không có quyền xóa');

    // Direct API denials: both mutation endpoints answer 403 before any effect; foreign scope is 404.
    const devPreview = await apiPost(`${scope}/resources/${ids.root1}/deletion-preview`, {});
    const devDelete = await apiPost(`${scope}/resources/${ids.root1}/deletions`, { token: 'x', confirmationName: names.root1, selectedResourceIds: [ids.root1] });
    expect([devPreview.status, devDelete.status]).toEqual([403, 403]);
    expect(JSON.stringify([devPreview.body, devDelete.body])).not.toContain(names.root1);
    const foreignEnv = await apiGet(`/applications/${appId}/environments/production/resources/${ids.root1}`);
    const unknownId = await apiGet(`${scope}/resources/00000000-0000-4000-8000-000000000000`);
    expect([foreignEnv.status, unknownId.status]).toEqual([404, 404]);
    expect(await statusOf('root1')).toBe('UNREFERENCED');
    await noCleanup('root1');
    check('API (denial assertions, no effect): Developer POST deletion-preview and deletions -> 403 without echoing the name; the resource under another Environment and an unknown id -> 404; nothing changed');
    mark('developer-api-denied');
    await signOut();

    // ---- Platform Engineer: preview, cancel, selection -----------------------
    caption('Bước 4: Kỹ sư nền tảng đăng nhập, mở Payments và chi tiết tài nguyên gốc postgres shared.orders-db');
    await signIn('platform-engineer');
    await openApplication();
    await openResource('root1');
    await expect(page.getByTestId('delete-blockers')).toHaveCount(0);
    await h.pause(SHORT_READ);

    caption('Bước 5: Bấm “Xóa tài nguyên”: hộp thoại xem trước. Chỉ tài nguyên gốc được chọn sẵn; dependency là tùy chọn, nền tảng không nằm trong phạm vi');
    await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
    await expect(dialog()).toBeVisible();
    await expect(page.getByTestId('root-name')).toHaveText(names.root1);
    await expect(dialog().getByRole('checkbox', { checked: true })).toHaveCount(1);
    await expect(tick('ordersNet')).not.toBeChecked();
    await expect(tick('sessions')).not.toBeChecked();
    await expect(tick('sharedNet')).toBeDisabled();
    await expect(dialog()).toContainText('Sẽ xóa 1 tài nguyên');
    await expect(confirmButton()).toBeDisabled();
    check('dialog: only the root is selected by default; ORDERS-NET and SESSIONS are unticked opt-ins; SHARED-NET is locked; confirm is disabled');
    mark('dialog-root-only');
    await h.read('Mặc định chỉ chọn tài nguyên gốc');

    caption('Bước 6: SHARED-NET bị khóa vì tài nguyên khác (LIVE-CACHE, đang sẵn sàng, ngoài tập xóa) vẫn dùng nó; lý do hiển thị rõ');
    await expect(dialog()).toContainText(names.liveCache);
    await expect(dialog()).toContainText('Không thể chọn');
    await h.moveTo(dialog().getByText(/Không thể chọn/).first());
    check('SHARED-NET shows why it is locked (consumer LIVE-CACHE outside the selected set)');
    mark('dialog-locked-dependency');
    await h.read('Dependency còn bên dùng không chọn được');

    caption('Bước 7: Hủy: không có yêu cầu nào được gửi, tài nguyên giữ nguyên trạng thái');
    await h.click(dialog().getByRole('button', { name: 'Hủy', exact: true }));
    await expect(dialog()).toHaveCount(0);
    expect(await statusOf('root1')).toBe('UNREFERENCED');
    await noCleanup('root1');
    check('cancel: no cleanup operation exists and the resource is still UNREFERENCED');
    mark('dialog-cancelled');
    await h.pause(SHORT_READ);

    caption('Bước 8: Mở lại, chỉ tick ORDERS-NET (dependency hợp lệ) và để SESSIONS không chọn. Gõ sai tên (đổi chữ hoa) thì nút xác nhận vẫn bị khóa');
    await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
    await expect(dialog()).toBeVisible();
    await h.click(tick('ordersNet'));
    await expect(tick('ordersNet')).toBeChecked();
    await expect(tick('sessions')).not.toBeChecked();
    await expect(dialog()).toContainText('Sẽ xóa 2 tài nguyên');
    await h.type(nameField(), names.root1.toUpperCase(), { delay: 25 });
    await expect(confirmButton()).toBeDisabled();
    check('selected set = ROOT + ORDERS-NET; SESSIONS stays unticked; a wrong-case name keeps confirm disabled (exact match, no case folding)');
    mark('dialog-wrong-name');
    await h.read('Tên phải khớp từng ký tự');
    // The server also rejects it: a wrong name is a 400 with no effect.
    const wrongPreview = await apiPost(`${scope}/resources/${ids.root1}/deletion-preview`, {});
    const wrong = await apiPost(`${scope}/resources/${ids.root1}/deletions`, { token: wrongPreview.body.preview?.token, confirmationName: names.root1.toUpperCase(), selectedResourceIds: [ids.root1, ids.ordersNet] });
    expect(wrong.status).toBe(400);
    expect(wrong.body.code).toBe('RESOURCE_CONFIRMATION_MISMATCH');
    expect(JSON.stringify(wrong.body)).not.toContain(names.root1);
    await noCleanup('root1');
    check('API (denial assertion): the same wrong name -> 400 RESOURCE_CONFIRMATION_MISMATCH, name not echoed, nothing admitted');

    caption('Bước 9: Gõ đúng tên; trong lúc hộp thoại mở, quản trị viên đổi cấu hình môi trường (SQL) nên bản xem trước cũ không còn hợp lệ');
    await h.type(nameField(), names.root1, { delay: 25, replace: true });
    await expect(confirmButton()).toBeEnabled();
    execFileSync(process.env.ORCH_E2E_RESDEL, ['touch-environment', '-private-dir', process.env.ORCH_E2E_PRIVATE_DIR, '-application', appId, '-environment', 'staging'], { encoding: 'utf8' });
    check('administrator SQL (touch-environment) bumped the Environment draft version while the preview was open');
    mark('stale-injected');
    await h.click(confirmButton());
    await expect(dialog().getByRole('alert')).toContainText('Hãy xem lại phạm vi mới');
    await expect(confirmButton()).toHaveCount(0);
    expect(await statusOf('root1')).toBe('UNREFERENCED');
    await noCleanup('root1');
    check('stale preview: the server answered 409, the dialog asks for a new review, nothing was admitted or retried automatically, the old name/token are not reusable');
    mark('stale-409');
    await h.moveTo(dialog().getByRole('alert'));
    await h.read('Bản xem trước cũ bị từ chối, không tự gửi lại');

    caption('Bước 10: Xem lại phạm vi: bản xem trước mới được tải, tên đã nhập bị xóa; chỉ các lựa chọn vẫn hợp lệ được giữ, không tự thêm mục nào');
    await h.click(dialog().getByRole('button', { name: 'Xem lại phạm vi', exact: true }));
    await expect(nameField()).toBeVisible();
    await expect(nameField()).toHaveValue('');
    await expect(tick('ordersNet')).toBeChecked();
    await expect(tick('sessions')).not.toBeChecked();
    await expect(dialog()).toContainText('Sẽ xóa 2 tài nguyên');
    check('after review: new preview, typed name cleared, ORDERS-NET kept, SESSIONS still not selected (nothing added automatically)');
    mark('stale-reviewed');
    await h.read('Phạm vi mới được xem lại');

    caption('Bước 11: Gõ lại đúng tên và xác nhận xóa vĩnh viễn: yêu cầu được ghi nhận rồi tiến trình hiện từ dữ liệu đã lưu');
    await h.type(nameField(), names.root1, { delay: 25 });
    await expect(confirmButton()).toBeEnabled();
    mark('before-confirm');
    await h.click(confirmButton());
    await expect(dialog()).toHaveCount(0);
    await expect(page.getByText('Yêu cầu xóa đã được ghi nhận.')).toBeVisible();
    await expect(progress()).toBeVisible();
    mark('admitted');
    await h.moveTo(progress());
    await h.read('Tiến trình xóa lấy từ dữ liệu đã lưu');
    await expect(progress()).toContainText('Tất cả tài nguyên đã chọn đã được xác minh là không còn', { timeout: 120_000 });
    await expect(progress()).toContainText('Hoàn tất');
    await expect(progress().locator('tbody tr')).toHaveCount(2);
    await expect(progress()).toContainText(names.root1);
    await expect(progress()).toContainText(names.ordersNet);
    await expect(progress()).not.toContainText(names.sessions);
    check('cleanup SUCCEEDED: the success banner appeared only after both confirmed resources were verified absent; the table lists exactly ROOT and ORDERS-NET');
    mark('cleanup-succeeded');
    await pageIsSafe('cleanup progress');
    await h.moveTo(progress());
    await h.read('Đã xác minh cả hai tài nguyên không còn');

    // Retention: only the confirmed IDs changed.
    const afterFirst = Object.fromEntries(await Promise.all(Object.keys(ids).map(async (key) => [key, await statusOf(key)])));
    expect(afterFirst).toMatchObject({ root1: 'DESTROYED', ordersNet: 'DESTROYED', sessions: 'UNREFERENCED', sharedNet: 'UNREFERENCED', liveCache: 'READY', root2: 'UNREFERENCED', auditNet: 'UNREFERENCED' });
    check('retention (API read): ROOT and ORDERS-NET DESTROYED; SESSIONS, SHARED-NET, AUDIT-NET, ROOT2 UNREFERENCED and LIVE-CACHE READY are untouched');
    const operation = (await apiGet(`${scope}/resources/${ids.root1}`)).body.resource.cleanup;
    expect(operation.status).toBe('SUCCEEDED');
    expect(operation.operationId).toMatch(UUID);

    caption('Bước 12: Tải lại trang: trạng thái xóa vẫn lấy từ dữ liệu đã lưu; tài nguyên đã xóa còn trong lịch sử và không thể xóa lần nữa');
    await page.reload();
    await expect(page.getByTestId('resource-name')).toHaveText(names.root1);
    await expect(page.getByText('Thao tác xóa gần nhất: Hoàn tất.')).toBeVisible();
    await expect(page.getByTestId('delete-blockers')).toContainText('đã được xóa và xác minh');
    await expect(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true })).toBeDisabled();
    check('reopen: the DESTROYED resource is still readable with its last cleanup, and cannot be deleted again');
    mark('reopened');
    await h.read('Lịch sử được giữ lại');

    // ---- Partial failure -------------------------------------------------------
    caption('Bước 13: Xóa tài nguyên thứ hai (shared.audit-db) cùng dependency AUDIT-NET. Việc hủy của audit-db được cố ý làm thất bại (FAKE) để xem kết quả một phần');
    await backToApplication();
    await openResource('root2');
    await h.click(page.getByRole('button', { name: 'Xóa tài nguyên', exact: true }));
    await expect(dialog()).toBeVisible();
    await h.click(tick('auditNet'));
    await expect(tick('auditNet')).toBeChecked();
    await h.type(nameField(), names.root2, { delay: 25 });
    mark('second-dialog-ready');
    await h.click(confirmButton());
    await expect(dialog()).toHaveCount(0);
    await expect(progress()).toBeVisible();
    await expect(progress()).toContainText('Thất bại', { timeout: 120_000 });
    await expect(progress()).toContainText('Chưa xóa hết tài nguyên đã chọn.');
    await expect(progress()).toContainText('Chưa xóa hết — còn lại');
    await expect(progress()).toContainText('Được giữ lại (không xóa)');
    await expect(progress()).toContainText('terraform_data.');
    await expect(progress().getByText('Tất cả tài nguyên đã chọn đã được xác minh là không còn')).toHaveCount(0);
    check('partial failure: FAILED, the success banner is absent, the remaining object ids are listed, and AUDIT-NET is RETAINED because its consumer failed');
    mark('partial-failure');
    await pageIsSafe('partial failure');
    await h.moveTo(progress());
    await h.read('Thất bại một phần: còn đối tượng, dependency được giữ lại');
    const afterSecond = Object.fromEntries(await Promise.all(Object.keys(ids).map(async (key) => [key, await statusOf(key)])));
    expect(afterSecond.auditNet).toBe('UNREFERENCED');
    expect(afterSecond.root2).not.toBe('DESTROYED');
    expect(afterSecond.sessions).toBe('UNREFERENCED');
    expect(afterSecond.liveCache).toBe('READY');
    check(`retention after the failure (API read): AUDIT-NET UNREFERENCED, ROOT2 ${afterSecond.root2} (not DESTROYED), SESSIONS UNREFERENCED, LIVE-CACHE READY`);

    caption('Bước 14: Đăng xuất. Kết thúc kịch bản: hạ tầng là đối tượng giả, không có gì trên đám mây hay cluster bị thay đổi');
    await signOut();
    mark('signed-out');
    await page.waitForTimeout(7000); // capture can start late under load: the recording must clearly outlast the last mark

    writeFileSync(`${evidenceDir}/outcome.json`, JSON.stringify({ afterFirst, afterSecond, firstOperation: operation.operationId }, null, 2));
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
  for (const name of ['ORCH_E2E_URL', 'ORCH_E2E_RUN_ID', 'ORCH_E2E_EVIDENCE_DIR', 'ORCH_E2E_XDOTOOL', 'ORCH_E2E_RESDEL', 'ORCH_E2E_PRIVATE_DIR', 'DISPLAY']) {
    if (!env[name]) throw new Error(`${name} is required`);
  }
  const baseURL = env.ORCH_E2E_URL;
  const evidenceDir = env.ORCH_E2E_EVIDENCE_DIR;
  const scenarioFile = `${evidenceDir}/scenario.json`;
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
    const x11 = x11Input(env.ORCH_E2E_XDOTOOL);
    const h = createHuman(page, { selectNative: nativeSelect(x11) });
    await page.goto(`${baseURL}/ui/sign-in`);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(1500);
    x11.focusBrowser();
    recorder = await startRecording({ display: env.DISPLAY, width, height, videoPath });
    startedAt = recorder.spawnedAtMs;
    await page.waitForTimeout(1500);
    const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
    await scenario({ page, h, x11, caption: captions.caption, mark, check, baseURL, evidenceDir, scenarioFile });
    result = 'PASS';
    console.log(`PASS: scenario ${args.scenario}`);
  } catch (error) {
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
