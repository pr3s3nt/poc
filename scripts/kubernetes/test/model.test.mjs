import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,writeFileSync,mkdirSync,chmodSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {render,readDocuments,serialize,assertNamespace,sameSecret,privateDirectory,loadConfig,owner,prepare,desiredSecrets} from '../model.mjs';
const config={environment:'kind',context:'kind-test',namespace:'owned-test',hostname:'test.example.com',scheme:'http',gateway:{name:'gateway',namespace:'infra',listener:'web'},storage:{class:'standard',postgres:'8Gi',vault:'4Gi',backend:'8Gi'},images:{backend:'orchestrator-backend:test',frontend:'orchestrator-frontend:test',postgres:'postgres:16-alpine',vault:'hashicorp/vault:1.21'}};
test('all overlays render without secrets; kind substitutions and least-consumer mounts are explicit',()=>{
 for(const environment of ['kind','staging','production']){
  const docs=render({...config,environment},'test-installation');assert(!docs.some(x=>x.kind==='Secret'));
  assert.equal(docs.filter(x=>x.kind==='PersistentVolumeClaim').length,3);
  const backend=docs.find(x=>x.kind==='Deployment'&&x.metadata.name==='backend').spec.template.spec;
  const sources=backend.volumes.find(v=>v.name==='source').projected.sources.map(s=>s.secret.name);
  assert(!sources.includes('orchestrator-owner'));assert(!sources.includes('orchestrator-postgres'));assert(!JSON.stringify(docs).includes('root-token'));
  const renewer=backend.containers.find(c=>c.name==='vault-token-renewer');assert(renewer.volumeMounts.filter(m=>m.name==='private').every(m=>m.subPath));
  const vault=docs.find(x=>x.kind==='Deployment'&&x.metadata.name==='vault');assert.equal(vault.spec.template.spec.containers[0].command[0],'vault');
  assert(!JSON.stringify(vault).includes('unseal'));
  const route=docs.find(x=>x.kind==='HTTPRoute');assert.deepEqual(route.spec.hostnames,[config.hostname]);assert.equal(route.spec.rules[0].backendRefs[0].name,'frontend');
  assert.equal(readDocuments(serialize(docs)).length,docs.length);
 }
});
test('namespace ownership and UID prevent adopting another installation',()=>{
 const state={id:'mine',namespaceUID:'original'};const ns={metadata:{name:config.namespace,uid:'original',labels:{'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':'mine'}}};assert.doesNotThrow(()=>assertNamespace(ns,config,state));
 for(const next of [{...ns,metadata:{...ns.metadata,uid:'new'}},{metadata:{...ns.metadata,labels:{}}}])assert.throws(()=>assertNamespace(next,config,state));
});
test('secret reuse compares complete byte data without allowing silent rotation',()=>{assert(sameSecret({data:{b:'Yg==',a:'YQ=='}},{data:{a:'YQ==',b:'Yg=='}}));assert(!sameSecret({data:{a:'b2xk'}},{data:{a:'bmV3'}}));assert(!sameSecret({data:{a:'YQ==',extra:'eA=='}},{data:{a:'YQ=='}}));});
test('private material is excluded from Git and rejects broad permissions',()=>{
 const dir=mkdtempSync(join(tmpdir(),'manifest-private-'));try{chmodSync(dir,0o700);assert.equal(privateDirectory(dir),dir);chmodSync(dir,0o755);assert.throws(()=>privateDirectory(dir));chmodSync(dir,0o700);mkdirSync(join(dir,'.git'));assert.throws(()=>privateDirectory(dir));}finally{rmSync(dir,{recursive:true,force:true});}
});
test('staging deployment requires completed configuration, HTTPS and digest pins',()=>{
 const dir=mkdtempSync(join(tmpdir(),'manifest-config-'));const file=join(dir,'config.json');try{writeFileSync(file,JSON.stringify({...config,environment:'staging'}));assert.throws(()=>loadConfig(file));const c={...config,environment:'staging',scheme:'https',images:Object.fromEntries(Object.keys(config.images).map(k=>[k,'registry.example.com/'+k+'@sha256:'+'a'.repeat(64)]))};writeFileSync(file,JSON.stringify(c));assert.equal(loadConfig(file).environment,'staging');writeFileSync(file,JSON.stringify({...c,hostname:'CHANGE_ME_HOSTNAME'}));assert.throws(()=>loadConfig(file));}finally{rmSync(dir,{recursive:true,force:true});}
});

test('private preparation is reusable for one target, independent across targets and validates matching TLS keys',()=>{
 const first=mkdtempSync(join(tmpdir(),'manifest-prepare-')),second=mkdtempSync(join(tmpdir(),'manifest-prepare-')),bad=mkdtempSync(join(tmpdir(),'manifest-prepare-'));
 try{
  const a=prepare(first,config,{testCertificates:true});const reused=prepare(first,config,{testCertificates:true});assert.deepEqual(a,reused);assert.throws(()=>prepare(first,{...config,namespace:'another'}));
  const b=prepare(second,{...config,namespace:'another'},{testCertificates:true});assert.notEqual(a.id,b.id);assert.notEqual(a.stateKey,b.stateKey);assert.notEqual(a.passwords.postgres,b.passwords.postgres);assert.notEqual(a.vaultTokens['connection-token'],b.vaultTokens['connection-token']);
  const secrets=desiredSecrets(config,a,{vault:true});assert(!JSON.stringify(secrets).includes('root_token'));assert(secrets.every(x=>x.metadata.labels['orchestrator.dev/installation']===a.id));
  assert.throws(()=>prepare(bad,{...config,tls:{...a.tls,vault:{...a.tls.vault,key:a.tls.postgres.key}}}),/do not match/);
 }finally{for(const d of [first,second,bad])rmSync(d,{recursive:true,force:true});}
});
