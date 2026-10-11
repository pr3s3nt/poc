// Scenario runner for the Web Console refactor (refactor.md). Invoked by
// backend/test/integration/refactor-ui-local.sh against a fake-adapter backend
// with temporary JSON state. Registry keys are task IDs (Txx); a task that
// has no scenario here is rejected before any process starts:
//   node refactor-local.mjs --check <Txx>
// Every product step is a UI action; API reads only assert results. Marks,
// captions, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { appendFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { addWorkload, createApplication, createHuman, expectVisibleCursor, installCursor } from './human.mjs';
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

    caption('Bước 2: Kỹ sư nền tảng mở trang Mẫu dựng ứng dụng; danh sách trống, có hướng dẫn tiếng Việt');
    await signInAs('platform-engineer');
    await h.click(ui('shell', 'navRenderingTemplates'));
    await expect(ui('renderingTemplates', 'heading')).toBeVisible();
    await expect(page).toHaveURL(`${baseURL}/ui/platform/rendering-templates`);
    await expect(page.getByText('Chưa có mẫu dựng ứng dụng nào.')).toBeVisible();
    await expect(page.getByText(/Mẫu là tùy chọn và chỉ dùng cho cluster nội bộ/)).toBeVisible();
    await expect(page.getByText(/không chuyển âm thầm sang renderer mặc định/)).toBeVisible();
    await expect(page.getByText(/chưa phải bộ chọn từ danh sách bundle đã cài/)).toBeVisible();
    check('sidebar peer link opens /ui/platform/rendering-templates; empty list, optional/internal-only/no-fallback guidance and explicit-bundle note shown');
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
    await h.type(ui('renderingTemplates', 'bundle'), 'bundle-khong-co');
    await h.click(editor.getByRole('radio', { name: /Theo ứng dụng/ }));
    await h.choose(select(/^Ứng dụng(?! mẫu)/), await optionLabel(select(/^Ứng dụng(?! mẫu)/), templateId));
    await h.choose(select(/^Môi trường \(tùy chọn\)/), await optionLabel(select(/^Môi trường \(tùy chọn\)/), 'staging'));
    await h.click(ui('renderingTemplates', 'submit'));
    await expect(formAlert()).toContainText('Máy chủ từ chối mẫu này');
    await expect(formAlert()).toContainText('bundle có thể chưa được cài');
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
    await h.type(ui('renderingTemplates', 'bundle'), 'score-k8s-internal-v1', { replace: true });
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
    await h.type(ui('renderingTemplates', 'bundle'), 'score-k8s-internal-v1');
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
    await h.click(page.getByRole('button', { name: 'Preview changes' }));
    const preview = page.getByLabel('Deployment preview');
    await expect(preview).toContainText('1 workload(s) affected', { timeout: 60_000 });
    await expect(preview.locator('li').filter({ hasText: /^web / })).toContainText('built-in Kubernetes');
    await expect(preview.locator('li').filter({ hasText: /^web / })).not.toContainText('score-k8s');
    check('Preview, application without matching template: native renderer (built-in Kubernetes), no score-k8s provenance');
    mark('preview-native');
    await h.moveTo(preview);
    await h.read('Không có mẫu khớp thì dùng renderer mặc định');
    expect(calls(), 'Preview never runs the renderer').toEqual([]);
    check('Preview ran no score-k8s call (pure planning)');

    caption('Bước 8: Triển khai ứng dụng này; renderer mặc định thành công, không gọi score-k8s');
    await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
    const nativeResult = page.getByLabel('Deployment result');
    await expect(nativeResult).toContainText('Deploy succeeded', { timeout: 120_000 });
    expect(calls(), 'native deploy does not call the template renderer').toEqual([]);
    check('Deploy of the native application succeeded (fake deliverer) with zero score-k8s stub calls');
    mark('deploy-native-ok');
    await h.moveTo(nativeResult);
    await h.read('Triển khai bằng renderer mặc định thành công');

    caption('Bước 9: Xem trước ứng dụng có mẫu khớp; hệ thống chọn đúng mẫu tpl-web');
    await h.click(ui('shell', 'navApplications'));
    await openApplication(TEMPLATE_NAME);
    await h.click(page.getByRole('button', { name: 'Preview changes' }));
    const templatePreview = page.getByLabel('Deployment preview');
    await expect(templatePreview).toContainText('1 workload(s) affected', { timeout: 60_000 });
    await expect(templatePreview.locator('li').filter({ hasText: /^web / })).toContainText('score-k8s 0.15.0 (tpl-web)');
    await expect(templatePreview.locator('li').filter({ hasText: /^web / })).not.toContainText('built-in Kubernetes');
    check('Preview, application matching criteria: selected template tpl-web (score-k8s 0.15.0), not native');
    mark('preview-template');
    expect(calls(), 'Preview selection did not run the renderer').toEqual([]);
    await h.moveTo(templatePreview);
    await h.read('Mẫu khớp điều kiện được chọn, thông tin mẫu hiện trong bản xem trước');

    caption('Bước 10: Triển khai; mẫu đã chọn render lỗi nên báo lỗi, không chuyển sang renderer mặc định');
    await h.click(page.getByRole('button', { name: 'Deploy these changes' }));
    const failedResult = page.getByLabel('Deployment result');
    await expect(failedResult).toContainText('Deploy failed', { timeout: 120_000 });
    await expect(failedResult.locator('li').filter({ hasText: /^web · / })).toContainText('failed');
    await expect(failedResult).not.toContainText('Deploy succeeded');
    await expect(failedResult).toContainText('Unfinished changes stay pending');
    const stubCalls = calls();
    expect(stubCalls.some((line) => line.startsWith('generate ')), 'selected renderer was invoked and failed').toBe(true);
    check(`Deploy with selected template failed through the product Deploy path (stub generate exit 1); stub calls: ${stubCalls.length}`);
    mark('deploy-render-failed');
    await h.moveTo(failedResult);
    await h.read('Mẫu được chọn render lỗi: báo lỗi, workload vẫn chờ triển khai');

    caption('Bước 11: Xem trước lại; workload vẫn chờ triển khai bằng mẫu, chưa bị renderer mặc định thay thế');
    await h.click(page.getByRole('button', { name: 'Preview changes' }));
    const again = page.getByLabel('Deployment preview');
    await expect(again).toContainText('1 workload(s) affected', { timeout: 60_000 });
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
      await h.click(page.getByRole('button', { name: 'Environment settings' }));
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
    await h.click(page.getByRole('button', { name: 'Environment settings' }));
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
    await expect(storePanel().getByRole('alert')).toContainText('busy with another operation');
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
    await expect(banner).toContainText('Cần xử lý: dừng workload đích web');
    check('recovery (MOCKED API): disabled until the stop confirmation, then result and "Cần xử lý" item shown in Vietnamese');
    mark('recovery');
    await h.moveTo(banner);
    await h.read('Khôi phục chỉ chạy sau xác nhận tường minh');
    await noLeak();

    caption('Bước 22: Đăng xuất; phiên bị thu hồi');
    await signOut();
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
