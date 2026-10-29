// Deploys the acceptance application (frontend, backend, worker with one
// shared PostgreSQL and one shared Redis) through the Web Console UI only:
// the developer creates the Application, selects the Secret Store, saves the
// Environment keys and the secret, imports three Score files, previews and
// deploys. Every step is a click or an entry on the screen; results are read
// from the screen. The deployed application is kept for the user.
//
// Optional ORCH_KUBE_CONTEXT opens the deployed frontend page through a local
// kubectl port-forward and checks its own page (5 checks PASS, one job DONE).
import { expect } from '@playwright/test';
import { spawn } from 'node:child_process';
import { createHash, randomBytes } from 'node:crypto';
import { mkdirSync, mkdtempSync, readFileSync, readdirSync, realpathSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from '@playwright/test';
import { ConsoleError, readConsoleConfig, readSecondAccount, signInContext, signOutContext } from '../../lib/console.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const env = process.env;
const WORKLOADS = ['backend', 'worker', 'frontend'];
const CHECKS = ['backend connection', 'environment', 'secret', 'database', 'cache'];
const minutes = (n) => n * 60_000;

function settings() {
  const platform = readConsoleConfig(env);
  const developer = readSecondAccount(platform, env, { usernameVar: 'ORCH_DEVELOPER_USERNAME', passwordFileVar: 'ORCH_DEVELOPER_PASSWORD_FILE', defaultUsername: 'developer' });
  const tag = (env.ORCH_ACCEPTANCE_IMAGE_TAG ?? '').trim();
  if (!/^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$/.test(tag)) throw new ConsoleError('Thiếu hoặc sai ORCH_ACCEPTANCE_IMAGE_TAG, ví dụ thieu-0eff925');
  const prefix = (env.ORCH_ACCEPTANCE_IMAGE_PREFIX ?? '').trim();
  const stamp = new Date().toISOString().slice(0, 16).replace(/[-:T]/g, '').slice(2);
  const appName = (env.ORCH_APP_NAME ?? `acceptance-${stamp}`).trim();
  if (!/^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$/.test(appName)) throw new ConsoleError('ORCH_APP_NAME chỉ gồm chữ thường, số và dấu gạch ngang');
  const secretStore = (env.ORCH_SECRET_STORE_NAME ?? 'K8S-4F Vault').trim();
  const delayMs = Number(env.ORCH_WORKLOAD_DELAY_MS ?? '2000');
  if (!Number.isInteger(delayMs) || delayMs < 0 || delayMs > 10000) throw new ConsoleError('ORCH_WORKLOAD_DELAY_MS phải là số nguyên 0..10000');
  const evidenceDir = env.ORCH_EVIDENCE_DIR ? resolve(env.ORCH_EVIDENCE_DIR) : '';
  if (evidenceDir) {
    mkdirSync(evidenceDir, { recursive: true, mode: 0o700 });
    const pathFromRepo = relative(realpathSync(resolve(here, '../../..')), realpathSync(evidenceDir));
    if (!pathFromRepo.startsWith('..' + '/') && pathFromRepo !== '..') throw new ConsoleError('ORCH_EVIDENCE_DIR phải nằm ngoài repository');
    if (readdirSync(evidenceDir).length) throw new ConsoleError('ORCH_EVIDENCE_DIR phải rỗng để không ghi đè evidence cũ');
  }
  return { developer, prefix, tag, appName, secretStore, delayMs, evidenceDir, kubeContext: (env.ORCH_KUBE_CONTEXT ?? '').trim() };
}

function writeScores(dir, { prefix, tag }) {
  for (const name of WORKLOADS) {
    const text = readFileSync(join(here, 'scores', `${name}.yaml`), 'utf8')
      .replace(/@@(\w+)_IMAGE@@/g, (_, w) => `${prefix}acceptance-${w.toLowerCase()}:${tag}`);
    writeFileSync(join(dir, `${name}.yaml`), text, { mode: 0o600 });
  }
}

let lastStep = 'Khởi tạo';
const step = (text) => { lastStep = text; console.log(`- ${text}`); };

async function createApplication(page, name) {
  await page.getByRole('button', { name: '+ Tạo ứng dụng' }).first().click();
  await page.getByLabel('Tên ứng dụng').fill(name);
  await page.getByLabel('Subdomain').fill(name);
  await page.getByRole('button', { name: 'Tạo ứng dụng' }).click();
  await expect(page.getByRole('heading', { name })).toBeVisible();
  const id = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!id) throw new ConsoleError('Không đọc được ID ứng dụng từ địa chỉ trang');
  return id;
}

