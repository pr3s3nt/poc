import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve, join } from 'node:path';

const evidence=resolve(process.env.ORCH_EVIDENCE_DIR??'');
assert(process.env.ORCH_EVIDENCE_DIR && !evidence.startsWith(resolve('.')+'/'));
const receiptFile=join(evidence,'receipt.json');
const receipt=JSON.parse(readFileSync(receiptFile,'utf8'));
assert.equal(receipt.report?.status,'SUCCEEDED');
const kubeconfig=process.env.ORCH_SCORE_KUBECONFIG;
assert(kubeconfig && process.env.ORCH_SCORE_CONTEXT==='kind-k8s-4f');
const args=['--kubeconfig',kubeconfig,'--context',process.env.ORCH_SCORE_CONTEXT,'--request-timeout=30s'];
const run=more=>execFileSync('kubectl',[...args,...more],{encoding:'utf8',maxBuffer:8*1024*1024,stdio:['ignore','pipe','pipe']});
const get=(kind,name,namespace)=>JSON.parse(run([...(namespace?['-n',namespace]:[]),'get',kind,...(name?[name]:[]),'-o','json']));
const action=process.env.ORCH_ROUTE_VERIFY_ACTION??'verify';
const save=()=>writeFileSync(receiptFile,JSON.stringify(receipt,null,2),{mode:0o600});
const gatewayIdentity=()=>get('gateways.gateway.networking.k8s.io',process.env.ORCH_GATEWAY_NAME,process.env.ORCH_GATEWAY_NAMESPACE).metadata.uid;
if(action==='cleanup-check'){
 assert(receipt.deletionSucceeded,'Application deletion must succeed through product UI');
 assert.equal(run(['get','namespace',receipt.namespace,'--ignore-not-found','-o','name']).trim(),'');
 assert.equal(gatewayIdentity(),receipt.identities.gateway,'Shared Gateway identity changed');
 const volumes=get('pv').items.filter(v=>v.spec.claimRef?.namespace===receipt.namespace);
 assert.equal(volumes.length,0,'Application PV remains after cleanup');
 receipt.cleanupVerified=true;save();console.log('PASS: application namespace/PV absent and shared Gateway preserved');
 process.exit(0);
}
const namespace=get('namespace',receipt.namespace);
assert.equal(namespace.metadata.uid,receipt.namespaceAccess.namespaceUid);
assert.equal(namespace.metadata.labels['app.kubernetes.io/managed-by'],'orchestrator');
if(action==='cleanup-rbac'){
 assert(receipt.workloadsRemoved,'Remove workloads through Environment Deploy before removing delivery access');
 assert.equal(get('httproutes.gateway.networking.k8s.io',undefined,receipt.namespace).items.length,0);
 assert.equal(get('deployments',undefined,receipt.namespace).items.length,0);
 for(const [kind,plural] of [['rolebinding','rolebindings'],['role','roles']]){
  const object=get(kind,'bootstrap-fleet-access',receipt.namespace);
  assert.equal(object.metadata.uid,receipt.namespaceAccess.rbacIdentities[kind]);
  assert.equal(object.metadata.labels['bootstrap.orchestrator.dev/application'],receipt.appId);
  execFileSync('kubectl',[...args,'delete','--raw',`/apis/rbac.authorization.k8s.io/v1/namespaces/${receipt.namespace}/${plural}/bootstrap-fleet-access`,'-f','-'],{input:JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:object.metadata.uid,resourceVersion:object.metadata.resourceVersion}}),stdio:['pipe','pipe','pipe']});
 }
 receipt.setupRemoved=true;save();console.log('PASS: only receipt-owned namespace access removed with UID preconditions');process.exit(0);
}
assert(['verify','restart'].includes(action),'Unknown native verification action');
if(action==='restart'){
 assert(receipt.nativeVerified && receipt.deployedUpdate,'Verify successful update before restart');
 const deployment=get('deployment','native-web',receipt.namespace);
 assert.equal(deployment.metadata.uid,receipt.identities.deployments['native-web']);
 const selector=Object.entries(deployment.spec.selector.matchLabels).map(([k,v])=>k+'='+v).join(',');
 const pods=JSON.parse(run(['-n',receipt.namespace,'get','pods','-l',selector,'-o','json'])).items.filter(p=>!p.metadata.deletionTimestamp);
 assert.equal(pods.length,1);const old=pods[0];
 assert.equal(old.metadata.labels['app.kubernetes.io/name'],'native-web');
 assert.equal(old.metadata.labels['orchestrator.io/application'],receipt.appId);
 assert.equal(old.metadata.labels['orchestrator.io/environment'],'staging');
 const owner=old.metadata.ownerReferences.find(o=>o.controller&&o.kind==='ReplicaSet');assert(owner);
 const replicaSet=get('replicaset',owner.name,receipt.namespace);assert.equal(replicaSet.metadata.uid,owner.uid);
 assert(replicaSet.metadata.ownerReferences.some(o=>o.controller&&o.kind==='Deployment'&&o.uid===deployment.metadata.uid));
 execFileSync('kubectl',[...args,'delete','--raw',`/api/v1/namespaces/${receipt.namespace}/pods/${old.metadata.name}`,'-f','-'],{input:JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:old.metadata.uid,resourceVersion:old.metadata.resourceVersion}}),stdio:['pipe','pipe','pipe']});
 const deadline=Date.now()+180000;let replacement;
 for(;;){
  replacement=JSON.parse(run(['-n',receipt.namespace,'get','pods','-l',selector,'-o','json'])).items.find(p=>p.metadata.uid!==old.metadata.uid&&!p.metadata.deletionTimestamp&&p.status.conditions?.some(c=>c.type==='Ready'&&c.status==='True'));
  if(replacement)break;assert(Date.now()<deadline,'Replacement Pod did not become ready');await new Promise(r=>setTimeout(r,2000));
 }
 receipt.restart={before:old.metadata.uid,after:replacement.metadata.uid};save();
}
const sts=get('statefulsets',undefined,receipt.namespace).items.filter(x=>x.metadata.labels['app.kubernetes.io/name']==='postgres');
assert.equal(sts.length,1,'Both workloads must share exactly one PostgreSQL');
assert.equal(sts[0].status.readyReplicas,1);
const pvcs=get('pvc',undefined,receipt.namespace).items;
assert.equal(pvcs.length,1);assert.equal(pvcs[0].status.phase,'Bound');
const gateway=get('gateways.gateway.networking.k8s.io',process.env.ORCH_GATEWAY_NAME,process.env.ORCH_GATEWAY_NAMESPACE);
const current={namespace:namespace.metadata.uid,postgres:sts[0].metadata.uid,pvc:pvcs[0].metadata.uid,gateway:gateway.metadata.uid,deployments:{},routes:{}};
const routes=get('httproutes.gateway.networking.k8s.io',undefined,receipt.namespace).items;
assert.equal(routes.length,2);
for(const [workload,url] of [['native-web',process.env.ORCH_NATIVE_APP_URL],['score-web',process.env.ORCH_SCORE_APP_URL]]){
 const deployment=get('deployment',workload,receipt.namespace);
 assert.equal(deployment.status.availableReplicas,1);
 if(receipt.deployedUpdate)assert.equal(deployment.spec.template.spec.containers.find(c=>c.name==='main').env.find(e=>e.name==='ACCEPTANCE_REVISION').value,'second');
 current.deployments[workload]=deployment.metadata.uid;
 const selector=Object.entries(deployment.spec.selector.matchLabels).map(([k,v])=>k+'='+v).join(',');
 const pods=JSON.parse(run(['-n',receipt.namespace,'get','pods','-l',selector,'-o','json'])).items.filter(p=>!p.metadata.deletionTimestamp);
 assert.equal(pods.length,1);
 assert.equal(pods[0].status.initContainerStatuses.find(x=>x.name==='dbcheck').state.terminated.exitCode,0);
 assert(run(['-n',receipt.namespace,'logs',pods[0].metadata.name,'-c','dbcheck']).includes('httproute84-database-ready'));
 const route=routes.find(x=>x.metadata.labels['orchestrator.io/workload']===workload);
 assert(route,'Route ownership missing');
 current.routes[workload]=route.metadata.uid;
 assert.deepEqual(route.spec.hostnames,[new URL(url).hostname]);
 assert.equal(route.spec.parentRefs[0].name,process.env.ORCH_GATEWAY_NAME);
 assert.equal(route.spec.parentRefs[0].namespace,process.env.ORCH_GATEWAY_NAMESPACE);
 assert.equal(route.spec.parentRefs[0].sectionName,process.env.ORCH_GATEWAY_LISTENER);
 assert.equal(route.spec.rules[0].matches[0].path.value,'/');
 assert.equal(route.spec.rules[0].backendRefs[0].name,workload);
 assert.equal(route.spec.rules[0].backendRefs[0].port,8080);
 assert(route.status.parents.some(p=>['Accepted','ResolvedRefs'].every(type=>p.conditions.some(c=>c.type===type&&c.status==='True'))));
}
const sql=run(['-n',receipt.namespace,'exec','statefulset/'+sts[0].metadata.name,'--','/bin/sh','-c','PGPASSWORD="$POSTGRES_PASSWORD" psql -h 127.0.0.1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -v ON_ERROR_STOP=1 -Atc "SELECT 1"']);
assert.equal(sql.trim(),'1','Authenticated database query failed');
if(receipt.identities)assert.deepEqual(current,receipt.identities,'Update replaced a preserved database/PVC/Gateway/workload/route identity');
else receipt.identities=current;
receipt.nativeVerified=true;
save();
console.log('PASS: inherited PostgreSQL placement, authenticated SQL, init clients, both accepted routes and preserved identities');
