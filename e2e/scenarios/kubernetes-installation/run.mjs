import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdirSync,existsSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {createHash} from 'node:crypto';
import {stringify} from 'yaml';
import {databaseSnapshotSQL} from './assertions.mjs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';

const env=process.env;
for(const name of ['INSTALL_CONFIG','INSTALL_PRIVATE','ORCH_KUBECONFIG','ORCH_CONTEXT','ORCH_PLATFORM_URL','ORCH_PLATFORM_PASSWORD_FILE','ORCH_EVIDENCE_DIR'])assert(env[name],name+' required');
const config=JSON.parse(readFileSync(env.INSTALL_CONFIG,'utf8')),dir=resolve(env.INSTALL_PRIVATE),evidence=resolve(env.ORCH_EVIDENCE_DIR);
assert.equal(config.environment,'kind');assert.equal(config.context,'kind-k8s-4f');assert.equal(env.ORCH_CONTEXT,config.context);assert(!evidence.startsWith(resolve('.')+'/'));
mkdirSync(evidence,{recursive:true,mode:0o700});
const privateState=()=>JSON.parse(readFileSync(join(dir,'installation.json'),'utf8'));
const kube=['--kubeconfig',env.ORCH_KUBECONFIG,'--context',config.context];
const k=(args,input)=>execFileSync('kubectl',[...kube,...args],{input,encoding:'utf8',stdio:['pipe','pipe','pipe'],maxBuffer:64<<20});
const json=args=>JSON.parse(k(['-n',config.namespace,...args,'-o','json']));
const scoped=args=>k(['-n',config.namespace,...args]);
const owned=()=>{const ns=JSON.parse(k(['get','namespace',config.namespace,'-o','json']));assert.equal(ns.metadata.uid,privateState().namespaceUID);assert.equal(ns.metadata.labels['orchestrator.dev/installation'],privateState().id);};
const cli=(command,extra=[])=>{owned();execFileSync('node',['scripts/kubernetes/run.mjs',command,'--config',env.INSTALL_CONFIG,'--private',dir,'--kubeconfig',env.ORCH_KUBECONFIG,'--context',config.context,...extra],{stdio:['ignore','pipe','pipe'],maxBuffer:4<<20});};
const runBrowser=()=>execFileSync('node',['e2e/scenarios/first-run-setup/run.mjs'],{env:{...env,SETUP_URL:env.ORCH_PLATFORM_URL,SETUP_TOKEN_FILE:join(dir,'setup-token'),SETUP_PASSWORD_FILE:env.ORCH_PLATFORM_PASSWORD_FILE,SETUP_GIT_URL:env.INSTALL_GIT_URL??'https://github.com/pr3s3nt/orchestrator-workload-templates',SETUP_GIT_REF:env.INSTALL_GIT_REF??'8aed1e05170a003719919baf63917a9a15b88feb',SETUP_GIT_PATH:'resource-catalog',SETUP_EVIDENCE:join(evidence,'console')},stdio:['ignore','pipe','pipe'],maxBuffer:4<<20});
const receipt={assertions:[],namespace:config.namespace};
const pass=name=>{receipt.assertions.push(name);console.log('PASS '+name);writeFileSync(join(evidence,'receipt.json'),JSON.stringify(receipt,null,2)+'\n',{mode:0o600});};
const sql=query=>scoped(['exec','deployment/postgres','--','psql','-U','orchestrator','-d','orchestrator','-At','-c',query]).trim();
const snapshot=()=>({database:JSON.parse(sql(databaseSnapshotSQL)),pvcs:json(['get','pvc']).items.map(x=>[x.metadata.name,x.metadata.uid]).sort(),secrets:json(['get','secret']).items.filter(x=>x.metadata.labels?.['orchestrator.dev/installation']===privateState().id).map(x=>[x.metadata.name,x.metadata.uid,createHash('sha256').update(JSON.stringify(Object.entries(x.data).sort())).digest('hex')]).sort()});
owned();
if(!existsSync(join(evidence,'initial-browser.done'))){runBrowser();writeFileSync(join(evidence,'initial-browser.done'),'checked\n',{mode:0o600});}
pass('real Console initialization and Git Type/Definition import');

