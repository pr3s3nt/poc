// Focused browser E2E: real HTTPS Git fetch/inspection and Definition HTTP
// create/edit/reload. The Go harness owns all fixtures; no deployment is made.
import { chromium, expect } from '@playwright/test';

// The shared source chooser of the form (UC-03 UI11-02): open the parameter card, type a literal, apply.
async function provideLiteral(page, name, text) {
  await page.getByRole('button', { name: new RegExp(`^(Cung cấp giá trị|Thay đổi) ${name}$`) }).click();
  const dialog = page.getByRole('dialog', { name: 'Chọn nguồn giá trị' });
  await dialog.getByRole('radio', { name: 'Nhập giá trị', exact: true }).check();
  await dialog.getByLabel(`Giá trị ${name}`, { exact: true }).fill(text);
  await dialog.getByRole('button', { name: 'Áp dụng', exact: true }).click();
}

const base = process.env.GIT_MODULE_CONSOLE_URL;
const source = process.env.GIT_MODULE_SOURCE_URL;
const commit = process.env.GIT_MODULE_COMMIT;
if (!base || !source || !commit) throw new Error('Run with the gitmodulebrowser Go harness');
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } });
  await page.goto(`${base}/ui/platform/resource-definitions`);
  await page.getByLabel(/Tên đăng nhập/).fill('platform-engineer');
  await page.getByLabel(/Mật khẩu/).fill('test-password');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await page.waitForURL('**/ui/applications');
  await page.goto(`${base}/ui/platform/resource-definitions`);
  await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  await page.getByLabel('ID cấu hình (Definition ID)', { exact: true }).fill('git-demo-definition');
  await page.getByLabel('Tên hiển thị', { exact: true }).fill('Git module demo');
  await page.getByRole('combobox', { name: /^Loại tài nguyên/ }).selectOption('git-demo');
  await expect(page.getByRole('button', { name: 'Thêm từ Git', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await page.getByLabel('Git HTTPS URL', { exact: true }).fill(source);
  await page.getByLabel('Thư mục module', { exact: true }).fill('modules');
  await page.getByLabel('Tag hoặc commit', { exact: true }).fill('v1.0.0');
  const inspected = page.waitForResponse((response) => response.url().endsWith('/terraform-modules/inspect'));
  await page.getByRole('button', { name: 'Kiểm tra module', exact: true }).click();
  const inspectResponse = await inspected;
  expect(inspectResponse.status()).toBe(200);
  const { capability } = await inspectResponse.json();
  expect(capability.source.revision).toBe(commit);
  expect(capability.source.contentSha256).toMatch(/^[0-9a-f]{64}$/);
  await expect(page.getByText(/Đã xác minh: v1.0.0/)).toBeVisible();
  await expect(page.getByText(capability.source.contentSha256, { exact: true })).not.toBeVisible();
  await page.locator('summary', { hasText: 'Thiết lập nâng cao' }).click();
  await page.getByRole('button', { name: 'Ghép đầu ra theo tên trùng', exact: true }).click();
  await provideLiteral(page, 'label', 'browser-value');
  const created = page.waitForResponse((response) => response.url().endsWith('/resource-definitions') && response.request().method() === 'POST');
  await page.getByRole('button', { name: 'Đăng ký cấu hình tài nguyên', exact: true }).click();
  const createResponse = await created;
  expect(createResponse.status()).toBe(201);
  await expect(page.getByRole('button', { name: 'Thêm từ Git', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await expect(page.getByLabel('Git HTTPS URL', { exact: true })).toHaveValue('');
  await expect(page.getByText(/Đã xác minh: v1.0.0/)).not.toBeVisible();
  const definition = await createResponse.json();
  expect(definition.driverInputs.values.source.revision).toBe(commit);
  expect(definition.driverInputs.values.variables.label).toBe('browser-value');
  expect(definition.driverInputs.values.output_mapping).toEqual({ name: 'name' });
  await page.reload();
  const row = page.locator('[data-definition="git-demo-definition"]');
  await expect(row).toContainText('Git module demo');
  await row.getByRole('button', { name: /Sửa/ }).click();
  await expect(page.locator('[data-variable="label"]')).toContainText('browser-value');
  await page.locator('summary', { hasText: 'Thiết lập nâng cao' }).click();
  await expect(page.getByLabel('Output module cung cấp name', { exact: true })).toHaveValue('name');
  await page.getByLabel('Tên hiển thị', { exact: true }).fill('Git module edited');
  const edited = page.waitForResponse((response) => response.url().endsWith('/resource-definitions/git-demo-definition') && response.request().method() === 'PUT');
  await page.getByRole('button', { name: 'Lưu thay đổi', exact: true }).click();
  const editResponse = await edited;
  expect(editResponse.status()).toBe(200);
  const saved = await editResponse.json();
  expect(saved.version).toBe(2);
  expect(saved.driverInputs.values.source).toEqual(definition.driverInputs.values.source);
  await page.reload();
  await expect(page.locator('[data-definition="git-demo-definition"]')).toContainText('Git module edited');
  // New drafts return to Git; registered modules require an explicit switch.
  await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Thêm từ Git', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('button', { name: 'Module đã đăng ký', exact: true }).click();
  await page.getByRole('combobox', { name: /^Loại tài nguyên/ }).selectOption('git-demo');
  await expect(page.getByRole('combobox', { name: /^Module đã kiểm chứng/ })).toContainText(`modules · ${commit.slice(0, 12)}`);
  console.log('PASS: Git tag → immutable commit/digest → typed inputs/mappings → create → reload/edit/CAS → installed module selection');
} finally {
  await browser.close();
}
