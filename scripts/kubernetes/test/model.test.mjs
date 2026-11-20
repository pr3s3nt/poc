import {execFileSync} from 'node:child_process';
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,writeFileSync,mkdirSync,chmodSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {render,readDocuments,serialize,assertNamespace,sameSecret,privateDirectory,loadConfig,owner,prepare,desiredSecrets,archiveListingScript} from '../model.mjs';
const config={environment:'kind',context:'kind-test',namespace:'owned-test',hostname:'test.example.com',scheme:'http',gateway:{name:'gateway',namespace:'infra',listener:'web'},storage:{class:'standard',postgres:'8Gi',vault:'4Gi',backend:'8Gi'},images:{backend:'orchestrator-backend:test',frontend:'orchestrator-frontend:test',postgres:'postgres:16-alpine',vault:'hashicorp/vault:1.21'}};
test('all overlays render without secrets; kind substitutions and least-consumer mounts are explicit',()=>{
 for(const environment of ['kind','staging','production']){
  const docs=render({...config,environment,tls:{}},'test-installation');assert(!docs.some(x=>x.kind==='Secret'));
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

test('new staging without internal TLS renders compatible services, clients and secrets',()=>{
 const dir=mkdtempSync(join(tmpdir(),'manifest-staging-'));
 try {
  const staging={...config,environment:'staging',scheme:'https',gateway:{...config.gateway,listener:'websecure'}};
  const state=prepare(dir,staging);
  assert.equal(state.tls,undefined);
  assert.deepEqual(prepare(dir,staging),state);
  assert.throws(()=>prepare(dir,{...staging,tls:{}}),/Cannot change internal TLS mode/);

  const docs=render(staging,state.id);
  const postgres=docs.find(d=>d.kind==='Deployment'&&d.metadata.name==='postgres').spec.template.spec;
  assert.deepEqual(postgres.containers[0].args,['-c','ssl=off']);
  const vault=docs.find(d=>d.kind==='Deployment'&&d.metadata.name==='vault').spec.template.spec;
  assert.deepEqual(vault.containers[0].env,[{name:'VAULT_ADDR',value:'http://127.0.0.1:8200'}]);
  const vaultConfig=docs.find(d=>d.kind==='ConfigMap'&&d.data['vault.hcl']).data['vault.hcl'];
  assert.match(vaultConfig,/tls_disable = true/);
  assert.match(vaultConfig,/api_addr = "http:\/\/vault:8200"/);
  assert.match(vaultConfig,/storage "file"/);

  const backend=docs.find(d=>d.kind==='Deployment'&&d.metadata.name==='backend').spec.template.spec;
  for(const container of backend.containers){
   const endpoints=container.env.filter(entry=>entry.name.endsWith('VAULT_ADDR'));
   assert(endpoints.length>0);
   assert(endpoints.every(entry=>entry.value==='http://vault:8200'));
  }
  const route=docs.find(d=>d.kind==='HTTPRoute');
  assert.equal(route.spec.parentRefs[0].sectionName,'websecure');
  assert.doesNotMatch(serialize(docs),/tls_(cert|key)_file|postgres-tls|vault-tls|orchestrator-trust|ssl=on|SSL_CERT_FILE|VAULT_CACERT/);
  assert(!postgres.containers[0].volumeMounts.some(mount=>mount.subPath==='postgres-require-tls.sh'));
  for(const document of docs.filter(d=>d.kind==='Deployment'||d.kind==='Job')){
   const pod=document.spec.template.spec;
   const names=new Set(pod.volumes.map(volume=>volume.name));
   for(const container of [...pod.containers,...pod.initContainers??[]]){
    assert((container.volumeMounts??[]).every(mount=>names.has(mount.name)));
   }
  }

  const secrets=desiredSecrets(staging,state,{vault:true});
  assert(!secrets.some(secret=>['postgres-tls','vault-tls','orchestrator-trust'].includes(secret.metadata.name)));
  assert(secrets.some(secret=>secret.metadata.name==='orchestrator-vault-runtime'));
  for(const name of ['orchestrator-owner','orchestrator-runtime']){
   const urls=Object.values(secrets.find(secret=>secret.metadata.name===name).data);
   assert(urls.every(value=>new URL(Buffer.from(value,'base64')).searchParams.get('sslmode')==='disable'));
  }
 }finally{rmSync(dir,{recursive:true,force:true});}
});

test('production and staging with an explicit TLS object require certificate paths',()=>{
 for(const configOverride of [{environment:'production'},{environment:'staging',tls:{}}]){
  const dir=mkdtempSync(join(tmpdir(),'manifest-missing-tls-'));
  try{assert.throws(()=>prepare(dir,{...config,...configOverride}),/Supply PostgreSQL\/Vault TLS file paths/);}
  finally{rmSync(dir,{recursive:true,force:true});}
 }
});

test('private preparation is reusable for one target, independent across targets and validates matching TLS keys',()=>{
 const first=mkdtempSync(join(tmpdir(),'manifest-prepare-')),second=mkdtempSync(join(tmpdir(),'manifest-prepare-')),bad=mkdtempSync(join(tmpdir(),'manifest-prepare-'));
 try{
  const a=prepare(first,config,{testCertificates:true});const reused=prepare(first,config,{testCertificates:true});assert.deepEqual(a,reused);assert.throws(()=>prepare(first,{...config,namespace:'another'}));
  const b=prepare(second,{...config,namespace:'another'},{testCertificates:true});assert.notEqual(a.id,b.id);assert.notEqual(a.stateKey,b.stateKey);assert.notEqual(a.passwords.postgres,b.passwords.postgres);assert.notEqual(a.vaultTokens['connection-token'],b.vaultTokens['connection-token']);
  const secrets=desiredSecrets(config,a,{vault:true});assert(!JSON.stringify(secrets).includes('root_token'));assert(secrets.every(x=>x.metadata.labels['orchestrator.dev/installation']===a.id));
  const staging={...config,environment:'staging',scheme:'https',tls:a.tls};
  const stagingState=prepare(join(first,'staging'),staging);
  assert.throws(()=>prepare(join(first,'staging'),{...staging,tls:undefined}),/Cannot change internal TLS mode/);
  const stagingSecrets=desiredSecrets(staging,stagingState);
  assert(stagingSecrets.some(secret=>secret.metadata.name==='orchestrator-trust'));
  const ownerUrl=Buffer.from(stagingSecrets.find(secret=>secret.metadata.name==='orchestrator-owner').data['database-url'],'base64');
  assert.equal(new URL(ownerUrl).searchParams.get('sslmode'),'verify-full');
  assert.throws(()=>prepare(bad,{...config,tls:{...a.tls,vault:{...a.tls.vault,key:a.tls.postgres.key}}}),/do not match/);
 }finally{for(const d of [first,second,bad])rmSync(d,{recursive:true,force:true});}
});

test('archive listing drains a large stdin after early TOC completion and preserves failures',()=>{
 const dir=mkdtempSync(join(tmpdir(),'manifest-archive-'));
 try{
  const fake=join(dir,'pg_restore');const run=()=>execFileSync('sh',['-c',archiveListingScript],{env:{...process.env,PATH:dir+':'+process.env.PATH},input:Buffer.alloc(16<<20),encoding:'utf8',timeout:10000,stdio:['pipe','pipe','pipe']});
  writeFileSync(fake,'#!/bin/sh\nprintf "archive listing\\n"\nexit 0\n',{mode:0o700});assert.equal(run(),'archive listing\n');
  writeFileSync(fake,'#!/bin/sh\nexit 7\n');assert.throws(run,e=>e.status===7&&!e.code);
 }finally{rmSync(dir,{recursive:true,force:true});}
});
