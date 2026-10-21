// I04-07 headed recording: Platform Engineer registers a Type and Definitions
// from verified registry pins, Developer chooses a Workload class, saves a
// draft and previews. Invoked by backend/test/integration/definition-workload-local.sh,
// which serves the Console from backend/test/integration/defworkserver:
//   REAL   Web Console bundle, product HTTP handlers and services, a disposable
//          PostgreSQL Console store, a run-owned Vault KV v2 credential store,
//          the verified Terraform module registry over the packaged release
//          modules and the pinned score-k8s binary (required renderer).
//   FAKE   workload/resource executors (nothing is provisioned or deployed) and
//          one network interception (page.route) that makes the class metadata
//          read fail once. A pass is NOT live proof for V2-A01/A02/A03.
// Every product step is a UI action; API reads only assert results.
//   node definition-workload-local.mjs --check I04-07
//   node definition-workload-local.mjs --scenario I04-07 --headed --captions vi --evidence DIR
// Marks, captions, assertions and the video are written also when a step fails.
import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { appendFileSync, writeFileSync } from 'node:fs';
import { requireCaptionLocale, createCaptions } from './captions.mjs';
import { createApplication, createHuman, expectVisibleCursor, installCursor } from './human.mjs';
import { launchHeaded, nativeSelect, screenSize, startRecording, stopRecording, x11Input } from './video.mjs';

const SHORT_READ = 2000;
const SEEDED_PASSWORD = 'test-password'; // fixed synthetic local account, typed into a masked field
const FORBIDDEN_IN_PREVIEW = [/password/i, /kubeconfig/i, /credential/i, /secretRef/i, /tfstate/i, /state ref/i, /AKIA/];