async function selectSecretStore(page, storeName) {
  await page.getByRole('tab', { name: 'Staging' }).click();
  const select = page.getByLabel('Kho bí mật cho Staging');
  await expect(select).toBeVisible();
  let label = '';
  await expect.poll(async () => {
    label = (await select.locator('option').allTextContents()).find((text) => text.startsWith(`${storeName} (`)) ?? '';
    return label;
  }, { message: `kho bí mật "${storeName}" chưa có trong danh sách chọn` }).not.toBe('');
  const save = page.getByRole('button', { name: 'Lưu kho bí mật' });
  if ((await select.locator('option:checked').textContent()) === label && await save.isDisabled()) return;
  await select.selectOption({ label });
  await save.click();
  await expect(save).toBeDisabled();
}

async function saveKey(page, kind, name, value) {
  const section = page.locator(`section[aria-label="${kind === 'secret' ? 'Bí mật' : 'Biến'}"]`);
  await section.getByRole('button', { name: kind === 'secret' ? '+ Thêm bí mật' : '+ Thêm biến' }).click();
  await page.getByLabel('Tên key').fill(name);
  const input = page.getByLabel(kind === 'secret' ? 'Giá trị bí mật mới' : 'Giá trị');
  if (kind === 'secret') await expect(input).toHaveAttribute('type', 'password');
  await input.fill(value);
  const [saved] = await Promise.all([
    page.waitForResponse(r => r.request().method() === 'PUT' && /\/configuration\/keys\//.test(new URL(r.url()).pathname), { timeout: 120_000 }),
    page.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }).click(),
  ]);
  if (!saved.ok()) throw new ConsoleError(`Không lưu được key ${name}: HTTP ${saved.status()}`);
  await expect(page.getByLabel('Tên key')).toHaveCount(0);
  const row = section.locator('.settings-table-row').filter({ has: page.locator('strong').getByText(name, { exact: true }) });
  await expect(row).toContainText(kind === 'secret' ? 'Đã cấu hình' : value);
}

// Show each imported workload and its form at a readable pace. Import uses
// the same product validation as form authoring; no direct draft/API writes.
async function importScore(page, file, name, delayMs) {
  const act = async (locator, method, ...args) => {
    step(`Workload ${name}: ${method} ${locator.toString()}`);
    await locator.scrollIntoViewIfNeeded();
    await locator.hover();
    await page.waitForTimeout(delayMs);
    await locator[method](...args);
    await page.waitForTimeout(delayMs);
  };
  await act(page.getByRole('button', { name: '+ Thêm Workload', exact: true }), 'click');
  const custom = page.getByRole('button', { name: /Tự cấu hình \/ nhập Score/ });
  if (await custom.count()) await act(custom, 'click');
  await act(page.getByRole('button', { name: 'Nhập Score', exact: true }), 'click');
  await page.getByLabel('File Score', { exact: true }).setInputFiles(file);
  await expect(page.getByText(`Đã phân tích Workload: ${name}`, { exact: false })).toBeVisible();
  await page.waitForTimeout(delayMs * 2);
  // Return to the product form to show the imported image, variables and dependencies.
  const form = page.getByRole('button', { name: /Nhập trên form|Nhập bằng form/ });
  if (await form.count() && await form.isEnabled()) {
    await act(form, 'click');
    await expect(page.getByLabel('Tên Workload', { exact: true })).toHaveValue(name);
    const image = page.getByLabel('Ảnh Container', { exact: true });
    await image.scrollIntoViewIfNeeded();
    await page.waitForTimeout(delayMs * 2);
    const table = page.getByRole('table').first();
    if (await table.count()) { await table.scrollIntoViewIfNeeded(); await page.waitForTimeout(delayMs * 2); }
  } else {
    // Older Consoles keep advanced Score fields in import mode. Read the actual
    // parsed Score instead of forcing a lossy form conversion.
    const score = page.locator('pre');
    if (await score.count()) {
      await score.scrollIntoViewIfNeeded();
      await score.evaluate(node => { node.scrollTop = 0; });
      await page.waitForTimeout(delayMs * 2);
      await score.evaluate(node => { node.scrollTop = node.scrollHeight / 2; });
      await page.waitForTimeout(delayMs * 2);
      await score.evaluate(node => { node.scrollTop = node.scrollHeight; });
      await page.waitForTimeout(delayMs * 2);
    }
  }
  await act(page.getByRole('button', { name: 'Lưu draft Workload', exact: true }), 'click');
  await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
}

