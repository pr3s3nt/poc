import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { loadBootstrapConfig } from './bootstrap-config.mjs';
import { treeDigest } from './direct-config.mjs';
import { definitionDocument, ensureDirectDefinitions, ensureTemplateSources, resolveDefinition, sameDefinitionPin, readDefinitionPin, sourceMatches, submitRegistration } from './direct-platform.mjs';
import { assertDirectSelections, acceptanceInputHash } from './direct-runner.mjs';
import { createRuntime } from './runtime.mjs';
function fixture() {
 const dir=mkdtempSync(join(tmpdir(),'bootstrap33-'));
 const raw=JSON.parse(readFileSync(new URL('../direct-targets/dev.json',import.meta.url)));
 const credential=join(dir,'credential');writeFileSync(credential,'private-fixture',{mode:0o600});
 raw.console.passwordFile=raw.developer.passwordFile=raw.platform.gitAccess.tokenFile=raw.platform.account.kubeconfigFile=raw.platform.secretStore.tokenFile=raw.application.secretFiles.ACCEPTANCE_SECRET=raw.runtime.clusterKubeconfigFile=credential;
 raw.platform.delivery.namespaceAccess.rulesFile=new URL('../applications/acceptance/fleet-access.json',import.meta.url).pathname;
 for(const entry of raw.platform.definitions)entry.file=new URL('../direct-targets/'+entry.file,import.meta.url).pathname;
 for(const [name,file]of Object.entries(raw.application.workloads))raw.application.workloads[name]=new URL('../direct-targets/'+file,import.meta.url).pathname;
 raw.runtime.stateDir=join(dir,'state');raw.runtime.reportDir=join(dir,'reports');
 const load=()=>{const file=join(dir,'config.json');writeFileSync(file,JSON.stringify(raw));return loadBootstrapConfig(file);};
 return {raw,load,dir,close(){rmSync(dir,{force:true,recursive:true});}};
}
test('direct full input validates without Catalog or upgrade and retains Generic Workload paths',()=>{
 const f=fixture();try{const c=f.load();assert.equal(c.schemaVersion,3);assert.equal(c.platform.catalog,undefined);assert.equal(c.upgrade.enabled,false);assert.equal(c.application.scores.backend,c.application.workloads.backend);assert.equal(c.platform.definitions.length,5);}finally{f.close();}
});
test('platform-only input has no Application/developer/images/workloads/verification dependency',()=>{
 const f=fixture();try{f.raw.runtime.mode='bootstrap';delete f.raw.application;delete f.raw.developer;delete f.raw.verification;const c=f.load();assert.equal(c.application,undefined);const rt=createRuntime(c);rt.save();rt.close('BOOTSTRAPPED');f.raw.environment='staging';assert.throws(()=>createRuntime(f.load()),/another target/);}finally{f.close();}
});
test('full run rejects absent acceptance input, disabled business verification and unpinned source',()=>{
 const f=fixture();try{f.raw.verification.enabled=false;assert.throws(f.load,/verification/);f.raw.verification.enabled=true;f.raw.platform.templateSources[0].ref='main';assert.throws(f.load,/pinned commit/);f.raw.platform.templateSources[0].ref=f.raw.platform.templateSources[0].pin.revision;delete f.raw.application;assert.throws(f.load,/application/);}finally{f.close();}
});
test('source pin reuse reconciles server after lost result without preparation mutation',async()=>{
 const pin={revision:'a'.repeat(40),contentSha256:'b'.repeat(64)},source={id:'immutable-id',source:{...pin,kind:'git',url:'https://git.example/repo',path:'.'},status:'AVAILABLE'};
 const ui={get:async()=>({templateSources:[source]})},rt={state:{},save(){},record(){}};
 const config={platform:{templateSources:[{name:'test',kind:'git',url:'https://git.example/repo',path:'.',pin}]}};
 assert.equal((await ensureTemplateSources(ui,config,rt)).test.id,source.id);
 assert.equal((await ensureTemplateSources(ui,config,rt)).test.id,source.id);
 source.id='different-engine-identity';await assert.rejects(()=>ensureTemplateSources(ui,config,rt),/missing/);
});
test('Definition comparison blocks any existing conflict before creating a missing entry',async()=>{
 const d={key:'existing',name:'Expected',resourceType:'k8s-cluster',driverType:'existing-cluster',driverInputs:{values:{variables:{}}},criteria:[{}]};
 const config={platform:{definitions:[{definition:{...d,key:'missing'}},{definition:d}]}};
 const ui={get:async()=>({resourceDefinitions:[{...d,name:'Foreign'}]})};
 await assert.rejects(()=>ensureDirectDefinitions(ui,config,{key:'account'},{},{state:{}}),/Definition existing/);
 assert.deepEqual(definitionDocument({...d,version:2,sourceFingerprint:'x'}),definitionDocument(d));
});
test('Definition input resolves only declared account/template identity and preserves resource expressions',()=>{
 const expression="${resources['namespace.default#environments.@app.@env'].outputs.name}";
 const d={key:'workload',name:'Test',resourceType:'workload',driverType:'template',driverAccount:'${account.key}',driverInputs:{values:{variables:{template_source:'${templateSources.test}',namespace:expression,template_inputs:{replicas:0}}}},criteria:[{}]};
 const result=resolveDefinition({definition:d},'account',{test:{id:'source-id'}});
 assert.equal(result.driverAccount,'account');assert.equal(result.driverInputs.values.variables.namespace,expression);assert.equal(result.driverInputs.values.variables.template_inputs.replicas,0);assert.equal(d.driverAccount,'${account.key}');
});
test('tree digest is deterministic and includes file identities/content using product framing',()=>{
 assert.equal(treeDigest({b:'two',a:'one'}),treeDigest({a:'one',b:'two'}));
 assert.notEqual(treeDigest({a:'one'}),treeDigest({b:'one'}));assert.notEqual(treeDigest({a:'one'}),treeDigest({a:'two'}));
});

