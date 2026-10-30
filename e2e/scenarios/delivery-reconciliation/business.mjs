import { expect } from '@playwright/test';
import { createHash, randomBytes } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync, spawn } from 'node:child_process';
import { grantWorkloadAccess } from './platform.mjs';

export async function runBusiness({ page, context, baseURL, run, state, saveState, step }) {
  const click = name => page.getByRole('button', { name, exact: true }).click();
  const appName = state.appName ?? process.env.ORCH_RECON_APP_NAME ?? 'reconciliation-ui-20261029';
  const tag = 'reconciliation-20261029';
  const table = () => page.getByRole('region', { name: 'Biến môi trường của container main', exact: true });
  const rows = () => table().locator('tbody tr');
  const kubeconfig = join(run, 'kubeconfig');
  const kubectl = (...args) => execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] });
  kubectl.withInput = (input, ...args) => execFileSync('kubectl', ['--kubeconfig', kubeconfig, ...args], { input, encoding: 'utf8', stdio: ['pipe', 'pipe', 'pipe'] });
  const getApp = async () => { const reply = await page.request.get(`${baseURL}/api/v1/applications/${state.appID}`); expect(reply.ok()).toBe(true); return (await reply.json()).application; };
  async function literal(name, value) { await table().getByRole('button', { name: '+ Thêm biến', exact: true }).click(); const row = rows().last(); await row.getByRole('textbox').first().fill(name); await row.getByRole('textbox').nth(1).fill(value); }
  async function reference(name, label) {
    await table().getByRole('button', { name: '+ Thêm biến', exact: true }).click(); const row = rows().last(); await row.getByRole('textbox').first().fill(name);
    await row.getByRole('button', { name: /^(Chọn|Đổi) nguồn, dòng/ }).click();
    const dialog = page.getByRole('dialog', { name: 'Chọn giá trị có sẵn', exact: true });
    await dialog.getByRole('searchbox').fill(label.split(' Bí mật')[0]); await dialog.getByRole('button', { name: label, exact: true }).click();
  }
  async function shared(name, value, secret = false) {
    await table().getByRole('button', { name: '+ Thêm biến', exact: true }).click(); const row = rows().last(); await row.getByRole('textbox').first().fill(name);
    if (secret) await row.getByRole('checkbox').click(); else { await row.getByRole('button', { name: /^(Chọn|Đổi) nguồn, dòng/ }).click(); await page.getByRole('dialog', { name: 'Chọn giá trị có sẵn' }).getByRole('button', { name: 'Tạo biến', exact: true }).click(); }
    const dialog = page.getByRole('dialog', { name: secret ? 'Tạo bí mật' : 'Tạo biến', exact: true });
    await dialog.getByLabel(secret ? 'Giá trị bí mật' : 'Giá trị', { exact: true }).fill(value); await dialog.getByRole('button', { name: 'Tạo và chọn', exact: true }).click(); await expect(dialog).not.toBeVisible();
    if (secret) await expect(page.locator('body')).not.toContainText(value);
  }
  async function resource(alias, type, sharedId) {
    await click('+ Thêm tài nguyên'); const aliases = page.getByLabel('Alias tài nguyên', { exact: true }); const n = await aliases.count() - 1;
    await aliases.nth(n).fill(alias); await page.getByLabel('Loại tài nguyên', { exact: true }).nth(n).selectOption(type);
    await page.getByLabel(`Cách cung cấp (class) của ${alias}`, { exact: true }).selectOption('in-cluster');
    await page.getByLabel(`ID dùng chung của ${alias}`, { exact: true }).fill(sharedId);
    if (type === 'postgres') { await page.getByLabel('Tham số database', { exact: true }).fill('acceptance'); await page.getByLabel('Tham số username', { exact: true }).fill('app'); }
  }
  async function dependencies() {
    await resource('db', 'postgres', `${appName}-db`); await resource('cache', 'redis', `${appName}-cache`);
    for (const [name, output] of [['PGHOST','host'],['PGPORT','port'],['PGDATABASE','database'],['PGUSER','username'],['PGPASSWORD','password']]) await reference(name, `db.${output}${output === 'password' ? ' Bí mật' : ''}`);
    for (const [name, output] of [['REDIS_HOST','host'],['REDIS_PORT','port'],['REDIS_USERNAME','username'],['REDIS_PASSWORD','password'],['REDIS_TLS','tls']]) await reference(name, `cache.${output}${output === 'password' ? ' Bí mật' : ''}`);
  }
  async function service() { await click('+ Thêm cổng'); await page.getByLabel('Tên cổng Service', { exact: true }).fill('http'); await page.getByRole('spinbutton', { name: 'Cổng Service', exact: true }).fill('8080'); }
  async function home() { await page.goto(`${baseURL}/ui/applications/${state.appID}?environment=staging`); await expect(page.getByRole('heading', { name: appName, exact: true })).toBeVisible(); }
  async function saveDraft() { await click('Lưu draft Workload'); await expect(page.getByRole('heading', { name: 'Workload', exact: true })).toBeVisible(); }
  async function deploy({ policyOnly = false } = {}) {
    const previewReply = policyOnly ? page.waitForResponse(r => r.request().method()==='POST' && new URL(r.url()).pathname.endsWith('/preview')) : undefined;
    await click('Xem trước thay đổi'); await expect(page.getByLabel('Xem trước triển khai')).toBeVisible({ timeout: 120_000 });
    if(previewReply){const reply=await previewReply;expect(reply.ok()).toBe(true);const body=await reply.json();expect(body.deliveryPolicyOnly).toBe(true);expect(body.changes).toHaveLength(0);await expect(page.getByText('Không đổi cluster hoặc dữ liệu',{exact:true})).toBeVisible();}
    await click('Triển khai các thay đổi này'); const result = page.getByLabel('Kết quả triển khai');
    await Promise.race([
      expect(result).toContainText(/Triển khai (thành công|thất bại)/, { timeout: 25 * 60_000 }),
      page.locator('.review-section-status').filter({hasText:/^INTERRUPTED/}).waitFor({state:'visible',timeout:25*60_000}).then(()=>{throw Error('Deployment interrupted; inspect and recover the held operation before resuming');}),
    ]);
    await expect(result).toContainText('Triển khai thành công');
  }
  await step('Create application through form', async () => {
    if (state.appID) { await home(); return; }
    await page.goto(`${baseURL}/ui/applications`); await page.getByRole('button', { name: '+ Tạo ứng dụng', exact: true }).first().click(); await page.getByLabel('Tên ứng dụng', { exact: true }).fill(appName); await page.getByLabel('Subdomain', { exact: true }).fill(appName); await click('Tạo ứng dụng');
    await expect(page.getByRole('heading', { name: appName, exact: true })).toBeVisible(); state.appID = new URL(page.url()).pathname.split('/').at(-1); state.appName = appName; saveState();
  });
  const app = await getApp(); const namespace = app.environments.find(e => e.key === 'staging').namespaceIdentity; state.namespace = namespace; saveState();
  await step('Select workload Vault and enable CD only in staging through Settings', async () => {
    if (state.firstDeployPassed) return;
    await page.goto(`${baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
    const store = page.getByLabel('Kho bí mật cho Staging', { exact: true }); await expect(store).toBeVisible();
    const label = (await store.locator('option').allTextContents()).find(s => s.startsWith('Reconciliation workload Vault'));
    if (!label) throw Error('Task workload Vault is not listed');
    await store.selectOption({ label }); const saveStore = page.getByRole('button', { name: 'Lưu kho bí mật', exact: true }); if (await saveStore.isEnabled()) { await saveStore.click(); await expect(saveStore).toBeDisabled({ timeout: 120_000 }); }
    await page.reload(); const cd = page.getByRole('checkbox', { name: 'Dùng CD provider', exact: true }); await expect(cd).toBeEnabled();
    if (!(await cd.isChecked())) await cd.check();
    await page.getByLabel('CD provider', { exact: true }).selectOption(`${state.managedBindingID}:${state.managedBindingVersion}`);
    const save = page.getByRole('button', { name: 'Lưu thay đổi', exact: true }); if (await save.isEnabled()) { await save.click(); await expect(page.getByText('Đã lưu. Xem trước để áp dụng.', { exact: false })).toBeVisible(); }
    await page.getByRole('tab', { name: 'Production', exact: true }).click(); await expect(page.getByRole('checkbox', { name: 'Dùng CD provider', exact: true })).not.toBeChecked(); await home();
  });
  await step('Create app workloads and PostgreSQL/Redis bindings through ordinary forms', async () => {
    const drafts = await page.request.get(`${baseURL}/api/v1/applications/${state.appID}/environments/staging/workloads`);
    const existing = (await drafts.json()).workloads ?? [];
    const secretFile = join(run, 'acceptance-secret'); let secret; try { secret = readFileSync(secretFile, 'utf8'); } catch (err) { if (err.code !== 'ENOENT') throw err; secret = randomBytes(24).toString('hex'); writeFileSync(secretFile, secret, { mode: 0o600 }); }
    for (const name of ['backend', 'worker', 'frontend']) {
      if (existing.some(w => w.workloadId === name || w.id === name)) continue;
      await click('+ Thêm Workload'); await page.getByLabel('Tên Workload', { exact: true }).fill(name); await page.getByLabel('Ảnh Container', { exact: true }).fill(`acceptance-${name}:${tag}`);
      if (name === 'backend') { await shared('ACCEPTANCE_CONFIG', 'acceptance-config-ok'); await shared('ACCEPTANCE_SECRET', secret, true); await literal('ACCEPTANCE_SECRET_SHA256', createHash('sha256').update(secret).digest('hex')); }
      if (name !== 'frontend') {
        for (const [k,v] of Object.entries({ ACCEPTANCE_CACHE_APP: state.appID, ACCEPTANCE_CACHE_ENV: 'staging', ACCEPTANCE_CACHE_RUN: appName, ACCEPTANCE_CACHE_TTL_SECONDS: '30', ...(name === 'worker' ? { POLL_INTERVAL_MS: '500' } : { PORT: '8080' }) })) await literal(k,v);
        await dependencies();
      } else { await literal('PORT', '8080'); await reference('BACKEND_URL', 'backend / http'); }
      if (name !== 'worker') await service();
      if(name==='frontend'&&process.env.ORCH_RECON_FOCUS==='semantics'){
       await click('+ Thêm đường dẫn công khai');await page.getByLabel('Đường dẫn công khai',{exact:true}).fill('/');
      }
      await saveDraft();
    }
  });
  await step('Preview and deploy Fleet application with Terraform dependencies', async () => {
    if (state.firstDeployPassed) return;
    await home();
    const permissions = grantWorkloadAccess({namespace,kubectl,state,saveState});
    // Attach a handler immediately; the setup task and UI deployment proceed together.
    let setupError; permissions.catch(error => { setupError = error; });
    await deploy(); await permissions; if (setupError) throw setupError;
    if(process.env.ORCH_RECON_FOCUS==='semantics'){
     const migration=JSON.parse(kubectl('get','job','backend-migration','-n',namespace,'-o','json'));
     expect(migration.status.succeeded).toBe(1);state.migrationJobUID=migration.metadata.uid;state.migrationJobTemplate=migration.spec.template;
    }
    state.firstDeployPassed = true; saveState();
  });
  async function checkBusiness({afterTransfer=false}={}) {
    const child = spawn('kubectl',['--kubeconfig',kubeconfig,'-n',namespace,'port-forward','svc/frontend',':8080'],{stdio:['ignore','pipe','pipe']});
    try {
      const port = await new Promise((resolve,reject) => { const timer=setTimeout(()=>reject(Error('Frontend port-forward timed out')),30_000); child.stdout.on('data',chunk=>{const m=String(chunk).match(/127\.0\.0\.1:(\d+)/);if(m){clearTimeout(timer);resolve(m[1]);}});child.on('exit',()=>{clearTimeout(timer);reject(Error('Frontend port-forward exited'));}); });
      const appPage=await context.newPage(); await appPage.goto(`http://127.0.0.1:${port}/`);
      await expect.poll(async()=>{ await appPage.reload(); return appPage.locator('#checks tr[data-check]').evaluateAll(trs=>trs.filter(tr=>tr.children[1].textContent.trim()==='PASS').length); },{timeout:300_000,intervals:[3000]}).toBe(5);
      const originalPayload=`hello ${appName}`;
      if(afterTransfer) await expect(appPage.locator('#jobs tr.job').filter({hasText:originalPayload}).first()).toContainText('DONE');
      const payload=afterTransfer?`after provider transfer ${appName}`:originalPayload;
      if (!(await appPage.locator('#jobs tr.job').filter({hasText:payload}).count())) { await appPage.locator('input[name="payload"]').fill(payload);await appPage.getByRole('button',{name:'Submit job',exact:true}).click(); }
      await expect.poll(async()=>{await appPage.reload();return appPage.locator('#jobs tr.job').filter({hasText:payload}).first().innerText();},{timeout:240_000,intervals:[1500]}).toContain('DONE');
      await appPage.screenshot({path:join(run,'ui-evidence',afterTransfer?'application-business-after-transfer-pass.png':'application-business-pass.png'),fullPage:true});await appPage.close();
    } finally { child.kill(); }
  }
  await step('Application checks and queued job through application UI',()=>checkBusiness());
  const snapshot = () => JSON.parse(kubectl('get',process.env.ORCH_RECON_FOCUS==='semantics'?'deployments,statefulsets,persistentvolumeclaims,services,jobs,cronjobs,hpa,ingresses':'deployments,statefulsets,persistentvolumeclaims,services','-n',namespace,'-o','json')).items.filter(o=>!o.metadata.ownerReferences?.some(ref=>ref.kind==='CronJob')).map(o=>({key:`${o.kind}/${o.metadata.name}`,uid:o.metadata.uid,template:o.spec.template})).sort((a,b)=>a.key.localeCompare(b.key));
  await step('Valid workload configuration update through Preview and Deploy', async () => {
    if (state.updatePassed) return;
    const before=snapshot(); await home();
    await page.locator('.table-row').filter({has:page.locator('.workload-name',{hasText:'backend'})}).getByRole('button',{name:'Sửa',exact:true}).click();
    await literal('RECONCILIATION_RELEASE','v2'); await saveDraft(); await deploy();
    const after=snapshot(); expect(after.map(({key,uid})=>({key,uid}))).toEqual(before.map(({key,uid})=>({key,uid})));
    const backend=JSON.parse(kubectl('get','deployment','backend','-n',namespace,'-o','json'));
    expect(backend.spec.template.spec.containers[0].env).toContainEqual({name:'RECONCILIATION_RELEASE',value:'v2'});
    state.updatePassed=true;saveState();
  });
  async function transfer(enabled) {
    await page.goto(`${baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
    const checkbox=page.getByRole('checkbox',{name:'Dùng CD provider',exact:true});await expect(checkbox).toBeEnabled();
    if ((await checkbox.isChecked()) === enabled) {
      const applied=page.getByText(`Đang dùng: ${enabled ? 'Fleet managed reconciliation' : 'Trực tiếp'}`, {exact:true});
      if(await applied.isVisible())return;
      // A previous terminal refusal left the desired choice pending. Review it
      // through the normal UI; never report the choice as already applied.
      await home();await deploy();return;
    }
    await checkbox.setChecked(enabled);
    if(enabled) await page.getByLabel('CD provider',{exact:true}).selectOption(`${state.managedBindingID}:${state.managedBindingVersion}`);
    await click('Lưu thay đổi');await expect(page.getByText('Đã lưu. Xem trước để áp dụng.',{exact:false})).toBeVisible();
    await home();await deploy();
  }
  const transfers = process.env.ORCH_RECON_FOCUS === 'infrastructure' ? [] : [['Fleet to Direct',false,'directTransferPassed'],['Direct to Fleet',true,'fleetTransferPassed']];
  for(const [name,enabled,key] of transfers) {
    await step(`${name} through Settings with stable UIDs and Pod templates`,async()=>{
      if(state[key]) return;
      const before=snapshot(); state.transferBaselines ??= {};state.transferBaselines[key] ??= before;saveState();await transfer(enabled);expect(snapshot()).toEqual(state.transferBaselines[key]);
      state[key]=true;saveState();
    });
  }
  if (process.env.ORCH_RECON_OBSERVATIONS === '1') {
    await step('Stored reconciliation status through Console',async()=>{
      await home();const panel=page.getByRole('region',{name:'Đối soát workload',exact:true});
      await expect(panel.getByRole('cell',{name:'Đã đồng bộ',exact:true})).toHaveCount(5,{timeout:120_000});
      await expect(panel.getByRole('cell',{name:'Không lệch',exact:true})).toHaveCount(5);
      await page.screenshot({path:join(run,'ui-evidence','stored-reconciliation.png'),fullPage:true});
    });
    await step('Drift reporting and opt-in self-heal through Settings',async()=>{
      if(state.selfHealPassed)return;
      const live=()=>JSON.parse(kubectl('get','deployment','backend','-n',namespace,'-o','json'));
      const before=live();expect(before.spec.replicas).toBe(1);
      if(JSON.parse(kubectl('get','hpa','-n',namespace,'-o','json')).items.length)throw Error('Controlled scale drift requires no HPA');
      const scale=()=>kubectl('patch','deployment','backend','-n',namespace,'--type=json','--patch',JSON.stringify([{op:'test',path:'/metadata/uid',value:before.metadata.uid},{op:'replace',path:'/spec/replicas',value:2}]));
      scale();await home();await expect(page.getByRole('region',{name:'Đối soát workload',exact:true}).getByRole('cell',{name:'Có sai lệch',exact:true}).first()).toBeVisible({timeout:150_000});
      expect(live().spec.replicas).toBe(2);
      await page.goto(`${baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
      await page.getByRole('checkbox',{name:'Tự sửa sai lệch',exact:true}).check();await click('Lưu thay đổi');await expect(page.getByText('Đã lưu. Xem trước để áp dụng.',{exact:false})).toBeVisible();
      const template=live().spec.template;
      await home();await deploy({policyOnly:true});expect(live().spec.template).toEqual(template);scale();
      await expect.poll(()=>live().spec.replicas,{timeout:180_000,intervals:[2000]}).toBe(1);
      expect(live().metadata.uid).toBe(before.metadata.uid);
      await home();await expect(page.getByRole('region',{name:'Đối soát workload',exact:true}).getByRole('cell',{name:'Không lệch',exact:true})).toHaveCount(5,{timeout:150_000});
      state.selfHealPassed=true;saveState();
    });
  }
  return { namespace, deploy, home, click, literal, rows, saveDraft, kubectl, context, spawn, getApp, checkBusiness };
}
