import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadBootstrapConfig } from './bootstrap-config.mjs';
import { workloadAction, desiredScores } from './workloads.mjs';
import { ensureApplication } from './application.mjs';
import { ensureCatalog } from './catalog-ui.mjs';
import { assertStore } from './platform.mjs';
import { createRuntime } from './runtime.mjs';
import { hasPending } from './deployment.mjs';
function fixture() {
 const dir=mkdtempSync(join(tmpdir(),'bootstrap-30-'));
 const raw=JSON.parse(readFileSync(new URL('../targets/dev.json',import.meta.url)));
 raw.platform.delivery.namespaceAccess.rulesFile=new URL('../applications/acceptance/fleet-access.json',import.meta.url).pathname;
 const privateFile=join(dir,'credential');writeFileSync(privateFile,'private-fixture-only',{mode:0o600});
 raw.console.passwordFile=raw.developer.passwordFile=raw.platform.gitAccess.tokenFile=raw.platform.account.kubeconfigFile=raw.platform.secretStore.tokenFile=raw.application.secretFiles.ACCEPTANCE_SECRET=raw.runtime.clusterKubeconfigFile=privateFile;
 for(const w of ['backend','worker','frontend'])raw.application.scores[w]=new URL(`../../../e2e/scenarios/acceptance-app/scores/${w}.yaml`,import.meta.url).pathname;
 raw.runtime.stateDir=join(dir,'state');raw.runtime.reportDir=join(dir,'reports');
 const file=join(dir,'config.json');const load=()=>{writeFileSync(file,JSON.stringify(raw));return loadBootstrapConfig(file);};
 return {dir,raw,load,close:()=>rmSync(dir,{recursive:true,force:true})};
}
test('complete file-only config validates, resolves files and defaults upgrade on',()=>{
 const f=fixture();try{delete f.raw.upgrade.enabled;const c=f.load();assert.equal(c.upgrade.enabled,true);assert.equal(c.environment,'dev');assert.equal(c.developer.baseURL,c.console.baseURL);assert.equal(desiredScores(c,c.application).backend.metadata.class,'pvc');const frontend=desiredScores(c,c.application).frontend;assert.equal(frontend.resources.svc_backend_http.params.workload,'backend');assert.equal(frontend.containers.main.variables.BACKEND_URL,'${resources.svc_backend_http.url}');}finally{f.close();}
});
test('reject unknown inputs and unpinned images without echoing credentials',()=>{
 const f=fixture();try{f.raw.application.images.backend='private-fixture-only:latest';assert.throws(f.load,e=>!e.message.includes('private-fixture-only'));f.raw.application.images.backend='image:v1';f.raw.upgrade.enabled='false';assert.throws(f.load);f.raw.upgrade.enabled=false;f.raw.misspelled=true;assert.throws(f.load,/unknown/);}finally{f.close();}
});
test('all environment files are explicit and isolated, without embedded credentials',()=>{
 for(const e of ['dev','staging','production']){const c=JSON.parse(readFileSync(new URL(`../targets/${e}.json`,import.meta.url)));assert.equal(c.environment,e);assert.equal(c.upgrade.enabled,true);assert.ok(c.runtime.stateDir.includes('/'+e+'/'));assert.ok(c.console.passwordFile);assert.equal(c.console.password,undefined);}
});
test('matching workload reuses, baseline may upgrade, foreign/pending deletion is blocked',()=>{
 const baseline={metadata:{name:'backend'},containers:{main:{image:'a:v1'}}},target={...baseline,containers:{main:{image:'a:v2'}}};
 assert.equal(workloadAction(undefined,target),'CREATE');assert.equal(workloadAction({workload:target},target),'REUSE');assert.equal(workloadAction({workload:baseline},target,baseline),'UPDATE');assert.throws(()=>workloadAction({workload:{x:1}},target,baseline),/BLOCKED/);assert.throws(()=>workloadAction({workload:target,state:'PENDING_DELETE'},target),/BLOCKED/);
});
test('server matching Application needs explicit identity receipt, not just same name',async()=>{
 const app={key:'id',name:'acceptance',environments:[{key:'staging'}]};
 const config={application:{name:'acceptance',environment:'staging'},runtime:{timeoutMs:1000}};
 const rt={state:{},save(){},record(){}};
 const ui={get:async p=>p==='/applications'?{applications:[app]}:p.endsWith('/operations')?{operations:[]}:{application:app}};
 await assert.rejects(()=>ensureApplication(ui,config,rt),/ownership/);
 rt.state.appKey='id';assert.equal((await ensureApplication(ui,config,rt)).key,'id');
});
test('resume an unknown create only after intent and exact server identity match',async()=>{
 const app={key:'id',name:'acceptance',environments:[{key:'staging'}]};
 const config={application:{name:'acceptance',environment:'staging'},runtime:{timeoutMs:1000}};
 const rt={state:{applicationIntent:true},save(){},record(){}};
 const ui={get:async p=>p==='/applications'?{applications:[app]}:p.endsWith('/operations')?{}:{application:app}};
 await ensureApplication(ui,config,rt);assert.equal(rt.state.appKey,'id');assert.equal(rt.state.applicationIntent,false);
 app.status='DELETING';await assert.rejects(()=>ensureApplication(ui,config,rt),/differs/);
});
test('store reuse verifies target and workload Kubernetes auth, not just READY/name',()=>{
 const want={name:'vault',backendAddress:'http://vault:8200',workloadAddress:'http://vault:8200',mount:'kv',authMount:'kubernetes'};
 const store={...want,status:'READY',verification:{verified:true,kubernetesAuth:'CONFIGURED'}};
 assertStore(store,want);assert.throws(()=>assertStore({...store,workloadAddress:'http://foreign'},want));assert.throws(()=>assertStore({...store,verification:{verified:true}},want));
});
test('unchanged Catalog skips refresh mutation when exact requested releases exist',async()=>{
 const source={id:'catalog',repository:'https://git.example.org/catalog',branch:'main',manifestPath:'catalog.yaml',tagPattern:'v*',includePrereleases:false,accountBindings:{internal:'account'}};
 const ui={source,requiredTags:['v1.0.0','v1.1.0'],baseURL:'http://console',page:{goto:async()=>{}},get:async p=>p==='/driver-accounts'?{accounts:[{name:'account',key:'account'}]}:p.endsWith('/releases')?{releases:[{tag:'v1.0.0'},{tag:'v1.1.0'}]}:{sources:[{...source,code:'READY'}]}};
 await ensureCatalog(ui);
 ui.source={...source,branch:'other'};await assert.rejects(()=>ensureCatalog(ui),/khác cấu hình/);
});
test('pending detector includes Catalog/delivery-only upgrades; empty preview is no-op',()=>{
 assert.equal(hasPending({changes:[]}),false);for(const flag of ['catalogPending','deliveryPending','deliveryPolicyOnly','routePending'])assert.equal(hasPending({changes:[],[flag]:true}),true);
});
test('runtime receipt survives restart, prevents simultaneous writers and cross-target reuse',()=>{
 const f=fixture();try{const c=f.load(),rt=createRuntime(c);rt.state.appKey='app-id';rt.save();assert.throws(()=>createRuntime(c),/locked/);rt.close('FAILED');const resumed=createRuntime(c);assert.equal(resumed.state.appKey,'app-id');resumed.close('SUCCEEDED');c.environment='production';assert.throws(()=>createRuntime(c),/another target/);}finally{f.close();}
});
test('namespace policy rejects cluster privileges and wildcard grants',()=>{
 const f=fixture();try{const policy=join(f.dir,'rules.json');f.raw.platform.delivery.namespaceAccess.rulesFile=policy;writeFileSync(policy,JSON.stringify({rules:[{apiGroups:[''],resources:['namespaces'],verbs:['patch']}]}));assert.throws(f.load,/bounded namespaced/);writeFileSync(policy,JSON.stringify({rules:[{apiGroups:['*'],resources:['*'],verbs:['*']}]}));assert.throws(f.load,/bounded namespaced/);}finally{f.close();}
});
test('read-only modes remain file-selected and cannot switch Console through developer URL',()=>{
 const f=fixture();try{for(const mode of ['check','verify']){f.raw.runtime.mode=mode;assert.equal(f.load().runtime.mode,mode);}f.raw.developer.url='https://other.example';assert.throws(f.load,/same target/);}finally{f.close();}
});
