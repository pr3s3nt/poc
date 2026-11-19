// Isolated restore rehearsal on task-owned temporary emptyDir Pods. It never
// writes the retained installation or mounts its data PVCs.
import assert from 'node:assert/strict';
import {databaseSnapshotSQL} from './assertions.mjs';
import {execFileSync,spawn} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {randomUUID,createHash} from 'node:crypto';
import {request as httpsRequest} from 'node:https';
for(const n of ['INSTALL_CONFIG','INSTALL_PRIVATE','INSTALL_BACKUP_DIR','ORCH_KUBECONFIG','ORCH_CONTEXT','ORCH_EVIDENCE_DIR'])assert(process.env[n],n+' required');
const env=process.env,c=JSON.parse(readFileSync(env.INSTALL_CONFIG)),dir=resolve(env.INSTALL_PRIVATE),backup=resolve(env.INSTALL_BACKUP_DIR),evidence=resolve(env.ORCH_EVIDENCE_DIR),state=JSON.parse(readFileSync(join(dir,'installation.json')));
assert(c.environment==='kind'&&env.ORCH_CONTEXT==='kind-k8s-4f'&&c.context===env.ORCH_CONTEXT);assert(!evidence.startsWith(resolve('.')+'/'));
const prior=JSON.parse(readFileSync(join(backup,'receipt.json')));assert(prior.complete&&prior.id===state.id&&prior.namespaceUID===state.namespaceUID,'Backup identity mismatch');
const kube=['--kubeconfig',env.ORCH_KUBECONFIG,'--context',c.context,'-n',c.namespace];
function k(args,input){try{return execFileSync('kubectl',[...kube,...args],{input,encoding:'utf8',maxBuffer:64<<20,stdio:['pipe','pipe','pipe']});}catch(error){writeFileSync(join(evidence,'restore-error.private'),Buffer.concat([Buffer.from(error.stdout??''),Buffer.from(error.stderr??'')]),{mode:0o600});throw new Error('Restore command failed; inspect private diagnostics');}}
const json=(args,input)=>JSON.parse(k([...args,'-o','json'],input));
const ns=json(['get','ns',c.namespace]);assert(ns.metadata.uid===state.namespaceUID&&ns.metadata.labels['orchestrator.dev/installation']===state.id);
const label={'orchestrator.dev/installation':state.id,'app.kubernetes.io/managed-by':'orchestrator-installer','orchestrator.dev/purpose':'restore-rehearsal'};
const created=[];const forwards=[];let result={postgres:false,vault:false,backendArchive:false,cleanup:false};
async function forward(name,service=false){
 const child=spawn('kubectl',[...kube,'port-forward','--address=127.0.0.1',(service?'service/':'pod/')+name,':8200'],{stdio:['ignore','pipe','pipe']});forwards.push(child);
 const port=await new Promise((r,j)=>{const t=setTimeout(()=>j(Error('Restore port-forward timeout')),120000);child.stdout.on('data',b=>{const m=/127\.0\.0\.1:(\d+)/.exec(String(b));if(m){clearTimeout(t);r(Number(m[1]));}});child.stderr.on('data',()=>{});child.on('exit',()=>j(Error('Restore port-forward stopped')));});
 return (path,{body,token,allow=[]}={})=>new Promise((r,j)=>{const data=body?JSON.stringify(body):undefined;const req=httpsRequest({hostname:'127.0.0.1',port,servername:'vault',ca:readFileSync(state.tls.vault.ca),path:'/v1/'+path,method:body?'POST':'GET',headers:{...(token?{'X-Vault-Token':token}:{}),...(data?{'Content-Type':'application/json','Content-Length':Buffer.byteLength(data)}:{})}},res=>{let text='';res.on('data',b=>text+=b);res.on('end',()=>{if((res.statusCode<200||res.statusCode>=300)&&!allow.includes(res.statusCode))return j(Error('Restore Vault API rejected operation: '+path+' HTTP '+res.statusCode));try{r(text?JSON.parse(text):{})}catch{j(Error('Restore Vault API invalid response'))}})});req.setTimeout(15000,()=>req.destroy());req.on('error',()=>j(Error('Restore Vault transport failed')));if(data)req.write(data);req.end();});
}
try{
 const name='restore-pg-'+randomUUID().slice(0,8);
 const pod={apiVersion:'v1',kind:'Pod',metadata:{name,namespace:c.namespace,labels:label},spec:{automountServiceAccountToken:false,restartPolicy:'Never',terminationGracePeriodSeconds:0,containers:[{name:'postgres',image:c.images.postgres,env:[{name:'POSTGRES_USER',value:'orchestrator'},{name:'POSTGRES_DB',value:'orchestrator'},{name:'POSTGRES_PASSWORD',value:'isolated-fixture-only'}],readinessProbe:{exec:{command:['pg_isready','-U','orchestrator','-d','orchestrator']},periodSeconds:2},volumeMounts:[{name:'data',mountPath:'/var/lib/postgresql/data'}]}],volumes:[{name:'data',emptyDir:{}}]}};
 created.push(json(['create','-f','-'],JSON.stringify(pod)));k(['wait','--for=condition=Ready','pod/'+name,'--timeout=180s']);
 k(['exec','-i',name,'--','pg_restore','-U','orchestrator','-d','orchestrator','--no-owner','--no-privileges','--exit-on-error'],readFileSync(join(backup,'database.dump')));
 const identity=k(['exec',name,'--','psql','-U','orchestrator','-d','orchestrator','-At','-c',databaseSnapshotSQL]);
 const restored=JSON.parse(identity),before=JSON.parse(readFileSync(join(evidence,'before.json'))).database;
 assert.deepEqual(restored,before,'Restored database must retain the exact pre-backup identities/catalog');assert.equal(restored.types.length,10);assert.equal(restored.definitions.length,8);result.postgres=true;console.log('PASS isolated PostgreSQL archive restore preserves installation/domain data');
 const marker=execFileSync('tar',['-xOf',join(backup,'backend-data.tgz'),'./recovery/manifest91-retained'],{encoding:'utf8',stdio:['ignore','pipe','pipe']});assert.equal(marker,'manifest-retained');result.backendArchive=true;
 const deployment=json(['get','deployment','vault']);const spec=structuredClone(deployment.spec.template.spec);spec.volumes.find(v=>v.name==='data').emptyDir={};delete spec.volumes.find(v=>v.name==='data').persistentVolumeClaim;
 assert(!spec.volumes.some(v=>v.persistentVolumeClaim),'Restore cannot mount retained PVC');
 spec.restartPolicy='Never';spec.terminationGracePeriodSeconds=0;
 spec.containers[0].command=['sh','-ec','while [ ! -f /tmp/restore-ready ]; do sleep 1; done; exec vault server -config=/config/vault.hcl'];
 const vaultName='restore-vault-'+randomUUID().slice(0,8);created.push(json(['create','-f','-'],JSON.stringify({apiVersion:'v1',kind:'Pod',metadata:{name:vaultName,namespace:c.namespace,labels:label},spec})));
 const deadline=Date.now()+180000;for(;;){const p=json(['get','pod',vaultName]);if(p.status?.containerStatuses?.some(x=>x.name==='vault'&&x.state?.running))break;assert(Date.now()<deadline,'Restore Vault process timeout');await new Promise(r=>setTimeout(r,1000));}
 k(['exec','-i',vaultName,'-c','vault','--','tar','-xzf','-','-C','/vault/data'],readFileSync(join(backup,'vault-data.tgz')));k(['exec',vaultName,'-c','vault','--','touch','/tmp/restore-ready']);
 // A running container can still be waiting for Vault's TLS listener. Starting
 // port-forward earlier can terminate it on the first refused connection.
 for(;;){try{k(['exec',vaultName,'-c','vault','--','sh','-ec','set +e; vault status >/dev/null 2>&1; code=$?; [ "$code" = 0 ] || [ "$code" = 2 ]']);break;}catch{assert(Date.now()<deadline,'Restore Vault listener timeout');await new Promise(r=>setTimeout(r,1000));}}
 const call=await forward(vaultName);let health;
 for(;;){try{health=await call('sys/health',{allow:[501,503]});break;}catch{assert(Date.now()<deadline,'Restore Vault reachability timeout');await new Promise(r=>setTimeout(r,1000));}}
 assert(health.initialized&&health.sealed,'Restored Vault must retain initialized seal');
 const recovery=JSON.parse(readFileSync(join(backup,'vault-init.json')));for(const key of recovery.keys??recovery.unseal_keys_b64){if(!health.sealed)break;await call('sys/unseal',{body:{key}});health=await call('sys/health',{allow:[503]});}assert(!health.sealed,'Matching external shares must unseal restored data');
 // Canonical scoped policies deliberately omit lookup-self. Inspect policy with
 // operator-held authorization, then read data using only the scoped token.
 const lookup=await call('auth/token/lookup',{token:recovery.root_token,body:{token:state.vaultTokens['connection-token']}});assert.deepEqual(lookup.data.policies,['orchestrator-connections']);
 const ref=k(['exec','deployment/postgres','--','psql','-U','orchestrator','-d','orchestrator','-At','-c',"SELECT secret_ref FROM connections WHERE kind='KUBERNETES' LIMIT 1"]).trim();const parsed=new URL(ref);assert(parsed.protocol==='kv2:'&&parsed.hostname==='kv'&&parsed.pathname.startsWith('/orchestrator/connections/'));
 const stored=await call('kv/data'+parsed.pathname,{token:state.vaultTokens['connection-token']});const sourceCall=await forward('vault',true);const source=await sourceCall('kv/data'+parsed.pathname,{token:state.vaultTokens['connection-token']});
 const hash=x=>createHash('sha256').update(x.data.data.value).digest('hex');assert(hash(stored)===hash(source),'Restored credential differs');result.vault=true;console.log('PASS isolated Vault restore unseals with external shares and retains exact scoped credential');
}finally{
 if(!result.vault){for(const made of created.filter(p=>p.metadata.name.startsWith('restore-vault-'))){try{writeFileSync(join(evidence,'restore-vault.private.log'),k(['logs',made.metadata.name,'-c','vault']),{mode:0o600});}catch{}}}
 for(const p of forwards)p.kill('SIGTERM');
 for(const made of created.reverse()){
  const live=json(['get','pod',made.metadata.name]);assert(live.metadata.uid===made.metadata.uid&&live.metadata.labels['orchestrator.dev/installation']===state.id&&live.metadata.labels['orchestrator.dev/purpose']==='restore-rehearsal','Restore cleanup ownership mismatch');
  k(['delete','--raw',`/api/v1/namespaces/${c.namespace}/pods/${made.metadata.name}`,'-f','-'],JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:live.metadata.uid,resourceVersion:live.metadata.resourceVersion}}));k(['wait','--for=delete','pod/'+made.metadata.name,'--timeout=180s']);
 }
 result.cleanup=true;writeFileSync(join(evidence,'restore-receipt.json'),JSON.stringify(result,null,2)+'\n',{mode:0o600});console.log('Temporary restore Pods cleanup verified; retained installation untouched');
}