// Local port-forward to the deployed frontend Service; returns { url, stop }.
async function forwardFrontend(context, namespace) {
  const child = spawn('kubectl', ['--context', context, '-n', namespace, 'port-forward', 'svc/frontend', ':8080'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let port;
  try {
    port = await new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new ConsoleError('kubectl port-forward không sẵn sàng sau 30 giây')), 30_000);
      child.stdout.on('data', (chunk) => {
        const match = String(chunk).match(/127\.0\.0\.1:(\d+)/);
        if (match) { clearTimeout(timer); resolve(match[1]); }
      });
      child.on('exit', () => { clearTimeout(timer); reject(new ConsoleError('kubectl port-forward dừng; kiểm tra ORCH_KUBE_CONTEXT')); });
      child.on('error', () => { clearTimeout(timer); reject(new ConsoleError('Không chạy được kubectl port-forward')); });
    });
  } catch (error) {
    child.kill();
    throw error;
  }
  return { url: `http://127.0.0.1:${port}`, stop: () => child.kill() };
}

async function waitForChecks(app) {
  const deadline = Date.now() + minutes(5);
  for (;;) {
    await app.reload();
    const rows = await app.locator('#checks tr[data-check]').evaluateAll((trs) => trs.map((tr) => [tr.getAttribute('data-check'), tr.children[1].textContent.trim()]));
    const passed = CHECKS.filter((name) => rows.some(([n, s]) => n === name && s === 'PASS'));
    if (passed.length === CHECKS.length) return rows;
    if (Date.now() > deadline) throw new ConsoleError(`Trang ứng dụng chưa PASS đủ: ${rows.map(([n, s]) => `${n}=${s}`).join(', ')}`);
    await app.waitForTimeout(3000);
  }
}

