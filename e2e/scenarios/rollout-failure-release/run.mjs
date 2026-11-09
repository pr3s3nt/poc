// Determined rollout failure releases the Environment — issue #55. See README.md.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const STAGES=['prepare','deploy','recover','fix','cleanup','status'];
const stage=process.argv[2];assert(STAGES.includes(stage),'stage: '+STAGES.join('|'));
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir,'ORCH_TEST_RUN_DIR');outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json'));
const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const rt={state,save,record(k,v){console.log(k,v);save()}};
const APP_NAME=process.env.ORCH_APP_NAME||'rollfail-issue55',ENV='staging',WORKLOAD='crash';
// The image is present on the kind node; the arguments decide crash or run.
const IMAGE='postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777';
const BAD=['sh','-c','echo missing subcommand; exit 0'],GOOD=['sh','-c','echo started; exec sleep 2147483647'];
const TERMINAL=['SUCCEEDED','FAILED','INTERRUPTED','RECOVERED'];
const c=readConsoleConfig(),s=await openConsole(c,{}),ui=makeUI(s.page,c.baseURL,1800000),{page}=ui;
const appBase=()=>`/applications/${state.app.key}/environments/${ENV}`;
const kubectl=(...args)=>execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f',...args],{encoding:'utf8',timeout:60000,stdio:['ignore','pipe','pipe']});
// Busy means the Environment still holds an operation claim.
const environmentBusy=async()=>{await ui.get(appBase()+'/operations');const e=(await ui.get('/applications/'+state.app.key)).application.environments.find(x=>x.key===ENV);return Boolean(e.activeOperation||e.activeOperationId);};
const shot=name=>page.screenshot({path:`${dir}/${name}.png`,fullPage:true});

