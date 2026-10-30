// Product mutations use ordinary Console forms only. Native reads verify the
// real provider; private fixture files/evidence are never repository artifacts.
import { expect } from '@playwright/test';
import { readFileSync, writeFileSync, mkdirSync, appendFileSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { execFileSync } from 'node:child_process';
import { openConsole, closeConsole, readConsoleConfig, readPrivateFile, ConsoleError } from '../../lib/console.mjs';
import { runBusiness } from './business.mjs';
import { runExisting } from './existing.mjs';
import { prepareSemantics, runSemantics } from './semantics.mjs';
import { runInfrastructureDrift } from '../infrastructure-drift/flow.mjs';

const config = readConsoleConfig();
const run = resolve(process.env.ORCH_RECON_RUN_DIR ?? '');
if (!process.env.ORCH_RECON_RUN_DIR || run.includes('/projects/thieu/poc/')) throw new ConsoleError('An absolute private run directory outside Git is required');
const statePath = join(run, process.env.ORCH_RECON_STATE_FILE ?? 'ui-state.json');
if (resolve(statePath).slice(0, run.length + 1) !== `${run}/`) throw new ConsoleError('State file must remain within the private run directory');
let state;
try { state = JSON.parse(readFileSync(statePath, 'utf8')); } catch (err) { if (err.code !== 'ENOENT') throw err; state = { runID: 'reconciliation-20261029', completed: [] }; }
const saveState = () => writeFileSync(statePath, JSON.stringify(state, null, 2), { mode: 0o600 });
const kubeconfig = join(run, 'kubeconfig');
const kubectl = (...args) => execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
const clusterName = readPrivateFile(join(run, 'cluster-name'));
const containerID = execFileSync('docker', ['inspect', '--format', '{{.Id}}', `${clusterName}-control-plane`], { encoding: 'utf8' }).trim();
if (!clusterName.startsWith('orch-reconciliation-') || containerID !== readPrivateFile(join(run, 'container-id'))) throw new ConsoleError('Owned cluster incarnation changed');
const systemNS = JSON.parse(kubectl('get', 'namespace', 'kube-system', '-o', 'json'));
if (systemNS.metadata.uid !== readPrivateFile(join(run, 'kube-system-uid'))) throw new ConsoleError('Owned cluster UID changed');
const evidence = join(run, 'ui-evidence'); mkdirSync(evidence, { recursive: true, mode: 0o700 });
const attempt = new Date().toISOString().replaceAll(':', '-');
const record = message => { console.log(message); appendFileSync(join(evidence, 'attempts.log'), `${attempt} ${message}\n`, { mode: 0o600 }); };
const writer = readPrivateFile(join(run, 'git-writer-auth')).split('\n');
const gitCA = readPrivateFile(join(run, 'git-tls', 'ca.pem'));
const accountName = process.env.ORCH_ACCOUNT_NAME ?? 'Reconciliation cluster';
const managedName = 'Fleet managed reconciliation';
let session;
const step = async (name, fn) => { record(`RUN ${name}`); await fn(); state.completed = [...new Set([...state.completed, name])]; saveState(); record(`PASS ${name}`); };
try {
  session = await openConsole(config, { role: 'PLATFORM_ENGINEER' });
  const { page } = session;
  page.on('pageerror', () => record('BROWSER_ERROR: Console runtime exception; inspect private progress screenshot'));
  const snapshotTimer = setInterval(() => { void page.screenshot({ path: join(evidence, `progress-${attempt}.png`), fullPage: true }).catch(() => {}); }, 15_000);
  page.on('close', () => clearInterval(snapshotTimer));
  page.setDefaultTimeout(30_000);
  await step('CD configuration: create and verify managed GitRepo through forms', async () => {
    await page.getByRole('link', { name: 'Cấu hình CD', exact: true }).click();
    await expect(page.getByRole('heading', { name: 'Cấu hình CD', level: 1 })).toBeVisible();
    await expect(page.getByRole('heading', { name: 'Cấu hình đã đăng ký', exact: true })).toBeVisible();
    const row = () => page.getByRole('row').filter({ has: page.getByRole('cell', { name: managedName, exact: true }) }).first();
    if (!(await row().count())) {
      await page.getByRole('button', { name: 'Thêm cấu hình CD', exact: true }).click();
      await page.getByLabel('Tên cấu hình', { exact: true }).fill(managedName);
      await page.getByLabel('Tài khoản quản lý CD', { exact: true }).selectOption({ label: accountName });
      await page.getByLabel('Tài khoản cluster chạy workload', { exact: true }).selectOption({ label: accountName });
      await page.getByRole('button', { name: 'Tìm cấu hình trên cluster', exact: true }).click();
      await expect(page.getByLabel('Định danh cluster đã xác minh', { exact: true })).not.toHaveValue('');
      await page.getByLabel('Git repository HTTPS', { exact: true }).fill('https://172.18.0.1:19443/repo.git');
      await page.getByLabel('Nhánh Git', { exact: true }).fill('dev');
      await page.getByLabel('Thư mục manifest được quản lý', { exact: true }).fill('generated-managed');
      await page.getByText('Chi tiết kết nối', { exact: true }).click();
      await page.getByLabel('Namespace agent', { exact: true }).fill('cattle-fleet-system');
      await page.getByLabel('ServiceAccount triển khai', { exact: true }).fill('orch-recon-delivery');
      await page.getByLabel('Namespace lưu Helm release', { exact: true }).fill('orch-recon-releases');
      await page.getByLabel('CA của Git (PEM)', { exact: true }).fill(gitCA);
      await page.getByText('Thông tin ghi Git', { exact: true }).click();
      await page.getByLabel('Tên đăng nhập Git', { exact: true }).fill(writer[0]);
      await page.getByLabel('Token hoặc mật khẩu Git', { exact: true }).fill(writer[1]);
      const saved = page.waitForResponse(r => r.request().method() === 'POST' && new URL(r.url()).pathname === '/api/v1/delivery-bindings');
      await page.getByRole('button', { name: 'Lưu cấu hình', exact: true }).click();
      const reply = await saved;
      if (reply.status() !== 201) throw new ConsoleError(`CD registration failed: HTTP ${reply.status()}`);
      const body = await reply.json(); state.managedBindingID = body.id; state.managedBindingVersion = body.version; saveState();
      await expect(page.getByLabel('Token hoặc mật khẩu Git', { exact: true })).toHaveCount(0);
    }
    await expect(row()).toHaveCount(1);
    if (!(await row().getByRole('cell', { name: 'Sẵn sàng', exact: true }).count())) {
      await row().getByRole('button', { name: 'Xác minh', exact: true }).click();
      await expect(row().getByRole('cell', { name: 'Sẵn sàng', exact: true })).toBeVisible({ timeout: 120_000 });
    }
    const detailReply = page.waitForResponse(r => r.request().method() === 'GET' && /\/delivery-bindings\/[^/]+$/.test(new URL(r.url()).pathname));
    await row().getByRole('button', { name: 'Xem chi tiết', exact: true }).click();
    const details = await (await detailReply).json();
    state.managedBindingID = details.binding.id; state.managedBindingVersion = details.binding.version; saveState();
    await page.screenshot({ path: join(evidence, 'cd-managed-ready.png'), fullPage: true });
  });
  await step('Platform catalog registration through typed forms', async () => {
    if(state.semanticRendererPrepared)return;
    const reply = await page.request.get(`${config.baseURL}/api/v1/driver-accounts`);
    const accounts = (await reply.json()).accounts;
    const account = accounts.find(a => a.name === accountName && a.status === 'READY');
    if (!account) throw new ConsoleError('The UI-registered task cluster account is unavailable');
    state.accountKey = account.key; saveState();
    // Existing setup runner stays at its historical path and runs in a separate
    // process so its frontend Playwright dependency cannot load a second copy
    // into the new root E2E runner. It uses typed Console forms only.
    execFileSync('node', ['frontend/test/e2e/bootstrap-staging-catalog.mjs'], { env: { ...process.env, ORCH_ACCOUNT_KEY: account.key }, stdio: ['ignore', 'pipe', 'pipe'] });
  });
  if(process.env.ORCH_RECON_FOCUS === 'semantics') await step('Upload lifecycle render bundle through Console',()=>prepareSemantics({page,baseURL:config.baseURL,run,state,saveState}));
  const business = await runBusiness({ page, context: session.context, baseURL: config.baseURL, run, state, saveState, step });
  if(process.env.ORCH_RECON_FOCUS === 'infrastructure') await runInfrastructureDrift({page,baseURL:config.baseURL,run,state,saveState,step,business});
  else if(process.env.ORCH_RECON_FOCUS === 'semantics') await runSemantics({page,baseURL:config.baseURL,run,state,saveState,step,business});
  else await runExisting({page,baseURL:config.baseURL,run,state,saveState,step,business,ca:gitCA,writer,accountName});
} catch (err) {
  if (session) await session.page.screenshot({ path: join(evidence, `failure-${attempt}.png`), fullPage: true }).catch(() => {});
  record(`FAIL ${err.message.split('\n')[0]}`); process.exitCode = 1;
} finally { if (session) await closeConsole(session, config); }
