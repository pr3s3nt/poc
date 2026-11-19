import assert from 'node:assert/strict';
import { readFileSync, mkdirSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { createRequire } from 'node:module';
const require = createRequire(new URL('../../../frontend/package.json', import.meta.url));
const { chromium } = require('@playwright/test');
const url = process.env.SETUP_URL;
const evidence = resolve(process.env.SETUP_EVIDENCE ?? '');
assert(url && process.env.SETUP_EVIDENCE && !evidence.startsWith(resolve('.') + '/'), 'Explicit URL and private evidence outside Git required');
for (const name of ['SETUP_TOKEN_FILE', 'SETUP_PASSWORD_FILE', 'SETUP_GIT_URL', 'SETUP_GIT_REF', 'SETUP_GIT_PATH']) assert(process.env[name], name + ' required');
const token = readFileSync(process.env.SETUP_TOKEN_FILE, 'utf8').trim();
const password = readFileSync(process.env.SETUP_PASSWORD_FILE, 'utf8').trim();
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const browser = await chromium.launch({ headless: true });
const page = await browser.newPage({ viewport: { width: 1440, height: 1000 }, locale: 'vi-VN' });
await page.addInitScript(() => window.localStorage.setItem('orchestrator.locale', 'vi'));
page.on('response', async response => { if (response.url().endsWith('/platform/setup/preview')) { const data = await response.json().catch(() => ({})); console.log('PREVIEW', response.status(), JSON.stringify(data.documents?.map(doc => ({ file: doc.file, status: doc.status, fields: doc.fields })) ?? { code: data.code })); } });
const receipt = { url, assertions: [], screenshots: [] };
const check = name => { receipt.assertions.push(name); console.log(name); };
const screenshot = async name => { await page.screenshot({ path: join(evidence, name + '.png'), fullPage: true }); receipt.screenshots.push(name + '.png'); };
try {
 await page.goto(url + '/ui/');
 const config = await (await page.request.get(url + '/api/v1/auth/config')).json();
 if (config.setupAvailable) {
  await page.getByLabel(/^Mã setup/).fill(token);
  await page.getByLabel('Mã Organization', { exact: true }).fill('setup-trial');
  await page.getByLabel('Tên Organization', { exact: true }).fill('Setup Trial');
  await page.getByLabel('Tên đăng nhập Admin', { exact: true }).fill('setup-admin');
  await page.getByLabel(/^Mật khẩu Admin/).fill(password);
  await page.getByLabel(/^Xác nhận mật khẩu/).fill(password);
  await page.getByRole('button', { name: 'Khởi tạo Console', exact: true }).click();
  await page.getByRole('button', { name: 'Đăng nhập', exact: true }).waitFor();
  check('Organization and first Admin initialized through browser');
 }
 await page.getByLabel('Tên đăng nhập', { exact: true }).fill('setup-admin');
 await page.getByLabel(/^Mật khẩu/).fill(password);
 await page.getByRole('button', { name: 'Đăng nhập', exact: true }).click();
 await page.getByRole('link', { name: 'Thiết lập platform', exact: true }).click();
 await page.getByRole('heading', { name: 'Thiết lập platform', exact: true }).waitFor();
 const status = await (await page.request.get(url + '/api/v1/setup')).json();
 assert.equal(status.available, false); check('Initialization remains closed after sign-in');
 await screenshot('platform-guide');
 // Exercise navigation to every reused product surface without saving credentials.
 for (const [step, heading] of [['2. Kubernetes', 'Tài khoản hạ tầng'], ['3. Kho bí mật', 'Cấu hình kho bí mật'], ['4. CD', 'Cấu hình CD']]) {
  await page.getByRole('button', { name: new RegExp('^' + step.replace('.', '\\.')) }).click();
  await page.getByRole('heading', { name: heading, exact: true }).waitFor();
 }
 check('Kubernetes, Secret Store and CD product forms load in guide');
 await page.getByRole('button', { name: /^5\. Types\/Definitions/ }).click();
 await page.getByLabel('URL repository Git', { exact: true }).fill(process.env.SETUP_GIT_URL);
 await page.getByLabel('Branch, tag hoặc commit', { exact: true }).fill(process.env.SETUP_GIT_REF);
 await page.getByLabel('Thư mục chứa Type/Definition', { exact: true }).fill(process.env.SETUP_GIT_PATH);
 await page.getByRole('button', { name: 'Đọc repository', exact: true }).click();
 await page.getByRole('button', { name: 'Kiểm tra trước khi nhập', exact: true }).waitFor({ timeout: 180000 });
 await page.getByText(/workload\/workload-type\.yaml/).waitFor();
 await page.getByText(/workload\/workload-definition\.yaml/).waitFor();
 check('User-supplied Git source discovers workload Type and Definition');
 await screenshot('git-discovery');
 // Import all Types and Definitions that do not need account/image substitutions.
 // Terraform/template inspection is real; registration never deploys workloads.
 const boxes = page.getByRole('checkbox');
 for (let i = 0; i < await boxes.count(); i++) {
  const box = boxes.nth(i);
  if (await box.isEnabled()) { const name = await box.locator('..').innerText(); await box.setChecked(!['cluster/cluster-definition.yaml', 'mysql/mysql-definition.yaml', 'mongodb/mongodb-definition.yaml'].some(file => name.includes(file))); }
 }
 await page.getByLabel('Thư mục template', { exact: true }).fill('templates/standard');
 await page.getByRole('button', { name: 'Kiểm tra trước khi nhập', exact: true }).click();
 await page.getByRole('button', { name: 'Xác nhận nhập tài nguyên', exact: true }).waitFor({ timeout: 180000 });
 await screenshot('import-preview');
 await page.getByRole('button', { name: 'Xác nhận nhập tài nguyên', exact: true }).click();
 await page.getByRole('cell', { name: /Đã tạo|Dùng lại/ }).first().waitFor();
 await page.getByRole('button', { name: 'Xác nhận nhập tài nguyên', exact: true }).waitFor({ state: 'visible' });
 await page.waitForFunction(() => ![...document.querySelectorAll('button')].some(button => button.textContent === 'Đang xử lý…'));
 check('Confirmed import creates/reuses 10 Types and 8 Definitions with real Terraform/template sources');
 await page.getByRole('button', { name: 'Kiểm tra trước khi nhập', exact: true }).click();
 await page.getByRole('cell', { name: 'Dùng lại', exact: true }).first().waitFor({ timeout: 180000 });
 assert.equal(await page.getByRole('cell', { name: 'Dùng lại', exact: true }).count(), 18);
 check('Rerun preview reuses 10 Types and 8 Definitions');
 await screenshot('import-reused');
 await page.reload();
 await page.getByRole('heading', { name: 'Thiết lập platform', exact: true }).waitFor();
 const progress = await (await page.request.get(url + '/api/v1/platform/setup')).json();
 assert.equal(progress.types, 10); assert.equal(progress.definitions, 8);check('Reload restores server configuration progress');
 receipt.progress = progress;
 writeFileSync(join(evidence, 'receipt.json'), JSON.stringify(receipt, null, 2), { mode: 0o600 });
} catch (error) { await screenshot('failure'); throw error; } finally { await browser.close(); }
