import { mkdir } from 'node:fs/promises';
import { resolve, join } from 'node:path';
import { createRequire } from 'node:module';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';
import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';

// Load assertions from the same Playwright installation as the shared browser
// helper; separate package installs cannot initialize the test runtime twice.
const { expect } = createRequire(new URL('../../../scripts/bootstrap/lib/console.mjs', import.meta.url))('@playwright/test');

const config = readConsoleConfig();
if (!process.env.ORCH_TEST_RUN_DIR) throw new Error('Set ORCH_TEST_RUN_DIR outside the repository for screenshots');
const evidence = resolve(process.env.ORCH_TEST_RUN_DIR);
outsideRepo(evidence);
await mkdir(evidence, { recursive: true });
let session;
try {
  session = await openConsole(config);
  const { page, browser } = session;
  const errors = [];
  const mutations = [];
  page.on('pageerror', error => errors.push(error.message));
  page.on('request', request => {
    if (new URL(request.url()).pathname.startsWith('/api/') && !['GET', 'HEAD'].includes(request.method())) mutations.push(request.method());
  });
  const sessionReply = await page.request.get(`${config.baseURL}/api/v1/auth/session`);
  expect(sessionReply.ok()).toBe(true);
  expect((await sessionReply.json()).user.Username).toBe(config.username);
  expect((await page.request.get(`${config.baseURL}/api/v1/applications`)).ok()).toBe(true);

  // Capture the product reference before navigating to the new article.
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true })).toBeVisible();
  await page.screenshot({ path: join(evidence, 'applications-reference.png') });
  const link = page.getByRole('link', { name: 'Hướng dẫn sử dụng', exact: true });
  await link.focus();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(`${config.baseURL}/ui/user-guide`);
  await expect(link).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('heading', { level: 1, name: 'Hướng dẫn sử dụng' })).toBeVisible();
  await expect(page.getByRole('article')).toHaveCount(1);
  await expect(page.getByRole('heading', { name: 'Viết và nhập biến môi trường', exact: true })).toBeVisible();
  for (const anchor of await page.getByRole('navigation', { name: 'Mục lục' }).getByRole('link').all()) {
    const href = await anchor.getAttribute('href');
    await expect(page.locator(href)).toHaveCount(1);
  }
  const secretLink = page.getByRole('link', { name: 'Nhập trực tiếp biến bí mật', exact: true });
  await secretLink.focus();
  await page.keyboard.press('Enter');
  await expect(page.locator('#guide-secrets')).toBeInViewport();
  await expect(page.getByLabel('Ví dụ đánh dấu tạo Secret mới', { exact: true })).toContainText('# @secret\nNEW_API_TOKEN=');
  await expect(page.getByRole('article')).toContainText('hủy form workload không xóa Secret đã tạo');
  await page.screenshot({ path: join(evidence, 'secrets-vi.png') });

  await page.goto(`${config.baseURL}/ui/user-guide`);
  await page.reload();
  await expect(page.getByRole('article')).toBeVisible();
  await page.getByRole('button', { name: 'Thu gọn thanh điều hướng', exact: true }).click();
  await expect(link).toHaveAttribute('title', 'Hướng dẫn sử dụng');
  await expect(link).toHaveAttribute('aria-current', 'page');
  await page.screenshot({ path: join(evidence, 'guide-collapsed.png') });
  await page.getByRole('button', { name: 'Mở rộng thanh điều hướng', exact: true }).click();
  for (const width of [1280, 1512]) {
    await page.setViewportSize({ width, height: 1000 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: join(evidence, `guide-vi-${width}.png`) });
  }
  await page.getByRole('link', { name: 'Ứng dụng', exact: true }).click();
  await page.goBack();
  await expect(page.getByRole('article')).toBeVisible();
  await page.goForward();
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true })).toBeVisible();
  await page.goBack();
  await expect(page.getByRole('article')).toBeVisible();
  await page.getByRole('combobox', { name: /Ngôn ngữ/ }).selectOption('en');
  await expect(page.getByRole('heading', { level: 1, name: 'User Guide' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'User Guide', exact: true })).toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('heading', { name: 'Write and import environment variables', exact: true })).toBeVisible();
  await expect(page.getByRole('article')).not.toContainText('guide.');
  await page.screenshot({ path: join(evidence, 'guide-en.png') });
  await page.getByRole('combobox', { name: /Language/ }).selectOption('vi');
  await expect(page.getByRole('heading', { level: 1, name: 'Hướng dẫn sử dụng' })).toBeVisible();
  expect(mutations).toEqual([]);
  expect(errors).toEqual([]);

  const anonymous = await browser.newContext();
  try {
    const anonymousPage = await anonymous.newPage();
    await anonymousPage.goto(`${config.baseURL}/ui/user-guide`);
    await expect(anonymousPage.locator('form.sign-in-form')).toBeVisible();
    await expect(anonymousPage.getByRole('article')).toHaveCount(0);
  } finally { await anonymous.close(); }
  console.log('PASS: real backend/session, sidebar/keyboard, anchors, Secret instructions, deep-link/refresh/history, collapse, VI/EN, no content mutations, anonymous auth gate');
} finally {
  if (session) await closeConsole(session, config);
}