async function openGroup(scope,title){
 const g=scope.locator('details').filter({has:page.locator('summary').getByText(title,{exact:true})}).first();
 assert(await g.count(),'Missing group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').first().click();return g;
}
async function setArgs(args){
 const box=page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:'Container 1',exact:true})});
 const more=box.getByRole('button',{name:/^Hiện nâng cao Container/});if(await more.count())await more.click();
 const startup=await openGroup(box,'Khởi chạy');
 await startup.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill(args.join('\n'));
 return box;
}
async function saveWorkload(){
 await ui.submit(appBase()+'/workloads/'+WORKLOAD,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 state.saved=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===WORKLOAD).workload;save();
}
async function prepare(){
 if(!state.app){
  assert(!(await ui.get('/applications')).applications.some(a=>a.name===APP_NAME),'Inspect existing same-name app before adopting it');
  await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(APP_NAME);
  state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();
 }
 const store=(await ui.get('/secret-stores')).secretStores.find(x=>x.key===input.secretStoreKey);assert(store,'Secret Store');
 await ensureEnvironment(ui,{schemaVersion:3,application:{environment:ENV,variables:{},secretFiles:{}}},state.app,store,{},rt);
 if((await ui.get(appBase()+'/workloads')).workloads.some(w=>w.id===WORKLOAD)){console.log('Workload draft exists');return;}
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();
 await page.getByRole('button',{name:/Xử lý nền/}).first().click();
 await page.getByLabel('Tên Workload',{exact:true}).fill(WORKLOAD);
 const wmore=page.getByRole('button',{name:'Hiện nâng cao Workload',exact:true});if(await wmore.count())await wmore.click();
 const rare=await openGroup(page,'Cấu hình ít thay đổi');await rare.getByRole('combobox',{name:'Class Workload',exact:true}).selectOption('git-dev');
 const box=page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:'Container 1',exact:true})});
 await box.getByLabel('Tên Container',{exact:true}).fill('main');
 await box.getByLabel('Ảnh Container',{exact:true}).fill(IMAGE);
 await setArgs(BAD);
 await shot('authoring-bad');await saveWorkload();
 console.log('PASS prepare: workload saved with arguments that exit immediately');
}
async function fix(){
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/${ENV}/workloads/${WORKLOAD}`);
 await page.getByLabel('Ảnh Container',{exact:true}).first().waitFor();
 await setArgs(GOOD);await shot('authoring-fixed');await saveWorkload();
 assert.deepEqual(state.saved.containers.main.args,GOOD);
 console.log('PASS fix: corrected arguments saved through the form');
}
async function deliverySubject(){if(!state.deliverySubject){const b=await ui.get('/delivery-default'),d=await ui.get(`/delivery-bindings/${b.bindingId}?version=${b.bindingVersion}`);state.deliverySubject={name:d.config.serviceAccount,namespace:d.config.agentNamespace};save();}}
// Fleet needs scoped access in the product-created namespace (technical setup).
async function namespaceAccess(){
 if(state.namespaceAccessReady)return;
 await deliverySubject();
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 if(!kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim())return;
 const config={runtime:{profile:'dev-test',timeoutMs:1800000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:ENV},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
 await ensureNamespaceAccess(ui,config,app,rt,{wait:false});state.namespaceAccessReady=true;save();
}
async function deploy(){
 const expect=process.env.ORCH_EXPECT;assert(TERMINAL.includes(expect),'ORCH_EXPECT: '+TERMINAL.join('|'));
 if(state.attempt&&!TERMINAL.includes(state.attempt.status))throw new Error('Prior attempt is not terminal; inspect it before deploying again');
 const workloads=(await ui.get(appBase()+'/workloads')).workloads;
 assert.deepEqual(workloads.find(w=>w.id===WORKLOAD)?.workload,state.saved,'Draft changed outside the recorded UI save');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 const preview=await ui.submit(appBase()+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 assert.deepEqual((preview.changes??[]).map(x=>x.workloadId),[WORKLOAD]);
 let count=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+appBase()+'/deploy'){count++;state.attempt={startedAt:new Date().toISOString(),reviewRequestId:r.postDataJSON()?.reviewRequestId,status:'UNKNOWN'};save();}});
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 await page.waitForTimeout(3000);assert.equal(count,1,'Exactly one Deploy request');
 const started=Date.now(),deadline=started+1500000;let op;
 for(;;){
  await namespaceAccess();
  op=(await ui.get(appBase()+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId))).operations?.find(o=>o.reviewRequestId===state.attempt.reviewRequestId);
  if(op&&TERMINAL.includes(op.status))break;
  assert(Date.now()<deadline,'Operation still active');await page.waitForTimeout(5000);
 }
 Object.assign(state.attempt,{operationId:op.id,status:op.status,seconds:Math.round((Date.now()-started)/1000)});save();
 writeFileSync(`${dir}/operation-${state.attempt.operationId}.json`,JSON.stringify(op,null,2));
 const busy=await environmentBusy();
 await page.reload();await page.waitForTimeout(2000);await shot(`deploy-${op.status.toLowerCase()}`);
 console.log('DEPLOY',JSON.stringify({status:op.status,seconds:state.attempt.seconds,environmentBusy:busy}));
 assert.equal(op.status,expect);
 if(expect==='FAILED'){
  assert.equal(busy,false,'A determined failure must release the Environment');
  const list=(await ui.get(appBase()+'/deployments')).deployments.filter(d=>d.workloadId===WORKLOAD).sort((a,b)=>b.startedAt.localeCompare(a.startedAt));
  assert(['ROLLOUT_DEGRADED','ROLLOUT_NOT_READY'].includes(list[0].failureCode),'rollout failure code '+list[0].failureCode);
  await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/${ENV}/deployments/${list[0].id}`);
  const alert=page.getByRole('alert').first();await alert.waitFor();
  assert.match(await alert.innerText(),/Workload đã được triển khai/);
  assert(await alert.getByRole('link',{name:'Xem log',exact:true}).count(),'log link');
  await shot('deployment-detail-failed');
  console.log('PASS determined failure: FAILED, Environment released, rollout reason and log link shown',list[0].failureCode);
 }
 if(expect==='INTERRUPTED')assert.equal(busy,true,'The old release holds the Environment');
}
async function recover(){
 assert(state.attempt?.operationId,'No recorded attempt');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings?environment=${ENV}`);
 await page.getByLabel('Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn').check();
 const started=Date.now();
 const result=await ui.submit(`${appBase()}/operations/${state.attempt.operationId}/recover`,'POST',page.getByRole('button',{name:'Khôi phục môi trường',exact:true}));
 const seconds=Math.round((Date.now()-started)/1000);
 writeFileSync(dir+'/recovery.json',JSON.stringify(result,null,2));await shot('recovery');
 const busy=await environmentBusy();
 const op=(await ui.get(appBase()+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId))).operations.find(o=>o.id===state.attempt.operationId);
 console.log('RECOVER',JSON.stringify({outcome:result.recovery?.outcome,status:op?.status,seconds,environmentBusy:busy}));
 assert.equal(op.status,'RECOVERED');assert.equal(busy,false);
 state.attempt.status='RECOVERED';save();
 console.log('PASS recovery released the Environment');
}
async function cleanup(){
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings`);
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
  if(stage==='status')console.log(JSON.stringify(state.attempt??{}));
  else if(stage==='deploy')await deploy();
  else if(stage==='recover')await recover();
  else if(stage==='fix')await fix();
  else if(stage==='cleanup')await cleanup();
 }
}catch(e){await shot(stage+'-error');console.error(stage,e.message);process.exitCode=1;}finally{await closeConsole(s,c);}
