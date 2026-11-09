// Class choices on kind — issue #61. See README.md in this directory.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const stage=process.argv[2];assert(['prepare','check','save','deploy','verify','cleanup'].includes(stage),'stage: prepare|check|save|deploy|verify|cleanup');
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir,'ORCH_TEST_RUN_DIR');outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json'));
const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const rt={state,save,record(k,v){console.log(k,v);save()}};
const APP_NAME='class-check-issue61',ENV='staging',WORKLOAD='class-check',ONLY_CLASS='git-dev';
const IMAGE='postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777';
const c=readConsoleConfig(),s=await openConsole(c,{}),ui=makeUI(s.page,c.baseURL,1800000),{page}=ui;
const appBase=()=>`/applications/${state.app.key}/environments/${ENV}`;
const kubectl=(...args)=>execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f',...args],{encoding:'utf8',timeout:60000,stdio:['ignore','pipe','pipe']});
const nameInput=()=>page.getByLabel('Tên Workload',{exact:true});
async function openGroup(scope,title){
 const g=scope.locator('details').filter({has:page.locator('summary').getByText(title,{exact:true})}).first();
 assert(await g.count(),'Missing group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').first().click();return g;
}
async function newWorkloadPage(){
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();
 await page.getByRole('button',{name:/Xử lý nền/}).first().click();
 await nameInput().waitFor();
}
async function prepare(){
 if(!state.app){
  assert(!(await ui.get('/applications')).applications.some(a=>a.name===APP_NAME),'Inspect existing same-name app before adopting it');
  await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(APP_NAME);
  state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();
 }
 const store=(await ui.get('/secret-stores')).secretStores.find(x=>x.key===input.secretStoreKey);assert(store,'Secret Store');
 await ensureEnvironment(ui,{schemaVersion:3,application:{environment:ENV,variables:{},secretFiles:{}}},state.app,store,{},rt);
 console.log('PASS application prepared',state.app.key);
}
// The catalog serves only `git-dev` for workloads and the default for namespaces (UC-16 BR-33).
async function check(){
 const types=(await ui.get('/resource-types')).resourceTypes;
 const workload=types.find(t=>t.key==='workload'),ns=types.find(t=>t.key==='k8s-namespace');
 assert.deepEqual([workload?.classes,workload?.defaultAvailable],[[ONLY_CLASS],false],'workload Type serves only '+ONLY_CLASS);
 assert.equal(ns?.defaultAvailable,true,'k8s-namespace serves default through a classless criterion');
 await newWorkloadPage();
 const wmore=page.getByRole('button',{name:'Hiện nâng cao Workload',exact:true});if(await wmore.count())await wmore.click();
 await openGroup(page,'Cấu hình ít thay đổi');
 const select=page.getByRole('combobox',{name:'Class Workload',exact:true});
 assert.equal(await select.inputValue(),ONLY_CLASS,'new workload takes the only class');
 const options=await select.locator('option').allTextContents();
 assert.deepEqual(options,[ONLY_CLASS],'no Default option when it is not served');
 await page.screenshot({path:dir+'/check-class.png',fullPage:true});
 console.log('PASS class choices',JSON.stringify({workload:{classes:workload.classes,defaultAvailable:workload.defaultAvailable},namespaceDefault:ns.defaultAvailable,options}));
}
// Saved without touching the class: the only served class is written.
async function saveWorkload(){
 const prior=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===WORKLOAD);
 if(prior){assert.deepEqual(prior.workload,state.saved,'Existing draft differs from the recorded save');console.log('Reused draft',WORKLOAD);return;}
 await newWorkloadPage();
 await nameInput().fill(WORKLOAD);
 const box=page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:'Container 1',exact:true})});
 await box.getByLabel('Tên Container',{exact:true}).fill('main');
 await box.getByLabel('Ảnh Container',{exact:true}).fill(IMAGE);
 const more=box.getByRole('button',{name:/^Hiện nâng cao Container/});if(await more.count())await more.click();
 const startup=await openGroup(box,'Khởi chạy');
 await startup.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill(['sh','-c','exec sleep 2147483647'].join('\n'));
 await ui.submit(appBase()+'/workloads/'+WORKLOAD,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 state.saved=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===WORKLOAD).workload;save();
 assert.equal(state.saved.metadata.class,ONLY_CLASS,'the only class is written');
 console.log('PASS saved with class',state.saved.metadata.class);
}
async function waitOperation(base){
 const b=await ui.get('/delivery-default'),delivery=await ui.get(`/delivery-bindings/${b.bindingId}?version=${b.bindingVersion}`);
 const config={runtime:{profile:'dev-test',timeoutMs:1800000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:ENV},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
 state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};save();
 const deadline=Date.now()+1800000;
 for(;;){
  const app=(await ui.get('/applications/'+state.app.key)).application;
  const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));
  const op=ops.operations?.find(o=>o.reviewRequestId===state.attempt.reviewRequestId);
  if(op){state.attempt.operationId=op.id;if(['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(op.status)){state.attempt.status=op.status;save();writeFileSync(dir+'/operation.json',JSON.stringify(op,null,2));console.log('DEPLOY',op.status);assert.equal(op.status,'SUCCEEDED');return;}}
  if(!state.namespaceAccessReady){const ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;if(kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim()){await ensureNamespaceAccess(ui,config,app,rt,{wait:false});state.namespaceAccessReady=true;save();}}
  assert(Date.now()<deadline,'Operation remains active; inspect before resuming');await page.waitForTimeout(5000);
 }
}
async function deploy(){
 const base=appBase();
 assert(!state.attempt,'A Deploy attempt is recorded; inspect it before retrying');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 writeFileSync(dir+'/preview.json',JSON.stringify(preview,null,2));await page.screenshot({path:dir+'/preview.png',fullPage:true});
 assert.deepEqual((preview.changes??[]).map(x=>x.workloadId),[WORKLOAD],'Preview contains the workload');
 let count=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+base+'/deploy'){count++;state.attempt={reviewRequestId:r.postDataJSON()?.reviewRequestId,status:'UNKNOWN'};save();}});
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 await page.waitForTimeout(3000);assert.equal(count,1,'Exactly one Deploy request');
 await waitOperation(base);
}
async function verify(){
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 const d=JSON.parse(kubectl('-n',ns,'get','deployment',WORKLOAD,'-o','json'));
 assert.equal(d.status.readyReplicas,1,'Deployment ready');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/${ENV}/workloads/${WORKLOAD}`);
 await page.getByRole('heading',{name:'Sửa '+WORKLOAD,exact:true}).waitFor();
 const wmore=page.getByRole('button',{name:'Hiện nâng cao Workload',exact:true});if(await wmore.count())await wmore.click();
 await openGroup(page,'Cấu hình ít thay đổi');
 assert.equal(await page.getByRole('combobox',{name:'Class Workload',exact:true}).inputValue(),ONLY_CLASS,'stored class shown');
 const report={namespace:ns,deployment:WORKLOAD,ready:true,class:ONLY_CLASS};
 writeFileSync(dir+'/verify.json',JSON.stringify(report,null,2));console.log('PASS verify',JSON.stringify(report));
}
async function cleanup(){
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings`);
 // The Fleet access granted by this scenario (outside the product) would keep the namespace alive.
 if(kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim())console.log(kubectl('-n',ns,'delete','role,rolebinding','-l','bootstrap.orchestrator.dev/application='+state.app.key,'--ignore-not-found').trim());
 if(app.status==='DELETING')await page.getByRole('region',{name:'Tiến trình xóa ứng dụng',exact:true}).getByRole('button',{name:'Thử lại',exact:true}).click();
 else await page.getByRole('region',{name:'Xóa ứng dụng',exact:true}).getByRole('button',{name:'Xóa ứng dụng',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'Xóa ứng dụng',exact:true});
 await dialog.getByRole('textbox').fill(app.name);
 const deletion=await ui.submit(`/applications/${state.app.key}/deletions`,'POST',dialog.getByRole('button',{name:'Xóa vĩnh viễn ứng dụng',exact:true}));
 const id=deletion.deletion.operationId,deadline=Date.now()+1800000;let progress;
 for(;;){progress=(await ui.get(`/applications/${state.app.key}/deletions/${id}`)).deletion;if(!progress.running)break;assert(Date.now()<deadline,'deletion still running');await page.waitForTimeout(5000);}
 writeFileSync(dir+'/deletion.json',JSON.stringify(progress,null,2));
 assert.equal(progress.status,'SUCCEEDED',progress.failureCode);
 assert.equal(kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim(),'');
 console.log('PASS cleanup',JSON.stringify({namespace:ns,namespaceAbsent:true,status:progress.status}));
}
try{
 if(stage==='prepare')await prepare();
 else{
  assert(state.app,'Run prepare first');
  if(stage==='check')await check();
  else if(stage==='save')await saveWorkload();
  else if(stage==='deploy')await deploy();
  else if(stage==='verify')await verify();
  else await cleanup();
 }
}catch(error){await page.screenshot({path:`${dir}/${stage}-error.png`,fullPage:true}).catch(()=>{});throw error;}
finally{await closeConsole(s,c);}
