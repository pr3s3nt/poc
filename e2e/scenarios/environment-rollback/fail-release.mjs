// DEFERRED by user on 2026-11-08: live scenario 2 is NOT verified.
// Retained for future review; do not treat this runner as passing acceptance.
import assert from 'node:assert/strict';
import{existsSync,readFileSync,writeFileSync}from'node:fs';
import{execFileSync}from'node:child_process';
import{outsideRepo,makeUI}from'../../../scripts/bootstrap/lib/runtime.mjs';
import{readConsoleConfig,openConsole,closeConsole}from'../../lib/console.mjs';
const app=process.env.ORCH_ROLLBACK_APP,dir=process.env.ORCH_TEST_RUN_DIR,kube=process.env.ORCH_CLUSTER_KUBECONFIG;
assert(app&&dir&&kube&&process.env.ORCH_ROLLBACK_FAILURE==='1');outsideRepo(dir);
assert(JSON.parse(readFileSync(dir+'/failure-setup.json')).prepared,'UI setup is incomplete');
const file=dir+'/failure-attempt.json';assert(!existsSync(file),'Inspect the prior deployment before any retry');
const config=readConsoleConfig(),session=await openConsole(config,{role:'ADMIN'}),ui=makeUI(session.page,config.baseURL,2400000),page=session.page;
const base=`/applications/${encodeURIComponent(app)}/environments/staging`;
let attempt;
try{
 const application=(await ui.get('/applications/'+app)).application,env=application.environments.find(e=>e.key==='staging');assert(!env.activeOperation);
 await page.goto(config.baseURL+`/ui/applications/${app}?environment=staging`);
 const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 assert.equal(preview.changes[0].workloadId,'ad');assert.equal(preview.changes.at(-1).workloadId,'zz-rollback-gate');assert(preview.changes.every(c=>c.action!=='REMOVE'));
 assert(!preview.changes[0].resourceIntents?.some(r=>r.resourceType==='rollback-gate'),'Gate would fail before ad');
 page.on('request',request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){attempt={startedAt:new Date().toISOString(),reviewRequestId:request.postDataJSON()?.reviewRequestId,completion:'UNKNOWN'};writeFileSync(file,JSON.stringify(attempt,null,2),{mode:0o600})}});
 const report=await ui.submit(base+'/deploy','POST',page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}));
 assert(attempt?.reviewRequestId,'Missing exact UI request correlation');attempt.completion=report.status;attempt.report=report;writeFileSync(file,JSON.stringify(attempt,null,2),{mode:0o600});
 assert.equal(report.status,'PARTIAL');
 assert.equal(report.results.find(r=>r.workloadId==='ad')?.status,'SUCCEEDED');
 assert.equal(report.results.find(r=>r.workloadId==='zz-rollback-gate')?.status,'FAILED');
 assert(report.results.filter(r=>r.workloadId!=='zz-rollback-gate').every(r=>r.status==='SUCCEEDED'),'Unexpected earlier failure');
 const after=(await ui.get('/applications/'+app)).application.environments.find(e=>e.key==='staging');assert(!after.activeOperation,'A confirmed Plan failure retained an operation; diagnose before rollback');
 const operations=(await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(attempt.reviewRequestId))).operations;
 assert(operations.some(o=>o.reviewRequestId===attempt.reviewRequestId&&o.status==='FAILED'));
 const objects=JSON.parse(execFileSync('kubectl',['--kubeconfig',kube,'--context','kind-k8s-4f','-n',env.namespaceIdentity,'get','deployments','-o','json'],{encoding:'utf8',timeout:30000}));
 assert.equal(objects.items.length,11,'The gate must not create another Deployment');
 const ad=objects.items.find(d=>d.metadata.name==='ad');assert.equal(ad.status.readyReplicas,1);
 assert.equal(ad.spec.template.spec.containers.find(c=>c.name==='main').env.find(e=>e.name==='ROLLBACK_LIVE_MARKER')?.value,'changed-before-plan-failure','No actual ad runtime change was proved');
 writeFileSync(dir+'/live-partial-failure.json',JSON.stringify({status:'PASSED',adReady:true,adRuntimeMarker:'changed-before-plan-failure',gateWorkloadFailed:true,extraDeployment:false,environmentIdle:true},null,2),{mode:0o600});
 await page.screenshot({path:dir+'/controlled-failure.png',fullPage:true});
 console.log('PASS live partial deployment: ad changed and Ready, gate failed, no extra Deployment, Environment released');
}catch(error){await page.screenshot({path:dir+'/failure-error.png',fullPage:true}).catch(()=>{});console.error(error.message);process.exitCode=1}finally{await closeConsole(session,config)}
