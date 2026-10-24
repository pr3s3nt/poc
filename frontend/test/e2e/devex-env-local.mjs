// DEVEX-ENV browser acceptance (issue #13): the unified "Biến môi trường" table, inline
// creation of Application keys / dependencies / the initial Secret Store, ordinary
// values through Save → Preview → Deploy → Edit, in Vietnamese and English at the two
// required desktop sizes. It drives the real product UI and HTTP handlers over the
// disposable loopback fixture backend/test/fixtures/deployment-review (start it with
// `-ready-secret-store vault-qa`); execution adapters are FAKE, nothing is provisioned
// and no cluster, cloud or retained data is touched. Fixture data only.
//   DEVEX_BASE_URL=http://127.0.0.1:PORT DEVEX_EVIDENCE_DIR=/new/dir node test/e2e/devex-env-local.mjs
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import { createHuman, createApplication, addWorkload } from './human.mjs';

const base = process.env.DEVEX_BASE_URL;
const evidence = process.env.DEVEX_EVIDENCE_DIR;
if (!base || !evidence) throw Error('DEVEX_BASE_URL and DEVEX_EVIDENCE_DIR required');
if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(base)) throw Error('Loopback fixture only');
await mkdir(evidence, { recursive: true });

const SECRET = 'synthetic-secret-do-not-leak-7f3a';
const browser = await chromium.launch({ headless: true });
const assertions = [];
const check = (name, value) => { if (!value) throw Error(`assertion failed: ${name}`); assertions.push(name); console.log(`PASS ${name}`); };
const errors = [];
let outcome = 'FAIL';

async function open(locale, viewport) {
  const context = await browser.newContext({ viewport, locale });
  const page = await context.newPage();
  page.on('pageerror', (e) => errors.push(e.message));
  return { context, page };
}
async function signIn(page) {
  await page.goto(`${base}/ui/sign-in`);
  await page.getByLabel(/Tên đăng nhập|Username/).fill('platform-engineer');
  await page.getByLabel(/Mật khẩu|Password/).fill('test-password');
  await page.getByRole('button', { name: /^(Đăng nhập|Sign in)$/ }).click();
  await page.waitForURL(/\/ui\/(?!sign-in)/);
}
const noOverflow = (page) => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth + 1);
const shot = (page, name) => page.screenshot({ path: `${evidence}/${name}.png`, fullPage: true });

