import {execFileSync} from 'node:child_process';
import {test} from 'node:test';
import assert from 'node:assert/strict';
import {mkdtempSync,readFileSync,writeFileSync,mkdirSync,chmodSync,rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {render,readDocuments,serialize,assertNamespace,assertInstallationConfig,sameSecret,privateDirectory,loadConfig,loadOIDC,owner,prepare,desiredSecrets,archiveListingScript,vaultReviewerMount} from '../model.mjs';
const config={environment:'kind',context:'kind-test',namespace:'owned-test',hostname:'test.example.com',scheme:'http',gateway:{name:'gateway',namespace:'infra',listener:'web'},storage:{class:'standard',postgres:'8Gi',vault:'4Gi',backend:'8Gi'},images:{backend:'orchestrator-backend:test',frontend:'orchestrator-frontend:test',postgres:'postgres:16-alpine',vault:'hashicorp/vault:1.21'}};
test('all overlays render without secrets; kind substitutions and least-consumer mounts are explicit',()=>{
 for(const environment of ['kind','staging','production']){
  const docs=render({...config,environment,tls:{}},'test-installation');assert(!docs.some(x=>x.kind==='Secret'));
  assert.equal(docs.filter(x=>x.kind==='PersistentVolumeClaim').length,3);
  const backend=docs.find(x=>x.kind==='Deployment'&&x.metadata.name==='backend').spec.template.spec;
  assert.equal(backend.securityContext?.fsGroup,undefined);
  const privateFiles=backend.initContainers.find(container=>container.name==='private-files');
  assert(privateFiles.command.some(command=>command.includes('prepare-backend-data.sh /backend-data')));
  assert(privateFiles.volumeMounts.some(mount=>mount.name==='data'&&mount.mountPath==='/backend-data'));
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
test('staging accepts mixed image tags and digests while production requires all digests',()=>{
 const dir=mkdtempSync(join(tmpdir(),'manifest-config-'));
 const file=join(dir,'config.json');
 const load=configuration=>{
  writeFileSync(file,JSON.stringify(configuration));
  return loadConfig(file);
 };
 const digest='sha256:'+'a'.repeat(64);
 const staging={...config,environment:'staging',scheme:'https',images:{
  ...config.images,
  backend:'registry.example.com:5000/orchestrator/backend:v1.2.3',
  frontend:'registry.example.com/orchestrator/frontend@'+digest,
 }};
 try{
  assert.deepEqual(load(staging),staging);
  const manifests=render(load(staging));
  for(const name of Object.keys(staging.images)){
   const deployment=manifests.find(document=>document.kind==='Deployment'&&document.metadata.name===name);
   assert.equal(deployment.spec.template.spec.containers[0].image,staging.images[name]);
  }
  assert.throws(()=>load({...staging,scheme:'http'}),/require an HTTPS Gateway listener/);
  assert.throws(()=>load({...staging,hostname:'CHANGE_ME_HOSTNAME'}),/Resolve configuration placeholders/);
  for(const image of ['postgres','postgres@sha256:short','postgres:invalid:tag']){
   assert.throws(()=>load({...staging,images:{...staging.images,postgres:image}}),/explicit tag or SHA-256 digest/);
  }

  const production={...staging,environment:'production'};
  assert.throws(()=>load(production),/Production requires immutable image digests/);
  const pinnedImages=Object.fromEntries(Object.keys(production.images).map(name=>[name,'registry.example.com/'+name+'@'+digest]));
  assert.deepEqual(load({...production,images:pinnedImages}).images,pinnedImages);
 }finally{rmSync(dir,{recursive:true,force:true});}
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

test('optional Vault reviewer has scoped TokenReview access and a Vault-only projected token',()=>{
 const staging={...config,environment:'staging',scheme:'https'};
 const disabled=render(staging);
 assert(!disabled.some(d=>['ServiceAccount','ClusterRole','ClusterRoleBinding'].includes(d.kind)));
 const docs=render({...staging,vaultKubernetesAuth:true},'installation-one');
 const other=render({...staging,vaultKubernetesAuth:true},'installation-two');
 const role=docs.find(d=>d.kind==='ClusterRole');
 const binding=docs.find(d=>d.kind==='ClusterRoleBinding');
 const account=docs.find(d=>d.kind==='ServiceAccount');
 assert.notEqual(role.metadata.name,other.find(d=>d.kind==='ClusterRole').metadata.name);
 assert.deepEqual(role.rules,[{apiGroups:['authentication.k8s.io'],resources:['tokenreviews'],verbs:['create']}]);
 assert.equal(role.metadata.namespace,undefined);
 assert.equal(binding.metadata.namespace,undefined);
 assert.equal(account.metadata.namespace,staging.namespace);
 assert.equal(account.automountServiceAccountToken,false);
 assert.equal(binding.roleRef.name,role.metadata.name);
 assert.deepEqual(binding.subjects,[{kind:'ServiceAccount',name:account.metadata.name,namespace:staging.namespace}]);
 for(const deployment of docs.filter(d=>d.kind==='Deployment')){
  const pod=deployment.spec.template.spec;
  assert.equal(pod.automountServiceAccountToken,false);
  if(deployment.metadata.name==='vault'){
   assert.equal(pod.serviceAccountName,account.metadata.name);
   const mount=pod.containers[0].volumeMounts.find(m=>m.mountPath===vaultReviewerMount);
   assert.equal(mount.readOnly,true);
   const volume=pod.volumes.find(v=>v.name===mount.name);
   assert.equal(volume.projected.sources[0].serviceAccountToken.expirationSeconds,3600);
  }else{
   assert(!pod.serviceAccountName);
   assert(!JSON.stringify(pod).includes(vaultReviewerMount));
  }
  assert(!(pod.initContainers??[]).some(container=>container.volumeMounts.some(m=>m.mountPath===vaultReviewerMount)));
 }
 assert.throws(()=>assertInstallationConfig(staging,{...staging,vaultKubernetesAuth:true}),/Removing managed Vault Kubernetes Auth/);
});

test('Vault UI is explicit and an optional route uses a distinct HTTPS hostname',()=>{
 const staging={...config,environment:'staging',scheme:'https'};
 const uiConfig={...staging,vaultUI:{hostname:'vault.test.example.com'}};
 const vaultHCL=docs=>docs.find(d=>d.data?.['vault.hcl']).data['vault.hcl'];
 assert.match(vaultHCL(render(staging)),/^ui = false$/m);
 const privateUI=render({...staging,vaultUI:{}});
 assert.match(vaultHCL(privateUI),/^ui = true$/m);
 assert.equal(privateUI.filter(d=>d.kind==='HTTPRoute').length,1);
 const routed=render(uiConfig);
 const route=routed.find(d=>d.kind==='HTTPRoute'&&d.metadata.name==='vault-ui');
 assert.deepEqual(route.spec.hostnames,[uiConfig.vaultUI.hostname]);
 assert.deepEqual(route.spec.rules[0].backendRefs,[{name:'vault',port:8200}]);
 assert.deepEqual(routed.find(d=>d.metadata.name==='console'&&d.kind==='HTTPRoute').spec.hostnames,[staging.hostname]);
 assert.throws(()=>assertInstallationConfig(staging,{...staging,vaultUIRouted:true}),/Removing the Vault UI route/);
 const dir=mkdtempSync(join(tmpdir(),'manifest-vault-options-'));
 const file=join(dir,'config.json');
 const load=configuration=>{
  writeFileSync(file,JSON.stringify(configuration));
  return loadConfig(file);
 };
 try{
  assert.deepEqual(load(uiConfig),uiConfig);
  assert.throws(()=>load({...uiConfig,vaultUI:{hostname:staging.hostname}}),/own DNS hostname/);
  assert.throws(()=>load({...uiConfig,tls:{}}),/requires HTTPS at the Gateway and HTTP to Vault/);
  assert.throws(()=>load({...uiConfig,vaultKubernetesAuth:'yes'}),/must be a boolean/);
 }finally{rmSync(dir,{recursive:true,force:true});}
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

test('optional OIDC projects private input only into the backend Pod', () => {
 const dir = mkdtempSync(join(tmpdir(), 'installer-oidc-'));
 const c = {...config, oidc:true, scheme:'https'};
 const file = join(dir, 'oidc.json');
 const input = {
  issuer:'https://sso.example.com/realms/platform', clientId:'orchestrator',
  clientSecret:'fixture-only-client-secret', publicUrl:'https://test.example.com',
  organizationKey:'platform'
 };
 try {
  writeFileSync(file, JSON.stringify(input), {mode:0o600});
  assert.equal(loadOIDC(config, dir), undefined);
  assert.deepEqual(loadOIDC(c, dir), input);
  const manifests = render(c);
  const backend = manifests.find(d => d.kind === 'Deployment' && d.metadata.name === 'backend');
  assert.match(serialize([backend]), /ORCHESTRATOR_OIDC_CONFIG_FILE/);
  assert.match(serialize([backend]), /name: orchestrator-oidc/);
  assert(!serialize([backend]).includes('ORCHESTRATOR_SETUP_TOKEN_FILE'));
  assert(!serialize([backend]).includes('name: orchestrator-setup'));
  assert(serialize(render(config)).includes('ORCHESTRATOR_SETUP_TOKEN_FILE'));
  assert(!serialize(manifests.filter(d => d !== backend)).includes('orchestrator-oidc'));
  assert(!serialize(manifests).includes(input.clientSecret));
  assert(!serialize(render(config)).includes('ORCHESTRATOR_OIDC_CONFIG_FILE'));
  const state = {passwords:{postgres:'pg',owner:'owner',console:'console',state:'state'}, id:'test',stateKey:'key',setupToken:'setup'};
  const secret = desiredSecrets(c, state, {oidc:input}).find(d => d.metadata.name === 'orchestrator-oidc');
  assert.deepEqual(JSON.parse(Buffer.from(secret.data['oidc.json'], 'base64')), input);
  assert.throws(() => loadOIDC({...c, hostname:'another.example.com'}, dir), /Console origin/);
  writeFileSync(file, JSON.stringify({...input, allowHttp:true}));
  assert.throws(() => loadOIDC({...c, environment:'staging'}, dir), /restricted to kind/);
  chmodSync(file, 0o644);
  assert.throws(() => loadOIDC(c, dir), /owner-only/);
 } finally {
  rmSync(dir, {recursive:true, force:true});
 }
});

test('preparation pins the retained key and does not relabel it on rerun',()=>{
 const dir=mkdtempSync(join(tmpdir(),'installer-state-key-'));
 try {
  const c={...config,environment:'staging',stateKeyId:'retained-key-1'};
  const file=join(dir,'retained-state-key');
  writeFileSync(file,'invalid',{mode:0o600});
  assert.throws(()=>prepare(dir,c),/encode exactly 32 bytes/);
  writeFileSync(file,'ab'.repeat(32)+'\n');
  const state=prepare(dir,c);
  assert.equal(state.stateKey,'ab'.repeat(32));
  assert.equal(state.stateKeyId,'retained-key-1');
  assert.throws(()=>prepare(dir,{...c,stateKeyId:'another-key'}),/Cannot change the state key identifier/);
  const backend=render(c).find(d=>d.kind==='Deployment'&&d.metadata.name==='backend');
  assert(backend.spec.template.spec.containers[0].env.some(e=>e.name==='ORCHESTRATOR_TERRAFORM_STATE_KEY_ID'&&e.value==='retained-key-1'));
  assert(!serialize(render(c)).includes(state.stateKey));
 } finally {
  rmSync(dir,{recursive:true,force:true});
 }
});
