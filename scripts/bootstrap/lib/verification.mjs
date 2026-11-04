import { openWorkloadEvidence } from './video.mjs';
import { execFileSync, spawn } from 'node:child_process';
import { openSync, closeSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { requireConfig, BootstrapError } from './bootstrap-config.mjs';
import { conflict, digest, equal } from './runtime.mjs';
export const CHECKS=['backend connection','environment','secret','database','cache'];
export function kubectl(config,...args) {
 try{return JSON.parse(execFileSync('kubectl',['--kubeconfig',config.runtime.clusterKubeconfigFile,'--context',config.platform.account.context,...args,'-o','json'],{encoding:'utf8',timeout:30000,stdio:['ignore','pipe','pipe']}));}
 catch{throw new BootstrapError('Cluster prerequisite/read failed; check configured kubeconfig/context');}
}
export function inspectCluster(config) {
 const nodes=kubectl(config,'get','nodes').items;
 requireConfig(nodes.length&&nodes.every(n=>n.status.conditions.some(c=>c.type==='Ready'&&c.status==='True')),'Cluster nodes not Ready');
 for(const crd of ['gitrepos.fleet.cattle.io','vaultstaticsecrets.secrets.hashicorp.com'])kubectl(config,'get','crd',crd);
 const deploys=kubectl(config,'get','deployments','-A').items;
 requireConfig(deploys.some(v=>v.metadata.name==='fleet-controller'&&v.status.availableReplicas>0),'Fleet controller missing/not Ready');
 requireConfig(deploys.some(v=>v.metadata.name.includes('vault-secrets-operator')&&v.status.availableReplicas>0),'VSO controller missing/not Ready');
 const agent=deploys.filter(v=>v.metadata.name==='fleet-agent');requireConfig(agent.length===1,'Ambiguous Fleet agent');
 const d=config.platform.delivery,subject=`system:serviceaccount:${d.config.agentNamespace??agent[0].metadata.namespace}:${d.config.serviceAccount??'orchestrator-delivery'}`;
 try {execFileSync('kubectl',['--kubeconfig',config.runtime.clusterKubeconfigFile,'--context',config.platform.account.context,'auth','can-i','patch',`namespace/${d.config.releaseNamespace??'fleet-releases'}`,`--as=${subject}`],{encoding:'utf8',timeout:15000,stdio:['ignore','pipe','pipe']});}
 catch {throw new BootstrapError('Fleet cannot patch its release Namespace; request explicitly scoped RBAC before bootstrap');}
}
export async function ensureAccess(ui,config,app,rt) {
 const forward=config.verification.forward;
 if(!forward)return;
 const namespace=`app-${app.key}-${config.application.environment}`;
 const service=kubectl(config,'-n',namespace,'get','service','frontend');
 const identity={uid:service.metadata.uid,namespace,url:config.verification.url};
 if(rt.state.forward) {
  conflict(rt.state.forward.uid===identity.uid&&rt.state.forward.url===identity.url,'Access service identity');
  let alive=false;try{process.kill(rt.state.forward.pid,0);alive=true;}catch{}
  if(alive){
   // A service forward stays attached to its original pod. The first request
   // after a rollout can reveal that the pod disappeared, despite a live PID.
   const reachable=await ui.page.request.get(identity.url,{timeout:5000}).then(r=>r.ok()).catch(()=>false);
   if(reachable){rt.record('access','REUSED');return;}
   let command=[];
   try{command=readFileSync(`/proc/${rt.state.forward.pid}/cmdline`,'utf8').split('\0');}catch(e){if(e.code!=='ENOENT')throw new BootstrapError('Cannot prove ownership of access forward');}
   if(command.length>1){
    conflict(command.includes('port-forward')&&command.includes(namespace)&&command.includes('svc/frontend')&&command.includes(config.runtime.clusterKubeconfigFile)&&command.includes(`${forward.port}:8080`),'Access forward process ownership');
    process.kill(rt.state.forward.pid,'SIGTERM');
    await ui.page.waitForTimeout(250);
   }
  }
 }
 const log=openSync(join(config.runtime.stateDir,'port-forward.log'),'a',0o600);
 const child=spawn('kubectl',['--kubeconfig',config.runtime.clusterKubeconfigFile,'--context',config.platform.account.context,'-n',namespace,'port-forward','--address',forward.bindAddress,'svc/frontend',`${forward.port}:8080`],{detached:true,stdio:['ignore',log,log]});
 closeSync(log);child.on('error',()=>{});child.unref();
 rt.state.forward={...identity,pid:child.pid};rt.save();
 await ui.page.waitForTimeout(2000);
 try{process.kill(child.pid,0);}catch{throw new BootstrapError('Access forward failed; inspect private port-forward.log');}
 rt.record('access','CREATED');
}
export async function verifyAcceptance(ui,config,rt,{submitJob=true}={}) {
 const {url,timeoutMs,jobPayload}=config.verification;
 const namespace=`app-${rt.state.appKey}-${config.application.environment}`;
 const deployments=kubectl(config,'-n',namespace,'get','deployments').items;
 requireConfig(['backend','worker','frontend'].every(name=>deployments.some(d=>d.metadata.name===name&&(d.status.availableReplicas??0)>=(d.spec.replicas??1))),'Acceptance deployments not Ready');
 const sts=kubectl(config,'-n',namespace,'get','statefulsets').items;
 for(const type of ['postgres','redis'])requireConfig(sts.filter(v=>v.metadata.name.startsWith(type+'-')).length===1,`Expected exactly one shared ${type}`);
 const pvcs=kubectl(config,'-n',namespace,'get','pvc').items;
 requireConfig(pvcs.length>0&&pvcs.every(v=>v.status.phase==='Bound'),'Data PVCs not Bound');
 const dataUids=[...sts,...pvcs].map(v=>v.metadata.uid).sort();
 if(rt.state.dataUids)conflict(equalArrays(rt.state.dataUids,dataUids),'Data resource identities after upgrade');
 rt.state.dataUids=dataUids;rt.save();
 const evidence=await openWorkloadEvidence(ui.page.context().browser(),config,'acceptance-app');
 const {page}=evidence;let status='FAILED';
 try {
  const deadline=Date.now()+timeoutMs;
  for(;;){
   await page.goto(url);
   const rows=await page.locator('#checks tr[data-check]').evaluateAll(nodes=>nodes.map(n=>[n.getAttribute('data-check'),n.children[1]?.textContent.trim()]));
   if(CHECKS.every(name=>rows.some(([n,s])=>n===name&&s==='PASS')))break;
   if(Date.now()>deadline)throw new BootstrapError('Acceptance app business checks incomplete');
   await page.waitForTimeout(2000);
  }
  const job=()=>page.locator('#jobs tr.job').filter({hasText:jobPayload});
  const before=await job().count();conflict(before<=1,'Duplicate acceptance job');
  if(before===0) {
   requireConfig(submitJob,'Baseline job missing after upgrade');
   // Record intent before submitting. Unknown outcome is observed, never blindly replayed.
   requireConfig(!rt.state.jobIntent,'Unknown job submission; inspect DB before retry');
   rt.state.jobIntent=true;rt.save();
   await page.locator('input[name="payload"]').fill(jobPayload);
   await page.getByRole('button',{name:'Submit job',exact:true}).click();
  }
  for(;;){
   conflict(await job().count()<=1,'Duplicate acceptance job');
   if((await job().innerText().catch(()=>'' )).includes('DONE'))break;
   if(Date.now()>deadline)throw new BootstrapError('Acceptance job not DONE');
   await page.waitForTimeout(2000);await page.reload();
  }
  const jobId=await job().locator('td').first().textContent();
  if(rt.state.jobId)conflict(rt.state.jobId===jobId,'Job/data identity after upgrade');
  rt.state.jobId=jobId;rt.state.jobIntent=false;rt.save();
  if(config.verification.storage?.enabled)await verifyBackendStorage(page,config,rt);
  await page.screenshot({path:join(config.runtime.reportDir,`app-${Date.now()}.png`),fullPage:true});
  rt.record('business-checks','REUSED',{checks:CHECKS,jobId,url});
  if(config.runtime.recordVideo)await page.waitForTimeout(1000);
  status='SUCCEEDED';
 } finally {await evidence.close(status);}
}

function equalArrays(a,b){return a.length===b.length&&a.every((v,i)=>v===b[i]);}

export function backendClaim(deployment) {
 const main=deployment.spec.template.spec.containers.find(c=>c.name==='main');
 requireConfig(main?.env?.some(e=>e.name==='ACCEPTANCE_STORAGE_DIR'&&e.value==='/data'),'Backend storage directory is not enabled');
 const mount=main.volumeMounts?.find(m=>m.mountPath==='/data'&&!m.readOnly);
 const volume=deployment.spec.template.spec.volumes?.find(v=>v.name===mount?.name);
 requireConfig(volume?.persistentVolumeClaim?.claimName,'Backend /data has no managed PVC');return volume.persistentVolumeClaim.claimName;
}
export async function verifyBackendStorage(page,config,rt) {
 const namespace=`app-${rt.state.appKey}-${config.application.environment}`;
 const claim=backendClaim(kubectl(config,'-n',namespace,'get','deployment','backend'));
 const pvc=kubectl(config,'-n',namespace,'get','pvc',claim);
 requireConfig(pvc.status.phase==='Bound'&&pvc.metadata.labels?.['app.kubernetes.io/managed-by']==='orchestrator','Backend PVC must be owned and Bound');
 const hash=digest(config.verification.storage.value),identity={namespace,name:claim,uid:pvc.metadata.uid,hash};
 if(rt.state.backendStorage)conflict(equal(rt.state.backendStorage,identity),'Backend PVC/file identity changed');
 const stored=page.locator('#storage-hash');
 if(!await stored.count()) {
  requireConfig(!rt.state.storageWriteIntent,'Unknown storage write result; inspect before retry');
  rt.state.storageWriteIntent=true;rt.save();
  await page.getByLabel('Test value',{exact:true}).fill(config.verification.storage.value);
  await page.getByRole('button',{name:'Write to volume',exact:true}).click();
 }
 conflict(await page.locator('#storage-hash').textContent()===hash&&await page.locator('#storage-value').textContent()===config.verification.storage.value,'Backend file value/hash');
 rt.state.backendStorage=identity;rt.state.storageWriteIntent=false;rt.save();rt.record('backend-pvc-file','REUSED',identity);
}
