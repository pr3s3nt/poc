import { GitAccessError, gitAccessAction, readGitToken } from './git-access-config.mjs';
import { isDeepStrictEqual } from 'node:util';

const endpoint = '/platform/settings/git';
// Mutations come from the product form only. Guard draft/version before forwarding.
export async function ensureGitAccess({ page, baseURL, get, desired, update = false }) {
  if (!desired) return;
  let token = '';
  const routes = [];
  try {
    const current = (await get(endpoint)).settings;
    const action = gitAccessAction(current, desired, update);
    if (action !== 'reuse') token = readGitToken(desired.tokenFile);
    await page.goto(`${baseURL}/ui/platform/settings`);
    const form = page.locator('form').filter({ has: page.getByRole('heading', { name: 'Truy cập Git', exact: true }) });
    await form.waitFor();
    await form.getByLabel('URL Git / phạm vi', { exact: true }).fill(desired.url);
    await form.getByLabel('Xác thực', { exact: true }).selectOption('token');
    await form.getByLabel('Username', { exact: true }).fill(desired.username);
    await form.getByLabel('Token', { exact: true }).fill(token);
    await form.locator('summary').click();
    await form.getByLabel('Chứng chỉ CA (PEM, tùy chọn)', { exact: true }).fill(desired.caPem);
    await form.getByLabel('Repo kiểm tra', { exact: true }).fill(desired.repositoryUrl);
    await form.getByLabel('Nhánh, tag hoặc commit kiểm tra', { exact: true }).fill(desired.ref);
    const expected = { url: desired.url, username: desired.username, authenticated: true, token, caPem: desired.caPem, expectedVersion: current.version };
    let blocked = false;
    for (const [suffix, method, body] of [['/test', 'POST', { ...expected, repositoryUrl: desired.repositoryUrl, ref: desired.ref }], ['', 'PUT', expected]]) {
      const url = `${baseURL}/api/v1${endpoint}${suffix}`;
      const guard = async route => {
        if (route.request().method() !== method) return route.fallback();
        let valid = false;
        try { valid = isDeepStrictEqual(route.request().postDataJSON(), body) && !(action === 'reuse' && method === 'PUT'); } catch { /* fixed safe failure below */ }
        if (!valid) { blocked = true; return route.fulfill({ status: 422, json: { code: 'INVALID_REQUEST' } }); }
        return route.fallback();
      };
      await page.route(url, guard); routes.push([url, guard]);
    }
    async function clickAndCheck(suffix, method, label) {
      const [reply] = await Promise.all([
        page.waitForResponse(reply => reply.request().method() === method && reply.url() === `${baseURL}/api/v1${endpoint}${suffix}`, { timeout: 45_000 }),
        form.getByRole('button', { name: label, exact: true }).click(),
      ]);
      if (blocked) throw new GitAccessError('Git access: draft/version UI không khớp; đã chặn request');
      if (!reply.ok()) throw new GitAccessError(`Git access: ${method} HTTP ${reply.status()}; ${method === 'PUT' && reply.status() === 503 ? 'kiểm tra credential store backend; ' : ''}dừng, không tự retry hoặc ghi đè`);
      const body = await reply.json();
      if (suffix && body.reachable !== true) throw new GitAccessError('Git access: không xác nhận được kết nối Git');
    }
    await clickAndCheck('/test', 'POST', 'Kiểm tra kết nối');
    if (action !== 'reuse') await clickAndCheck('', 'PUT', 'Lưu');
    const persisted = (await get(endpoint)).settings;
    if (gitAccessAction(persisted, desired, false) !== 'reuse' || persisted.version !== current.version + (action === 'reuse' ? 0 : 1)) throw new GitAccessError('Git access: không xác minh được cấu hình/version sau thao tác; kiểm tra trước khi chạy lại');
    console.log(`PASS: Git access ${action}, backend đọc được repo/ref; cấu hình và version khớp`);
  } catch (error) {
    // Never expose Playwright call logs, request bodies, token or filled values.
    if (error instanceof GitAccessError) throw error;
    throw new GitAccessError('Git access: thao tác UI/API thất bại; kiểm tra Console và cấu hình, không tự retry');
  } finally {
    token = '';
    for (const [url, guard] of routes) await page.unroute(url, guard).catch(() => {});
    // Avoid leaving an entered token in the browser even when a request fails.
    await page.evaluate(() => { const input = document.querySelector('#git-token'); if (input) input.value = ''; }).catch(() => {});
  }
}
