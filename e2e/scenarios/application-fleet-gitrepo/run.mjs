import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {parseArgs} from 'node:util';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {openConsole,closeConsole,signInContext,signOutContext} from '../../lib/console.mjs';
import {loadBootstrapConfig} from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import {makeUI,createRuntime,outsideRepo,equal,waitIdle} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {envBase} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureWorkloads} from '../../../scripts/bootstrap/lib/workloads.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
import {kubectl,ensureAccess,verifyAcceptance} from '../../../scripts/bootstrap/lib/verification.mjs';
import {verifyApplicationFleet} from '../../../scripts/bootstrap/lib/fleet-verification.mjs';
const {values:args}=parseArgs({options:{'staging-config':{type:'string'},'production-config':{type:'string'},'backend-container':{type:'string'},evidence:{type:'string'},phase:{type:'string'},'resume-recovered':{type:'boolean'},'verify-again':{type:'boolean'}}});
assert(!args['verify-again']||args.phase==='final','Only final verification may repeat');
for(const key of ['staging-config','production-config','backend-container','evidence','phase'])assert(args[key],`Missing --${key}`);
const configs={staging:loadBootstrapConfig(args['staging-config']),production:loadBootstrapConfig(args['production-config'])};
assert.equal(configs.staging.console.baseURL,configs.production.console.baseURL);
assert.equal(configs.staging.application.name,configs.production.application.name);
assert.equal(configs.staging.runtime.profile,'dev-test');assert.equal(configs.production.runtime.profile,'dev-test');
assert.notEqual(configs.staging.runtime.stateDir,configs.production.runtime.stateDir);
outsideRepo(args.evidence);const stateFile=join(args.evidence,'scenario.json');
const state=existsSync(stateFile)?JSON.parse(readFileSync(stateFile,'utf8')):{completed:{}};
const save=()=>writeFileSync(stateFile,JSON.stringify(state,null,2),{mode:0o600});
const sha=bytes=>createHash('sha256').update(bytes).digest('hex');
const docker=(...more)=>execFileSync('docker',['exec',args['backend-container'],...more],{encoding:'utf8',timeout:30000,stdio:['ignore','pipe','pipe']});
let pe,developer,rt;
function gitProof(repo,allowEmpty=false){
 const dirs=docker('find','/var/lib/orchestrator/delivery-git','-maxdepth','2','-type','d','-name','repository').trim().split('\n').filter(Boolean);
 const matches=dirs.filter(dir=>docker('git','-C',dir,'config','--get','remote.origin.url').trim()===repo.spec.repo);assert.equal(matches.length,1,'Scoped application Git checkout');const dir=matches[0];
 const commit=allowEmpty?docker('git','-C',dir,'rev-parse','HEAD').trim():repo.spec.revision;
 const files=docker('git','-C',dir,'ls-tree','-r','--name-only',commit).trim().split('\n').filter(Boolean);
 const pins=JSON.parse(repo.metadata.annotations['orchestrator.io/unit-pins']);const active=Object.values(pins).filter(p=>!p.removed);
 const units=[];
 for(const pin of allowEmpty?[]:active){
  const tree=files.filter(f=>f.startsWith(pin.path+'/')).sort();assert(tree.length>1,'Empty admitted tree');const hash=createHash('sha256');
  for(const file of tree){const bytes=Buffer.from(docker('git','-C',dir,'show',commit+':'+file));hash.update(file.slice(pin.path.length+1));hash.update(Buffer.from([0]));hash.update(createHash('sha256').update(bytes).digest());}
  const digest=hash.digest('hex');assert.equal(digest,pin.digest,'Complete Git tree must match admitted digest');units.push({path:pin.path,digest,fileCount:tree.length});
 }
 const manifestFiles=files.filter(f=>f.startsWith('environments/'));
 if(allowEmpty)assert.equal(manifestFiles.length,0,'Withdrawn manifests remain in Git');
 else assert(manifestFiles.every(f=>active.some(p=>f.startsWith(p.path+'/'))),'Unadmitted manifest subtree');
 assert(files.every(f=>f==='README.md'||f.startsWith('metadata/')&&f.endsWith('/revision.json')||f.startsWith('environments/')&&f.endsWith('.yaml')),'Unexpected source/state/credential file in delivery repo');
 return {url:repo.spec.repo,commit,files,units};
}
async function capture(ui,app,allowNoRepo=false,allowDrafts=false){
 const repos=kubectl(configs.staging,'get','gitrepos.fleet.cattle.io','-A').items;
 const owned=repos.filter(r=>r.metadata.labels?.['orchestrator.io/application-id']===app.key);
 assert.equal(owned.length,allowNoRepo?0:1,'Exactly one Application Fleet GitRepo');
 const bundles=kubectl(configs.staging,'get','bundles.fleet.cattle.io','-A').items;
 const result={appKey:app.key,otherRepoUids:repos.filter(r=>!owned.includes(r)).map(r=>r.metadata.uid).sort(),environments:{}};
 if(state.baseline)assert(equal(result.otherRepoUids,state.baseline.otherRepoUids),'Unrelated Fleet controller inventory changed');
 if(owned.length){const repo=owned[0];assert.equal(repo.status.commit,repo.spec.revision);result.repo={uid:repo.metadata.uid,name:repo.metadata.name,namespace:repo.metadata.namespace,spec:repo.spec,metadata:{name:repo.metadata.name,namespace:repo.metadata.namespace,uid:repo.metadata.uid,labels:repo.metadata.labels,annotations:{'orchestrator.io/unit-pins':repo.metadata.annotations['orchestrator.io/unit-pins']}}};result.git=gitProof(repo);}
 for(const env of ['staging','production']){
  const cfg=configs[env],base=envBase(app.key,env),allWorkloads=(await ui.get(base+'/workloads')).workloads,workloads=allowDrafts?allWorkloads.filter(w=>!w.state):allWorkloads,deployments=(await ui.get(base+'/deployments')).deployments;
  if(allowDrafts)assert(allWorkloads.every(w=>['backend','worker','frontend'].includes(w.id)&&(!w.state||w.state==='PENDING_UPSERT')),'Unexpected draft during runtime refresh');
  const namespace=`app-${app.key}-${env}`;let native=[];
  const ns=kubectl(cfg,'get','namespaces').items.find(n=>n.metadata.name===namespace);
  if(ns)native=kubectl(cfg,'get','deployments,statefulsets,pvc,services,pods','-n',namespace).items;
  const data=native.filter(o=>o.kind==='PersistentVolumeClaim'||o.kind==='StatefulSet').map(o=>({kind:o.kind,name:o.metadata.name,uid:o.metadata.uid})).sort((a,b)=>a.uid.localeCompare(b.uid));
  const runtime=native.filter(o=>o.kind==='Deployment').map(o=>({name:o.metadata.name,uid:o.metadata.uid,replicas:o.spec.replicas,podTemplateHash:sha(JSON.stringify(o.spec.template))})).sort((a,b)=>a.name.localeCompare(b.name));
  let units=[];
  if(workloads.length){assert(workloads.every(w=>w.ready&&!w.state),'Workload is not applied/ready');assert(deployments.length);const detail=await ui.get(base+'/deployments/'+deployments[0].id);assert.equal(deployments[0].status,'SUCCEEDED');
   assert(Object.values(detail.rendering??{}).every(r=>r.driverType==='template'),'Template driver required');
   units=verifyApplicationFleet(repos,bundles,app.key,detail.deployment.environmentId).units;
  }
  result.environments[env]={namespaceUid:ns?.metadata.uid,data,runtime,units,workloads:workloads.map(w=>w.id).sort(),deploymentIds:deployments.map(d=>d.id).sort()};
 }
 return result;
}
async function deployPending(ui,cfg,app,{expectedRemove=false}={}){
 const base=envBase(app.key,cfg.application.environment);await waitIdle(ui,app.key,cfg.application.environment,cfg.runtime.timeoutMs);
 await ui.page.goto(ui.baseURL+`/ui/applications/${app.key}?environment=${cfg.application.environment}`);
 const preview=await ui.submit(base+'/preview','POST',ui.page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 assert(preview.changes.length,'Expected reviewed change');assert(preview.changes.every(c=>['backend','worker','frontend'].includes(c.workloadId)&&(expectedRemove?c.action==='REMOVE':c.action!=='REMOVE')),'Unexpected mutation scope');
 assert(!rt.state.phaseAttempt,'Previous mutation outcome needs reconciliation');
 const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){rt.state.phaseAttempt={phase:args.phase,reviewRequestId:request.postDataJSON()?.reviewRequestId,completion:'UNKNOWN'};rt.save();}};ui.page.on('request',track);
 try{const submission=ui.submit(base+'/deploy','POST',ui.page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}));const [report]=await Promise.all([submission,expectedRemove?Promise.resolve():ensureNamespaceAccess(ui,cfg,app,rt)]);assert.equal(report.status,'SUCCEEDED','Deployment requires diagnosis');delete rt.state.phaseAttempt;rt.save();}
 finally{ui.page.off('request',track);}
 await waitIdle(ui,app.key,cfg.application.environment,cfg.runtime.timeoutMs);
}
function retireForward(cfg,rt,app){
 const forward=rt.state.forward;if(!forward)return;
 assert.equal(forward.namespace,`app-${app.key}-${cfg.application.environment}`);
 assert(!kubectl(cfg,'get','services','-n',forward.namespace).items.some(s=>s.metadata.uid===forward.uid),'Old frontend Service still exists');
 let cmd=[];try{cmd=readFileSync(`/proc/${forward.pid}/cmdline`,'utf8').split('\0');}catch(error){assert.equal(error.code,'ENOENT');}
 if(cmd.length){assert(cmd.includes('port-forward')&&cmd.includes(forward.namespace)&&cmd.includes('svc/frontend')&&cmd.includes(cfg.runtime.clusterKubeconfigFile),'Forward ownership mismatch');process.kill(forward.pid,'SIGTERM');}
 state.retiredForwards??={};state.retiredForwards[cfg.application.environment]=forward;save();
 rt.record('access','WITHDRAWN',{uid:forward.uid});delete rt.state.forward;rt.save();
}
async function importWorkload(ui,cfg,app,name,workload){
 await ui.page.goto(ui.baseURL+`/ui/applications/${app.key}/environments/${cfg.application.environment}/workloads/${name}`);
 await ui.page.getByRole('button',{name:'Nhập Workload',exact:true}).click();const file=join(cfg.runtime.stateDir,'scaled-'+name+'.json');writeFileSync(file,JSON.stringify(workload),{mode:0o600});
 await ui.page.getByLabel('File Workload',{exact:true}).setInputFiles(file);await ui.page.getByText(`Đã phân tích Workload: ${name}`,{exact:false}).waitFor();
 await ui.submit(envBase(app.key,cfg.application.environment)+'/workloads/'+name,'PUT',ui.page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
}
try{
 pe=await openConsole(configs.staging.console,{role:'PLATFORM_ENGINEER'});const platform=makeUI(pe.page,configs.staging.console.baseURL,900000);
 developer=await signInContext(pe.browser,configs.staging.developer,{role:'DEVELOPER'});const ui=makeUI(developer.page,configs.staging.console.baseURL,900000);
 const apps=(await ui.get('/applications')).applications;const app=apps.find(a=>a.name===configs.staging.application.name&&a.subdomain===configs.staging.application.subdomain);assert(app,'Receipt-owned acceptance app missing');
 const recorded=JSON.parse(readFileSync(join(configs.staging.runtime.stateDir,'identity.json'),'utf8'));assert.equal(app.key,recorded.appKey);if(state.appKey)assert.equal(state.appKey,app.key);state.appKey=app.key;save();
 const binding=await platform.get('/delivery-default');if(state.binding)assert(equal(state.binding,binding),'Default binding changed');else state.binding=binding;save();
 if(state.completed[args.phase]&&!args['verify-again']){console.log('REUSED: phase '+args.phase);process.exitCode=0;}
 else {
 const phase=args.phase;const env=phase==='remove-production'||phase==='restore-production'?'production':'staging';const cfg=configs[env];rt=createRuntime(cfg);
 let recoveredBefore;
 if(rt.state.phaseAttempt){
  assert(args['resume-recovered']&&rt.state.phaseAttempt.phase===phase,'Original phase operation requires explicit recovery');
  const previous=rt.state.phaseAttempt;
  const operations=(await ui.get(envBase(app.key,env)+'/operations?reviewRequestId='+encodeURIComponent(previous.reviewRequestId))).operations;
  assert.equal(operations.length,1);assert.equal(operations[0].status,'RECOVERED','Original operation must be recovered by the product');
  rt.state.phaseAttemptHistory??=[];rt.state.phaseAttemptHistory.push({...previous,completion:'RECOVERED',operationId:operations[0].id});
  delete rt.state.phaseAttempt;rt.save();
  state.recoveries??=[];state.recoveries.push({phase,operationId:operations[0].id,reviewRequestId:previous.reviewRequestId});save();
  const prior={scale:'dual','remove-production':'scaled','remove-staging':'remove-production','restore-staging':'remove-staging','restore-production':'restore-staging','refresh-staging':'restore-staging'}[phase];
  recoveredBefore=state[prior];assert(recoveredBefore,'Original pre-mutation checkpoint required');
 }
 state.phaseBefore??={};
 const before=state.phaseBefore[phase]??recoveredBefore??await capture(ui,app,phase==='restore-staging',phase==='refresh-staging');
 if(['scale','remove-production','remove-staging','restore-staging','restore-production','refresh-staging'].includes(phase)&&!state.phaseBefore[phase]){state.phaseBefore[phase]=before;save();}

 if(phase==='baseline'){assert.equal(before.environments.staging.workloads.length,3);assert.equal(before.environments.production.workloads.length,0);state.baseline=before;}
 else if(phase==='production'){assert(state.completed.baseline);assert.equal(before.repo.uid,state.baseline.repo.uid);assert(equal(before.environments.staging,state.baseline.environments.staging),'Production deploy changed staging units/native identities');assert.equal(before.environments.production.workloads.length,3);state.dual=before;}
 else if(phase==='scale'){
  assert(state.completed.production);const current=(await ui.get(envBase(app.key,env)+'/workloads')).workloads.find(w=>w.id==='backend').workload;const desired=structuredClone(current);desired.replicas=(before.environments.staging.runtime.find(r=>r.name==='backend')?.replicas??1)+1;assert.notEqual(current.replicas,desired.replicas);
  await importWorkload(ui,cfg,app,'backend',desired);await deployPending(ui,cfg,app);const after=await capture(ui,app);assert.equal(after.repo.uid,before.repo.uid);assert(equal(after.environments.production,before.environments.production),'Staging scale changed production');assert(equal(after.environments.staging.data,before.environments.staging.data),'Scale replaced data resources');state.scaled=after;
 } else if(phase==='remove-production'||phase==='remove-staging'){
  assert(state.completed[phase==='remove-production'?'scale':'remove-production']);ui.page.on('dialog',dialog=>dialog.accept());
  for(const name of ['frontend','worker','backend']){const current=(await ui.get(envBase(app.key,env)+'/workloads')).workloads.find(w=>w.id===name);if(!current||current.state==='PENDING_DELETE')continue;await ui.page.goto(ui.baseURL+`/ui/applications/${app.key}?environment=${env}`);const row=ui.page.locator('.workload-table .table-row').filter({has:ui.page.locator('.workload-name').filter({hasText:name})});await ui.submit(envBase(app.key,env)+'/workloads/'+name,'DELETE',row.getByRole('button',{name:'Xóa',exact:true}));}
  await deployPending(ui,cfg,app,{expectedRemove:true});const after=await capture(ui,app,phase==='remove-staging');retireForward(cfg,rt,app);assert.equal(after.environments[env].workloads.length,0);assert.equal(after.environments[env].runtime.length,0);assert(equal(after.environments[env].data,before.environments[env].data),'Workload removal deleted data');
  if(phase==='remove-production'){assert.equal(after.repo.uid,before.repo.uid);assert(equal(after.environments.staging,before.environments.staging),'Production withdrawal affected staging');}
  else {after.git=gitProof(state.dual.repo,true);assert.equal(kubectl(cfg,'get','bundles.fleet.cattle.io','-A').items.filter(b=>b.metadata.labels?.['fleet.cattle.io/repo-name']===state.dual.repo.name).length,0);}
  state[phase]=after;
 } else if(phase==='refresh-staging'){
  assert(state.completed['restore-staging']);await deployPending(ui,cfg,app);const after=await capture(ui,app,false,true);assert.equal(after.repo.uid,before.repo.uid);assert(equal(after.environments.staging.data,before.environments.staging.data),'Runtime refresh replaced data');assert(equal(after.environments.production,before.environments.production),'Runtime refresh changed production runtime');state.runtimeRefresh=after;
 } else if(phase==='restore-staging'||phase==='restore-production'){
  assert(state.completed[phase==='restore-staging'?'remove-staging':'restore-staging']);await ensureWorkloads(ui,cfg,app,cfg.application,rt);await deployPending(ui,cfg,app);const after=await capture(ui,app);assert(equal(after.environments[env].data,state.dual.environments[env].data),'Restoration replaced PostgreSQL/Redis/PVC');assert.equal(after.git.url,state.dual.git.url,'Remote application repository changed');if(phase==='restore-staging')assert.notEqual(after.repo.uid,state.dual.repo.uid,'Final controller was not retired');else {assert.equal(after.repo.uid,state['restore-staging'].repo.uid);assert(equal(after.environments.staging,(state.runtimeRefresh??state['restore-staging']).environments.staging),'Production restore changed staging');}state[phase]=after;
 } else if(phase==='final'){
  assert(state.completed['restore-production']);
  for(const key of ['staging','production']){const ecfg=configs[key];const ert=key===env?rt:createRuntime(ecfg);try{await ensureAccess(ui,ecfg,app,ert);await verifyAcceptance(ui,ecfg,ert,{submitJob:false});await ui.page.goto(ui.baseURL+`/ui/applications/${app.key}?environment=${key}`);const preview=await ui.submit(envBase(app.key,key)+'/preview','POST',ui.page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));assert.equal(preview.changes.length,0);assert(!preview.deliveryPending&&!preview.routePending&&!preview.catalogPending&&!preview.definitionsPending);await ui.page.screenshot({path:join(args.evidence,key+'-console.png'),fullPage:true});}finally{if(ert!==rt)ert.close('SUCCEEDED');}}
  const after=await capture(ui,app);assert(equal(after,before),'Unchanged Preview/business checks mutated deployment state');state.final=after;
 } else throw Error('Unknown phase');
 state.completed[phase]=new Date().toISOString();save();rt.close('SUCCEEDED');rt=undefined;console.log('PASS: '+phase);
 }
}catch(error){if(rt){rt.close('FAILED');rt=undefined;}writeFileSync(join(args.evidence,'failure-'+args.phase+'.json'),JSON.stringify({phase:args.phase,message:error.message,at:new Date().toISOString()},null,2),{mode:0o600});throw error;}
finally{if(developer){await signOutContext(developer.context,configs.staging.developer);await developer.context.close();}if(pe)await closeConsole(pe,configs.staging.console);}
