// Read-only product UI verification; no API interception or platform mutation.
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';
import assert from 'node:assert/strict';
let session;
let config;
try {
 config = readConsoleConfig();
 session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
 const { page } = session;
 await page.goto(`${config.baseURL}/ui/platform/cd-providers`);
 await page.getByRole('heading', { name: 'Cấu hình CD', exact: true }).waitFor();
 const response = await page.request.get(`${config.baseURL}/api/v1/delivery-default`);
 assert.equal(response.status(), 200);
 const current = await response.json();
 assert(Number.isInteger(current.revision));
 await page.getByRole('button', { name: 'Thêm cấu hình CD', exact: true }).click();
 const git = page.getByLabel('Kết nối Git', { exact: true });
 await git.waitFor();
 const shared = await git.inputValue() === '__platform';
 if (shared) {
  assert.equal(await page.getByLabel('Token hoặc mật khẩu Git', { exact: true }).count(), 0);
  assert.equal(await page.getByLabel('Tài khoản hoặc tổ chức GitHub', { exact: true }).count(), 0);
 }
 assert.equal(await page.getByRole('button', { name: 'Thêm kết nối Kubernetes', exact: true }).count(), 1);
 assert.equal(await page.getByLabel('Định danh cluster đã xác minh', { exact: true }).count(), 0);
 await page.getByLabel('Workspace Fleet', { exact: true }).waitFor();
 await page.getByLabel('Cluster đã đăng ký', { exact: true }).waitFor();
 assert.equal(await page.getByLabel('Git repository HTTPS', { exact: true }).count(), 0);
 await page.getByRole('button', { name: 'Hủy', exact: true }).click();
 const apps = await page.request.get(`${config.baseURL}/api/v1/applications`);
 assert(apps.ok());
 const data = await apps.json();
 if (data.applications.length) {
  const app = data.applications[0];
  await page.goto(`${config.baseURL}/ui/applications/${encodeURIComponent(app.key ?? app.id)}/settings?environment=staging`);
  await page.getByRole('heading', { name: 'Cài đặt môi trường', exact: true }).waitFor();
  assert.equal(await page.getByLabel('CD provider', { exact: true }).count(), 0);
 }
 console.log('PASS: platform-only CD policy UI and authenticated APIs; no GitHub/Fleet deployment asserted.');
} finally {
 if (session) await closeConsole(session, config);
}
