// Shared Web Console access for bootstrap and E2E scenarios. Every scenario reads the same
// configuration, so one script can target any Console:
//
//   ORCH_PLATFORM_URL            Console origin, e.g. http://localhost:3001 (a trailing /ui is accepted)
//   ORCH_PLATFORM_USERNAME       login account
//   ORCH_PLATFORM_PASSWORD_FILE  private file (0600/0400) holding only the password
//   ORCH_HEADLESS=1              run without a visible browser window
//
// Custom passwords are read from a private file, never CLI arguments. Bootstrap
// presets may supply the known product seed password as a fallback. Neither is printed. No browser session or trace is stored. Scenarios may opt into video by passing
// recordVideo to signInContext; evidence must stay outside Git.
import { chromium } from '@playwright/test';
import { readFileSync, statSync } from 'node:fs';

export class ConsoleError extends Error {}

export function readConsoleConfig(env = process.env, { defaultPassword } = {}) {
  const raw = (env.ORCH_PLATFORM_URL ?? '').trim();
  if (!raw) throw new ConsoleError('Thiếu ORCH_PLATFORM_URL, ví dụ http://localhost:3001');
  let url;
  try { url = new URL(raw); } catch { throw new ConsoleError(`ORCH_PLATFORM_URL không hợp lệ: ${raw}`); }
  const path = url.pathname.replace(/\/+$/, '');
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash || (path !== '' && path !== '/ui')) {
    throw new ConsoleError('ORCH_PLATFORM_URL phải là địa chỉ gốc của Console, ví dụ http://localhost:3001');
  }
  const username = (env.ORCH_PLATFORM_USERNAME ?? '').trim();
  if (!username) throw new ConsoleError('Thiếu ORCH_PLATFORM_USERNAME, ví dụ platform-engineer');
  const passwordFile = (env.ORCH_PLATFORM_PASSWORD_FILE ?? '').trim();
  if (!passwordFile && !defaultPassword) throw new ConsoleError('Thiếu ORCH_PLATFORM_PASSWORD_FILE: đường dẫn file chỉ chứa mật khẩu');
  return { baseURL: url.origin, username, password: passwordFile ? readPrivateFile(passwordFile, 'mật khẩu') : defaultPassword, headless: env.ORCH_HEADLESS === '1' };
}

// A second account of the same Console (for example the developer next to the
// platform engineer). Its password file defaults to the first account's file.
export function readSecondAccount(config, env, { usernameVar, passwordFileVar, defaultUsername }) {
  const username = (env[usernameVar] ?? defaultUsername ?? '').trim();
  if (!username) throw new ConsoleError(`Thiếu ${usernameVar}`);
  const passwordFile = (env[passwordFileVar] ?? '').trim();
  return { ...config, username, password: passwordFile ? readPrivateFile(passwordFile, 'mật khẩu') : config.password };
}

// Reads a credential file that only its owner may read; the content is never printed.
export function readPrivateFile(file, what = 'bí mật', { platform = process.platform } = {}) {
  let stat;
  try { stat = statSync(file); } catch { throw new ConsoleError(`Không tìm thấy file ${what}: ${file}`); }
  if (!stat.isFile()) throw new ConsoleError(`File ${what} không phải file thường: ${file}`);
  if (platform !== 'win32' && (stat.mode & 0o077)) throw new ConsoleError(`File ${what} phải riêng tư; chạy: chmod 600 ${file}`);
  const value = readFileSync(file, 'utf8').replace(/\r?\n$/, '');
  if (!value) throw new ConsoleError(`File ${what} rỗng: ${file}`);
  return value;
}

// Returns the signed-in user of the page's browser context, or null when signed out.
export async function currentUser(page, baseURL) {
  const response = await page.request.get(`${baseURL}/api/v1/auth/session`);
  if (response.status() === 401) return null;
  if (!response.ok()) throw new ConsoleError(`Không đọc được phiên đăng nhập: HTTP ${response.status()}`);
  const { user } = await response.json();
  return { username: user.Username, role: user.Role, organization: user.OrganizationKey };
}

// Opens the Console, reuses a session that already belongs to the configured
// account, otherwise signs in through the form. A session of another account
// or a role other than the required one stops the scenario before any action.
export async function ensureSignedIn(page, config, { role } = {}) {
  await page.goto(`${config.baseURL}/ui/`);
  let user = await currentUser(page, config.baseURL);
  if (user && user.username !== config.username) {
    throw new ConsoleError(`Console đang đăng nhập bằng ${user.username}, không phải ${config.username}; đăng xuất rồi chạy lại`);
  }
  if (!user) {
    const form = page.locator('form.sign-in-form');
    await form.locator('input[autocomplete="username"]').fill(config.username);
    await form.locator('input[autocomplete="current-password"]').fill(config.password);
    const reply = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/auth/sign-in');
    await form.locator('button[type="submit"]').click();
    const response = await reply;
    if (response.status() === 401) throw new ConsoleError(`Sai tên đăng nhập hoặc mật khẩu cho ${config.username}`);
    if (!response.ok()) throw new ConsoleError(`Đăng nhập thất bại: HTTP ${response.status()}`);
    await form.waitFor({ state: 'hidden' });
    user = await currentUser(page, config.baseURL);
    if (user?.username !== config.username) throw new ConsoleError('Đăng nhập xong nhưng không xác nhận được phiên');
  }
  if (role && user.role !== role) {
    throw new ConsoleError(`Tài khoản ${user.username} có vai trò ${user.role}; kịch bản cần ${role}`);
  }
  return user;
}

// Signs one account in, in its own browser context of an already launched browser.
export async function signInContext(browser, config, { role, recordVideo } = {}) {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, ...(recordVideo ? { recordVideo } : {}) });
  await context.addInitScript(() => { if (!globalThis.localStorage.getItem('orchestrator.locale')) globalThis.localStorage.setItem('orchestrator.locale', 'vi'); });
  const page = await context.newPage();
  page.setDefaultTimeout(30_000);
  const user = await ensureSignedIn(page, config, { role });
  return { context, page, user };
}

export async function openConsole(config, { role } = {}) {
  const browser = await chromium.launch({ headless: config.headless });
  try {
    return { browser, ...await signInContext(browser, config, { role }) };
  } catch (error) {
    await browser.close();
    throw error;
  }
}

// Ends the server session of one context (best effort).
export async function signOutContext(context, config) {
  await context.request.post(`${config.baseURL}/api/v1/auth/sign-out`).catch(() => {});
}

// Ends the server session (best effort) and closes the browser.
export async function closeConsole({ browser, context }, config) {
  if (browser.isConnected()) {
    await signOutContext(context, config);
    await browser.close().catch(() => {});
  }
}
