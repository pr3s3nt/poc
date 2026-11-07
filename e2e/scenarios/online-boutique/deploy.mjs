import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
const dir=process.env.ORCH_TEST_RUN_DIR;if(!dir)throw Error('ORCH_TEST_RUN_DIR required');
outsideRepo(dir);
const state=JSON.parse(readFileSync(dir+'/state.json'));
const config={runtime:{profile:'dev-test',timeoutMs:1200000,clusterKubeconfigFile:process.env.ORCH_CLUSTER_KUBECONFIG},application:{environment:'staging'},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
assert(config.runtime.clusterKubeconfigFile,'ORCH_CLUSTER_KUBECONFIG required');
const rt={state,save(){writeFileSync(dir+'/state.json',JSON.stringify(state,null,2),{mode:0o600})},record(key,status){console.log(key,status);this.save()}};
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,1200000),{page}=ui;
const base=`/applications/${state.app.key}/environments/staging`;
try{
const app=(await ui.get('/applications/'+state.app.key)).application;
const binding=(await ui.get('/delivery-bindings')).bindings.find(b=>b.name==='boutique-fleet');assert(binding);
const delivery=await ui.get(`/delivery-bindings/${binding.id}?version=${binding.version}`);
state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};rt.save();
assert.equal(binding.status,'READY');assert(state.deliverySubject.name&&state.deliverySubject.namespace);console.log('Delivery READY; scoped subject verified');
const workloads=(await ui.get(base+'/workloads')).workloads;assert.equal(workloads.length,11);
await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=staging`);
const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
writeFileSync(dir+'/preview.json',JSON.stringify(preview,null,2),{mode:0o600});
console.log('Preview',JSON.stringify({changes:preview.changes?.map(x=>({workload:x.workloadId,action:x.action})),errors:preview.errors,status:preview.status}));
assert(preview.changes?.length>0);assert(preview.changes.every(x=>workloads.some(w=>w.id===x.workloadId)));assert(preview.changes.every(x=>x.action!=='REMOVE'));
if(process.env.ORCH_DEPLOY!=='1'){console.log('PASS Preview only; set ORCH_DEPLOY=1 for the authorized deployment');}
else{
if(state.deployAttempt){
 assert.equal(process.env.ORCH_RETRY_CONFIRMED,'1','Inspect and fix the prior failed attempt before explicitly retrying');
 assert.equal(state.deployAttempt.completion,'FAILED');
 assert(!app.environments.find(e=>e.key==='staging').activeOperation,'Prior operation must be released');
 state.previousAttempts??=[];state.previousAttempts.push(state.deployAttempt);delete state.deployAttempt;rt.save();
}
page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+base+'/deploy'){state.deployAttempt={startedAt:new Date().toISOString(),reviewRequestId:r.postDataJSON()?.reviewRequestId,completion:'UNKNOWN'};rt.save()}});
await Promise.all([page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click(),ensureNamespaceAccess(ui,config,app,rt)]);
assert(state.deployAttempt?.reviewRequestId,'Console Deploy request ID was not observed');
const deadline=Date.now()+config.runtime.timeoutMs;let report;
for(;;){
 const latest=(await ui.get('/applications/'+state.app.key)).application.environments.find(e=>e.key==='staging');
 const history=await ui.get(base+'/deployments?reviewRequestId='+encodeURIComponent(state.deployAttempt.reviewRequestId));
 const records=history.deployments??[];
 const operations=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.deployAttempt.reviewRequestId));
 const exactOperation=operations.operations?.find(op=>op.reviewRequestId===state.deployAttempt.reviewRequestId);
 // Admission runs after the browser request is emitted. An empty history is
 // not completion; keep the page alive until this exact request is admitted.
 if(!exactOperation){
  if(Date.now()>deadline)throw new Error('Deployment admission was not observed; inspect request before retry');
  await page.waitForTimeout(1000);continue;
 }
 if(['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(exactOperation.status)){
  const byWorkload=new Map(records.map(d=>[d.workloadId,d]));
  const results=preview.changes.map(change=>{const d=byWorkload.get(change.workloadId);return{workloadId:change.workloadId,action:change.action,status:d?.status??'SKIPPED',...(d?{deploymentId:d.id}:{}),...(d?.failureReason?{error:d.failureReason}:{})}});
  const failed=results.some(r=>r.status==='FAILED');
  const succeeded=results.some(r=>r.status==='SUCCEEDED');
  report={status:failed?(succeeded?'PARTIAL':'FAILED'):(results.every(r=>r.status==='SUCCEEDED')?'SUCCEEDED':'PARTIAL'),results,operation:latest.activeOperation&&{id:latest.activeOperation.id,status:latest.activeOperation.status,stage:latest.activeOperation.stage}};
  break;
 }
 if(Date.now()>deadline)throw new Error('Console Deploy remains active; inspect operation before resuming');
 await page.waitForTimeout(2000);
}
state.deployAttempt.completion=report.status;rt.save();writeFileSync(dir+'/deployment.json',JSON.stringify(report,null,2),{mode:0o600});
console.log('Deploy',report.status);assert.equal(report.status,'SUCCEEDED');
}
}catch(e){await page.screenshot({path:dir+'/deployment-error.png',fullPage:true});console.error('Deployment error',e.message);process.exitCode=1}finally{await closeConsole(s,c)}
