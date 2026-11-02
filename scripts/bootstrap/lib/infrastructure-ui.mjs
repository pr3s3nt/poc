import { expect } from '@playwright/test';
import { readPrivateFile } from './console.mjs';
import { assertAccountTarget, discoverAccountTarget } from './infrastructure-config.mjs';

export async function ensureInfrastructureAccount({ page, baseURL, account, act, get, responseFor, kubeconfigFile }) {
  if (!account) return;
  const { accounts } = await get('/driver-accounts');
  if (!Array.isArray(accounts)) throw new Error('Danh sách accounts không hợp lệ');
  const matches = accounts.filter(item => item.name === account.name);
  if (matches.length > 1) throw new Error('Có nhiều account cùng tên; cần kiểm tra Console');
  if (matches.length) {
    if (account.discover) account = discoverAccountTarget(account.name, [{ name: matches[0].config?.kubeContext, endpoint: matches[0].config?.endpoint }]);
    if (!account.context || !account.endpoint) throw new Error('Account thiếu context/endpoint');
    assertAccountTarget(matches[0], account);
    console.log(`PASS: account ${account.name} READY và đúng target; không đọc/upload kubeconfig`);
    return matches[0];
  }
  const file = kubeconfigFile || account.kubeconfigFile;
  if (!file) throw new Error('Account chưa có; cần ORCH_KUBECONFIG_FILE hoặc account.kubeconfigFile');
  const content = readPrivateFile(file, 'kubeconfig');
  const secrets = [...content.matchAll(/^\s*(?:token|client-key-data):\s*(\S+)\s*$/gm)].map(match => match[1].replace(/^["']|["']$/g, '')).filter(value => value.length > 12);
  const leaks = text => secrets.some(value => text.includes(value));
  console.log(`UI: đăng ký account ${account.name} bằng upload kubeconfig`);
  await page.goto(`${baseURL}/ui/platform/driver-accounts`);
  const form = page.getByRole('group', { name: 'Đăng ký tài khoản mới', exact: true });
  await act(form.getByLabel('Tên tài khoản', { exact: true }), 'fill', account.name);
  const inspectReply = responseFor('/driver-accounts/internal/inspect', 'POST');
  await act(form.locator('input[type="file"]'), 'setInputFiles', file);
  const inspected = await inspectReply;
  if (!inspected.ok()) throw new Error(`Kubeconfig inspect HTTP ${inspected.status()}`);
  const { contexts } = await inspected.json();
  if (account.discover) account = discoverAccountTarget(account.name, contexts);
  if (!Array.isArray(contexts) || !contexts.some(context => context.name === account.context && context.endpoint === account.endpoint)) throw new Error('Kubeconfig không có đúng context/endpoint đã cấu hình; không đăng ký');
  if (contexts.length > 1) await act(form.getByLabel('Context', { exact: true }), 'selectOption', account.context);
  await expect(form.getByLabel('Đích đã chọn', { exact: true })).toContainText(account.endpoint);
  const reply = responseFor('/driver-accounts/internal', 'POST');
  await act(form.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }), 'click');
  const saved = await reply;
  if (leaks(await saved.text()) || leaks(await page.locator('body').innerText())) throw new Error('Phát hiện credential bị hiển thị; dừng');
  if (!saved.ok()) throw new Error(`Account registration HTTP ${saved.status()}; kiểm tra Console trước khi chạy lại`);
  const persisted = (await get('/driver-accounts')).accounts.find(item => item.name === account.name);
  if (!persisted) throw new Error('Không tìm thấy account sau đăng ký');
  assertAccountTarget(persisted, account);
  console.log(`PASS: account ${account.name} READY, đúng endpoint/context`);
  return persisted;
}
