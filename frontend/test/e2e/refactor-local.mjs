// Scenario runner for the Web Console refactor (refactor.md). Invoked by
// backend/test/integration/refactor-ui-local.sh against a fake-adapter backend
// with temporary JSON state. Registry keys are task IDs (Txx); a task that
// has no scenario here is rejected before any process starts:
//   node refactor-local.mjs --check <Txx>
// Every product step is a UI action; API reads only assert results. Marks,
// captions, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { appendFileSync, existsSync, readFileSync, writeFileSync } from 'node:fs';
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