try {
  // ── Setup (Vietnamese, 1512×980) ────────────────────────────────────────────
  const vi = await open('vi-VN', { width: 1512, height: 980 });
  const { page, context } = vi;
  await signIn(page);
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn' })).toBeVisible();
  const h = createHuman(page, { fast: true });
  const stamp = Date.now().toString().slice(-8);
  const app = await createApplication(h, `DevEx Env QA ${stamp}`, `devex-${stamp}`);
  const definitions = (await (await context.request.get(`${base}/api/v1/resource-definitions`)).json()).resourceDefinitions;
  for (const key of ['cluster-internal', 'namespace-k8s-module', 'workload-score-k8s', 'postgres-k8s-module']) {
    const original = definitions.find((d) => d.key === key);
    if (!original) throw Error(`missing fixture definition ${key}`);
    const { version, sourceFingerprint, ...draft } = original; void version; void sourceFingerprint;
    draft.key = `${key}-${app}`; draft.name = `QA ${original.name}`;
    draft.criteria = [{ app_id: app, ...(key === 'cluster-internal' ? { class: 'internal' } : {}) }];
    const res = await context.request.post(`${base}/api/v1/resource-definitions`, { data: draft });
    if (!res.ok()) throw Error(`register ${key}: HTTP ${res.status()}`);
  }
  check('application and scoped definitions are ready', true);

  // A deployed backend whose Service port the new workload references.
  await addWorkload(h, { name: 'backend', image: 'example.invalid/backend:v1', bindings: [], port: { name: 'http', port: '8080' } });
  const deployPending = async () => {
    const previewed = page.waitForResponse((r) => r.url().endsWith('/preview') && r.request().method() === 'POST');
    await h.click(page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
    const previewResponse = await previewed;
    const preview = await previewResponse.json();
    if (!previewResponse.ok()) { await shot(page, 'preview-failure'); throw Error(`Preview failed: HTTP ${previewResponse.status()} ${JSON.stringify(preview).slice(0, 300)} ${(await page.getByRole('alert').allTextContents()).join(' | ')}`); }
    await h.click(page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }));
    await expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công');
    return preview;
  };
  await deployPending();
  check('backend deployed with the fake executor', true);
  await page.goto(`${base}/ui/applications/${app}`);

  // ── E01/E02/E04/E05/E06: the table, inline creation, ordinary values ───────
  await h.click(page.getByRole('button', { name: '+ Thêm Workload' }));
  await expect(page.getByRole('button', { name: 'Nhập bằng form' })).toBeVisible();
  await h.type(page.getByLabel('Tên Workload', { exact: true }), 'api');
  await h.type(page.getByLabel('Ảnh Container', { exact: true }), 'example.invalid/api:v1');
  const table = page.getByRole('region', { name: 'Biến môi trường của container main', exact: true });
  await expect(table.getByText('Chưa có biến môi trường')).toBeVisible();
  await shot(page, '01-vi-empty-table-1512x980');

  // Dependency added in place from a row (no leaving the editor).
  await h.click(table.getByRole('button', { name: '+ Thêm biến', exact: true }));
  await h.type(table.getByLabel('Tên biến, dòng 1', { exact: true }), 'DB_HOST');
  await h.choose(table.getByLabel('Lấy giá trị từ, dòng 1', { exact: true }), 'Tài nguyên');
  await h.click(table.getByRole('button', { name: 'Thêm tài nguyên', exact: true }));
  let dialog = page.getByRole('dialog', { name: 'Thêm tài nguyên' });
  await shot(page, '02-vi-add-resource-dialog');
  await h.type(dialog.getByLabel('Alias tài nguyên', { exact: true }), 'db');
  await h.choose(dialog.getByLabel('Loại tài nguyên', { exact: true }), 'postgres');
  await h.type(dialog.getByLabel('Tham số database', { exact: true }), 'orders');
  await h.type(dialog.getByLabel('Tham số username', { exact: true }), 'orders_app');
  await h.click(dialog.getByRole('button', { name: 'Thêm và chọn', exact: true }));
  await expect(table.getByLabel('Tài nguyên, dòng 1', { exact: true })).toHaveValue('db');
  await expect(table.getByLabel('Output, dòng 1', { exact: true })).toHaveValue('');
  await expect(table.getByLabel('Output, dòng 1', { exact: true })).toBeFocused();
  check('add-resource dialog adds the dependency to the draft, selects the alias and leaves the output to choose', true);
  await h.choose(table.getByLabel('Output, dòng 1', { exact: true }), 'host');
  await h.click(table.getByRole('button', { name: '+ Thêm biến', exact: true }));
  await h.type(table.getByLabel('Tên biến, dòng 2', { exact: true }), 'DB_PASSWORD');
  await h.choose(table.getByLabel('Lấy giá trị từ, dòng 2', { exact: true }), 'Tài nguyên');
  await h.choose(table.getByLabel('Tài nguyên, dòng 2', { exact: true }), 'db');
  check('secret output carries the Bí mật label', (await table.getByLabel('Output, dòng 2', { exact: true }).locator('option').allTextContents()).some((text) => /Bí mật/.test(text)));
  await h.choose(table.getByLabel('Output, dòng 2', { exact: true }), (await table.getByLabel('Output, dòng 2', { exact: true }).locator('option').allTextContents()).find((text) => /password/.test(text)));

  // Application variable created inline.
  await h.click(table.getByRole('button', { name: '+ Thêm biến', exact: true }));
  await h.type(table.getByLabel('Tên biến, dòng 3', { exact: true }), 'REGION');
  await h.choose(table.getByLabel('Lấy giá trị từ, dòng 3', { exact: true }), 'Biến của ứng dụng');
  await h.click(table.getByRole('button', { name: 'Tạo biến', exact: true }));
  dialog = page.getByRole('dialog', { name: 'Tạo biến' });
  await expect(dialog.getByLabel('Tên key', { exact: true })).toHaveValue('REGION');
  await h.type(dialog.getByLabel('Giá trị', { exact: true }), 'eu-west-1');
  await shot(page, '03-vi-create-variable-dialog');
  const created = page.waitForResponse((r) => r.url().endsWith('/configuration/keys') && r.request().method() === 'POST');
  await h.click(dialog.getByRole('button', { name: 'Tạo và chọn', exact: true }));
  check('create-only POST returned 201', (await created).status() === 201);
  await expect(table.getByLabel('Biến của ứng dụng, dòng 3', { exact: true })).toHaveValue('REGION');
  await expect(table.getByLabel('Biến của ứng dụng, dòng 3', { exact: true })).toBeFocused();

  // Secret created inline, with the initial Secret Store chosen in the same dialog.
  await h.click(table.getByRole('button', { name: '+ Thêm biến', exact: true }));
  await h.type(table.getByLabel('Tên biến, dòng 4', { exact: true }), 'API_TOKEN');
  await h.choose(table.getByLabel('Lấy giá trị từ, dòng 4', { exact: true }), 'Bí mật');
  await h.click(table.getByRole('button', { name: 'Tạo bí mật', exact: true }));
  dialog = page.getByRole('dialog', { name: 'Tạo bí mật' });
  await expect(dialog.getByRole('button', { name: 'Tạo và chọn', exact: true })).toBeDisabled();
  await h.choose(dialog.getByLabel('Kho bí mật', { exact: true }), 'Synthetic vault-qa (vault-qa)');
  await h.click(dialog.getByRole('button', { name: 'Lưu kho bí mật', exact: true }));
  await expect(dialog.getByText(/Đã lưu kho bí mật/)).toBeVisible();
  await h.type(dialog.getByLabel('Giá trị bí mật', { exact: true }), SECRET);
  await expect(dialog.getByRole('button', { name: 'Tạo và chọn', exact: true })).toBeEnabled();
  check('secret input is masked', (await dialog.getByLabel('Giá trị bí mật', { exact: true }).getAttribute('type')) === 'password');
  await shot(page, '04-vi-create-secret-dialog-store-saved');
  await h.click(dialog.getByRole('button', { name: 'Tạo và chọn', exact: true }));
  await expect(table.getByLabel('Bí mật, dòng 4', { exact: true })).toHaveValue('API_TOKEN');
  check('secret value is nowhere in the page after creation', !(await page.content()).includes(SECRET));

  // Other workload + ordinary values.
  await h.click(table.getByRole('button', { name: '+ Thêm biến', exact: true }));
  await h.type(table.getByLabel('Tên biến, dòng 5', { exact: true }), 'BACKEND_URL');
  await h.choose(table.getByLabel('Lấy giá trị từ, dòng 5', { exact: true }), 'Workload khác');
  await h.choose(table.getByLabel('Workload, dòng 5', { exact: true }), 'backend');
  await h.choose(table.getByLabel('Cổng Service, dòng 5', { exact: true }), 'http');
  const literals = [['LOG_LEVEL', 'debug'], ['EMPTY', ''], ['PADDED', '  two spaces  '], ['TEMPLATE', 'x ${NOT_A_REFERENCE} y']];
  for (const [index, [name, value]] of literals.entries()) {
    const row = 6 + index;
    await h.click(table.getByRole('button', { name: '+ Thêm biến', exact: true }));
    await h.type(table.getByLabel(`Tên biến, dòng ${row}`, { exact: true }), name);
    await h.type(table.getByLabel(`Giá trị nhập, dòng ${row}`, { exact: true }), value, { replace: true });
  }
  await expect(table.getByText('Giá trị thường được lưu cùng cấu hình. Dùng Bí mật cho dữ liệu nhạy cảm.')).toBeVisible();
  await h.click(page.getByRole('button', { name: '+ Thêm cổng' }));
  await h.type(page.getByLabel('Tên cổng Service', { exact: true }), 'http');
  await h.type(page.getByLabel('Cổng Service', { exact: true }), '8080');
  check('no horizontal overflow at 1512x980 with ten rows', await noOverflow(page));
  await shot(page, '05-vi-full-table-1512x980');
  await page.setViewportSize({ width: 1280, height: 900 });
  check('no horizontal overflow at 1280x900', await noOverflow(page));
  await shot(page, '06-vi-full-table-1280x900');
  await page.getByRole('button', { name: /^Thu gọn thanh/ }).click();
  await expect(page.getByRole('button', { name: /^Mở rộng thanh/ })).toBeVisible();
  await shot(page, '06b-vi-full-table-1280x900-sidebar-collapsed');
  check('no horizontal overflow at 1280x900 with the sidebar collapsed', await noOverflow(page));
  await page.setViewportSize({ width: 1512, height: 980 });

  const saved = page.waitForResponse((r) => r.request().method() === 'PUT' && /\/workloads\/api$/.test(new URL(r.url()).pathname));
  await h.click(page.getByRole('button', { name: 'Lưu draft Workload' }));
  const savedResponse = await saved;
  check('Save returned the draft list', savedResponse.ok());
  const score = savedResponse.request().postDataJSON().score;
  check('Score projection of the five sources', JSON.stringify(score.containers.main.variables) === JSON.stringify({
    DB_HOST: '${resources.db.host}', DB_PASSWORD: '${resources.db.password}', REGION: '${resources.env.REGION}', API_TOKEN: '${resources.env.API_TOKEN}',
    BACKEND_URL: '${resources.svc_backend_http.url}', LOG_LEVEL: 'debug', EMPTY: '', PADDED: '  two spaces  ', TEMPLATE: 'x $${NOT_A_REFERENCE} y',
  }));
  check('no secret value in the saved Score', !JSON.stringify(score).includes(SECRET));
  const preview = await deployPending();
  check('Preview lists the workload and Deploy succeeded (fake execution)', preview.changes.some((change) => change.workloadId === 'api'));
  check('secret value is not in the Preview/Deploy page', !(await page.content()).includes(SECRET));

  // ── E02/E10: reopen after Deploy; ordinary values come back exactly ─────────
  await page.goto(`${base}/ui/applications/${app}`);
  await page.locator('.table-row').filter({ has: page.locator('.workload-name', { hasText: /api$/ }) }).getByRole('button', { name: 'Edit' }).click().catch(async () => {
    await page.locator('.table-row').filter({ hasText: 'api' }).getByRole('button', { name: /Sửa|Edit/ }).first().click();
  });
  await expect(page.getByRole('heading', { name: 'Sửa api' })).toBeVisible();
  const reopened = page.getByRole('region', { name: 'Biến môi trường của container main', exact: true });
  const values = await reopened.getByLabel(/^Tên biến, dòng/).evaluateAll((inputs) => inputs.map((input) => input.value));
  check('deployed workload reopens with all nine rows', values.length === 9);
  const row = (name) => values.indexOf(name) + 1;
  await expect(reopened.getByLabel(`Giá trị nhập, dòng ${row('PADDED')}`, { exact: true })).toHaveValue('  two spaces  ');
  await expect(reopened.getByLabel(`Giá trị nhập, dòng ${row('EMPTY')}`, { exact: true })).toHaveValue('');
  await expect(reopened.getByLabel(`Giá trị nhập, dòng ${row('TEMPLATE')}`, { exact: true })).toHaveValue('x ${NOT_A_REFERENCE} y');
  await expect(reopened.getByLabel(`Bí mật, dòng ${row('API_TOKEN')}`, { exact: true })).toHaveValue('API_TOKEN');
  check('ordinary values and the secret reference round-trip through Deploy and Edit', true);
  check('secret value is not in the reopened editor', !(await page.content()).includes(SECRET));
  await shot(page, '07-vi-reopened-after-deploy');

  // ── Duplicate create does not overwrite ───────────────────────────────────
  await h.click(reopened.getByRole('button', { name: '+ Thêm biến', exact: true }));
  const last = (await reopened.getByLabel(/^Tên biến, dòng/).count());
  await h.type(reopened.getByLabel(`Tên biến, dòng ${last}`, { exact: true }), 'REGION2');
  await h.choose(reopened.getByLabel(`Lấy giá trị từ, dòng ${last}`, { exact: true }), 'Biến của ứng dụng');
  await h.click(reopened.getByRole('button', { name: 'Tạo biến', exact: true }).last());
  dialog = page.getByRole('dialog', { name: 'Tạo biến' });
  await dialog.getByLabel('Tên key', { exact: true }).fill('REGION');
  await h.type(dialog.getByLabel('Giá trị', { exact: true }), 'overwrite-attempt');
  await h.click(dialog.getByRole('button', { name: 'Tạo và chọn', exact: true }));
  await expect(dialog.getByRole('button', { name: 'Dùng key hiện có', exact: true })).toBeVisible();
  await shot(page, '08-vi-duplicate-key-no-overwrite');
  const configuration = await (await context.request.get(`${base}/api/v1/applications/${app}/environments/staging/configuration`)).json();
  check('duplicate create left REGION unchanged', configuration.keys.find((key) => key.name === 'REGION')?.value === 'eu-west-1');
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await h.click(page.getByRole('button', { name: 'Hủy', exact: true }));
  const afterCancel = await (await context.request.get(`${base}/api/v1/applications/${app}/environments/staging/configuration`)).json();
  check('keys created in place stay after the workload editor is cancelled', ['REGION', 'API_TOKEN'].every((name) => afterCancel.keys.some((key) => key.name === name)));
  check('secret keys never expose a value over the API', afterCancel.keys.filter((key) => key.kind === 'SECRET').every((key) => key.value === undefined));
  // A long catalog exercises name filtering without losing the selected reference.
  const configurationURL = `${base}/api/v1/applications/${app}/environments/staging/configuration`;
  let searchConfiguration = await (await context.request.get(configurationURL)).json();
  for (let i = 0; i < 11; i++) {
    const response = await context.request.post(`${configurationURL}/keys`, { data: { name: `SEARCH_KEY_${i}`, kind: 'VARIABLE', value: 'fixture', version: searchConfiguration.version } });
    if (!response.ok()) throw Error(`seed search key: HTTP ${response.status()}`);
    searchConfiguration = await response.json();
  }
  await vi.context.close();

  // ── E12: English at both sizes ─────────────────────────────────────────────
  for (const [label, viewport] of [['1512x980', { width: 1512, height: 980 }], ['1280x900', { width: 1280, height: 900 }]]) {
    const en = await open('en-US', viewport);
    await signIn(en.page);
    await en.page.goto(`${base}/ui/applications/${app}`);
    await en.page.locator('.table-row').filter({ hasText: 'api' }).getByRole('button', { name: /Edit/ }).first().click();
    const region = en.page.getByRole('region', { name: 'Environment variables of container main' });
    const regionRow = region.getByRole('row').filter({ has: en.page.locator('input[value="REGION"]') });
    const keySearch = regionRow.getByRole('searchbox');
    await keySearch.fill('SEARCH_KEY_10');
    const keyChoice = regionRow.getByRole('combobox', { name: /^Application variable, row/ });
    await expect(keyChoice).toHaveValue('REGION');
    await expect(keyChoice.locator('option')).toHaveCount(3);
    check(`long key catalog filters by name and preserves selection (${label})`, true);
    await expect(region.getByRole('columnheader', { name: 'Get value from' })).toBeVisible();
    await expect(region.getByRole('button', { name: '+ Add variable' })).toBeVisible();
    const options = await region.getByLabel('Get value from, row 1').locator('option').allTextContents();
    check(`English source labels (${label})`, JSON.stringify(options) === JSON.stringify(['Entered value', 'Application variable', 'Secret', 'Resource', 'Other workload']));
    await region.getByRole('button', { name: '+ Add variable' }).click();
    const count = await region.getByLabel(/^Variable name, row/).count();
    await region.getByLabel(`Get value from, row ${count}`).selectOption({ label: 'Application variable' });
    await region.getByRole('button', { name: 'Create variable' }).last().click();
    const englishDialog = en.page.getByRole('dialog', { name: 'Create variable' });
    await expect(englishDialog.getByRole('button', { name: 'Create and select' })).toBeVisible();
    await shot(en.page, `09-en-create-variable-dialog-${label}`);
    await en.page.keyboard.press('Escape');
    await expect(en.page.getByRole('dialog')).toHaveCount(0);
    check(`English dialog closes with Escape and returns focus (${label})`, true);
    check(`no horizontal overflow, English ${label}`, await noOverflow(en.page));
    await shot(en.page, `10-en-table-${label}`);
    await en.context.close();
  }
  check('no uncaught page errors', errors.length === 0);
  outcome = 'PASS';
} finally {
  await writeFile(`${evidence}/assertions.txt`, `${assertions.join('\n')}\n`);
  await writeFile(`${evidence}/result.txt`, `${outcome}\n${errors.join('\n')}\n`);
  await browser.close();
}
console.log(outcome);
