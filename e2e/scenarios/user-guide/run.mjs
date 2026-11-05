import assert from 'node:assert/strict';
import { mkdir } from 'node:fs/promises';
import { resolve } from 'node:path';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';

const config = readConsoleConfig();
const role = process.env.ORCH_GUIDE_ROLE ?? 'DEVELOPER';
assert.ok(['DEVELOPER', 'PLATFORM_ENGINEER', 'ADMIN'].includes(role));
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '/tmp/orchestrator-user-guide');
await mkdir(evidence, { recursive: true });
const session = await openConsole(config, { role });
const { page } = session;
await page.emulateMedia({ reducedMotion: 'reduce' });
const mutations = [];
page.on('request', request => {
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(request.method())) mutations.push(new URL(request.url()).pathname);
});
try {
  await page.getByRole('link', { name: 'Hướng dẫn sử dụng', exact: true }).click();
  assert.equal(new URL(page.url()).pathname, '/ui/guides');
  await page.getByRole('heading', { name: 'DEV — Triển khai acceptance app từng bước', exact: true }).waitFor();
  assert.equal(await page.getByRole('link', { name: 'Hướng dẫn sử dụng', exact: true }).getAttribute('aria-current'), 'page');
  assert.equal(await page.getByRole('link', { name: 'Loại tài nguyên', exact: true }).count(), role === 'DEVELOPER' ? 0 : 1);
  const screenshots = page.locator('.guide-article img');
  assert.equal(await screenshots.count(), 15);
  for (const illustration of await screenshots.all()) {
    await illustration.scrollIntoViewIfNeeded();
    await illustration.evaluate(image => image.complete && image.naturalWidth > 0 ? undefined : new Promise((resolve, reject) => {
      image.addEventListener('load', resolve, { once: true }); image.addEventListener('error', reject, { once: true });
    }));
    const src = await illustration.getAttribute('src');
    assert.ok(src.startsWith('/ui/guides/acceptance-app/'), 'illustration must use Console assets');
    assert.equal(new URL(await illustration.evaluate(image => image.currentSrc)).origin, new URL(config.baseURL).origin);
  }
  const emptyState = page.getByRole('button', { name: /Xem ảnh: Ngay sau khi tạo/ });
  await emptyState.focus();
  await page.keyboard.press('Enter');
  await page.getByRole('dialog', { name: /Ngay sau khi tạo/ }).waitFor();
  await page.keyboard.press('Escape');
  assert.ok(await emptyState.evaluate(element => document.activeElement === element));
  await page.getByRole('link', { name: '2. Thêm workload backend', exact: true }).click();
  assert.equal(new URL(page.url()).hash, '#2-them-workload-backend');
  await page.getByRole('button', { name: 'Thu gọn thanh điều hướng', exact: true }).click();
  await page.reload();
  await page.getByRole('heading', { name: 'DEV — Triển khai acceptance app từng bước', exact: true }).waitFor();
  assert.equal(await page.locator('.app-shell-collapsed').count(), 1);
  await page.getByRole('button', { name: 'Mở rộng thanh điều hướng', exact: true }).click();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({ path: resolve(evidence, `${role.toLowerCase()}.png`), fullPage: false });
  assert.deepEqual(mutations, [], 'reading the guide must not mutate application data');
  const anonymous = await session.browser.newContext();
  try {
    const signedOut = await anonymous.newPage();
    await signedOut.goto(`${config.baseURL}/ui/guides`);
    await signedOut.locator('form.sign-in-form').waitFor();
    assert.equal(await signedOut.locator('.guide-article').count(), 0);
  } finally { await anonymous.close(); }
  console.log(JSON.stringify({ result: 'PASS', role, url: page.url(), images: 15, applicationMutations: mutations.length, evidence }));
} finally {
  await closeConsole(session, config);
}
