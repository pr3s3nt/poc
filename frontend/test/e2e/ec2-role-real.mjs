// Explicit real-EC2 account verification. This creates no cloud infrastructure;
// the separate Go acceptance workload tests Terraform and owns its own cleanup.
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';

const baseURL = process.env.EC2_CONSOLE_URL;
const roleArn = process.env.EC2_EXPECTED_ROLE_ARN;
const evidence = process.env.EC2_EVIDENCE_DIR;
if (!baseURL || !roleArn || !evidence) throw new Error('EC2_CONSOLE_URL, EC2_EXPECTED_ROLE_ARN and EC2_EVIDENCE_DIR required');
await mkdir(evidence, { recursive: true, mode: 0o700 });
const browser = await chromium.launch({ headless: true });
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
  await page.goto(`${baseURL}/ui/platform/driver-accounts`);
  await page.getByLabel(/Tên đăng nhập/).fill('platform-engineer');
  await page.getByLabel(/Mật khẩu/).fill(process.env.EC2_CONSOLE_PASSWORD ?? 'test-password');
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
  await page.waitForURL('**/ui/applications');
  await page.goto(`${baseURL}/ui/platform/driver-accounts`);
  if (process.env.EC2_RECHECK_ACCOUNT) {
    const key = process.env.EC2_RECHECK_ACCOUNT;
    await page.goto(`${baseURL}/ui/platform/driver-accounts?account=${encodeURIComponent(key)}`);
    const detail = page.getByRole('region', { name: 'Chi tiết tài khoản EC2 role verified' });
    await expect(detail.locator('dt').filter({ hasText: /^IAM Role ARN$/ }).locator('..').locator('dd')).toHaveText(roleArn);
    const result = await page.request.get(`${baseURL}/api/v1/driver-accounts/${encodeURIComponent(key)}`);
    const account = await result.json();
    if (result.status() !== 200 || account.version !== 3 || account.authenticationType !== 'AWS_EC2_ROLE') throw new Error('restart lost account identity/version');
    const verification = page.waitForResponse((response) => response.url().endsWith(`/api/v1/driver-accounts/${key}/verify`), { timeout: 120000 });
    await detail.getByRole('button', { name: 'Kiểm tra kết nối', exact: true }).click();
    const verified = await verification;
    const afterRestart = await verified.json();
    if (verified.status() !== 200 || afterRestart.version !== 4) throw new Error('verification after restart failed');
    await expect(detail).toContainText('phiên bản 4');
    await writeFile(`${evidence}/account-after-restart.json`, JSON.stringify(afterRestart), { mode: 0o600 });
    await page.screenshot({ path: `${evidence}/driver-accounts.png`, fullPage: true });
    const createForm = page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    await createForm.getByLabel('AWS', { exact: true }).check();
    await createForm.getByLabel('Phương thức xác thực').selectOption('ec2-role');
    await createForm.screenshot({ path: `${evidence}/ec2-role-form.png` });
    await page.setViewportSize({ width: 390, height: 844 });
    if (await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)) throw new Error('mobile horizontal overflow');
    await page.screenshot({ path: `${evidence}/driver-accounts-mobile.png`, fullPage: true });
    await writeFile(`${evidence}/restart-result.json`, JSON.stringify({ result: 'PASS', key, persistedVersion: 3, reverifiedVersion: 4 }), { mode: 0o600 });
    console.log('EC2 role restart verification PASS');
  } else {
  let account;
  if (process.env.EC2_RESUME_ACCOUNT) {
    const key = process.env.EC2_RESUME_ACCOUNT;
    const result = await page.request.get(`${baseURL}/api/v1/driver-accounts/${encodeURIComponent(key)}`);
    if (result.status() !== 200) throw new Error('cannot resume registered account');
    account = await result.json();
    await page.goto(`${baseURL}/ui/platform/driver-accounts?account=${encodeURIComponent(key)}`);
  } else {
  const form = page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
  await form.getByLabel('AWS', { exact: true }).check();
  await form.getByLabel('Phương thức xác thực').selectOption('ec2-role');
  await expect(form.getByLabel('Access key ID')).toHaveCount(0);
  await form.getByLabel('Tên tài khoản', { exact: true }).fill('EC2 instance role');
  await form.getByLabel('IAM Role ARN', { exact: true }).fill(roleArn);
  await form.getByLabel('Vùng kiểm tra', { exact: true }).fill('us-east-1');
  const creation = page.waitForResponse((response) => response.url().endsWith('/api/v1/driver-accounts/aws/ec2-role') && response.request().method() === 'POST', { timeout: 120000 });
  await form.getByRole('button', { name: 'Kiểm tra và lưu', exact: true }).click();
  const response = await creation;
  if (response.status() !== 201) {
    const failure = await response.json();
    throw new Error(`EC2 registration rejected: ${response.status()} ${failure.code}`);
  }
  account = await response.json();
  }
  const detail = page.getByRole('region', { name: 'Chi tiết tài khoản EC2 instance role' });
  await expect(detail.locator('dt').filter({ hasText: /^IAM Role ARN$/ }).locator('..').locator('dd')).toHaveText(roleArn);
  await expect(detail.getByRole('group', { name: 'Thay credential' })).toHaveCount(0);
  await detail.getByLabel('Tên tài khoản', { exact: true }).fill('EC2 role verified');
  const rename = page.waitForResponse((response) => response.url().endsWith(`/api/v1/driver-accounts/${account.key}`) && response.request().method() === 'PUT');
  await detail.getByRole('button', { name: 'Lưu tên', exact: true }).click();
  if ((await rename).status() !== 200) throw new Error('rename failed');
  const updatedDetail = page.getByRole('region', { name: 'Chi tiết tài khoản EC2 role verified' });
  const verification = page.waitForResponse((response) => response.url().endsWith(`/api/v1/driver-accounts/${account.key}/verify`), { timeout: 120000 });
  await updatedDetail.getByRole('button', { name: 'Kiểm tra kết nối', exact: true }).click();
  const verified = await verification;
  if (verified.status() !== 200) throw new Error(`role reverify rejected ${verified.status()}`);
  account = await verified.json();
  if (account.version !== 3 || account.authenticationType !== 'AWS_EC2_ROLE' || !account.config.roleId) throw new Error('version/identity not preserved');
  const encoded = JSON.stringify(account);
  if (/secretRef|accessKeyId|secretAccessKey|sessionToken|"Token"/.test(encoded)) throw new Error('unsafe Account DTO');
  await writeFile(`${evidence}/account.json`, `${JSON.stringify(account, null, 2)}\n`, { mode: 0o600 });
  await page.reload();
  await expect(page.getByRole('region', { name: 'Chi tiết tài khoản EC2 role verified' })).toBeVisible();
  await page.screenshot({ path: `${evidence}/driver-accounts.png`, fullPage: true });
  const createForm = page.getByRole('group', { name: 'Đăng ký tài khoản mới' });
  await createForm.getByLabel('AWS', { exact: true }).check();
  await createForm.getByLabel('Phương thức xác thực').selectOption('ec2-role');
  await createForm.screenshot({ path: `${evidence}/ec2-role-form.png` });
  await writeFile(`${evidence}/browser-result.json`, `${JSON.stringify({ result: 'PASS', key: account.key, version: account.version, roleArn, roleId: account.config.roleId, authenticationType: account.authenticationType, assertions: ['UI register via EC2 role without keys', 'safe identity metadata', 'rename CAS', 'reverify version 3', 'no credential replacement', 'reload retains account'] }, null, 2)}\n`, { mode: 0o600 });
  console.log('EC2 role browser verification PASS');
  }
} finally {
  await browser.close();
}