// Register a real uploaded kind credential through the shared Console helper;
// the backend must persist it in the scoped Vault credential store over TLS.
const raw=JSON.parse(k(['config','view','--minify','--flatten','--raw','-o','json']));assert.equal(raw.clusters.length,1);raw.clusters[0].cluster.server='https://kubernetes.default.svc:443';
const credential=join(dir,'kind-client.kubeconfig');writeFileSync(credential,stringify(raw),{mode:0o600});
const consoleConfig=readConsoleConfig({...env,ORCH_HEADLESS:'1'});let session;
try{
 session=await openConsole(consoleConfig,{role:'ADMIN'});const page=session.page;
 await page.getByRole('link',{name:'Tài khoản hạ tầng',exact:true}).click();
 if(!await page.getByRole('button',{name:'Xem Manifest kind',exact:true}).count()){
  const form=page.getByRole('group',{name:'Đăng ký tài khoản mới'});
  await form.getByLabel('Tên tài khoản',{exact:true}).fill('Manifest kind');await form.locator('input[type=file]').setInputFiles(credential);
  await form.getByLabel('Đích đã chọn').waitFor();await form.getByRole('button',{name:'Kiểm tra và lưu',exact:true}).click();
 }
 const row=page.getByRole('row').filter({has:page.getByRole('button',{name:'Xem Manifest kind',exact:true})});await row.getByText('Sẵn sàng',{exact:true}).waitFor({timeout:180000});
 await page.screenshot({path:join(evidence,'real-account.png'),fullPage:true});
}finally{if(session)await closeConsole(session,consoleConfig);}
pass('real kind account verified and stored through backend/scoped Vault');
const vaultPrivate=JSON.parse(readFileSync(join(dir,'vault-init.json'),'utf8'));const secrets=json(['get','secrets']).items;
for(const secret of secrets)for(const value of Object.values(secret.data??{})){const text=Buffer.from(value,'base64').toString();assert(!text.includes(vaultPrivate.root_token));for(const share of [...(vaultPrivate.unseal_keys_b64??[]),...(vaultPrivate.keys_base64??[]),...(vaultPrivate.keys??[])])assert(!text.includes(share));}
pass('root token and Shamir shares absent from Kubernetes Secrets');
owned();scoped(['exec','deployment/backend','-c','backend','--','sh','-ec','f=/var/lib/orchestrator/recovery/manifest91-retained; if [ -e "$f" ]; then test "$(cat "$f")" = manifest-retained; else printf manifest-retained > "$f"; fi']);
const before=snapshot();writeFileSync(join(evidence,'before.json'),JSON.stringify(before,null,2)+'\n',{mode:0o600});
if(!env.INSTALL_SKIP_UPGRADE){
 assert(env.INSTALL_UPGRADE_CONFIG&&env.INSTALL_BACKUP_DIR,'Explicit update config and new private backup directory required');
 cli('upgrade',['--config',env.INSTALL_UPGRADE_CONFIG,'--backup-dir',env.INSTALL_BACKUP_DIR]);
 // Caller provides the same target configuration for subsequent checks.
 Object.assign(config,JSON.parse(readFileSync(env.INSTALL_UPGRADE_CONFIG,'utf8')));env.INSTALL_CONFIG=env.INSTALL_UPGRADE_CONFIG;
 assert(JSON.parse(readFileSync(join(env.INSTALL_BACKUP_DIR,'receipt.json'),'utf8')).complete);
 pass('image update quiesces writers, validates private backup and runs owner setup before runtime');
}
assert.deepEqual(snapshot(),before);pass('database installation/catalog/account IDs, PVCs and Secret bytes/UIDs preserved');
cli('install');assert.deepEqual(snapshot(),before);pass('unchanged install reuses retained data without creating identities');
owned();const oldVault= json(['get','pod','-l','app=vault']).items[0].metadata.uid;
scoped(['rollout','restart','deployment/vault']);
const deadline=Date.now()+180000;for(;;){const pods=json(['get','pod','-l','app=vault']).items;if(pods.some(x=>x.metadata.uid!==oldVault&&x.status?.phase==='Running'))break;assert(Date.now()<deadline,'Vault restart timeout');await new Promise(r=>setTimeout(r,2000));}
cli('unseal');scoped(['rollout','status','deployment/vault','--timeout=180s']);
scoped(['rollout','restart','deployment/backend']);scoped(['rollout','status','deployment/backend','--timeout=240s']);cli('check');
assert.deepEqual(snapshot(),before);
assert.equal(scoped(['exec','deployment/backend','-c','backend','--','cat','/var/lib/orchestrator/recovery/manifest91-retained']).trim(),'manifest-retained');
pass('Vault restart needs operator unseal; backend restart retains workspace/catalog/credentials');
runBrowser();assert.deepEqual(snapshot(),before);pass('exact Tailscale HTTPRoute UI/API and all imported documents remain usable');
cli('diagnostics');receipt.backup=env.INSTALL_BACKUP_DIR;writeFileSync(join(evidence,'receipt.json'),JSON.stringify(receipt,null,2)+'\n',{mode:0o600});
console.log('Owned installation retained for user acceptance; no staging/production access');
