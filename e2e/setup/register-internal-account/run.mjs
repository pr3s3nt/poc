// Registers one Internal Driver Account (existing Kubernetes cluster) on the
// Infrastructure accounts page, from a kubeconfig file. Everything is read from
// and done on the screen. An account already listed under the same name is
// reused when it is a READY Internal account; it is never changed or deleted.
// The kubeconfig content is never printed, typed into a visible field or kept.
import { expect } from '@playwright/test';
import { closeConsole, ConsoleError, openConsole, readConsoleConfig, readPrivateFile } from '../../lib/console.mjs';

const env = process.env;
let session;
let config;
try {
  config = readConsoleConfig(env);
  const name = (env.ORCH_ACCOUNT_NAME ?? 'k8s-4f').trim();
  const kubeconfigFile = (env.ORCH_KUBECONFIG_FILE ?? '').trim();
  if (!kubeconfigFile) throw new ConsoleError('Thiếu ORCH_KUBECONFIG_FILE: kubeconfig chỉ chứa một context của cluster');
  const kubeconfig = readPrivateFile(kubeconfigFile, 'kubeconfig');
  const secrets = [...kubeconfig.matchAll(/^\s*(?:token|client-key-data|client-certificate-data):\s*(\S+)\s*$/gm)]
    .map((m) => m[1].replace(/^["']|["']$/g, '')).filter((v) => v.length >= 12);
  const leaks = (text) => secrets.some((v) => text.includes(v));

  console.log(`Console: ${config.baseURL}`);
  session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
  const { page } = session;
  await page.getByRole('link', { name: /(^|\s)Tài khoản hạ tầng$/ }).click();
  await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', level: 1, exact: true })).toBeVisible();
  const table = page.getByRole('table', { name: 'Tài khoản hạ tầng' });
  await expect(table.or(page.getByText('Chưa có tài khoản hạ tầng nào được đăng ký.'))).toBeVisible();
  const row = () => table.getByRole('row').filter({ has: page.getByRole('button', { name: `Xem ${name}`, exact: true }) });

  if (await row().count()) {
    const text = await row().innerText();
    if (!text.includes('Internal') || !text.includes('Sẵn sàng')) {
      throw new ConsoleError(`Account ${name} đã có trên trang nhưng không phải Internal ở trạng thái Sẵn sàng; kiểm tra trên Console, script không sửa`);
    }
    console.log(`PASS: dùng lại account ${name} (Internal, Sẵn sàng)`);
  } else {
    const form = page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    await form.getByLabel('Tên tài khoản', { exact: true }).fill(name);
    await form.locator('input[type="file"]').setInputFiles(kubeconfigFile);
    await expect(form.getByLabel('Đích đã chọn')).toBeVisible();
    const reply = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/driver-accounts/internal', { timeout: 180_000 });
    await form.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }).click();
    const response = await reply;
    if (leaks(await response.text()) || leaks(await page.locator('body').innerText())) throw new ConsoleError('Phát hiện dữ liệu xác thực trên trang; dừng');
    if (response.status() !== 201) {
      const alert = page.getByRole('alert').first();
      throw new ConsoleError(`Console từ chối đăng ký (HTTP ${response.status()}): ${(await alert.isVisible()) ? await alert.innerText() : 'xem thông báo trên trang'}`);
    }
    await expect(row()).toContainText('Sẵn sàng', { timeout: 30_000 });
    console.log(`PASS: đã đăng ký account ${name} (Internal, Sẵn sàng)`);
  }
} catch (error) {
  console.error(`FAIL: ${error.message.split('\n')[0]}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, config);
}