test('Definition receipt survives JSON serialization when existing-cluster has no source fingerprint',()=>{
 const record=JSON.parse(JSON.stringify({version:1,sourceFingerprint:undefined}));
 assert.equal(sameDefinitionPin(record,{version:1}),true);
 assert.equal(sameDefinitionPin(record,{version:2}),false);
 assert.equal(sameDefinitionPin(record,{version:1,sourceFingerprint:'changed'}),false);
});

test('platform-only can resume into full acceptance using the same environment receipt',()=>{
 const f=fixture();try{
  const full=f.load();f.raw.runtime.mode='bootstrap';delete f.raw.application;delete f.raw.developer;delete f.raw.verification;
  const platform=createRuntime(f.load());platform.state.templateSources={test:{id:'source-id'}};platform.save();platform.close('BOOTSTRAPPED');
  const resumed=createRuntime(full);assert.equal(resumed.state.templateSources.test.id,'source-id');resumed.save();resumed.close('SUCCEEDED');
  full.application.environment='production';assert.throws(()=>createRuntime(full),/another Application/);full.application.environment='staging';
  full.platform.account.context='another-context';assert.throws(()=>createRuntime(full),/another target/);
 }finally{f.close();}
});

test('active Definition revision must match input; promotion/generation changes cannot be silently adopted',async()=>{
 const definition={key:'test',name:'Test',resourceType:'k8s-cluster',driverType:'existing-cluster',driverInputs:{values:{variables:{}}},criteria:[{}],version:1};
 const history={activeId:'r1',generation:1,versions:[{id:'r1',definition,archived:false}]};
 const ui={get:async()=>({history})};
 const pin=await readDefinitionPin(ui,definition);assert.equal(pin.revisionId,'r1');
 assert.equal(sameDefinitionPin(pin,{...pin,generation:2}),false);
 history.versions[0].definition={...definition,name:'Foreign'};
 await assert.rejects(()=>readDefinitionPin(ui,definition),/active revision differs/);
});
test('acceptance validates exact desired/applied Definition IDs and rejects foreign infrastructure',async()=>{
 const row={key:'test',desiredRevisionId:'r1',appliedRevisionId:'r1',actualRevisionIds:['r1'],inUse:true};
 const selections={pending:false,definitions:[row]},ui={get:async()=>selections};
 const config={application:{environment:'staging'}},app={key:'app'},rt={state:{definitionPins:{test:{revisionId:'r1'}}}};
 await assertDirectSelections(ui,config,app,rt,{applied:true});
 row.desiredRevisionId='r2';await assert.rejects(()=>assertDirectSelections(ui,config,app,rt),/desired revision/);row.desiredRevisionId='r1';
 row.actualRevisionIds=['r2'];await assert.rejects(()=>assertDirectSelections(ui,config,app,rt,{applied:true}),/applied revision/);row.actualRevisionIds=['r1'];
 selections.definitions.push({...row,key:'foreign'});await assert.rejects(()=>assertDirectSelections(ui,config,app,rt,{applied:true}),/unconfigured infrastructure/);
});
test('template pin comparison preserves Git origin and directory even for identical content',()=>{
 const pin={revision:'a'.repeat(40),contentSha256:'b'.repeat(64)},spec={kind:'git',url:'https://git.example/owned',path:'templates',pin};
 const actual={...pin,kind:'git',url:spec.url,path:spec.path};assert.equal(sourceMatches(actual,spec),true);
 assert.equal(sourceMatches({...actual,url:'https://git.example/foreign'},spec),false);
 assert.equal(sourceMatches({...actual,path:'foreign'},spec),false);
});