async function main() {
  const cfg = settings();
  const scoreDir = mkdtempSync(join(tmpdir(), 'orch-acceptance-'));
  const browser = await chromium.launch({ headless: cfg.developer.headless });
  let session;
  let forward;
  let appPage;
  const resultSummary = { appName: cfg.appName, imageTag: cfg.tag, status: 'FAIL', assertions: [] };
  try {
    writeScores(scoreDir, cfg);
    console.log(`Console: ${cfg.developer.baseURL}`);
    console.log(`Ứng dụng: ${cfg.appName}; image ${cfg.prefix}acceptance-*:${cfg.tag}`);
    session = await signInContext(browser, cfg.developer, { role: 'DEVELOPER', ...(cfg.evidenceDir ? { recordVideo: { dir: join(cfg.evidenceDir, 'raw'), size: { width: 1440, height: 1000 } } } : {}) });
    const { page } = session;
    page.setDefaultTimeout(60_000);
    step(`Đăng nhập ${session.user.username}`);

    if (await page.getByRole('button', { name: new RegExp(`^${cfg.appName}\\b`) }).count()) {
      throw new ConsoleError(`Ứng dụng ${cfg.appName} đã có; đặt ORCH_APP_NAME khác (script không sửa ứng dụng cũ)`);
    }
    const appId = await createApplication(page, cfg.appName);
    resultSummary.appId = appId;
    resultSummary.url = page.url();
    step(`Tạo ứng dụng ${cfg.appName} (ID ${appId})`);

    const secret = randomBytes(24).toString('hex');
    const variables = [
      ['ACCEPTANCE_CACHE_APP', appId], ['ACCEPTANCE_CACHE_ENV', 'staging'], ['ACCEPTANCE_CACHE_RUN', cfg.appName],
      ['ACCEPTANCE_CACHE_TTL_SECONDS', '30'], ['ACCEPTANCE_CONFIG', 'acceptance-config-ok'],
      ['ACCEPTANCE_SECRET_SHA256', createHash('sha256').update(secret).digest('hex')], ['POLL_INTERVAL_MS', '500'], ['PORT', '8080'],
    ];
    await page.getByRole('button', { name: 'Cài đặt môi trường' }).click();
    await selectSecretStore(page, cfg.secretStore);
    step(`Chọn kho bí mật "${cfg.secretStore}" cho staging`);
    for (const [name, value] of variables) await saveKey(page, 'variable', name, value);
    await saveKey(page, 'secret', 'ACCEPTANCE_SECRET', secret);
    if ((await page.locator('body').innerText()).includes(secret)) throw new ConsoleError('Giá trị bí mật hiển thị trên trang; dừng');
    step(`Lưu ${variables.length} biến và 1 bí mật (giá trị bí mật không hiển thị)`);
    await page.getByRole('button', { name: `← ${cfg.appName}` }).click();
    await expect(page.getByRole('heading', { name: cfg.appName })).toBeVisible();

    for (const name of WORKLOADS) {
      await importScore(page, join(scoreDir, `${name}.yaml`), name, cfg.delayMs);
      step(`Nhập Score ${name}`);
    }

    await page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }).click();
    const preview = page.getByLabel('Xem trước triển khai');
    await expect(preview).toContainText(`${WORKLOADS.length} Workload bị ảnh hưởng`, { timeout: minutes(2) });
    step(`Xem trước: ${WORKLOADS.length} Workload bị ảnh hưởng`);

    await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
    step('Triển khai... (thường 3-10 phút)');
    const result = page.getByLabel('Kết quả triển khai');
    await expect(result).toContainText(/Triển khai (thành công|thất bại)/, { timeout: minutes(25) });
    if (!(await result.innerText()).includes('Triển khai thành công')) {
      throw new ConsoleError(`Triển khai thất bại: ${(await result.innerText()).split('\n').slice(0, 4).join(' | ')}`);
    }
    for (const name of WORKLOADS) await expect(result.locator('li').filter({ hasText: new RegExp(`^${name} · [^:]+: thành công`) })).toHaveCount(1);
    step('Triển khai thành công: backend, worker, frontend');
    resultSummary.assertions.push('Preview: 3 workloads', 'Deployment succeeded: backend, worker, frontend');

    if (!cfg.kubeContext) {
      console.log('Bỏ qua mở trang ứng dụng: chưa đặt ORCH_KUBE_CONTEXT');
    } else {
      const namespace = env.ORCH_APP_NAMESPACE ?? `app-${appId}-staging`;
      forward = await forwardFrontend(cfg.kubeContext, namespace);
      const app = await session.context.newPage();
      appPage = app;
      await app.goto(`${forward.url}/`);
      await expect(app.getByRole('heading', { name: 'Acceptance application' })).toBeVisible();
      const rows = await waitForChecks(app);
      resultSummary.assertions.push(...CHECKS.map(name => `${name}: PASS`));
      step(`Trang ứng dụng: ${rows.map(([n, s]) => `${n} ${s}`).join(', ')}`);
      const payload = `hello ${cfg.appName}`;
      await app.locator('input[name="payload"]').fill(payload);
      await app.getByRole('button', { name: 'Submit job' }).click();
      const job = app.locator('#jobs tr.job').filter({ hasText: payload }).first();
      const deadline = Date.now() + minutes(4);
      while (!(await job.innerText().catch(() => '')).includes('DONE')) {
        if (Date.now() > deadline) throw new ConsoleError('Job chưa DONE sau 4 phút');
        await app.waitForTimeout(1500);
        await app.reload();
      }
      resultSummary.assertions.push('Job: DONE');
      if (cfg.evidenceDir) await app.screenshot({ path: join(cfg.evidenceDir, 'application.png'), fullPage: true });
      await app.waitForTimeout(cfg.delayMs * 3);
      step(`Gửi job "${payload}": DONE (worker xử lý, ghi PostgreSQL và Redis)`);
    }
    resultSummary.status = 'PASS';
    console.log(`PASS: acceptance app ${cfg.appName} đã chạy trên ${cfg.developer.baseURL}/ui/applications/${appId}`);

  } catch (error) {
    resultSummary.error = String(error?.message ?? error).split('\n')[0];
    throw error;
  } finally {
    resultSummary.lastStep = lastStep;
    forward?.stop();
    rmSync(scoreDir, { recursive: true, force: true });
    try {
      if (cfg.evidenceDir) writeFileSync(join(cfg.evidenceDir, 'result.json'), JSON.stringify(resultSummary, null, 2));
      if (session && browser.isConnected()) {
        if (cfg.evidenceDir) await session.page.screenshot({ path: join(cfg.evidenceDir, 'console.png'), fullPage: true }).catch(() => {});
        await signOutContext(session.context, cfg.developer);
        const consoleVideo = session.page.video();
        const appVideo = appPage?.video();
        await session.context.close(); // flush and export before closing browser
        if (cfg.evidenceDir) {
          await consoleVideo?.saveAs(join(cfg.evidenceDir, 'console.webm'));
          await appVideo?.saveAs(join(cfg.evidenceDir, 'application.webm'));
          console.log(`Evidence: ${cfg.evidenceDir}`);
        }
      }
    } finally { await browser.close().catch(() => {}); }
  }
}

try {
  await main();
} catch (error) {
  console.error(`FAIL: ${String(error?.message ?? error).split('\n')[0]}`);
  process.exitCode = 1;
}
