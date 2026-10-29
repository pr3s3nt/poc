// Real kind acceptance: every product mutation is a UI click/fill. No Score import,
// direct deploy API or kubectl workload apply. Only technical verification and a
// port-forward use kubectl. Run-owned app remains for review, never touches older apps.
import { chromium, expect } from '@playwright/test';
import { appendFile, mkdir, writeFile } from 'node:fs/promises';
import { createHash, randomBytes } from 'node:crypto';
import { spawn, execFileSync } from 'node:child_process';
import { readConsoleConfig, signInContext, signOutContext } from '../../lib/console.mjs';
import { deleteApplicationViaUI } from '../../lib/delete-application.mjs';

const config = readConsoleConfig();
const run = process.env.ORCH_ENV_RUN ?? `env-form-${Date.now()}`;
const evidence = process.env.ORCH_EVIDENCE_DIR;
const tag = process.env.ORCH_ACCEPTANCE_IMAGE_TAG;
const kube = process.env.ORCH_KUBE_CONTEXT ?? 'kind-k8s-4f';
if (!evidence || !tag || !/^[a-z0-9-]{1,40}$/.test(run)) throw Error('Set ORCH_EVIDENCE_DIR (outside Git), ORCH_ACCEPTANCE_IMAGE_TAG and valid ORCH_ENV_RUN');
await mkdir(evidence, { recursive: true, mode: 0o700 });
const browser = await chromium.launch({ headless: config.headless });
const session = await signInContext(browser, config, { role: 'DEVELOPER', recordVideo: { dir: evidence, size: { width: 1440, height: 1000 } } });
const { page, context } = session;
page.setDefaultTimeout(30_000);
const assertions = [], phases = [], errors = [];
let appId, forward, appPage, appVideo, outcome = 'FAIL';
const video = page.video();
page.on('pageerror', (error) => errors.push(error.message));
const mark = async (phase) => { phases.push({ phase, at: new Date().toISOString() }); console.log(phase); await page.screenshot({ path: `${evidence}/${String(phases.length).padStart(2, '0')}.png`, fullPage: true }); };
const check = (label) => { assertions.push(label); console.log(`PASS ${label}`); };
const click = (name) => page.getByRole('button', { name, exact: true }).click();
const table = () => page.getByRole('region', { name: 'Biến môi trường của container main', exact: true });
const rows = () => table().locator('tbody tr');
async function row(name) { await table().getByRole('button', { name: '+ Thêm biến', exact: true }).click(); const entry = rows().last(); await entry.getByRole('textbox').first().fill(name); return entry; }
async function literal(name, value) { const entry = await row(name); await entry.getByRole('textbox').nth(1).fill(value); return entry; }
async function picker(entry) { await entry.getByRole('button', { name: /^(Chọn|Đổi) nguồn, dòng/ }).click(); return page.getByRole('dialog', { name: 'Chọn giá trị có sẵn', exact: true }); }
async function reference(name, label) {
  const entry = await row(name); const dialog = await picker(entry);
  await dialog.getByRole('searchbox').fill(label.split(' Bí mật')[0]);
  await dialog.getByRole('button', { name: label, exact: true }).click(); return entry;
}
async function shared(name, value, secret = false) {
  const entry = await row(name);
  if (secret) await entry.getByRole('checkbox').click();
  else { const dialog = await picker(entry); await dialog.getByRole('button', { name: 'Tạo biến', exact: true }).click(); }
  const dialog = page.getByRole('dialog', { name: secret ? 'Tạo bí mật' : 'Tạo biến', exact: true });
  await expect(dialog.getByLabel('Tên key', { exact: true })).toHaveValue(name);
  if (secret) {
    await expect(dialog.getByRole('combobox')).toBeVisible();
    await dialog.getByRole('combobox').selectOption(process.env.ORCH_SECRET_STORE_KEY ?? 'k8s-4f-vault');
    await dialog.getByRole('button', { name: 'Lưu kho bí mật', exact: true }).click();
    await expect(dialog.getByRole('combobox')).toHaveCount(0);
  }
  const input = dialog.getByLabel(secret ? 'Giá trị bí mật' : 'Giá trị', { exact: true });
  if (secret) await expect(input).toHaveAttribute('type', 'password');
  await input.fill(value);
  await dialog.getByRole('button', { name: 'Tạo và chọn', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await expect(entry.getByRole('button', { name: /Đổi nguồn/ })).toContainText(`Ứng dụng / ${name}`);
  if (secret) { await expect(page.locator('body')).not.toContainText(value); check('Secret entered masked, stored as app/environment reference, never read back'); }
}
async function resource(alias, type, reuse) {
  await click('+ Thêm tài nguyên');
  const aliases = page.getByLabel('Alias tài nguyên', { exact: true });
  const i = await aliases.count() - 1;
  await aliases.nth(i).fill(alias);
  const source = page.getByLabel('Nguồn tài nguyên', { exact: true }).nth(i);
  const available = source.locator('optgroup option').filter({ hasText: `${alias} · ${type} · backend` });
  if (reuse) {
    await expect(available).toHaveCount(1);
    await source.selectOption(await available.first().getAttribute('value'));
    await expect(page.getByLabel('Loại tài nguyên', { exact: true }).nth(i)).toBeDisabled();
    check(`Worker explicitly selected backend ${type}; configuration locked`);
    return;
  }
  await page.getByLabel('Loại tài nguyên', { exact: true }).nth(i).selectOption(type);
  await page.getByLabel(`Cách cung cấp (class) của ${alias}`, { exact: true }).selectOption('in-cluster');
  if (type === 'postgres') {
    await page.getByLabel('Tham số database', { exact: true }).fill('acceptance');
    await page.getByLabel('Tham số username', { exact: true }).fill('app');
  }
}
async function service() {
  await click('+ Thêm cổng');
  await page.getByLabel('Tên cổng Service', { exact: true }).fill('http');
  await page.getByRole('spinbutton', { name: 'Cổng Service', exact: true }).fill('8080');
}
async function begin(name) {
  await click('+ Thêm Workload');
  await page.getByRole('dialog', { name: 'Chọn loại workload', exact: true }).getByRole('button', { name: 'Tự cấu hình / nhập Score', exact: true }).click();
  await page.getByLabel('Tên Workload', { exact: true }).fill(name);
  await page.getByLabel('Ảnh Container', { exact: true }).fill(`acceptance-${name}:${tag}`);
  check(`${name}: form entry, no Score import`);
}
async function save(name) {
  await mark(`${name}: form complete`);
  await click('Lưu draft Workload');
  await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible();
}
async function databaseAndCache(reuse = false) {
  await resource('db', 'postgres', reuse); await resource('cache', 'redis', reuse);
  for (const [name, output] of [['PGHOST','host'],['PGPORT','port'],['PGDATABASE','database'],['PGUSER','username'],['PGPASSWORD','password']]) await reference(name, `db.${output}${output === 'password' ? ' Bí mật' : ''}`);
  for (const [name, output] of [['REDIS_HOST','host'],['REDIS_PORT','port'],['REDIS_USERNAME','username'],['REDIS_PASSWORD','password'],['REDIS_TLS','tls']]) await reference(name, `cache.${output}${output === 'password' ? ' Bí mật' : ''}`);
}
async function deploy(count) {
  await click('Xem trước thay đổi');
  await expect(page.getByLabel('Xem trước triển khai')).toContainText(`${count} Workload bị ảnh hưởng`, { timeout: 120_000 });
  await mark(`Preview ${count} workloads`);
  await click('Triển khai các thay đổi này');
  const result = page.getByLabel('Kết quả triển khai');
  await expect(result).toContainText(/Triển khai (thành công|thất bại)/, { timeout: 25 * 60_000 });
  await mark('Deployment result');
  await expect(result).toContainText('Triển khai thành công');
  check(`UI Preview and Deploy succeeded for ${count} workloads`);
}
function kubectl(args) { return execFileSync('kubectl', ['--context', kube, ...args], { encoding: 'utf8' }); }
async function portForward(namespace) {
  const child = spawn('kubectl', ['--context', kube, '-n', namespace, 'port-forward', 'svc/frontend', ':8080'], { stdio: ['ignore','pipe','pipe'] });
  const port = await new Promise((resolve,reject) => {
    const timer = setTimeout(() => reject(Error('Port-forward timeout')), 30_000);
    child.stdout.on('data', data => { const m = String(data).match(/127\.0\.0\.1:(\d+)/); if (m) { clearTimeout(timer); resolve(m[1]); } });
    child.once('exit', () => { clearTimeout(timer); reject(Error('Port-forward stopped')); });
  });
  return { child, url: `http://127.0.0.1:${port}` };
}
async function verifyApplication(minimumWrites) {
  // DONE commits before the worker records its Redis write-through outcome.
  // Wait for both business completion and every cache marker, rather than
  // accepting the pre-job health check (which has no completed writes yet).
  await expect.poll(async()=>{await appPage.reload();return appPage.locator('#checks tr[data-check]').evaluateAll(rows=>rows.filter(r=>r.children[1].textContent.trim()==='PASS').length);},
    {timeout:120_000,intervals:[1500]}).toBe(5);
  const response=await appPage.request.get(`${forward.url}/api/checks`);
  expect(response.ok()).toBe(true);
  const {checks,cache}=await response.json();
  await writeFile(`${evidence}/checks-after-${minimumWrites}-jobs.json`,JSON.stringify({checks,cache},null,2));
  expect(Object.values(checks).every(value=>value===true)).toBe(true);
  expect(cache.workerWrites.ok).toBeGreaterThanOrEqual(minimumWrites);
  expect(cache.workerWrites.failed).toBe(0);
  expect(cache.workerWrites.unverified).toBe(0);
  expect(cache.workerWrites.unknown??false).toBe(false);
  expect(cache.configured&&cache.reachable).toBe(true);
  expect(cache.unrecoveredFailures??[]).toHaveLength(0);
  expect(cache.failuresOverflow).toBe(false);
  await appPage.screenshot({path:`${evidence}/application-after-${minimumWrites}-jobs.png`,fullPage:true});
  check(`All application checks PASS after ${minimumWrites} jobs; Redis worker writes confirmed, none failed/unverified`);
}
try {
  await click('+ Tạo ứng dụng');
  await page.getByLabel('Tên ứng dụng', { exact: true }).fill(run);
  await page.getByLabel('Subdomain', { exact: true }).fill(run);
  await click('Tạo ứng dụng'); await expect(page.getByRole('heading', { name: run, exact: true })).toBeVisible();
  appId = new URL(page.url()).pathname.match(/\/applications\/([^/]+)$/)?.[1];
  if (!appId) throw Error('Missing new app ID');
  await writeFile(`${evidence}/run.json`, JSON.stringify({run, appId, kube, imageTag:tag, baseURL:config.baseURL},null,2));
  await mark('Created isolated application');
  const secret = randomBytes(24).toString('hex');
  if (process.env.ORCH_SECRET_SCAN_FILE) await appendFile(process.env.ORCH_SECRET_SCAN_FILE, `ACCEPTANCE_SECRET=${secret}\n`, { mode: 0o600 });
  await begin('backend');
  await shared('ACCEPTANCE_CONFIG', 'acceptance-config-ok');
  await shared('ACCEPTANCE_SECRET', secret, true);
  for(const [key,value] of Object.entries({PORT:'8080',ACCEPTANCE_SECRET_SHA256:createHash('sha256').update(secret).digest('hex'),ACCEPTANCE_CACHE_APP:appId,ACCEPTANCE_CACHE_ENV:'staging',ACCEPTANCE_CACHE_RUN:run,ACCEPTANCE_CACHE_TTL_SECONDS:'30'})) await literal(key,value);
  await databaseAndCache(); await service(); await save('backend');
  await begin('worker');
  for(const [key,value] of Object.entries({POLL_INTERVAL_MS:'500',ACCEPTANCE_CACHE_APP:appId,ACCEPTANCE_CACHE_ENV:'staging',ACCEPTANCE_CACHE_RUN:run,ACCEPTANCE_CACHE_TTL_SECONDS:'30'})) await literal(key,value);
  await databaseAndCache(true); await save('worker');
  const desiredReply = await page.request.get(`${config.baseURL}/api/v1/applications/${appId}/environments/staging/workloads`);
  expect(desiredReply.status()).toBe(200);
  const desired = (await desiredReply.json()).workloads;
  const apiResources = desired.find(item => item.id === 'backend').score.resources;
  const workerResources = desired.find(item => item.id === 'worker').score.resources;
  for (const alias of ['db', 'cache']) {
    expect(apiResources[alias].id).toMatch(/^resource-[a-f0-9]{32}$/);
    expect(workerResources[alias]).toEqual(apiResources[alias]);
  }
  await writeFile(`${evidence}/shared-resource-identities.json`, JSON.stringify({
    application: appId, environment: 'staging', consumers: ['backend', 'worker'],
    postgres: apiResources.db.id, redis: apiResources.cache.id,
  }, null, 2));
  check('Backend and worker desired resources have identical generated IDs, class and typed params');
  await begin('frontend'); await literal('PORT','8080'); await reference('BACKEND_URL','backend / http'); await service(); await save('frontend');
  await deploy(3);
  const namespace = `app-${appId}-staging`;
  const objects = JSON.parse(kubectl(['-n',namespace,'get','deployments,statefulsets,persistentvolumeclaims','-o','json']));
  expect(objects.items.filter(x => x.kind === 'Deployment').map(x => x.metadata.name).sort()).toEqual(['backend','frontend','worker']);
  expect(objects.items.filter(x => x.kind === 'StatefulSet')).toHaveLength(2);
  expect(objects.items.filter(x => x.kind === 'PersistentVolumeClaim')).toHaveLength(2);
  const identity = objects.items.map(x=>({kind:x.kind,name:x.metadata.name,uid:x.metadata.uid}));
  await writeFile(`${evidence}/objects.json`,JSON.stringify(identity,null,2));
  check('Real kind objects exist: app deployments, PostgreSQL, Redis and PVC');
  forward = await portForward(namespace);
  appPage = await context.newPage(); appVideo = appPage.video();
  appPage.on('pageerror', error => errors.push(error.message));
  await appPage.goto(forward.url);
  await expect(appPage.getByRole('heading',{name:'Acceptance application'})).toBeVisible();
  await expect.poll(async()=>{await appPage.reload(); return appPage.locator('#checks tr[data-check]').evaluateAll(rows=>rows.filter(r=>r.children[1].textContent.trim()==='PASS').length);},{timeout:300_000,intervals:[2000]}).toBe(5);
  await appPage.locator('input[name="payload"]').fill(`hello ${run}`);await appPage.getByRole('button',{name:'Submit job'}).click();
  await expect.poll(async()=>{await appPage.reload();return appPage.locator('#jobs tr.job').filter({hasText:`hello ${run}`}).first().innerText();},{timeout:120_000,intervals:[1500]}).toContain('DONE');
  await verifyApplication(1);
  await appPage.screenshot({path:`${evidence}/acceptance-app.png`,fullPage:true});check('Acceptance UI: five PASS checks and job DONE through worker/PostgreSQL/Redis');
  // Reopen the deployed workload through UI and prove direct values/references survived.
  const backend = page.locator('.workload-table .table-row').filter({hasText:'backend'});
  await backend.getByRole('button',{name:'Sửa',exact:true}).click();
  await expect(page.getByLabel('Tên Workload',{exact:true})).toHaveValue('backend');
  const portRow = rows().filter({has:page.locator('input[value="PORT"]')});await expect(portRow.getByRole('textbox').nth(1)).toHaveValue('8080');
  await literal('ENV_UI_UPDATED','true');await save('backend update');await deploy(1);
  const after=JSON.parse(kubectl(['-n',namespace,'get','statefulsets,persistentvolumeclaims','-o','json']));
  expect(after.items).toHaveLength(identity.filter(object=>['StatefulSet','PersistentVolumeClaim'].includes(object.kind)).length);
  for(const object of after.items){const before=identity.find(x=>x.kind===object.kind&&x.name===object.metadata.name);expect(object.metadata.uid).toBe(before?.uid);}
  check('Valid UI update deployed; PostgreSQL/Redis/PVC UIDs retained');
  const liveBackend = JSON.parse(kubectl(['-n',namespace,'get','deployment','backend','-o','json']));
  expect(liveBackend.spec.template.spec.containers[0].env.find(entry => entry.name === 'ENV_UI_UPDATED')?.value).toBe('true');
  await expect.poll(async()=>{await appPage.reload();return appPage.locator('#jobs tr.job').filter({hasText:`hello ${run}`}).first().innerText();},{timeout:120_000,intervals:[1500]}).toContain('DONE');
  await appPage.locator('input[name="payload"]').fill(`updated ${run}`);await appPage.getByRole('button',{name:'Submit job'}).click();
  await expect.poll(async()=>{await appPage.reload();return appPage.locator('#jobs tr.job').filter({hasText:`updated ${run}`}).first().innerText();},{timeout:120_000,intervals:[1500]}).toContain('DONE');
  await verifyApplication(2);
  check('Updated value is live; existing job persisted and a new UI-submitted job completed');
  if (process.env.ORCH_DELETE_APPLICATION === '1') {
    await appPage.close();
    forward.child.kill();forward=undefined;
    await deleteApplicationViaUI({page,baseURL:config.baseURL,appId,name:run,namespace,kube,evidence,check});
  }
  expect(errors).toEqual([]);outcome='PASS';
} catch(error) {
  if(forward&&appPage&&!appPage.isClosed()) {
    try {
      const response=await appPage.request.get(`${forward.url}/api/checks`);
      if(response.ok()) {const {checks,cache}=await response.json();await writeFile(`${evidence}/checks-on-failure.json`,JSON.stringify({checks,cache},null,2));}
    } catch { /* Preserve the original failure if the runtime cannot be read. */ }
  }
  console.error(`FAIL ${error.message}`);await page.screenshot({path:`${evidence}/failure.png`,fullPage:true}).catch(()=>{});process.exitCode=1;
} finally {
  forward?.child.kill();
  await writeFile(`${evidence}/result.json`,JSON.stringify({outcome,run,appId,assertions,phases,browserErrors:errors},null,2));
  await signOutContext(context,config);await context.close();await video?.saveAs(`${evidence}/console.webm`);await appVideo?.saveAs(`${evidence}/application.webm`);await browser.close();
  console.log(`${outcome} evidence: ${evidence}`);
}
