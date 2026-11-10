// Recovery of a first Rollout interrupted after its Prerequisite — issue #48. See README.md.
import assert from 'node:assert/strict';
import {execFileSync,spawn} from 'node:child_process';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const STAGES=['prepare','interrupt','recover','deploy','cleanup','status'];
const stage=process.argv[2];assert(STAGES.includes(stage),'stage: '+STAGES.join('|'));
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir,'ORCH_TEST_RUN_DIR');outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json'));
assert(input.consoleNamespace,'config.consoleNamespace (isolated Console namespace) required');
const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const rt={state,save,record(k,v){console.log(k,v);save()}};
const APP_NAME=process.env.ORCH_APP_NAME||'prereq-issue48',ENV='staging',WORKLOAD='payment',PORT=8080;
const IMAGE='postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777';
const ARGS=['sh','-c','echo started; exec sleep 2147483647'];
const TERMINAL=['SUCCEEDED','FAILED','INTERRUPTED','RECOVERED'];
const c=readConsoleConfig(),s=await openConsole(c,{}),ui=makeUI(s.page,c.baseURL,1800000),{page}=ui;
const appBase=()=>`/applications/${state.app.key}/environments/${ENV}`;
const kubectlArgs=['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f'];
const kubectl=(...args)=>execFileSync('kubectl',[...kubectlArgs,...args],{encoding:'utf8',timeout:120000,stdio:['ignore','pipe','pipe']});
// Technical reads of the isolated Console database: evidence only, never writes.
const sql=q=>kubectl('-n',input.consoleNamespace,'exec','deploy/postgres','-c','postgres','--','psql','-U','orchestrator','-d','orchestrator','-Atc',q).trim();
const environmentBusy=async()=>{await ui.get(appBase()+'/operations');const e=(await ui.get('/applications/'+state.app.key)).application.environments.find(x=>x.key===ENV);return Boolean(e.activeOperation||e.activeOperationId);};
const shot=name=>page.screenshot({path:`${dir}/${name}.png`,fullPage:true});
const namespace=async()=>(await ui.get('/applications/'+state.app.key)).application.environments.find(e=>e.key===ENV).namespaceIdentity;
const intents=ns=>sql(`select u.phase||'|'||i.status||'|'||coalesce(i.receipt->>'IntentID','')||'|'||i.version||'|'||coalesce(i.receipt->'Handle'->>'gitrepoUID','') from delivery_intents i join delivery_units u on u.id=i.unit_id join environments e on e.id=u.environment_id where e.namespace_identity='${ns}' and u.workload_id='${WORKLOAD}' order by i.created_at`).split('\n').filter(Boolean).map(l=>{const [phase,status,receipt,version,gitrepoUID]=l.split('|');return {phase,status,published:receipt!=='',version,gitrepoUID};});

