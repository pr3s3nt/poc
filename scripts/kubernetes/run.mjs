#!/usr/bin/env node
import { execFileSync, spawn } from 'node:child_process';
import { readFileSync, writeFileSync, existsSync, mkdirSync, openSync, closeSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { request as httpsRequest } from 'node:https';
import { randomUUID } from 'node:crypto';
import { loadConfig, privateDirectory, privateRead, savePrivate, prepare, render, serialize, desiredSecrets, sameSecret, assertNamespace, requireThat, digest, root, owner } from './model.mjs';

const args=process.argv.slice(2),command=args.shift();
const options={};for(let i=0;i<args.length;i++){requireThat(args[i].startsWith('--'),'Unknown argument');const key=args[i].slice(2);options[key]=args[i+1]&&!args[i+1].startsWith('--')?args[++i]:true;}
const commands=['render','prepare','check','dependencies','install','vault-init','unseal','upgrade','backup','diagnostics'];
try{
 requireThat(commands.includes(command),'Use render|prepare|check|install|vault-init|unseal|upgrade|backup|diagnostics');
 requireThat(options.config&&options.private,'Explicit --config and --private required');
 const c=loadConfig(resolve(options.config),{complete:command!=='render'}),dir=privateDirectory(options.private);
 const statePath=join(dir,'installation.json');
 let state;
 if(command==='prepare') {state=prepare(dir,c,{testCertificates:options['test-certificates']===true});console.log('Private installation inputs prepared; setup token is in the private directory');process.exit(0);}
 requireThat(existsSync(statePath),'Run prepare first');state=JSON.parse(privateRead(statePath));
 requireThat(state.context===c.context&&state.namespace===c.namespace&&state.environment===c.environment,'Private inputs belong to another target');
 const docs=render(c,state.id);
 if(command==='render'){requireThat(options.output,'Use --output to write rendered nonsecret manifests');const out=resolve(options.output);requireThat(out.startsWith(dir+'/'),'Render output must be in private directory');savePrivate(out,serialize(docs));console.log('Nonsecret manifests rendered');process.exit(0);}
 requireThat(options.context===c.context&&typeof options.kubeconfig==='string','Explicit --kubeconfig and matching --context required');
 const kube=['--kubeconfig',resolve(options.kubeconfig),'--context',c.context,'--request-timeout=60s'];
 const k=(a,input,{binary=false}={})=>{
  try{return execFileSync('kubectl',[...kube,...a],{input,encoding:binary?undefined:'utf8',maxBuffer:512<<20,stdio:['pipe','pipe','pipe']});}
  catch{throw new Error('Kubernetes command failed ('+a.slice(0,3).join(' ')+'); raw diagnostics withheld');}
 };
 const json=(a,input)=>JSON.parse(k(a,input));
 const scoped=(a,input,opt)=>k(['-n',c.namespace,...a],input,opt);
 const streamFile=(a,file,inputFile)=>{const output=openSync(file,'wx',0o600);const input=inputFile?openSync(inputFile,'r'):undefined;try{execFileSync('kubectl',[...kube,'-n',c.namespace,...a],{stdio:[input??'ignore',output,'pipe'],maxBuffer:1<<20});}catch{throw new Error('Private backup stream failed; retained partial output requires operator review');}finally{closeSync(output);if(input!==undefined)closeSync(input);}};
 const get=(kind,name)=>{const raw=k(['-n',c.namespace,'get',kind,name,'--ignore-not-found','-o','json']);return raw.trim()?JSON.parse(raw):undefined;};
 const getNamespace=()=>{const raw=k(['get','namespace',c.namespace,'--ignore-not-found','-o','json']);return raw.trim()?JSON.parse(raw):undefined;};
 const persist=()=>savePrivate(statePath,JSON.stringify(state,null,2)+'\n');
 const ownership=()=>{const ns=getNamespace();requireThat(ns,'Installation namespace does not exist');assertNamespace(ns,c,state);return ns;};
 const apply=objects=>{for(const object of objects) {
  const existing=object.kind==='Namespace'?getNamespace():get(object.kind,object.metadata.name);
  if(existing)requireThat(existing.metadata.labels?.['orchestrator.dev/installation']===state.id,'Resource is not owned by this installation');
  k(['apply','-f','-'],JSON.stringify(object));
 }};
 const phase=name=>docs.filter(d=>d.metadata.labels?.['orchestrator.dev/phase']===name);
 const secrets=(includeVault=false,readOnly=false)=>{for(const d of desiredSecrets(c,state,{vault:includeVault})){
  const old=get('secret',d.metadata.name);
  if(old){requireThat(old.metadata.labels?.['orchestrator.dev/installation']===state.id&&sameSecret(old,d),'Existing Secret conflicts; nothing rotated');}
  else {requireThat(!readOnly,'Required Secret missing; check never creates credentials');k(['create','-f','-'],JSON.stringify(d));}
 }};
 const rollout=name=>scoped(['rollout','status','deployment/'+name,'--timeout=240s']);
 const waitPodsDeleted=name=>scoped(['wait','--for=delete','pod','-l','app='+name,'--timeout=180s']);
 const scale=(name,count)=>{const d=get('deployment',name);requireThat(d&&d.metadata.labels?.['orchestrator.dev/installation']===state.id,'Missing owned deployment '+name);scoped(['scale','deployment/'+name,'--replicas='+count]);};
 async function vaultForward(){
  // Running is earlier than the TLS listener. A refused first connection kills
  // kubectl port-forward, so wait locally before opening the operator tunnel.
  const deadline=Date.now()+180000;
  for(;;){try{scoped(['exec','deployment/vault','-c','vault','--','sh','-ec','set +e; vault status >/dev/null 2>&1; code=$?; [ "$code" = 0 ] || [ "$code" = 2 ]']);break;}catch{requireThat(Date.now()<deadline,'Vault TLS listener did not become reachable');await new Promise(r=>setTimeout(r,1000));}}
  const child=spawn('kubectl',[...kube,'-n',c.namespace,'port-forward','--address=127.0.0.1','service/vault',':8200','--pod-running-timeout=120s'],{stdio:['ignore','pipe','pipe']});
  let timeout;const port=await new Promise((resolve,reject)=>{
   timeout=setTimeout(()=>reject(new Error('Vault port-forward did not become ready')),130000);
   child.stdout.on('data',chunk=>{const m=/127\.0\.0\.1:(\d+)/.exec(String(chunk));if(m)resolve(Number(m[1]));});
   child.stderr.on('data',()=>{});child.on('exit',()=>reject(new Error('Vault port-forward stopped')));child.on('error',()=>reject(new Error('Vault port-forward failed')));
  }).catch(err=>{child.kill();throw err;}).finally(()=>clearTimeout(timeout));
  return {close:()=>child.kill('SIGTERM'),call:(path,{method='GET',body,token,allow=[]}={})=>new Promise((resolve,reject)=>{
   const data=body===undefined?undefined:JSON.stringify(body);
   const req=httpsRequest({hostname:'127.0.0.1',port,servername:'vault',ca:readFileSync(state.tls.vault.ca),path:'/v1/'+path,method,headers:{...(token?{'X-Vault-Token':token}:{}),...(data?{'Content-Type':'application/json','Content-Length':Buffer.byteLength(data)}:{})}},res=>{
    let text='';res.on('data',chunk=>{text+=chunk;if(text.length>2<<20)req.destroy()});res.on('end',()=>{if((res.statusCode<200||res.statusCode>=300)&&!allow.includes(res.statusCode))return reject(new Error('Vault request rejected ('+path+', HTTP '+res.statusCode+')'));try{resolve({status:res.statusCode,value:text?JSON.parse(text):{}})}catch{reject(new Error('Invalid Vault response'))}});
   });req.setTimeout(20000,()=>req.destroy());req.on('error',()=>reject(new Error('Vault transport failed; no automatic replay')));if(data)req.write(data);req.end();
  })};
 }
 async function vaultState(client){return (await client.call('sys/health',{allow:[429,472,473,501,503]})).value;}
 async function unseal(client){
  const file=options['shares-file']?resolve(options['shares-file']):join(dir,'vault-init.json');
  const saved=JSON.parse(privateRead(file)),keys=saved.unseal_keys_b64??saved.keys;
  requireThat(Array.isArray(keys)&&keys.length>0,'No private unseal shares supplied');
  let health=await vaultState(client);requireThat(health.initialized,'Vault is not initialized');
  for(const key of keys){if(!health.sealed)break;await client.call('sys/unseal',{method:'POST',body:{key}});health=await vaultState(client);}
  requireThat(!health.sealed,'Insufficient matching unseal shares; operator action required');
 }
 async function configureVault(client,rootToken){
  const response=(await client.call('sys/mounts',{token:rootToken})).value;const mounts=response.data??response;
  if(!mounts['kv/'])await client.call('sys/mounts/kv',{method:'POST',token:rootToken,body:{type:'kv',options:{version:'2'}}});
  else requireThat(mounts['kv/'].type==='kv'&&mounts['kv/'].options?.version==='2','Existing kv mount conflicts');
  for(const [policy,file]of [['orchestrator-connections','connections-policy.hcl'],['orchestrator-applications','applications-policy.hcl'],['orchestrator-outputs','outputs-policy.hcl']]){
   const text=readFileSync(join(root,'deploy/local',file),'utf8');const old=await client.call('sys/policies/acl/'+policy,{token:rootToken,allow:[404]});
   if(old.status===404)await client.call('sys/policies/acl/'+policy,{method:'PUT',token:rootToken,body:{policy:text}});
   else requireThat(old.value.data?.policy?.trim()===text.trim(),'Existing Vault policy differs; review before changing');
  }
  for(const [name,policy]of [['connection-token','orchestrator-connections'],['output-token','orchestrator-outputs'],['app-token','orchestrator-applications']]){
   const value=state.vaultTokens[name];const lookup=await client.call('auth/token/lookup',{method:'POST',token:rootToken,body:{token:value},allow:[400,403,404]});
   if(lookup.status===200)requireThat(JSON.stringify(lookup.value.data.policies.sort())===JSON.stringify([policy]),'Scoped token policy differs');
   else{
    requireThat(!get('secret','orchestrator-vault-runtime'),'Existing runtime Vault credential is no longer valid; no silent rotation');
    await client.call('auth/token/create-orphan',{method:'POST',token:rootToken,body:{id:value,policies:[policy],no_default_policy:true,period:'24h',renewable:true}});
   }
  }
  secrets(true);savePrivate(join(dir,'application-vault-token'),state.vaultTokens['app-token']+'\n');
 }
 async function initializeVault(){
  ownership();const client=await vaultForward();
  try{
   let health=await vaultState(client);const file=join(dir,'vault-init.json');
   if(!health.initialized){
    requireThat(!existsSync(join(dir,'vault-init-intent'))&&!existsSync(file),'Previous initialization outcome requires recovery, never reset Vault');
    savePrivate(join(dir,'vault-init-intent'),'Owner initialization requested\n',true);
    const result=await client.call('sys/init',{method:'POST',body:{secret_shares:5,secret_threshold:3}});
    savePrivate(file,JSON.stringify(result.value,null,2)+'\n',true);
   }
   await unseal(client);const saved=JSON.parse(privateRead(file));requireThat(saved.root_token,'Private owner root authorization required');
   await configureVault(client,saved.root_token);console.log('Vault initialized/unsealed; shares/root authorization remain outside the cluster');
  }finally{client.close();}
 }
 async function readyVault(){const client=await vaultForward();try{const v=await vaultState(client);requireThat(v.initialized&&!v.sealed,'Vault is sealed/uninitialized: run vault-init or unseal explicitly');requireThat(get('secret','orchestrator-vault-runtime'),'Run vault-init to configure scoped runtime tokens');}finally{client.close();}}
 function databaseIdentity(){return scoped(['exec','deployment/postgres','--','psql','-U','orchestrator','-d','orchestrator','-At','-c','SELECT installation_id::text FROM schema_baseline']).trim();}
 function assertDatabase(){requireThat(state.databaseID&&databaseIdentity()===state.databaseID,'Database installation identity changed/missing; restore matching retained data');}
 function runDatabase(initial=false){
  const source=structuredClone(phase('database')[0]);source.metadata.name='database-setup-'+digest(c.images.backend).slice(0,10)+'-'+Date.now().toString(36);
  k(['create','-f','-'],JSON.stringify(source));scoped(['wait','--for=condition=complete','job/'+source.metadata.name,'--timeout=300s']);
  const identity=databaseIdentity();requireThat(/^[0-9a-f-]{36}$/.test(identity),'Missing database installation identity');
  if(state.databaseID)requireThat(state.databaseID===identity,'Owner setup changed installation identity');else{requireThat(initial,'Missing installation receipt');state.databaseID=identity;persist();}
  console.log('Owner database setup completed');
 }
 function checkGateway(){
  const gateway=json(['-n',c.gateway.namespace,'get','gateway',c.gateway.name,'-o','json']);
  requireThat(gateway.status?.conditions?.some(x=>x.type==='Programmed'&&x.status==='True'),'Gateway is not Programmed');
  const listener=gateway.spec.listeners.find(x=>x.name===c.gateway.listener);requireThat(listener,'Gateway listener missing');
  requireThat(listener.protocol===(c.scheme==='https'?'HTTPS':'HTTP'),'Gateway listener scheme differs');
  const permission=listener.allowedRoutes?.namespaces?.from??'Same';
  if(permission==='Same')requireThat(c.gateway.namespace===c.namespace,'Gateway does not allow this namespace');
  if(permission==='Selector'){
   const selector=listener.allowedRoutes.namespaces.selector??{};const labels=getNamespace()?.metadata.labels??docs.find(x=>x.kind==='Namespace').metadata.labels;
   requireThat(Object.entries(selector.matchLabels??{}).every(([k,v])=>labels[k]===v),'Gateway namespace selector does not match');
   for(const e of selector.matchExpressions??[]){const present=Object.hasOwn(labels,e.key),value=labels[e.key];const pass=e.operator==='In'?present&&e.values.includes(value):e.operator==='NotIn'?!present||!e.values.includes(value):e.operator==='Exists'?present:e.operator==='DoesNotExist'?!present:false;requireThat(pass,'Gateway namespace expression does not match');}
  }
 }
 async function check(){
  ownership();for(const name of ['postgres','vault','backend','frontend'])requireThat(get('deployment',name)?.spec.replicas===1,'Expected running single replica for '+name);checkGateway();assertStorage();secrets(true,true);await readyVault();rollout('postgres');assertDatabase();rollout('vault');rollout('backend');rollout('frontend');
  const route=get('httproute','console');requireThat(route?.status?.parents?.some(p=>p.conditions?.some(x=>x.type==='Accepted'&&x.status==='True')&&p.conditions?.some(x=>x.type==='ResolvedRefs'&&x.status==='True')),'HTTPRoute not accepted/resolved');
  console.log('Owned installation, TLS dependencies, rollouts and HTTPRoute checked; browser acceptance is separate');
 }
 async function install(dependenciesOnly=false){
  checkGateway();let ns=getNamespace();
  if(ns)assertNamespace(ns,c,state);else{const desired=docs.find(d=>d.kind==='Namespace');ns=json(['create','-f','-','-o','json'],JSON.stringify(desired));}
  state.namespaceUID=ns.metadata.uid;persist();
  const current=get('deployment','backend');if(current){
   const desired=docs.find(d=>d.kind==='Deployment'&&d.metadata.name==='backend');
   requireThat(current.spec.template.metadata.annotations?.['orchestrator.dev/release']===desired.spec.template.metadata.annotations['orchestrator.dev/release'],'Existing release differs: use upgrade with backup');
   await check();console.log('Existing installation reused without owner/credential changes');return;
  }
  apply(phase('config').filter(d=>d.kind!=='Namespace'));secrets();apply(phase('storage'));apply(phase('dependencies'));rollout('postgres');
  if(dependenciesOnly){const client=await vaultForward();try{await vaultState(client);}finally{client.close();}console.log('PostgreSQL ready and Vault reachable; run vault-init before install');return;}
  await readyVault();rollout('vault');runDatabase(true);apply(phase('application'));await check();console.log('Console installed; use the private setup token in the browser');
 }
 const pods=name=>{const items=json(['-n',c.namespace,'get','pods','-l','app='+name,'-o','json']).items;requireThat(items.every(p=>p.metadata.labels?.['orchestrator.dev/installation']===state.id),'Unexpected Pod ownership');return items;};
 function assertStorage(){for(const name of ['postgres','vault','backend']){const pvc=get('pvc',name+'-data');requireThat(pvc?.metadata.labels?.['orchestrator.dev/installation']===state.id&&pvc.spec.storageClassName===c.storage.class&&pvc.spec.resources.requests.storage===c.storage[name],'Storage changes require a separate reviewed procedure');}}
 function assertIdle(){const count=scoped(['exec','deployment/postgres','--','psql','-U','orchestrator','-d','orchestrator','-At','-c',"SELECT count(*) FROM environment_operations WHERE status IN ('ACTIVE','RECOVERING','INTERRUPTED')"]).trim();requireThat(count==='0','Installation has active/interrupted operations; resolve before upgrade');}
 async function backup(){
  ownership();assertDatabase();assertStorage();assertIdle();requireThat(options['backup-dir'],'Explicit private --backup-dir required');const destination=privateDirectory(options['backup-dir']);
  requireThat(!existsSync(join(destination,'receipt.json')),'Use a new backup directory');
  const backendNode=pods('backend')[0]?.spec.nodeName,vaultNode=pods('vault')[0]?.spec.nodeName;requireThat(backendNode&&vaultNode,'Backup requires running source Pods to capture placement');
  const receipt={namespaceUID:state.namespaceUID,id:state.id,createdAt:new Date().toISOString(),complete:false};savePrivate(join(destination,'receipt.json'),JSON.stringify(receipt,null,2)+'\n',true);
  scale('backend',0);scale('frontend',0);waitPodsDeleted('backend');waitPodsDeleted('frontend');assertIdle();
  streamFile(['exec','deployment/postgres','--','sh','-ec','PGPASSWORD=$(cat /private/password); export PGPASSWORD; pg_dump -U orchestrator -d orchestrator -Fc'],join(destination,'database.dump'));
  streamFile(['exec','-i','deployment/postgres','--','pg_restore','--list'],join(destination,'database-list.txt'),join(destination,'database.dump'));
  scale('vault',0);waitPodsDeleted('vault');
  for(const [claim,node]of [['backend-data',backendNode],['vault-data',vaultNode]]){
   const name='backup-'+randomUUID().slice(0,8);const object={apiVersion:'v1',kind:'Pod',metadata:{name,namespace:c.namespace,labels:{'app.kubernetes.io/managed-by':owner,'orchestrator.dev/installation':state.id}},spec:{automountServiceAccountToken:false,nodeName:node,terminationGracePeriodSeconds:0,restartPolicy:'Never',containers:[{name:'archive',image:c.images.postgres,command:['sleep','600'],volumeMounts:[{name:'data',mountPath:'/data',readOnly:true}]}],volumes:[{name:'data',persistentVolumeClaim:{claimName:claim,readOnly:true}}]}};
   const created=json(['create','-f','-','-o','json'],JSON.stringify(object));
   try{scoped(['wait','--for=condition=Ready','pod/'+name,'--timeout=180s']);streamFile(['exec',name,'--','tar','-czf','-','-C','/data','.'],join(destination,claim+'.tgz'));try{execFileSync('tar',['-tzf',join(destination,claim+'.tgz')],{stdio:['ignore','ignore','pipe']});}catch{throw new Error('Backup archive validation failed');}}
   finally{const live=get('pod',name);if(live){requireThat(live.metadata.uid===created.metadata.uid&&live.metadata.labels?.['orchestrator.dev/installation']===state.id,'Backup Pod ownership changed');scoped(['delete','--raw',`/api/v1/namespaces/${c.namespace}/pods/${name}`,'-f','-'],JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:live.metadata.uid,resourceVersion:live.metadata.resourceVersion}}));scoped(['wait','--for=delete','pod/'+name,'--timeout=180s']);}}
  }
  savePrivate(join(destination,'installation.json'),privateRead(statePath),true);if(existsSync(join(dir,'vault-init.json')))savePrivate(join(destination,'vault-init.json'),privateRead(join(dir,'vault-init.json')),true);
  for(const service of ['postgres','vault'])for(const type of ['ca','cert','key'])savePrivate(join(destination,service+'-'+type+'.pem'),readFileSync(state.tls[service][type]),true);
  receipt.complete=true;savePrivate(join(destination,'receipt.json'),JSON.stringify(receipt,null,2)+'\n');
  console.log('Private database/data backup archives validated; writers and Vault remain stopped until explicit resume/upgrade');return destination;
 }
 if(command==='check')await check();
 if(command==='dependencies')await install(true);
 if(command==='install')await install();
 if(command==='vault-init')await initializeVault();
 if(command==='unseal'){ownership();const client=await vaultForward();try{await unseal(client);console.log('Vault unsealed by operator');}finally{client.close();}}
 if(command==='backup')await backup();
 if(command==='upgrade'){
  ownership();checkGateway();secrets(true,true);await backup();
  apply(phase('config').filter(d=>d.kind!=='Namespace'));apply(phase('dependencies'));scale('vault',1);rollout('postgres');
  const client=await vaultForward();try{await unseal(client);}finally{client.close();}rollout('vault');
  runDatabase();apply(phase('application'));scale('backend',1);scale('frontend',1);await check();console.log('Upgrade checked; retained volumes and credentials preserved');
 }
 if(command==='diagnostics'){
  ownership();const objects=json(['-n',c.namespace,'get','deploy,pods,pvc,job,httproute','-o','json']);
  const safe=objects.items.map(x=>({kind:x.kind,name:x.metadata.name,uid:x.metadata.uid,phase:x.status?.phase,readyReplicas:x.status?.readyReplicas,conditions:x.status?.conditions?.map(y=>({type:y.type,status:y.status,reason:y.reason})),images:x.spec?.template?.spec?.containers?.map(y=>y.image)}));savePrivate(join(dir,'diagnostics.json'),JSON.stringify(safe,null,2)+'\n');console.log('Allowlisted diagnostics saved privately; no Secret data, logs or kubeconfig included');
 }
}catch(error){console.error(error instanceof SyntaxError?'Invalid JSON input; contents withheld':error instanceof Error?error.message:'Installer failed');process.exitCode=1;}
