import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { chromium, readConsoleConfig, readPrivateFile, signInContext, signOutContext } from '../../lib/console.mjs';

const privateDir = process.env.KEYCLOAK_PRIVATE_DIR;
if (!privateDir) throw new Error('KEYCLOAK_PRIVATE_DIR required');
const config = readConsoleConfig();
const issuer = new URL(config.oidcIssuer);
if (!issuer.pathname.endsWith('/realms/platform-test')) throw new Error('Only dedicated platform-test realm is allowed');
const base = issuer.origin;
const realm = 'platform-test';
const output = resolve(process.env.ORCH_RESULT_DIR ?? '/tmp/poc-keycloak-77-evidence');
mkdirSync(output, { recursive: true, mode: 0o700 });
const controller = readPrivateFile(`${privateDir}/controller-secret`);
const tokenResponse = await fetch(`${config.oidcIssuer}/protocol/openid-connect/token`, { method: 'POST', body: new URLSearchParams({ grant_type: 'client_credentials', client_id: 'test-controller', client_secret: controller }) });
assert.equal(tokenResponse.status, 200, 'test controller authentication');
const { access_token: controllerToken } = await tokenResponse.json();
async function admin(path, options = {}) {
  const response = await fetch(`${base}/admin/realms/${realm}${path}`, { ...options, headers: { Authorization: `Bearer ${controllerToken}`, 'Content-Type': 'application/json' } });
  assert.ok(response.ok, `test realm admin API: ${response.status}`);
  return response.status === 204 ? undefined : response.json();
}
const users = await admin('/users?max=100');
const developer = users.find((u) => u.username === 'developer');
assert.ok(developer, 'test developer');
const clients = await admin('/clients?clientId=orchestrator');
const client = clients[0];
const role = await admin(`/clients/${client.id}/roles/platform-engineer`);
const mappings = `/users/${developer.id}/role-mappings/clients/${client.id}`;
const browser = await chromium.launch({ headless: config.headless });
const sessions = [];
const checks = [];
const accountConfig = (username) => ({ ...config, username, password: readPrivateFile(`${privateDir}/${username}-password`) });
try {
  for (const [username, expected] of [['developer', 'DEVELOPER'], ['platform-engineer', 'PLATFORM_ENGINEER'], ['admin', 'ADMIN']]) {
    const cfg = accountConfig(username);
    const session = await signInContext(browser, cfg, { role: expected });
    sessions.push({ ...session, config: cfg });
    const { page } = session;
    await page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true }).waitFor();
    assert.equal(await page.getByRole('link', { name: 'Loại tài nguyên', exact: true }).count(), username === 'developer' ? 0 : 1);
    assert.equal(await page.getByRole('button', { name: 'Đổi mật khẩu', exact: true }).count(), 0);
    assert.equal(await page.getByRole('link', { name: 'Người dùng', exact: true }).count(), 0);
    const api = (path) => page.request.get(`${config.baseURL}/api/v1${path}`);
    assert.equal((await api('/applications')).status(), 200);
    assert.equal((await api('/users')).status(), 403, 'internal account management disabled');
    const registration = await page.request.post(`${config.baseURL}/api/v1/resource-types`, { data: {} });
    assert.equal(registration.status(), username === 'developer' ? 403 : 400, 'backend role gate before invalid body');
    assert.equal((await page.request.post(`${config.baseURL}/api/v1/auth/sign-in`, { data: { username, password: cfg.password } })).status(), 403, 'no local sign-in bypass');
    const cookies = await session.context.cookies(config.baseURL);
    const cookie = cookies.find((c) => c.name === 'orchestrator_session');
    assert.ok(cookie?.httpOnly && cookie.sameSite === 'Lax', 'opaque HttpOnly session');
    const sessionReply = await api('/auth/session');
    const body = await sessionReply.text();
    assert.ok(!/refresh_token|access_token|id_token|clientSecret/.test(body), 'session DTO contains no OIDC credentials');
    await page.screenshot({ path: `${output}/${username}.png`, fullPage: true });
    checks.push(`${username}: browser, API gates, session cookie, no local bypass`);
  }
  let noRoleRejected = false;
  try { const unexpected = await signInContext(browser, accountConfig('no-role')); sessions.push({ ...unexpected, config: accountConfig('no-role') }); }
  catch (error) { assert.match(error.message, /Đăng nhập xong nhưng không xác nhận được phiên/); noRoleRejected = true; }
  assert.ok(noRoleRejected, 'no-role account rejected');
  checks.push('no-role account rejected by real Keycloak callback');

  const devSession = sessions[0];
  await admin(mappings, { method: 'POST', body: JSON.stringify([role]) });
  await new Promise((r) => setTimeout(r, 31000));
  const changed = await devSession.page.request.get(`${config.baseURL}/api/v1/auth/session`);
  assert.equal(changed.status(), 200);assert.equal((await changed.json()).user.Role, 'PLATFORM_ENGINEER');
  await devSession.page.reload();await devSession.page.getByRole('link', { name: 'Loại tài nguyên', exact: true }).waitFor();
  checks.push('existing session and Console observe role promotion within 30s refresh window');
  await admin(mappings, { method: 'DELETE', body: JSON.stringify([role]) });
  await new Promise((r) => setTimeout(r, 31000));
  const demoted = await devSession.page.request.get(`${config.baseURL}/api/v1/auth/session`);
  assert.equal((await demoted.json()).user.Role, 'DEVELOPER');
  assert.equal((await devSession.page.request.post(`${config.baseURL}/api/v1/resource-types`, { data: {} })).status(), 403);
  checks.push('existing session loses elevated permission after role removal');
  await admin(`/users/${developer.id}`, { method: 'PUT', body: JSON.stringify({ enabled: false }) });
  await new Promise((r) => setTimeout(r, 31000));
  assert.equal((await devSession.page.request.get(`${config.baseURL}/api/v1/auth/session`)).status(), 401, 'disabled employee loses platform session');
  checks.push('disabled employee existing session rejected');
  const pe = sessions[1];
  await pe.page.getByRole('button', { name: 'Đăng xuất', exact: true }).click();
  await pe.page.getByRole('button', { name: 'Đăng nhập SSO' }).waitFor();
  assert.equal((await pe.page.request.get(`${config.baseURL}/api/v1/auth/session`)).status(), 401);
  checks.push('Console sign-out revokes platform session');
  writeFileSync(`${output}/result.json`, JSON.stringify({ status: 'PASS', checks, scope: 'dedicated test realm + real PostgreSQL platform; no company Keycloak mutation' }, null, 2), { mode: 0o600 });
  console.log(JSON.stringify({ status: 'PASS', checks }));
} finally {
  await admin(`/users/${developer.id}`, { method: 'PUT', body: JSON.stringify({ enabled: true }) });
  await admin(mappings, { method: 'DELETE', body: JSON.stringify([role]) });
  for (const session of sessions) { await signOutContext(session.context, session.config); await session.context.close(); }
  await browser.close();
  const restored = await admin(`/users/${developer.id}`);
  assert.equal(restored.enabled, true, 'cleanup enabled test developer');
  const currentRoles = await admin(mappings);
  assert.ok(!currentRoles.some((r) => r.name === 'platform-engineer'), 'cleanup removes only test-added role');
  console.log('Test account changes restored; dedicated preview stack retained.');
}