export const SCENARIOS = {
  async 'I04-07'({ page, h, caption, mark, check, baseURL, evidenceDir }) {
    const nav = (name) => page.getByRole('link', { name: new RegExp(`(^|\\s)${name}$`) });
    const get = async (path) => (await (await page.request.get(`${baseURL}/api/v1${path}`)).json());
    // Native selects wrapped in a <label> have the option texts in their label, so exact label matching fails for them.
    const wrappedSelects = new Set(['Loại tài nguyên', 'Module đã kiểm chứng', 'Tài khoản hạ tầng', 'Bundle dựng ứng dụng']);
    const form = (name) => (wrappedSelects.has(name)
      ? page.locator(`select[aria-label="${name}"], label:has-text("${name}") > select`).first()
      : page.getByLabel(name, { exact: true }));
    const alert = () => page.getByRole('alert').first();
    const submitDefinition = () => page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true });
    // The visible option label that starts with `prefix` (types and pins are chosen by what the screen shows).
    async function optionLabel(select, prefix) {
      const texts = await select.locator('option').allTextContents();
      const found = texts.find((text) => text.trim().startsWith(prefix));
      if (!found) throw new Error(`no option starting with "${prefix}"`);
      return found;
    }
    const choose = async (select, prefix) => h.choose(select, await optionLabel(select, prefix));

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
    // Opts into a typed override of one module variable and pastes its value.
    async function override(name, value) {
      await h.click(form(`Ghi đè ${name}`));
      await h.paste(form(`Giá trị ${name}`), value);
    }
    async function startDefinition(id, type, module, _scope, name = id) {
      await h.type(form('ID cấu hình (Definition ID)'), id);
      await h.type(form('Tên hiển thị'), name);
      await choose(form('Loại tài nguyên'), type);
      await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
      await expect(form('Loại tài nguyên')).toHaveValue(type);
      if (module) {
        const pins = form('Module đã kiểm chứng');
        if (await pins.isEnabled()) await choose(pins, module);
        await expect(pins).toContainText(module);
      }
      const account = form('Tài khoản hạ tầng');
      if (await account.count()) {
        await choose(account, 'aws-account'); // the seeded READY fake account (Name = key)
        await h.type(form('Vùng AWS'), 'us-east-1');
      }
    }
    async function registered(id) {
      await expect(page.getByRole('status').filter({ hasText: `Đã đăng ký cấu hình tài nguyên ${id}.` })).toBeVisible();
    }
    const pinOf = (capabilities, module) => capabilities.find((item) => item.driverType === 'terraform' && item.module === module);

    // ---- Platform Engineer -------------------------------------------------
    caption('Bước 1: Kỹ sư nền tảng đăng nhập bằng tài khoản thử nghiệm cục bộ; ô mật khẩu luôn bị che');
    await h.showCursor();
    await expectVisibleCursor(page);
    await signIn('platform-engineer');
    mark('platform-engineer-signed-in');
    await h.pause(SHORT_READ);

    caption('Bước 2: Mở Nền tảng › Loại tài nguyên; Redis chưa được đăng ký nên cần đăng ký hợp đồng Redis trước khi tạo cấu hình');
    await h.click(nav('Loại tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Loại tài nguyên', level: 1, exact: true })).toBeVisible();
    const typesBefore = (await get('/resource-types')).resourceTypes;
    expect(typesBefore.some((item) => item.key === 'redis')).toBe(false);
    check('resource types: redis is not registered yet (the Type is registered below through the form)');
    await h.read('Chưa có loại redis trong danh mục');

    await h.type(form('ID loại tài nguyên'), 'redis');
    const outputs = [['host', 'string', true, false], ['port', 'number', true, false], ['username', 'string', true, false], ['password', 'string', true, true], ['tls', 'bool', true, false]];
    for (const [index, [name, type, required, secret]] of outputs.entries()) {
      await h.click(page.getByRole('button', { name: '+ Thêm tham số đầu ra', exact: true }));
      await h.type(form(`Đầu ra ${index + 1} tên`), name);
      if (type !== 'string') await h.choose(form(`Đầu ra ${index + 1} kiểu`), type);
      if (required) await h.click(form(`Đầu ra ${index + 1} bắt buộc`));
      if (secret) await h.click(form(`Đầu ra ${index + 1} bí mật`));
    }
    caption('Bước 3: Khai báo năm output của Redis; password được đánh dấu bí mật nên chỉ được dùng như binding, không bao giờ hiển thị giá trị');
    mark('redis-type-filled');
    await h.pause(SHORT_READ);
    await h.click(page.getByRole('button', { name: 'Đăng ký loại tài nguyên', exact: true }));
    await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký loại tài nguyên redis.' })).toBeVisible();
    const redisType = (await get('/resource-types')).resourceTypes.find((item) => item.key === 'redis');
    expect(redisType.outputs.map((item) => item.name)).toEqual(['host', 'port', 'username', 'password', 'tls']);
    expect(redisType.outputs.find((item) => item.name === 'password').secret).toBe(true);
    expect(redisType.classes).toEqual([]);
    check('GET /resource-types: redis registered with a secret password output and classes [] (no Definition serves it yet)');
    mark('redis-type-registered');
    await h.read('Đã đăng ký loại Redis');

    caption('Bước 4: Mở Cấu hình tài nguyên; biểu mẫu chỉ liệt kê module đã được registry xác minh, không có ô nhập nguồn, URL hay digest');
    await h.click(nav('Cấu hình tài nguyên'));
    await expect(page.getByRole('heading', { name: 'Cấu hình tài nguyên', level: 1, exact: true })).toBeVisible();
    const capabilities = (await get('/resource-definition-capabilities')).capabilities;
    await expect(form('Loại tài nguyên')).toBeVisible();
    await expect(page.getByLabel(/URL|digest|revision|source_id/i)).toHaveCount(0);
    check('definition form: no free-text source, URL, revision or digest input exists');
    mark('definition-form');
    await h.read('Chỉ có lựa chọn từ registry đã kiểm chứng');

    // Redis in-cluster
    caption('Bước 5: Đăng ký Redis chạy trong cluster: chọn module redis-k8s đã kiểm chứng; pin source, revision và SHA-256 do máy chủ công bố');
    await startDefinition('redis-in-cluster', 'redis', 'redis-k8s', 'Cluster nội bộ');
    const redisPin = pinOf(capabilities, 'redis-k8s');
    const pinPanel = page.locator('dl[aria-label="Pin module đã kiểm chứng"]');
    await expect(pinPanel).toContainText(redisPin.source.revision);
    await expect(pinPanel).toContainText(redisPin.source.contentSha256);
    await h.moveTo(pinPanel);
    check(`pin shown read-only equals the server metadata (${redisPin.module} ${redisPin.source.revision})`);
    mark('redis-pin');
    await h.read('Pin module đầy đủ nằm trong metadata của máy chủ');
    caption('Bước 6: Ánh xạ output bằng nút tường minh, chọn cluster của Môi trường, dùng preset namespace và gắn class “in-cluster” vào điều kiện áp dụng');
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await choose(form('Cluster đích của provider kubernetes'), 'Cluster của Môi trường');
    await h.click(page.getByRole('button', { name: 'Dùng preset cho namespace', exact: true }));
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(form('Điều kiện 1 Class'), 'in-cluster');
    await h.pause(SHORT_READ);
    await h.click(submitDefinition());
    await registered('redis-in-cluster');
    mark('redis-in-cluster-registered');
    await h.read('Đã đăng ký Redis in-cluster');

    // PostgreSQL in-cluster
    caption('Bước 7: Đăng ký PostgreSQL in-cluster từ module postgres-k8s; hai tham số database/username của Loại được ánh xạ vào biến module');
    await startDefinition('pg-in-cluster', 'postgres', 'postgres-k8s', 'Cluster nội bộ');
    await h.click(page.getByRole('button', { name: 'Ghép đầu vào theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await choose(form('Cluster đích của provider kubernetes'), 'Cluster của Môi trường');
    await h.click(page.getByRole('button', { name: 'Dùng preset cho namespace', exact: true }));
    await expect(page.getByText(/Nguồn: tham số "database" của Loại tài nguyên/)).toBeVisible();
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(form('Điều kiện 1 Class'), 'in-cluster');
    await h.moveTo(page.getByText(/Nguồn: tham số "database" của Loại tài nguyên/));
    await h.pause(SHORT_READ);
    await h.click(submitDefinition());
    await registered('pg-in-cluster');
    mark('pg-in-cluster-registered');
    await h.read('Đã đăng ký PostgreSQL in-cluster');

    // Aurora: cloud-managed, its own Driver Account and region.
    caption('Bước 8: PostgreSQL managed (Aurora): chọn Tài khoản hạ tầng và vùng AWS của CHÍNH cấu hình; không có ô Kết nối AWS, không lấy gì từ Môi trường');
    await startDefinition('pg-cloud-managed', 'postgres', 'aurora');
    await expect(page.getByLabel('Kết nối AWS')).toHaveCount(0);
    await expect(page.getByText(/mỗi cấu hình có thể dùng tài khoản và vùng khác nhau/)).toBeVisible();
    check('aurora form: own Driver Account and literal region, no AWS Connection selector, nothing inherited from the Environment');
    await h.click(page.getByRole('button', { name: 'Ghép đầu vào theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    for (const variable of ['name', 'vpc_id', 'subnet_ids', 'private_subnet_ids']) await h.click(page.getByRole('button', { name: `Dùng preset cho ${variable}`, exact: true }));
    await override('engine_version', '16.4');
    await override('min_capacity', '0.5');
    await override('max_capacity', '2');
    await expect(form('Giá trị private_subnet_ids')).toHaveValue("${resources['vpc.default#@infra'].outputs.privateSubnetIds}");
    check('aurora private_subnet_ids is mapped from privateSubnetIds of the shared VPC reference; no subnet allocator in the form');
    await h.moveTo(form('Giá trị private_subnet_ids'));
    await h.click(page.getByRole('radio', { name: /Tùy chỉnh nâng cao/ }));
    await h.type(form('Điều kiện 1 Class'), 'cloud-managed');
    await h.pause(SHORT_READ);
    await h.click(submitDefinition());
    await registered('pg-cloud-managed');
    mark('aurora-registered');
    await h.read('Đã đăng ký Aurora, chưa tạo tài nguyên nào');

    // EKS: wide CIDR is refused by the server.
    caption('Bước 9: EKS bắt buộc danh sách IPv4 /32 tường minh; thử 0.0.0.0/0 để thấy máy chủ từ chối, không thêm IP của executor và không có mặc định');
    await startDefinition('eks-wide-open', 'k8s-cluster', 'eks');
    await h.click(page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }));
    await h.click(page.getByRole('button', { name: 'Dùng preset cho name', exact: true }));
    await h.click(page.getByRole('button', { name: 'Dùng preset cho subnet_ids', exact: true }));
    await expect(page.getByRole('button', { name: 'Dùng preset cho public_access_cidrs' })).toHaveCount(0);
    await expect(form('Ghi đè public_access_cidrs')).not.toBeChecked();
    await override('kubernetes_version', '1.31');
    await override('node_ami_type', 'AL2023_x86_64_STANDARD');
    await override('node_capacity_type', 'ON_DEMAND');
    await override('node_disk_size', '20');
    await override('node_instance_type', 't3.medium');
    await h.click(page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }));
    await expect(alert()).toContainText('Cần nhập trước khi lưu: public_access_cidrs');
    check('eks: public_access_cidrs has no default or preset; the form blocks until the author lists addresses');
    await override('public_access_cidrs', '["0.0.0.0/0"]');
    await h.click(submitDefinition());
    await expect(alert()).toContainText('Máy chủ từ chối cấu hình');
    expect((await get('/resource-definitions')).resourceDefinitions.some((item) => item.key === 'eks-wide-open')).toBe(false);
    check('eks: a wide CIDR is rejected by the server (400) and nothing is stored');
    mark('eks-wide-refused');
    await h.moveTo(alert());
    await h.read('Máy chủ từ chối 0.0.0.0/0');

    // Metadata published for the Developer.
    caption('Bước 10: Quay lại Loại tài nguyên; mỗi loại hiển thị class đã công bố, được tính từ catalog và registry đã kiểm chứng');
    await h.click(nav('Loại tài nguyên'));
    const types = (await get('/resource-types')).resourceTypes;
    const classesOf = (key) => types.find((item) => item.key === key).classes;
    expect(classesOf('redis')).toEqual(['in-cluster']);
    expect(classesOf('postgres')).toEqual(['cloud-managed', 'in-cluster']);
    expect(classesOf('vpc')).toEqual([]);
    check('GET /resource-types classes: redis [in-cluster], postgres [cloud-managed, in-cluster], vpc []');
    await expect(page.locator('.catalog-entry', { hasText: 'redis' }).first()).toContainText('Class đã công bố: in-cluster');
    mark('classes-published');
    await h.read('Class được công bố theo từng loại');
    await signOut();

    // ---- Developer ---------------------------------------------------------
    caption('Bước 11: Nhà phát triển đăng nhập và tạo ứng dụng; Workload chỉ chọn loại, tham số, class và ID dùng chung, không chọn module hay thông tin xác thực');
    await signIn('developer');
    const applicationName = `Class Demo ${Date.now().toString(36).slice(-4)}`;
    const subdomain = `i0407-${Date.now().toString(36).slice(-6)}`;
    const applicationId = await createApplication(h, applicationName, subdomain);
    mark('application-created');
    await h.click(page.getByRole('button', { name: '+ Thêm Workload', exact: true }));
    await expect(page.getByRole('button', { name: 'Nhập bằng form' })).toBeVisible();
    await h.type(form('Tên Workload'), 'api');
    await h.type(form('Ảnh Container'), 'example.invalid/api:v1');
    await h.click(page.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }));
    await h.type(form('Alias tài nguyên'), 'cache');
    await choose(form('Loại tài nguyên'), 'redis');
    caption('Bước 12: Class “Cách cung cấp” lấy từ metadata của máy chủ; Mặc định giữ nguyên nghĩa default, không tự chuyển sang in-cluster hay managed');
    const cacheClass = form('Cách cung cấp (class) của cache');
    await expect(cacheClass).toHaveValue('');
    expect((await cacheClass.locator('option').allTextContents())).toEqual(['Mặc định (default)', 'Trong cluster (in-cluster)']);
    check('redis dependency: class defaults to empty (default) and offers only the published class');
    await choose(cacheClass, 'Trong cluster');
    await expect(cacheClass).toHaveValue('in-cluster');
    await h.type(form('ID dùng chung của cache'), 'shared-cache');
    mark('redis-class-chosen');
    await h.read('Redis: in-cluster, ID dùng chung shared-cache');
    await h.click(page.getByRole('button', { name: '+ Thêm tài nguyên', exact: true }));
    const aliases = page.getByLabel('Alias tài nguyên', { exact: true });
    await h.type(aliases.nth(1), 'db');
    await choose(page.locator('select[aria-label="Loại tài nguyên"]').nth(1), 'postgres');
    const dbClass = form('Cách cung cấp (class) của db');
    expect((await dbClass.locator('option').allTextContents())).toEqual(['Mặc định (default)', 'Dịch vụ managed AWS (cloud-managed)', 'Trong cluster (in-cluster)']);
    await choose(dbClass, 'Trong cluster');
    await h.type(form('Tham số database'), 'shop');
    await h.type(form('Tham số username'), 'shop');
    await expect(page.getByLabel(/module|provider|credential|xác thực/i)).toHaveCount(0);
    check('developer editor: no Terraform module, provider or credential control; class and shared ID only');
    mark('db-class-chosen');
    await h.pause(SHORT_READ);
    caption('Bước 13: Lưu draft; việc lưu không xem trước và không triển khai');
    await h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
    await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
    const row = (name) => page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: new RegExp(`${name}$`) }) });
    await expect(row('api')).toContainText('Chờ thay đổi');
    const deploymentsBefore = (await get(`/applications/${applicationId}/environments/staging/deployments`)).deployments ?? [];
    expect(deploymentsBefore).toHaveLength(0);
    check('draft saved as pending; no Deployment exists (Save never previews or deploys)');
    mark('draft-saved');
    await h.read('Draft đã lưu, chưa có triển khai');

    // No-match guard: there is no rendering template yet.
    caption('Bước 14: Xem trước khi chưa có Mẫu dựng ứng dụng; hệ thống chặn trước khi tạo tài nguyên và không dùng renderer mặc định');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByRole('alert').filter({ hasText: 'Chưa có Mẫu dựng ứng dụng (score-k8s)' })).toBeVisible();
    await expect(page.getByLabel('Xem trước triển khai')).toHaveCount(0);
    check('preview without a rendering template is refused with renderer-no-match; no Deploy action is offered');
    mark('no-template-guard');
    await h.moveTo(page.getByRole('alert').first());
    await h.read('Chưa có mẫu dựng ứng dụng nên Xem trước bị chặn');
    await signOut();

    // ---- Platform Engineer registers the rendering template ---------------
    caption('Bước 15: Kỹ sư nền tảng đăng ký đúng một Mẫu dựng ứng dụng score-k8s cho phạm vi cluster nội bộ; bundle lấy từ danh sách đã cài');
    await signIn('platform-engineer');
    await h.click(nav('Mẫu dựng ứng dụng'));
    await expect(page.getByRole('heading', { name: 'Mẫu dựng ứng dụng', level: 1, exact: true })).toBeVisible();
    await h.type(form('ID mẫu'), 'render-internal');
    await h.type(form('Tên hiển thị'), 'Mẫu dựng cluster nội bộ');
    await expect(form('Bundle dựng ứng dụng')).toHaveValue('score-k8s-internal-v1');
    await h.click(page.getByRole('button', { name: 'Đăng ký mẫu dựng ứng dụng', exact: true }));
    await expect(page.getByRole('status').filter({ hasText: 'Đã đăng ký mẫu dựng ứng dụng render-internal.' })).toBeVisible();
    check('rendering template registered for internal-k8s from the installed bundle list (the real pinned score-k8s)');
    mark('template-registered');
    await h.read('Đã đăng ký mẫu dựng ứng dụng');
    await signOut();

    // ---- Developer previews ------------------------------------------------
    caption('Bước 16: Nhà phát triển xem trước lại; kết quả hiển thị loại/class, cấu hình, module·revision, đích, quyền sở hữu và ý định, không có tham số hay bí mật');
    await signIn('developer');
    await h.click(page.getByRole('button', { name: new RegExp(applicationName) }).first());
    await expect(page.getByRole('heading', { name: applicationName })).toBeVisible();
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    const panel = page.getByLabel('Xem trước triển khai');
    await expect(panel).toContainText('trình dựng: score-k8s 0.15.0 (mẫu render-internal)');
    const table = panel.getByRole('table');
    const redisRow = table.locator('tbody tr').filter({ hasText: 'redis /' });
    await expect(redisRow).toContainText('in-cluster');
    await expect(redisRow).toContainText('redis-in-cluster');
    await expect(redisRow).toContainText(`redis-k8s · ${redisPin.source.revision}`);
    await expect(redisRow).toContainText(`sha256:${redisPin.source.contentSha256.slice(0, 12)}`);
    await expect(redisRow).toContainText('Tạo mới');
    await expect(redisRow).toContainText('Orchestrator');
    const pgRow = table.locator('tbody tr').filter({ hasText: 'postgres /' });
    await expect(pgRow).toContainText('pg-in-cluster');
    const previewText = await panel.innerText();
    for (const pattern of FORBIDDEN_IN_PREVIEW) expect(previewText).not.toMatch(pattern);
    expect(previewText).not.toContain('shop'); // Score param values are not part of the projection
    check('preview: renderer, Definition, module revision/digest, ownership and intent are shown; no param, secret, credential or state text');
    await expect(page.getByLabel('Đích triển khai').first()).toContainText('internal-cluster');
    mark('preview-pinned');
    await h.moveTo(panel);
    await h.read('Kết quả xem trước đã ghim module, đích và trình dựng');
    const afterPreview = (await get(`/applications/${applicationId}/environments/staging/deployments`)).deployments ?? [];
    expect(afterPreview).toHaveLength(0);
    check('preview created no Deployment (it is pure; no Terraform, score-k8s or provider call)');

    // Wrong class: never switched, never silently mapped.
    caption('Bước 17: Gõ class tùy chỉnh “cloud-managed” cho Redis (chưa có cấu hình nào phục vụ); Xem trước báo không khớp, không tự đổi class hay Definition');
    await h.click(row('api').getByRole('button', { name: 'Sửa', exact: true }));
    await expect(page.getByRole('heading', { name: 'Sửa api' })).toBeVisible();
    await h.click(page.getByRole('button', { name: 'Hiện nâng cao tài nguyên cache', exact: true }));
    await h.type(form('Lớp tài nguyên'), 'cloud-managed', { replace: true });
    await expect(form('Cách cung cấp (class) của cache')).toHaveValue('cloud-managed');
    await h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
    await expect(row('api')).toContainText('Chờ thay đổi');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByRole('alert').filter({ hasText: 'Không dựng hoặc khớp được đồ thị tài nguyên' })).toBeVisible();
    await expect(page.getByLabel('Xem trước triển khai')).toHaveCount(0);
    check('custom class cloud-managed for redis: no Definition matches, preview refused (graph stage), no Deploy offered; matching stays server-authoritative');
    mark('wrong-class-guard');
    await h.moveTo(page.getByRole('alert').first());
    await h.read('Class sai nên Xem trước bị chặn, class không bị tự đổi');
    await h.click(row('api').getByRole('button', { name: 'Sửa', exact: true }));
    await expect(form('Cách cung cấp (class) của cache')).toHaveValue('cloud-managed');
    check('the editor still shows cloud-managed: the class was neither switched nor rewritten');

    // Metadata error: classes unavailable.
    caption('Bước 18: Metadata class không tải được (mô phỏng lỗi mạng đọc /resource-types); lựa chọn class bị khóa, Lưu bị chặn và mọi chỉnh sửa được giữ nguyên');
    await h.type(form('Ảnh Container'), 'example.invalid/api:v2', { replace: true });
    let failMetadata = true;
    await page.route('**/api/v1/resource-types', async (route) => {
      if (failMetadata) await route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ code: 'INTERNAL_ERROR' }) });
      else await route.continue();
    });
    await h.click(page.getByRole('button', { name: 'Làm mới metadata class', exact: true }));
    const metadataAlert = page.getByRole('alert', { name: 'Lỗi tải metadata class' });
    await expect(metadataAlert).toBeVisible();
    await expect(form('Cách cung cấp (class) của cache')).toBeDisabled();
    await expect(form('Cách cung cấp (class) của cache')).toHaveValue('cloud-managed');
    await expect(form('Ảnh Container')).toHaveValue('example.invalid/api:v2');
    await h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
    await expect(page.getByText(/Chưa xác nhận được metadata class của tài nguyên/)).toBeVisible();
    check('class metadata failure (FAKE network interception): class select locked, Save blocked, edits kept');
    mark('metadata-error-guard');
    await h.moveTo(metadataAlert);
    await h.read('Metadata lỗi nên khóa lựa chọn class nhưng giữ chỉnh sửa');
    failMetadata = false;
    await h.click(metadataAlert.getByRole('button', { name: 'Thử lại tải metadata class' }));
    await expect(form('Cách cung cấp (class) của cache')).toBeEnabled();
    await choose(form('Cách cung cấp (class) của cache'), 'Trong cluster');
    await expect(form('Cách cung cấp (class) của cache')).toHaveValue('in-cluster');
    await expect(form('Ảnh Container')).toHaveValue('example.invalid/api:v2');
    mark('metadata-recovered');
    await page.unroute('**/api/v1/resource-types');
    await h.click(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }));
    await expect(row('api')).toContainText('Chờ thay đổi');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    await expect(page.getByLabel('Xem trước triển khai')).toContainText('mẫu render-internal');
    const final = (await get(`/applications/${applicationId}/environments/staging/deployments`)).deployments ?? [];
    expect(final).toHaveLength(0);
    check('after retry the class is chosen again explicitly, saved and previewed; still no Deployment (nothing was deployed in this run)');
    mark('preview-final');
    await h.read('Xem trước lại thành công; không triển khai trong lần chạy này');

    // ---- Standalone Score Preview (UC-05 BR-10) -----------------------------
    caption('Bước 20: Xem trước Score độc lập cũng hiển thị tài nguyên hạ tầng an toàn: loại/class, module·revision, đích, quyền sở hữu và ý định; không có tham số hay bí mật');
    await h.click(page.getByRole('button', { name: 'Xem trước Score', exact: true }));
    await h.type(page.getByLabel('ID Workload', { exact: true }), 'api');
    await h.type(page.getByLabel('ID lần chạy', { exact: true }), 'standalone-1');
    await h.paste(page.getByLabel('Score sau (YAML hoặc JSON)', { exact: true }), JSON.stringify({ apiVersion: 'score.dev/v1b1', metadata: { name: 'api' }, containers: { main: { image: 'example.invalid/api:v1' } }, resources: { cache: { type: 'redis', class: 'in-cluster' }, db: { type: 'postgres', class: 'in-cluster', params: { database: 'shop', username: 'shop' } } } }));
    await h.click(page.getByRole('button', { name: 'Xem trước', exact: true }));
    const standalone = page.getByRole('region', { name: 'Kết quả xem trước Score' });
    const intents = standalone.getByLabel('Tài nguyên hạ tầng sẽ được xử lý');
    await expect(intents).toContainText(`redis-k8s · ${redisPin.source.revision}`);
    await expect(intents).toContainText('Tạo mới');
    await expect(intents).toContainText('Orchestrator');
    await expect(standalone.getByLabel('Đích đã ghim của xem trước')).toContainText('internal-cluster');
    const standaloneText = await standalone.innerText();
    for (const pattern of FORBIDDEN_IN_PREVIEW) expect(standaloneText).not.toMatch(pattern);
    expect(await intents.innerText()).not.toContain('shop');
    check('standalone Score Preview shows the same safe resource intents, module pin, pinned target and ownership as the pending Preview; no param, secret or credential text');
    mark('standalone-intents');
    await h.moveTo(intents);
    await h.read('Xem trước Score độc lập hiển thị đúng ý định tài nguyên và đích đã ghim');
    expect(((await get(`/applications/${applicationId}/environments/staging/deployments`)).deployments ?? [])).toHaveLength(0);
    check('the standalone Preview created no Deployment either');

    // ---- Persisted attempt progress (SEEDED, not executed) ------------------
    caption('Bước 21: Tiến trình đã lưu của một lần triển khai: giai đoạn, lần chạy và trạng thái an toàn. Dữ liệu được ghi sẵn (không chạy Terraform); văn bản lạ trong CSDL không bao giờ hiện ra');
    const seeded = JSON.parse(execFileSync(process.env.ORCH_E2E_DEFWORK, ['seed-progress', '-private-dir', process.env.ORCH_E2E_PRIVATE_DIR, '-application', applicationId, '-environment', 'staging'], { encoding: 'utf8' }));
    check('seeded three Deployments with persisted attempts through the real Console and State stores (FAKE source: nothing was executed; one row carries hostile text written as the database owner)');
    await h.click(page.getByRole('button', { name: /^←/ }).first());
    await expect(page.getByRole('button', { name: 'Xem tất cả', exact: true })).toBeVisible();
    await h.click(page.getByRole('button', { name: 'Xem tất cả', exact: true }));
    const entries = page.locator('button.deployment-entry');
    await expect(entries).toHaveCount(3);
    // Newest first: no-attempts, hostile, good. Error handling first: the detail read fails once (FAKE interception).
    let failOnce = true;
    await page.route(`**/deployments/${seeded.good}`, async (route) => { if (failOnce) { failOnce = false; await route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"internal AKIAPRIVATE detail"}' }); } else await route.continue(); });
    await h.click(entries.nth(2));
    await expect(page.getByRole('alert')).toContainText('Không thể tải chi tiết triển khai.');
    await expect(page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' })).toHaveCount(0);
    expect(await page.locator('body').innerText()).not.toContain('AKIAPRIVATE');
    check('detail read failure: safe Vietnamese error with retry, no progress rows, no raw server text');
    await h.click(page.getByRole('alert').getByRole('button', { name: 'Thử lại', exact: true }));
    const progress = page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' });
    await expect(progress.locator('tbody tr')).toHaveCount(2);
    await expect(progress).toContainText('redis-k8s');
    await expect(progress).toContainText('Sẵn sàng');
    await expect(progress).toContainText('Thành công');
    await expect(progress).toContainText('Quá thời gian');
    mark('progress-shown');
    await h.moveTo(progress);
    await h.read('Mỗi tài nguyên hiển thị giai đoạn, lần chạy, trạng thái và mã lỗi an toàn đã lưu');
    // Reopen: a full page reload re-reads the persisted attempts.
    await page.reload();
    await expect(page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' }).locator('tbody tr')).toHaveCount(2);
    check('reopen: a full reload re-reads the same persisted attempts');
    await h.click(page.getByRole('button', { name: /^← Lịch sử/ }));
    await h.click(page.locator('button.deployment-entry').nth(1));
    const hostile = page.getByRole('region', { name: 'Tiến trình tạo tài nguyên' });
    await expect(hostile).toContainText('Giai đoạn không rõ');
    await expect(hostile).toContainText('Lỗi không xác định');
    const hostileHtml = await page.content();
    for (const marker of ['AKIA', 'PRIVATE', '/var/lib', 'vault://', 'seeded-private-handle']) expect(hostileHtml).not.toContain(marker);
    check('hostile persisted stage/error text is replaced by generic Vietnamese labels; no credential, path or handle marker reaches the page');
    mark('progress-hostile-safe');
    await h.read('Văn bản lạ trong cơ sở dữ liệu chỉ hiện thành nhãn chung, không lộ nội dung');
    await h.click(page.getByRole('button', { name: /^← Lịch sử/ }));
    await h.click(page.locator('button.deployment-entry').nth(0));
    await expect(page.getByText('Chưa có bước thực thi Terraform nào được ghi cho lần triển khai này.')).toBeVisible();
    check('a Deployment without attempts says so; it is not an unavailable projection');
    mark('progress-empty');
    await signOut();
    mark('signed-out');
    caption('Bước 22: Đã đăng xuất; kết thúc kịch bản. Không có gì được triển khai trong lần chạy này');
    await page.waitForTimeout(7000); // capture can start late under load: the recording must clearly outlast the last mark

    writeFileSync(`${evidenceDir}/scenario.json`, JSON.stringify({ applicationId, registeredPins: { redis: redisPin.source, postgresK8s: pinOf(capabilities, 'postgres-k8s').source }, types: classesOf('postgres'), progress: seeded }, null, 2));
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
    await scenario({ page, h, x11, caption: captions.caption, mark, check, baseURL, evidenceDir });
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