async function openGroup(scope,title){
 const g=scope.locator('details').filter({has:page.locator('summary').getByText(title,{exact:true})}).first();
 assert(await g.count(),'Missing group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').first().click();return g;
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
 // A Service port makes the Service a separate Prerequisite unit.
 const net=await openGroup(page,'Service & kết nối');
 await net.getByRole('button',{name:'+ Thêm cổng',exact:true}).click();
 await net.getByLabel('Tên cổng Service',{exact:true}).fill('http');await net.getByLabel('Cổng Service',{exact:true}).fill(String(PORT));
 const box=page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:'Container 1',exact:true})});
 await box.getByLabel('Tên Container',{exact:true}).fill('main');
 await box.getByLabel('Ảnh Container',{exact:true}).fill(IMAGE);
 const more=box.getByRole('button',{name:/^Hiện nâng cao Container/});if(await more.count())await more.click();
 const startup=await openGroup(box,'Khởi chạy');await startup.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill(ARGS.join('\n'));
 await shot('authoring');
 await ui.submit(appBase()+'/workloads/'+WORKLOAD,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 state.saved=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===WORKLOAD).workload;save();
 console.log('PASS prepare: workload with a Service port saved through the form');
}
async function namespaceAccess(){
 if(state.namespaceAccessReady)return;
 if(!state.deliverySubject){const b=await ui.get('/delivery-default'),d=await ui.get(`/delivery-bindings/${b.bindingId}?version=${b.bindingVersion}`);state.deliverySubject={name:d.config.serviceAccount,namespace:d.config.agentNamespace};save();}
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 if(!kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim())return;
 const config={runtime:{profile:'dev-test',timeoutMs:1800000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:ENV},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
 await ensureNamespaceAccess(ui,config,app,rt,{wait:false});state.namespaceAccessReady=true;save();
}
// Stops the Console backend at the first moment the Prerequisite is
// acknowledged while the Rollout intent is running without a receipt.
function watchAndStop(ns){
 const query=`select string_agg(u.phase||':'||i.status||':'||(coalesce(i.receipt->>'IntentID','')<>'')::text, ',') from delivery_intents i join delivery_units u on u.id=i.unit_id join environments e on e.id=u.environment_id where e.namespace_identity='${ns}' and u.workload_id='${WORKLOAD}' and i.environment_operation_id=e.active_operation_id \\watch 0.2\n`;
 const psql=spawn('kubectl',[...kubectlArgs,'-n',input.consoleNamespace,'exec','-i','deploy/postgres','-c','postgres','--','psql','-U','orchestrator','-d','orchestrator','-At'],{stdio:['pipe','pipe','ignore']});
 psql.stdin.write(query);
 return new Promise((resolve,reject)=>{
  let buffer='',done=false;const timer=setTimeout(()=>{if(!done){done=true;psql.kill();reject(new Error('Rollout start not observed'));}},900000);
  psql.stdout.on('data',chunk=>{
   if(done)return;buffer+=chunk;const lines=buffer.split('\n');buffer=lines.pop();
   for(const line of lines){
    const parts=line.split(',');
    if(parts.some(p=>p.startsWith('PREREQUISITE:ACKNOWLEDGED'))&&parts.includes('ROLLOUT:RUNNING:false')){
     done=true;clearTimeout(timer);
     kubectl('-n',input.consoleNamespace,'delete','pod','-l','app=backend','--grace-period=0','--force','--wait=false');
     psql.kill();resolve({observed:line,stoppedAt:new Date().toISOString()});return;
    }
    if(parts.some(p=>p.startsWith('ROLLOUT:')&&!p.startsWith('ROLLOUT:RUNNING')&&!p.startsWith('ROLLOUT:PENDING'))){done=true;clearTimeout(timer);psql.kill();reject(new Error('Rollout progressed before the stop: '+line));return;}
   }
  });
 });
}
// A read may hit the backend while it restarts; bound each one.
const getWithin=(path,ms=30000)=>Promise.race([ui.get(path),new Promise((_,reject)=>setTimeout(()=>reject(new Error('timeout '+path)),ms))]);
async function interrupt(){
 if(state.attempt?.stoppedAt&&state.kept)return classify();
 assert(!state.attempt,'An attempt is already recorded; use a new run directory');
 const workloads=(await ui.get(appBase()+'/workloads')).workloads;
 assert.deepEqual(workloads.find(w=>w.id===WORKLOAD)?.workload,state.saved,'Draft changed outside the recorded UI save');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 const preview=await ui.submit(appBase()+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 assert.deepEqual((preview.changes??[]).map(x=>x.workloadId),[WORKLOAD]);
 const ns=await namespace();state.namespace=ns;save();
 let count=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+appBase()+'/deploy'){count++;state.attempt={startedAt:new Date().toISOString(),reviewRequestId:r.postDataJSON()?.reviewRequestId,status:'UNKNOWN'};save();}});
 const stop=watchAndStop(ns);
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click().catch(()=>{});
 // Fleet needs namespace access before the Prerequisite can become Ready.
 let stopped;const access=(async()=>{while(!stopped){try{await namespaceAccess();}catch{}if(state.namespaceAccessReady)return;await new Promise(r=>setTimeout(r,2000));}})();
 stopped=await stop;await access;
 assert.equal(count,1,'Exactly one Deploy request');
 Object.assign(state.attempt,stopped);save();
 console.log('STOPPED backend',JSON.stringify(stopped));
 kubectl('-n',input.consoleNamespace,'rollout','status','deploy/backend','--timeout=300s');
 // Snapshot what recovery must keep: Prerequisite intent, Service and controller.
 const before=intents(ns),prereq=before.find(x=>x.phase==='PREREQUISITE'&&x.status==='ACKNOWLEDGED');
 assert(prereq&&prereq.published,'Prerequisite acknowledged before the stop');
 const service=kubectl('-n',ns,'get','service',WORKLOAD,'-o','jsonpath={.metadata.uid}').trim();assert(service,'Prerequisite Service exists');
 state.kept={prerequisite:prereq,serviceUID:service,gitrepoUID:prereq.gitrepoUID,rolloutPublished:before.some(x=>x.phase==='ROLLOUT'&&x.published)};save();
 return classify();
}
async function classify(){
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`).catch(()=>{});
 const deadline=Date.now()+900000;let op;
 for(;;){
  // The unfiltered list classifies a stale heartbeat (as the Settings page does);
  // the reviewRequestId filter only reads.
  try{await getWithin(appBase()+'/operations');op=(await getWithin(appBase()+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId))).operations?.find(o=>o.reviewRequestId===state.attempt.reviewRequestId);}catch{}
  if(op&&TERMINAL.includes(op.status))break;
  assert(Date.now()<deadline,'Operation not classified after the stop');await page.waitForTimeout(5000);
 }
 Object.assign(state.attempt,{operationId:op.id,status:op.status});save();
 writeFileSync(`${dir}/operation-${op.id}.json`,JSON.stringify(op,null,2));
 console.log('INTERRUPT',JSON.stringify({status:op.status,kept:state.kept}));
 assert.equal(op.status,'INTERRUPTED');assert.equal(await environmentBusy(),true,'The unknown Rollout holds the Environment');
 assert.equal(state.kept.rolloutPublished,false,'Rollout already published; this run did not reach the issue #48 case');
 console.log('PASS interrupt: Prerequisite acknowledged, Rollout unpublished, Environment held');
}
async function recover(){
 assert(state.attempt?.status==='INTERRUPTED','No interrupted attempt');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings?environment=${ENV}`);
 await page.getByLabel('Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn').check();
 const result=await ui.submit(`${appBase()}/operations/${state.attempt.operationId}/recover`,'POST',page.getByRole('button',{name:'Khôi phục môi trường',exact:true}));
 writeFileSync(dir+'/recovery.json',JSON.stringify(result,null,2));await page.waitForTimeout(1500);await shot('recovery');
 const op=(await ui.get(appBase()+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId))).operations.find(o=>o.id===state.attempt.operationId);
 const ns=state.namespace,after=intents(ns);
 const rollout=after.filter(x=>x.phase==='ROLLOUT'),prereq=after.find(x=>x.phase==='PREREQUISITE'&&x.status==='ACKNOWLEDGED');
 const evidence={outcome:result.recovery?.outcome,status:op.status,environmentBusy:await environmentBusy(),rollout,prerequisite:prereq,
  serviceUID:kubectl('-n',ns,'get','service',WORKLOAD,'-o','jsonpath={.metadata.uid}').trim(),
  deployment:kubectl('-n',ns,'get','deployment',WORKLOAD,'--ignore-not-found','-o','name').trim()};
 writeFileSync(dir+'/recovery-evidence.json',JSON.stringify(evidence,null,2));
 console.log('RECOVER',JSON.stringify(evidence));
 assert.equal(op.status,'RECOVERED');assert.equal(evidence.environmentBusy,false);
 assert(rollout.length===1&&rollout[0].status==='FAILED'&&!rollout[0].published,'Rollout settled FAILED without publication');
 assert.deepEqual(prereq,state.kept.prerequisite,'Prerequisite intent unchanged');
 assert.equal(evidence.serviceUID,state.kept.serviceUID,'Prerequisite Service unchanged');
 assert.equal(evidence.deployment,'','No Rollout workload was applied');
 assert.match(await page.locator('body').innerText(),/./);
 state.attempt.status='RECOVERED';save();
 console.log('PASS recovery: Rollout FAILED unpublished, Prerequisite and Service kept, Environment released');
}
async function deploy(){
 assert.equal(state.attempt?.status,'RECOVERED','Recover first');
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 await ui.submit(appBase()+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 let reviewRequestId;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+appBase()+'/deploy')reviewRequestId=r.postDataJSON()?.reviewRequestId;});
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 const deadline=Date.now()+1500000;let op;
 for(;;){await page.waitForTimeout(5000);if(!reviewRequestId)continue;op=(await ui.get(appBase()+'/operations?reviewRequestId='+encodeURIComponent(reviewRequestId))).operations?.find(o=>o.reviewRequestId===reviewRequestId);if(op&&TERMINAL.includes(op.status))break;assert(Date.now()<deadline,'Deploy still active');}
 await shot('deploy-after-recovery');
 const gitrepo=intents(state.namespace).filter(x=>x.gitrepoUID).map(x=>x.gitrepoUID);
 console.log('DEPLOY',JSON.stringify({status:op.status,controllers:[...new Set(gitrepo)]}));
 assert.equal(op.status,'SUCCEEDED');
 assert.deepEqual([...new Set(gitrepo)],[state.kept.gitrepoUID],'Fleet controller identity unchanged');
 assert.equal(kubectl('-n',state.namespace,'get','service',WORKLOAD,'-o','jsonpath={.metadata.uid}').trim(),state.kept.serviceUID,'Service kept across redeploy');
 console.log('PASS deploy after recovery: SUCCEEDED with the same controller and Service');
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
  if(stage==='status')console.log(JSON.stringify({attempt:state.attempt,kept:state.kept}));
  else if(stage==='interrupt')await interrupt();
  else if(stage==='recover')await recover();
  else if(stage==='deploy')await deploy();
  else if(stage==='cleanup')await cleanup();
 }
}catch(e){await shot(stage+'-error').catch(()=>{});console.error(stage,e.message);process.exitCode=1;}finally{await closeConsole(s,c).catch(()=>{});}
