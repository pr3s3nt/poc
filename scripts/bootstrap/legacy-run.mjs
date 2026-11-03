import { assertSameResourceType, parseResourceTypes } from './lib/catalog-config.mjs';
import { ensureInfrastructureAccount } from './lib/infrastructure-ui.mjs';
import { parseCatalogConfig } from './lib/catalog-config.mjs';
import { ensureCatalog } from './lib/catalog-ui.mjs';
import { parseGitAccess } from './lib/git-access-config.mjs';
import { ensureGitAccess } from './lib/git-access-ui.mjs';
import { parseArgs } from 'node:util';
import { ensureTypes } from './lib/platform.mjs';
import { closeConsole, openConsole, readConsoleConfig } from './lib/console.mjs';
import { loadTarget } from './lib/target-config.mjs';
import { parseInfrastructure, readSetupPacing } from './lib/infrastructure-config.mjs';

const { values } = parseArgs({ options: { config: { type: 'string' }, target: { type: 'string' }, url: { type: 'string' }, 'git-url': { type: 'string' }, 'update-git-access': { type: 'boolean' }, kubeconfig: { type: 'string' }, help: { type: 'boolean' }, ...Object.fromEntries(['step', 'account', 'type', 'definition', 'bundle'].map(phase => [`${phase}-delay-ms`, { type: 'string' }])) } });
if (values.help) {
  console.log('node scripts/bootstrap/run.mjs --target dev|staging [--url https://console.example] [--kubeconfig /private/kubeconfig] --git-url https://git.company/catalog.git [--update-git-access] OR --config /private/bootstrap.json');
  process.exit(0);
}

let session, consoleConfig, lastAction = '';
try {
  const target = loadTarget(values);
  const config = { ...target.raw, ...parseInfrastructure(target.raw), resourceTypes: parseResourceTypes(target.raw.resourceTypes ?? []) };
  parseCatalogConfig(target.raw, values);
  const gitAccess = parseGitAccess(target.raw, values);
  if (values['update-git-access'] && !gitAccess) throw new Error('--update-git-access cần gitAccess đang bật trong cấu hình');
  if (gitAccess && (process.env.DEBUG || process.env.NODE_DEBUG || process.env.PWDEBUG && process.env.PWDEBUG !== '0')) throw new Error('Git access: tắt DEBUG, NODE_DEBUG và PWDEBUG trước khi nhập token');
  const pacing = readSetupPacing(values, target.env);
  let phase = 'account';
  consoleConfig = readConsoleConfig(target.env, { defaultPassword: target.defaultPassword });
  console.log(`Console: ${consoleConfig.baseURL}`);
  session = await openConsole(consoleConfig);
  if (!['PLATFORM_ENGINEER', 'ADMIN'].includes(session.user.role)) throw new Error('Cần tài khoản Platform Engineer hoặc Admin');
  const { page } = session;
  const act = async (locator, method, ...args) => {
    lastAction = `${phase}: ${method} ${locator.toString()}`;
    const stepDelay = pacing[phase];
    const visible = method === 'setInputFiles' ? locator.locator('..') : locator;
    await visible.scrollIntoViewIfNeeded();
    await visible.hover();
    await page.waitForTimeout(stepDelay);
    if (method === 'fill') {
      await locator.fill('');
      await locator.pressSequentially(args[0], { delay: Math.min(45, Math.floor(stepDelay / 20)) });
    } else await locator[method](...args);
    await page.waitForTimeout(stepDelay);
  };
  const get = async path => {
    const reply = await page.request.get(`${consoleConfig.baseURL}/api/v1${path}`);
    if (!reply.ok()) throw new Error(`GET ${path}: HTTP ${reply.status()}`);
    return reply.json();
  };
  const responseFor = (path, method) => page.waitForResponse(reply => reply.request().method() === method && new URL(reply.url()).pathname === `/api/v1${path}`, { timeout: 180_000 });
  const readTypes = async () => {
    const { resourceTypes } = await get('/resource-types');
    if (!Array.isArray(resourceTypes)) throw new Error('Danh sách Resource Types không hợp lệ');
    return resourceTypes;
  };
  // Check existing schemas before any create, including before bundle inspection.
  const existingTypes = await readTypes();
  for (const type of config.resourceTypes) {
    const existing = existingTypes.find(item => item.key === type.key);
    if (existing) assertSameResourceType(existing, type);
  }
  const ui = { page, baseURL: consoleConfig.baseURL, act, get, responseFor };
  await ensureGitAccess({ ...ui, desired: gitAccess, update: values['update-git-access'] === true });
  await ensureInfrastructureAccount({ ...ui, account: config.account, kubeconfigFile: target.env.ORCH_KUBECONFIG_FILE });
  phase = 'type';
  await ensureTypes(ui,config.resourceTypes,{record:(step,status)=>console.log(`${status}: ${step}`)});
  phase = 'bundle';
  await ensureCatalog({ ...ui, source: parseCatalogConfig(target.raw, values) });
} catch (error) {
  // Raw Playwright errors can include filled values; do not log them.
  console.error(`FAIL: ${error.name === 'Error' ? error.message.split('\n')[0] : 'Browser assertion/timeout thất bại; kiểm tra Console và cấu hình'}`);
  if (lastAction) console.error(`UI action: ${lastAction}`);
  process.exitCode = 1;
} finally {
  if (session) await closeConsole(session, consoleConfig);
}
