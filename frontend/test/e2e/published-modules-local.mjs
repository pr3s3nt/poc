// Real GitHub inspection of every published module, using the isolated
// gitmodulebrowser HTTP/catalog harness. No Terraform apply or cloud calls.
import { chromium, expect } from '@playwright/test';
import { readFile } from 'node:fs/promises';

const base = process.env.GIT_MODULE_CONSOLE_URL;
const commit = process.env.PUBLISHED_MODULE_COMMIT;
if (!base || !commit) throw new Error('Run with gitmodulebrowser and PUBLISHED_MODULE_COMMIT');
const url = 'https://github.com/pr3s3nt/terraform-modules.git';
const release = JSON.parse(await readFile(new URL('../../../backend/internal/adapters/terraform/registry/release.json', import.meta.url), 'utf8'));
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  await page.goto(`${base}/ui/platform/resource-definitions`);
  await page.getByLabel(/Tên đăng nhập/).fill('platform-engineer');
  await page.getByLabel(/Mật khẩu/).fill('test-password');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await page.waitForURL('**/ui/applications');
  // The AWS default seed predates Redis; register the normal Redis Type
  // contract in this isolated catalog before inspecting its two modules.
  const redis = await page.request.post(`${base}/api/v1/resource-types`, { data: {
    key: 'redis', inputs: [], outputs: [
      { name: 'host', type: 'string', required: true }, { name: 'port', type: 'number', required: true },
      { name: 'username', type: 'string', required: true }, { name: 'password', type: 'string', required: true, secret: true },
      { name: 'tls', type: 'bool', required: true },
    ],
  } });
  expect(redis.status()).toBe(201);
  for (const module of release.packaged.modules) {
    const response = await page.request.post(`${base}/api/v1/terraform-modules/inspect`, { data: {
      resourceType: module.resourceTypes[0], url, path: `modules/${module.id}`, ref: 'v1.0.0',
    } });
    expect(response.status(), module.id).toBe(200);
    const { capability } = await response.json();
    expect(capability.source.sourceId).toMatch(/^git-/);
    expect(capability.source.moduleId).toBe(module.id);
    expect(capability.source.revision).toBe(commit);
    expect(capability.source.contentSha256).toBe(module.contentSha256);
    for (const output of module.executorOutputs ?? []) expect(capability.outputs.map((entry) => entry.name)).toContain(output);
    console.log(`PASS: ${module.id} GitHub tag → commit/digest and release lifecycle identity`);
  }
  await page.goto(`${base}/ui/platform/resource-definitions`);
  await page.getByRole('button', { name: 'Tạo cấu hình', exact: true }).click();
  await expect(page.getByRole('button', { name: 'Thêm từ Git', exact: true })).toHaveAttribute('aria-pressed', 'true');
  await page.getByRole('combobox', { name: /^Loại tài nguyên/ }).selectOption('k8s-cluster');
  await page.getByLabel('Git HTTPS URL', { exact: true }).fill(url);
  await page.getByLabel('Thư mục module', { exact: true }).fill('modules/eks');
  await page.getByLabel('Tag hoặc commit', { exact: true }).fill('v1.0.0');
  const inspected = page.waitForResponse((response) => response.url().endsWith('/terraform-modules/inspect'));
  await page.getByRole('button', { name: 'Kiểm tra module', exact: true }).click();
  expect((await inspected).status()).toBe(200);
  await expect(page.getByText(/Đã xác minh: v1.0.0/)).toBeVisible();
  await page.locator('summary', { hasText: 'Thiết lập nâng cao' }).click();
  await expect(page.getByLabel('Output module cung cấp kubeContext', { exact: true })).toContainText('kubeContext');
  console.log('PASS: default Git editor inspects published EKS and retains executor output mapping');
} finally {
  await browser.close();
}
