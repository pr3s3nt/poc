import { openWorkloadEvidence } from '../../../scripts/bootstrap/lib/video.mjs';
import { parseArgs } from 'node:util';
import { execFileSync, spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync, unlinkSync } from 'node:fs';
import { randomUUID } from 'node:crypto';
import { dirname, resolve, join } from 'node:path';
import { loadBootstrapConfig, requireConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { createRuntime, conflict } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { kubectl, backendClaim, verifyBackendStorage } from '../../../scripts/bootstrap/lib/verification.mjs';
import { openConsole, closeConsole } from '../../lib/console.mjs';
const {values}=parseArgs({options:{config:{type:'string'}}});requireConfig(values.config,'--config required');
const file=resolve(values.config),config=loadBootstrapConfig(file);
requireConfig(config.environment==='dev'&&config.gitOnly&&config.verification.storage?.enabled,'Requires reviewed dev Git-only input with backend storage enabled');
// Nested setup/idempotency runs do not retain intermediate videos. Keep path
// resolution unchanged by placing the temporary private input beside the original.
const childConfig=join(dirname(file),`.git-pvc-no-video-${randomUUID()}.json`);
const raw=JSON.parse(readFileSync(file,'utf8'));raw.runtime.recordVideo=false;
writeFileSync(childConfig,JSON.stringify(raw),{mode:0o600,flag:'wx'});
const run=path=>{const result=spawnSync(process.execPath,[new URL(path,import.meta.url).pathname,'--config',childConfig],{stdio:'inherit'});requireConfig(result.status===0,'Bootstrap/idempotency failed; inspect private report before retry');};
let evidence,session,rt,runtimeClosed=false,status='FAILED';
try {
 run('../bootstrap-acceptance/run.mjs');
 rt=createRuntime(config);
 session=await openConsole(config.console);
 evidence=await openWorkloadEvidence(session.browser,config,'backend-pvc-retention');const {page}=evidence;
 const namespace=`app-${rt.state.appKey}-${config.application.environment}`;
 const namespaceObject=kubectl(config,'get','namespace',namespace);
 conflict(namespaceObject.metadata.uid===rt.state.namespaceUid&&namespaceObject.metadata.labels['orchestrator-managed']==='true','Namespace ownership/UID');
 const deployment=kubectl(config,'-n',namespace,'get','deployment','backend');
 const claim=backendClaim(deployment),pvc=kubectl(config,'-n',namespace,'get','pvc',claim);
 conflict(pvc.metadata.uid===rt.state.backendStorage.uid&&pvc.metadata.labels['app.kubernetes.io/managed-by']==='orchestrator','PVC ownership/UID');
 await page.goto(config.verification.url);await verifyBackendStorage(page,config,rt);
 const pods=kubectl(config,'-n',namespace,'get','pods','-l','app.kubernetes.io/name=backend').items.filter(p=>!p.metadata.deletionTimestamp);
 requireConfig(pods.length===1,'Expected one backend Pod');const pod=pods[0];
 conflict(pod.metadata.labels['orchestrator.io/application']===rt.state.appKey,'Backend Pod Application owner');
 const owner=pod.metadata.ownerReferences?.find(o=>o.controller&&o.kind==='ReplicaSet');requireConfig(owner,'Missing Pod controller');
 const rs=kubectl(config,'-n',namespace,'get','replicaset',owner.name);
 conflict(rs.metadata.uid===owner.uid&&rs.metadata.ownerReferences?.some(o=>o.controller&&o.uid===deployment.metadata.uid),'Pod controller UID chain');
 const before={podName:pod.metadata.name,podUid:pod.metadata.uid,deploymentUid:deployment.metadata.uid,pvcUid:pvc.metadata.uid};
 execFileSync('kubectl',['--kubeconfig',config.runtime.clusterKubeconfigFile,'--context',config.platform.account.context,'delete','--raw',`/api/v1/namespaces/${namespace}/pods/${pod.metadata.name}`,'-f','-'],{input:JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:pod.metadata.uid}}),encoding:'utf8',timeout:30000,stdio:['pipe','pipe','pipe']});
 const deadline=Date.now()+180000;let replacement;
 for(;;) {
  const current=kubectl(config,'-n',namespace,'get','pods','-l','app.kubernetes.io/name=backend').items;
  replacement=current.find(p=>p.metadata.uid!==pod.metadata.uid&&!p.metadata.deletionTimestamp&&p.status.conditions?.some(c=>c.type==='Ready'&&c.status==='True'));
  if(replacement){await page.goto(config.verification.url);if(await page.locator('#storage-process').textContent()===replacement.metadata.name)break;}
  requireConfig(Date.now()<deadline,'Backend replacement did not become ready');await page.waitForTimeout(2000);
 }
 conflict(kubectl(config,'-n',namespace,'get','pvc',claim).metadata.uid===pvc.metadata.uid,'PVC UID after Pod recreation');
 conflict(kubectl(config,'-n',namespace,'get','deployment','backend').metadata.uid===deployment.metadata.uid,'Deployment UID after Pod recreation');
 await verifyBackendStorage(page,config,rt);await page.screenshot({path:join(config.runtime.reportDir,'backend-pvc-retained.png'),fullPage:true});
 const result={status:'PASS',applicationKey:rt.state.appKey,namespace,before,replacement:{podName:replacement.metadata.name,podUid:replacement.metadata.uid},fileHash:rt.state.backendStorage.hash,gitFiles:config.gitInputReceipts};
 writeFileSync(join(config.runtime.reportDir,'backend-pvc-retention.json'),JSON.stringify(result,null,2),{mode:0o600});
 console.log('PASS: backend file/hash and PVC UID retained after ownership-checked Pod recreation');
 if(config.runtime.recordVideo)await page.waitForTimeout(1000);
 // Release the state lock, then verify the final unchanged bootstrap too. The
 // video is published only if this whole scenario, including that rerun, passes.
 rt.close('SUCCEEDED');runtimeClosed=true;
 run('../../../scripts/bootstrap/run.mjs');
 status='SUCCEEDED';
}finally{
 try{if(evidence)await evidence.close(status);}
 finally{try{if(session)await closeConsole(session,config.console);}
 finally{try{if(rt&&!runtimeClosed)rt.close(status);}finally{unlinkSync(childConfig);}}}
}
