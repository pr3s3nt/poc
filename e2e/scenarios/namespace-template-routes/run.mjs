import { createRequire } from 'node:module';
const { expect } = createRequire(new URL('../../../scripts/bootstrap/package.json', import.meta.url))('@playwright/test');
import assert from 'node:assert/strict';
import { readFileSync, writeFileSync, mkdirSync, existsSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { openConsole, readConsoleConfig, closeConsole } from '../../lib/console.mjs';
import { ensureScoreNamespaceAccess } from '../../setup/score-authoring-namespace/access.mjs';

const config = readConsoleConfig();
const evidence = resolve(process.env.ORCH_EVIDENCE_DIR ?? '');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.') + '/'), 'Evidence must be outside Git');
mkdirSync(evidence, { recursive: true, mode: 0o700 });
const receiptFile = join(evidence, 'receipt.json');
const receipt = existsSync(receiptFile) ? JSON.parse(readFileSync(receiptFile, 'utf8')) : { name: 'httproute84-' + Date.now().toString(36), saved: [] };
const save = () => writeFileSync(receiptFile, JSON.stringify(receipt, null, 2), { mode: 0o600 });
const phase = process.env.ORCH_ROUTE_PHASE ?? 'configure';
const session = await openConsole(config);
const { page } = session;
const get = async path => { const r = await page.request.get(config.baseURL + '/api/v1' + path); assert(r.ok(), `Read ${path}: HTTP ${r.status()}`); return r.json(); };
const required = key => { assert(process.env[key], 'Missing ' + key); return process.env[key]; };
const definitionsRoute = config.baseURL + '/ui/platform/resource-definitions';
try {
 if (phase === 'configure') {
  const variables = { cluster: required('ORCH_GATEWAY_CLUSTER_REFERENCE'), rootDomain: required('ORCH_GATEWAY_ROOT_DOMAIN'), scheme: 'http', gatewayName: required('ORCH_GATEWAY_NAME'), gatewayNamespace: required('ORCH_GATEWAY_NAMESPACE'), gatewaySectionName: required('ORCH_GATEWAY_LISTENER') };
  for (const [type,key,name,values] of [
   ['k8s-gateway','httproute84-gateway','HTTPRoute existing Gateway',variables],
   ['endpoint','httproute84-endpoint','HTTPRoute Gateway endpoint',{gateway:"${resources['k8s-gateway.default#environments.@app.@env'].outputs.gateway}"}],
  ]) {
   const stored = (await get('/resource-definitions')).resourceDefinitions.find(d=>d.key===key);
   if (stored) { assert.equal(stored.resourceType,type); assert.deepEqual(stored.driverInputs.values.variables,values); continue; }
   await page.goto(definitionsRoute);
   await page.getByRole('button',{name:'Tạo cấu hình',exact:true}).click();
   await page.getByRole('combobox').filter({has:page.locator('option[value="k8s-gateway"]')}).selectOption(type);
   await page.getByLabel('ID cấu hình (Definition ID)',{exact:true}).fill(key);
   await page.getByLabel('Tên hiển thị',{exact:true}).fill(name);
   if (type==='k8s-gateway') {
    for (const [label,value] of [['Tham chiếu cluster',variables.cluster],['Domain gốc',variables.rootDomain],['Tên Gateway',variables.gatewayName],['Namespace của Gateway',variables.gatewayNamespace],['Listener (tùy chọn)',variables.gatewaySectionName]]) await page.getByLabel(label,{exact:true}).fill(value);
    await page.getByLabel(/Giao thức/).selectOption('http');
   } else await page.getByLabel('Tham chiếu Gateway',{exact:true}).fill(values.gateway);
   const posted = page.waitForResponse(r=>r.request().method()==='POST' && r.url().endsWith('/resource-definitions'));
   await page.getByRole('button',{name:'Đăng ký cấu hình tài nguyên',exact:true}).click();
   const response=await posted; assert(response.ok(),'Definition save HTTP '+response.status());
   const read=(await get('/resource-definitions')).resourceDefinitions.find(d=>d.key===key);
   assert.deepEqual(read.driverInputs.values.variables,values);
   await page.screenshot({path:join(evidence,type+'-form.png'),fullPage:true});
  }
  receipt.configured=true; save(); console.log('PASS: separate existing Gateway and endpoint registered through Console');
 } else if (phase==='author' || phase==='update') {
  assert(receipt.configured);
  if (!receipt.appId) {
   await page.goto(config.baseURL+'/ui/applications/new');
   await page.getByLabel('Tên ứng dụng',{exact:true}).fill(receipt.name);
   const posted=page.waitForResponse(r=>r.request().method()==='POST' && new URL(r.url()).pathname==='/api/v1/applications');
   await page.getByRole('button',{name:'Tạo ứng dụng',exact:true}).click();
   const response=await posted; assert(response.ok(),'Application create failed');
   const body=await response.json(); receipt.appId=body.application.key??body.application.id; save();
  }
  const app=(await get('/applications/'+receipt.appId)).application; assert.equal(app.name,receipt.name);
  receipt.namespace=app.environments.find(e=>e.key==='staging').namespaceIdentity; save();
  const image=required('ORCH_ROUTE_IMAGE');
  const dbImage='postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777';
  for (const [name,format,hostname] of [['score-web','score',required('ORCH_SCORE_ENDPOINT_NAME')],['native-web','native',required('ORCH_NATIVE_ENDPOINT_NAME')]]) {
   if (phase==='author' && receipt.saved.includes(name)) continue;
   const dbcheck={image:dbImage,command:['/bin/sh','-c'],args:['until pg_isready -h "$PGHOST" -p 5432; do sleep 2; done; echo httproute84-database-ready'],variables:{PGHOST:'${resources.db.host}'}};
   const containers={main:{image,variables:{ACCEPTANCE_REVISION:phase==='update'?'second':'first'}},dbcheck};
   const resources={db:{type:'postgres',id:'httproute84-db',params:{database:'route_acceptance',username:'route_app'}},public:{type:'endpoint',params:{name:hostname,port:'http',path:'/'}}};
   let document,extension;
   if(format==='score') {
    document={apiVersion:'score.dev/v1b1',metadata:{name},containers,service:{ports:{http:{port:8080,targetPort:8080}}},resources};
    extension={apiVersion:'humanitec.org/v1b1',profile:'frontend',spec:{orchestrator:{metadata:{class:required('ORCH_WORKLOAD_CLASS')},containers:{dbcheck:{role:'init',initOrder:1}}},deployment:{replicas:1}}};
   } else {
    document={apiVersion:'orchestrator.dev/workload/v1',metadata:{name,class:required('ORCH_WORKLOAD_CLASS')},containers:{...containers,dbcheck:{...dbcheck,role:'init',initOrder:1}},service:{ports:{http:{port:8080,targetPort:8080}}},resources};
   }
   const base=`/applications/${receipt.appId}/environments/staging`;
   await page.goto(config.baseURL+'/ui'+base+'/workloads/'+(phase==='update'?name:'new'));
   await page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
   if(extension) await page.getByLabel('File extension (tùy chọn)',{exact:true}).setInputFiles({name:'extension.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(extension))});
   const parsed=page.waitForResponse(r=>r.request().method()==='POST' && r.url().endsWith('/workloads/parse'));
   await page.getByLabel('File Workload',{exact:true}).setInputFiles({name:format+'.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify(document))});
   const response=await parsed; const body=await response.json(); assert(response.ok(),'Parse HTTP '+response.status()+' '+(body.code??''));
   assert.equal(body.workload.metadata.name,name); assert.equal(body.workload.containers.dbcheck.role,'init');
   assert.equal(body.workload.resources.public.params.name,hostname);
   const saved=page.waitForResponse(r=>r.request().method()==='PUT' && r.url().endsWith('/workloads/'+name));
   await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();
   assert((await saved).ok(),'Draft save failed');
   receipt.saved=[...new Set([...receipt.saved,name])];save();
   await page.screenshot({path:join(evidence,phase+'-'+name+'.png'),fullPage:true});
  }
  receipt.allSaved=true;receipt.updated=phase==='update';save();console.log('PASS: native and Score workloads saved before Environment Deploy');
 } else if (phase==='remove-workloads') {
  assert(receipt.restart && receipt.nativeVerified,'Verify update/restart first');
  const app=(await get('/applications/'+receipt.appId)).application;assert.equal(app.name,receipt.name);
  await page.goto(config.baseURL+'/ui/applications/'+receipt.appId);
  for(const name of ['native-web','score-web']){
   const removed=page.waitForResponse(r=>r.request().method()==='DELETE'&&r.url().includes('/workloads/'+name));
   page.once('dialog',async dialog=>{assert.equal(dialog.type(),'confirm');assert(dialog.message().includes(name));await dialog.accept();});
   await page.locator('.table-row').filter({has:page.locator('.workload-name',{hasText:name})}).getByRole('button',{name:'Xóa',exact:true}).click();
   assert((await removed).ok(),'Workload removal draft failed');
  }
  receipt.removalDrafts=true;save();console.log('PASS: both workload removals saved through Console');
 } else if (phase==='deploy' || phase==='deploy-update' || phase==='deploy-removal') {
  assert(receipt.allSaved); assert(phase==='deploy-update'?receipt.updated:phase==='deploy-removal'?receipt.removalDrafts:!receipt.deployStarted,'Inspect prior operation before resubmission');
  const app=(await get('/applications/'+receipt.appId)).application;assert.equal(app.name,receipt.name);
  await page.goto(config.baseURL+'/ui/applications/'+receipt.appId);
  const preview=page.waitForResponse(r=>r.request().method()==='POST' && r.url().endsWith('/environments/staging/preview'),{timeout:180000});
  await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();
  const response=await preview; const view=await response.json();assert(response.ok(),'Preview HTTP '+response.status()+' '+(view.code??''));
  assert.deepEqual(view.changes.map(c=>c.workloadId).sort(),['native-web','score-web']);
  receipt.previewWorkloads=view.changes.map(c=>c.workloadId);save();
  await page.screenshot({path:join(evidence,phase+'-preview.png'),fullPage:true});
  const deployed=page.waitForResponse(r=>r.request().method()==='POST' && r.url().endsWith('/environments/staging/deploy'),{timeout:600000});
  receipt.deployStarted=true;save();
  await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
  await ensureScoreNamespaceAccess(page,config,receipt,save);
  const reply=await deployed;receipt.report=await reply.json();save();
  assert(reply.ok() && receipt.report.status==='SUCCEEDED','Deployment failed: '+JSON.stringify(receipt.report));
  if(phase==='deploy-update')receipt.deployedUpdate=true;
  if(phase==='deploy-removal')receipt.workloadsRemoved=true;
  save();
  await page.screenshot({path:join(evidence,phase+'-done.png'),fullPage:true});
  console.log('PASS: one Environment Deploy succeeded for both workloads');
 } else if (phase==='delete-application') {
  assert(receipt.workloadsRemoved&&receipt.setupRemoved,'Remove workloads and receipt-owned setup first');
  const app=(await get('/applications/'+receipt.appId)).application;assert.equal(app.name,receipt.name);
  await page.goto(config.baseURL+'/ui/applications/'+receipt.appId+'/settings');
  const preview=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/deletion-preview'));
  await page.locator('.danger-zone').getByRole('button').click();
  const reply=await preview;assert(reply.ok());const {preview:scope}=await reply.json();
  assert(!scope.blocked&&scope.token,'Deletion blocked; inspect product scope');assert.equal(scope.confirmationName,receipt.name);
  const dialog=page.getByRole('dialog',{name:'Xóa ứng dụng',exact:true});
  await dialog.locator('input').fill(receipt.name);
  const admission=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/deletions'));
  await dialog.getByRole('button',{name:'Xóa vĩnh viễn ứng dụng',exact:true}).click();
  const admitted=await admission;assert(admitted.ok());receipt.deletion=(await admitted.json()).deletion;save();
  const deadline=Date.now()+600000;
  for(;;){
   const {deletion}=await get(`/applications/${receipt.appId}/deletions/${receipt.deletion.operationId}`);
   receipt.deletionProgress=deletion;save();
   if(!deletion.running){assert.equal(deletion.status,'SUCCEEDED','Application cleanup failed: '+JSON.stringify(deletion));break;}
   assert(Date.now()<deadline,'Deletion timed out; inspect existing operation before retry');await page.waitForTimeout(3000);
  }
  receipt.deletionSucceeded=true;save();await page.screenshot({path:join(evidence,'cleanup-done.png'),fullPage:true});
  console.log('PASS: receipt-owned Application deleted through product UI');
 } else if (phase==='browser') {
  assert.equal(receipt.report?.status,'SUCCEEDED');
  for (const [name,url] of [['native',required('ORCH_NATIVE_APP_URL')],['score',required('ORCH_SCORE_APP_URL')]]) {
   const response=await page.goto(url);assert(response?.ok(),name+' application not reachable');
   await expect(page.locator('body')).toContainText('Welcome to nginx');
   await page.screenshot({path:join(evidence,name+'-live.png'),fullPage:true});
  }
  receipt.browserPassed=true;save();console.log('PASS: both real applications reached through the existing Gateway over Tailscale');
 } else throw new Error('Unknown ORCH_ROUTE_PHASE');
} catch(error) {
 receipt.lastError=error.message;save();await page.screenshot({path:join(evidence,phase+'-failure.png'),fullPage:true}).catch(()=>{});throw error;
} finally { await closeConsole(session,config); }
