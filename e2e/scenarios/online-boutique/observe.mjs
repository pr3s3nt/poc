import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
const dir=process.env.ORCH_TEST_RUN_DIR,kube=process.env.ORCH_CLUSTER_KUBECONFIG;assert(dir&&kube);
outsideRepo(dir);
const state=JSON.parse(readFileSync(dir+'/state.json')),fixture=JSON.parse(readFileSync(new URL('./fixtures/workloads.json',import.meta.url)));
const env=state.app.environments.find(e=>e.key==='staging'),ns=env.namespaceIdentity;assert(ns,'Stored namespace identity required');
const get=(kind)=>JSON.parse(execFileSync('kubectl',['--kubeconfig',kube,'--context','kind-k8s-4f','-n',ns,'get',kind,'-o','json'],{encoding:'utf8',timeout:30000}));
const namespace=JSON.parse(execFileSync('kubectl',['--kubeconfig',kube,'get','ns',ns,'-o','json'],{encoding:'utf8'}));assert.equal(namespace.metadata.uid,state.namespaceUid);
const pods=get('pods').items,controllers=get('deployments').items,services=get('services').items,endpoints=get('endpointslices').items;
assert.equal(controllers.length,11);
const evidence=[];
for(const w of fixture.workloads){
 const d=controllers.find(d=>d.metadata.name===w.name);assert(d,w.name);assert.equal(d.status.readyReplicas,1,w.name+' ready');
 const pod=pods.find(p=>p.metadata.labels['app.kubernetes.io/name']===w.name&&!p.metadata.deletionTimestamp);assert(pod,w.name+' pod');
 assert(pod.status.conditions.some(c=>c.type==='Ready'&&c.status==='True'),w.name+' Pod readiness');
 const containers=[...(pod.spec.containers??[]),...(pod.spec.initContainers??[])];assert(containers.length>0);
 for(const c of containers)assert.equal(c.securityContext?.privileged,false,w.name+'/'+c.name+' compatible default');
 const main=pod.spec.containers.find(c=>c.image===w.image);assert(main,w.name+' image');
 for(const [name,value]of Object.entries(w.variables))assert.equal(main.env.find(e=>e.name===name)?.value,value,w.name+'/'+name);
 if(w.redis){const binding=main.env.find(e=>e.name==='REDIS_ADDR');assert(binding.valueFrom?.secretKeyRef,'Redis is a Secret reference');assert(!binding.value,'No Redis literal');}
 evidence.push({workload:w.name,ready:true,restarts:pod.status.containerStatuses.reduce((n,c)=>n+c.restartCount,0),containers:containers.map(c=>({name:c.name,securityContext:c.securityContext})),podSecurityContext:pod.spec.securityContext,automountServiceAccountToken:pod.spec.automountServiceAccountToken});
 if(w.service)for(const port of Object.values(w.service.ports)){const service=services.find(s=>s.metadata.name===w.name);assert(service);assert(service.spec.ports.some(p=>p.port===port.port&&p.targetPort===port.targetPort));assert(endpoints.some(e=>e.metadata.labels['kubernetes.io/service-name']===w.name&&e.endpoints.some(x=>x.conditions.ready===true)),w.name+' ready endpoint');}
}
assert(get('statefulsets').items.some(s=>s.metadata.name.startsWith('redis-')&&s.status.readyReplicas===1),'Redis ready');
assert(get('vaultstaticsecrets').items.some(s=>s.status.conditions.some(c=>c.type==='SecretSynced'&&c.status==='True')),'VSO secret sync');
writeFileSync(dir+'/runtime-security.json',JSON.stringify({namespace:ns,namespaceUid:state.namespaceUid,evidence},null,2),{mode:0o600});
console.log('PASS 11 ready app Pods, Redis, service endpoints, pinned images/env, compatible security and Cart Secret reference');