test('acceptance input receipt compares semantic workload/config and secret content before mutation',()=>{
 const f=fixture();try{
  const c=f.load(),initial=acceptanceInputHash(c);
  c.application.variables=Object.fromEntries(Object.entries(c.application.variables).reverse());assert.equal(acceptanceInputHash(c),initial);
  c.application.images.backend='different:v2';assert.notEqual(acceptanceInputHash(c),initial);
  c.application.images.backend=f.raw.application.images.backend;
  writeFileSync(c.application.secretFiles.ACCEPTANCE_SECRET,'changed-private-fixture');assert.notEqual(acceptanceInputHash(c),initial);
 }finally{f.close();}
});

test('lost registration response is not replayed until exact server state confirms completion',async()=>{
 const rt={state:{},save(){}},listeners=new Map();let calls=0;
 const ui={page:{on:(event,fn)=>listeners.set(event,fn),off:event=>listeners.delete(event)},submit:async()=>{calls++;listeners.get('request')({method:()=> 'POST',url:()=> 'https://console/api/v1/template-sources/inspect',postData:()=> 'private-input-sentinel'});throw Error('lost response');}};
 await assert.rejects(()=>submitRegistration(ui,rt,'template:test','/template-sources/inspect',{}),/lost response/);
 assert.equal(calls,1);assert.ok(rt.state.pendingRegistrations['template:test']);assert.ok(!JSON.stringify(rt.state).includes('private-input-sentinel'));
 await assert.rejects(()=>submitRegistration(ui,rt,'template:test','/template-sources/inspect',{}),/Unknown registration/);assert.equal(calls,1);
 const pin={revision:'a',contentSha256:'b'};
 await ensureTemplateSources({get:async()=>({templateSources:[{id:'confirmed',source:pin,status:'AVAILABLE'}]})},{platform:{templateSources:[{name:'test',kind:'installed',pin}]}},{...rt,record(){}});
 assert.equal(rt.state.pendingRegistrations['template:test'],undefined);
});
test('known registration rejection clears only its own intent',async()=>{
 const rt={state:{pendingRegistrations:{other:{path:'/other'}}},save(){}},listeners=new Map();
 const ui={page:{on:(e,fn)=>listeners.set(e,fn),off:e=>listeners.delete(e)},submit:async()=>{listeners.get('request')({method:()=> 'POST',url:()=> 'https://console/api/v1/resource-definitions',postData:()=> '{}'});const error=Error('rejected');error.status=422;throw error;}};
 await assert.rejects(()=>submitRegistration(ui,rt,'definition:test','/resource-definitions',{}),/rejected/);
 assert.equal(rt.state.pendingRegistrations['definition:test'],undefined);assert.ok(rt.state.pendingRegistrations.other);
});

test('environment and Git-owned Definition values expand explicitly without replacing product placeholders',()=>{
 const f=fixture();try{
  f.raw.parameters={env:'company-staging',prefix:'corporate'};f.raw.environment='${bootstrap.env}';
  const first=f.raw.platform.definitions[0];const definition=JSON.parse(readFileSync(first.file,'utf8'));
  definition.key='${bootstrap.prefix}-cluster';const file=join(f.dir,'definition.json');writeFileSync(file,JSON.stringify(definition));first.file=file;
  const c=f.load();assert.equal(c.environment,'company-staging');assert.equal(c.platform.definitions[0].definition.key,'corporate-cluster');
  delete f.raw.parameters.prefix;assert.throws(f.load,/Missing bootstrap parameter/);
 }finally{f.close();}
});
