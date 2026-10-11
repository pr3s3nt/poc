// Scenario runner for the Web Console refactor (refactor.md). Invoked by
// backend/test/integration/refactor-ui-local.sh against a fake-adapter backend
// with temporary JSON state. Registry keys are task IDs (Txx); a task that
// has no scenario here is rejected before any process starts:
//   node refactor-local.mjs --check <Txx>
// Every product step is a UI action; API reads only assert results. Marks,
// captions, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { LONG_TYPE_DELAY, addWorkload, createApplication, createHuman, expectVisibleCursor, installCursor, previewAndDeploy, putKeys, setEnvironmentConnections } from './human.mjs';
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
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveCount(0);
    check('vpc: terraform driver, aws-eks scope locked, module vpc, no AWS account control (T16: the Environment connection is used)');
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

    caption('Bước 6: Quay lại vpc (không có ô Kết nối AWS) và nhập tham số sai kiểu; máy chủ từ chối và form được giữ');
    await h.choose(ui('resourceDefinitions', 'type'), 'vpc');
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveCount(0);
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
    expect(vpc).toMatchObject({ resourceType: 'vpc', executionProfile: 'aws-eks', driverType: 'terraform' });
    expect(vpc.connectionKey ?? '').toBe('');
    check('registered vpc-t02: payload aws-eks, terraform, empty connection (T16), listed in UI');
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

  // T02B: editor Điều kiện áp dụng. Platform Engineer, fake adapters; seeded
  // application acceptance (environment dev, type development)
  // and definitions namespace/vpc/postgres (internal-k8s postgres is [{}]).
  async T02B({ page, h, ui, caption, mark, check, baseURL }) {
    const definitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const definition = async (key) => (await definitions()).find((item) => item.key === key);
    const editor = page.locator('.criteria-editor');
    const mode = (name) => editor.getByRole('radio', { name });
    const select = (text) => editor.locator('label').filter({ hasText: text }).locator('select');
    const group = (name) => page.getByRole('group', { name });
    const registered = (key) => expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${key}.` })).toBeVisible();
    const APP = 'Acceptance Application (acceptance)';
    const ENV = 'Development (dev) · loại development';

    caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    mark('applications');

    caption('Bước 2: Mở Cấu hình tài nguyên, chọn Loại tài nguyên postgres; chế độ mặc định là Mọi nơi');
    await h.click(ui('shell', 'navResourceDefinitions'));
    await expect(ui('resourceDefinitions', 'heading')).toBeVisible();
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t02b-all');
    await h.choose(ui('resourceDefinitions', 'type'), 'postgres');
    await expect(mode(/Mọi nơi/)).toBeChecked();
    await expect(editor.getByLabel('Loại môi trường', { exact: true })).toHaveCount(0);
    await expect(group('Tóm tắt phạm vi áp dụng')).toContainText('Mọi nơi (không giới hạn)');
    await expect(group('Tóm tắt phạm vi áp dụng')).toContainText('Phạm vi triển khai (executionProfile): Cluster nội bộ');
    check('mode Mọi nơi default: no extra controls, summary shows scope and profile apart from env_type');
    mark('mode-all');
    await h.moveTo(group('Tóm tắt phạm vi áp dụng'));
    await h.read('Mọi nơi: không có ô nhập nào, phạm vi triển khai tách khỏi loại môi trường');

    caption('Bước 3: Cảnh báo nguy cơ chồng lấn với cấu hình postgres cùng phạm vi; không kết luận matching chính xác');
    const overlap = group('Cảnh báo nguy cơ chồng lấn');
    await expect(overlap).toContainText('postgres-internal-statefulset');
    await expect(overlap).not.toContainText('postgres-aws-aurora');
    expect(await overlap.innerText()).not.toMatch(/ambiguous|thắng|winner|mơ hồ/i);
    await expect(overlap).toContainText('chưa phải kết quả matching chính xác');
    check('overlap warning names same type/profile Definition only, no ambiguity/winner claim');
    mark('overlap-warning');
    await h.moveTo(overlap);
    await h.read('Chỉ là cảnh báo nguy cơ chồng lấn, không phải kết quả matching');

    caption('Bước 4: Đăng ký với Mọi nơi; payload gửi [{}]');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('pg-t02b-all');
    expect((await definition('pg-t02b-all')).criteria).toEqual([{}]);
    check('registered pg-t02b-all: criteria [{}]');
    mark('all-registered');
    await h.pause(SHORT_READ);

    caption('Bước 5: Chế độ Theo loại môi trường chỉ hiện một ô; nhập development');
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t02b-env');
    await h.click(mode(/Theo loại môi trường/));
    await expect(editor.getByLabel('Loại môi trường', { exact: true })).toBeVisible();
    await expect(editor.getByLabel('Ứng dụng', { exact: true })).toHaveCount(0);
    await h.type(editor.getByLabel('Loại môi trường', { exact: true }), 'development');
    await expect(group('Tóm tắt phạm vi áp dụng')).toContainText('Loại môi trường (env_type): development');
    await expect(group('Tóm tắt phạm vi áp dụng')).toContainText('không phải Loại môi trường (env_type)');
    mark('mode-env-type');
    await h.read('Loại môi trường là điều kiện riêng, khác phạm vi triển khai');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('pg-t02b-env');
    expect((await definition('pg-t02b-env')).criteria).toEqual([{ env_type: 'development' }]);
    check('registered pg-t02b-env: criteria env_type only');
    mark('env-type-registered');

    caption('Bước 6: Chế độ Theo ứng dụng (+ môi trường): chọn ứng dụng và môi trường thật từ danh sách');
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t02b-app');
    await h.click(mode(/Theo ứng dụng/));
    const appOptions = await select(/^Ứng dụng(?! mẫu)/).locator('option').evaluateAll((options) => options.map((option) => option.value));
    expect(appOptions).toEqual(['', 'acceptance']);
    await expect(select(/^Môi trường \(tùy chọn\)/)).toBeDisabled();
    await h.choose(select(/^Ứng dụng(?! mẫu)/), APP);
    const envOptions = await select(/^Môi trường \(tùy chọn\)/).locator('option').evaluateAll((options) => options.map((option) => option.value));
    expect(envOptions).toEqual(['', 'dev']);
    await h.choose(select(/^Môi trường \(tùy chọn\)/), ENV);
    await expect(group('Tóm tắt phạm vi áp dụng')).toContainText('Ứng dụng (app_id): Acceptance Application (acceptance)');
    await expect(group('Tóm tắt phạm vi áp dụng')).toContainText('Môi trường (env_id): dev');
    check('app/env dropdown: real IDs from GET /api/v1/applications, environment follows the application');
    mark('mode-app-env');
    await h.read('Ứng dụng và môi trường dùng ID thật từ máy chủ');

    caption('Bước 7: Xem trước phạm vi thiếu context; báo cần thêm res_id và Class, không lưu gì');
    const preview = group(/Xem trước phạm vi/);
    await expect(preview).toContainText('Cần thêm context: ứng dụng và môi trường mẫu, ID tài nguyên (res_id), Class');
    await h.choose(select(/^Ứng dụng mẫu/), APP);
    await h.choose(select(/^Môi trường mẫu/), 'Development (dev)');
    await expect(preview).toContainText('Cần thêm context: ID tài nguyên (res_id), Class');
    mark('preview-incomplete');
    await h.moveTo(preview);
    await h.read('Chưa đủ context nên chưa đánh giá điều kiện');
    await h.type(editor.getByLabel('ID tài nguyên mẫu (res_id)'), 'db');
    await h.type(editor.getByLabel('Class mẫu'), 'fast');
    await expect(preview).not.toContainText('Cần thêm context');
    await expect(preview).toContainText('Điều kiện 1: thỏa context mẫu');
    await expect(preview).toContainText('không phải kết quả matching chính xác');
    expect((await definition('pg-t02b-app')) === undefined, 'preview did not register').toBe(true);
    check('preview: incomplete context asks for more, complete context is assistive only, nothing persisted');
    mark('preview-complete');
    await h.read('Đủ context: chỉ là gợi ý ở giao diện, chưa lưu');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('pg-t02b-app');
    expect((await definition('pg-t02b-app')).criteria).toEqual([{ app_id: 'acceptance', env_id: 'dev' }]);
    check('registered pg-t02b-app: criteria app_id + env_id real IDs');
    mark('app-env-registered');

    caption('Bước 8: Tùy chỉnh nâng cao: năm field và nhiều dòng điều kiện');
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t02b-adv');
    await h.click(mode(/Tùy chỉnh nâng cao/));
    for (const label of ['Loại môi trường', 'ID ứng dụng', 'ID môi trường', 'ID tài nguyên', 'Class']) await expect(editor.getByLabel(`Điều kiện 1 ${label}`)).toBeVisible();
    await h.type(editor.getByLabel('Điều kiện 1 Loại môi trường'), 'development');
    await h.type(editor.getByLabel('Điều kiện 1 Class'), 'fast');
    await h.click(editor.getByRole('button', { name: '+ Thêm điều kiện' }));
    await h.type(editor.getByLabel('Điều kiện 2 ID ứng dụng'), 'acceptance-cloud');
    await h.type(editor.getByLabel('Điều kiện 2 ID tài nguyên'), 'db');
    mark('mode-advanced');
    await h.read('Hai dòng điều kiện là lựa chọn thay thế nhau');

    caption('Bước 9: Chuyển sang Mọi nơi bị chặn vì sẽ mất điều kiện nâng cao; chọn Giữ lại');
    await h.click(mode(/Mọi nơi/));
    const confirmGroup = group('Xác nhận thay đổi điều kiện');
    await expect(confirmGroup).toContainText('sẽ bị bỏ');
    await expect(mode(/Tùy chỉnh nâng cao/)).toBeChecked();
    mark('switch-blocked');
    await h.moveTo(confirmGroup);
    await h.read('Không âm thầm mất điều kiện nâng cao');
    await h.click(confirmGroup.getByRole('button', { name: 'Giữ điều kiện nâng cao' }));
    await expect(editor.getByLabel('Điều kiện 2 ID tài nguyên')).toHaveValue('db');
    await expect(confirmGroup).toHaveCount(0);
    check('advanced round-trip: unsafe switch blocked, both rows kept after Giữ điều kiện nâng cao');
    mark('advanced-kept');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('pg-t02b-adv');
    expect((await definition('pg-t02b-adv')).criteria).toEqual([{ env_type: 'development', class: 'fast' }, { app_id: 'acceptance-cloud', res_id: 'db' }]);
    check('registered pg-t02b-adv: two criterion rows, five-field contract');
    mark('advanced-registered');

    caption('Bước 10: Sao chép điều kiện từ cluster-aws-eks; bản sao độc lập, mở ở chế độ nâng cao');
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t02b-copy');
    await h.choose(select(/^Sao chép điều kiện/), 'cluster-aws-eks (k8s-cluster)');
    await h.click(editor.getByRole('button', { name: 'Sao chép điều kiện' }));
    await expect(mode(/Tùy chỉnh nâng cao/)).toBeChecked();
    await expect(editor.getByLabel('Điều kiện 1 Class')).toHaveValue('eks');
    await h.type(editor.getByLabel('Điều kiện 1 Class'), 'eks-copy', { replace: true });
    const source = await definition('cluster-aws-eks');
    expect(source.criteria).toEqual([{ class: 'eks' }]);
    check('copy: criteria copied as independent value, editing the copy leaves the source Definition unchanged');
    mark('copied');
    await h.read('Sửa bản sao không làm đổi cấu hình nguồn');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('pg-t02b-copy');
    expect((await definition('pg-t02b-copy')).criteria).toEqual([{ class: 'eks-copy' }]);
    expect((await definition('cluster-aws-eks')).criteria).toEqual([{ class: 'eks' }]);
    check('registered pg-t02b-copy: copied criteria edited independently');
    mark('copy-registered');
    await h.pause(SHORT_READ);

    caption('Bước 11: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T03: trang Mẫu dựng ứng dụng. Fake adapters + a test-local score-k8s stub
  // whose `generate` always fails (wrapper passes -score-k8s). Phases:
  //   Developer creates two Applications and a workload each (UI);
  //   Platform Engineer registers a template for ONE Application (UI);
  //   Developer runs Preview (pure planning: selection only) and Deploy (the
  //   render phase) on both. Preview never renders (ADR-010); the render
  //   failure is produced by the Deploy path of the real product backend, with
  //   a fake deliverer and the stub binary. Limit: the stub proves the product
  //   path propagates a renderer failure; it is not the real score-k8s output.
  async T03({ page, h, ui, caption, mark, check, baseURL }) {
    const log = process.env.ORCH_E2E_SCORE_K8S_LOG;
    if (!log) throw new Error('ORCH_E2E_SCORE_K8S_LOG is required for T03 (set by refactor-ui-local.sh)');
    const calls = () => (existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).map((line) => line.split('\t')[1]) : []);
    const definitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const editor = page.locator('.criteria-editor');
    const select = (text) => editor.locator('label').filter({ hasText: text }).locator('select');
    const formAlert = () => page.locator('form .form-error[role="alert"]');
    const optionLabel = (locator, value) => locator.locator('option').evaluateAll((options, wanted) => options.find((option) => option.value === wanted)?.textContent, value);
    const workload = { name: 'web', image: 'registry.example/web:t03', bindings: [], port: { name: 'http', port: '8080', targetPort: '8080' } };
    const NATIVE_NAME = 'T03 Native App';
    const TEMPLATE_NAME = 'T03 Template App';
    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await h.click(ui('signIn', 'submit'));
      await expect(ui('applications', 'heading')).toBeVisible();
    };
    const signOut = async () => {
      await h.click(ui('shell', 'signOut'));
      await expect(ui('signIn', 'username')).toBeVisible();
    };
    const openApplication = async (name) => {
      await h.click(page.getByRole('button', { name: new RegExp(name) }));
      await expect(page.getByRole('heading', { name })).toBeVisible();
    };

    caption('Bước 1: Lập trình viên đăng nhập và tạo hai ứng dụng, mỗi ứng dụng một workload');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signInAs('developer');
    const nativeId = await createApplication(h, NATIVE_NAME, 't03-native');
    await addWorkload(h, workload);
    await h.click(ui('shell', 'navApplications'));
    const templateId = await createApplication(h, TEMPLATE_NAME, 't03-template');
    await addWorkload(h, workload);
    check(`developer created applications ${nativeId} and ${templateId}, each with workload web, through the UI`);
    mark('applications-prepared');
    await h.read('Hai ứng dụng đã sẵn sàng, mỗi ứng dụng có workload web');
    await signOut();

    // MOCK (projection of GET /render-bundles): the real backend lists exactly one installed bundle
    // (score-k8s-internal-v1 from the stub). One extra entry makes the list explicit and lets step 4
    // select a bundle the server does not have; POST /resource-definitions stays real and rejects it.
    await page.route('**/api/v1/render-bundles', async (route) => {
      const response = await route.fetch();
      const data = await response.json();
      await route.fulfill({ response, json: { renderBundles: [...data.renderBundles, { id: 'bundle-khong-co', version: '0.0.0', status: 'AVAILABLE' }] } });
    });
    caption('Bước 2: Kỹ sư nền tảng mở trang Mẫu dựng ứng dụng; danh sách trống, có hướng dẫn tiếng Việt');
    await signInAs('platform-engineer');
    await h.click(ui('shell', 'navRenderingTemplates'));
    await expect(ui('renderingTemplates', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/rendering-templates`);
    await expect(page.getByText('Chưa có mẫu dựng ứng dụng nào.')).toBeVisible();
    await expect(page.getByText(/Mẫu là tùy chọn và chỉ dùng cho cluster nội bộ/)).toBeVisible();
    await expect(page.getByText(/không chuyển âm thầm sang renderer mặc định/)).toBeVisible();
    await expect(page.getByText(/Một bundle duy nhất được chọn sẵn; nhiều bundle cần chọn tường minh/)).toBeVisible();
    check('sidebar peer link opens /ui/platform/rendering-templates; empty list, optional/internal-only/no-fallback guidance and bundle-selector note shown');
    mark('templates-page');
    await h.read('Mẫu là tùy chọn, chỉ cho cluster nội bộ; mẫu đã chọn render lỗi sẽ báo lỗi');

    caption('Bước 3: Form không có trường hạ tầng; editor Điều kiện áp dụng là editor chung');
    for (const label of [/Kết nối/, /Terraform/, /module/i, /JSON/, /Quy tắc tạo tài nguyên/, /Loại tài nguyên/, /Cách tạo/]) await expect(page.getByLabel(label)).toHaveCount(0);
    const modes = await editor.getByRole('radio').evaluateAll((radios) => radios.map((radio) => radio.closest('label')?.textContent?.trim().split('\n')[0] ?? ''));
    expect(modes.length, 'four criteria modes').toBe(4);
    check('no Connection/Terraform/module/JSON/type/driver controls; criteria editor shows 4 modes');
    mark('no-infrastructure-fields');
    await h.moveTo(editor);
    await h.read('Chỉ có ID mẫu, ID bundle và Điều kiện áp dụng');

    caption('Bước 4: Nhập bundle không tồn tại; máy chủ từ chối, ID, bundle và điều kiện được giữ');
    await h.type(ui('renderingTemplates', 'id'), 'tpl-web');
    await h.choose(ui('renderingTemplates', 'bundle'), 'bundle-khong-co (phiên bản 0.0.0)');
    await h.click(editor.getByRole('radio', { name: /Theo ứng dụng/ }));
    await h.choose(select(/^Ứng dụng(?! mẫu)/), await optionLabel(select(/^Ứng dụng(?! mẫu)/), templateId));
    await h.choose(select(/^Môi trường \(tùy chọn\)/), await optionLabel(select(/^Môi trường \(tùy chọn\)/), 'staging'));
    await h.click(ui('renderingTemplates', 'submit'));
    await expect(formAlert()).toContainText('Máy chủ từ chối mẫu này');
    await expect(formAlert()).toContainText('bundle có thể không còn khả dụng');
    expect(await formAlert().innerText()).not.toMatch(/bundle-khong-co|unavailable|not installed|invalid/i);
    await expect(ui('renderingTemplates', 'id')).toHaveValue('tpl-web');
    await expect(ui('renderingTemplates', 'bundle')).toHaveValue('bundle-khong-co');
    await expect(select(/^Ứng dụng(?! mẫu)/)).toHaveValue(templateId);
    await expect(select(/^Môi trường \(tùy chọn\)/)).toHaveValue('staging');
    expect((await definitions()).some((item) => item.key === 'tpl-web')).toBe(false);
    check('unavailable bundle: safe Vietnamese error without raw detail, ID/bundle/criteria kept, nothing registered');
    mark('bundle-rejected');
    await h.moveTo(formAlert());
    await h.read('Bundle không có sẵn: dữ liệu đã nhập vẫn còn và chưa lưu gì');

    caption('Bước 5: Nhập đúng bundle score-k8s-internal-v1 rồi đăng ký mẫu cho ứng dụng thứ hai');
    await h.choose(ui('renderingTemplates', 'bundle'), 'score-k8s-internal-v1 (phiên bản 0.15.0)');
    await h.click(ui('renderingTemplates', 'submit'));
    await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký mẫu dựng ứng dụng tpl-web.' })).toBeVisible();
    const entry = page.locator('.catalog-entry').filter({ has: page.getByText('tpl-web', { exact: true }) });
    await expect(entry).toContainText('Bundle: score-k8s-internal-v1 · 1 điều kiện áp dụng');
    await expect(ui('renderingTemplates', 'id')).toHaveValue('');
    await expect(ui('renderingTemplates', 'bundle')).toHaveValue('');
    const stored = (await definitions()).find((item) => item.key === 'tpl-web');
    expect(stored).toMatchObject({ resourceType: 'workload', driverType: 'score-k8s', executionProfile: 'internal-k8s', criteria: [{ app_id: templateId, env_id: 'staging' }] });
    expect(stored.driverInputs.values.variables).toEqual({ render_bundle: 'score-k8s-internal-v1' });
    expect(stored.driverInputs.values.source, 'no source').toBeUndefined();
    expect(stored.connectionKey ?? '', 'no connection').toBe('');
    expect(stored.provision ?? {}, 'no provision').toEqual({});
    check('registered tpl-web: workload/score-k8s/internal-k8s, variables only render_bundle, criteria app+staging, no connection/provision/source, listed with bundle and criteria count');
    mark('template-registered');
    await h.moveTo(entry);
    await h.read('Đã đăng ký tpl-web; danh sách hiện ID, bundle và số điều kiện');

    caption('Bước 6: Đăng ký lại cùng ID; báo trùng ID bằng tiếng Việt và giữ form');
    await h.type(ui('renderingTemplates', 'id'), 'tpl-web');
    await h.choose(ui('renderingTemplates', 'bundle'), 'score-k8s-internal-v1 (phiên bản 0.15.0)');
    await h.click(ui('renderingTemplates', 'submit'));
    await expect(formAlert()).toContainText('ID mẫu đã tồn tại');
    await expect(ui('renderingTemplates', 'id')).toHaveValue('tpl-web');
    await expect(ui('renderingTemplates', 'bundle')).toHaveValue('score-k8s-internal-v1');
    expect((await definitions()).filter((item) => item.key === 'tpl-web')).toHaveLength(1);
    check('duplicate 409: Vietnamese message, form kept, the original template unchanged');
    mark('duplicate-rejected');
    await h.moveTo(formAlert());
    await h.read('ID đã tồn tại; Definition gốc không bị thay thế');
    await signOut();

    caption('Bước 7: Xem trước ứng dụng không có mẫu khớp; dùng renderer mặc định (built-in Kubernetes)');
    await signInAs('developer');
    await openApplication(NATIVE_NAME);
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi' }));
    const preview = page.getByLabel('Xem trước triển khai');
    await expect(preview).toContainText('1 Workload bị ảnh hưởng', { timeout: 60_000 });
    await expect(preview.locator('li').filter({ hasText: /^web / })).toContainText('Kubernetes tích hợp sẵn');
    await expect(preview.locator('li').filter({ hasText: /^web / })).not.toContainText('score-k8s');
    check('Preview, application without matching template: native renderer (built-in Kubernetes), no score-k8s provenance');
    mark('preview-native');
    await h.moveTo(preview);
    await h.read('Không có mẫu khớp thì dùng renderer mặc định');
    expect(calls(), 'Preview never runs the renderer').toEqual([]);
    check('Preview ran no score-k8s call (pure planning)');

    caption('Bước 8: Triển khai ứng dụng này; renderer mặc định thành công, không gọi score-k8s');
    await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này' }));
    const nativeResult = page.getByLabel('Kết quả triển khai');
    await expect(nativeResult).toContainText('Triển khai thành công', { timeout: 120_000 });
    expect(calls(), 'native deploy does not call the template renderer').toEqual([]);
    check('Deploy of the native application succeeded (fake deliverer) with zero score-k8s stub calls');
    mark('deploy-native-ok');
    await h.moveTo(nativeResult);
    await h.read('Triển khai bằng renderer mặc định thành công');

    caption('Bước 9: Xem trước ứng dụng có mẫu khớp; hệ thống chọn đúng mẫu tpl-web');
    await h.click(ui('shell', 'navApplications'));
    await openApplication(TEMPLATE_NAME);
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi' }));
    const templatePreview = page.getByLabel('Xem trước triển khai');
    await expect(templatePreview).toContainText('1 Workload bị ảnh hưởng', { timeout: 60_000 });
    await expect(templatePreview.locator('li').filter({ hasText: /^web / })).toContainText('score-k8s 0.15.0 (tpl-web)');
    await expect(templatePreview.locator('li').filter({ hasText: /^web / })).not.toContainText('Kubernetes tích hợp sẵn');
    check('Preview, application matching criteria: selected template tpl-web (score-k8s 0.15.0), not native');
    mark('preview-template');
    expect(calls(), 'Preview selection did not run the renderer').toEqual([]);
    await h.moveTo(templatePreview);
    await h.read('Mẫu khớp điều kiện được chọn, thông tin mẫu hiện trong bản xem trước');

    caption('Bước 10: Triển khai; mẫu đã chọn render lỗi nên báo lỗi, không chuyển sang renderer mặc định');
    await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này' }));
    const failedResult = page.getByLabel('Kết quả triển khai');
    await expect(failedResult).toContainText('Triển khai thất bại', { timeout: 120_000 });
    await expect(failedResult.locator('li').filter({ hasText: /^web · / })).toContainText('thất bại');
    await expect(failedResult).not.toContainText('Triển khai thành công');
    await expect(failedResult).toContainText('Thay đổi chưa hoàn tất vẫn ở trạng thái chờ');
    const stubCalls = calls();
    expect(stubCalls.some((line) => line.startsWith('generate ')), 'selected renderer was invoked and failed').toBe(true);
    check(`Deploy with selected template failed through the product Deploy path (stub generate exit 1); stub calls: ${stubCalls.length}`);
    mark('deploy-render-failed');
    await h.moveTo(failedResult);
    await h.read('Mẫu được chọn render lỗi: báo lỗi, workload vẫn chờ triển khai');

    caption('Bước 11: Xem trước lại; workload vẫn chờ triển khai bằng mẫu, chưa bị renderer mặc định thay thế');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi' }));
    const again = page.getByLabel('Xem trước triển khai');
    await expect(again).toContainText('1 Workload bị ảnh hưởng', { timeout: 60_000 });
    await expect(again.locator('li').filter({ hasText: /^web / })).toContainText('score-k8s 0.15.0 (tpl-web)');
    check('after the render failure web is still a pending change selecting tpl-web: nothing was committed, no native fallback');
    mark('preview-after-failure');
    await h.read('Workload vẫn là thay đổi chờ xử lý và vẫn chọn mẫu tpl-web');
    await signOut();
    mark('signed-out');
    await h.pause(3000);
  },
  // T04: trang Loại tài nguyên. Platform Engineer, fake adapters, catalog dựng
  // sẵn (postgres có inputs/outputs và password secret). Schema tùy chỉnh được
  // tạo qua UI bằng cách sao chép; không seed bằng API.
  async T04({ page, h, ui, caption, mark, check, baseURL }) {
    const types = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-types`)).json()).resourceTypes;
    const definitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const entry = (key) => page.locator('.catalog-entry').filter({ has: page.locator('strong', { hasText: new RegExp(`^${key}$`) }) });
    const formAlert = () => page.locator('form .form-error[role="alert"]');
    const ctas = () => page.getByRole('link', { name: /^Tạo Cấu hình tài nguyên cho / });
    const cells = async (table) => table.locator('tbody tr').evaluateAll((rows) => rows.map((row) => [...row.querySelectorAll('td')].map((cell) => cell.textContent)));
    let definitionCount = 0;

    caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    mark('applications');

    caption('Bước 2: Mở trang Loại tài nguyên; nhãn hỗ trợ theo đúng bốn type có cách tạo');
    await h.click(ui('shell', 'navResourceTypes'));
    await expect(ui('resourceTypes', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-types`);
    for (const key of ['vpc', 'k8s-cluster', 'k8s-namespace', 'postgres']) await expect(entry(key)).toContainText('Có cách tạo hỗ trợ');
    await expect(entry('workload')).toContainText('Chỉ có contract');
    await expect(entry('postgres')).toContainText('Kubernetes hoặc Terraform/aurora');
    await expect(entry('vpc')).toContainText('Terraform/vpc');
    await expect(entry('k8s-cluster')).toContainText('Terraform/eks');
    await expect(ctas()).toHaveCount(4);
    await expect(entry('workload').getByRole('link', { name: 'Mẫu dựng ứng dụng' })).toBeVisible();
    await expect(entry('workload').getByRole('link', { name: /Tạo Cấu hình tài nguyên/ })).toHaveCount(0);
    check('list: vpc/k8s-cluster/k8s-namespace/postgres labelled "Có cách tạo hỗ trợ" with drivers; workload "Chỉ có contract" pointing to Mẫu dựng ứng dụng; exactly 4 definition CTAs');
    mark('types-page');
    await h.moveTo(entry('postgres'));
    await h.read('Chỉ bốn type có cách tạo được gắn nhãn hỗ trợ; workload đi qua Mẫu dựng ứng dụng');

    caption('Bước 3: Xem schema của postgres; tên, kiểu, bắt buộc và bí mật lấy nguyên từ máy chủ');
    await h.click(page.getByRole('button', { name: 'Xem schema postgres' }));
    const inputsTable = page.getByRole('table', { name: 'Đầu vào của postgres' });
    const outputsTable = page.getByRole('table', { name: 'Đầu ra của postgres' });
    await expect(inputsTable).toBeVisible();
    expect(await cells(inputsTable)).toEqual([['database', 'string', 'Có'], ['username', 'string', 'Có']]);
    expect(await cells(outputsTable)).toEqual([
      ['host', 'string', 'Có', 'Không'], ['port', 'number', 'Có', 'Không'], ['database', 'string', 'Có', 'Không'],
      ['username', 'string', 'Có', 'Không'], ['password', 'string', 'Có', 'Có']]);
    check('postgres details: 2 inputs and 5 outputs with type/required/secret exactly as GET; only password is secret');
    mark('postgres-details');
    await h.moveTo(outputsTable);
    await h.read('Chỉ password là bí mật; không có sửa, xóa hay phiên bản schema');
    await expect(page.getByRole('button', { name: /sửa|xóa loại|phiên bản/i })).toHaveCount(0);

    caption('Bước 4: Sao chép schema sang bản nháp; ID để trống, giữ nguyên kiểu và cờ');
    await h.click(page.getByRole('button', { name: 'Sao chép schema postgres' }));
    await expect(ui('resourceTypes', 'id')).toHaveValue('');
    await expect(ui('resourceTypes', 'id')).toBeFocused();
    await expect(page.getByLabel('Đầu vào 1 tên')).toHaveValue('database');
    await expect(page.getByLabel('Đầu vào 1 bắt buộc')).toBeChecked();
    await expect(page.getByLabel('Đầu ra 2 kiểu')).toHaveValue('number');
    await expect(page.getByLabel('Đầu ra 5 tên')).toHaveValue('password');
    await expect(page.getByLabel('Đầu ra 5 bí mật')).toBeChecked();
    await expect(page.getByLabel('Đầu ra 1 bí mật')).not.toBeChecked();
    await expect(page.getByText(/Bản nháp sao chép từ/)).toContainText('postgres');
    check('copy: blank ID, 2 inputs + 5 outputs with types/required/secret preserved, no request sent');
    mark('schema-copied');
    await h.moveTo(page.getByText(/Bản nháp sao chép từ/));
    await h.read('Bản nháp độc lập: nhập ID mới và sửa schema trước khi đăng ký');

    caption('Bước 5: Sửa tên input thứ nhất; nhập ID postgres đã tồn tại; máy chủ từ chối và giữ bản nháp');
    await h.type(page.getByLabel('Đầu vào 1 tên'), 'dbname', { replace: true });
    await h.type(ui('resourceTypes', 'id'), 'postgres');
    await h.click(ui('resourceTypes', 'submit'));
    await expect(formAlert()).toContainText('ID loại tài nguyên đã tồn tại');
    expect(await formAlert().innerText()).not.toMatch(/already exists|duplicate|conflict/i);
    await expect(ui('resourceTypes', 'id')).toHaveValue('postgres');
    await expect(page.getByLabel('Đầu vào 1 tên')).toHaveValue('dbname');
    await expect(page.getByLabel('Đầu ra 5 bí mật')).toBeChecked();
    const original = (await types()).find((item) => item.key === 'postgres');
    expect(original.inputs.map((field) => field.name)).toEqual(['database', 'username']);
    expect((await types()).filter((item) => item.key === 'postgres')).toHaveLength(1);
    check('duplicate 409: Vietnamese message without raw text, ID and edited draft kept, stored postgres schema unchanged (inputs database, username)');
    mark('duplicate-rejected');
    await h.moveTo(formAlert());
    await h.read('ID đã tồn tại; schema postgres gốc không bị ghi đè, bản nháp vẫn còn');

    caption('Bước 6: Đổi sang ID mới postgres-ha và đăng ký lại bản nháp đã sửa');
    await h.type(ui('resourceTypes', 'id'), 'postgres-ha', { replace: true });
    await h.click(ui('resourceTypes', 'submit'));
    await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký loại tài nguyên postgres-ha.' })).toBeVisible();
    await expect(ui('resourceTypes', 'id')).toHaveValue('');
    const stored = await types();
    const copy = stored.find((item) => item.key === 'postgres-ha');
    expect(copy.inputs).toEqual([{ name: 'dbname', type: 'string', required: true }, { name: 'username', type: 'string', required: true }]);
    expect(copy.outputs.map((field) => [field.name, field.type, field.required === true, field.secret === true])).toEqual([
      ['host', 'string', true, false], ['port', 'number', true, false], ['database', 'string', true, false],
      ['username', 'string', true, false], ['password', 'string', true, true]]);
    expect(stored.find((item) => item.key === 'postgres').inputs.map((field) => field.name)).toEqual(['database', 'username']);
    check('registered postgres-ha from the edited copy: flags/types preserved, postgres untouched');
    mark('custom-registered');
    await h.moveTo(entry('postgres-ha'));
    await h.read('Đã đăng ký postgres-ha');

    caption('Bước 7: postgres-ha chỉ có contract; không có nút tạo Cấu hình tài nguyên');
    await expect(entry('postgres-ha')).toContainText('Chỉ có contract');
    await expect(entry('postgres-ha').getByRole('link', { name: /Tạo Cấu hình tài nguyên/ })).toHaveCount(0);
    await expect(entry('postgres-ha')).not.toContainText('Cách tạo:');
    await expect(ctas()).toHaveCount(4);
    check('custom postgres-ha: "Chỉ có contract", no creation method text, still exactly 4 CTAs');
    mark('custom-no-cta');
    await h.read('postgres-ha chỉ là contract, không được quảng bá là triển khai được');

    caption('Bước 8: Bấm Tạo Cấu hình tài nguyên của postgres; form được chọn sẵn Loại postgres');
    definitionCount = (await definitions()).length;
    await h.click(page.getByRole('link', { name: 'Tạo Cấu hình tài nguyên cho postgres' }));
    await expect(ui('resourceDefinitions', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/resource-definitions?resourceType=postgres`);
    await expect(ui('resourceDefinitions', 'type')).toHaveValue('postgres');
    await expect(ui('resourceDefinitions', 'driver')).toHaveValue('kubernetes');
    expect((await definitions()).length, 'CTA registers nothing').toBe(definitionCount);
    check('supported CTA: /resource-definitions?resourceType=postgres, Loại tài nguyên preselected, no definition created');
    mark('cta-prefill');
    await h.moveTo(ui('resourceDefinitions', 'type'));
    await h.read('Loại tài nguyên đã được chọn sẵn; chưa đăng ký hay triển khai gì');

    caption('Bước 9: Mở thẳng liên kết với type chỉ có contract; không chọn sẵn gì');
    await page.goto(`${baseURL}/ui/platform/resource-definitions?resourceType=postgres-ha`);
    await expect(ui('resourceDefinitions', 'heading')).toBeVisible();
    await expect(ui('resourceDefinitions', 'type').locator('option[value="postgres"]')).toBeAttached();
    await expect(ui('resourceDefinitions', 'type')).toHaveValue('');
    check('custom/invalid query postgres-ha: no preselection');
    mark('invalid-query-no-prefill');
    await h.moveTo(ui('resourceDefinitions', 'type'));
    await h.read('Query không hợp lệ thì không chọn type nào');

    caption('Bước 10: Quay lại Loại tài nguyên; workload dẫn sang Mẫu dựng ứng dụng, không có CTA');
    await h.click(ui('shell', 'navResourceTypes'));
    await expect(ui('resourceTypes', 'heading')).toBeVisible();
    await h.click(entry('workload').getByRole('link', { name: 'Mẫu dựng ứng dụng' }));
    await expect(ui('renderingTemplates', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/rendering-templates`);
    check('workload guidance link opens Mẫu dựng ứng dụng');
    mark('workload-templates');
    await h.read('workload được tạo bằng Mẫu dựng ứng dụng');

    caption('Bước 11: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },
  // T05: trang Kho bí mật. Platform Engineer; backend fake verifier thật
  // (token "bad-token" bị từ chối 422, token khác được chấp nhận). Token chỉ
  // là chuỗi tổng hợp, nhập vào ô bị che. Mocked bằng Playwright route (ghi rõ
  // trong captions/assertions): chỉ phản hồi GET danh sách ở bước projection
  // ABSENT và bước lỗi tải lại danh sách; mọi POST đi tới backend fake thật.
  async T05({ page, h, ui, caption, mark, check, baseURL }) {
    const BAD_TOKEN = 'bad-token';
    const GOOD_TOKEN = 'synthetic-scoped-token-t05';
    const storeApi = async () => (await (await page.request.get(`${baseURL}/api/v1/secret-stores`)).json()).secretStores;
    const posts = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/secret-stores') posts.push(request.postDataJSON());
    });
    // 'real' passes GET through; 'absent' rewrites Kubernetes auth of "Kho B" to ABSENT (mocked projection); 'fail' returns 500 once.
    let listMode = 'real';
    await page.route('**/api/v1/secret-stores', async (route) => {
      if (route.request().method() !== 'GET' || listMode === 'real') { await route.continue(); return; }
      if (listMode === 'fail') { listMode = 'absent'; await route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"mocked list failure"}' }); return; }
      const response = await route.fetch();
      const data = await response.json();
      for (const store of data.secretStores) if (store.name === 'Kho B') store.verification = { ...store.verification, kubernetesAuth: 'ABSENT' };
      await route.fulfill({ response, json: data });
    });
    const token = () => ui('secretStores', 'token');
    const alert = () => page.locator('form[aria-label="Đăng ký kho bí mật"] .form-error[role="alert"]');
    const summary = () => page.getByLabel('Tóm tắt cấu hình hiệu lực');
    const row = (name) => page.locator('.settings-table-row').filter({ has: page.locator('strong', { hasText: name }) });
    const noLeak = async (allow) => { const { assertNoSecretLeaks } = await import('./secret-scan.mjs'); await assertNoSecretLeaks(page, [BAD_TOKEN, GOOD_TOKEN], { allow }); };

    caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    mark('applications');

    caption('Bước 2: Mở trang Kho bí mật; chưa có kho nào, phần tóm tắt luôn hiện mount mặc định kv và kubernetes');
    await h.click(ui('shell', 'navSecretStores'));
    await expect(ui('secretStores', 'heading')).toBeVisible();
    await expect(page.getByText('Chưa có kho bí mật nào được đăng ký.')).toBeVisible();
    await expect(summary()).toContainText('Mount KV v2: kv (mặc định)');
    await expect(summary()).toContainText('Mount xác thực Kubernetes: kubernetes (mặc định)');
    await expect(summary()).toContainText('TLS luôn được xác minh');
    await expect(ui('secretStores', 'sameAddress')).not.toBeChecked();
    await expect(ui('secretStores', 'mount')).not.toBeVisible();
    await expect(token()).toHaveAttribute('type', 'password');
    expect(await storeApi(), 'fake backend starts with no store').toEqual([]);
    check('page (real fake backend): Vietnamese heading, empty list, default mounts in summary, checkbox off, advanced hidden, token masked');
    mark('secret-stores-page');
    await h.read('Phần tóm tắt luôn cho biết mount hiệu lực và TLS luôn được xác minh');

    caption('Bước 3: Bấm đăng ký khi form còn trống; lỗi tiếng Việt chỉ rõ từng ô và không có POST nào được gửi');
    await h.click(ui('secretStores', 'submit'));
    await expect(page.getByText('Tên kho là bắt buộc.')).toBeVisible();
    await expect(page.getByText('Địa chỉ backend là bắt buộc.')).toBeVisible();
    await expect(page.getByText('Token là bắt buộc.')).toBeVisible();
    expect(posts, 'empty form sends no POST').toEqual([]);
    check('local validation: empty form shows required errors, 0 POST');
    mark('local-required');
    await h.read('Form trống bị chặn ngay tại frontend, chưa gửi gì lên máy chủ');

    caption('Bước 4: Nhập địa chỉ sai (ftp, có thông tin đăng nhập) và mount sai trong phần nâng cao; vẫn không có POST');
    await h.type(ui('secretStores', 'name'), 'Kho A');
    await h.type(ui('secretStores', 'backend'), 'ftp://user:pw@vault-a.local:8200');
    await h.type(ui('secretStores', 'workload'), 'http://vault-a.vault.svc:8200');
    await h.type(token(), BAD_TOKEN);
    await h.click(ui('secretStores', 'showAdvanced'));
    await h.type(ui('secretStores', 'mount'), 'sai/mount');
    await h.click(ui('secretStores', 'submit'));
    await expect(page.getByText(/^Địa chỉ backend phải là URL http\(s\)/)).toBeVisible();
    await expect(page.getByText(/^Mount KV v2 chỉ gồm 1–64/)).toBeVisible();
    expect(posts, 'invalid URL and mount send no POST').toEqual([]);
    check('local validation: bad URL scheme/userinfo and bad mount rejected, still 0 POST');
    mark('local-invalid');
    await h.read('Địa chỉ và mount không hợp lệ bị chặn; chưa kiểm tra mạng hay chứng chỉ');

    caption('Bước 5: Sửa thành địa chỉ đúng, mount team-kv và xác thực k8s_a; địa chỉ backend và workload là hai địa chỉ khác nhau');
    await h.type(ui('secretStores', 'backend'), 'http://vault-a.local:8200', { replace: true });
    await h.type(ui('secretStores', 'mount'), 'team-kv', { replace: true });
    await h.type(ui('secretStores', 'authMount'), 'k8s_a');
    await expect(summary()).toContainText('Mount KV v2: team-kv');
    await expect(summary()).not.toContainText('Mount KV v2: team-kv (mặc định)');
    await expect(summary()).toContainText('Mount xác thực Kubernetes: k8s_a');
    await noLeak(['input[type="password"]']);
    check('advanced values update the always-visible effective summary; token only inside the masked field');
    mark('advanced-filled');
    await h.moveTo(summary());
    await h.read('Tóm tắt phản ánh mount tùy chỉnh đã nhập');

    caption('Bước 6: Gửi với token bị backend fake từ chối; form báo hướng dẫn cố định, xóa token, giữ nguyên metadata');
    const failed = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/secret-stores');
    await h.click(ui('secretStores', 'submit'));
    expect((await failed).status(), 'real fake verifier rejects the token').toBe(422);
    await expect(alert()).toContainText('Không xác minh được kho bí mật nên chưa lưu kho');
    await expect(alert()).not.toContainText('rejected the token');
    await expect(token()).toHaveValue('');
    await expect(ui('secretStores', 'name')).toHaveValue('Kho A');
    await expect(ui('secretStores', 'backend')).toHaveValue('http://vault-a.local:8200');
    await expect(ui('secretStores', 'workload')).toHaveValue('http://vault-a.vault.svc:8200');
    await expect(ui('secretStores', 'mount')).toHaveValue('team-kv');
    await expect(ui('secretStores', 'mount')).toBeVisible();
    expect(posts.length, 'exactly one POST reached the backend').toBe(1);
    expect(posts[0]).toMatchObject({ name: 'Kho A', backendAddress: 'http://vault-a.local:8200', workloadAddress: 'http://vault-a.vault.svc:8200', mount: 'team-kv', authMount: 'k8s_a' });
    expect(await storeApi(), 'rejected registration stored nothing').toEqual([]);
    await noLeak([]);
    check('backend failure (real fake verifier 422): fixed Vietnamese guidance without raw error, token cleared on screen, name/addresses/mount/advanced kept, nothing stored');
    mark('verification-failed');
    await h.moveTo(alert());
    await h.read('Token bị xóa nhưng tên, địa chỉ và mount vẫn còn để sửa');

    caption('Bước 7: Bật "Dùng cùng địa chỉ backend"; địa chỉ workload theo backend, không ghi đè địa chỉ riêng đã nhập');
    await h.click(ui('secretStores', 'sameAddress'));
    await expect(ui('secretStores', 'workload')).toHaveValue('http://vault-a.local:8200');
    await h.type(ui('secretStores', 'backend'), 'http://vault-a.local:8200/v1', { replace: true });
    await expect(ui('secretStores', 'workload')).toHaveValue('http://vault-a.local:8200/v1');
    await h.click(ui('secretStores', 'sameAddress'));
    await expect(ui('secretStores', 'workload')).toHaveValue('http://vault-a.vault.svc:8200');
    check('checkbox on follows backend edits; checkbox off restores the independent workload draft');
    mark('same-address-toggle');
    await h.read('Tắt checkbox thì địa chỉ workload riêng quay lại nguyên vẹn');

    caption('Bước 8: Đóng phần nâng cao rồi mở lại; dữ liệu không mất. Bật cùng địa chỉ, nhập token hợp lệ và đăng ký');
    await h.click(ui('secretStores', 'hideAdvanced'));
    await expect(ui('secretStores', 'mount')).not.toBeVisible();
    await expect(summary()).toContainText('Mount KV v2: team-kv');
    await h.click(ui('secretStores', 'showAdvanced'));
    await expect(ui('secretStores', 'mount')).toHaveValue('team-kv');
    await h.click(ui('secretStores', 'sameAddress'));
    await h.paste(token(), GOOD_TOKEN);
    await noLeak(['input[type="password"]']);
    mark('advanced-persisted');
    const created = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/secret-stores');
    await h.click(ui('secretStores', 'submit'));
    const createdResponse = await created;
    expect(createdResponse.status()).toBe(201);
    expect(await createdResponse.text()).not.toContain(GOOD_TOKEN);
    await expect(page.getByRole('status').filter({ hasText: 'Kho bí mật "Kho A" đã sẵn sàng' })).toBeVisible();
    expect(posts.length).toBe(2);
    expect(posts[1]).toMatchObject({ name: 'Kho A', backendAddress: 'http://vault-a.local:8200/v1', workloadAddress: 'http://vault-a.local:8200/v1', mount: 'team-kv', authMount: 'k8s_a' });
    await expect(token()).toHaveValue('');
    await expect(ui('secretStores', 'name')).toHaveValue('');
    await expect(ui('secretStores', 'sameAddress')).not.toBeChecked();
    await noLeak([]);
    check('success (real fake backend 201): same-address payload uses backend for workload, form and token cleared, no token on screen');
    mark('registered-a');
    await h.read('Kho đã sẵn sàng; form được xóa sạch');

    caption('Bước 9: Danh sách hiển thị hai địa chỉ, mount, xác minh backend và xác thực Kubernetes tách riêng; không có token hay tham chiếu');
    const rowA = row('Kho A');
    await expect(rowA).toContainText('Backend: http://vault-a.local:8200/v1');
    await expect(rowA).toContainText('Workload: http://vault-a.local:8200/v1');
    await expect(rowA).toContainText('KV v2: team-kv');
    await expect(rowA).toContainText('Trạng thái: Sẵn sàng');
    await expect(rowA).toContainText('Xác minh backend: Backend đã xác minh khi đăng ký');
    await expect(rowA).toContainText('Xác thực Kubernetes: Đã cấu hình');
    await expect(page.getByLabel('Kho bí mật đã đăng ký')).toContainText('không đảm bảo ứng dụng đã sẵn sàng triển khai');
    const apiA = (await storeApi()).find((store) => store.name === 'Kho A');
    expect(apiA.verification.kubernetesAuth, 'real fake-backend projection').toBe('CONFIGURED');
    check('list (real fake backend): both addresses, mount, READY as "Sẵn sàng", backend verification and Kubernetes auth CONFIGURED separate, READY not deploy-ready');
    mark('list-a');
    await h.moveTo(rowA);
    await h.read('Sẵn sàng chỉ nghĩa là backend đã được xác minh lúc đăng ký');

    caption('Bước 10: Đăng ký Kho B với mặc định và hai địa chỉ khác nhau; danh sách tải lại lỗi (mock) nhưng đăng ký vẫn thành công');
    listMode = 'fail';
    await h.type(ui('secretStores', 'name'), 'Kho B');
    await h.type(ui('secretStores', 'backend'), 'https://vault-b.local:8200');
    await h.type(ui('secretStores', 'workload'), 'http://vault-b.vault.svc:8200');
    await h.paste(token(), GOOD_TOKEN);
    await h.click(ui('secretStores', 'submit'));
    await expect(page.getByRole('status').filter({ hasText: 'Kho bí mật "Kho B" đã sẵn sàng' })).toBeVisible();
    await expect(page.getByText(/^Không tải được danh sách kho bí mật/)).toBeVisible();
    expect(posts.length).toBe(3);
    expect(posts[2]).toMatchObject({ name: 'Kho B', backendAddress: 'https://vault-b.local:8200', workloadAddress: 'http://vault-b.vault.svc:8200', mount: 'kv', authMount: 'kubernetes' });
    expect(posts[2]).not.toHaveProperty('tlsCaPem');
    expect((await storeApi()).map((store) => store.name).sort(), 'Kho B committed on the real fake backend').toEqual(['Kho A', 'Kho B']);
    check('registration success stays distinct from the MOCKED list reload failure (GET 500 once); defaults kv/kubernetes sent, different addresses independent, no CA sent');
    mark('registered-b-list-failed');
    await h.moveTo(page.getByText(/^Không tải được danh sách kho bí mật/));
    await h.read('Đăng ký thành công; chỉ danh sách tải lại bị lỗi');

    caption('Bước 11: Bấm Thử lại chỉ tải lại danh sách; xác thực Kubernetes của Kho B hiện "Chưa cấu hình" (projection mock ABSENT)');
    await h.click(ui('secretStores', 'retry'));
    await expect(row('Kho B')).toBeVisible();
    await expect(row('Kho B')).toContainText('Xác thực Kubernetes: Chưa cấu hình');
    await expect(row('Kho B')).toContainText('Xác minh backend: Backend đã xác minh khi đăng ký');
    await expect(row('Kho B')).toContainText('Trạng thái: Sẵn sàng');
    await expect(row('Kho A')).toContainText('Xác thực Kubernetes: Đã cấu hình');
    expect(posts.length, 'retry only reloads the list').toBe(3);
    check('list retry sends only GET; MOCKED projection shows Kho B auth "Chưa cấu hình" while backend verification stays verified and Kho A stays "Đã cấu hình"');
    mark('list-absent-configured');
    await h.moveTo(row('Kho B'));
    await h.read('Backend đã xác minh nhưng xác thực Kubernetes chưa cấu hình; hai thông tin tách riêng');

    caption('Bước 12: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },
  // T06: trang Kết nối. Platform Engineer; backend fake với memory credential
  // store, không state file. Inspect, parser, cảnh báo và xác minh thất bại
  // (422) là backend thật: kubeconfig tổng hợp trỏ tới endpoint .invalid nên
  // verifier kubectl thật không tới được cụm nào. Playwright route (ghi rõ
  // "MOCK" trong captions/assertions) chỉ dùng cho: (1) giữ phản hồi inspect của
  // một document lại rồi thả muộn bằng route.continue (kết quả vẫn do backend
  // thật trả), (2) giả lập POST đăng ký thành công 201 và projection danh sách
  // sau đó, vì fake backend không có verifier thành công cho kubeconfig.
  // Token trong fixture chỉ là chuỗi tổng hợp; không có credential thật.
  async T06({ page, h, ui, caption, mark, check, baseURL }) {
    const TOKEN = 'synthetic-t06-token-0123456789abcdef';
    const evidenceDir = process.env.ORCH_E2E_EVIDENCE_DIR;
    const fixtures = `${evidenceDir}/fixtures`;
    mkdirSync(fixtures, { recursive: true });
    const doc = (marker, entries, current) => [
      'apiVersion: v1', 'kind: Config', `# ${marker}`,
      'clusters:', ...entries.flatMap(([, cluster]) => [`- name: ${cluster}`, '  cluster:', `    server: https://${cluster}.invalid:6443`]),
      'users:', '- name: t06-user', '  user:', `    token: ${TOKEN}`,
      'contexts:', ...entries.flatMap(([context, cluster]) => [`- name: ${context}`, '  context:', `    cluster: ${cluster}`, '    user: t06-user']),
      `current-context: ${current ?? entries[0][0]}`,
    ].join('\n');
    const single = doc('t06-single', [['ctx-a', 'cluster-a']]);
    const multi = doc('t06-multi', [['dev-t06', 'cluster-dev'], ['prod-t06', 'cluster-prod']], 'prod-t06');
    const delayed = doc('t06-delayed', [['ctx-late', 'cluster-late']]);
    const docB = doc('t06-b', [['ctx-b', 'cluster-b']]);
    writeFileSync(`${fixtures}/single.kubeconfig`, single);
    writeFileSync(`${fixtures}/multi.kubeconfig`, multi);

    const inspects = [];
    const registers = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (request.method() === 'POST' && path === '/api/v1/connections/kubernetes/inspect') inspects.push(request.postDataJSON());
      if (request.method() === 'POST' && path === '/api/v1/connections/kubernetes') registers.push(request.postDataJSON());
    });
    // MOCK 1: hold the inspect reply of the marked document, then let the real backend answer late.
    const gate = { armed: false, release: () => {}, promise: Promise.resolve() };
    await page.route('**/api/v1/connections/kubernetes/inspect', async (route) => {
      if (gate.armed && (route.request().postData() ?? '').includes('t06-delayed')) { gate.armed = false; await gate.promise; }
      await route.continue();
    });
    // MOCK 2: successful registration and the list projection that follows it.
    let mockSave = false;
    let savedRow;
    await page.route('**/api/v1/connections/kubernetes', async (route) => {
      if (route.request().method() !== 'POST' || !mockSave) { await route.continue(); return; }
      const { name } = route.request().postDataJSON();
      savedRow = { key: 'cum-thu-t06', name, kind: 'KUBERNETES', authenticationType: 'KUBECONFIG', status: 'READY', config: { cluster: 'cluster-b', kubeContext: 'ctx-b', endpoint: 'https://cluster-b.invalid:6443' } };
      await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify(savedRow) });
    });
    await page.route('**/api/v1/connections', async (route) => {
      if (route.request().method() !== 'GET' || !savedRow) { await route.continue(); return; }
      const response = await route.fetch();
      const data = await response.json();
      await route.fulfill({ response, json: { ...data, connections: [...data.connections, savedRow] } });
    });

    const connectionsApi = async () => (await (await page.request.get(`${baseURL}/api/v1/connections`)).json()).connections;
    const name = () => ui('connections', 'name');
    const content = () => ui('connections', 'content');
    const summary = () => page.getByLabel('Đích đã chọn');
    const alert = () => page.locator('form .form-error[role="alert"]');
    const fileInput = () => page.locator('input[type="file"]');
    const inspectReply = () => page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/connections/kubernetes/inspect');
    const uploadFile = async (path) => {
      const chooser = page.waitForEvent('filechooser');
      chooser.catch(() => {});
      await h.click(fileInput());
      await (await chooser).setFiles(path);
    };
    // The DOM minus textareas (masked pasted documents live only there) never holds the token.
    /* global document -- runs inside the browser page. */
    const noLeak = async () => {
      const leaks = await page.evaluate((token) => {
        const found = [];
        if (document.body.innerText.includes(token)) found.push('text');
        for (const element of document.querySelectorAll('*')) {
          if (element.tagName === 'TEXTAREA') continue;
          for (const attribute of element.attributes) if (attribute.value.includes(token)) found.push(`attribute ${attribute.name}`);
          if (['INPUT', 'SELECT'].includes(element.tagName) && element.value.includes(token)) found.push('field value');
        }
        return found;
      }, TOKEN);
      expect(leaks, 'token outside the masked textarea').toEqual([]);
    };

    caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    mark('applications');

    caption('Bước 2: Mở trang Kết nối; danh sách tiếng Việt, chỉ kết nối Sẵn sàng, không có nút Inspect');
    await h.click(ui('shell', 'navConnections'));
    await expect(ui('connections', 'heading')).toBeVisible();
    const table = page.getByRole('table', { name: 'Kết nối đã đăng ký' });
    await expect(table).toBeVisible();
    await expect(table.locator('tbody tr').first()).toContainText('Sẵn sàng');
    await expect(page.getByRole('button', { name: /Inspect|Kiểm tra lại/ })).toHaveCount(0);
    await expect(ui('connections', 'submit')).toBeVisible();
    const baseline = (await connectionsApi()).length;
    check('page (real fake backend): Vietnamese heading, READY list "Sẵn sàng", no manual inspect button');
    mark('connections-page');
    await h.moveTo(table);
    await h.read('Trang tiếng Việt; chỉ kết nối Sẵn sàng được liệt kê');

    caption('Bước 3: Tải lên file có một context; Console tự inspect, tự chọn context và gợi ý tên từ cluster, không lưu gì');
    await uploadFile(`${fixtures}/single.kubeconfig`);
    await expect(summary()).toContainText('Context ctx-a');
    await expect(summary()).toContainText('Cluster cluster-a');
    await expect(name()).toHaveValue('cluster-a');
    await expect(page.getByText(/Đã chọn file: single.kubeconfig/)).toBeVisible();
    expect(inspects.length, 'one automatic inspect').toBe(1);
    expect(inspects[0]).toEqual({ kubeconfig: single });
    expect(registers, 'inspect never registers').toEqual([]);
    expect((await connectionsApi()).length, 'nothing persisted').toBe(baseline);
    await noLeak();
    check('upload (real backend parser): auto inspect once after file read, single context auto-selected, name suggested "cluster-a", 0 register POST, list unchanged, token not on screen');
    mark('upload-auto-inspect');
    await h.moveTo(summary());
    await h.read('Chỉ một context nên được chọn sẵn; tên gợi ý đi theo cluster');

    caption('Bước 4: Tải file nhiều context; gợi ý tên biến mất, UI không đoán theo current-context (prod-t06) và không tự chọn');
    await uploadFile(`${fixtures}/multi.kubeconfig`);
    await expect(ui('connections', 'context')).toBeVisible();
    await expect(ui('connections', 'context')).toHaveValue('');
    await expect(summary()).toHaveCount(0);
    await expect(name()).toHaveValue('');
    expect(inspects.length).toBe(2);
    expect(registers).toEqual([]);
    check('multi-context (real backend): select empty although current-context=prod-t06, no summary, suggestion cleared with the old document');
    mark('multi-context');
    await h.read('Nhiều context phải chọn tường minh; Console không đoán');

    caption('Bước 5: Chọn dev-t06 thì tên gợi ý theo cluster-dev; tự sửa tên thì đổi context không ghi đè');
    await h.choose(ui('connections', 'context'), 'dev-t06');
    await expect(summary()).toContainText('Cluster cluster-dev');
    await expect(name()).toHaveValue('cluster-dev');
    await h.type(name(), 'Cụm thử T06', { replace: true });
    await h.choose(ui('connections', 'context'), 'prod-t06');
    await expect(summary()).toContainText('Cluster cluster-prod');
    await expect(name()).toHaveValue('Cụm thử T06');
    check('name ownership: untouched suggestion followed context; after user edit, another context did not overwrite');
    mark('name-owned');
    await h.read('Tên người dùng đã sửa được giữ nguyên khi đổi context');

    caption('Bước 6: Xóa trống tên rồi đổi file; tên trống cũng là lựa chọn của người dùng, không bị gợi ý ghi đè');
    await h.type(name(), '', { replace: true });
    await expect(name()).toHaveValue('');
    await uploadFile(`${fixtures}/single.kubeconfig`);
    await expect(summary()).toContainText('Context ctx-a');
    await expect(name()).toHaveValue('');
    expect(registers).toEqual([]);
    check('blank name stays blank after switching file and auto-selecting a single context');
    mark('blank-name-kept');
    await h.read('Tên để trống vẫn do người dùng sở hữu');

    caption('Bước 7: Chuyển sang nhập nội dung; nội dung bị che, vừa gõ chưa inspect, gõ xong và ổn định mới inspect đúng một lần');
    await h.click(ui('connections', 'sourcePaste'));
    await expect(summary()).toHaveCount(0);
    await expect(content()).toHaveClass(/masked-text/);
    const before = inspects.length;
    const settled = inspectReply();
    await h.click(content());
    const typing = content().pressSequentially(docB, { delay: 25 });
    await page.waitForTimeout(2500);
    expect(inspects.length, 'no inspect while the document is still being typed').toBe(before);
    await typing;
    await settled;
    await expect(summary()).toContainText('Cluster cluster-b');
    expect(inspects.length, 'exactly one debounced inspect').toBe(before + 1);
    await page.waitForTimeout(1200);
    expect(inspects.length, 'no extra inspect').toBe(before + 1);
    await noLeak();
    check('typed paste (real backend): masked textarea, no inspect while typing, one inspect after the document settled, no register');
    mark('paste-debounced');
    await h.moveTo(summary());
    await h.read('Gõ xong mới inspect một lần, không inspect từng ký tự');

    caption('Bước 8: Nhập lại trang. Dán document trễ (route MOCK giữ phản hồi inspect), gõ tên, rồi đổi sang document khác; phản hồi muộn bị bỏ');
    await page.reload();
    await expect(ui('connections', 'heading')).toBeVisible();
    await h.click(ui('connections', 'sourcePaste'));
    gate.promise = new Promise((resolve) => { gate.release = resolve; });
    gate.armed = true;
    await h.paste(content(), delayed);
    await expect(page.getByText('Đang kiểm tra kubeconfig…')).toBeVisible({ timeout: 5000 });
    await h.type(name(), 'Tên gõ lúc chờ');
    const lateInspects = inspects.length;
    await h.click(content());
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Delete');
    await expect(page.getByText('Đang kiểm tra kubeconfig…')).toHaveCount(0);
    const settledB = inspectReply();
    await h.paste(content(), docB);
    await settledB;
    await expect(summary()).toContainText('Cluster cluster-b');
    gate.release();
    await page.waitForTimeout(1500);
    await expect(summary()).toContainText('Cluster cluster-b');
    await expect(summary()).not.toContainText('cluster-late');
    await expect(name()).toHaveValue('Tên gõ lúc chờ');
    expect(inspects.length, 'delayed + replacement inspects only').toBe(lateInspects + 1);
    expect(registers).toEqual([]);
    check('stale reply (MOCK holds the real backend reply, released after the switch): old document context cluster-late never shown; new document summary kept; typed name kept; file-read race covered by unit tests only');
    mark('stale-reply-ignored');
    await h.moveTo(summary());
    await h.read('Phản hồi muộn của document cũ không ghi đè context hay tên');

    caption('Bước 9: Kiểm tra và lưu; backend thật từ chối xác minh (422), Console chỉ hiện hướng dẫn cố định, giữ form, không có raw error');
    const rejected = page.waitForResponse((response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/api/v1/connections/kubernetes', { timeout: 60_000 });
    await h.click(ui('connections', 'submit'));
    const rejectedResponse = await rejected;
    expect(rejectedResponse.status(), 'real verifier rejects the synthetic endpoint').toBe(422);
    expect(await rejectedResponse.text()).not.toContain(TOKEN);
    await expect(alert()).toContainText('Không xác minh được cluster nên chưa lưu kết nối.');
    await expect(name()).toHaveValue('Tên gõ lúc chờ');
    await expect(content()).toHaveClass(/masked-text/);
    await expect(summary()).toContainText('Context ctx-b');
    expect(registers.length, 'exactly one explicit register').toBe(1);
    expect(registers[0]).toEqual({ name: 'Tên gõ lúc chờ', kubeconfig: docB, context: 'ctx-b' });
    expect((await connectionsApi()).some((item) => item.name === 'Tên gõ lúc chờ'), 'nothing saved').toBe(false);
    await noLeak();
    check('failed verification (real fake backend, real kubectl verifier, 422): fixed Vietnamese guidance, form/name/masked content kept, payload only {name,kubeconfig,context}, nothing saved');
    mark('verification-failed');
    await h.moveTo(alert());
    await h.read('Xác minh thất bại: chỉ hướng dẫn cố định, không lộ lỗi gốc');

    caption('Bước 10: Dán nội dung không phải kubeconfig; inspect lỗi (400 thật) hiện hướng dẫn Việt, có nút Kiểm tra lại, không tự thử lại');
    await h.click(content());
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Delete');
    const failedInspect = inspectReply();
    await h.paste(content(), 'đây không phải kubeconfig');
    expect((await failedInspect).status()).toBe(400);
    await expect(alert()).toContainText('Không đọc được kubeconfig');
    await expect(ui('connections', 'retryInspect')).toBeVisible();
    const afterFail = inspects.length;
    await page.waitForTimeout(1500);
    expect(inspects.length, 'no automatic retry').toBe(afterFail);
    const retried = inspectReply();
    await h.click(ui('connections', 'retryInspect'));
    expect((await retried).status()).toBe(400);
    expect(inspects.length, 'explicit retry sends one more inspect').toBe(afterFail + 1);
    await expect(name()).toHaveValue('Tên gõ lúc chờ');
    check('inspect failure (real backend 400): Vietnamese guidance without raw text, retry only on click, no loop, name kept');
    mark('inspect-retry');
    await h.moveTo(alert());
    await h.read('Chỉ khi bấm Kiểm tra lại mới gửi inspect thêm');

    caption('Bước 11: Dán lại document hợp lệ rồi lưu; POST đăng ký bị MOCK trả 201 (fake backend không có verifier thành công); form được xóa sạch');
    await h.click(content());
    await page.keyboard.press('Control+A');
    await page.keyboard.press('Delete');
    const okInspect = inspectReply();
    await h.paste(content(), docB);
    await okInspect;
    await expect(summary()).toContainText('Cluster cluster-b');
    mockSave = true;
    await h.click(ui('connections', 'submit'));
    await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký kết nối Tên gõ lúc chờ (cum-thu-t06).' })).toBeVisible();
    expect(registers.length).toBe(2);
    await expect(name()).toHaveValue('');
    await expect(content()).toHaveValue('');
    await expect(summary()).toHaveCount(0);
    const row = table.locator('tbody tr').filter({ hasText: 'Tên gõ lúc chờ' });
    await expect(row).toContainText('Sẵn sàng');
    await expect(row).toContainText('cum-thu-t06');
    await noLeak();
    check('explicit save success (MOCKED 201 + list projection): notice, name/content/summary cleared, READY row "Sẵn sàng" listed, no token on screen');
    mark('registered');
    await h.moveTo(row);
    await h.read('Đăng ký chỉ chạy khi bấm Kiểm tra và lưu; form được xóa sạch');

    caption('Bước 12: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T07: Cài đặt môi trường. Developer + Platform Engineer, fake adapters, bộ nhớ memory credential store.
  // Backend thật (fake adapters) trả lời mọi thao tác đọc/ghi sản phẩm. Các MOCK (page.route) chỉ dùng để dựng
  // trạng thái mà backend fake không tạo được: chỉ-1/không READY choice, copy failure/stale/busy, usedBy, và
  // projection transition/operation đã chạy. Mỗi MOCK được ghi chú ở chỗ dùng.
  async T07({ page, h, ui, caption, mark, check, baseURL }) {
    const GOOD_TOKEN = 'synthetic-scoped-token-t07';
    const SECRET_VALUE = 'synthetic-t07-secret-value-0123';
    const APP_NAME = 'T07 Settings';
    const writes = []; // every non-GET request that can persist an Environment binding or configuration
    page.on('request', (request) => {
      const { pathname } = new URL(request.url());
      if (request.method() !== 'GET' && /\/api\/v1\/applications\/[^/]+\/environments\//.test(pathname)) writes.push(`${request.method()} ${pathname.replace(/^.*\/environments\//, '')}`);
    });
    const countWrites = (needle) => writes.filter((line) => line.includes(needle)).length;

    // ---- MOCK state -------------------------------------------------------------------
    const mock = {
      connections: 'real', // 'real' | 'none' | 'multi' | 'other-only'  (projection of /application-connections)
      stores: 'real', // 'real' | 'none'
      storePut: [], // queue of one-shot replies for PUT .../secret-store: 'copy' | 'stale' | 'busy'
      configPut: [], // queue for PUT .../configuration/keys/*: 'stale'
      usedBy: false, // inject usedBy ['web'] on LOG_LEVEL
      envPatch: {}, // Environment projection patched into GET /applications/{id}
      gate: { armed: false, promise: Promise.resolve(), release: () => {} },
      transition: { phase: 'none', polls: 0 },
    };
    const EXTRA_READY = { key: 'lab-cluster', name: 'Cluster lab', kind: 'KUBERNETES', status: 'READY' };
    const EXTRA_PENDING = { key: 'pending-cluster', name: 'Cluster chờ', kind: 'KUBERNETES', status: 'PENDING' };
    const EXTRA_ODD = { key: 'odd-target', name: 'Đích lạ', kind: 'OTHER', status: 'READY' };
    const json = (status, body) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });
    await page.route('**/api/v1/application-connections', async (route) => {
      if (mock.connections === 'real') { await route.continue(); return; }
      if (mock.connections === 'none') { await route.fulfill(json(200, { connections: [] })); return; }
      const response = await route.fetch();
      const data = await response.json();
      // multi: the real list plus one more READY Connection and two that must be filtered out (not READY, unknown kind).
      if (mock.connections === 'multi') { await route.fulfill({ response, json: { ...data, connections: [...data.connections, EXTRA_READY, EXTRA_PENDING, EXTRA_ODD] } }); return; }
      await route.fulfill({ response, json: { ...data, connections: [EXTRA_READY] } });
    });
    await page.route('**/api/v1/secret-store-choices', async (route) => {
      if (mock.stores === 'real') { await route.continue(); return; }
      await route.fulfill(json(200, { secretStores: [] }));
    });
    await page.route('**/api/v1/applications/*/environments/*/secret-store', async (route) => {
      const mode = route.request().method() === 'PUT' ? mock.storePut.shift() : undefined;
      if (!mode) { await route.continue(); return; }
      if (mode === 'copy') await route.fulfill(json(502, { error: 'copy failed', code: 'SECRET_COPY_FAILED' }));
      else if (mode === 'stale') await route.fulfill(json(409, { error: 'the environment changed since it was loaded; reload and try again', code: 'STALE_VERSION' }));
      else await route.fulfill(json(409, { error: 'the environment is busy with another operation', code: 'ENVIRONMENT_BUSY' }));
    });
    await page.route('**/api/v1/applications/*/environments/*/configuration/keys/*', async (route) => {
      if (route.request().method() !== 'PUT') { await route.continue(); return; }
      if (mock.configPut[0] === 'stale') { mock.configPut.shift(); await route.fulfill(json(409, { error: 'the configuration changed since it was loaded', code: 'STALE_VERSION' })); return; }
      if (mock.gate.armed) { mock.gate.armed = false; await mock.gate.promise; }
      await route.continue();
    });
    await page.route('**/api/v1/applications/*/environments/*/configuration', async (route) => {
      if (route.request().method() !== 'GET' || !mock.usedBy) { await route.continue(); return; }
      const response = await route.fetch();
      const data = await response.json();
      await route.fulfill({ response, json: { ...data, keys: data.keys.map((key) => key.name === 'LOG_LEVEL' ? { ...key, usedBy: ['web'] } : key) } });
    });
    await page.route('**/api/v1/applications/*', async (route) => {
      if (route.request().method() !== 'GET' || Object.keys(mock.envPatch).length === 0) { await route.continue(); return; }
      const response = await route.fetch();
      const data = await response.json();
      if (data.application?.environments) data.application.environments = data.application.environments.map((env) => ({ ...env, ...(mock.envPatch[env.key] ?? {}) }));
      await route.fulfill({ response, json: data });
    });
    // MOCK transition API (the fake backend has no deployed runtime to move): preview, execute, progress, cleanup, recovery.
    const stage = (name, status, message) => ({ stage: name, status, message, startedAt: '2026-10-11T00:00:00Z' });
    const endpoint = (key, name, generation, namespace) => ({ connectionKey: key, connectionName: name, executionProfile: 'internal-k8s', generation, namespace });
    const SOURCE = endpoint('internal-cluster', 'Cluster nội bộ', 0, 't07-ns');
    const DEST = endpoint('lab-cluster', 'Cluster lab', 1, 't07-ns-g1');
    const detail = (over) => ({ id: 'tr-t07', operationId: 'op-t07', mode: 'MIGRATE_POSTGRES', stage: 'BACKUP', status: 'RUNNING', source: SOURCE, destination: DEST, sourceState: 'AUTHORITATIVE', authority: 'SOURCE', stages: [stage('PREFLIGHT', 'SUCCEEDED')], canCleanupSource: false, createdAt: '', updatedAt: '', ...over });
    const finished = () => detail({ status: 'SUCCEEDED', stage: 'CUTOVER', sourceState: 'RETAINED_QUIESCED', authority: 'DESTINATION', stages: [stage('PREFLIGHT', 'SUCCEEDED'), stage('BACKUP', 'SUCCEEDED'), stage('CUTOVER', 'SUCCEEDED')], canCleanupSource: true });
    const cleaned = () => ({ ...finished(), sourceState: 'CLEANED', canCleanupSource: false });
    await page.route('**/connection-transition/preview', async (route) => {
      await route.fulfill(json(200, { preview: { token: 'tok-t07', mode: 'MIGRATE_POSTGRES', source: SOURCE, destination: DEST, downtimeRequired: true, notes: [], mappings: [{ sourceDescriptor: 'postgres.default#shared.db', destinationDescriptor: 'postgres.default#shared.db' }], workloads: [{ workloadId: 'web', action: 'UNCHANGED', configChanged: true }, { workloadId: 'api', action: 'UPDATED' }, { workloadId: 'batch', action: 'REMOVED' }] } }));
    });
    await page.route('**/connection-transitions**', async (route) => {
      const request = route.request();
      const { pathname } = new URL(request.url());
      if (request.method() === 'POST' && pathname.endsWith('/cleanup-source')) { mock.transition.phase = 'cleaned'; await route.fulfill(json(200, { transition: cleaned() })); return; }
      if (request.method() === 'POST') { mock.transition = { phase: 'running', polls: 0 }; await route.fulfill(json(202, { transition: detail({}) })); return; }
      if (mock.transition.phase === 'none') { await route.fulfill(json(200, { transitions: [] })); return; }
      if (pathname.endsWith('/tr-t07')) {
        mock.transition.polls += 1;
        if (mock.transition.phase === 'running' && mock.transition.polls >= 3) { mock.transition.phase = 'done'; mock.envPatch.staging = { connectionKey: 'lab-cluster', connectionName: 'Cluster lab', connectionKind: 'KUBERNETES', targetGeneration: 1, runtimeExists: true, configured: true }; }
        await route.fulfill(json(200, { transition: mock.transition.phase === 'running' ? detail({ stage: 'RESTORING', stages: [stage('PREFLIGHT', 'SUCCEEDED'), stage('BACKUP', 'SUCCEEDED'), stage('RESTORING', 'RUNNING')] }) : mock.transition.phase === 'cleaned' ? cleaned() : finished() }));
        return;
      }
      await route.fulfill(json(200, { transitions: [mock.transition.phase === 'running' ? detail({}) : mock.transition.phase === 'cleaned' ? cleaned() : finished()] }));
    });
    await page.route('**/operations/op-t07/recover', async (route) => {
      mock.envPatch = {};
      await route.fulfill(json(200, { recovery: { operationId: 'op-t07', kind: 'TRANSITION', outcome: 'đã khôi phục nguồn nhưng còn bước cần xử lý', compensationFailed: ['dừng workload đích web'] } }));
    });

    // ---- helpers ----------------------------------------------------------------------
    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await h.click(ui('signIn', 'submit'));
      await expect(ui('applications', 'heading')).toBeVisible();
    };
    const signOut = async () => { await h.click(ui('shell', 'signOut')); await expect(ui('signIn', 'username')).toBeVisible(); };
    const registerStore = async (name, address) => {
      await h.click(ui('shell', 'navSecretStores'));
      await expect(ui('secretStores', 'heading')).toBeVisible();
      await h.type(ui('secretStores', 'name'), name);
      await h.type(ui('secretStores', 'backend'), address);
      await h.click(ui('secretStores', 'showAdvanced'));
      await h.click(ui('secretStores', 'sameAddress'));
      await h.paste(ui('secretStores', 'token'), GOOD_TOKEN);
      await h.click(ui('secretStores', 'submit'));
      await expect(page.getByRole('status').filter({ hasText: `Kho bí mật "${name}" đã sẵn sàng` })).toBeVisible();
    };
    const envApi = async (key) => {
      const response = await page.request.get(`${baseURL}/api/v1/applications/${appId}`);
      return (await response.json()).application.environments.find((env) => env.key === key);
    };
    const openSettings = async (environment = 'Staging') => {
      await h.click(page.getByRole('button', { name: new RegExp(APP_NAME) }));
      await expect(page.getByRole('heading', { name: APP_NAME })).toBeVisible();
      await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
      await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
      if (environment !== 'Staging') await h.click(page.getByRole('tab', { name: environment }));
    };
    const tab = async (name) => { await h.click(page.getByRole('tab', { name })); await expect(page.getByRole('tab', { name })).toHaveAttribute('aria-selected', 'true'); };
    const connectionPanel = () => page.getByLabel('Kết nối triển khai', { exact: true });
    const storePanel = () => page.getByLabel('Kho bí mật', { exact: true });
    const connSelect = (title) => page.getByLabel(`Kết nối cho ${title}`);
    const storeSelect = (title) => page.getByLabel(`Kho bí mật cho ${title}`);
    const suggestion = () => page.getByText(/Gợi ý chưa lưu/);
    const sectionOf = (label) => page.locator(`section[aria-label="${label}"]`);
    const row = (name) => page.locator('.settings-table-row').filter({ has: page.locator('strong', { hasText: new RegExp(`^${name}$`) }) });
    const noLeak = async () => { const { assertNoSecretLeaks } = await import('./secret-scan.mjs'); await assertNoSecretLeaks(page, [SECRET_VALUE, GOOD_TOKEN], { allow: ['input[type="password"]'] }); };
    const addVariable = async (name, value) => {
      await h.click(sectionOf('Biến').getByRole('button', { name: '+ Thêm biến' }));
      await h.type(page.getByLabel('Tên key'), name);
      await h.type(page.getByLabel('Giá trị', { exact: true }), value);
      await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
      await expect(row(name)).toBeVisible();
    };
    let appId = '';

    // ---- 1. Developer prepares an application with no binding --------------------------
    caption('Bước 1: Nhà phát triển đăng nhập và tạo ứng dụng; chưa gán kết nối nào cho staging hay production');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signInAs('developer');
    appId = await createApplication(h, APP_NAME, 't07-settings', { connections: null });
    expect((await envApi('staging')).configured, 'staging starts unconfigured').toBeFalsy();
    check(`developer created ${appId} through the UI; both Environments unconfigured`);
    mark('app-created');
    await h.read('Ứng dụng mới chưa có kết nối hay kho bí mật');

    // ---- 2. One READY connection (real list): suggested draft, nothing persisted ---------------
    caption('Bước 2: Trang Cài đặt môi trường; backend fake chỉ có một kết nối Sẵn sàng nên staging được gợi ý nó trong bản nháp chưa lưu');
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Staging' })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Production' })).toBeVisible();
    await expect(connSelect('Staging')).toHaveValue('internal-cluster');
    await expect(suggestion()).toContainText('không phải mặc định của tổ chức');
    await expect(connectionPanel()).toContainText('Chưa cấu hình');
    expect(await connSelect('Staging').locator('option').count(), 'placeholder + the one READY connection').toBe(2);
    expect(countWrites('/connection'), 'nothing persisted yet').toBe(0);
    expect((await envApi('staging')).configured, 'API still unconfigured: suggestion is only a draft').toBeFalsy();
    check('one READY (real fake backend): draft suggested with the unsaved / not-an-Organization-default note; 0 binding writes; API unconfigured');
    mark('single-suggested-real');
    await h.moveTo(suggestion());
    await h.read('Gợi ý chỉ là bản nháp, không phải mặc định hay fallback của tổ chức');

    // ---- 3. Secret store: none READY ----------------------------------------------------
    caption('Bước 3: Chưa có kho bí mật nào; trang hướng dẫn đăng ký kho và cho thử lại. Biến vẫn thêm được, Bí mật thì chưa');
    await expect(storePanel()).toContainText('Chưa có kho bí mật Sẵn sàng');
    await expect(storePanel().getByRole('button', { name: 'Thử lại' })).toBeVisible();
    await h.click(sectionOf('Bí mật').getByRole('button', { name: '+ Thêm bí mật' }));
    await expect(page.getByText(/Hãy chọn kho bí mật ở trên trước khi thêm Bí mật/)).toBeVisible();
    await expect(page.getByLabel('Tên key')).toHaveCount(0);
    await addVariable('LOG_LEVEL', 'debug');
    await expect(row('LOG_LEVEL')).toContainText('debug');
    check('no READY store (real): guidance + Thử lại, Secret add blocked, ordinary variable saved');
    mark('no-store');
    await h.read('Không có kho: hướng dẫn đăng ký, Bí mật bị chặn, Biến vẫn lưu được');

    // ---- 4. Connections: none (MOCK) then retry -----------------------------------------
    caption('Bước 4: (MOCK danh sách rỗng) Không có kết nối Sẵn sàng; hướng dẫn nhờ Kỹ sư nền tảng và Thử lại sẽ tải lại danh sách');
    mock.connections = 'none';
    await tab('Production');
    await tab('Staging');
    await expect(connectionPanel()).toContainText('Chưa có kết nối Sẵn sàng');
    await expect(connSelect('Staging')).toHaveCount(0);
    await h.click(connectionPanel().getByRole('button', { name: 'Thử lại' }));
    await expect(connectionPanel()).toContainText('Chưa có kết nối Sẵn sàng');
    mock.connections = 'real';
    await h.click(connectionPanel().getByRole('button', { name: 'Thử lại' }));
    await expect(connSelect('Staging')).toHaveValue('internal-cluster');
    expect(countWrites('/connection'), 'retry persisted nothing').toBe(0);
    check('no READY connection (MOCKED empty projection): guidance, retry reloads; real list restores the suggestion; 0 binding writes');
    mark('no-connection');
    await h.read('Thử lại chỉ tải lại danh sách, không đổi binding');

    // ---- 5. Several READY connections (MOCK): explicit choice and Save ---------------------------
    caption('Bước 5: (MOCK thêm một kết nối Sẵn sàng, một Đang chờ, một loại lạ) Có nhiều lựa chọn hợp lệ nên không điền sẵn; chọn tường minh rồi Lưu');
    mock.connections = 'multi';
    await tab('Production');
    await tab('Staging');
    await expect(connSelect('Staging')).toHaveValue('');
    await expect(suggestion()).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
    expect(await connSelect('Staging').locator('option').count(), 'placeholder + 2 eligible READY (pending/unknown kind filtered)').toBe(3);
    await expect(connSelect('Staging').locator('option', { hasText: 'pending-cluster' })).toHaveCount(0);
    await expect(connSelect('Staging').locator('option', { hasText: 'odd-target' })).toHaveCount(0);
    mark('multiple-choices');
    await h.moveTo(connSelect('Staging'));
    await h.read('Có nhiều lựa chọn hợp lệ thì người dùng tự chọn; kết nối chưa sẵn sàng hoặc loại lạ bị lọc');
    const stagingOption = (await connSelect('Staging').locator('option').allTextContents()).find((text) => text.includes('(internal-cluster)'));
    await h.choose(connSelect('Staging'), stagingOption);
    await expect(connSelect('Staging')).toHaveValue('internal-cluster');
    expect(countWrites('/connection'), 'selecting alone persists nothing').toBe(0);
    expect((await envApi('staging')).configured, 'API still unconfigured before Save').toBeFalsy();
    const putStaging = page.waitForRequest((request) => request.method() === 'PUT' && request.url().endsWith('/environments/staging/connection'));
    await h.click(page.getByRole('button', { name: 'Lưu kết nối' }));
    expect((await putStaging).postDataJSON()).toEqual({ connectionKey: 'internal-cluster', expectedVersion: 1 });
    await expect(connectionPanel()).toContainText('Thế hệ 0');
    await expect(connectionPanel()).toContainText('internal-k8s');
    await expect(connectionPanel().getByRole('combobox')).toHaveCount(1);
    expect((await envApi('staging')).connectionKey).toBe('internal-cluster');
    check('multiple eligible (MOCKED list, real PUT): no preselection, ineligible filtered, explicit Save sent {connectionKey, expectedVersion:1}; profile read-only');
    mark('staging-saved');
    await h.moveTo(connectionPanel());
    await h.read('Chỉ nút Lưu mới đổi binding; phạm vi triển khai là thông tin chỉ đọc');

    // ---- 6. Production: single READY suggestion survives tab switch and reload ----------------------
    caption('Bước 6: Danh sách thật chỉ có một kết nối Sẵn sàng; production được gợi ý lại, rời tab và tải lại trang vẫn chưa lưu');
    mock.connections = 'real';
    await tab('Production');
    await expect(connSelect('Production')).toHaveValue('internal-cluster');
    await expect(suggestion()).toContainText('không phải mặc định của tổ chức');
    await tab('Staging');
    await tab('Production');
    await page.reload();
    await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
    await expect(page.getByRole('tab', { name: 'Production' })).toHaveAttribute('aria-selected', 'true');
    await expect(connSelect('Production')).toHaveValue('internal-cluster');
    expect((await envApi('production')).configured, 'still unsaved after reload').toBeFalsy();
    mark('production-draft');
    await h.read('Gợi ý chỉ được tính lại từ danh sách; chưa có binding nào được ghi');
    await h.click(page.getByRole('button', { name: 'Lưu kết nối' }));
    await expect(connectionPanel()).toContainText('Thế hệ 0');
    await expect(suggestion()).toHaveCount(0);
    expect((await envApi('production')).connectionKey).toBe('internal-cluster');
    check('production draft only a derived suggestion across tab switch/reload (API unconfigured); explicit Save wrote the binding');
    mark('production-saved');
    await h.read('Chỉ sau khi bấm Lưu production mới có binding');

    caption('Bước 7: (MOCK chỉ còn Cluster lab) Binding đã cấu hình của production không bao giờ bị đổi sang lựa chọn duy nhất khác');
    mock.connections = 'other-only';
    await tab('Staging');
    await tab('Production');
    await expect(connSelect('Production')).toHaveValue('internal-cluster');
    await expect(suggestion()).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
    expect((await envApi('production')).connectionKey).toBe('internal-cluster');
    mock.connections = 'real';
    check('bound Environment never retargeted although exactly one other READY connection is listed; Save disabled (same-key no-op)');
    mark('bound-not-retargeted');
    await h.read('Đã có binding thì không có gợi ý nào thay được binding đó');

    // ---- 9. Stores: one READY store suggested ---------------------------------------------
    caption('Bước 9: Kỹ sư nền tảng đăng ký Kho T07 A (backend fake, token chỉ nhập vào ô được che)');
    await signOut();
    await signInAs('platform-engineer');
    await registerStore('Kho T07 A', 'http://vault-t07-a.local:8200');
    await noLeak();
    check('Kho T07 A registered through the UI (real fake backend); token never on screen');
    mark('store-a-registered');
    await signOut();

    caption('Bước 10: Nhà phát triển mở lại cài đặt; chỉ có một kho Sẵn sàng nên được gợi ý trong draft chưa lưu, rồi bấm Lưu kho bí mật');
    await signInAs('developer');
    await openSettings();
    await expect(storeSelect('Staging')).toHaveValue(/.+/);
    await expect(storePanel().getByText(/Gợi ý chưa lưu/)).toContainText('không phải mặc định của tổ chức');
    await expect(storePanel()).toContainText('Chưa chọn');
    expect((await envApi('staging')).secretStoreKey ?? '', 'store not persisted by suggestion').toBe('');
    expect(countWrites('/secret-store'), 'no store write before Save').toBe(0);
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel()).toContainText('Đã lưu kho bí mật');
    await expect(storePanel().locator('.pending-pill')).toContainText('Kho T07 A');
    const storeAKey = (await envApi('staging')).secretStoreKey;
    expect(storeAKey).toBeTruthy();
    check('one READY store (real): unsaved suggestion, API unchanged until Save, then explicit Save bound it');
    mark('store-a-saved');
    await h.read('Gợi ý một kho chỉ là draft cho đến khi Lưu');

    // ---- 11. Secret forms ----------------------------------------------------------------
    caption('Bước 11: Thêm Bí mật API_TOKEN; ô nhập bị che, sau khi lưu chỉ hiện "Đã cấu hình" và không bao giờ đọc lại giá trị');
    await h.click(sectionOf('Bí mật').getByRole('button', { name: '+ Thêm bí mật' }));
    await expect(page.getByLabel('Giá trị bí mật mới')).toHaveAttribute('type', 'password');
    await h.type(page.getByLabel('Tên key'), 'API_TOKEN');
    await h.paste(page.getByLabel('Giá trị bí mật mới'), SECRET_VALUE);
    await noLeak();
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expect(row('API_TOKEN')).toContainText('Đã cấu hình');
    await noLeak();
    await h.click(row('API_TOKEN').getByRole('button', { name: 'Cập nhật' }));
    await expect(page.getByLabel('Giá trị bí mật mới')).toHaveValue('');
    await h.click(page.getByRole('button', { name: 'Hủy' }));
    check('secret: masked input, saved row shows only "Đã cấu hình", update form starts empty, value never in page text/attributes/storage');
    mark('secret-saved');
    await h.read('Giá trị bí mật chỉ nhập vào, không bao giờ hiển thị lại');

    // ---- 12. Second store: bound stays, same-key no-op, explicit choice ---------------------
    caption('Bước 12: Đăng ký thêm Kho T07 B. Staging đã gán kho A nên không bị đổi; production chưa có kho phải tự chọn');
    await signOut();
    await signInAs('platform-engineer');
    await registerStore('Kho T07 B', 'http://vault-t07-b.local:8200');
    await signOut();
    await signInAs('developer');
    await openSettings();
    await expect(storeSelect('Staging')).toHaveValue(storeAKey);
    await expect(page.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
    await tab('Production');
    await expect(storeSelect('Production')).toHaveValue('');
    await expect(storePanel().getByText(/Gợi ý chưa lưu/)).toHaveCount(0);
    await expect(page.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
    check('two READY stores: bound staging store unchanged and Save disabled (no copy for the same key); production not preselected');
    mark('two-stores');
    await h.read('Nhiều kho thì tự chọn; chọn lại cùng kho không sao chép');

    const prodStoreB = (await storeSelect('Production').locator('option').allTextContents()).find((text) => text.startsWith('Kho T07 B'));
    await h.choose(storeSelect('Production'), prodStoreB);
    expect((await envApi('production')).secretStoreKey ?? '').toBe('');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel()).toContainText('Đã lưu kho bí mật');
    expect((await envApi('production')).secretStoreKey).toBeTruthy();
    check('production: explicit choice + Save bound store B; nothing before Save');
    mark('production-store-b');
    await tab('Staging');

    // ---- 13. Copy failure / stale / busy keep pending text -----------------------------------
    caption('Bước 13: Mở form biến và gõ dở, rồi đổi kho A sang B. (MOCK) lỗi sao chép, stale, BUSY; kho cũ giữ nguyên và chữ đang gõ không mất');
    await h.click(sectionOf('Biến').getByRole('button', { name: '+ Thêm biến' }));
    await h.type(page.getByLabel('Tên key'), 'DRAFT_NOTE');
    await h.type(page.getByLabel('Giá trị', { exact: true }), 'đang gõ dở');
    const storeB = (await storeSelect('Staging').locator('option').allTextContents()).find((text) => text.startsWith('Kho T07 B'));
    await h.choose(storeSelect('Staging'), storeB);
    mock.storePut.push('copy');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel().getByRole('alert')).toContainText('Kho cũ vẫn đang được chọn và không có gì thay đổi');
    expect((await envApi('staging')).secretStoreKey).toBe(storeAKey);
    await expect(page.getByLabel('Giá trị', { exact: true })).toHaveValue('đang gõ dở');
    check('copy failure (MOCKED 502): Vietnamese message, API keeps store A, open variable text kept');
    mark('copy-failure');
    await h.moveTo(storePanel().getByRole('alert'));
    await h.read('Sao chép lỗi thì kho cũ giữ nguyên');

    mock.storePut.push('stale');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel().getByRole('alert')).toContainText('Đã tải lại lựa chọn mới nhất');
    await expect(storeSelect('Staging')).toHaveValue(/.+/);
    await expect(page.getByLabel('Giá trị', { exact: true })).toHaveValue('đang gõ dở');
    mock.storePut.push('busy');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel().getByRole('alert')).toContainText('Môi trường đang bận');
    expect(countWrites('/secret-store') >= 3).toBe(true);
    check('stale (MOCKED 409) and BUSY (MOCKED 409): messages shown, draft choice and typed text kept, no store change');
    mark('stale-busy');
    await h.read('Stale hoặc BUSY không xóa lựa chọn và chữ đang gõ');

    caption('Bước 14: Lưu thật A sang B: bí mật được sao chép trước khi commit; làm mới cấu hình không xóa form đang mở');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel()).toContainText('1 bí mật đã được sao chép và xác minh');
    expect((await envApi('staging')).secretStoreKey).not.toBe(storeAKey);
    await expect(page.getByLabel('Giá trị', { exact: true })).toHaveValue('đang gõ dở');
    await expect(page.getByLabel('Tên key')).toHaveValue('DRAFT_NOTE');
    await h.click(page.getByRole('button', { name: 'Hủy' }));
    check('real store change A->B: 1 secret copied+verified; configuration reload kept the open editor text');
    mark('store-changed');
    await h.read('Sao chép được xác minh rồi mới đổi kho');

    // ---- 15. Late reply across Environment switch -------------------------------------------
    caption('Bước 15: Bấm lưu một biến ở staging nhưng phản hồi bị giữ (MOCK); chuyển sang production rồi thả phản hồi; production không bị lẫn dữ liệu staging');
    mock.gate.promise = new Promise((resolve) => { mock.gate.release = resolve; });
    mock.gate.armed = true;
    await h.click(sectionOf('Biến').getByRole('button', { name: '+ Thêm biến' }));
    await h.type(page.getByLabel('Tên key'), 'STAGING_LATE');
    await h.type(page.getByLabel('Giá trị', { exact: true }), 'muộn');
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await tab('Production');
    await expect(sectionOf('Biến')).toBeVisible();
    mock.gate.release();
    await page.waitForTimeout(1500);
    await expect(row('STAGING_LATE')).toHaveCount(0);
    await expect(row('LOG_LEVEL')).toHaveCount(0);
    await tab('Staging');
    await expect(row('STAGING_LATE')).toBeVisible();
    check('late save reply of staging did not appear on production; staging shows the saved key after returning');
    mark('late-reply');
    await h.read('Phản hồi muộn không ghi sang môi trường khác');

    // ---- 16. Rename/delete warnings and BUSY/stale configuration -----------------------------
    caption('Bước 16: (MOCK usedBy) Đổi tên/Xóa key đang được workload dùng; hộp thoại cảnh báo tham chiếu không tự được cập nhật');
    mock.usedBy = true;
    await tab('Production');
    await tab('Staging');
    await h.click(row('LOG_LEVEL').getByRole('button', { name: 'Đổi tên' }));
    await expect(page.getByRole('dialog', { name: 'Đổi tên LOG_LEVEL' })).toContainText('Đang được tham chiếu bởi web. Các tham chiếu sẽ không được cập nhật tự động');
    await h.click(page.getByRole('button', { name: 'Hủy' }));
    await h.click(row('LOG_LEVEL').getByRole('button', { name: 'Xóa' }));
    await expect(page.getByRole('dialog', { name: 'Xóa LOG_LEVEL' })).toContainText('không được cập nhật tự động');
    await h.click(page.getByRole('button', { name: 'Hủy' }));
    mock.usedBy = false;
    check('rename/delete dialogs warn that references are not rewritten (MOCKED usedBy projection); both cancelled, nothing written');
    mark('rename-delete-warning');
    await h.read('T07 chỉ đổi nhãn; việc sửa tham chiếu thuộc về người dùng');

    caption('Bước 17: Đổi tên thật một key không được tham chiếu rồi xóa nó; phiên bản cấu hình tăng, giá trị kỹ thuật giữ nguyên');
    await addVariable('TEMP_NOTE', 'x1');
    await h.click(row('TEMP_NOTE').getByRole('button', { name: 'Đổi tên' }));
    await expect(page.getByRole('dialog', { name: 'Đổi tên TEMP_NOTE' })).toContainText('chỉ áp dụng cho Staging');
    await h.type(page.getByLabel('Tên key mới'), 'TEMP_RENAMED', { replace: true });
    await h.click(page.getByRole('button', { name: 'Tiếp tục' }));
    await expect(row('TEMP_RENAMED')).toContainText('x1');
    await h.click(row('TEMP_RENAMED').getByRole('button', { name: 'Xóa' }));
    await h.click(page.getByRole('button', { name: 'Tiếp tục' }));
    await expect(row('TEMP_RENAMED')).toHaveCount(0);
    check('real rename and delete of an unreferenced variable through the dialogs');
    mark('rename-delete-real');

    caption('Bước 18: (MOCK) Phiên bản cấu hình cũ bị 409; thông báo yêu cầu xem lại. Rồi một thao tác môi trường chạy: form giữ chữ, nút Lưu khóa');
    await h.click(sectionOf('Biến').getByRole('button', { name: '+ Thêm biến' }));
    await h.type(page.getByLabel('Tên key'), 'STALE_NOTE');
    await h.type(page.getByLabel('Giá trị', { exact: true }), 'giữ nguyên');
    mock.configPut.push('stale');
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expect(page.getByRole('alert').filter({ hasText: 'Đã tải lại giá trị mới nhất' })).toBeVisible();
    await expect(page.getByLabel('Giá trị', { exact: true })).toHaveValue('giữ nguyên');
    mock.envPatch.staging = { activeOperation: { id: 'op-busy', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOYING', startedAt: '2026-10-11T00:00:00Z', updatedAt: '2026-10-11T00:00:00Z', heartbeatAt: '2026-10-11T00:00:00Z', recoverable: false } };
    await expect(page.getByRole('heading', { name: 'Thao tác đang chạy' })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeDisabled();
    await expect(page.getByLabel('Giá trị', { exact: true })).toHaveValue('giữ nguyên');
    await expect(connSelect('Staging')).toBeDisabled();
    mark('busy-locked');
    await h.moveTo(page.getByRole('heading', { name: 'Thao tác đang chạy' }));
    await h.read('Đang BUSY: giữ nội dung form nhưng không cho gửi');
    mock.envPatch = {};
    await expect(page.getByRole('heading', { name: 'Thao tác đang chạy' })).toHaveCount(0, { timeout: 15_000 });
    await expect(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeEnabled();
    await expect(page.getByLabel('Giá trị', { exact: true })).toHaveValue('giữ nguyên');
    await h.click(page.getByRole('button', { name: 'Hủy' }));
    check('stale configuration 409 (MOCKED) kept the editor; polled BUSY (MOCKED projection) disabled submit and kept text; release re-enabled it');
    mark('busy-released');

    // ---- 19. Transition: preview, downtime, progress, cleanup ---------------------------------
    caption('Bước 19: (MOCK projection đã có runtime) Chọn kết nối khác thì phải xem lại chuyển đích; không có nút Lưu im lặng');
    mock.envPatch.staging = { runtimeExists: true };
    mock.connections = 'multi';
    await tab('Production');
    await tab('Staging');
    await expect(page.getByRole('region', { name: 'Chuyển đích kết nối' })).toBeVisible({ timeout: 15_000 });
    const labOption = (await connSelect('Staging').locator('option').allTextContents()).find((text) => text.includes('(lab-cluster)'));
    await h.choose(connSelect('Staging'), labOption);
    await expect(page.getByRole('button', { name: 'Lưu kết nối' })).toHaveCount(0);
    const before = countWrites('staging/connection');
    await h.click(page.getByRole('button', { name: 'Xem lại chuyển đích…' }));
    await expect(page.getByLabel('Kết nối đích')).toHaveValue('lab-cluster');
    expect(countWrites('staging/connection'), 'reviewing the transition persists no binding').toBe(before);
    mark('transition-open');
    await h.read('Đã có runtime thì đổi đích phải qua Xem trước và xác nhận');

    caption('Bước 20: Xem trước chuyển đích: workload không đổi/cập nhật/bị gỡ, ánh xạ cơ sở dữ liệu; phải xác nhận downtime mới bắt đầu');
    const panel = page.getByRole('region', { name: 'Chuyển đích kết nối' });
    await h.click(panel.getByRole('button', { name: 'Xem trước chuyển đích' }));
    const impact = panel.getByLabel('Xem trước chuyển đích');
    await expect(impact).toContainText('Triển khai lại, không đổi · nhận cấu hình mới hơn');
    await expect(impact).toContainText('Triển khai lại kèm thay đổi nháp đang chờ');
    await expect(impact).toContainText('Bị gỡ — không có ở đích (nguồn không bị đụng tới)');
    await expect(impact.getByLabel('Ánh xạ cơ sở dữ liệu')).toContainText('postgres.default#shared.db → postgres.default#shared.db');
    await expect(impact.getByRole('button', { name: 'Bắt đầu chuyển đích' })).toBeDisabled();
    await h.moveTo(impact);
    await h.read('Preview liệt kê tác động; nút bắt đầu bị khóa cho đến khi xác nhận downtime');
    await h.click(impact.getByLabel(/ứng dụng sẽ bị dừng trong lúc dữ liệu được sao chép/));
    await h.click(impact.getByRole('button', { name: 'Bắt đầu chuyển đích' }));
    await expect(panel.getByLabel('Tiến trình chuyển đích')).toContainText('Chuyển đích đang chạy');
    await expect(panel.getByLabel('Tiến trình chuyển đích')).toContainText('Khôi phục dữ liệu', { timeout: 30_000 });
    mark('transition-running');
    await h.read('Tiến trình dùng nhãn tiếng Việt; mã stage kỹ thuật giữ nguyên');
    await expect(page.getByText('Đích đang chạy. Thế hệ nguồn được giữ lại ở trạng thái dừng')).toBeVisible({ timeout: 60_000 });
    await expect(panel.getByRole('button', { name: 'Dọn dẹp thế hệ nguồn' })).toBeVisible();
    check('transition (MOCKED API): preview impact in Vietnamese, downtime acknowledgement gate, progress, retained source with explicit cleanup action');
    mark('transition-done');
    await h.read('Nguồn được giữ lại cho đến khi bạn tự dọn dẹp');
    await h.click(panel.getByRole('button', { name: 'Dọn dẹp thế hệ nguồn' }));
    await expect(page.getByText('Đích đang chạy và thế hệ nguồn đã được dọn dẹp.')).toBeVisible();
    mark('source-cleaned');

    // ---- 21. Recovery of an interrupted operation (MOCK) ---------------------------------------
    caption('Bước 21: (MOCK) Một thao tác bị gián đoạn: chỉ khi xác nhận tiến trình cũ đã dừng mới khôi phục được');
    mock.envPatch.staging = { activeOperation: { id: 'op-t07', kind: 'TRANSITION', status: 'INTERRUPTED', stage: 'DEPLOYING', startedAt: '2026-10-11T00:00:00Z', updatedAt: '2026-10-11T00:00:00Z', heartbeatAt: '2026-10-11T00:00:00Z', recoverable: true } };
    const banner = page.getByRole('region', { name: 'Thao tác trên môi trường' });
    await expect(banner.getByRole('heading', { name: 'Thao tác bị gián đoạn' })).toBeVisible({ timeout: 15_000 });
    await expect(banner.getByRole('button', { name: 'Khôi phục môi trường' })).toBeDisabled();
    await h.moveTo(banner);
    await h.read('Heartbeat cũ chưa chứng minh tiến trình đã dừng');
    await h.click(banner.getByLabel('Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn'));
    await h.click(banner.getByRole('button', { name: 'Khôi phục môi trường' }));
    await expect(banner).toContainText('Kết quả khôi phục:');
    await expect(banner).toContainText('còn 1 bước cần xử lý thủ công');
    // Safe summary only: the raw remaining-step detail must not be shown.
    await expect(banner).not.toContainText('dừng workload đích web');
    check('recovery (MOCKED API): disabled until the stop confirmation, then a safe Vietnamese summary of the remaining manual step is shown; raw step detail is absent');
    mark('recovery');
    await h.moveTo(banner);
    await h.read('Khôi phục chỉ chạy sau xác nhận tường minh');
    await noLeak();

    caption('Bước 22: Đăng xuất; phiên bị thu hồi');
    await signOut();
    mark('signed-out');
    await h.pause(3000);
  },

  // T08: trang Workload editor. Developer (+ Kỹ sư nền tảng chỉ để đăng ký một Loại tài nguyên số/bool). Backend thật (fake adapters,
  // state tạm) trả lời mọi đọc/ghi sản phẩm; editor chỉ lưu draft, không Preview/Deploy. Các MOCK (page.route) chỉ dựng trạng thái
  // mà backend fake không tạo được và được ghi chú tại chỗ dùng: (1) một Bí mật API_TOKEN chèn vào catalog key (backend fake chưa có
  // kho bí mật), (2) một lần PUT workload trả 409 (stale), (3) projection Environment đang BUSY. Điều kiện thật: lưu draft, parse Score,
  // danh sách Workload và Loại tài nguyên đều do backend thật trả về.
  async T08({ page, h, ui, caption, mark, check, baseURL }) {
    const APP_NAME = 'T08 Editor';
    const KEY = 't08-queue';
    const editor = (key) => ui('workloadEditor', key);
    const putWorkloadCalls = []; // every PUT .../workloads/<id> the browser sent (incl. the mocked stale one)
    page.on('request', (request) => {
      const { pathname } = new URL(request.url());
      if (request.method() === 'PUT' && /\/environments\/staging\/workloads\/[^/]+$/.test(pathname)) putWorkloadCalls.push(decodeURIComponent(pathname.split('/').pop()));
    });
    const mock = { secretKey: false, stale: 0, envPatch: {} };
    const json = (status, body) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });
    // MOCK (1): add one Secret key to GET .../configuration so the picker's secret group has an entry that must stay unticked.
    await page.route('**/api/v1/applications/*/environments/*/configuration', async (route) => {
      if (route.request().method() !== 'GET' || !mock.secretKey) { await route.continue(); return; }
      const response = await route.fetch();
      const data = await response.json();
      await route.fulfill({ response, json: { ...data, keys: [...data.keys, { name: 'API_TOKEN', kind: 'SECRET', configured: true, usedBy: [] }] } });
    });
    // MOCK (2): the next PUT workload answers 409 STALE_VERSION without reaching the backend.
    await page.route('**/api/v1/applications/*/environments/*/workloads/*', async (route) => {
      if (route.request().method() === 'PUT' && mock.stale > 0) { mock.stale -= 1; await route.fulfill(json(409, { error: 'workloads changed since they were loaded; reload and try again', code: 'STALE_VERSION' })); return; }
      await route.continue();
    });
    // MOCK (3): Environment projection patched while a scenario step needs a BUSY environment.
    await page.route('**/api/v1/applications/*', async (route) => {
      if (route.request().method() !== 'GET' || Object.keys(mock.envPatch).length === 0) { await route.continue(); return; }
      const response = await route.fetch();
      const data = await response.json();
      if (data.application?.environments) data.application.environments = data.application.environments.map((env) => ({ ...env, ...(mock.envPatch[env.key] ?? {}) }));
      await route.fulfill({ response, json: data });
    });

    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await h.click(ui('signIn', 'submit'));
      await expect(ui('applications', 'heading')).toBeVisible();
    };
    const note = (name) => page.getByRole('note', { name, exact: true });
    const storedScore = async (appId, id) => {
      const data = await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/workloads`)).json();
      return data.workloads.find((item) => item.id === id)?.score;
    };
    const openEditor = async () => {
      await h.click(page.getByRole('button', { name: '+ Thêm Workload' }));
      await expect(page.getByRole('heading', { name: 'Thêm Workload', level: 1 })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'Thông tin cơ bản' })).toBeVisible();
    };
    const saveDraft = async (id) => {
      const saved = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname.endsWith(`/workloads/${encodeURIComponent(id)}`));
      await h.click(editor('save'));
      const response = await saved;
      expect(response.status(), `PUT workload ${id}`).toBe(200);
      await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
      return response.request().postDataJSON().score;
    };
    const portNames = () => editor('portName');
    const portNumbers = () => editor('portNumber');
    const routeSelects = () => editor('routePort');
    const invalidRoute = (port) => page.locator('p.form-error').filter({ hasText: `Cổng Service "${port}" không còn hợp lệ` });
    const groupOf = (title) => page.getByRole('group', { name: `${title} cho main`, exact: true });

    // ---- Setup (UI): Kỹ sư nền tảng đăng ký một Loại tài nguyên có input number/bool --------------------------------
    caption('Chuẩn bị: Kỹ sư nền tảng sao chép schema postgres thành loại t08-queue có input kiểu số và bool (qua UI)');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signInAs('platform-engineer');
    await h.click(ui('shell', 'navResourceTypes'));
    await expect(ui('resourceTypes', 'heading')).toBeVisible();
    await h.click(page.getByRole('button', { name: 'Xem schema postgres' }));
    await h.click(page.getByRole('button', { name: 'Sao chép schema postgres' }));
    await h.type(page.getByLabel('Đầu vào 1 tên'), 'replicas', { replace: true });
    await h.choose(page.getByLabel('Đầu vào 1 kiểu'), 'number');
    await h.type(page.getByLabel('Đầu vào 2 tên'), 'durable', { replace: true });
    await h.choose(page.getByLabel('Đầu vào 2 kiểu'), 'bool');
    await h.click(page.getByLabel('Đầu vào 2 bắt buộc'));
    await h.type(ui('resourceTypes', 'id'), KEY);
    await h.click(ui('resourceTypes', 'submit'));
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký loại tài nguyên ${KEY}.` })).toBeVisible();
    const registered = (await (await page.request.get(`${baseURL}/api/v1/resource-types`)).json()).resourceTypes.find((item) => item.key === KEY);
    expect(registered.inputs).toEqual([{ name: 'replicas', type: 'number', required: true }, { name: 'durable', type: 'bool' }]);
    check(`setup via UI + backend GET: ${KEY} has inputs replicas:number (required) and durable:bool (optional)`);
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();

    caption('Chuẩn bị: Lập trình viên đăng nhập, tạo ứng dụng T08 và hai biến API_URL, LOG_LEVEL (qua UI)');
    await signInAs('developer');
    const appId = await createApplication(h, APP_NAME, 't08-editor');
    await putKeys(h, APP_NAME, [{ kind: 'variable', name: 'API_URL', value: 'https://api.t08.example' }, { kind: 'variable', name: 'LOG_LEVEL', value: 'info' }]);
    mark('prepared');
    mock.secretKey = true; // from here the picker also lists a Secret (MOCK 1)

    // ---- 1. Workload cơ bản: defaults, key aliases, cổng và đường dẫn công khai --------------------------------------
    caption('Bước 1: Mở form thêm Workload; nhãn tiếng Việt, Container đầu là main, phần nâng cao đóng và có tóm tắt defaults');
    await openEditor();
    await expect(page.getByRole('tab').or(page.getByRole('button', { name: 'Nhập bằng form' }))).toBeVisible();
    await expect(page.getByLabel('Tên Container')).toHaveValue('main');
    await expect(editor('cpuRequest')).toHaveCount(0);
    await expect(editor('resourceClass')).toHaveCount(0);
    await expect(editor('targetPort')).toHaveCount(0);
    await expect(note('Tóm tắt nâng cao Container 1')).toHaveText('Yêu cầu hiệu lực: CPU 10m (mặc định) · Bộ nhớ 32Mi (mặc định). Giới hạn: CPU chưa đặt · Bộ nhớ chưa đặt.');
    check('new editor: Vietnamese labels, container name main, advanced inputs closed, summary shows requests 10m/32Mi (default) and limits chưa đặt');
    mark('editor-basic');
    await h.read('Phần nâng cao đóng, tóm tắt defaults vẫn hiện; không tự thêm capacity');

    caption('Bước 2: Nhập tên, image; bộ chọn key không tự chọn biến hay bí mật nào; chọn API_URL và đặt tên Container BACKEND_URL');
    await h.type(editor('name'), 'web');
    await h.type(editor('image'), 'registry.example/t08-web:v1');
    for (const [title, key] of [['Biến của ứng dụng', 'API_URL'], ['Biến của ứng dụng', 'LOG_LEVEL'], ['Bí mật của ứng dụng', 'API_TOKEN']]) await expect(groupOf(title).getByRole('checkbox', { name: key, exact: true })).not.toBeChecked();
    check('key picker: API_URL, LOG_LEVEL and API_TOKEN (MOCKED secret entry) are all unticked on load');
    await h.click(groupOf('Biến của ứng dụng').getByRole('checkbox', { name: 'API_URL', exact: true }));
    await h.click(page.getByRole('checkbox', { name: 'Dùng tên Container khác cho API_URL', exact: true }));
    await h.type(page.getByLabel('Tên trong Container cho API_URL', { exact: true }), 'BACKEND_URL', { replace: true });
    await expect(groupOf('Biến của ứng dụng').getByRole('checkbox', { name: 'LOG_LEVEL', exact: true })).not.toBeChecked();
    await expect(groupOf('Bí mật của ứng dụng').getByRole('checkbox', { name: 'API_TOKEN', exact: true })).not.toBeChecked();
    mark('key-alias');
    await h.read('Chỉ key được chọn mới đi vào Score; secret và biến khác vẫn chưa chọn');

    caption('Bước 3: Thêm cổng http 8080; chưa có đường dẫn công khai nào tự xuất hiện. Mở nâng cao Container, đặt CPU limit 500m');
    await h.click(editor('addPort'));
    await h.type(portNames().nth(0), 'http');
    await h.type(portNumbers().nth(0), '8080');
    await expect(editor('routePath')).toHaveCount(0);
    check('adding a valid port does not add a public route');
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao Container 1', exact: true }));
    await h.type(editor('cpuLimit'), '500m');
    await expect(note('Tóm tắt nâng cao Container 1')).toContainText('CPU 500m (theo limit) · Bộ nhớ 32Mi (mặc định). Giới hạn: CPU 500m · Bộ nhớ chưa đặt.');
    await h.click(page.getByRole('button', { name: 'Ẩn nâng cao Container 1', exact: true }));
    await expect(editor('cpuLimit')).toHaveCount(0);
    await expect(note('Tóm tắt nâng cao Container 1')).toContainText('CPU 500m (theo limit)');
    check('collapsed advanced section keeps CPU limit 500m and its summary; request falls back to the limit (CPU), default (Memory)');
    mark('advanced-collapsed');
    await h.moveTo(note('Tóm tắt nâng cao Container 1'));
    await h.read('Đóng phần nâng cao chỉ ẩn ô nhập; giá trị và tóm tắt vẫn còn');

    caption('Bước 4: Thêm đường dẫn công khai: đúng một cổng hợp lệ nên chọn sẵn http; nhập đường dẫn /');
    await h.click(editor('addRoute'));
    await expect(routeSelects().nth(0)).toHaveValue('http');
    await h.type(editor('routePath').nth(0), '/');
    check('Add public route with exactly one valid port preselects http');
    mark('route-one-port');

    caption('Bước 5: Thêm cổng admin 9000 rồi thêm đường dẫn thứ hai: có nhiều cổng hợp lệ nên phải chọn tường minh; lưu bị chặn');
    await h.click(editor('addPort'));
    await h.type(portNames().nth(1), 'admin');
    await h.type(portNumbers().nth(1), '9000');
    await h.click(editor('addRoute'));
    await expect(routeSelects().nth(1)).toHaveValue('');
    await expect(routeSelects().nth(0)).toHaveValue('http');
    await h.type(editor('routePath').nth(1), '/admin');
    const putsBefore = putWorkloadCalls.length;
    await h.click(editor('save'));
    await expect(page.getByRole('alert')).toContainText('Hãy chọn một cổng Service cho đường dẫn công khai này.');
    expect(putWorkloadCalls.length, 'save blocked before any PUT').toBe(putsBefore);
    check('two valid ports: second route is not auto-targeted, Save shows a Vietnamese error and sends no PUT');
    mark('route-multi-port');
    await h.read('Nhiều cổng hợp lệ thì không tự chọn thay người dùng');
    await h.click(editor('routePath').nth(1).locator('xpath=ancestor::div[contains(@class,"binding-row")]').getByRole('button', { name: 'Xóa' }));
    await expect(editor('routePath')).toHaveCount(1);

    caption('Bước 6: Đổi tên cổng http thành http2: đường dẫn / giữ đích cũ http, hiện lỗi tiếng Việt và chặn lưu; trả lại tên thì hợp lệ');
    await h.type(portNames().nth(0), 'http2', { replace: true });
    await expect(routeSelects().nth(0)).toHaveValue('http');
    await expect(invalidRoute('http')).toBeVisible();
    await h.click(editor('save'));
    await expect(page.getByRole('alert')).toContainText('Cổng Service "http" không còn hợp lệ');
    expect(putWorkloadCalls.length).toBe(putsBefore);
    await h.moveTo(invalidRoute('http'));
    await h.read('Đích route không bị đổi âm thầm sang http2 hay admin');
    await h.type(portNames().nth(0), 'http', { replace: true });
    await expect(invalidRoute('http')).toHaveCount(0);
    check('renamed port: route kept target http, Vietnamese invalid message shown, Save blocked with no PUT; renaming back clears it');
    mark('route-rename-invalid');

    caption('Bước 7: Đặt cổng admin 70000 (không hợp lệ): thêm đường dẫn mới chỉ còn http hợp lệ nên chọn sẵn http; xóa rồi trả admin về 9000');
    await h.type(portNumbers().nth(1), '70000', { replace: true });
    await h.click(editor('addRoute'));
    await expect(routeSelects().nth(1)).toHaveValue('http');
    await h.click(editor('routePath').nth(1).locator('xpath=ancestor::div[contains(@class,"binding-row")]').getByRole('button', { name: 'Xóa' }));
    await h.type(portNumbers().nth(1), '9000', { replace: true });
    await expect(editor('routePath')).toHaveCount(1);
    check('invalid numeric port (70000) is not counted as valid; the lone valid port http is preselected for a new route');
    mark('route-invalid-number');

    caption('Bước 8: Lưu draft web; Score giữ cổng dạng số, targetPort mặc định bằng cổng Service, chỉ có limit CPU do người dùng nhập');
    const webScore = await saveDraft('web');
    expect(webScore.containers.main).toEqual({ image: 'registry.example/t08-web:v1', variables: { BACKEND_URL: '${resources.env.API_URL}' }, resources: { limits: { cpu: '500m' } } });
    expect(webScore.resources).toEqual({ env: { type: 'environment' } });
    expect(webScore.service).toEqual({ ports: { http: { port: 8080, targetPort: 8080 }, admin: { port: 9000, targetPort: 9000 } }, publicRoutes: [{ path: '/', port: 'http' }] });
    expect(typeof webScore.service.ports.http.port).toBe('number');
    expect(await storedScore(appId, 'web')).toEqual(webScore);
    check('PUT payload (browser) and stored draft (backend GET) equal: numeric ports/targetPort, only limits.cpu, one key reference, single public route');
    mark('web-saved');
    await h.read('Draft web đã lưu; chưa Xem trước hay Triển khai');

    // ---- 2. Params kiểu số/bool, class, target port override --------------------------------------------------------
    caption('Bước 9: Workload jobs: thêm tài nguyên t08-queue; lưu khi thiếu replicas bị chặn bằng thông báo tiếng Việt');
    await openEditor();
    await h.type(editor('name'), 'jobs');
    await h.type(editor('image'), 'registry.example/t08-jobs:v1');
    await h.click(editor('addResource'));
    await h.type(editor('resourceAlias'), 'q');
    await h.choose(editor('resourceType'), KEY);
    await expect(editor('resourceType')).toHaveValue(KEY);
    await h.click(editor('save'));
    await expect(page.getByRole('alert')).toContainText('q: replicas là bắt buộc.');
    check('required numeric param blocks Save with a Vietnamese FE message');
    mark('required-param');
    await h.type(page.getByLabel('Tham số replicas', { exact: true }), '3');
    await h.choose(page.getByLabel('Tham số durable', { exact: true }), 'Đúng');

    caption('Bước 10: Class nằm trong phần nâng cao của tài nguyên: để trống thì hiệu lực là default; nhập gold rồi đóng, tóm tắt vẫn hiện');
    await expect(editor('resourceClass')).toHaveCount(0);
    await expect(note('Tóm tắt nâng cao tài nguyên q')).toContainText('Lớp hiệu lực: default (mặc định)');
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao tài nguyên q', exact: true }));
    await h.type(editor('resourceClass'), 'gold');
    await h.click(page.getByRole('button', { name: 'Ẩn nâng cao tài nguyên q', exact: true }));
    await expect(note('Tóm tắt nâng cao tài nguyên q')).toContainText('Lớp hiệu lực: gold (tường minh)');
    mark('class-advanced');

    caption('Bước 11: Cổng grpc 9090; target port nằm trong nâng cao: đặt 50051, đóng lại vẫn thấy tóm tắt. Không thêm đường dẫn công khai nào');
    await h.click(editor('addPort'));
    await h.type(portNames().nth(0), 'grpc');
    await h.type(portNumbers().nth(0), '9090');
    await expect(note('Tóm tắt nâng cao cổng grpc')).toContainText('Cổng đích hiệu lực: 9090 (theo cổng Service)');
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao cổng grpc', exact: true }));
    await h.type(editor('targetPort'), '50051');
    await h.click(page.getByRole('button', { name: 'Ẩn nâng cao cổng grpc', exact: true }));
    await expect(note('Tóm tắt nâng cao cổng grpc')).toContainText('Cổng đích hiệu lực: 50051 (ghi đè)');
    await expect(editor('routePath')).toHaveCount(0);
    mark('target-port-advanced');

    caption('Bước 12: Lưu draft jobs; params replicas là số 3, durable là bool true; class gold; targetPort số 50051; không có route công khai');
    const jobsScore = await saveDraft('jobs');
    expect(jobsScore.resources).toEqual({ q: { type: KEY, class: 'gold', params: { replicas: 3, durable: true } } });
    expect(typeof jobsScore.resources.q.params.replicas).toBe('number');
    expect(typeof jobsScore.resources.q.params.durable).toBe('boolean');
    expect(jobsScore.service).toEqual({ ports: { grpc: { port: 9090, targetPort: 50051 } } });
    expect(jobsScore.containers.main).toEqual({ image: 'registry.example/t08-jobs:v1' });
    expect(await storedScore(appId, 'jobs')).toEqual(jobsScore);
    check('jobs payload and stored draft: params number/bool typed, class gold, targetPort override 50051 numeric, no publicRoutes, no variables/capacity');
    mark('jobs-saved');
    await h.read('Kiểu số và bool được giữ nguyên trong Score');

    // ---- 3. Score nhập vào: overrides, alias nhiều-một, proto an toàn -----------------------------------------------
    caption('Bước 13: Nhập file Score api có override tài nguyên, class, target port, route và hai tên Container cho cùng key API_URL');
    const importedScore = {
      apiVersion: 'score.dev/v1b1',
      metadata: { name: 'api' },
      containers: { main: { image: 'registry.example/t08-api:v1', variables: { PRIMARY_URL: '${resources.env.API_URL}', SECONDARY_URL: '${resources.env.API_URL}' }, resources: { requests: { cpu: '250m' }, limits: { memory: '256Mi' } } } },
      service: { ports: { http: { port: 80, targetPort: 8080 } }, publicRoutes: [{ path: '/api', port: 'http' }] },
      resources: { env: { type: 'environment' }, q: { type: KEY, class: 'gold', params: { replicas: 5, durable: false } } },
    };
    await openEditor();
    await h.click(editor('importTab'));
    // File selection stands in for the OS file dialog (headed Chromium cannot be driven through it); the parse call is the real backend.
    await editor('scoreFile').setInputFiles({ name: 'api.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(importedScore)) });
    await expect(page.getByText('Đã phân tích Workload:')).toContainText('api');
    check('import via UI file input: backend parsed the Score and the panel shows "Đã phân tích Workload: api"');
    mark('imported');
    await h.read('Backend thật đã phân tích Score; nhập chưa lưu hay triển khai');
    await h.click(editor('formTab'));
    await expect(page.getByLabel('Tên trong Container cho API_URL (1)', { exact: true })).toHaveValue('PRIMARY_URL');
    await expect(page.getByLabel('Tên trong Container cho API_URL (2)', { exact: true })).toHaveValue('SECONDARY_URL');
    await expect(note('Tóm tắt nâng cao Container 1')).toContainText('CPU 250m (tường minh) · Bộ nhớ 256Mi (theo limit). Giới hạn: CPU chưa đặt · Bộ nhớ 256Mi.');
    await expect(note('Tóm tắt nâng cao tài nguyên q')).toContainText('Lớp hiệu lực: gold (tường minh)');
    await expect(note('Tóm tắt nâng cao cổng http')).toContainText('Cổng đích hiệu lực: 8080 (ghi đè)');
    check('imported form: two Container names for API_URL (1)/(2), collapsed summaries show the request/limit, class gold and targetPort 8080 overrides');
    mark('imported-form');
    await h.moveTo(note('Tóm tắt nâng cao Container 1'));
    await h.read('Override đã nhập hiện ngay cả khi phần nâng cao đóng');

    caption('Bước 14: Sửa image (trường cơ bản); mở và đóng nâng cao; đổi tên cổng thì route /api báo lỗi và chặn lưu, trả lại thì lưu được');
    await h.type(editor('image'), 'registry.example/t08-api:v2', { replace: true });
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao Container 1', exact: true }));
    await expect(editor('cpuRequest')).toHaveValue('250m');
    await h.click(page.getByRole('button', { name: 'Ẩn nâng cao Container 1', exact: true }));
    const apiPuts = putWorkloadCalls.length;
    await h.type(portNames().nth(0), 'web', { replace: true });
    await expect(routeSelects().nth(0)).toHaveValue('http');
    await expect(invalidRoute('http')).toBeVisible();
    await h.click(editor('save'));
    await expect(page.getByRole('alert')).toContainText('Cổng Service "http" không còn hợp lệ');
    expect(putWorkloadCalls.length).toBe(apiPuts);
    await h.type(portNames().nth(0), 'http', { replace: true });
    await expect(invalidRoute('http')).toHaveCount(0);
    mark('imported-edited');
    const apiScore = await saveDraft('api');
    expect(apiScore).toEqual({ ...importedScore, containers: { main: { ...importedScore.containers.main, image: 'registry.example/t08-api:v2' } } });
    expect(typeof apiScore.resources.q.params.replicas).toBe('number');
    expect(apiScore.resources.q.params.durable).toBe(false);
    expect(await storedScore(appId, 'api')).toEqual(apiScore);
    check('api saved after basic edit + collapse + blocked rename: only image changed; overrides, both aliases, numeric/bool params, class and /api route kept (browser payload = backend draft)');
    mark('api-saved');
    await h.read('Chỉ image đổi; mọi override và alias nhập vào vẫn còn');

    // ---- 4. Validation, stale, BUSY ----------------------------------------------------------------------------------
    caption('Bước 15: Nhập Score có loại tài nguyên không tồn tại: backend từ chối, form hiện lỗi, không coi là đã phân tích; thiếu tên Workload bị chặn');
    await openEditor();
    await h.click(editor('importTab'));
    const bad = { apiVersion: 'score.dev/v1b1', metadata: { name: 'broken' }, containers: { main: { image: 'registry.example/t08-broken:v1' } }, resources: { cache: { type: 'no-such-type' } } };
    await editor('scoreFile').setInputFiles({ name: 'broken.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(bad)) });
    await expect(page.getByRole('alert')).toBeVisible();
    await expect(page.getByText('Đã phân tích Workload:')).toHaveCount(0);
    check('import validation: backend rejected an unregistered type, an alert is shown (raw server text, mapping is T20), no parsed preview');
    mark('import-rejected');
    await h.moveTo(page.getByRole('alert'));
    await h.read('Backend từ chối Score; trang không đánh dấu là đã phân tích');
    await h.click(editor('formTab'));
    await h.type(editor('image'), 'registry.example/t08-stale:v1');
    await h.click(editor('save'));
    await expect(page.getByRole('alert')).toContainText('Cần nhập tên Workload.');
    await expect(editor('image')).toHaveValue('registry.example/t08-stale:v1');
    check('missing name: Vietnamese FE validation, image text kept');

    caption('Bước 16: (MOCK) Máy chủ báo draft đã đổi (409): chữ nhập được giữ, Lưu bị khóa cho đến khi tải lại trạng thái');
    await h.type(editor('name'), 'stale-w');
    mock.stale = 1;
    const beforeStale = putWorkloadCalls.length;
    await h.click(editor('save'));
    await expect(page.getByRole('alert', { name: 'Draft Workload đã cũ' })).toContainText('đã thay đổi sau khi trang này được tải');
    await expect(editor('save')).toBeDisabled();
    await expect(editor('name')).toHaveValue('stale-w');
    await expect(editor('image')).toHaveValue('registry.example/t08-stale:v1');
    expect(putWorkloadCalls.length - beforeStale, 'exactly one PUT, never re-sent').toBe(1);
    mark('stale');
    await h.read('Lưu bị chặn sau 409; không tự gửi lại');
    await h.click(page.getByRole('button', { name: 'Tải lại trạng thái hiện tại' }));
    await expect(page.getByRole('status').filter({ hasText: 'Đã tải lại draft phiên bản' })).toBeVisible();
    await expect(editor('save')).toBeEnabled();
    await expect(editor('image')).toHaveValue('registry.example/t08-stale:v1');
    expect(putWorkloadCalls.length - beforeStale).toBe(1);
    check('stale 409 (MOCKED reply): Save disabled, text kept, no resend; explicit reload re-enabled Save with edits intact');

    caption('Bước 17: (MOCK) Môi trường đang chạy thao tác: nút Lưu khóa và hiện thông báo; thả khóa thì Lưu dùng lại được, chữ vẫn còn');
    mock.envPatch.staging = { activeOperation: { id: 'op-busy', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOYING', startedAt: '2026-10-11T00:00:00Z', updatedAt: '2026-10-11T00:00:00Z', heartbeatAt: '2026-10-11T00:00:00Z', recoverable: false } };
    await expect(page.getByRole('status').filter({ hasText: 'Một thao tác môi trường đang chạy' })).toBeVisible({ timeout: 15_000 });
    await expect(editor('save')).toBeDisabled();
    await expect(editor('image')).toHaveValue('registry.example/t08-stale:v1');
    mark('busy');
    await h.read('BUSY: giữ chữ nhưng không cho lưu');
    mock.envPatch = {};
    await expect(page.getByText('Một thao tác môi trường đang chạy')).toHaveCount(0, { timeout: 15_000 });
    await expect(editor('save')).toBeEnabled();
    await expect(editor('image')).toHaveValue('registry.example/t08-stale:v1');
    check('BUSY (MOCKED projection): Save disabled with Vietnamese notice while busy, re-enabled after release, text kept');
    mark('busy-released');
    const listed = await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/workloads`)).json();
    expect(listed.workloads.map((item) => item.id).sort()).toEqual(['api', 'jobs', 'web']);
    await h.click(editor('cancel'));
    await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
    check('backend GET: only web, jobs and api drafts exist; stale-w and broken were never stored');

    caption('Bước 18: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T09: trang Tạo ứng dụng. Developer. Backend thật (fake adapters, state tạm) trả lời tạo ứng dụng, phát hiện trùng tên/subdomain và
  // dựng Application home. Điều kiện thật: slug gợi ý, giữ subdomain thủ công, validation phía client, 409 trùng name/subdomain do backend
  // trả, payload POST và điều hướng. MOCK (page.route, ghi chú tại chỗ dùng): đúng một POST /applications bị giữ ~2s rồi trả 500 để dựng
  // lỗi bất đồng bộ và kiểm tra chặn submit lặp; backend fake không tạo được lỗi này. Không có MOCK nào cho kết quả tạo thành công.
  async T09({ page, h, ui, caption, mark, check, baseURL }) {
    const posts = []; // every POST /api/v1/applications the browser sent (incl. the mocked failing one)
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications') posts.push(request.postDataJSON());
    });
    const mock = { failNext: 0 };
    // MOCK: the next POST /applications is held, then answered 500 without reaching the backend.
    await page.route('**/api/v1/applications', async (route) => {
      if (route.request().method() !== 'POST' || mock.failNext === 0) { await route.continue(); return; }
      mock.failNext -= 1;
      await new Promise((resolve) => setTimeout(resolve, 2000));
      await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal secret detail xyz' }) });
    });
    const nameField = () => page.getByLabel('Tên ứng dụng', { exact: true });
    const subdomainField = () => page.getByLabel('Subdomain', { exact: true });
    const submitButton = () => page.getByRole('button', { name: 'Tạo ứng dụng', exact: true });
    const preview = () => page.getByRole('group', { name: 'Endpoint sẽ được tạo' });
    const openForm = async () => {
      await h.click(page.getByRole('button', { name: '+ Tạo ứng dụng' }).first());
      await expect(page.getByRole('heading', { name: 'Tạo ứng dụng', level: 1 })).toBeVisible();
      await expect(page.getByRole('form', { name: 'Tạo ứng dụng' })).toBeVisible();
    };
    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await h.click(ui('signIn', 'submit'));
      await expect(ui('applications', 'heading')).toBeVisible();
    };
    const FIRST = 'T09 Thanh toán Điện tử';

    caption('Bước 1: Lập trình viên đăng nhập và mở form Tạo ứng dụng; chỉ có Tên ứng dụng và Subdomain, không có lựa chọn kết nối');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signInAs('developer');
    await openForm();
    await expect(page.getByRole('textbox')).toHaveCount(2);
    await expect(page.getByRole('combobox')).toHaveCount(0);
    await expect(preview()).toContainText('staging.ten-ung-dung.example.com');
    check('create form: Vietnamese heading/form name, exactly 2 text inputs, no combobox/connection choice, placeholder preview');
    mark('create-form');
    await h.read('Form chỉ có tên và subdomain; kết nối chọn sau trong Cài đặt môi trường');

    caption('Bước 2: Nhập tên có dấu tiếng Việt; subdomain tự gợi ý thành slug không dấu, đ/Đ thành d; sửa tên thì gợi ý đổi theo');
    await h.type(nameField(), FIRST);
    await expect(subdomainField()).toHaveValue('t09-thanh-toan-dien-tu');
    await expect(preview()).toContainText('staging.t09-thanh-toan-dien-tu.example.com');
    await h.type(nameField(), `${FIRST} Mới`, { replace: true });
    await expect(subdomainField()).toHaveValue('t09-thanh-toan-dien-tu-moi');
    await h.type(nameField(), 'T09 Thanh toán Điện tử', { replace: true });
    await expect(subdomainField()).toHaveValue('t09-thanh-toan-dien-tu');
    expect(posts).toHaveLength(0);
    check('suggestion: "T09 Thanh toán Điện tử" -> t09-thanh-toan-dien-tu, follows name edits and preview; no POST yet');
    mark('suggestion');
    await h.read('Gợi ý chỉ hỗ trợ nhập liệu; chưa gọi API');

    caption('Bước 3: Người dùng sửa subdomain; từ đó đổi tên không ghi đè lựa chọn thủ công');
    await h.type(subdomainField(), 't09-pay', { replace: true });
    await h.type(nameField(), `${FIRST} hai`, { replace: true, delay: 80 });
    await expect(nameField()).toHaveValue('T09 Thanh toán Điện tử hai');
    await expect(subdomainField()).toHaveValue('t09-pay');
    await expect(preview()).toContainText('staging.t09-pay.example.com');
    check('custom subdomain t09-pay kept after the name changed; preview follows the subdomain');
    mark('custom-kept');
    await h.read('Subdomain thủ công được giữ nguyên');

    caption('Bước 4: Xóa trống subdomain rồi đổi tên: vẫn không tự gợi lại; bấm Tạo ứng dụng báo lỗi field, không gửi POST');
    await h.type(subdomainField(), '', { replace: true });
    await h.type(nameField(), `${FIRST} ba`, { replace: true, delay: 80 });
    await expect(subdomainField()).toHaveValue('');
    await h.click(submitButton());
    await expect(page.getByText('Nhập subdomain.')).toBeVisible();
    await expect(subdomainField()).toHaveAttribute('aria-invalid', 'true');
    expect(posts).toHaveLength(0);
    check('cleared subdomain stays empty after a name edit; empty submit shows "Nhập subdomain." and sends no POST');
    mark('cleared-empty');
    await h.read('Subdomain bị xóa trống nên không còn gợi ý; lỗi tiếng Việt, không POST');

    caption('Bước 5: Subdomain không hợp lệ (Bad_Label) và tên rỗng: lỗi đúng field tiếng Việt, preview giữ nguyên bản nháp, không POST');
    await h.type(subdomainField(), 'Bad_Label', { replace: true });
    await h.type(nameField(), '', { replace: true });
    await h.click(submitButton());
    await expect(page.getByText('Nhập tên ứng dụng.')).toBeVisible();
    await expect(page.getByText(/Chỉ dùng chữ thường không dấu, số và dấu gạch ngang; bắt đầu/)).toBeVisible();
    await expect(preview()).toContainText('staging.bad_label.example.com');
    expect(posts).toHaveLength(0);
    check('invalid input: name and subdomain Vietnamese field errors, preview keeps draft bad_label, no POST');
    mark('invalid');
    await h.read('Lỗi nhập liệu hiển thị trên từng field; chưa gửi gì lên backend');

    caption('Bước 6: Tạo ứng dụng đầu tiên (backend thật); POST chỉ gửi name và subdomain, rồi mở Application home ở staging');
    await h.type(nameField(), FIRST, { replace: true });
    await h.type(subdomainField(), 't09-pay', { replace: true });
    const created = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications');
    await h.click(submitButton());
    expect((await created).postDataJSON()).toEqual({ name: FIRST, subdomain: 't09-pay' });
    await expect(page.getByRole('heading', { name: FIRST })).toBeVisible();
    await expect(page.getByRole('button', { name: /Staging/i })).toHaveClass(/tab-active/);
    const appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
    if (!appId || !/^[a-z0-9-]+$/.test(appId)) throw new Error(`invalid application id: ${appId}`);
    const stored = (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}`)).json()).application;
    expect(stored.name).toBe(FIRST);
    expect(stored.subdomain).toBe('t09-pay');
    check(`create (real fake backend): payload exactly {name, subdomain}; navigated to application ${appId} with Staging tab active; backend GET confirms name/subdomain`);
    mark('created');
    await h.read('Ứng dụng đã tạo; payload chỉ có name và subdomain');

    caption('Bước 7: Tạo ứng dụng thứ hai trùng tên: backend trả 409, lỗi hiện đúng ở field Tên ứng dụng và bản nháp được giữ');
    await h.click(ui('shell', 'navApplications'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await openForm();
    await h.type(nameField(), FIRST, { replace: true });
    await h.type(subdomainField(), 't09-other', { replace: true });
    const before = posts.length;
    await h.click(submitButton());
    await expect(page.getByText('Tên ứng dụng này đã tồn tại.')).toBeVisible();
    await expect(nameField()).toHaveValue(FIRST);
    await expect(subdomainField()).toHaveValue('t09-other');
    expect(posts).toHaveLength(before + 1);
    expect(posts.at(-1)).toEqual({ name: FIRST, subdomain: 't09-other' });
    check('duplicate name (backend 409): Vietnamese error on the name field, draft kept, one POST with name/subdomain only');
    mark('duplicate-name');
    await h.read('Trùng tên: lỗi ở field tên, nội dung vẫn còn để sửa');

    caption('Bước 8: Đổi tên rồi dùng lại subdomain đã có: backend trả 409, lỗi hiện ở field Subdomain');
    await h.type(nameField(), 'T09 Ứng dụng khác', { replace: true });
    await expect(subdomainField()).toHaveValue('t09-other'); // manual edit survives the name change
    await h.type(subdomainField(), 't09-pay', { replace: true });
    const beforeSub = posts.length;
    await h.click(submitButton());
    await expect(page.getByText('Subdomain này đã được sử dụng.')).toBeVisible();
    await expect(nameField()).toHaveValue('T09 Ứng dụng khác');
    await expect(subdomainField()).toHaveValue('t09-pay');
    expect(posts).toHaveLength(beforeSub + 1);
    check('duplicate subdomain (backend 409): Vietnamese error on the subdomain field, draft kept, no automatic retry');
    mark('duplicate-subdomain');
    await h.read('Trùng subdomain: lỗi ở field subdomain; không tự thử lại');

    caption('Bước 9: Lỗi bất đồng bộ (MOCK 500 sau ~2s): form khóa, nút đổi trạng thái, không gửi lặp; lỗi chung tiếng Việt, không lộ chi tiết backend');
    await h.type(subdomainField(), 't09-async', { replace: true });
    mock.failNext = 1;
    const beforeAsync = posts.length;
    await h.click(submitButton());
    await expect(page.getByRole('button', { name: 'Đang tạo ứng dụng…' })).toBeDisabled();
    await expect(nameField()).toBeDisabled();
    await expect(subdomainField()).toBeDisabled();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('alert')).toHaveText('Không thể tạo ứng dụng. Hãy thử lại.');
    await expect(page.getByText('internal secret detail')).toHaveCount(0);
    await expect(nameField()).toHaveValue('T09 Ứng dụng khác');
    await expect(subdomainField()).toHaveValue('t09-async');
    await expect(submitButton()).toBeEnabled();
    expect(posts).toHaveLength(beforeAsync + 1);
    check('async failure (MOCKED 500): fields and button locked while pending, exactly one POST, generic Vietnamese alert without backend text, draft kept');
    mark('async-failure');
    await h.read('Lỗi chung an toàn; thử lại chỉ khi người dùng bấm lại');

    caption('Bước 10: Người dùng bấm thử lại (backend thật): tạo thành công và mở Application home staging');
    const retried = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications');
    await h.click(submitButton());
    expect((await retried).postDataJSON()).toEqual({ name: 'T09 Ứng dụng khác', subdomain: 't09-async' });
    await expect(page.getByRole('heading', { name: 'T09 Ứng dụng khác' })).toBeVisible();
    await expect(page.getByRole('button', { name: /Staging/i })).toHaveClass(/tab-active/);
    expect(posts).toHaveLength(beforeAsync + 2);
    check('explicit retry (real backend): POST {name, subdomain} only, navigated to the new application home with Staging active');
    mark('retry-created');
    await h.read('Thử lại thành công; điều hướng sang ứng dụng mới');

    caption('Bước 11: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T10: Đăng nhập và shared shell. Backend fake thật với tài khoản test cố định
  // (developer, platform-engineer). PRODUCT (UI): đăng nhập, đăng xuất, điều hướng.
  // SETUP (API, không phải thao tác sản phẩm): thu hồi phiên phía server để mô phỏng
  // phiên hết hạn. MOCK: Role ADMIN được chiếu qua GET /auth/session vì backend chưa seed
  // tài khoản ADMIN; cookie phiên vẫn là của Kỹ sư nền tảng thật.
  async T10({ page, h, ui, caption, mark, check, baseURL }) {
    const chip = () => page.locator('.user-chip');
    const platformLinks = ['navResourceTypes', 'navResourceDefinitions', 'navRenderingTemplates', 'navConnections', 'navSecretStores'];
    const platformPaths = { navResourceTypes: 'resource-types', navResourceDefinitions: 'resource-definitions', navRenderingTemplates: 'rendering-templates', navConnections: 'connections', navSecretStores: 'secret-stores' };
    // The inline error span sits inside the label, so a field with an error is matched by prefix.
    const userField = () => page.getByLabel('Tên đăng nhập');
    const passField = () => page.getByLabel('Mật khẩu');
    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await expect(ui('signIn', 'password')).toHaveAttribute('type', 'password');
      await h.click(ui('signIn', 'submit'));
    };

    caption('Bước 1: Mở trang đăng nhập tiếng Việt; ô mật khẩu luôn bị che');
    await h.showCursor();
    await expectVisibleCursor(page);
    await expect(page.getByRole('heading', { name: 'Đăng nhập vào không gian làm việc' })).toBeVisible();
    await expect(page.getByText('Chào mừng trở lại', { exact: true })).toBeVisible();
    await expect(page.getByText('Nền tảng nội bộ cho nhà phát triển', { exact: true })).toBeVisible();
    await expect(page.getByRole('heading', { level: 1 })).toContainText('Làm việc nhanh hơn.');
    await expect(page.getByText('Hạ tầng giúp bạn tập trung vào ứng dụng.', { exact: true })).toBeVisible();
    await expect(page.getByText('Dùng tài khoản kiểm thử local của bạn.', { exact: true })).toBeVisible();
    await expect(ui('signIn', 'password')).toHaveAttribute('type', 'password');
    check('sign-in page (real): Vietnamese heading, brand text and helper, password input type=password');
    mark('sign-in-page');
    await h.read('Trang đăng nhập tiếng Việt, mật khẩu bị che');

    caption('Bước 2: Bấm Đăng nhập khi để trống; hiện lời nhắc, không gọi API');
    const signInPosts = [];
    page.on('request', (request) => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/auth/sign-in') signInPosts.push(request); });
    await h.click(ui('signIn', 'submit'));
    await expect(page.getByText('Nhập tên đăng nhập.', { exact: true })).toBeVisible();
    await expect(page.getByText('Nhập mật khẩu.', { exact: true })).toBeVisible();
    expect(signInPosts).toHaveLength(0);
    check('client validation (real): both required messages shown, no sign-in request sent');
    mark('validation');
    await h.read('Lời nhắc bắt buộc nhập tên đăng nhập và mật khẩu');

    caption('Bước 3: Nhập sai mật khẩu; backend trả 401, form báo lỗi chung, mật khẩu bị xóa');
    await h.type(userField(), 'developer');
    await h.type(passField(), 'sai-mat-khau-thu-nghiem');
    await h.click(ui('signIn', 'submit'));
    await expect(page.getByRole('alert')).toHaveText('Tên đăng nhập hoặc mật khẩu không đúng.');
    await expect(passField()).toHaveValue('');
    await expect(userField()).toHaveValue('developer');
    await expect(ui('signIn', 'submit')).toBeEnabled();
    await expect(page).toHaveURL(`${baseURL}/ui/sign-in`);
    expect(signInPosts).toHaveLength(1);
    check('invalid login (real 401): generic Vietnamese alert, password cleared, username kept, still on /ui/sign-in');
    mark('invalid-login');
    await h.read('Thông báo lỗi chung, mật khẩu đã bị xóa');

    caption('Bước 4: Nhập đúng mật khẩu; đăng nhập với vai trò Nhà phát triển và vào danh sách ứng dụng');
    await h.type(passField(), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/applications`);
    check('successful login (real): redirected to /ui/applications');
    mark('developer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 5: Thanh điều hướng của Nhà phát triển: chỉ Ứng dụng; Lịch sử triển khai bị khóa (Sắp có)');
    const nav = ui('shell', 'nav');
    await expect(nav).toBeVisible();
    await expect(ui('shell', 'navApplications')).toHaveAttribute('href', '/ui/applications');
    for (const link of platformLinks) await expect(ui('shell', link)).toHaveCount(0);
    await expect(nav.locator('.nav-item-disabled')).toContainText('Lịch sử triển khai');
    await expect(nav.locator('.nav-item-disabled')).toContainText('Sắp có');
    await expect(nav.getByRole('link')).toHaveCount(1);
    await expect(chip()).toContainText('developer');
    await expect(chip()).toContainText('Nhà phát triển');
    check('developer shell (real session): only Ứng dụng link, disabled "Lịch sử triển khai / Sắp có", footer developer + Nhà phát triển');
    mark('developer-nav');
    await h.moveTo(chip());
    await h.read('Nhà phát triển không thấy mục nền tảng');

    caption('Bước 6: Nhà phát triển mở thẳng đường dẫn Mẫu dựng ứng dụng; trang không hiển thị');
    await page.goto(`${baseURL}/ui/platform/rendering-templates`);
    await expect(nav).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng' })).toHaveCount(0);
    await expect(ui('shell', 'navRenderingTemplates')).toHaveCount(0);
    check('role gate (real): developer deep link /ui/platform/rendering-templates renders no page and no link');
    mark('developer-gate');
    await h.read('Đường dẫn trực tiếp bị chặn theo vai trò');

    caption('Bước 7: Tải lại trang; phiên được khôi phục, không quay lại trang đăng nhập');
    await page.goto(`${baseURL}/ui/applications`);
    await expect(ui('applications', 'heading')).toBeVisible();
    await expect(ui('signIn', 'username')).toHaveCount(0);
    await expect(chip()).toContainText('developer');
    check('session restore (real cookie): reload keeps shell, no sign-in form');
    mark('session-restored');
    await h.read('Phiên được khôi phục từ cookie');

    caption('Bước 8: Mở ứng dụng không tồn tại; hiển thị "Không tìm thấy ứng dụng"');
    await page.goto(`${baseURL}/ui/applications/khong-ton-tai`);
    await expect(page.getByRole('heading', { name: 'Không tìm thấy ứng dụng' })).toBeVisible();
    check('missing application (real, unknown key): Vietnamese not-found heading inside the shell');
    mark('missing-application');
    await h.read('Không tìm thấy ứng dụng');

    caption('Bước 9: Đăng xuất; về trang đăng nhập và API trả 401');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/sign-in`);
    await expect(page.getByRole('status')).toHaveCount(0);
    expect((await page.request.get(`${baseURL}/api/v1/applications`)).status()).toBe(401);
    check('sign-out (real): sign-in page without expiry notice, API returns 401');
    mark('developer-signed-out');
    await h.pause(SHORT_READ);

    caption('Bước 10: Kỹ sư nền tảng đăng nhập; thanh điều hướng đủ mục nền tảng');
    await signInAs('platform-engineer');
    await expect(ui('applications', 'heading')).toBeVisible();
    await expect(chip()).toContainText('platform-engineer');
    await expect(chip()).toContainText('Kỹ sư nền tảng');
    await expect(nav.getByRole('link')).toHaveCount(6);
    await expect(ui('shell', 'navApplications')).toHaveAttribute('href', '/ui/applications');
    for (const link of platformLinks) await expect(ui('shell', link)).toHaveAttribute('href', `/ui/platform/${platformPaths[link]}`);
    await expect(nav.locator('.nav-item-disabled')).toContainText('Sắp có');
    check('platform-engineer shell (real session): six links with expected hrefs, role label Kỹ sư nền tảng');
    mark('pe-nav');
    await h.moveTo(chip());
    await h.read('Kỹ sư nền tảng thấy đủ mục');

    caption('Bước 11: Bấm từng mục nền tảng, gồm Mẫu dựng ứng dụng; đường dẫn và mục đang chọn đúng');
    for (const link of platformLinks) {
      await h.click(ui('shell', link));
      await expect(page).toHaveURL(`${baseURL}/ui/platform/${platformPaths[link]}`);
      await expect(ui('shell', link)).toHaveClass(/nav-item-active/);
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible();
      await h.pause(800);
    }
    check('platform links (real): all five open their route with active highlight');
    mark('pe-links');
    await h.read('Các mục nền tảng mở đúng trang');

    caption('Bước 12: Mô phỏng vai trò Quản trị viên (MOCK GET /auth/session); tải lại, thấy cùng mục và nhãn Quản trị viên');
    await page.route('**/api/v1/auth/session', (route) => route.fulfill({ json: { user: { Username: 'admin-mo-phong', Role: 'ADMIN' } } }));
    await page.goto(`${baseURL}/ui/platform/rendering-templates`);
    await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1 })).toBeVisible();
    await expect(chip()).toContainText('Quản trị viên');
    await expect(nav.getByRole('link')).toHaveCount(6);
    for (const link of platformLinks) await expect(ui('shell', link)).toBeVisible();
    check('ADMIN (MOCKED session projection, real PE cookie): label Quản trị viên, six links, rendering templates page renders');
    mark('admin-mock');
    await h.read('Vai trò ADMIN (mô phỏng) hiển thị Quản trị viên');
    await page.unroute('**/api/v1/auth/session');

    caption('Bước 13: Phiên bị thu hồi phía server (SETUP qua API); thao tác tiếp theo đưa về đăng nhập kèm thông báo');
    await page.goto(`${baseURL}/ui/applications`);
    await expect(ui('applications', 'heading')).toBeVisible();
    expect((await page.request.post(`${baseURL}/api/v1/auth/sign-out`)).status()).toBeLessThan(300);
    await h.click(ui('shell', 'navResourceTypes'));
    await expect(ui('signIn', 'username')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/sign-in`);
    await expect(page.getByRole('status')).toHaveText('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.');
    check('expired session (real 401 after SETUP revoke): sign-in page with Vietnamese info notice');
    mark('session-expired');
    await h.read('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.');

    caption('Bước 14: Đăng nhập lại; thông báo biến mất, rồi đăng xuất');
    await signInAs('platform-engineer');
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.pause(SHORT_READ);
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    await expect(page.getByRole('status')).toHaveCount(0);
    expect((await page.request.get(`${baseURL}/api/v1/applications`)).status()).toBe(401);
    check('re-login and sign-out (real): no stale expiry notice, API returns 401');
    mark('signed-out');
    await h.pause(3000);
  },

  // T11: Danh sách ứng dụng. Developer. PRODUCT (UI, backend fake thật, state tạm): đăng nhập, tạo 2 ứng dụng qua form, mở card, deep link và
  // reload. Backend fake đã seed sẵn các ứng dụng acceptance nên danh sách nhiều ứng dụng là dữ liệu thật. MOCK (page.route, ghi chú tại
  // chỗ dùng): chỉ GET /api/v1/applications trả {applications: []} một lần (trạng thái rỗng), bị giữ ~2.5s (đang tải) và trả 500 một lần
  // (lỗi/thử lại); backend fake không tạo được ba trạng thái này.
  async T11({ page, h, ui, caption, mark, check, baseURL }) {
    const posts = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/applications') posts.push(request.postDataJSON());
    });
    const mock = { holdMs: 0, failNext: 0, emptyNext: 0, gets: 0 };
    // MOCK: GET /applications is held (loading), answered empty or answered 500 (error) on demand; otherwise it reaches the backend.
    await page.route('**/api/v1/applications', async (route) => {
      if (route.request().method() !== 'GET') { await route.continue(); return; }
      mock.gets += 1;
      if (mock.holdMs > 0) { const ms = mock.holdMs; mock.holdMs = 0; await new Promise((resolve) => setTimeout(resolve, ms)); }
      if (mock.emptyNext > 0) {
        mock.emptyNext -= 1;
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ applications: [] }) });
        return;
      }
      if (mock.failNext > 0) {
        mock.failNext -= 1;
        await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'internal secret detail xyz' }) });
        return;
      }
      await route.continue();
    });
    const FIRST = 'T11 Thanh toán';
    const SECOND = 'T11 Danh mục';
    const apiList = async () => (await (await page.request.get(`${baseURL}/api/v1/applications`)).json()).applications;
    const cards = () => page.locator('button.application-card');
    const createButtons = () => ui('applications', 'create');

    caption('Bước 1: Lập trình viên đăng nhập; danh sách thật của backend (đã seed sẵn) hiển thị nhãn tiếng Việt');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await expect(page.getByText('Không gian làm việc', { exact: true })).toBeVisible();
    await expect(page.getByText('Tạo ứng dụng, sau đó triển khai lên staging hoặc production.', { exact: true })).toBeVisible();
    await expect(createButtons()).toHaveCount(1);
    const seeded = await apiList();
    expect(seeded.length).toBeGreaterThan(0);
    await expect(cards()).toHaveCount(seeded.length);
    check('initial list (real backend, seeded apps): Vietnamese eyebrow/title/hint, header create button, one card per backend application');
    mark('initial-list');
    await h.read('Danh sách ban đầu: nhãn tiếng Việt, số card bằng dữ liệu backend');

    caption('Bước 2: Danh sách rỗng (MOCK GET trả rỗng): nhãn tiếng Việt; nút Tạo ứng dụng trong khung rỗng mở form tạo hiện có, chưa gửi dữ liệu');
    mock.emptyNext = 1;
    await page.reload();
    await expect(ui('applications', 'emptyHeading')).toBeVisible();
    await expect(page.getByText('Tạo ứng dụng để có môi trường staging và production.', { exact: true })).toBeVisible();
    await expect(createButtons()).toHaveCount(2);
    await expect(cards()).toHaveCount(0);
    await h.read('Trạng thái rỗng tiếng Việt: nút tạo ở header và trong khung rỗng');
    await h.click(createButtons().nth(1));
    await expect(page.getByRole('form', { name: 'Tạo ứng dụng' })).toBeVisible();
    expect(posts).toHaveLength(0);
    check('empty state (MOCKED empty reply): Vietnamese heading/hint, header and empty create buttons; empty button opens the create form; no POST sent');
    mark('empty-create-cta');
    await h.click(ui('shell', 'navApplications'));
    await expect(ui('applications', 'heading')).toBeVisible();

    caption('Bước 3: Tạo hai ứng dụng qua form (thao tác sản phẩm), mở form bằng nút Tạo ứng dụng ở header');
    await createApplication(h, FIRST, 't11-thanh-toan', { connections: null });
    await h.click(ui('shell', 'navApplications'));
    await createApplication(h, SECOND, 't11-danh-muc', { connections: null });
    expect(posts).toEqual([{ name: FIRST, subdomain: 't11-thanh-toan' }, { name: SECOND, subdomain: 't11-danh-muc' }]);
    check('two applications created through the UI (header create button); each POST has only name/subdomain');
    mark('apps-created');
    await h.read('Đã tạo hai ứng dụng bằng giao diện');

    await h.click(ui('shell', 'navApplications'));
    caption('Bước 4: Danh sách nhiều ứng dụng: thứ tự, tên, URL production và nhãn môi trường khớp dữ liệu backend');
    await page.reload(); // the list lives in app state and is refetched on load: reload shows the backend order
    const apps = await apiList();
    expect(apps).toHaveLength(seeded.length + 2);
    await expect(cards()).toHaveCount(apps.length);
    for (const [index, app] of apps.entries()) {
      const card = cards().nth(index);
      await expect(card.locator('strong')).toHaveText(app.name);
      await expect(card.locator('small').first()).toHaveText(`${app.subdomain}.example.com`);
      await expect(card.getByText('staging', { exact: true })).toBeVisible();
      await expect(card.getByText('production', { exact: true })).toBeVisible();
      await expect(card.getByText('Sẵn sàng cấu hình', { exact: true })).toHaveCount(2);
    }
    expect(apps.map((app) => app.name)).toEqual(expect.arrayContaining([FIRST, SECOND]));
    await expect(page.getByText('Ready to configure')).toHaveCount(0);
    await expect(page.getByRole('searchbox')).toHaveCount(0);
    check('multi-app list (real backend): card order, names and production URLs equal GET /applications (seeded + 2 created); labels literal lowercase staging/production, status "Sẵn sàng cấu hình"; no search box');
    mark('multi-list');
    await h.read('Danh sách giữ thứ tự, tên và URL của dữ liệu thật');

    caption('Bước 5: Tải lại trang khi API chậm (MOCK giữ GET ~4.5s): hiện trạng thái đang tải accessible, rồi hiện đúng dữ liệu');
    mock.holdMs = 4500;
    await page.reload();
    await expect(ui('applications', 'loading')).toHaveAttribute('aria-busy', 'true');
    await h.read('Đang tải ứng dụng: skeleton có role status');
    await expect(cards()).toHaveCount(apps.length);
    expect(await cards().locator('strong').allTextContents()).toEqual(apps.map((app) => app.name));
    check('loading (MOCKED hold): role=status aria-busy "Đang tải ứng dụng", then the same cards in the same order');
    mark('loading');

    caption('Bước 6: Lỗi tải danh sách (MOCK 500): thông báo tiếng Việt, không lộ lỗi backend, không dữ liệu giả; bấm Thử lại mới đọc lại');
    mock.failNext = 1;
    await page.reload();
    await expect(page.getByRole('alert')).toContainText('Không thể tải danh sách ứng dụng.');
    await expect(page.getByRole('alert')).not.toContainText('internal secret');
    await expect(page.getByText(/internal secret detail|xyz/)).toHaveCount(0);
    expect(await page.locator('body').innerText()).not.toContain('internal secret detail xyz');
    await expect(cards()).toHaveCount(0);
    const getsBefore = mock.gets;
    await h.read('Lỗi tải: không có danh sách giả, không lộ chi tiết backend');
    expect(mock.gets).toBe(getsBefore);
    await h.click(ui('applications', 'retry'));
    await expect(cards()).toHaveCount(apps.length);
    expect(mock.gets).toBe(getsBefore + 1);
    expect(await cards().locator('strong').allTextContents()).toEqual(apps.map((app) => app.name));
    check('error (MOCKED 500): Vietnamese alert, no raw backend text, no cards, no automatic retry; one explicit retry GET restores the same cards in order');
    mark('error-retry');

    caption('Bước 7: Mở card ứng dụng; điều hướng đến Application home đúng ID, không gửi thêm POST');
    const postsBefore = posts.length;
    await h.click(cards().nth(1));
    await expect(page).toHaveURL(new RegExp(`/ui/applications/${apps[1].key}$`));
    await expect(page.getByRole('heading', { name: apps[1].name })).toBeVisible();
    expect(posts).toHaveLength(postsBefore);
    check('card click opens /ui/applications/<id> of that application; no mutation request');
    mark('card-navigation');
    await h.read('Card mở Application home đúng ứng dụng');

    caption('Bước 8: Tải lại deep link ứng dụng rồi quay lại danh sách; URL và dữ liệu giữ nguyên');
    await page.reload();
    await expect(page).toHaveURL(new RegExp(`/ui/applications/${apps[1].key}$`));
    await expect(page.getByRole('heading', { name: apps[1].name })).toBeVisible();
    await h.click(ui('shell', 'navApplications'));
    await expect(cards()).toHaveCount(apps.length);
    await expect(page).toHaveURL(/\/ui\/applications$/);
    expect(await apiList()).toEqual(apps);
    check('deep link survives reload; list returns; backend applications unchanged (T11 never mutates)');
    mark('deep-link-reload');
    await h.read('Deep link giữ nguyên sau tải lại; dữ liệu không đổi');

    caption('Bước 9: Đăng xuất; phiên bị thu hồi');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T12: Application home. Developer. PRODUCT (UI, backend fake thật với fake deliverer, state tạm): tạo ứng dụng, hướng dẫn chưa cấu hình, chọn kết nối
  // Staging trong Cài đặt môi trường, lưu 2 draft Workload, Preview chờ, Deploy thật, Preview no-op, xóa/hoàn tác. MOCK (page.route, mỗi lần dùng được ghi
  // vào mocks.json và assertions): (1) POST deploy trả PARTIAL giả (web thành công, api thất bại kèm chuỗi bí mật giả) vì fake deliverer luôn thành công;
  // backend KHÔNG thật sự triển khai một phần, danh sách tải lại là dữ liệu thật; (2) POST deploy trả 409 giả, DELETE trả 409 giả (stale); (3) POST preview
  // bị giữ ~3.5s để chứng minh phản hồi đến muộn bị bỏ qua. Không có BUSY thật (cần operation môi trường đang chạy): BUSY chỉ được kiểm bằng unit test.
  async T12({ page, h, ui, caption, mark, check, baseURL, x11, snap }) {
    const NAME = 'T12 Trang ứng dụng';
    const SECRET_TEXT = 'provider said token=ABC-SECRET-777';
    const mocks = [];
    const mocked = (what) => { mocks.push(what); appendFileSync(`${process.env.ORCH_E2E_EVIDENCE_DIR}/mocks.json`, `${JSON.stringify({ at: new Date().toISOString(), what })}\n`); };
    const requests = [];
    page.on('request', (request) => {
      if (request.method() !== 'GET') requests.push(`${request.method()} ${new URL(request.url()).pathname.replace(/^\/api\/v1\/applications\/[^/]+\//, '')}`);
    });
    const count = (suffix) => requests.filter((line) => line.endsWith(suffix)).length;
    const row = (name) => page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`^◫?${name}$`) }) });
    const previewPanel = () => page.getByLabel('Xem trước triển khai');
    const resultPanel = () => page.getByLabel('Kết quả triển khai');
    const previewButton = () => page.getByRole('button', { name: 'Xem trước thay đổi' });
    const deployButton = () => page.getByRole('button', { name: 'Triển khai các thay đổi này' });
    const noRaw = async () => expect(await page.locator('body').innerText()).not.toMatch(/ABC-SECRET|token=|provider said|stale detail/);
    const dialogs = [];
    // The native confirm stays open (a listener prevents auto-dismiss): the click is pending, the frame is
    // grabbed and a pause lets the viewer read it, then a real X11 key answers (Return = accept, Escape = cancel).
    page.on('dialog', (dialog) => dialogs.push(dialog.message()));
    const answerConfirm = async (button, key, label) => {
      const opened = new Promise((resolve) => page.once('dialog', resolve));
      const clicking = h.click(button);
      await opened;
      await h.pause(600);
      mark(`dialog-visible-${label}`);
      snap(`T12-dialog-${label}`);
      await h.pause(2200);
      x11.keys([key]);
      await clicking;
    };
    const apiWorkloads = async (environment) => (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/${environment}/workloads`)).json()).workloads;
    let appId = '';
    const spec = (name) => ({ name, image: `registry.example/${name}:t12`, bindings: [], port: { name: 'http', port: '8080', targetPort: '8080' } });

    caption('Bước 1: Lập trình viên đăng nhập và tạo ứng dụng mới qua form; trang ứng dụng báo chưa triển khai và chưa chọn kết nối');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    appId = await createApplication(h, NAME, 't12-trang-ung-dung', { connections: null });
    await expect(page.getByRole('status').filter({ hasText: 'Đã tạo ứng dụng. Hãy chọn kết nối cho từng môi trường trong Cài đặt môi trường' })).toContainText('chưa có gì được triển khai');
    await expect(page.getByLabel('Đích triển khai').first()).toContainText('staging chưa có kết nối triển khai');
    await expect(page.getByText('Môi trường này chưa có Workload nào.')).toBeVisible();
    await expect(page.getByText('Chưa có triển khai nào trong staging.')).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Triển khai gần đây' })).toBeVisible();
    check('new application home (real backend): Vietnamese created notice, unconfigured target, empty workload and deployment states');
    mark('home-created');
    await h.read('Ứng dụng mới: chưa có kết nối, chưa có Workload, chưa có triển khai');

    caption('Bước 2: Bấm Xem trước thay đổi khi chưa có kết nối; backend từ chối an toàn, trang hướng dẫn mở Cài đặt môi trường, không gọi Triển khai');
    await h.click(previewButton());
    const alert = page.getByRole('alert');
    await expect(alert).toContainText('Môi trường chưa có kết nối triển khai');
    await expect(alert).toContainText('chưa có gì được lập kế hoạch hoặc triển khai');
    expect(count('/preview')).toBe(1);
    expect(count('/deploy')).toBe(0);
    check('unconfigured Preview (real 422): Vietnamese guidance, no preview panel, zero deploy requests');
    mark('unconfigured');
    await h.moveTo(alert);
    await h.read('Chưa cấu hình: cần chọn kết nối, chưa lập kế hoạch hay triển khai gì');
    await h.click(alert.getByRole('button', { name: 'Mở Cài đặt môi trường' }));
    await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
    await h.click(page.getByRole('button', { name: `← ${NAME}` }));
    await expect(page.getByRole('heading', { name: NAME })).toBeVisible();

    caption('Bước 3: Chọn kết nối Staging trong Cài đặt môi trường và lưu; tóm tắt đích hiển thị Kết nối, Phạm vi triển khai bằng tiếng Việt');
    await setEnvironmentConnections(h, NAME, { staging: 'internal-cluster' });
    const target = page.getByLabel('Đích triển khai').first();
    await expect(target).toContainText('staging · Kết nối internal-cluster (internal-cluster) · Phạm vi triển khai Cluster nội bộ (internal-k8s)');
    check('target summary after UI Save: Kết nối internal-cluster, Phạm vi triển khai "Cluster nội bộ (internal-k8s)", technical IDs intact');
    mark('connection-set');
    await h.moveTo(target);
    await h.read('Staging dùng Kết nối internal-cluster, phạm vi Cluster nội bộ');

    caption('Bước 4: Thêm hai Workload api và web bằng form; lưu chỉ tạo draft, trạng thái Chờ thay đổi, chưa triển khai');
    await addWorkload(h, spec('api'));
    await expect(row('api')).toContainText('Chờ thay đổi');
    await addWorkload(h, spec('web'));
    await expect(row('web')).toContainText('Chờ thay đổi');
    await expect(row('api')).toContainText('Chờ thay đổi');
    expect(count('/deploy')).toBe(0);
    check('two draft Workloads saved through the UI: both rows "Chờ thay đổi", no deploy request');
    mark('drafts-saved');
    await h.read('Hai Workload ở trạng thái Chờ thay đổi');

    caption('Bước 5: Xem trước thay đổi (chỉ đọc): bảng xem trước tiếng Việt, token giữ nguyên, danh sách không đổi, chưa triển khai');
    const draftsBefore = JSON.stringify(await apiWorkloads('staging'));
    await h.click(previewButton());
    await expect(previewPanel()).toContainText('2 Workload bị ảnh hưởng');
    await expect(previewPanel().locator('li').filter({ hasText: /^api · triển khai/ })).toContainText('Kubernetes tích hợp sẵn');
    await expect(previewPanel().getByLabel('Đích triển khai')).toContainText('Kết nối internal-cluster');
    await expect(deployButton()).toBeEnabled();
    expect(count('/deploy')).toBe(0);
    expect(JSON.stringify(await apiWorkloads('staging'))).toBe(draftsBefore);
    check('Preview (real): "2 Workload bị ảnh hưởng", action words Vietnamese, target in panel, Deploy only enabled button, drafts unchanged, no deploy');
    mark('preview-pending');
    await h.moveTo(previewPanel());
    await h.read('Xem trước chỉ đọc; Triển khai vẫn là thao tác tường minh');

    caption('Bước 6: MOCK Triển khai một phần: web thành công, api thất bại; không hiện lỗi thô, thay đổi chưa xong vẫn chờ');
    await page.route('**/environments/staging/deploy', async (route) => {
      mocked('POST staging/deploy answered with fake PARTIAL (web SUCCEEDED, api FAILED with secret-like error text); backend deployed nothing');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ status: 'PARTIAL', results: [
        { workloadId: 'web', action: 'DEPLOY', status: 'SUCCEEDED', deploymentId: 'mock-dep-web' },
        { workloadId: 'api', action: 'DEPLOY', status: 'FAILED', error: SECRET_TEXT },
      ] }) });
    });
    await h.click(deployButton());
    await expect(resultPanel().getByRole('heading', { name: 'Triển khai một phần' })).toBeVisible();
    await expect(resultPanel()).toContainText('web · triển khai: thành công');
    await expect(resultPanel()).toContainText('api · triển khai: thất bại');
    await expect(resultPanel()).toContainText('Thay đổi chưa hoàn tất vẫn ở trạng thái chờ');
    await expect(resultPanel().getByRole('link', { name: 'xem triển khai' })).toHaveCount(1);
    await noRaw();
    await expect(row('api')).toContainText('Chờ thay đổi');
    await expect(previewPanel()).toHaveCount(0);
    await expect(deployButton()).toHaveCount(0);
    check('partial report (MOCKED deploy reply; real list reload): per-Workload Vietnamese results, one deployment link, no raw error text, rows still pending, Deploy gone until fresh Preview');
    mark('partial-report');
    await h.moveTo(resultPanel());
    await h.read('Kết quả từng Workload; lỗi thô không hiển thị; không hoàn tác tự động');

    caption('Bước 7: Thử lại bằng Xem trước mới (đọc backend thật), rồi Triển khai thật: cả hai Workload thành công và trở thành Sẵn sàng');
    await page.unroute('**/environments/staging/deploy');
    const previewsBefore = count('/preview');
    await h.click(previewButton());
    await expect(previewPanel()).toContainText('2 Workload bị ảnh hưởng');
    expect(count('/preview')).toBe(previewsBefore + 1);
    await expect(resultPanel()).toHaveCount(0);
    await h.click(deployButton());
    await expect(resultPanel().getByRole('heading', { name: 'Triển khai thành công' })).toBeVisible({ timeout: 120_000 });
    await expect(resultPanel()).toContainText('api · triển khai: thành công');
    await expect(resultPanel()).toContainText('web · triển khai: thành công');
    await expect(row('api')).toContainText('Sẵn sàng');
    await expect(row('web')).toContainText('Sẵn sàng');
    check('retry = fresh Preview request, then real Deploy (fake deliverer) succeeded for both; rows "Sẵn sàng"');
    mark('retry-deploy-ok');
    await h.moveTo(resultPanel());
    await h.read('Thử lại phải xem trước lại; Triển khai thật thành công');

    caption('Bước 8: Triển khai gần đây hiển thị thời gian, trạng thái và hành động bằng tiếng Việt; thứ tự và liên kết theo dữ liệu thật');
    const recent = page.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Triển khai gần đây' }) });
    await expect(recent.locator('button.deployment-entry').first()).toContainText('Thành công');
    await expect(recent.locator('button.deployment-entry').first()).toContainText('triển khai');
    await expect(recent.getByRole('button', { name: 'Xem tất cả' })).toBeVisible();
    expect(await recent.locator('button.deployment-entry').count()).toBeLessThanOrEqual(5);
    check('recent deployments (real): Vietnamese heading, Thành công badge, action word, "Xem tất cả", at most five entries');
    mark('recent-deployments');
    await h.moveTo(recent);
    await h.read('Triển khai gần đây bằng tiếng Việt');

    caption('Bước 9: Xem trước khi không còn gì thay đổi: thông báo no-op, nút Triển khai bị khóa');
    await h.click(previewButton());
    await expect(page.getByText('Không có thay đổi Workload nào để triển khai.')).toBeVisible();
    await expect(deployButton()).toBeDisabled();
    check('no-op Preview (real): Vietnamese no-change message, Deploy disabled');
    mark('noop-preview');
    await h.read('Không có thay đổi: Deploy bị khóa');

    caption('Bước 10: Xóa Workload web: hủy xác nhận thì không xóa; đồng ý thì Chờ xóa; Hoàn tác trả về Sẵn sàng');
    const deletesBefore = count('/workloads/web');
    await answerConfirm(row('web').getByRole('button', { name: 'Xóa' }), 'Escape', 'cancel');
    expect(dialogs.at(-1)).toBe('Đánh dấu xóa web trong staging? Workload đang chạy chỉ thay đổi sau khi Triển khai.');
    expect(count('/workloads/web')).toBe(deletesBefore);
    await expect(row('web')).toContainText('Sẵn sàng');
    await answerConfirm(row('web').getByRole('button', { name: 'Xóa' }), 'Return', 'accept');
    await expect(row('web')).toContainText('Chờ xóa');
    expect((await apiWorkloads('staging')).find((item) => item.id === 'web')?.state).toBe('PENDING_DELETE');
    await expect(previewPanel()).toHaveCount(0);
    mark('delete-pending');
    await h.read('Xác nhận có tên Workload và môi trường; xóa chỉ là draft');
    await h.click(row('web').getByRole('button', { name: 'Hoàn tác' }));
    await expect(row('web')).toContainText('Sẵn sàng');
    check('delete confirm text names web+staging; cancel sent no DELETE; accepted => PENDING_DELETE (real); Hoàn tác => Sẵn sàng');
    mark('undo');

    caption('Bước 11: MOCK stale khi Triển khai (409): bỏ xem trước cũ, tải lại danh sách, yêu cầu xem trước lại, không tự triển khai lại');
    await answerConfirm(row('web').getByRole('button', { name: 'Xóa' }), 'Return', 'stale-deploy');
    await expect(row('web')).toContainText('Chờ xóa');
    await h.click(previewButton());
    await expect(previewPanel()).toContainText('web · gỡ bỏ');
    await page.route('**/environments/staging/deploy', async (route) => {
      mocked('POST staging/deploy answered with fake 409 (stale), raw detail text must not render');
      await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'stale detail: token=ABC-SECRET-777' }) });
    });
    const deploysBefore = count('/deploy');
    await h.click(deployButton());
    await expect(page.getByRole('status').filter({ hasText: 'đã thay đổi kể từ khi trang được tải hoặc xem trước' })).toContainText('xem trước lại');
    await expect(previewPanel()).toHaveCount(0);
    await expect(deployButton()).toHaveCount(0);
    await expect(row('web')).toContainText('Chờ xóa');
    await noRaw();
    expect(count('/deploy')).toBe(deploysBefore + 1);
    await page.unroute('**/environments/staging/deploy');
    check('stale Deploy (MOCKED 409): Vietnamese notice, preview dropped, list reloaded from backend, exactly one deploy attempt, no raw text');
    mark('stale-deploy');
    await h.read('Xem trước đã cũ: tải lại danh sách, không tự triển khai lại');
    await h.click(row('web').getByRole('button', { name: 'Hoàn tác' }));
    await expect(row('web')).toContainText('Sẵn sàng');

    caption('Bước 12: MOCK stale khi Xóa (409): thông báo tiếng Việt, danh sách tải lại, thao tác không được thử lại ngầm');
    await page.route('**/environments/staging/workloads/api', async (route) => {
      if (route.request().method() !== 'DELETE') { await route.continue(); return; }
      mocked('DELETE staging/workloads/api answered with fake 409 (stale)');
      await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: 'stale detail: token=ABC-SECRET-777' }) });
    });
    const apiDeletes = count('/workloads/api');
    await answerConfirm(row('api').getByRole('button', { name: 'Xóa' }), 'Return', 'stale-delete');
    await expect(page.getByRole('status').filter({ hasText: 'đã thay đổi kể từ khi trang được tải hoặc xem trước' })).toBeVisible();
    await expect(row('api')).toContainText('Sẵn sàng');
    expect(count('/workloads/api')).toBe(apiDeletes + 1);
    await noRaw();
    await page.unroute('**/environments/staging/workloads/api');
    check('stale Delete (MOCKED 409): Vietnamese notice, real reload shows api unchanged, one DELETE only, no raw text');
    mark('stale-delete');
    await h.read('Xóa gặp xung đột: danh sách được tải lại và không thử lại ngầm');

    caption('Bước 13: MOCK giữ phản hồi Xem trước 3.5s rồi chuyển sang Production; phản hồi muộn của staging bị bỏ qua');
    await page.route('**/environments/staging/preview', async (route) => {
      mocked('POST staging/preview held 3.5s then forwarded to the real backend (late response)');
      await new Promise((resolve) => setTimeout(resolve, 3500));
      await route.continue();
    });
    await h.click(previewButton());
    await expect(page.getByRole('button', { name: 'Đang tính toán xem trước…' })).toBeDisabled();
    await h.click(page.getByRole('button', { name: /^Production/i }));
    await expect(page.getByText('Môi trường này chưa có Workload nào.')).toBeVisible();
    await expect(page.getByLabel('Đích triển khai').first()).toContainText('production chưa có kết nối triển khai');
    await page.waitForTimeout(4500);
    await expect(previewPanel()).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Xem trước cho staging' })).toHaveCount(0);
    await page.unroute('**/environments/staging/preview');
    check('late staging Preview (MOCKED delay, real reply) ignored after switching to Production; Production shows its own empty list and target');
    mark('late-scope');
    await h.read('Phản hồi muộn của staging không xuất hiện ở production');
    await h.click(page.getByRole('button', { name: /^Staging/i }));
    await expect(row('api')).toContainText('Sẵn sàng');

    caption('Bước 14: Đăng xuất');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    check(`mocks used (${mocks.length} activations, see mocks.json); real: create/connection/drafts/preview/deploy/delete/undo/list/history`);
    mark('signed-out');
    await h.pause(3000);
  },

  // T13: Standalone Score Preview. Developer. PRODUCT (UI, backend fake thật, state tạm): đăng nhập, tạo ứng dụng, mở trang Xem trước Score, validation cục bộ,
  // 422 chưa cấu hình thật, chọn kết nối trong Cài đặt môi trường, Score hợp lệ => kết quả thật (Delta/graph/lô/tài liệu), 400 thật (ID không khớp metadata.name),
  // đổi thao tác sang xóa (payload chỉ có scoreBefore). MOCK (page.route, mỗi lần dùng ghi vào mocks.json): (1) score-preview trả 413 giả; (2) 500 giả rồi Thử lại đến
  // backend thật; (3) 404 giả; (4) phản hồi bị giữ ~3.5s rồi chuyển Production (đến muộn bị bỏ qua); (5) 401 giả => phiên hết hạn. Backend fake không tạo được 413/500/404/401
  // theo yêu cầu. Setup độc lập (không phải bước product): không có; mọi bước đều qua UI. Mọi request không phải GET sau thiết lập chỉ được là POST score-preview.
  async T13({ page, h, ui, caption, mark, check, baseURL }) {
    const NAME = 'T13 Xem trước Score';
    const RAW = 'RAW-BACKEND-TEXT-XYZ';
    const mocks = [];
    const mocked = (what) => { mocks.push(what); appendFileSync(`${process.env.ORCH_E2E_EVIDENCE_DIR}/mocks.json`, `${JSON.stringify({ at: new Date().toISOString(), what })}\n`); };
    const writes = [];
    const previews = [];
    page.on('request', (request) => {
      if (request.method() === 'GET') return;
      const path = new URL(request.url()).pathname;
      if (path.endsWith('/score-preview')) previews.push(request.postDataJSON());
      writes.push(`${request.method()} ${path.replace(/^\/api\/v1\/applications\/[^/]+\//, '')}`);
    });
    const nonPreviewWrites = (from) => writes.slice(from).filter((line) => !line.endsWith('/score-preview'));
    let appId = '';
    const apiGet = async (suffix) => (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/${suffix}`)).json());
    const submit = () => page.getByRole('button', { name: 'Xem trước', exact: true });
    const alert = () => page.getByRole('alert');
    const result = () => page.getByRole('region', { name: 'Kết quả xem trước Score' });
    const noRaw = async () => expect(await page.locator('body').innerText()).not.toMatch(new RegExp(`${RAW}|token=|does not match|metadata\\.name "`));
    const apiScore = 'apiVersion: score.dev/v1b1\nmetadata:\n  name: api\ncontainers:\n  main:\n    image: registry.example/api:t13\n';
    const fillBasic = async (workload, run, score) => {
      await h.type(page.getByLabel('ID Workload'), workload, { replace: true });
      await h.type(page.getByLabel('ID lần chạy'), run, { replace: true });
      if (score !== undefined) await h.type(page.getByLabel('Score sau (YAML hoặc JSON)'), score, { delay: LONG_TYPE_DELAY, replace: true });
    };
    const preview = (suffix) => page.route(`**/environments/*/score-preview`, suffix);
    const openPreview = async () => {
      await h.click(page.getByRole('button', { name: 'Xem trước Score' }));
      await expect(page.getByRole('heading', { name: 'Xem trước Score', level: 1 })).toBeVisible();
    };

    caption('Bước 1: Nhà phát triển đăng nhập; không hiện điều hướng nền tảng; tạo ứng dụng mới chưa có kết nối');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await expect(ui('shell', 'navResourceTypes')).toHaveCount(0);
    await expect(ui('shell', 'navConnections')).toHaveCount(0);
    appId = await createApplication(h, NAME, 't13-xem-truoc-score', { connections: null });
    check('role gate (real): Developer has no platform navigation links; new application created through UI with both Environments unconfigured');
    mark('developer-app');
    const afterSetup = writes.length;
    await h.read('Nhà phát triển không có mục nền tảng; ứng dụng chưa có kết nối');

    caption('Bước 2: Mở trang Xem trước Score; nhãn, hướng dẫn và lưu ý bí mật bằng tiếng Việt; không có nút lưu hay triển khai');
    await openPreview();
    await expect(page).toHaveURL(`${baseURL}/ui/applications/${appId}/environments/staging/preview`);
    await expect(page.getByRole('note')).toContainText('Không dán giá trị bí mật');
    await expect(page.getByLabel('Thao tác')).toBeVisible();
    await expect(page.getByLabel('ID Workload')).toHaveAttribute('placeholder', 'phải trùng metadata.name');
    await expect(page.getByLabel('Score sau (YAML hoặc JSON)')).toBeVisible();
    await expect(page.getByLabel('Score trước (YAML hoặc JSON)')).toHaveCount(0);
    await expect(page.getByRole('tablist', { name: 'Môi trường' })).toBeVisible();
    await expect(page.getByRole('button', { name: /triển khai|lưu/i })).toHaveCount(0);
    check('preview page (real): Vietnamese labels/note/placeholder, add action hides Score trước, no save/deploy button');
    mark('preview-page');
    await h.moveTo(page.getByRole('note'));
    await h.read('Trang xem trước chỉ đọc, nhắc không dán bí mật');

    caption('Bước 3: Gửi form trống: lỗi cục bộ tiếng Việt, không gọi backend, nội dung đã nhập được giữ');
    await h.click(submit());
    await expect(alert()).toContainText('ID Workload là bắt buộc.');
    await expect(alert()).toContainText('ID lần chạy là bắt buộc.');
    await expect(alert()).toContainText('Score sau là bắt buộc.');
    expect(previews).toHaveLength(0);
    check('empty submit: three Vietnamese local errors, zero score-preview requests');
    mark('local-empty');
    await h.moveTo(alert());
    await h.read('Ba lỗi cục bộ bằng tiếng Việt');
    const broken = 'containers: [';
    await fillBasic('api', 'run-t13-1', broken);
    await h.click(submit());
    await expect(alert()).toContainText('Score sau không phải YAML hoặc JSON hợp lệ.');
    await expect(page.getByLabel('Score sau (YAML hoặc JSON)')).toHaveValue(broken);
    expect(previews).toHaveLength(0);
    await noRaw();
    expect(await page.locator('body').innerText()).not.toMatch(/Flow sequence|Unexpected|at line/i);
    check('invalid YAML: Vietnamese parser message, raw yaml text hidden, input kept, still zero requests');
    mark('local-invalid-yaml');
    await h.moveTo(alert());
    await h.read('Lỗi YAML bằng tiếng Việt, không lộ thông báo thô');

    caption('Bước 4: Score hợp lệ khi môi trường chưa có kết nối: backend thật từ chối 422, trang hướng dẫn mở Cài đặt môi trường');
    await h.type(page.getByLabel('Score sau (YAML hoặc JSON)'), apiScore, { delay: LONG_TYPE_DELAY, replace: true });
    await h.click(submit());
    await expect(alert()).toContainText('Môi trường staging chưa có kết nối triển khai');
    await expect(alert()).toContainText('Cài đặt môi trường');
    await expect(alert().getByRole('button', { name: 'Thử lại' })).toBeVisible();
    await expect(result()).toHaveCount(0);
    expect(previews).toHaveLength(1);
    expect(previews[0]).toEqual({ workloadId: 'api', action: 'deploy', runId: 'run-t13-1', scoreAfter: { apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'registry.example/api:t13' } } } });
    await noRaw();
    check('unconfigured (real 422): Vietnamese guidance, Thử lại offered, no result, payload = deploy + parsed scoreAfter only');
    mark('unconfigured');
    await h.moveTo(alert());
    await h.read('Chưa cấu hình: hãy chọn kết nối, chưa có gì được lập kế hoạch');

    caption('Bước 5: Quay lại ứng dụng, chọn kết nối Staging trong Cài đặt môi trường rồi mở lại Xem trước Score');
    await h.click(page.getByRole('button', { name: `← ${NAME}` }));
    await expect(page.getByRole('heading', { name: NAME })).toBeVisible();
    await setEnvironmentConnections(h, NAME, { staging: 'internal-cluster' });
    await openPreview();
    await expect(page.getByLabel('Đích triển khai')).toContainText('Kết nối internal-cluster');
    await expect(page.getByLabel('ID Workload')).toHaveValue('');
    check('after UI connection save: preview page shows Kết nối internal-cluster; form starts empty (input not persisted)');
    mark('connection-set');

    caption('Bước 6: Backend thật từ chối ID Workload không khớp metadata.name: thông báo Việt an toàn, không lặp lại giá trị đã gửi, giữ nguyên input');
    await fillBasic('backend', 'run-t13-2', apiScore);
    await h.click(submit());
    await expect(alert()).toContainText('Máy chủ không chấp nhận dữ liệu xem trước');
    await expect(alert()).not.toContainText('backend');
    await expect(alert().getByRole('button', { name: 'Thử lại' })).toHaveCount(0);
    await expect(page.getByLabel('Score sau (YAML hoặc JSON)')).toHaveValue(apiScore);
    await noRaw();
    check('server 400 (real): Vietnamese generic message without echoed workload id/raw text, no retry button, input kept');
    mark('server-400');
    await h.moveTo(alert());
    await h.read('Lỗi 400 an toàn bằng tiếng Việt');

    caption('Bước 7: Sửa ID Workload và xem trước: kết quả thật gồm Delta, thay đổi tài nguyên, thứ tự tạo, liên kết thực thi, đồ thị, tài liệu');
    await h.type(page.getByLabel('ID Workload'), 'api', { replace: true });
    await expect(alert()).toHaveCount(0);
    await h.click(submit());
    await expect(result()).toBeVisible();
    await expect(result()).toContainText('Xem trước · api · Thêm Workload');
    await expect(result()).toContainText('Kết quả chỉ đọc');
    const panel = (name) => result().locator('section.content-panel').filter({ has: page.getByRole('heading', { name, exact: true }) });
    await expect(panel('Delta')).toContainText('thêm module api');
    await expect(panel('Thay đổi tài nguyên')).toContainText('Mới (');
    await expect(panel('Thứ tự tạo tài nguyên')).toContainText('Lô 1:');
    await expect(panel('Liên kết thực thi tài nguyên')).toContainText('Kết nối của môi trường internal-cluster');
    await expect(panel('Đồ thị tài nguyên')).toContainText('cạnh phụ thuộc');
    await expect(panel('Tài liệu (bản đã lọc dữ liệu nhạy cảm)')).toContainText('***redacted***');
    await expect(result()).toContainText('run-t13-2');
    const last = previews.at(-1);
    expect(last.action).toBe('deploy');
    expect(Object.keys(last).sort()).toEqual(['action', 'runId', 'scoreAfter', 'workloadId']);
    check('preview result (real): Vietnamese sections, Delta "thêm module api", Kết nối của môi trường binding, graph and sanitized documents present; payload keys exact');
    mark('result');
    await h.moveTo(result().getByRole('heading', { name: 'Delta', exact: true }));
    await h.read('Delta: thêm module api');
    await h.moveTo(panel('Đồ thị tài nguyên'));
    await h.pause(SHORT_READ);
    const details = panel('Tài liệu (bản đã lọc dữ liệu nhạy cảm)').locator('summary');
    await h.click(details.first());
    await expect(panel('Tài liệu (bản đã lọc dữ liệu nhạy cảm)').locator('pre').first()).toContainText('api');
    await h.click(details.nth(1));
    await expect(panel('Tài liệu (bản đã lọc dữ liệu nhạy cảm)').locator('pre').nth(1)).toContainText('modules');
    mark('documents');
    await h.read('Tài liệu Delta và Candidate là bản đã lọc dữ liệu nhạy cảm');
    await expect(page.getByRole('button', { name: /triển khai|lưu/i })).toHaveCount(0);

    caption('Bước 8: MOCK 413 giả rồi 404 giả: thông báo tiếng Việt, không có raw backend text');
    await preview(async (route) => {
      mocked('POST score-preview answered with fake 413 (raw text must not render)');
      await route.fulfill({ status: 413, contentType: 'application/json', body: JSON.stringify({ error: RAW }) });
    });
    await h.click(submit());
    await expect(alert()).toContainText('Score quá lớn để xem trước');
    await expect(result()).toHaveCount(0);
    await noRaw();
    check('413 (MOCKED): Vietnamese, old result cleared, no raw text');
    mark('mock-413');
    await h.read('Lỗi 413 bằng tiếng Việt');
    await page.unroute('**/environments/*/score-preview');
    await preview(async (route) => {
      mocked('POST score-preview answered with fake 404 (raw text must not render)');
      await route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ error: RAW }) });
    });
    await h.click(submit());
    await expect(alert()).toContainText('Không tìm thấy ứng dụng hoặc môi trường này.');
    await noRaw();
    check('404 (MOCKED): Vietnamese, no raw text');
    mark('mock-404');
    await h.read('Lỗi 404 bằng tiếng Việt');
    await page.unroute('**/environments/*/score-preview');

    caption('Bước 9: MOCK 500 giả: thông báo Việt có Thử lại; Thử lại chỉ gọi score-preview và nhận kết quả thật');
    let fail500 = true;
    await preview(async (route) => {
      if (fail500) { fail500 = false; mocked('POST score-preview answered with fake 500 once (raw text must not render)'); await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: RAW }) }); return; }
      await route.continue();
    });
    await h.click(submit());
    await expect(alert()).toContainText('Không thể xem trước Score. Hãy thử lại.');
    await noRaw();
    mark('mock-500');
    await h.moveTo(alert());
    await h.read('Lỗi 500 có nút Thử lại');
    const writesBeforeRetry = writes.length;
    await h.click(alert().getByRole('button', { name: 'Thử lại' }));
    await expect(result()).toBeVisible();
    await expect(alert()).toHaveCount(0);
    expect(writes.slice(writesBeforeRetry).every((line) => line.endsWith('/score-preview'))).toBe(true);
    await page.unroute('**/environments/*/score-preview');
    check('500 (MOCKED once): Vietnamese + Thử lại; retry sent only POST score-preview and showed the real result');
    mark('retry-ok');
    await h.read('Thử lại thành công với backend thật');

    caption('Bước 10: Đổi thao tác sang Xóa Workload: chỉ còn Score trước, kết quả cũ bị xóa; gửi payload remove');
    await h.choose(page.getByLabel('Thao tác'), 'Xóa Workload');
    await expect(page.getByLabel('Thao tác')).toHaveValue('remove');
    await expect(result()).toHaveCount(0);
    await expect(page.getByLabel('Score sau (YAML hoặc JSON)')).toHaveCount(0);
    await h.type(page.getByLabel('Score trước (YAML hoặc JSON)'), '{"metadata":{"name":"api"}}', { delay: LONG_TYPE_DELAY });
    await h.click(submit());
    await expect(page.getByRole('alert').or(result())).toBeVisible();
    expect(previews.at(-1)).toEqual({ workloadId: 'api', action: 'remove', runId: 'run-t13-2', scoreBefore: { metadata: { name: 'api' } } });
    check('remove action: Score sau editor hidden, old result cleared, payload has scoreBefore only (backend outcome not asserted: workload is not deployed)');
    mark('remove-payload');
    await h.read('Payload remove chỉ chứa Score trước');
    await expect(await page.locator('body').innerText()).not.toContain(RAW);

    caption('Bước 11: MOCK giữ phản hồi 3.5s rồi chuyển sang Production; phản hồi muộn của staging bị bỏ qua và Production báo chưa có kết nối');
    await h.choose(page.getByLabel('Thao tác'), 'Thêm Workload');
    await fillBasic('api', 'run-t13-3', apiScore);
    await preview(async (route) => {
      mocked('POST score-preview held 3.5s then forwarded to the real backend (late response)');
      await new Promise((resolve) => setTimeout(resolve, 3500));
      await route.continue();
    });
    await h.click(submit());
    await expect(page.getByRole('button', { name: 'Đang tính kết quả xem trước…' })).toBeDisabled();
    await expect(page.getByRole('status').filter({ hasText: 'Đang tính kết quả xem trước…' })).toBeVisible();
    await h.click(page.getByRole('tab', { name: 'production', exact: true }));
    await expect(page.getByRole('tab', { name: 'production', exact: true })).toHaveAttribute('aria-selected', 'true');
    await expect(page).toHaveURL(`${baseURL}/ui/applications/${appId}/environments/production/preview`);
    await expect(page.getByLabel('Đích triển khai')).toContainText('production chưa có kết nối triển khai');
    await page.waitForTimeout(4500);
    await expect(result()).toHaveCount(0);
    await expect(alert()).toHaveCount(0);
    await page.unroute('**/environments/*/score-preview');
    check('late response (MOCKED delay, real reply) ignored after scope switch; Production shows unconfigured target, no stale result');
    mark('late-scope');
    await h.read('Phản hồi muộn của staging không hiện ở production');

    caption('Bước 12: Xác minh chỉ đọc qua API: không tạo draft, không có triển khai, mọi request ghi sau thiết lập chỉ là POST score-preview');
    expect((await apiGet('workloads')).workloads ?? []).toEqual([]);
    expect((await apiGet('deployments')).deployments ?? []).toEqual([]);
    expect(nonPreviewWrites(afterSetup).filter((line) => !/\/connection$/.test(line))).toEqual([]);
    check(`read-only (real API reads): zero drafts, zero deployments; non-preview writes after setup were only the UI connection PUT (${nonPreviewWrites(afterSetup).length})`);
    mark('read-only');
    await h.pause(SHORT_READ);

    caption('Bước 13: MOCK 401 giả khi xem trước: phiên hết hạn, quay về đăng nhập theo luồng chung, không hiện kết quả');
    await h.click(page.getByRole('tab', { name: 'staging', exact: true }));
    await expect(page.getByRole('tab', { name: 'staging', exact: true })).toHaveAttribute('aria-selected', 'true');
    await fillBasic('api', 'run-t13-4', apiScore);
    await preview(async (route) => {
      mocked('POST score-preview answered with fake 401 (session expired)');
      await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: RAW }) });
    });
    await h.click(submit());
    await expect(ui('signIn', 'username')).toBeVisible();
    await expect(page.getByText('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.')).toBeVisible();
    await noRaw();
    await page.unroute('**/environments/*/score-preview');
    check(`401 (MOCKED): sign-in page with Vietnamese expired notice, no raw text; mocks used (${mocks.length} activations, see mocks.json)`);
    mark('session-expired');
    await h.read('Phiên hết hạn được báo bằng tiếng Việt, không lộ nội dung thô');

    caption('Bước 14: Đăng nhập lại rồi đăng xuất để kết thúc phiên');
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    check('sign out (real): signed back in after the MOCKED 401, then signed out through the shell');
    mark('signed-out');
    await h.pause(3000);
  },

  // T14: Lịch sử triển khai. Developer. PRODUCT (UI, backend fake thật với fake deliverer, state tạm): tạo ứng dụng, mở "Xem tất cả" khi trống, thêm 2 draft
  // Workload và Triển khai thật (2 Deployment SUCCEEDED trong staging), lịch sử thật + bộ lọc Thành công/Thất bại do server lọc, mở chi tiết đúng deployment ID, Production
  // trống (scope). MOCK (page.route trên GET .../staging/deployments, mỗi lần dùng ghi vào mocks.json): (1) danh sách chiếu giả đủ 5 enum trạng thái + 3 hành động
  // (fake deliverer chỉ tạo SUCCEEDED/DEPLOY) gồm actor thiếu; (2) 500 giả kèm chuỗi bí mật giả rồi Thử lại đến backend thật; (3) phản hồi giữ ~3.5s để thấy trạng thái
  // đang tải, rồi (4) phản hồi FAILED đến muộn sau khi đổi bộ lọc bị bỏ qua; (5) lọc DEPLOYING giả trả rỗng. Các mock KHÔNG thay đổi backend; mọi bước
  // product (đăng nhập, tạo app, thêm Workload, Triển khai, điều hướng, lọc, mở chi tiết) đi qua UI. Sau khi Triển khai, mọi request không phải GET bị cấm.
  async T14({ page, h, ui, caption, mark, check, baseURL }) {
    const NAME = 'T14 Lịch sử triển khai';
    const RAW = 'provider said token=ABC-SECRET-777';
    const mocks = [];
    const mocked = (what) => { mocks.push(what); appendFileSync(`${process.env.ORCH_E2E_EVIDENCE_DIR}/mocks.json`, `${JSON.stringify({ at: new Date().toISOString(), what })}\n`); };
    const writes = [];
    const listGets = [];
    const detailGets = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (request.method() !== 'GET') { writes.push(`${request.method()} ${url.pathname}`); return; }
      if (/\/environments\/(staging|production)\/deployments$/.test(url.pathname)) listGets.push(`${url.pathname.split('/environments/')[1]}${url.search}`);
      else if (/\/deployments\/[^/]+$/.test(url.pathname)) detailGets.push(decodeURIComponent(url.pathname.split('/').pop()));
    });
    let appId = '';
    const spec = (name) => ({ name, image: `registry.example/${name}:t14`, bindings: [], port: { name: 'http', port: '8080', targetPort: '8080' } });
    const apiList = async (environment, query = '') => (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/${environment}/deployments${query}`)).json()).deployments ?? [];
    const entries = () => page.locator('button.deployment-entry');
    const filter = () => page.getByRole('combobox', { name: 'Trạng thái' });
    const listRoute = /\/environments\/staging\/deployments(\?.*)?$/;
    const noRaw = async () => expect(await page.locator('body').innerText()).not.toMatch(/ABC-SECRET|token=|provider said/);
    const rowText = async () => entries().evaluateAll((nodes) => nodes.map((node) => node.textContent ?? ''));
    const openHistory = async (environmentButton) => {
      await h.click(page.getByRole('button', { name: environmentButton }));
      await h.click(page.getByRole('button', { name: 'Xem tất cả' }));
      await expect(page.getByRole('heading', { name: 'Lịch sử triển khai', level: 1 })).toBeVisible();
    };
    const fake = (id, workloadId, action, status, extra = {}) => ({ id, applicationKey: appId, environmentKey: 'staging', workloadId, action, status, actorRef: 'developer', startedAt: '2026-09-01T10:00:00Z', ...extra });

    caption('Bước 1: Lập trình viên đăng nhập và tạo ứng dụng mới; trang ứng dụng chưa có triển khai nào');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    appId = await createApplication(h, NAME, 't14-lich-su-trien-khai');
    await expect(page.getByText('Chưa có triển khai nào trong staging.')).toBeVisible();
    check('new application (real backend): both Environments connected through the UI, no deployments yet');
    mark('home-created');
    await h.read('Ứng dụng mới chưa có triển khai nào');

    caption('Bước 2: Bấm Xem tất cả: trang Lịch sử triển khai tiếng Việt, trạng thái rỗng thật, bộ lọc Trạng thái và nút Tải lại');
    await h.click(page.getByRole('button', { name: 'Xem tất cả' }));
    await expect(page).toHaveURL(`${baseURL}/ui/applications/${appId}/environments/staging/deployments`);
    await expect(page.getByRole('heading', { name: 'Lịch sử triển khai', level: 1 })).toBeVisible();
    await expect(page.getByText('Chưa có triển khai nào trong môi trường này.')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Tải lại' })).toBeVisible();
    await expect(page.getByRole('button', { name: `← ${NAME}` })).toBeVisible();
    expect(await filter().locator('option').evaluateAll((options) => options.map((option) => [option.value, option.textContent]))).toEqual([
      ['ALL', 'Tất cả'], ['SUCCEEDED', 'Thành công'], ['FAILED', 'Thất bại'], ['DEPLOYING', 'Đang triển khai'], ['PROVISIONING', 'Đang tạo tài nguyên'], ['PLANNING', 'Đang lập kế hoạch'],
    ]);
    expect(await page.locator('body').innerText()).not.toMatch(/Deployment history|No deployments|Refresh|Retry|Loading/);
    check('history page (real empty): Vietnamese title/empty text/Tải lại, filter labels Vietnamese with original enum values, no English leftovers');
    mark('history-empty');
    await h.moveTo(page.getByText('Chưa có triển khai nào trong môi trường này.'));
    await h.read('Lịch sử rỗng bằng tiếng Việt; giá trị bộ lọc vẫn là enum gốc');

    caption('Bước 3: Quay lại ứng dụng, thêm hai Workload api và web rồi Triển khai thật (fake deliverer) để có lịch sử thật');
    await h.click(page.getByRole('button', { name: `← ${NAME}` }));
    await expect(page.getByRole('heading', { name: NAME })).toBeVisible();
    await addWorkload(h, spec('api'));
    await previewAndDeploy(h, ['api']);
    await addWorkload(h, spec('web'));
    await previewAndDeploy(h, ['web']);
    const afterDeploy = writes.length;
    const real = await apiList('staging');
    expect(real.map((item) => `${item.workloadId}/${item.action}/${item.status}`)).toEqual(['web/DEPLOY/SUCCEEDED', 'api/DEPLOY/SUCCEEDED']);
    check('two real Deployments created through the UI (fake deliverer): web newest, then api, both DEPLOY/SUCCEEDED');
    mark('deployed');

    caption('Bước 4: Mở lịch sử thật: đúng hai dòng theo thứ tự máy chủ (mới nhất trước), nhãn Thành công / triển khai, thời gian theo locale vi-VN');
    await openHistory(/^Staging/i);
    await expect(entries()).toHaveCount(2);
    const texts = await rowText();
    const expectedTime = await page.evaluate((iso) => new Date(iso).toLocaleString('vi-VN'), real[0].startedAt);
    expect(texts[0]).toContain('web · triển khai');
    expect(texts[1]).toContain('api · triển khai');
    expect(texts[0]).toContain('Thành công');
    expect(texts[0]).toContain(expectedTime);
    expect(real[0].actorRef).toBeTruthy();
    expect(texts[0]).toContain(real[0].actorRef);
    expect(texts.join(' ')).not.toMatch(/SUCCEEDED|DEPLOY\b/);
    check(`real history: rows in server order [web, api], Vietnamese action/status, vi-VN time "${expectedTime}", actor kept, no raw enums`);
    mark('history-real');
    await h.moveTo(entries().first());
    await h.read('Lịch sử thật: web mới nhất, api sau; trạng thái và hành động bằng tiếng Việt');

    caption('Bước 5: Lọc Thành công: yêu cầu gửi ?status=SUCCEEDED (enum gốc); lọc Thất bại: backend thật trả rỗng và hiện thông báo tiếng Việt');
    await h.choose(filter(), 'Thành công');
    await expect(filter()).toHaveValue('SUCCEEDED');
    await expect(entries()).toHaveCount(2);
    expect(listGets.at(-1)).toMatch(/staging\/deployments\?status=SUCCEEDED$/);
    await h.choose(filter(), 'Thất bại');
    await expect(page.getByText('Chưa có triển khai nào ở trạng thái “Thất bại”.')).toBeVisible();
    expect(listGets.at(-1)).toMatch(/staging\/deployments\?status=FAILED$/);
    expect(await page.locator('body').innerText()).not.toMatch(/trạng thái “FAILED”/);
    check('server filter (real): SUCCEEDED returned both rows, FAILED returned empty with Vietnamese status label; query values are original enums');
    mark('filter-real');
    await h.read('Bộ lọc gửi enum gốc; thông báo rỗng dùng nhãn tiếng Việt');
    await h.choose(filter(), 'Tất cả');
    await expect(entries()).toHaveCount(2);

    caption('Bước 6: Mở dòng api: giữ đúng deployment ID trong route chi tiết (trang chi tiết thuộc T15 nên không kiểm nội dung)');
    const apiId = real.find((item) => item.workloadId === 'api').id;
    await h.click(entries().filter({ hasText: 'api · triển khai' }));
    await expect(page).toHaveURL(`${baseURL}/ui/applications/${appId}/environments/staging/deployments/${encodeURIComponent(apiId)}`);
    await expect.poll(() => detailGets.includes(apiId)).toBe(true);
    check(`open details (real): URL carries selected deployment ID ${apiId}; detail GET issued for that ID only`);
    mark('open-detail');
    await h.read('Mở chi tiết giữ đúng ID triển khai đã chọn');
    expect([...new Set(detailGets)]).toEqual([apiId]);
    await page.goBack();
    await expect(page.getByRole('heading', { name: 'Lịch sử triển khai', level: 1 })).toBeVisible();
    await expect(entries()).toHaveCount(2);

    caption('Bước 7: MOCK danh sách chiếu giả: đủ 5 trạng thái, 3 hành động và actor thiếu, thứ tự giữ nguyên; backend không tạo được các trạng thái này');
    await page.route(listRoute, async (route) => {
      mocked('GET staging/deployments answered with fake projection: all 5 statuses, 3 actions, one missing actor');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ deployments: [
        fake('m-1', 'svc-planning', 'DEPLOY', 'PLANNING'),
        fake('m-2', 'svc-provisioning', 'UPDATE', 'PROVISIONING'),
        fake('m-3', 'svc-deploying', 'REMOVE', 'DEPLOYING'),
        fake('m-4', 'svc-failed', 'DEPLOY', 'FAILED', { actorRef: '' }),
        fake('m-5', 'svc-ok', 'UPDATE', 'SUCCEEDED'),
      ] }) });
    });
    await h.click(page.getByRole('button', { name: 'Tải lại' }));
    await expect(entries()).toHaveCount(5);
    const mockRows = await rowText();
    expect(mockRows.map((text) => text.split('·')[0])).toEqual(['svc-planning ', 'svc-provisioning ', 'svc-deploying ', 'svc-failed ', 'svc-ok ']);
    for (const [index, needles] of [['Đang lập kế hoạch', 'triển khai'], ['Đang tạo tài nguyên', 'cập nhật'], ['Đang triển khai', 'gỡ bỏ'], ['Thất bại', 'Không rõ người thực hiện'], ['Thành công', 'cập nhật']].entries()) {
      for (const needle of needles) expect(mockRows[index]).toContain(needle);
    }
    expect(mockRows.join(' ')).not.toMatch(/PLANNING|PROVISIONING|DEPLOYING|FAILED|SUCCEEDED|REMOVE|UPDATE/);
    check('projection (MOCKED list): all 5 statuses and 3 actions render Vietnamese in server order; missing actor "Không rõ người thực hiện"; no raw enums');
    mark('mock-all-statuses');
    await h.moveTo(entries().first());
    await h.read('Đủ năm trạng thái bằng tiếng Việt (dữ liệu giả, backend không tạo được)');
    await page.unroute(listRoute);

    caption('Bước 8: MOCK 500 giả kèm chuỗi bí mật giả: thông báo an toàn, không phải danh sách rỗng; Thử lại đến backend thật');
    let failOnce = true;
    await page.route(listRoute, async (route) => {
      if (failOnce) { failOnce = false; mocked('GET staging/deployments answered with fake 500 once (secret-like raw text must not render)'); await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: RAW }) }); return; }
      await route.continue();
    });
    await h.click(page.getByRole('button', { name: 'Tải lại' }));
    await expect(page.getByRole('alert')).toContainText('Không thể tải lịch sử triển khai.');
    await expect(entries()).toHaveCount(0);
    await expect(page.getByText(/Chưa có triển khai/)).toHaveCount(0);
    await noRaw();
    mark('mock-500');
    await h.moveTo(page.getByRole('alert'));
    await h.read('Lỗi an toàn bằng tiếng Việt, không lộ nội dung thô');
    await h.click(page.getByRole('alert').getByRole('button', { name: 'Thử lại' }));
    await expect(entries()).toHaveCount(2);
    await expect(page.getByRole('alert')).toHaveCount(0);
    await page.unroute(listRoute);
    check('error (MOCKED 500 once): Vietnamese safe alert with Thử lại, not shown as empty, no raw text; retry reached the real backend and showed the 2 real rows');
    mark('retry-ok');
    await h.read('Thử lại thành công với dữ liệu thật');

    caption('Bước 9: MOCK giữ phản hồi lọc Thất bại bằng cổng điều khiển: đang tải có aria-busy và danh sách cũ bị xóa; đổi lại Tất cả, tải xong rồi mới nhả phản hồi muộn');
    let releaseFailed = () => undefined;
    const failedGate = new Promise((resolve) => { releaseFailed = resolve; });
    let lateFulfilled = false;
    await page.route(listRoute, async (route) => {
      if (!route.request().url().includes('status=FAILED')) { await route.continue(); return; }
      mocked('GET staging/deployments?status=FAILED held on an explicit gate (released only after the ALL list loaded) then answered with a fake FAILED row (late response)');
      await failedGate;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ deployments: [fake('m-late', 'late-failed', 'FAILED', 'FAILED')] }) });
      lateFulfilled = true;
    });
    await h.choose(filter(), 'Thất bại');
    await expect(page.getByRole('status')).toHaveAttribute('aria-busy', 'true');
    await expect(page.getByRole('status')).toContainText('Đang tải lịch sử triển khai…');
    await expect(entries()).toHaveCount(0);
    await expect(filter()).toBeVisible();
    mark('loading');
    await h.read('Đang tải: danh sách cũ đã bị xóa, bộ lọc vẫn hiện');
    await h.choose(filter(), 'Tất cả');
    await expect(entries()).toHaveCount(2);
    await expect(filter()).toHaveValue('ALL');
    expect(lateFulfilled).toBe(false);
    const lateResponse = page.waitForResponse((response) => response.url().includes('status=FAILED'));
    releaseFailed();
    await (await lateResponse).finished();
    await expect.poll(() => lateFulfilled).toBe(true);
    // Two animation frames: the page has run the settled fetch continuation before the no-stale assertions.
    await page.evaluate('new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
    await expect(page.getByText('late-failed')).toHaveCount(0);
    await expect(entries()).toHaveCount(2);
    await expect(filter()).toHaveValue('ALL');
    await expect(page.getByRole('status')).toHaveCount(0);
    await page.unroute(listRoute);
    check('loading + late reply (MOCKED gate): aria-busy status and cleared rows while loading; FAILED reply released only after ALL loaded and completed, then ignored (no late-failed row, still 2 rows)');
    mark('late-filter');
    await h.read('Phản hồi muộn của bộ lọc cũ không thay kết quả hiện tại');

    caption('Bước 10: MOCK lọc Đang triển khai trả rỗng: thông báo tiếng Việt kèm nhãn trạng thái, không hiện enum');
    await page.route(listRoute, async (route) => {
      if (!route.request().url().includes('status=DEPLOYING')) { await route.continue(); return; }
      mocked('GET staging/deployments?status=DEPLOYING answered with fake empty list');
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ deployments: [] }) });
    });
    await h.choose(filter(), 'Đang triển khai');
    await expect(page.getByText('Chưa có triển khai nào ở trạng thái “Đang triển khai”.')).toBeVisible();
    expect(listGets.at(-1)).toMatch(/staging\/deployments\?status=DEPLOYING$/);
    expect(await page.locator('body').innerText()).not.toMatch(/DEPLOYING/);
    await page.unroute(listRoute);
    check('filtered empty (MOCKED): Vietnamese message with selected label; query used original enum DEPLOYING');
    mark('filter-empty');
    await h.read('Rỗng khi lọc: dùng nhãn tiếng Việt');
    await h.choose(filter(), 'Tất cả');
    await expect(entries()).toHaveCount(2);

    caption('Bước 11: Chuyển sang Production rồi Xem tất cả: lịch sử thật của Production trống, không lẫn deployment của Staging');
    await h.click(page.getByRole('button', { name: `← ${NAME}` }));
    await expect(page.getByRole('heading', { name: NAME })).toBeVisible();
    await openHistory(/^Production/i);
    await expect(page).toHaveURL(`${baseURL}/ui/applications/${appId}/environments/production/deployments`);
    await expect(page.getByText('Chưa có triển khai nào trong môi trường này.')).toBeVisible();
    await expect(entries()).toHaveCount(0);
    expect(listGets.some((line) => line.startsWith('production/deployments'))).toBe(true);
    expect(await apiList('production')).toEqual([]);
    check('scope (real): Production history empty while Staging has 2 deployments; production list request issued');
    mark('production-empty');
    await h.read('Production trống; không lẫn lịch sử của Staging');

    caption('Bước 12: Xác minh chỉ đọc: sau khi triển khai, xem lịch sử, lọc, tải lại, mở chi tiết không gửi request ghi nào; đăng xuất');
    expect(writes.slice(afterDeploy)).toEqual([]);
    expect(await apiList('staging')).toHaveLength(2);
    check(`read-only (real): zero non-GET requests after the product Deploy steps; deployments still 2; mocks used (${mocks.length} activations, see mocks.json)`);
    mark('read-only');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T15: Chi tiết triển khai. Developer. PRODUCT (UI, backend fake thật với fake deliverer, state tạm): đăng nhập, tạo ứng dụng, thêm 2 draft Workload, Xem trước và
  // Triển khai thật (2 Deployment SUCCEEDED), mở lịch sử rồi chi tiết thật: nhãn tiếng Việt, ID/digest/thời gian vi-VN, plan thật (có hoặc không, theo API), 404 thật khi
  // mở ID của staging dưới production. MOCK (page.route trên GET .../deployments/<id>, mỗi lần dùng ghi vào mocks.json, backend KHÔNG thay đổi): (1) snapshot thành công
  // giả có graph/matches/batches, output nested + marker che + resolvedInputs giả; (2) FAILED không plan kèm lý do thô giả; (3) trạng thái đang chạy giả; (4) 404 giả;
  // (5) 500 giả kèm chuỗi bí mật giả rồi Thử lại đến backend thật; (6) phản hồi chi tiết giữ bằng cổng điều khiển, đổi sang deployment khác, rồi nhả phản hồi muộn.
  // Mock chỉ là chiếu giao diện, không chứng minh adapter thật. Sau khi Triển khai, mọi request không phải GET bị cấm.
  async T15({ page, h, ui, caption, mark, check, baseURL }) {
    const NAME = 'T15 Chi tiết triển khai';
    const RAW = 'provider said token=ABC-SECRET-777';
    const NESTED = 'NESTED-SECRET-555';
    const RESOLVED = 'RESOLVED-INPUT-999';
    const mocks = [];
    const mocked = (what) => { mocks.push(what); appendFileSync(`${process.env.ORCH_E2E_EVIDENCE_DIR}/mocks.json`, `${JSON.stringify({ at: new Date().toISOString(), what })}\n`); };
    const writes = [];
    const detailGets = [];
    page.on('request', (request) => {
      const url = new URL(request.url());
      if (request.method() !== 'GET') { writes.push(`${request.method()} ${url.pathname}`); return; }
      if (/\/deployments\/[^/]+$/.test(url.pathname)) detailGets.push(`${url.pathname.split('/environments/')[1]}`);
    });
    let appId = '';
    const spec = (name) => ({ name, image: `registry.example/${name}:t15`, bindings: [], port: { name: 'http', port: '8080', targetPort: '8080' } });
    const apiList = async (environment) => (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/${environment}/deployments`)).json()).deployments ?? [];
    const apiDetail = async (environment, id) => (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/${environment}/deployments/${encodeURIComponent(id)}`)).json();
    const entries = () => page.locator('button.deployment-entry');
    const body = async () => page.locator('body').innerText();
    const routeFor = (id) => new RegExp(`/environments/staging/deployments/${id}$`);
    const noLeak = async () => expect(await body()).not.toMatch(/ABC-SECRET|token=|provider said|NESTED-SECRET|RESOLVED-INPUT|\*\*\*redacted\*\*\*/);
    const noEnglish = async () => expect(await body()).not.toMatch(/Deployment details|deployment history|Loading|Refresh|Retry|Started|Finished|Triggered|Provision plan|Resources\b|Workloads\b|not found|graph nodes|Could not load/);
    const heading = () => page.getByRole('heading', { name: 'Chi tiết triển khai', level: 1 });
    const fulfill = (route, payload, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(payload) });
    let apiId = ''; let webId = '';
    const deployment = (id, overrides = {}) => ({ id, applicationKey: appId, environmentKey: 'staging', action: 'DEPLOY', workloadId: 'api', actorRef: 'developer', status: 'SUCCEEDED', startedAt: '2026-09-01T10:00:00Z', finishedAt: '2026-09-01T10:02:00Z', ...overrides });
    const openDetail = async (text) => {
      await h.click(entries().filter({ hasText: text }));
      await expect(heading()).toBeVisible();
    };

    caption('Bước 1: Lập trình viên đăng nhập và tạo ứng dụng mới');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    appId = await createApplication(h, NAME, 't15-chi-tiet-trien-khai');
    check('new application (real backend): both Environments connected through the UI');
    mark('home-created');

    caption('Bước 2: Thêm hai Workload api và web rồi Xem trước và Triển khai thật (fake deliverer)');
    await addWorkload(h, spec('api'));
    await previewAndDeploy(h, ['api']);
    await addWorkload(h, spec('web'));
    await previewAndDeploy(h, ['web']);
    const afterDeploy = writes.length;
    const real = await apiList('staging');
    expect(real.map((item) => `${item.workloadId}/${item.action}/${item.status}`)).toEqual(['web/DEPLOY/SUCCEEDED', 'api/DEPLOY/SUCCEEDED']);
    webId = real[0].id; apiId = real[1].id;
    check('two real Deployments created through the UI (fake deliverer): web newest, then api, both DEPLOY/SUCCEEDED');
    mark('deployed');

    caption('Bước 3: Mở Xem tất cả rồi mở chi tiết triển khai api thật: nhãn tiếng Việt, ID và thời gian vi-VN, không còn tiếng Anh');
    await h.click(page.getByRole('button', { name: /^Staging/i }));
    await h.click(page.getByRole('button', { name: 'Xem tất cả' }));
    await expect(entries()).toHaveCount(2);
    await openDetail('api · triển khai');
    await expect(page).toHaveURL(`${baseURL}/ui/applications/${appId}/environments/staging/deployments/${encodeURIComponent(apiId)}`);
    const realDetail = await apiDetail('staging', apiId);
    const started = await page.evaluate((iso) => new Date(iso).toLocaleString('vi-VN'), real[1].startedAt);
    await expect(page.getByRole('heading', { name: 'api · triển khai' })).toBeVisible();
    await expect(page.getByText(apiId, { exact: true })).toBeVisible();
    await expect(page.locator('.status', { hasText: 'Thành công' })).toBeVisible();
    await expect(page.getByRole('button', { name: '← Lịch sử triển khai staging' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Tải lại' })).toBeVisible();
    await expect(page.getByText(`${NAME} · staging`)).toBeVisible();
    await expect(page.locator('dd', { hasText: started }).first()).toBeVisible();
    await expect(page.locator('dt', { hasText: 'Người thực hiện' })).toBeVisible();
    await expect(page.locator('dd', { hasText: real[1].actorRef }).first()).toBeVisible();
    for (const item of realDetail.workloads ?? []) await expect(page.getByText(item.manifestDigest, { exact: true })).toBeVisible();
    await noEnglish();
    expect(await body()).not.toMatch(/SUCCEEDED|\bDEPLOY\b|Rollback|Hoàn tác/);
    const planned = Boolean(realDetail.planHash);
    if (planned) await expect(page.getByRole('heading', { name: 'Kế hoạch tạo tài nguyên' })).toBeVisible();
    else await expect(page.getByText('Không có kế hoạch tạo tài nguyên được lưu cho triển khai này.')).toBeVisible();
    check(`real detail: Vietnamese title/labels/badge, deployment ID ${apiId}, vi-VN time "${started}", actor and ${realDetail.workloads?.length ?? 0} workload digest(s) from API; plan saved=${planned}; no English leftovers or raw enums`);
    mark('detail-real');
    await h.moveTo(page.getByRole('heading', { name: 'api · triển khai' }));
    await h.read('Chi tiết thật bằng tiếng Việt; ID và digest giữ nguyên');

    caption('Bước 4: MOCK snapshot thành công giả: graph consumer → provider, Definition, batches, output nested và marker che; Workload lịch sử khác trạng thái hiện tại');
    const successMock = {
      deployment: deployment(apiId), deploymentSetId: 'set-snapshot-1', planHash: 'abcdef0123456789deadbeef',
      graph: { nodes: [{ descriptor: 'db', kind: 'resource' }, { descriptor: 'app/web', kind: 'workload' }], edges: [{ consumer: 'app/web', provider: 'db' }] },
      matches: { db: { definitionKey: 'pg-kind', driverType: 'fake' } }, batches: [['db'], ['app/web', 'cache']],
      resources: [{ descriptor: 'db', resourceType: 'postgres', definitionKey: 'pg-kind', status: 'READY', outputs: { host: 'db.internal', port: 5432, password: '***redacted***', nested: { token: NESTED } }, resolvedInputs: { password: RESOLVED } }, { descriptor: 'old-cache', resourceType: 'redis', definitionKey: 'redis-kind', status: 'UNREFERENCED', outputs: {} }],
      workloads: [{ workloadId: 'api', status: 'RUNNING', manifestDigest: 'sha256:historical-api-digest', lastDeploymentId: apiId, observedAt: '2026-09-01T10:01:00Z' }],
    };
    await page.route(routeFor(apiId), async (route) => { mocked('GET staging deployment detail answered with fake success snapshot (graph, matches, batches, redacted + nested outputs, fake resolvedInputs)'); await fulfill(route, successMock); });
    await h.click(page.getByRole('button', { name: 'Tải lại' }));
    await expect(page.getByText('set-snapshot-1')).toBeVisible();
    await expect(page.getByText('Tập triển khai', { exact: true })).toBeVisible();
    await expect(page.getByText('Deployment Set', { exact: true })).toHaveCount(0);
    await expect(page.getByText('sha256:historical-api-digest')).toBeVisible();
    await expect(page.getByText('Đang chạy', { exact: true })).toBeVisible();
    await expect(page.getByText('Sẵn sàng · pg-kind')).toBeVisible();
    await expect(page.getByText('Không còn được tham chiếu · redis-kind')).toBeVisible();
    await expect(page.getByText('Đầu ra: host=db.internal, port=5432, password=Đã che, nested=[giá trị đã ẩn]')).toBeVisible();
    await expect(page.getByText('2 nút đồ thị · 1 phụ thuộc · kế hoạch abcdef012345')).toBeVisible();
    await expect(page.getByText('db · resource · pg-kind')).toBeVisible();
    await expect(page.getByText('app/web → db')).toBeVisible();
    expect(await page.locator('ol > li').allInnerTexts()).toEqual(['db', 'app/web, cache']);
    await noLeak();
    await noEnglish();
    check('success projection (MOCKED detail): graph direction app/web → db, matched Definition, batch order [db],[app/web, cache], historical digest/IDs unchanged, secret marker shown as "Đã che", nested value and resolvedInputs never rendered');
    mark('mock-success');
    await h.moveTo(page.getByText('password=Đã che', { exact: false }));
    await h.read('Output bí mật hiện là Đã che; giá trị lồng nhau và resolved inputs không bao giờ hiện (dữ liệu giả)');
    await page.unroute(routeFor(apiId));

    caption('Bước 5: MOCK FAILED không có plan kèm lý do thô giả: chỉ hiện hướng dẫn tiếng Việt an toàn, không dựng graph giả');
    const failedMock = { deployment: deployment(apiId, { status: 'FAILED', failureReason: RAW, finishedAt: undefined }), deploymentSetId: '', planHash: '', graph: null, matches: null, batches: null, resources: [], workloads: [] };
    await page.route(routeFor(apiId), async (route) => { mocked('GET staging deployment detail answered with fake FAILED detail without plan and with secret-like failureReason'); await fulfill(route, failedMock); });
    await h.click(page.getByRole('button', { name: 'Tải lại' }));
    await expect(page.getByRole('alert')).toContainText('Triển khai không hoàn tất.');
    await expect(page.getByRole('alert')).toContainText('Xem trước các thay đổi rồi triển khai lại');
    await expect(page.getByRole('alert')).not.toContainText('Definition');
    await expect(page.locator('.status', { hasText: 'Thất bại' })).toBeVisible();
    await expect(page.getByText('Không có kế hoạch tạo tài nguyên được lưu cho triển khai này.')).toBeVisible();
    await expect(page.getByText('Chưa ghi nhận trạng thái Workload nào.')).toBeVisible();
    await expect(page.getByText('Chưa ghi nhận trạng thái tài nguyên nào.')).toBeVisible();
    await expect(page.getByText(/nút đồ thị/)).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'Các đợt tạo tài nguyên' })).toHaveCount(0);
    await noLeak();
    check('failed projection (MOCKED detail): "Thất bại" badge, safe Vietnamese guidance without raw failureReason, no plan/graph/batches, Vietnamese empty states');
    mark('mock-failed');
    await h.moveTo(page.getByRole('alert'));
    await h.read('Thất bại: chỉ hướng dẫn an toàn, không có plan giả (dữ liệu giả)');
    await page.unroute(routeFor(apiId));

    caption('Bước 6: MOCK các trạng thái đang chạy PLANNING, PROVISIONING, DEPLOYING: nhãn tiếng Việt, không lộ enum');
    let phase = 'PLANNING';
    await page.route(routeFor(apiId), async (route) => { mocked(`GET staging deployment detail answered with fake ${phase} detail`); await fulfill(route, { ...failedMock, deployment: deployment(apiId, { status: phase, failureReason: undefined, finishedAt: undefined }) }); });
    for (const [status, label] of [['PLANNING', 'Đang lập kế hoạch'], ['PROVISIONING', 'Đang tạo tài nguyên'], ['DEPLOYING', 'Đang triển khai']]) {
      phase = status;
      await h.click(page.getByRole('button', { name: 'Tải lại' }));
      await expect(page.locator('.status', { hasText: label })).toBeVisible();
      await expect(page.getByRole('alert')).toHaveCount(0);
      expect(await body()).not.toContain(status);
      await h.read(`${label} (dữ liệu giả)`);
    }
    await page.unroute(routeFor(apiId));
    check('in-progress projections (MOCKED detail): PLANNING/PROVISIONING/DEPLOYING render Vietnamese badges, no raw enum, no failure alert');
    mark('mock-running');

    caption('Bước 7: Không tìm thấy: MOCK 404 giả, rồi 404 thật khi mở deployment ID của staging dưới production; không có nút Thử lại');
    await page.route(routeFor(apiId), async (route) => { mocked('GET staging deployment detail answered with fake 404'); await fulfill(route, { error: 'tenant-other row hidden' }, 404); });
    await h.click(page.getByRole('button', { name: 'Tải lại' }));
    await expect(page.getByRole('heading', { name: 'Không tìm thấy triển khai' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Thử lại' })).toHaveCount(0);
    await expect(page.getByRole('alert')).toHaveCount(0);
    expect(await body()).not.toContain('tenant-other');
    await page.unroute(routeFor(apiId));
    mark('mock-404');
    await h.read('Không tìm thấy trong phạm vi đã chọn; không tiết lộ chi tiết (404 giả)');
    await page.goto(`${baseURL}/ui/applications/${appId}/environments/production/deployments/${encodeURIComponent(apiId)}`);
    await expect(page.getByRole('heading', { name: 'Không tìm thấy triển khai' })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Thử lại' })).toHaveCount(0);
    await expect(page.getByText('Triển khai này không tồn tại trong ứng dụng và môi trường đã chọn.')).toBeVisible();
    expect(detailGets.at(-1)).toBe(`production/deployments/${apiId}`);
    check('not found: MOCKED 404 and REAL 404 (staging deployment ID opened under production) show Vietnamese heading/explanation, no Thử lại, no leaked backend text');
    mark('real-404');
    await h.read('404 thật khi mở ID của staging dưới production');

    caption('Bước 8: MOCK 500 giả kèm chuỗi bí mật giả: thông báo an toàn, không dữ liệu cũ; Thử lại đến backend thật');
    await page.goto(`${baseURL}/ui/applications/${appId}/environments/staging/deployments/${encodeURIComponent(webId)}`);
    await expect(heading()).toBeVisible();
    let failOnce = true;
    await page.route(routeFor(webId), async (route) => {
      if (failOnce) { failOnce = false; mocked('GET staging deployment detail answered with fake 500 once (secret-like raw text must not render)'); await fulfill(route, { error: RAW }, 500); return; }
      await route.continue();
    });
    await h.click(page.getByRole('button', { name: 'Tải lại' }));
    await expect(page.getByRole('alert')).toContainText('Không thể tải chi tiết triển khai.');
    await expect(page.getByRole('heading', { name: 'web · triển khai' })).toHaveCount(0);
    await noLeak();
    mark('mock-500');
    await h.moveTo(page.getByRole('alert'));
    await h.read('Lỗi an toàn bằng tiếng Việt, không lộ nội dung thô (500 giả)');
    const before = detailGets.length;
    await h.click(page.getByRole('alert').getByRole('button', { name: 'Thử lại' }));
    await expect(page.getByRole('heading', { name: 'web · triển khai' })).toBeVisible();
    await expect(page.getByRole('alert')).toHaveCount(0);
    expect(detailGets.slice(before)).toEqual([`staging/deployments/${webId}`]);
    await page.unroute(routeFor(webId));
    check('error (MOCKED 500 once): Vietnamese safe alert with Thử lại, stale detail cleared, no raw text; retry issued one explicit GET to the real backend and showed the real detail');
    mark('retry-ok');
    await h.read('Thử lại thành công với dữ liệu thật');

    caption('Bước 9: MOCK giữ phản hồi chi tiết api bằng cổng điều khiển, chuyển sang deployment web, tải xong rồi mới nhả phản hồi muộn');
    let releaseLate = () => undefined;
    const lateGate = new Promise((resolve) => { releaseLate = resolve; });
    let lateFulfilled = false;
    await page.route(routeFor(apiId), async (route) => {
      mocked('GET staging deployment detail of the previous ID held on an explicit gate (released only after the other deployment loaded), then answered with a fake late snapshot');
      await lateGate;
      await fulfill(route, { ...successMock, deployment: deployment(apiId, { workloadId: 'late-stale-workload' }) });
      lateFulfilled = true;
    });
    await h.click(page.getByRole('button', { name: '← Lịch sử triển khai staging' }));
    await expect(entries()).toHaveCount(2);
    await h.click(entries().filter({ hasText: 'api · triển khai' }));
    await expect(page.getByRole('status')).toHaveAttribute('aria-busy', 'true');
    await expect(page.getByRole('status')).toContainText('Đang tải chi tiết triển khai…');
    await expect(page.getByRole('heading', { name: /triển khai$/ }).filter({ hasText: 'web' })).toHaveCount(0);
    mark('loading');
    await h.read('Đang tải: dữ liệu cũ đã bị xóa, trạng thái có aria-busy');
    await page.goBack();
    await expect(entries()).toHaveCount(2);
    await h.click(entries().filter({ hasText: 'web · triển khai' }));
    await expect(page.getByRole('heading', { name: 'web · triển khai' })).toBeVisible();
    expect(lateFulfilled).toBe(false);
    const lateResponse = page.waitForResponse((response) => response.url().endsWith(`/deployments/${apiId}`));
    releaseLate();
    await (await lateResponse).finished();
    await expect.poll(() => lateFulfilled).toBe(true);
    // Two animation frames: the page has run the settled fetch continuation before the no-stale assertions.
    await page.evaluate('new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
    await expect(page.getByText('late-stale-workload')).toHaveCount(0);
    await expect(page.getByRole('heading', { name: 'web · triển khai' })).toBeVisible();
    await expect(page.getByText(webId, { exact: true })).toBeVisible();
    await expect(page.getByRole('status')).toHaveCount(0);
    await page.unroute(routeFor(apiId));
    check('late reply (MOCKED gate): api detail held, user moved to web; reply released only after web loaded and completed, then ignored (still web, no late-stale-workload)');
    mark('late-detail');
    await h.read('Phản hồi muộn của deployment cũ không thay kết quả hiện tại');

    caption('Bước 10: Xác minh chỉ đọc: không có rollback hay trạng thái live, mọi request sau Triển khai đều là GET; đăng xuất');
    expect(writes.slice(afterDeploy)).toEqual([]);
    expect(await body()).not.toMatch(/Rollback|Hoàn tác|Khôi phục/);
    expect(await apiList('staging')).toHaveLength(2);
    check(`read-only (real): zero non-GET requests after the product Deploy steps; deployments still 2; mocks used (${mocks.length} activations, see mocks.json); mocks are UI projections only, not proof of a real adapter`);
    mark('read-only');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T16: VPC/EKS luôn dùng Kết nối AWS của Môi trường. Fake adapters, state JSON tạm.
  // SETUP (UI, Nhà phát triển): tạo ứng dụng và chọn Kết nối AWS aws-account cho Staging.
  // SETUP/ASSERTION (API, không phải thao tác sản phẩm): POST resource-definitions với
  // connectionKey khác rỗng để chứng minh API từ chối 400; chỉ đọc danh sách để xác minh.
  async T16({ page, h, ui, caption, mark, check, baseURL }) {
    const NAME = 'T16 Kết nối Môi trường';
    const apiDefinitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const apiDefinition = async (key) => (await apiDefinitions()).find((item) => item.key === key);
    const posts = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/resource-definitions') posts.push(request.postDataJSON());
    });
    const registered = (key) => expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${key}.` })).toBeVisible();
    const editor = page.locator('.criteria-editor');
    const select = (text) => editor.locator('label').filter({ hasText: text }).locator('select');
    let appId = '';
    // Same module inputs as the seeded Definitions; literals match the declared types.
    const eksVars = JSON.stringify({ name: '${context.infra.resourceName}', subnet_ids: "${resources['vpc.default#@infra'].outputs.subnetIds}", kubernetes_version: '1.34', node_instance_type: 't4g.small', node_capacity_type: 'SPOT', node_ami_type: 'AL2023_ARM_64_STANDARD', node_disk_size: 20 });
    const auroraVars = JSON.stringify({ name: '${context.infra.resourceName}', engine_version: '16.14', min_capacity: 0, max_capacity: 1, vpc_id: "${resources['vpc.default#@infra'].outputs.id}", vpc_cidr: "${resources['vpc.default#@infra'].outputs.cidr}", subnet_ids: "${resources['vpc.default#@infra'].outputs.subnetIds}" });

    caption('SETUP (qua giao diện): Nhà phát triển đăng nhập, tạo ứng dụng và chọn Kết nối AWS aws-account cho Staging');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    appId = await createApplication(h, NAME, 't16-ket-noi-moi-truong', { connections: { staging: 'aws-account' } });
    await expect(page.getByText('aws-account').first()).toBeVisible();
    check('setup via UI: application created and Staging bound to the fake AWS connection aws-account (not product steps of T16)');
    mark('setup-app');
    await h.read('Thiết lập: ứng dụng Staging dùng Kết nối AWS aws-account');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();

    caption('Bước 1: Kỹ sư nền tảng đăng nhập và mở Cấu hình tài nguyên');
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.click(ui('shell', 'navResourceDefinitions'));
    await expect(ui('resourceDefinitions', 'heading')).toBeVisible();
    mark('definitions-page');
    await h.pause(SHORT_READ);

    caption('Bước 2: Chọn vpc: không có ô Kết nối AWS, kể cả Nâng cao; trợ giúp nói rõ dùng Kết nối của Môi trường');
    await h.type(ui('resourceDefinitions', 'id'), 'vpc-t16');
    await h.choose(ui('resourceDefinitions', 'type'), 'vpc');
    await expect(ui('resourceDefinitions', 'driver')).toHaveValue('terraform');
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveCount(0);
    const note = page.getByRole('note').filter({ hasText: 'dùng Kết nối AWS của Môi trường' });
    await expect(note).toContainText('Xem trước hoặc Triển khai');
    await h.click(ui('resourceDefinitions', 'showAdvanced'));
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveCount(0);
    check('vpc: no AWS account control (also under Nâng cao); Vietnamese note explains Environment inheritance');
    mark('vpc-no-account');
    await h.moveTo(note);
    await h.read('VPC dùng Kết nối AWS của Môi trường; không chọn tài khoản ở đây');

    caption('Bước 3: Đặt điều kiện Theo ứng dụng cho ứng dụng thử rồi đăng ký vpc không có tài khoản');
    await h.click(editor.getByRole('radio', { name: /Theo ứng dụng/ }));
    await h.choose(select(/^Ứng dụng(?! mẫu)/), `${NAME} (${appId})`);
    await h.type(ui('resourceDefinitions', 'variables'), '{"name":"${context.infra.resourceName}","cidr":"10.30.0.0/16"}', { replace: true, delay: 30 });
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('vpc-t16');
    expect(posts.at(-1)).toMatchObject({ key: 'vpc-t16', resourceType: 'vpc', driverType: 'terraform', executionProfile: 'aws-eks', connectionKey: '' });
    const vpc = await apiDefinition('vpc-t16');
    expect(vpc.connectionKey ?? '').toBe('');
    expect(vpc.criteria).toEqual([{ app_id: appId }]);
    check('registered vpc-t16 through UI: POST payload and stored Definition have empty connectionKey');
    mark('vpc-registered');
    await h.pause(SHORT_READ);

    caption('Bước 4: Đăng ký k8s-cluster (module eks) cũng không có tài khoản');
    await h.type(ui('resourceDefinitions', 'id'), 'eks-t16');
    await h.choose(ui('resourceDefinitions', 'type'), 'k8s-cluster');
    await expect(page.locator('code', { hasText: /^eks$/ })).toBeVisible();
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveCount(0);
    // The seeded EKS Definition matches Class eks; app + Class eks is strictly more specific.
    await h.click(editor.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(editor.getByLabel('Điều kiện 1 ID ứng dụng'), appId);
    await h.type(editor.getByLabel('Điều kiện 1 Class'), 'eks');
    await h.type(ui('resourceDefinitions', 'variables'), eksVars, { replace: true, delay: 5 });
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('eks-t16');
    expect(posts.at(-1)).toMatchObject({ key: 'eks-t16', resourceType: 'k8s-cluster', driverType: 'terraform', connectionKey: '' });
    expect((await apiDefinition('eks-t16')).connectionKey ?? '').toBe('');
    expect((await apiDefinition('eks-t16')).criteria).toEqual([{ app_id: appId, class: 'eks' }]);
    check('registered eks-t16 through UI: empty connectionKey');
    mark('eks-registered');
    await h.pause(SHORT_READ);

    caption('Bước 5: Aurora (postgres + Terraform) vẫn bắt buộc chọn Kết nối AWS riêng; thiếu thì bị chặn cục bộ');
    await h.type(ui('resourceDefinitions', 'id'), 'aurora-t16');
    await h.choose(ui('resourceDefinitions', 'type'), 'postgres');
    await h.choose(ui('resourceDefinitions', 'driver'), 'Tạo trên AWS (Terraform)');
    await expect(ui('resourceDefinitions', 'awsConnection')).toBeVisible();
    await h.click(editor.getByRole('radio', { name: /Theo ứng dụng/ }));
    await h.choose(select(/^Ứng dụng(?! mẫu)/), `${NAME} (${appId})`);
    await h.type(ui('resourceDefinitions', 'variables'), auroraVars, { replace: true, delay: 5 });
    const postsBefore = posts.length;
    await h.click(ui('resourceDefinitions', 'submit'));
    await expect(page.locator('form .form-error[role="alert"]')).toContainText('Chọn một Kết nối AWS sẵn sàng');
    expect(posts.length).toBe(postsBefore);
    check('Aurora without account: Vietnamese local error, no POST');
    mark('aurora-required');
    await h.moveTo(page.locator('form .form-error[role="alert"]'));
    await h.read('Aurora cần Kết nối AWS riêng');

    caption('Bước 6: Chọn aws-account cho Aurora, đổi sang vpc rồi quay lại: lựa chọn Aurora được giữ nhưng không gửi cho VPC/EKS');
    await h.choose(ui('resourceDefinitions', 'awsConnection'), 'aws-account');
    await h.choose(ui('resourceDefinitions', 'type'), 'vpc');
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveCount(0);
    await h.choose(ui('resourceDefinitions', 'type'), 'postgres');
    await expect(ui('resourceDefinitions', 'driver')).toHaveValue('terraform');
    await expect(ui('resourceDefinitions', 'awsConnection')).toHaveValue('aws-account');
    check('type switch vpc <-> postgres keeps the Aurora account selection; VPC shows no selector');
    mark('aurora-selection-kept');
    await h.read('Lựa chọn Aurora được giữ khi đổi loại');

    caption('Bước 7: Đăng ký Aurora với Kết nối AWS riêng; sau đó đăng ký lại vpc: payload vẫn rỗng dù vừa chọn tài khoản');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('aurora-t16');
    expect(posts.at(-1)).toMatchObject({ key: 'aurora-t16', resourceType: 'postgres', driverType: 'terraform', connectionKey: 'aws-account' });
    expect((await apiDefinition('aurora-t16')).connectionKey).toBe('aws-account');
    expect((await apiDefinition('vpc-t16')).connectionKey ?? '').toBe('');
    await h.type(ui('resourceDefinitions', 'id'), 'vpc-t16-after-aurora');
    await h.choose(ui('resourceDefinitions', 'type'), 'vpc');
    await h.click(editor.getByRole('radio', { name: /Theo ứng dụng/ }));
    await h.choose(select(/^Ứng dụng(?! mẫu)/), `${NAME} (${appId})`);
    // Limited to Production so it never ties with vpc-t16 on the Staging Preview below.
    const productionLabel = (await select(/^Môi trường \(tùy chọn\)/).locator('option').allTextContents()).find((text) => text.includes('(production)'));
    expect(productionLabel, 'production environment option').toBeTruthy();
    await h.choose(select(/^Môi trường \(tùy chọn\)/), productionLabel);
    await h.type(ui('resourceDefinitions', 'variables'), '{"name":"${context.infra.resourceName}","cidr":"10.31.0.0/16"}', { replace: true, delay: 30 });
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('vpc-t16-after-aurora');
    expect(posts.at(-1)).toMatchObject({ key: 'vpc-t16-after-aurora', connectionKey: '' });
    check('registered aurora-t16 with its own account aws-account; VPC registered afterwards still sends connectionKey ""');
    mark('aurora-registered');
    await h.pause(SHORT_READ);

    caption('SETUP/ASSERTION (API, không phải thao tác sản phẩm): gửi trực tiếp connectionKey khác rỗng cho vpc/eks; API từ chối 400 và không lưu');
    const before = (await apiDefinitions()).length;
    for (const [type, module] of [['vpc', 'vpc'], ['k8s-cluster', 'eks']]) {
      for (const [label, connectionKey] of [['aws-account (bằng đích Môi trường)', 'aws-account'], ['không tồn tại', 'khong-ton-tai']]) {
        const response = await page.request.post(`${baseURL}/api/v1/resource-definitions`, { data: {
          key: `api-reject-${module}-${connectionKey}`, resourceType: type, executionProfile: 'aws-eks', driverType: 'terraform', connectionKey,
          driverInputs: { values: { variables: {}, source: { module } } }, criteria: [{}],
        } });
        expect(response.status(), `${type} with ${label}`).toBe(400);
        const text = JSON.stringify(await response.json());
        expect(text).toContain('leave connectionKey empty');
        expect(text).not.toContain(connectionKey);
      }
    }
    expect((await apiDefinitions()).length).toBe(before);
    check('API (setup/assertion): vpc and eks with nonempty keys (equal to the target, nonexistent) -> 400 safe message, nothing inserted');
    mark('api-reject');
    await h.read('API từ chối mọi connectionKey khác rỗng cho VPC/EKS');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();

    caption('Bước 8: Nhà phát triển mở Xem trước Score của Staging: đích hiển thị Kết nối aws-account');
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.click(page.getByRole('link', { name: NAME }).or(page.getByRole('button', { name: NAME })).first());
    await expect(page.getByRole('heading', { name: NAME })).toBeVisible();
    await h.click(page.getByRole('button', { name: 'Xem trước Score' }));
    await expect(page.getByRole('heading', { name: 'Xem trước Score', level: 1 })).toBeVisible();
    await expect(page.getByLabel('Đích triển khai')).toContainText('Kết nối aws-account');
    mark('preview-page');
    await h.moveTo(page.getByLabel('Đích triển khai'));
    await h.read('Đích triển khai của Staging là Kết nối AWS aws-account');

    caption('Bước 9: Xem trước workload dùng postgres: VPC/EKS đã đăng ký khớp mà không có tài khoản riêng; Aurora dùng cấu hình đã đăng ký');
    const score = 'apiVersion: score.dev/v1b1\nmetadata:\n  name: api\ncontainers:\n  main:\n    image: registry.example/api:t16\nresources:\n  db:\n    type: postgres\n    params:\n      database: acceptance\n      username: app\n';
    await h.type(page.getByLabel('ID Workload'), 'api', { replace: true });
    await h.type(page.getByLabel('ID lần chạy'), 'run-t16', { replace: true });
    await h.type(page.getByLabel('Score sau (YAML hoặc JSON)'), score, { delay: LONG_TYPE_DELAY, replace: true });
    const previewResponse = page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/score-preview'));
    await h.click(page.getByRole('button', { name: 'Xem trước', exact: true }));
    const previewReply = await previewResponse;
    expect(previewReply.status(), 'score-preview status').toBe(200);
    const previews = [await previewReply.json()];
    const result = page.getByRole('region', { name: 'Kết quả xem trước Score' });
    await expect(result).toBeVisible();
    const links = result.locator('section.content-panel').filter({ has: page.getByRole('heading', { name: 'Liên kết thực thi tài nguyên', exact: true }) });
    await expect(links).toContainText('vpc-t16');
    await expect(links).toContainText('eks-t16');
    await expect(links).toContainText('aurora-t16');
    const keys = Object.fromEntries(previews[0].matches.map((match) => [match.definitionKey, match.driverType]));
    expect(keys).toMatchObject({ 'vpc-t16': 'terraform', 'eks-t16': 'terraform', 'aurora-t16': 'terraform' });
    expect(JSON.stringify(previews[0])).not.toMatch(/secret|password/i);
    check('Preview (real backend, fake adapters): winning matches are UI-registered vpc-t16, eks-t16 (no account) and aurora-t16 on Staging bound to aws-account; vpc-t16-after-aurora (Production only) does not tie. NOTE: aws-account is both the Environment target and Aurora\'s explicit key in this run; distinct accounts are covered by backend tests only');
    mark('preview-result');
    await h.moveTo(links);
    await h.read('Xem trước chọn đúng cấu hình VPC, EKS và Aurora đã đăng ký');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T18: bộ chọn bundle dựng ứng dụng từ GET /api/v1/render-bundles. Fake adapters + the T03 score-k8s stub
  // (wrapper passes -score-k8s), so the REAL backend lists exactly one installed bundle and registers/plans for real.
  // MOCKS (page.route, projections of GET /render-bundles only; every POST/Preview stays real): 'empty' (0 bundle),
  // 'error' (list failure), 'hold'/'many' (the real bundle + one extra entry the server does not have, so the list has n
  // entries and the user can select an unavailable one). The stub is not the real score-k8s: Preview only proves selection.
  async T18({ page, h, ui, caption, mark, check, baseURL }) {
    const log = process.env.ORCH_E2E_SCORE_K8S_LOG;
    if (!log) throw new Error('ORCH_E2E_SCORE_K8S_LOG is required for T18 (set by refactor-ui-local.sh)');
    const calls = () => (existsSync(log) ? readFileSync(log, 'utf8').split('\n').filter(Boolean).length : 0);
    const NAME = 'T18 Mẫu dựng ứng dụng';
    const TEMPLATE = 'tpl-t18';
    const EXTRA = { id: 'score-k8s-bundle-gone', version: '0.0.1', status: 'AVAILABLE' };
    const json = (status, body) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });
    const mock = { mode: 'real', gate: Promise.resolve(), release: () => {}, listCalls: 0 };
    const posts = [];
    page.on('request', (request) => {
      if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1/resource-definitions') posts.push(request.postDataJSON());
    });
    await page.route('**/api/v1/render-bundles', async (route) => {
      mock.listCalls += 1;
      if (mock.mode === 'real') { await route.continue(); return; }
      if (mock.mode === 'empty') { await route.fulfill(json(200, { renderBundles: [] })); return; }
      if (mock.mode === 'error') { await route.fulfill(json(500, { error: 'sentinel-internal-detail /srv/bundles' })); return; }
      const response = await route.fetch();
      const data = await response.json();
      if (mock.mode === 'hold') await mock.gate;
      await route.fulfill({ response, json: { renderBundles: [...data.renderBundles, EXTRA] } });
    });
    const definitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const editor = page.locator('.criteria-editor');
    const select = (text) => editor.locator('label').filter({ hasText: text }).locator('select');
    const optionLabel = (locator, value) => locator.locator('option').evaluateAll((options, wanted) => options.find((option) => option.value === wanted)?.textContent, value);
    const bundle = () => ui('renderingTemplates', 'bundle');
    const submit = () => ui('renderingTemplates', 'submit');
    const alertWith = (name) => page.getByRole('alert', { name });
    const workload = { name: 'web', image: 'registry.example/web:t18', bindings: [], port: { name: 'http', port: '8080', targetPort: '8080' } };
    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await h.click(ui('signIn', 'submit'));
      await expect(ui('applications', 'heading')).toBeVisible();
    };
    const signOut = async () => {
      await h.click(ui('shell', 'signOut'));
      await expect(ui('signIn', 'username')).toBeVisible();
    };

    caption('Bước 1: Lập trình viên đăng nhập và tạo ứng dụng có workload web; API danh sách bundle từ chối vai trò này');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signInAs('developer');
    const appId = await createApplication(h, NAME, 't18-mau-dung');
    await addWorkload(h, workload);
    expect((await page.request.get(`${baseURL}/api/v1/render-bundles`)).status()).toBe(403);
    check(`developer created application ${appId} with workload web through the UI; ASSERTION (API read): GET /render-bundles as developer -> 403`);
    mark('application-prepared');
    await h.read('Ứng dụng đã sẵn sàng; Lập trình viên không đọc được danh sách bundle');
    await signOut();

    caption('Bước 2: Kỹ sư nền tảng đăng nhập; ASSERTION (API đọc): danh sách chỉ có ID, phiên bản và trạng thái AVAILABLE');
    await signInAs('platform-engineer');
    const listed = await page.request.get(`${baseURL}/api/v1/render-bundles`);
    expect(listed.status()).toBe(200);
    const listedBody = await listed.json();
    expect(Object.keys(listedBody)).toEqual(['renderBundles']);
    expect(listedBody.renderBundles).toHaveLength(1);
    const real = listedBody.renderBundles[0];
    expect(Object.keys(real).sort()).toEqual(['id', 'status', 'version']);
    expect(real.status).toBe('AVAILABLE');
    expect(JSON.stringify(listedBody)).not.toMatch(/digest|path|binary|source|token|secret/i);
    const REAL_LABEL = `${real.id} (phiên bản ${real.version})`;
    check(`API (read, real backend): one installed bundle ${real.id} ${real.version}, keys id/version/status only, no digest/path/binary/source`);
    mark('api-list');
    await h.read('API chỉ trả ID, phiên bản và trạng thái AVAILABLE của bundle đã cài');

    caption('Bước 3: Registry không có bundle (mock rỗng): form khóa đăng ký và hướng dẫn liên hệ Kỹ sư nền tảng');
    mock.mode = 'empty';
    await h.click(ui('shell', 'navRenderingTemplates'));
    await expect(ui('renderingTemplates', 'heading')).toBeVisible();
    await expect(alertWith('Chưa có bundle')).toContainText('Hãy liên hệ Kỹ sư nền tảng');
    await expect(bundle()).toBeDisabled();
    await expect(submit()).toBeDisabled();
    await h.type(ui('renderingTemplates', 'id'), TEMPLATE);
    await expect(submit()).toBeDisabled();
    expect(posts).toHaveLength(0);
    check('zero bundles (mock projection): Vietnamese guidance, selector and submit disabled, ID draft typed, no POST');
    mark('zero-bundles');
    await h.moveTo(alertWith('Chưa có bundle'));
    await h.read('Không có bundle thì không thể đăng ký mẫu');

    caption('Bước 4: Lỗi tải danh sách (mock 500): thông báo Việt an toàn, ID mẫu đã nhập được giữ, vẫn khóa đăng ký');
    mock.mode = 'error';
    await h.click(page.getByRole('button', { name: 'Làm mới danh sách bundle' }));
    await expect(alertWith('Lỗi tải bundle')).toContainText('Không tải được danh sách bundle đã cài.');
    expect(await alertWith('Lỗi tải bundle').innerText()).not.toMatch(/sentinel-internal-detail|\/srv/);
    await expect(ui('renderingTemplates', 'id')).toHaveValue(TEMPLATE);
    await expect(submit()).toBeDisabled();
    expect(posts).toHaveLength(0);
    check('list error (mock 500): safe Vietnamese message without raw detail, draft ID kept, submit disabled, no POST');
    mark('list-error');
    await h.moveTo(alertWith('Lỗi tải bundle'));
    await h.read('Lỗi tải danh sách: dữ liệu đã nhập được giữ và có nút thử lại');

    caption('Bước 5: Thử lại khi có hai bundle (đang tải khóa chọn): không tự chọn, phải chọn tường minh');
    mock.mode = 'hold';
    mock.gate = new Promise((resolve) => { mock.release = resolve; });
    await h.click(page.getByRole('button', { name: 'Thử lại tải bundle' }));
    await expect(page.getByText('Đang tải danh sách bundle đã cài…')).toBeVisible();
    await expect(bundle()).toBeDisabled();
    await expect(submit()).toBeDisabled();
    await h.read('Đang tải: đăng ký và chọn bundle bị khóa');
    mock.release();
    await expect(bundle()).toBeEnabled();
    await expect(bundle()).toHaveValue('');
    await expect(ui('renderingTemplates', 'id')).toHaveValue(TEMPLATE);
    await expect(submit()).toBeEnabled();
    check('n bundles (mock projection: real + one extra): loading locks selector/submit; after reply nothing is auto-selected; draft ID kept');
    mark('many-bundles');
    await h.moveTo(bundle());
    await h.read('Có nhiều bundle thì không tự chọn');
    await h.click(submit());
    await expect(page.getByRole('alert').filter({ hasText: 'Chọn bundle dựng ứng dụng.' })).toBeVisible();
    expect(posts).toHaveLength(0);
    check('submit without an explicit selection: Vietnamese validation, no POST');
    mark('select-required');

    caption('Bước 6: Chọn bundle mà máy chủ không còn có; POST bị từ chối, ID, bundle và điều kiện được giữ');
    await h.choose(bundle(), `${EXTRA.id} (phiên bản ${EXTRA.version})`);
    await h.click(editor.getByRole('radio', { name: /Theo ứng dụng/ }));
    await h.choose(select(/^Ứng dụng(?! mẫu)/), await optionLabel(select(/^Ứng dụng(?! mẫu)/), appId));
    await h.choose(select(/^Môi trường \(tùy chọn\)/), await optionLabel(select(/^Môi trường \(tùy chọn\)/), 'staging'));
    await h.click(submit());
    const rejected = page.getByRole('alert').filter({ hasText: 'Máy chủ từ chối mẫu này' });
    await expect(rejected).toContainText('bundle có thể không còn khả dụng');
    expect(await rejected.innerText()).not.toMatch(/unavailable|not installed|invalid|score-k8s-bundle-gone/i);
    await expect(ui('renderingTemplates', 'id')).toHaveValue(TEMPLATE);
    await expect(bundle()).toHaveValue(EXTRA.id);
    await expect(select(/^Ứng dụng(?! mẫu)/)).toHaveValue(appId);
    await expect(select(/^Môi trường \(tùy chọn\)/)).toHaveValue('staging');
    expect(posts).toHaveLength(1);
    expect(posts[0].driverInputs.values.variables).toEqual({ render_bundle: EXTRA.id });
    expect((await definitions()).some((item) => item.key === TEMPLATE)).toBe(false);
    check('POST with the unavailable bundle (real backend 400): safe message, ID/bundle/criteria kept, nothing registered');
    mark('unavailable-post');
    await h.moveTo(rejected);
    await h.read('Máy chủ kiểm tra lại bundle khi đăng ký và từ chối trước khi lưu');

    caption('Bước 7: Làm mới danh sách (backend thật, một bundle): lựa chọn cũ không còn khả dụng, không thể gửi, không bị đổi hộ');
    mock.mode = 'real';
    await h.click(page.getByRole('button', { name: 'Làm mới danh sách bundle' }));
    await expect(alertWith('Bundle không còn khả dụng')).toContainText(EXTRA.id);
    await expect(bundle()).toHaveValue(EXTRA.id);
    await expect(ui('renderingTemplates', 'id')).toHaveValue(TEMPLATE);
    await expect(page.getByRole('option', { name: `${EXTRA.id} (không còn khả dụng)` })).toBeAttached();
    await expect(submit()).toBeDisabled();
    expect(posts).toHaveLength(1);
    check('refresh to the real list: selected bundle stays as a visible option flagged unavailable; submit is disabled; no second POST; selection not replaced');
    mark('stale-selection');
    await h.moveTo(alertWith('Bundle không còn khả dụng'));
    await h.read('Lựa chọn không còn trong danh sách thì không thể gửi');

    caption('Bước 8: Chọn bundle đã cài rồi đăng ký mẫu; sau khi lưu, form mới tự chọn bundle duy nhất');
    await h.choose(bundle(), REAL_LABEL);
    await h.click(submit());
    await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký mẫu dựng ứng dụng ${TEMPLATE}.` })).toBeVisible();
    const entry = page.locator('.catalog-entry').filter({ has: page.getByText(TEMPLATE, { exact: true }) });
    await expect(entry).toContainText(`Bundle: ${real.id} · 1 điều kiện áp dụng`);
    await expect(ui('renderingTemplates', 'id')).toHaveValue('');
    await expect(bundle()).toHaveValue(real.id);
    expect(posts).toHaveLength(2);
    const stored = (await definitions()).find((item) => item.key === TEMPLATE);
    expect(stored).toMatchObject({ resourceType: 'workload', driverType: 'score-k8s', executionProfile: 'internal-k8s', criteria: [{ app_id: appId, env_id: 'staging' }] });
    expect(stored.driverInputs.values.variables).toEqual({ render_bundle: real.id });
    expect(posts[1].driverInputs.values.variables).toEqual({ render_bundle: real.id });
    check(`registered ${TEMPLATE} with render_bundle ${real.id} (payload unchanged shape); a single installed bundle is auto-selected for the next draft`);
    mark('template-registered');
    await h.moveTo(entry);
    await h.read('Đã đăng ký; form mới tự chọn bundle duy nhất đã cài');
    await signOut();

    caption('Bước 9: Lập trình viên Xem trước ứng dụng; hệ thống chọn đúng mẫu tpl-t18 (renderer fake/stub, chưa render)');
    await signInAs('developer');
    await h.click(page.getByRole('button', { name: new RegExp(NAME) }));
    await expect(page.getByRole('heading', { name: NAME })).toBeVisible();
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi' }));
    const preview = page.getByLabel('Xem trước triển khai');
    await expect(preview).toContainText('1 Workload bị ảnh hưởng', { timeout: 60_000 });
    await expect(preview.locator('li').filter({ hasText: /^web / })).toContainText(`score-k8s ${real.version} (${TEMPLATE})`);
    expect(calls(), 'Preview never runs the renderer').toBe(0);
    check(`Preview (real backend, fake adapters, stub binary): selected template ${TEMPLATE} with score-k8s ${real.version}; zero stub calls`);
    mark('preview-template');
    await h.moveTo(preview);
    await h.read('Xem trước chọn đúng mẫu đã đăng ký từ bundle đã chọn');
    await signOut();

    caption('Bước 10: Chưa đăng nhập, API danh sách bundle trả 401');
    expect((await page.request.get(`${baseURL}/api/v1/render-bundles`)).status()).toBe(401);
    check('ASSERTION (API read): GET /render-bundles without session -> 401');
    mark('signed-out');
    await h.pause(3000);
  },

  // T19: kiểm tra matching chính xác (POST /resource-definitions/matching-preview). Backend thật, fake adapters,
  // JSON state tạm. Mọi bước sản phẩm đi qua UI. MOCK duy nhất: một lần page.route trả 500 cho matching-preview để
  // chứng minh lỗi transport tách khỏi kết quả matching; mọi kết quả matching khác là phản hồi thật của backend.
  async T19({ page, h, ui, caption, mark, check, baseURL }) {
    const NAME = 'T19 Matching chính xác';
    const json = (status, body) => ({ status, contentType: 'application/json', body: JSON.stringify(body) });
    const previews = [];
    const registrations = [];
    page.on('request', (request) => {
      const path = new URL(request.url()).pathname;
      if (request.method() !== 'POST') return;
      if (path === '/api/v1/resource-definitions/matching-preview') previews.push(request.postDataJSON());
      if (path === '/api/v1/resource-definitions') registrations.push(request.postDataJSON());
    });
    const mock = { fail: false };
    await page.route('**/api/v1/resource-definitions/matching-preview', async (route) => {
      if (mock.fail) { mock.fail = false; await route.fulfill(json(500, { error: 'sentinel-internal-detail /srv/secret' })); return; }
      await route.continue();
    });
    const apiDefinitions = async () => (await (await page.request.get(`${baseURL}/api/v1/resource-definitions`)).json()).resourceDefinitions;
    const editor = page.locator('.criteria-editor');
    const panel = page.getByRole('group', { name: 'Kiểm tra matching chính xác' });
    const result = page.getByRole('region', { name: 'Kết quả matching chính xác' });
    const run = panel.getByRole('button', { name: 'Kiểm tra matching' });
    const sample = (label) => editor.locator('label').filter({ hasText: label }).locator('select, input');
    const optionLabel = (locator, value) => locator.locator('option').evaluateAll((options, wanted) => options.find((option) => option.value === wanted)?.textContent, value);
    const classRow = () => ui('resourceDefinitions', 'criterionClass');
    const registered = (key) => expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${key}.` })).toBeVisible();
    let appId = '';

    caption('SETUP (qua giao diện): Lập trình viên đăng nhập và tạo ứng dụng; Staging dùng Kết nối internal-cluster');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    appId = await createApplication(h, NAME, 't19-matching');
    check(`setup via UI: application ${appId} created; Staging bound to the fake internal-cluster Connection (not a product step of T19)`);
    mark('setup-app');
    await h.read('Thiết lập: ứng dụng và môi trường Staging để làm context mẫu');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();

    caption('Bước 1: Kỹ sư nền tảng đăng nhập, mở Cấu hình tài nguyên và nhập cấu hình postgres chưa lưu (điều kiện Mọi nơi)');
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.click(ui('shell', 'navResourceDefinitions'));
    await expect(ui('resourceDefinitions', 'heading')).toBeVisible();
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t19-wild');
    await h.choose(ui('resourceDefinitions', 'type'), 'postgres');
    await expect(ui('resourceDefinitions', 'driver')).toHaveValue('kubernetes');
    await expect(ui('resourceDefinitions', 'scope')).toHaveValue('internal-k8s');
    await expect(run).toBeDisabled();
    await expect(panel).toContainText('Chọn ứng dụng và môi trường mẫu');
    check('exact preview is disabled with a Vietnamese reason until app/env sample context is chosen');
    mark('draft-wild');
    await h.read('Chưa chọn context mẫu thì chưa thể kiểm tra matching chính xác');

    caption('Bước 2: Chọn ứng dụng, môi trường, ID tài nguyên và Class mẫu rồi bấm Kiểm tra matching');
    await h.choose(sample('Ứng dụng mẫu'), await optionLabel(sample('Ứng dụng mẫu'), appId));
    await h.choose(sample('Môi trường mẫu'), await optionLabel(sample('Môi trường mẫu'), 'staging'));
    await h.type(sample('ID tài nguyên mẫu (res_id)'), 'db');
    await h.type(sample('Class mẫu'), 'default');
    await h.click(run);
    await expect(result).toContainText('Mơ hồ: nhiều cấu hình cùng điểm cao nhất');
    await expect(result).toContainText('pg-t19-wild, postgres-internal-statefulset');
    await expect(result.getByRole('list', { name: 'Chi tiết từng cấu hình' })).toContainText(/postgres-aws-aurora: không đủ điều kiện/);
    await expect(result).toContainText('Kết quả theo snapshot; hệ thống vẫn kiểm tra lại khi đăng ký và triển khai');
    expect(previews).toHaveLength(1);
    expect(previews[0].definition).toMatchObject({ key: 'pg-t19-wild', resourceType: 'postgres', driverType: 'kubernetes', executionProfile: 'internal-k8s', criteria: [{}] });
    expect(previews[0].context).toEqual({ applicationId: appId, environmentId: 'staging', resourceId: 'db', class: 'default' });
    expect(Object.keys(previews[0].context).sort()).toEqual(['applicationId', 'class', 'environmentId', 'resourceId']);
    expect(registrations).toHaveLength(0);
    expect((await apiDefinitions()).some((item) => item.key === 'pg-t19-wild')).toBe(false);
    check('wildcard/shared vs AWS (real backend): AMBIGUOUS ties pg-t19-wild + postgres-internal-statefulset; postgres-aws-aurora ineligible by profile; snapshot warning shown; candidate NOT registered; context body has exactly 4 fields');
    mark('wildcard-ties');
    await h.moveTo(result);
    await h.read('Cấu hình wildcard hòa điểm với cấu hình dùng chung; cấu hình AWS bị loại theo phạm vi');
    await expect(editor.getByRole('group', { name: 'Cảnh báo nguy cơ chồng lấn' })).toBeVisible();
    await expect(editor.getByRole('group', { name: 'Xem trước phạm vi (trợ giúp ở giao diện)' })).toContainText('không phải kết quả matching chính xác');
    check('the frontend-only overlap hint and local scope preview remain separate, clearly labelled as not the exact result');

    caption('Bước 3: Sửa điều kiện sang Class “fast”: kết quả cũ biến mất; Class mẫu “fast” thì điểm cao hơn loại hòa thấp hơn');
    await h.click(editor.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await expect(result).toHaveCount(1); // wildcard payload [{}] is identical in both modes: the result is still valid
    await h.type(classRow(), 'fast');
    await expect(result).toHaveCount(0);
    await h.type(sample('Class mẫu'), 'fast', { replace: true });
    await expect(result).toHaveCount(0);
    check('editing criteria/mode/context drops the previous exact result without clearing the draft');
    await h.click(run);
    await expect(result).toContainText('Có một cấu hình thắng');
    await expect(result).toContainText('Cấu hình thắng: pg-t19-wild (điểm 16)');
    await expect(result).not.toContainText('Các cấu hình đang hòa');
    await expect(result.getByRole('list', { name: 'Chi tiết từng cấu hình' })).toContainText(/postgres-internal-statefulset: khớp, điểm 0/);
    check('class specific: winner pg-t19-wild score 16; the lower tie postgres-internal-statefulset (score 0) is listed but is not ambiguous');
    mark('class-winner');
    await h.moveTo(result);
    await h.read('Điểm class 16 thắng điểm 0: không còn mơ hồ');

    caption('Bước 4: Đăng ký cấu hình này qua giao diện (hành vi đăng ký không đổi), rồi nhập cấu hình thứ hai cùng điều kiện Class “fast”');
    await h.click(ui('resourceDefinitions', 'submit'));
    await registered('pg-t19-wild');
    expect(registrations).toHaveLength(1);
    expect(registrations[0]).toMatchObject({ key: 'pg-t19-wild', criteria: [{ class: 'fast' }] });
    await h.type(ui('resourceDefinitions', 'id'), 'pg-t19-twin');
    await h.choose(ui('resourceDefinitions', 'type'), 'postgres');
    await h.click(editor.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(classRow(), 'fast');
    await h.choose(sample('Ứng dụng mẫu'), await optionLabel(sample('Ứng dụng mẫu'), appId));
    await h.choose(sample('Môi trường mẫu'), await optionLabel(sample('Môi trường mẫu'), 'staging'));
    await h.type(sample('ID tài nguyên mẫu (res_id)'), 'db', { replace: true });
    await h.type(sample('Class mẫu'), 'fast', { replace: true });
    await h.click(run);
    await expect(result).toContainText('Mơ hồ: nhiều cấu hình cùng điểm cao nhất');
    await expect(result).toContainText('pg-t19-twin, pg-t19-wild');
    await expect(result).not.toContainText('postgres-internal-statefulset, ');
    check('exact tie: registered pg-t19-wild and the unsaved pg-t19-twin both score 16 -> AMBIGUOUS ties only at the winning score (shared score 0 not in ties)');
    mark('exact-tie');
    await h.moveTo(result);
    await h.read('Hai cấu hình cùng điểm cao nhất vẫn mơ hồ, trước khi lưu cấu hình thứ hai');

    caption('Bước 5: Đổi điều kiện sang Class “slow” (không giao với context “fast”): cấu hình mới không khớp, cấu hình đã đăng ký thắng');
    await h.type(classRow(), 'slow', { replace: true });
    await expect(result).toHaveCount(0);
    await h.click(run);
    await expect(result).toContainText('Cấu hình thắng: pg-t19-wild (điểm 16)');
    await expect(result.getByRole('list', { name: 'Chi tiết từng cấu hình' })).toContainText(/pg-t19-twin: đủ điều kiện nhưng không khớp context/);
    check('non-overlap: candidate class slow vs context fast is eligible but not matched; the registered pg-t19-wild wins alone');
    mark('non-overlap');
    await h.moveTo(result);
    await h.read('Điều kiện không giao nhau thì cấu hình mới không tham gia thắng/hòa');

    caption('Bước 6: Xóa ID tài nguyên mẫu: máy chủ trả “Chưa đủ context”, không kết luận hòa hay thắng');
    await h.type(sample('ID tài nguyên mẫu (res_id)'), '', { replace: true });
    await expect(result).toHaveCount(0);
    await h.click(run);
    await expect(result).toContainText('Chưa đủ context để kết luận');
    await expect(result).toContainText('Cần bổ sung context: ID tài nguyên (res_id)');
    await expect(result).not.toContainText('Cấu hình thắng:');
    await expect(result).not.toContainText('đang hòa');
    await expect(result.getByRole('list', { name: 'Chi tiết từng cấu hình' })).toContainText(/pg-t19-twin: chưa đánh giá do thiếu context/);
    await expect(result.getByRole('list', { name: 'Chi tiết từng cấu hình' })).not.toContainText('không khớp context');
    check('incomplete (real backend 200 INCOMPLETE): missing res_id reported; no winner and no ties');
    mark('incomplete');
    await h.moveTo(result);
    await h.read('Thiếu ID tài nguyên thì chưa kết luận gì');

    caption('Bước 7 (MOCK): Lỗi transport 500 hiển thị riêng, không phải kết quả matching; chỉ Thử lại thủ công mới gửi lại');
    await h.type(sample('ID tài nguyên mẫu (res_id)'), 'db');
    mock.fail = true;
    const before = previews.length;
    await h.click(run);
    const failure = page.getByRole('alert', { name: 'Lỗi kiểm tra matching' });
    await expect(failure).toContainText('Đây không phải kết quả matching');
    expect(await failure.innerText()).not.toMatch(/sentinel-internal-detail|\/srv/);
    await expect(result).toHaveCount(0);
    await expect(ui('resourceDefinitions', 'id')).toHaveValue('pg-t19-twin');
    await h.pause(1500);
    expect(previews.length).toBe(before + 1);
    check('transport 500 (MOCK via page.route): safe Vietnamese error separate from matching statuses; draft kept; no automatic retry');
    mark('transport-error');
    await h.moveTo(failure);
    await h.read('Lỗi mạng/máy chủ tách khỏi kết quả matching');
    await h.click(page.getByRole('button', { name: 'Thử lại kiểm tra matching' }));
    await expect(result).toContainText('Cấu hình thắng: pg-t19-wild (điểm 16)');
    expect(previews.length).toBe(before + 2);
    check('explicit retry sends the read-only calculation again and shows the real result');
    mark('retry-result');
    await h.read('Thử lại thủ công cho kết quả thật');

    const stored = await apiDefinitions();
    expect(stored.some((item) => item.key === 'pg-t19-twin')).toBe(false);
    expect(registrations).toHaveLength(1);
    check('no preview persisted the unsaved candidate: only pg-t19-wild was registered (by the explicit UI submit)');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T20: stable-code errors in Vietnamese. Real fake-adapter backend for auth,
  // duplicate, invalid ID, invalid variable, stale and kubeconfig errors.
  // BUSY, copy failure and an unknown code are PROJECTION MOCKS (page.route),
  // each labelled in its caption: no real operation, store or provider runs.
  async T20({ page, h, ui, caption, mark, check, baseURL }) {
    const CANARY = process.env.ORCH_E2E_CANARY || 'T20-CANARY-secret-9f3a7c';
    const MOCK_RAW = 'T20-MOCK-RAW-LEAK-pq-password';
    const NAME = 'T20 Lỗi theo mã';
    const realBodies = [];
    const mocked = new Set();
    page.on('response', async (response) => {
      if (response.status() < 400 || !response.url().includes('/api/v1/') || mocked.has(response.url())) return;
      realBodies.push({ url: response.url(), status: response.status(), body: await response.text().catch(() => '') });
    });
    const alertOf = () => page.getByRole('alert');
    // Brings the error sentence into the recorded viewport before the phase mark and the reading pause.
    const reveal = async (locator = alertOf().first()) => { await locator.evaluate((element) => element.scrollIntoView({ block: 'center', behavior: 'smooth' })); await h.pause(900); };
    const noRaw = async () => {
      const text = (await page.locator('body').innerText()).replace(/\s+/g, ' ');
      expect(text, 'page text must not hold the canary or mocked raw text').not.toContain(CANARY);
      expect(text).not.toContain(MOCK_RAW);
    };
    const expectBody = async (pending, status, code) => {
      const reply = await pending;
      expect(reply.status()).toBe(status);
      const body = await reply.json();
      expect(body.code).toBe(code);
      expect(typeof body.error).toBe('string');
      expect(JSON.stringify(body)).not.toContain(CANARY);
      return body;
    };
    const waitFor = (method, suffix) => page.waitForResponse((reply) => reply.request().method() === method && new URL(reply.url()).pathname.endsWith(suffix));

    caption('Bước 1 (thật): Nhà phát triển nhập sai mật khẩu; Console chỉ hiện câu tiếng Việt theo mã INVALID_CREDENTIALS');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), 'wrong-password');
    const badSignIn = waitFor('POST', '/auth/sign-in');
    await h.click(ui('signIn', 'submit'));
    await expectBody(badSignIn, 401, 'INVALID_CREDENTIALS');
    await expect(alertOf()).toHaveText('Tên đăng nhập hoặc mật khẩu không đúng.');
    check('REAL: auth: HTTP 401 code INVALID_CREDENTIALS shows the Vietnamese sentence; no raw English');
    await reveal();
    mark('auth-error');
    await h.read('Sai mật khẩu: thông báo tiếng Việt, không tiết lộ tài khoản hay trường nào sai');

    caption('Bước 2 (thật): Đăng nhập đúng rồi tạo ứng dụng; Staging dùng cluster mặc định');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD, { replace: true });
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    const appId = await createApplication(h, NAME, 't20-loi-theo-ma', { connections: { staging: 'internal-cluster' } });
    mark('setup-app');

    caption('Bước 3 (thật): Tạo lại ứng dụng cùng tên; mã DUPLICATE hiện ở ô Tên, bản nháp được giữ');
    await h.click(ui('shell', 'navApplications'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.click(page.getByRole('button', { name: '+ Tạo ứng dụng' }).first());
    await h.type(page.getByLabel('Tên ứng dụng'), NAME);
    await h.type(page.getByLabel('Subdomain'), 't20-trung-ten', { replace: true });
    const dup = waitFor('POST', '/applications');
    await h.click(page.getByRole('button', { name: 'Tạo ứng dụng' }));
    await expectBody(dup, 409, 'DUPLICATE');
    await expect(page.getByText('Tên ứng dụng này đã tồn tại.')).toBeVisible();
    await expect(page.getByLabel('Subdomain')).toHaveValue('t20-trung-ten');
    check('REAL: duplicate: 409 DUPLICATE with field name shows the field message and keeps the draft');
    await reveal(page.getByText('Tên ứng dụng này đã tồn tại.'));
    mark('duplicate-app');
    await h.read('Trùng tên ứng dụng: thông báo ở đúng ô, nội dung đang nhập được giữ');

    caption('Bước 4 (thật): Mở Cài đặt môi trường và thêm biến tên không hợp lệ, giá trị là chuỗi bí mật giả (canary); mã INVALID_VARIABLE, giá trị không hiện lại');
    await page.goto(`${baseURL}/ui/applications/${appId}`);
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
    const variables = page.locator('section[aria-label="Biến"]');
    await h.click(variables.getByRole('button', { name: '+ Thêm biến' }));
    await h.type(page.getByLabel('Tên key'), 'BAD-NAME');
    await h.type(page.getByLabel('Giá trị'), CANARY, { delay: 20 });
    const invalid = waitFor('PUT', '/configuration/keys/BAD-NAME');
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expectBody(invalid, 400, 'INVALID_VARIABLE');
    await expect(alertOf().first()).toContainText('Tên, giá trị hoặc tham chiếu biến không hợp lệ');
    expect(await alertOf().first().innerText()).not.toContain(CANARY);
    check('REAL: invalid variable: 400 INVALID_VARIABLE shows fixed Vietnamese text without the submitted value');
    await reveal();
    mark('invalid-variable');
    await h.read('Biến sai: thông báo tiếng Việt, giá trị đã nhập không bị lặp lại');

    caption('Bước 5 (thật): Hai phiên cùng sửa cấu hình; phiên thứ hai lưu trước nên phiên đầu nhận STALE_VERSION và tải lại');
    await h.type(page.getByLabel('Tên key'), 'T20_FIRST', { replace: true });
    await h.type(page.getByLabel('Giá trị'), 'one', { replace: true });
    const second = await page.context().newPage();
    await second.goto(`${baseURL}/ui/applications/${appId}`);
    const secondH = createHuman(second);
    await secondH.click(second.getByRole('button', { name: 'Cài đặt môi trường' }));
    await secondH.click(second.locator('section[aria-label="Biến"]').getByRole('button', { name: '+ Thêm biến' }));
    await secondH.type(second.getByLabel('Tên key'), 'T20_OTHER');
    await secondH.type(second.getByLabel('Giá trị'), 'other');
    const okSave = second.waitForResponse((reply) => reply.request().method() === 'PUT' && reply.url().endsWith('/configuration/keys/T20_OTHER'));
    await secondH.click(second.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    expect((await okSave).status()).toBe(200);
    await second.close();
    await page.bringToFront();
    const stale = waitFor('PUT', '/configuration/keys/T20_FIRST');
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expectBody(stale, 409, 'STALE_VERSION');
    await expect(alertOf().first()).toContainText('Dữ liệu đã thay đổi kể từ lần tải trước');
    await expect(alertOf().first()).toContainText('Đã tải lại giá trị mới nhất');
    await expect(variables.locator('.settings-table-row').filter({ hasText: 'T20_OTHER' })).toHaveCount(1);
    check('REAL: stale: 409 STALE_VERSION; the reload claim appears only after the authoritative GET succeeded; the other session variable is now listed; edit kept');
    await reveal();
    mark('stale-version');
    await h.read('Dữ liệu cũ: đã tải lại thật; bản đang nhập vẫn còn, lưu lại là thao tác chủ động');

    caption('Bước 6 (MÔ PHỎNG projection): Máy chủ trả ENVIRONMENT_BUSY; thông báo bận, không tải lại và không mất bản nhập');
    let projection = 'busy';
    const configUrl = '**/api/v1/applications/*/environments/staging/configuration/keys/*';
    await page.route(configUrl, async (route) => {
      if (route.request().method() !== 'PUT' || projection === 'off') { await route.continue(); return; }
      mocked.add(route.request().url());
      if (projection === 'busy') await route.fulfill({ status: 409, contentType: 'application/json', body: JSON.stringify({ error: `busy ${MOCK_RAW}`, code: 'ENVIRONMENT_BUSY' }) });
      else await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: `pq: password=${MOCK_RAW}`, code: 'SOMETHING_FROM_THE_FUTURE', field: 'user.alias', params: { stage: MOCK_RAW } }) });
    });
    let configGets = 0;
    page.on('request', (request) => { if (request.method() === 'GET' && new URL(request.url()).pathname.endsWith('/environments/staging/configuration')) configGets += 1; });
    const getsBefore = configGets;
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expect(alertOf().first()).toContainText('Môi trường đang bận');
    await expect(alertOf().first()).not.toContainText('Đã tải lại');
    await h.pause(800);
    expect(configGets).toBe(getsBefore);
    await expect(page.getByLabel('Tên key')).toHaveValue('T20_FIRST');
    await expect(page.getByLabel('Giá trị')).toHaveValue('one');
    await noRaw();
    check('PROJECTION: BUSY projection: fixed busy sentence, no configuration reload, form values kept, raw mocked text absent');
    await reveal();
    mark('busy-projection');
    await h.read('Bận (mô phỏng): giữ nguyên bản nhập, không coi là dữ liệu cũ');

    caption('Bước 7 (MÔ PHỎNG projection): Mã lỗi lạ từ tương lai; Console dùng câu dự phòng tiếng Việt, không hiện chữ gốc');
    projection = 'unknown';
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expect(alertOf().first()).toContainText('Máy chủ gặp lỗi không xác định');
    await noRaw();
    check('PROJECTION: unknown-code projection: HTTP 500 fallback Vietnamese; raw English, unknown field and params never shown');
    await reveal();
    mark('unknown-projection');
    await h.read('Mã lạ (mô phỏng): câu dự phòng tiếng Việt an toàn');
    projection = 'off';
    await page.unroute(configUrl);

    caption('Bước 8 (MÔ PHỎNG projection): Thiết lập mô phỏng Staging đang chọn Kho A; chuyển sang Kho B bị SECRET_COPY_FAILED, Kho A vẫn được chọn');
    // PROJECTION SETUP (not a real store): the Environment read is patched so Staging reports the bound store "kho-a";
    // choices and the store PUT are mocked too. No Vault, no copy and no real store-selection write happen.
    const stores = [{ key: 'kho-a', name: 'Kho A', provider: 'VAULT_KV_V2', status: 'READY' }, { key: 'kho-b', name: 'Kho B', provider: 'VAULT_KV_V2', status: 'READY' }];
    let storePuts = 0;
    const patchStaging = (environments) => environments?.map((item) => (item.key === 'staging' ? { ...item, secretStoreKey: 'kho-a', secretStoreName: 'Kho A' } : item));
    await page.route(/\/api\/v1\/applications(\/[^/]+)?$/, async (route) => {
      if (route.request().method() !== 'GET') { await route.continue(); return; }
      const reply = await route.fetch();
      const body = await reply.json();
      if (body.applications) body.applications = body.applications.map((item) => ({ ...item, environments: patchStaging(item.environments) }));
      if (body.application) body.application = { ...body.application, environments: patchStaging(body.application.environments) };
      mocked.add(route.request().url());
      await route.fulfill({ response: reply, json: body });
    });
    await page.route('**/api/v1/secret-store-choices', async (route) => { mocked.add(route.request().url()); await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ secretStores: stores }) }); });
    await page.route('**/api/v1/applications/*/environments/staging/secret-store', async (route) => {
      if (route.request().method() !== 'PUT') { await route.continue(); return; }
      storePuts += 1;
      mocked.add(route.request().url());
      await route.fulfill({ status: 502, contentType: 'application/json', body: JSON.stringify({ error: `copy failed ${MOCK_RAW}`, code: 'SECRET_COPY_FAILED' }) });
    });
    await page.goto(`${baseURL}/ui/applications`);
    await expect(ui('applications', 'heading')).toBeVisible();
    await page.goto(`${baseURL}/ui/applications/${appId}`);
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
    const storeSelect = page.getByLabel('Kho bí mật cho Staging');
    await expect(storeSelect).toBeVisible();
    await expect(storeSelect).toHaveValue('kho-a');
    check('PROJECTION: copy projection setup (MOCK): Staging reads as bound to kho-a; the real backend still has no store bound');
    const storeLabel = (await storeSelect.locator('option').allTextContents()).find((text) => text.includes('Kho B'));
    if (!storeLabel) throw new Error('mocked store Kho B is not offered');
    await h.choose(storeSelect, storeLabel);
    await expect(storeSelect).toHaveValue('kho-b');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(alertOf().first()).toContainText('Kho cũ vẫn đang được chọn và không có gì thay đổi');
    expect(storePuts).toBe(1);
    await noRaw();
    await reveal();
    mark('copy-failed-alert-visible');
    await h.read('Sao chép bí mật lỗi (mô phỏng): thông báo hiện rõ, Kho A vẫn là kho đã lưu');
    await h.pause(1500);
    expect(storePuts).toBe(1);
    // The persisted view is still the previous store: reload the (patched) Environment and read it back.
    await page.reload();
    await expect(page.getByLabel('Kho bí mật cho Staging')).toHaveValue('kho-a');
    check('PROJECTION: SECRET_COPY_FAILED projection: Vietnamese copy-failure sentence; exactly one store PUT, no automatic replay; after a reload the previous store kho-a is still the selected one (the Environment read is a labelled projection patch, not a real store binding)');
    await h.moveTo(storeSelect);
    await h.read('Sau khi tải lại: Kho A vẫn được chọn (mô phỏng)');
    mark('copy-failed-after-reload');
    await page.unroute(/\/api\/v1\/applications(\/[^/]+)?$/);
    await page.unroute('**/api/v1/secret-store-choices');
    await page.unroute('**/api/v1/applications/*/environments/staging/secret-store');

    caption('Bước 9 (thật): Kỹ sư nền tảng đăng ký loại tài nguyên với ID sai chứa chuỗi giả (canary); mã INVALID_ID kèm nhãn trường');
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    await h.type(ui('signIn', 'username'), 'platform-engineer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    await h.click(ui('shell', 'navResourceTypes'));
    await expect(ui('resourceTypes', 'heading')).toBeVisible();
    await h.type(ui('resourceTypes', 'id'), `Bad_${CANARY}`);
    const badId = waitFor('POST', '/resource-types');
    await h.click(ui('resourceTypes', 'submit'));
    await expectBody(badId, 400, 'INVALID_ID');
    await expect(alertOf().first()).toContainText('ID không đúng quy tắc đặt tên');
    await expect(alertOf().first()).toContainText('ID loại tài nguyên');
    expect(await alertOf().first().innerText()).not.toContain(CANARY);
    check('REAL: invalid ID: 400 INVALID_ID with trusted field type.key; the submitted ID is not echoed');
    await reveal();
    mark('invalid-id');
    await h.read('ID sai: thông báo cụ thể, không lặp lại ID đã gửi');

    caption('Bước 10 (thật): Đăng ký hai lần cùng một loại tài nguyên; lần hai nhận DUPLICATE và không ghi đè');
    await h.type(ui('resourceTypes', 'id'), 't20-bo-nho', { replace: true });
    const first = waitFor('POST', '/resource-types');
    await h.click(ui('resourceTypes', 'submit'));
    expect((await first).status()).toBe(201);
    await expect(page.getByRole('status').filter({ hasText: 't20-bo-nho' }).first()).toBeVisible();
    await h.type(ui('resourceTypes', 'id'), 't20-bo-nho', { replace: true });
    const again = waitFor('POST', '/resource-types');
    await h.click(ui('resourceTypes', 'submit'));
    await expectBody(again, 409, 'DUPLICATE');
    await expect(alertOf().first()).toContainText('ID loại tài nguyên đã tồn tại');
    check('REAL: duplicate type: 201 then 409 DUPLICATE; the first registration is untouched');
    await reveal();
    mark('duplicate-type');
    await h.read('Trùng ID: không ghi đè bản đã đăng ký');

    caption('Bước 11 (thật): Dán kubeconfig sai có chứa chuỗi bí mật giả (canary, không phải thông tin thật); lỗi KUBECONFIG_* không hiện lại nội dung');
    await h.click(ui('shell', 'navConnections'));
    await expect(ui('connections', 'heading')).toBeVisible();
    await h.click(ui('connections', 'sourcePaste'));
    const inspect = waitFor('POST', '/connections/kubernetes/inspect');
    await h.paste(ui('connections', 'content'), `not: [valid kubeconfig ${CANARY}`);
    const inspected = await inspect;
    expect(inspected.status()).toBe(400);
    const inspectBody = await inspected.json();
    expect(inspectBody.code).toMatch(/^KUBECONFIG_/);
    expect(JSON.stringify(inspectBody)).not.toContain(CANARY);
    await expect(alertOf().first()).toBeVisible();
    expect(await alertOf().first().innerText()).not.toContain(CANARY);
    check(`REAL: kubeconfig: 400 ${inspectBody.code}; the pasted secret is not echoed by the API or the alert`);
    await reveal();
    mark('kubeconfig-error');
    await h.read('Kubeconfig sai: thông báo cụ thể, không lặp lại nội dung đã dán');

    for (const item of realBodies) {
      expect(item.body, `${item.status} ${item.url} leaked canary`).not.toContain(CANARY);
      expect(item.body).not.toContain(MOCK_RAW);
    }
    check(`REAL: all ${realBodies.length} real error responses carry neither the submitted canary nor raw provider text`);
    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(3000);
  },

  // T23: ordinary-store selection. The fake backend registers and verifies stores
  // through the same input path as real registration; the only MOCK is a stale
  // `legacy: true` flag injected into the store list/choices to prove the Console
  // renders no legacy label for any store.
  async T23({ page, h, ui, caption, mark, check, baseURL }) {
    const TOKEN = 'synthetic-scoped-token-t23';
    const SECRET_VALUE = 'synthetic-t23-secret-value-4567';
    const APP_NAME = 'T23 Kho thường';
    const STORE_NAME = 'Kho thường T23';
    const bodies = []; // every store list/choices response, scanned at the end
    const staleLegacy = (payload) => ({ ...payload, secretStores: payload.secretStores.map((item) => ({ ...item, legacy: true })) });
    for (const pattern of ['**/api/v1/secret-stores', '**/api/v1/secret-store-choices']) {
      await page.route(pattern, async (route) => {
        if (route.request().method() !== 'GET') { await route.continue(); return; }
        const response = await route.fetch();
        const data = await response.json();
        bodies.push(JSON.stringify(data));
        await route.fulfill({ response, json: staleLegacy(data) }); // MOCK: stale legacy flag on every item
      });
    }
    const signInAs = async (username) => {
      await h.type(ui('signIn', 'username'), username);
      await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
      await h.click(ui('signIn', 'submit'));
      await expect(ui('applications', 'heading')).toBeVisible();
    };
    const signOut = async () => { await h.click(ui('shell', 'signOut')); await expect(ui('signIn', 'username')).toBeVisible(); };
    const noLeak = async () => { const { assertNoSecretLeaks } = await import('./secret-scan.mjs'); await assertNoSecretLeaks(page, [SECRET_VALUE, TOKEN], { allow: ['input[type="password"]'] }); };
    const storePanel = () => page.getByLabel('Kho bí mật', { exact: true });
    const storeSelect = () => page.getByLabel('Kho bí mật cho Staging');
    const envApi = async (appId) => {
      const response = await page.request.get(`${baseURL}/api/v1/applications/${appId}`);
      return (await response.json()).application.environments.find((env) => env.key === 'staging');
    };

    caption('Bước 1: Nhà phát triển tạo ứng dụng mới; chưa có kho bí mật nào nên không có gì được chọn sẵn');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signInAs('developer');
    const appId = await createApplication(h, APP_NAME, 't23-ordinary-store', { connections: null });
    check(`developer created ${appId} through the UI`);
    mark('app-created');
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await expect(page.getByRole('heading', { name: 'Cài đặt môi trường', level: 1 })).toBeVisible();
    await expect(storePanel()).toContainText('Chưa có kho bí mật Sẵn sàng');
    expect((await envApi(appId)).secretStoreKey ?? '', 'no store is fabricated or selected').toBe('');
    check('no registered store: guidance shown, Environment has no store (fail closed, nothing fabricated)');
    mark('no-store');
    await h.read('Chưa đăng ký kho thì chưa có kho nào được chọn hay tạo giả');

    caption('Bước 2: Kỹ sư nền tảng đăng ký một kho thường; token chỉ nhập vào ô được che, danh sách không có nhãn cấu hình cũ (MOCK: cờ legacy cũ bị chèn vào phản hồi)');
    await signOut();
    await signInAs('platform-engineer');
    await h.click(ui('shell', 'navSecretStores'));
    await expect(ui('secretStores', 'heading')).toBeVisible();
    await h.type(ui('secretStores', 'name'), STORE_NAME);
    await h.type(ui('secretStores', 'backend'), 'http://vault-t23.local:8200');
    await h.click(ui('secretStores', 'showAdvanced'));
    await h.click(ui('secretStores', 'sameAddress'));
    await h.paste(ui('secretStores', 'token'), TOKEN);
    mark('store-form-filled');
    await h.click(ui('secretStores', 'submit'));
    await expect(page.getByRole('status').filter({ hasText: `Kho bí mật "${STORE_NAME}" đã sẵn sàng` })).toBeVisible();
    const list = await page.getByLabel('Kho bí mật đã đăng ký');
    await expect(list).toContainText(STORE_NAME);
    expect(await list.innerText()).not.toMatch(/cấu hình cũ|nền tảng cũ|legacy/i);
    await noLeak();
    check('store registered through the UI; registered list has no legacy label even though the response carried a stale legacy flag (MOCK)');
    mark('store-registered');
    await h.moveTo(list);
    await h.read('Kho thường hiển thị tên, khóa, địa chỉ và trạng thái; không có nhãn cấu hình cũ');

    caption('Bước 3: Nhà phát triển chỉ có một kho Sẵn sàng; nó được gợi ý trong bản nháp chưa lưu, nhãn chỉ gồm tên và khóa, rồi bấm Lưu kho bí mật');
    await signOut();
    await signInAs('developer');
    await h.click(page.getByRole('button', { name: new RegExp(APP_NAME) }));
    await h.click(page.getByRole('button', { name: 'Cài đặt môi trường' }));
    await expect(storeSelect()).toHaveValue(/.+/);
    mark('store-suggested');
    const labels = await storeSelect().locator('option').allTextContents();
    expect(labels.join('|')).toContain(STORE_NAME);
    expect(labels.join('|')).not.toMatch(/cũ|legacy/i);
    expect((await envApi(appId)).secretStoreKey ?? '', 'suggestion is not persisted').toBe('');
    await h.click(page.getByRole('button', { name: 'Lưu kho bí mật' }));
    await expect(storePanel()).toContainText('Đã lưu kho bí mật');
    const storeKey = (await envApi(appId)).secretStoreKey;
    expect(storeKey).toBeTruthy();
    check('selector labels carry only name and key (stale legacy flag ignored); explicit Save bound the ordinary store');
    mark('store-selected');
    await h.read('Chỉ sau khi bấm Lưu môi trường mới dùng kho thường này');

    caption('Bước 4: Thêm Bí mật API_TOKEN vào kho đã chọn; giá trị bị che và không bao giờ hiện lại');
    await h.click(page.locator('section[aria-label="Bí mật"]').getByRole('button', { name: '+ Thêm bí mật' }));
    mark('secret-form-open');
    await h.type(page.getByLabel('Tên key'), 'API_TOKEN');
    await h.paste(page.getByLabel('Giá trị bí mật mới'), SECRET_VALUE);
    await h.click(page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
    await expect(page.locator('.settings-table-row').filter({ hasText: 'API_TOKEN' })).toContainText('Đã cấu hình');
    await noLeak();
    check('secret written through the selected ordinary store; value never on the page');
    mark('secret-saved');
    await h.read('Bí mật đã lưu qua kho thường; giá trị không hiển thị lại');

    for (const body of bodies) {
      expect(body, 'store response').not.toMatch(/legacy|credentialRef|kv2:\/\/|synthetic-scoped-token/i);
    }
    check(`all ${bodies.length} real store list/choice responses carry no legacy, credential reference or token`);
    await signOut();
    mark('signed-out');
    await h.pause(2000);
  },
  // T25: chỉ publicRoutes. Developer. PRODUCT (UI, backend thật với fake adapters, state tạm): đăng nhập, tạo ứng dụng, nhập Score nhiều route hợp lệ,
  // sửa image, Lưu draft, Xem trước (adapter fake, không có Ingress cluster thật), rồi nhập Score còn publicPort bị backend từ chối INVALID_SCORE.
  // Không có MOCK page.route. Các GET/PUT bằng page.request chỉ để kiểm tra kết quả/âm tính, không là bước sản phẩm; adapter Kubernetes là fake.
  async T25({ page, h, ui, caption, mark, check, baseURL }) {
    const NAME = 'T25 Public routes';
    const editor = (key) => ui('workloadEditor', key);
    const putCalls = [];
    page.on('request', (request) => { if (request.method() === 'PUT' && /\/environments\/staging\/workloads\/[^/]+$/.test(new URL(request.url()).pathname)) putCalls.push(request.url()); });
    const stored = async (appId) => (await (await page.request.get(`${baseURL}/api/v1/applications/${appId}/environments/staging/workloads`)).json());
    const importScore = async (name, score) => editor('scoreFile').setInputFiles({ name, mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(score)) });
    const base = (name, service) => ({ apiVersion: 'score.dev/v1b1', metadata: { name }, containers: { main: { image: `registry.example/${name}:t25` } }, service });
    const ports = { http: { port: 8080, targetPort: 8080 }, admin: { port: 9000, targetPort: 9000 } };
    const routes = [{ path: '/', port: 'http' }, { path: '/admin', port: 'admin' }];
    const editedRoutes = [{ path: '/', port: 'http' }, { path: '/ops', port: 'admin' }];

    caption('Bước 1: Lập trình viên đăng nhập và tạo ứng dụng dùng backend fake adapters (Kubernetes là fake, không phải cluster thật)');
    await h.showCursor();
    await expectVisibleCursor(page);
    await h.type(ui('signIn', 'username'), 'developer');
    await h.type(ui('signIn', 'password'), SEEDED_PASSWORD);
    await h.click(ui('signIn', 'submit'));
    await expect(ui('applications', 'heading')).toBeVisible();
    const appId = await createApplication(h, NAME, 't25-routes');
    mark('prepared');
    check('setup via UI: application created with fake internal-cluster target (fake adapter, no real cluster)');

    caption('Bước 2: Nhập Score có hai publicRoutes hợp lệ (/ và /admin); form hiện đủ hai đường dẫn, chưa lưu gì');
    await h.click(page.getByRole('button', { name: '+ Thêm Workload' }));
    await expect(page.getByRole('heading', { name: 'Thêm Workload', level: 1 })).toBeVisible();
    await h.click(editor('importTab'));
    await importScore('web.json', base('web', { ports, publicRoutes: routes }));
    await expect(page.getByText('Đã phân tích Workload:')).toContainText('web');
    await h.click(editor('formTab'));
    await expect(editor('routePath')).toHaveCount(2);
    await expect(editor('routePath').nth(0)).toHaveValue('/');
    await expect(editor('routePath').nth(1)).toHaveValue('/admin');
    expect(putCalls.length).toBe(0);
    expect((await stored(appId)).workloads).toHaveLength(0);
    check('valid import (real backend parse): two routes shown in the form, no PUT and no draft written yet');
    mark('imported-valid');
    await h.read('Hai route hợp lệ đã vào form; nhập chưa lưu');

    caption('Bước 3: Sửa image và đổi route /admin thành /ops bằng form rồi Lưu draft; Score lưu đúng hai publicRoutes đã sửa, không có publicPort');
    await h.type(editor('image'), 'registry.example/web:t25-v2', { replace: true });
    await h.type(editor('routePath').nth(1), '/ops', { replace: true });
    await expect(editor('routePath').nth(1)).toHaveValue('/ops');
    mark('edited');
    const saved = page.waitForResponse((response) => response.request().method() === 'PUT' && new URL(response.url()).pathname.endsWith('/workloads/web'));
    await h.click(editor('save'));
    const response = await saved;
    expect(response.status()).toBe(200);
    const sent = response.request().postDataJSON().score;
    expect(sent.service).toEqual({ ports, publicRoutes: editedRoutes });
    expect(JSON.stringify(sent)).not.toContain('publicPort');
    const afterSave = await stored(appId);
    const savedWeb = afterSave.workloads.find((item) => item.id === 'web').score;
    expect(savedWeb.service).toEqual({ ports, publicRoutes: editedRoutes });
    expect(savedWeb.containers.main.image).toBe('registry.example/web:t25-v2');
    check('PUT payload and stored draft (backend GET) hold exactly the edited routes / and /ops, edited image, no publicPort');
    mark('saved');
    await h.read('Draft lưu đúng hai route đã sửa');

    caption('Bước 3b: Mở lại Sửa web; hai route / và /ops được khôi phục đầy đủ từ draft, rồi Hủy về trang ứng dụng');
    await h.click(page.getByRole('button', { name: 'Sửa', exact: true }));
    await expect(page.getByRole('heading', { name: 'Sửa web', level: 1 })).toBeVisible();
    await expect(editor('routePath')).toHaveCount(2);
    await expect(editor('routePath').nth(0)).toHaveValue('/');
    await expect(editor('routePath').nth(1)).toHaveValue('/ops');
    await expect(editor('routePort').nth(0)).toHaveValue('http');
    await expect(editor('routePort').nth(1)).toHaveValue('admin');
    check('reopened Edit restores both routes: / -> http and /ops -> admin');
    mark('reopened');
    await h.read('Form sửa khôi phục đủ hai route');
    await h.click(page.getByRole('button', { name: 'Hủy', exact: true }));
    await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();

    caption('Bước 4: Xem trước thay đổi (adapter fake): một Workload bị ảnh hưởng; Triển khai vẫn là thao tác riêng, không bấm');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi' }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('1 Workload bị ảnh hưởng', { timeout: 120_000 });
    await h.moveTo(page.getByLabel('Xem trước triển khai'));
    check('Preview (real backend planner, fake adapter): 1 Workload affected with two valid routes; nothing deployed');
    mark('preview');
    await h.read('Xem trước đọc từ backend thật, adapter fake; chưa triển khai');

    caption('Bước 5: Nhập Score cũ còn service.publicPort; backend từ chối INVALID_SCORE, giao diện báo tiếng Việt, không tạo route / và không lưu');
    const putsBefore = putCalls.length;
    const draftsBefore = JSON.stringify(await stored(appId));
    await h.click(page.getByRole('button', { name: '+ Thêm Workload' }));
    await expect(page.getByRole('heading', { name: 'Thêm Workload', level: 1 })).toBeVisible();
    await h.click(editor('importTab'));
    await importScore('old.json', base('old', { ports: { http: { port: 8080 } }, publicPort: 'http' }));
    const alert = page.getByRole('alert');
    await expect(alert).toContainText('publicPort không còn được hỗ trợ');
    await expect(alert).toContainText('publicRoutes');
    await expect(page.getByText('Đã phân tích Workload:')).toHaveCount(0);
    expect(putCalls.length).toBe(putsBefore);
    expect(JSON.stringify(await stored(appId))).toBe(draftsBefore);
    check('old alias import: Vietnamese rejection naming publicRoutes, no parsed panel, no PUT, draft list byte-identical (no conversion to "/")');
    mark('alias-rejected');
    await h.moveTo(alert);
    await h.read('publicPort bị từ chối; không tự chuyển thành route /');

    caption('Bước 6 (kiểm tra âm tính, không phải thao tác sản phẩm): lưu thẳng qua API với publicPort rỗng/null/cùng publicRoutes đều bị 400 INVALID_SCORE');
    const version = (await stored(appId)).draftVersion;
    for (const publicPort of ['', null, 'http']) {
      const reply = await page.request.put(`${baseURL}/api/v1/applications/${appId}/environments/staging/workloads/old`, { data: { version, score: base('old', { ports: { http: { port: 8080 } }, publicRoutes: [{ path: '/x', port: 'http' }], publicPort }) } });
      expect(reply.status()).toBe(400);
      expect((await reply.json()).code).toBe('INVALID_SCORE');
    }
    const finalList = await stored(appId);
    expect(finalList.draftVersion).toBe(version);
    expect(finalList.workloads.map((item) => item.id)).toEqual(['web']);
    check('API negative probe: empty, null and with-routes publicPort all 400 INVALID_SCORE; draft version and list unchanged');
    mark('api-negative');

    await h.click(ui('shell', 'signOut'));
    await expect(ui('signIn', 'username')).toBeVisible();
    mark('signed-out');
    await h.pause(2000);
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
    startedAt = recorder.spawnedAtMs;
    await page.waitForTimeout(1500);
    const mark = (label) => marks.push({ label, seconds: Number(((Date.now() - startedAt) / 1000).toFixed(1)) });
    const fail = async (message) => { appendFileSync(assertionsPath, `FAIL ${message}\n`); await expect(message, message).toBe(''); };
    // One still frame of the whole X screen: native dialogs are not in the page screenshot.
    const snap = (name) => execFileSync('ffmpeg', ['-hide_banner', '-loglevel', 'error', '-y', '-f', 'x11grab', '-video_size', `${width}x${height}`, '-i', env.DISPLAY, '-frames:v', '1', `${evidenceDir}/${name}.png`]);
    await scenario({ page, h, ui: locators(page), caption: captions.caption, mark, check, baseURL, fail, x11, snap });
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
