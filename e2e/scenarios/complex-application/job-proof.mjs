import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {execFileSync} from 'node:child_process';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
import {selectDefinitionRevisions} from '../../../scripts/bootstrap/lib/environment.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file,'utf8')):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2));const rt={state,save,record(key,status,value){state[key]=value??status;save();console.log(key,status)}};
const stage=process.argv[2];assert(['author','preview','deploy','status','update'].includes(stage));
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000),{page}=ui;
try{
 if(!state.jobApp){assert.equal(stage,'author');await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill('job-issue50');state.jobApp=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();}
 const app=state.jobApp,base=`/applications/${app.key}/environments/staging`,home=c.baseURL+`/ui/applications/${app.key}?environment=staging`;
 if(stage==='author'){
  const view=await ui.get(base+'/definition-versions'),pins={};for(const row of view.definitions??[]){if(!row.key.startsWith('complex-'))continue;const versions=row.versions.filter(v=>!v.archived);assert.equal(versions.length,1,'Only the newly authored Definition revision is expected');pins[row.key]=versions[0].id;}
  assert.equal(Object.keys(pins).length,3);await page.goto(c.baseURL+`/ui/applications/${app.key}/settings?environment=staging`);await selectDefinitionRevisions(ui,base,pins,rt);
  for(const[name,label,kind,script]of [['z-prepare','Job chạy một lần','Job','echo ISSUE50_PREPARED; date; exit 0'],['a-consumer','Xử lý nền','Deployment','echo ISSUE50_CONSUMER; sleep 36000']]){
   const prior=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===name);if(prior){assert.equal(prior.workload?.workloadExecution?.kind,kind);assert.equal(prior.workload?.containers?.main?.args?.[0],script);console.log('Existing owned draft checked',name);continue;}
   await page.goto(home);await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:new RegExp('^'+label)}).click();await page.getByLabel('Tên Workload',{exact:true}).fill(name);await page.getByLabel('Ảnh Container',{exact:true}).fill('docker.io/library/busybox:1.36.1');
   await page.locator('button[data-workload-advanced]').first().click();await page.getByText('Khởi chạy',{exact:true}).click();await page.getByLabel('Lệnh (mỗi dòng một phần tử)',{exact:true}).fill('/bin/sh\n-c');await page.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill(script);
   if(kind==='Job'){await page.getByLabel('Số lần thử lại khi thất bại',{exact:true}).fill('0');await page.getByLabel('Thời gian chạy tối đa (giây)',{exact:true}).fill('300');}
   else await page.getByLabel('z-prepare',{exact:true}).check();
   await ui.submit(base+'/workloads/'+name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));await page.waitForURL(home);
  }
  const list=(await ui.get(base+'/workloads')).workloads;assert.equal(list.length,2);const job=list.find(w=>w.id==='z-prepare'),consumer=list.find(w=>w.id==='a-consumer');assert.equal(job.workload.workloadExecution.job.backoffLimit,0);assert.deepEqual(consumer.workload.workloadExecution.waitForJobs,['z-prepare']);assert.equal((await ui.get(base+'/deployments')).deployments.length,0);state.jobAuthored=true;save();await page.screenshot({path:dir+'/job-drafts.png',fullPage:true});console.log('PASS both workloads authored via UI before any Deploy');
 }else if(stage==='update'){
  assert.equal(state.jobAttempt?.status,'SUCCEEDED');assert(!state.jobUpdateAttempt,'Update attempt exists; inspect instead of replay');
  await page.goto(c.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/a-consumer`);await page.getByLabel('Tên Workload',{exact:true}).waitFor();await page.locator('button[data-workload-advanced]').first().click();await page.getByText('Khởi chạy',{exact:true}).click();await page.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill('echo ISSUE50_CONSUMER_UPDATED; sleep 36000');await ui.submit(base+'/workloads/a-consumer','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));await page.waitForURL(home);
  const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));assert.deepEqual(preview.changes.map(x=>x.workloadId),['a-consumer']);assert.equal(preview.changes[0].action,'UPDATE');writeFileSync(dir+'/job-update-preview.json',JSON.stringify(preview,null,2));
  let count=0;const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){count++;state.jobUpdateAttempt={reviewRequestId:request.postDataJSON()?.reviewRequestId,status:'UNKNOWN',requestCount:count,startedAt:new Date().toISOString()};save();}};page.on('request',track);await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();page.off('request',track);assert.equal(count,1);assert(state.jobUpdateAttempt?.reviewRequestId);
 }else if(stage!=='status'){
  assert(state.jobAuthored);await page.goto(home);const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));writeFileSync(dir+'/job-preview.json',JSON.stringify(preview,null,2));console.log('Job batch preview',JSON.stringify(preview.changes?.map(x=>({workload:x.workloadId,action:x.action}))));assert.deepEqual(preview.changes.map(x=>x.workloadId),['z-prepare','a-consumer']);state.jobPreviewed=true;save();await page.screenshot({path:dir+'/job-preview.png',fullPage:true});
  if(stage==='deploy'){
   assert(!state.jobAttempt,'Recorded attempt exists; inspect status, never replay');
   const binding=await ui.get('/delivery-default'),details=await ui.get(`/delivery-bindings/${binding.bindingId}?version=${binding.bindingVersion}`);state.deliverySubject={name:details.config.serviceAccount,namespace:details.config.agentNamespace};save();
   let count=0;const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){count++;state.jobAttempt={reviewRequestId:request.postDataJSON()?.reviewRequestId,status:'UNKNOWN',requestCount:count,startedAt:new Date().toISOString()};save();}};page.on('request',track);
   await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();page.off('request',track);assert.equal(count,1);assert(state.jobAttempt?.reviewRequestId);
  }

 }
 if(stage==='deploy'||stage==='status'||stage==='update'){
  const attempt=stage==='update'?state.jobUpdateAttempt:state.jobAttempt;assert(attempt);const input=JSON.parse(readFileSync(dir+'/config.json','utf8'));
  const accessConfig={runtime:{profile:'dev-test',timeoutMs:1200000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:'staging'},platform:{account:{context:input.context},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
  const deadline=Date.now()+1200000;
  for(;;){
   const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(attempt.reviewRequestId));const op=ops.operations?.find(o=>o.reviewRequestId===attempt.reviewRequestId);
   if(op&&['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(op.status)){attempt.status=op.status;attempt.operationId=op.id;save();writeFileSync(dir+(stage==='update'?'/job-update-operation.json':'/job-operation.json'),JSON.stringify(op,null,2));console.log('Job batch operation',op.status);assert.equal(op.status,'SUCCEEDED');break;}
   if(!state.namespaceAccessReady){const ns=app.environments.find(e=>e.key==='staging').namespaceIdentity;const result=execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context',input.context,'get','namespace',ns,'--ignore-not-found','-o','name'],{encoding:'utf8',timeout:15000,stdio:['ignore','pipe','pipe']});if(result.trim()){await ensureNamespaceAccess(ui,accessConfig,app,rt,{wait:false});state.namespaceAccessReady=true;save();}}
   assert(Date.now()<deadline,'Operation still active; inspect without resubmitting');await page.waitForTimeout(2000);
  }
  const ns=app.environments.find(e=>e.key==='staging').namespaceIdentity;
  const objects=JSON.parse(execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context',input.context,'-n',ns,'get','job/z-prepare','deployment/a-consumer','-o','json'],{encoding:'utf8',timeout:15000,stdio:['ignore','pipe','pipe']}));const job=objects.items.find(o=>o.kind==='Job'),consumer=objects.items.find(o=>o.kind==='Deployment');assert.equal(job.status.succeeded,1);assert.equal(consumer.status.readyReplicas,1);assert(consumer.metadata.creationTimestamp>=job.status.completionTime);
  if(state.jobIdentity){assert.equal(job.metadata.uid,state.jobIdentity.uid);assert.equal(job.status.completionTime,state.jobIdentity.completedAt);}else{state.jobIdentity={uid:job.metadata.uid,completedAt:job.status.completionTime};save();}
  if(stage==='update'){const first=JSON.parse(readFileSync(dir+'/job-first-objects.json','utf8')).items.find(o=>o.kind==='Job');assert.equal(job.metadata.uid,first.metadata.uid);assert.equal(job.status.completionTime,first.status.completionTime);state.jobUnchangedAfterConsumerUpdate=true;save();}
  writeFileSync(dir+(stage==='update'?'/job-update-objects.json':'/job-first-objects.json'),JSON.stringify(objects,null,2));await page.goto(home);await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).waitFor();await page.screenshot({path:dir+(stage==='update'?'/job-updated.png':'/job-deployed.png'),fullPage:true});console.log('PASS completed Job before consumer; immutable Job identity retained');
 }

}catch(error){await page.screenshot({path:dir+`/job-${stage}-error.png`,fullPage:true});console.error('Job UI proof failed; inspect private evidence; status',error.status??'unknown');throw error;}finally{await closeConsole(s,c);}
