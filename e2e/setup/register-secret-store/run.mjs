// Registers a Vault KV v2 Secret Store on the Secret stores page. Everything is
// read from and done on the screen. A store already listed under the same name
// is reused when it is READY; it is never changed or deleted. The scoped Vault
// token is filled into the masked field only and never printed.
import { expect } from '@playwright/test';
import { closeConsole, ConsoleError, openConsole, readConsoleConfig, readPrivateFile } from '../../lib/console.mjs';

const env = process.env;
let session;
let config;
try {
  config = readConsoleConfig(env);
  const store = {
    name: (env.ORCH_SECRET_STORE_NAME ?? 'K8S-4F Vault').trim(),
    backendAddress: (env.ORCH_VAULT_BACKEND_ADDRESS ?? '').trim(),
    workloadAddress: (env.ORCH_VAULT_WORKLOAD_ADDRESS ?? env.ORCH_VAULT_BACKEND_ADDRESS ?? '').trim(),
    mount: (env.ORCH_VAULT_KV_MOUNT ?? 'kv').trim(),
    authMount: (env.ORCH_VAULT_AUTH_MOUNT ?? 'kubernetes').trim(),
  };
  if (!store.backendAddress) throw new ConsoleError('Thiếu ORCH_VAULT_BACKEND_ADDRESS: địa chỉ Vault mà backend Console truy cập');
  const tokenFile = (env.ORCH_VAULT_TOKEN_FILE ?? '').trim();
  if (!tokenFile) throw new ConsoleError('Thiếu ORCH_VAULT_TOKEN_FILE: file chứa token Vault có phạm vi');
  const token = readPrivateFile(tokenFile, 'token Vault').trim();

  console.log(`Console: ${config.baseURL}`);
  session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
  const { page } = session;
  await page.getByRole('link', { name: /(^|\s)Kho bí mật$/ }).click();
  await expect(page.getByRole('heading', { name: 'Kho bí mật', level: 1 })).toBeVisible();
  const list = page.getByRole('region', { name: 'Kho bí mật đã đăng ký' });
  await expect(list.locator('.settings-table').or(list.getByText('Chưa có kho bí mật nào được đăng ký.'))).toBeVisible();
  const row = () => list.locator('.settings-table-row').filter({ hasText: store.name });

  if (await row().count()) {
    const text = await row().first().innerText();
    if (!text.includes('Sẵn sàng')) throw new ConsoleError(`Kho "${store.name}" đã có trên trang nhưng chưa Sẵn sàng; kiểm tra trên Console, script không sửa`);
    console.log(`PASS: dùng lại kho bí mật "${store.name}" (Sẵn sàng)`);
  } else {
    await page.getByLabel('Tên kho', { exact: true }).fill(store.name);
    await page.getByRole('textbox', { name: 'Địa chỉ backend' }).fill(store.backendAddress);
    await page.getByLabel('Địa chỉ workload', { exact: true }).fill(store.workloadAddress);
    await page.getByRole('button', { name: 'Hiện nâng cao' }).click();
    await page.getByLabel('Mount KV v2', { exact: true }).fill(store.mount);
    await page.getByLabel('Mount xác thực Kubernetes', { exact: true }).fill(store.authMount);
    await page.getByLabel('Token Vault', { exact: true }).fill(token);
    await expect(page.getByLabel('Token Vault', { exact: true })).toHaveAttribute('type', 'password');
    const reply = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/secret-stores', { timeout: 120_000 });
    await page.getByRole('button', { name: 'Xác minh và đăng ký' }).click();
    const response = await reply;
    if ((await response.text()).includes(token) || (await page.locator('body').innerText()).includes(token)) throw new ConsoleError('Token Vault xuất hiện trên trang; dừng');
    if (response.status() !== 201) {
      const alert = page.getByRole('alert').first();
      throw new ConsoleError(`Console từ chối đăng ký kho (HTTP ${response.status()}): ${(await alert.isVisible()) ? await alert.innerText() : 'xem thông báo trên trang'}`);
    }
    await expect(row().first()).toContainText('Sẵn sàng', { timeout: 30_000 });
    console.log(`PASS: đã đăng ký kho bí mật "${store.name}" (Sẵn sàng)`);
  }
} catch (error) {
  console.error(`FAIL: ${error.message.split('\n')[0]}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, config);
}
