// Live positive-path verification, using the existing frontend Playwright package.
// Usage: node deploy/ec2/verify.mjs BASE_URL ROLE_ARN REGION [SCREENSHOT]
import { createRequire } from 'node:module';
const require = createRequire(new URL('../../frontend/package.json', import.meta.url));
const { chromium, expect } = require('@playwright/test');
const [base, roleArn, region, screenshot] = process.argv.slice(2);
if (!base || !/^arn:aws:iam::\d{12}:role\//.test(roleArn ?? '') || !region) {
  throw new Error('Supply BASE_URL ROLE_ARN REGION [SCREENSHOT]');
}
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage();
  page.setDefaultTimeout(30_000);
  await page.goto(`${base.replace(/\/$/, '')}/ui/sign-in`);
  await page.getByLabel('Tên đăng nhập').fill('platform-engineer');
  await page.getByLabel('Mật khẩu').fill('test-password');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Ứng dụng của bạn', exact: true })).toBeVisible();
  await page.goto(`${base.replace(/\/$/, '')}/ui/platform/driver-accounts`);
  await expect(page.getByRole('heading', { name: 'Tài khoản hạ tầng', exact: true })).toBeVisible();
  const response = await page.request.get(`${base.replace(/\/$/, '')}/api/v1/driver-accounts`);
  if (!response.ok()) throw new Error(`Account list HTTP ${response.status()}`);
  const { accounts } = await response.json();
  const name = 'ec2-instance-role';
  let account = accounts.find((item) => item.name === name);
  const reused = Boolean(account);
  if (!account) {
    await page.getByRole('radio', { name: 'AWS', exact: true }).check();
    await page.getByLabel('Phương thức xác thực').selectOption('ec2-role');
    await page.getByLabel('Tên tài khoản', { exact: true }).fill(name);
    await page.getByLabel('IAM Role ARN', { exact: true }).fill(roleArn);
    await page.getByLabel('Vùng kiểm tra', { exact: true }).fill(region);
    const registered = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/driver-accounts/aws/ec2-role', { timeout: 120_000 });
    await page.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }).click();
    const saved = await registered;
    if (saved.status() !== 201) {
      const body = await saved.json();
      throw new Error(`Role registration HTTP ${saved.status()} code=${body.code ?? body.error?.code ?? 'unknown'}`);
    }
    account = await saved.json();
  }
  if (account.status !== 'READY' || account.authenticationType !== 'AWS_EC2_ROLE' || account.config?.roleArn !== roleArn || !account.config?.verificationRegions?.includes(region)) {
    throw new Error('Stored account does not match the READY EC2 role and region');
  }
  await expect(page.getByRole('table', { name: 'Tài khoản hạ tầng' })).toContainText(name);
  if (reused) {
    await page.getByRole('button', { name: `Xem ${name}`, exact: true }).click();
    const verified = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === `/api/v1/driver-accounts/${account.key}/verify`, { timeout: 120_000 });
    await page.getByRole('button', { name: 'Kiểm tra kết nối', exact: true }).click();
    const refreshed = await verified;
    if (refreshed.status() !== 200) throw new Error(`Role re-verification HTTP ${refreshed.status()}`);
    const current = await refreshed.json();
    if (current.status !== 'READY' || current.config?.roleArn !== roleArn || current.version <= account.version) throw new Error('Role re-verification did not retain the expected READY identity');
    account = current;
  }
  if (screenshot) await page.screenshot({ path: screenshot, fullPage: true });
  console.log(JSON.stringify({ result: 'PASS', login: true, reverified: reused, accountKey: account.key, status: account.status, roleArn, region }));
} finally {
  await browser.close();
}
