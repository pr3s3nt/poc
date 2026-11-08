import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureEnvironment,selectDefinitionRevisions} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureSecretStore,ensureTypes} from '../../../scripts/bootstrap/lib/platform.mjs';
import {ensureDirectDefinitions} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
import {authorGitLabWorkload,updateGitLabStorage} from './gitlab-form.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const file=dir+'/gitlab-state.json',state=existsSync(file)?JSON.parse(readFileSync(file,'utf8')):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2));
const rt={state,save,record(key,status){console.log(key,status);save()}};
const stage=process.argv[2];assert(['inspect','prepare','author','endpoint','attach-endpoint','preview','storage'].includes(stage));
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000),{page}=ui;
try{
 const stores=(await ui.get('/secret-stores')).secretStores;
 if(stage==='inspect')console.log(JSON.stringify(stores.map(v=>({key:v.key,name:v.name,status:v.status,verification:v.verification})),null,2));
 else if(stage==='endpoint'){
  assert(state.prepared);const input=JSON.parse(readFileSync(dir+'/gitlab-input.json','utf8'));assert(input.endpoint);
  await ensureTypes(ui,[{key:'endpoint',inputs:[{name:'name',type:'string',required:true},{name:'port',type:'string',required:true},{name:'path',type:'string',required:false}],outputs:[{name:'hostname',type:'string',required:true,secret:false},{name:'url',type:'string',required:true,secret:false}]}],rt);
  const definition={key:'gitlab-endpoint',name:'GitLab HTTPS endpoint',resourceType:'endpoint',driverType:'endpoint',driverInputs:{values:{variables:{rootDomain:input.endpoint.rootDomain,scheme:'https',gatewayName:input.endpoint.gatewayName,gatewayNamespace:input.endpoint.gatewayNamespace,gatewaySectionName:input.endpoint.gatewaySectionName}}},provision:{},criteria:[{app_id:state.app.key}]};
  await ensureDirectDefinitions(ui,{platform:{definitions:[{definition}]}},{},{},rt);state.endpointPrepared=true;save();
 }
 else if(stage==='storage'){
  assert(state.authored);const base=`/applications/${state.app.key}/environments/staging`;assert.equal((await ui.get(base+'/deployments')).deployments.length,0);
  const specs=JSON.parse(readFileSync(dir+'/gitlab-profile.json','utf8'));state.storageWorkloads??=[];
  for(const name of ['objectstore','object-init']){const prior=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===name);assert.deepEqual(prior.workload,state.savedWorkloads[name]);if(state.storageWorkloads.includes(name))continue;state.savedWorkloads[name]=await updateGitLabStorage(ui,base,state.app.key,specs.find(s=>s.name===name));state.storageWorkloads.push(name);save();console.log('Storage draft updated through UI',name);}
  state.storageUpdated=true;delete state.previewed;save();
 }
 else if(stage==='preview'){
  assert(state.authored&&state.endpointAttached);const base=`/applications/${state.app.key}/environments/staging`;
  assert.equal((await ui.get(base+'/deployments')).deployments.length,0);
  const workloads=(await ui.get(base+'/workloads')).workloads;assert.equal(workloads.length,9);for(const w of workloads)assert.deepEqual(w.workload,state.savedWorkloads[w.id]);
  await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=staging`);const response=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+base+'/preview');await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();const reply=await response,preview=await reply.json();writeFileSync(dir+'/gitlab-preview.json',JSON.stringify(preview,null,2));assert(reply.ok(),'Preview failed; inspect private response');
  assert.equal(preview.changes.length,9);assert(preview.changes.every(c=>c.action==='DEPLOY'));const order=preview.changes.map(c=>c.workloadId);for(const[provider,consumer]of[['object-init','migrations'],['migrations','webservice'],['migrations','sidekiq'],['migrations','toolbox']])assert(order.indexOf(provider)<order.indexOf(consumer));state.previewed=true;save();await page.screenshot({path:dir+'/gitlab-preview.png',fullPage:true});console.log('PASS whole GitLab batch Preview',JSON.stringify(order));
 }
 else if(stage==='attach-endpoint'){
  assert(state.authored&&state.endpointPrepared);const input=JSON.parse(readFileSync(dir+'/gitlab-input.json','utf8')),base=`/applications/${state.app.key}/environments/staging`;
  assert.equal((await ui.get(base+'/deployments')).deployments.length,0);
  await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings?environment=staging`);
  const row=(await ui.get(base+'/definition-versions')).definitions.find(d=>d.key==='gitlab-endpoint');assert(row);assert.equal(row.versions.length,1);await selectDefinitionRevisions(ui,base,{'gitlab-endpoint':row.versions[0].id},rt);
  await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/staging/workloads/webservice`);
  const prior=(await ui.get(base+'/workloads')).workloads.find(w=>w.id==='webservice');assert.deepEqual(prior.workload,state.savedWorkloads.webservice);
  if(!prior.workload.resources?.public){await page.getByRole('button',{name:'Tạo endpoint từ cổng http',exact:true}).click();await page.getByLabel('Tên endpoint',{exact:true}).fill(input.endpoint.name);await ui.submit(base+'/workloads/webservice','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));state.savedWorkloads.webservice=(await ui.get(base+'/workloads')).workloads.find(w=>w.id==='webservice').workload;save();}
  state.endpointAttached=true;save();console.log('PASS endpoint attached to webservice before first batch Deploy');
 }
 else if(stage==='author'){
  assert(state.prepared);const base=`/applications/${state.app.key}/environments/staging`,home=c.baseURL+`/ui/applications/${state.app.key}?environment=staging`;
  assert.equal((await ui.get(base+'/deployments')).deployments.length,0,'All drafts before first Deploy');
  const specs=JSON.parse(readFileSync(dir+'/gitlab-profile.json','utf8'));state.savedWorkloads??={};
  for(const spec of specs){const prior=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===spec.name);if(prior){assert.deepEqual(prior.workload,state.savedWorkloads[spec.name],'Existing draft differs; reconcile before authoring');console.log('Reused verified draft',spec.name);continue;}state.savedWorkloads[spec.name]=await authorGitLabWorkload(ui,base,home,spec);save();console.log('Saved through UI',spec.name);}
  assert.equal((await ui.get(base+'/workloads')).workloads.length,specs.length);assert.equal((await ui.get(base+'/deployments')).deployments.length,0);state.authored=true;save();await page.goto(home);await page.screenshot({path:dir+'/gitlab-drafts.png',fullPage:true});console.log('PASS all GitLab workload drafts before any Deploy');
 }
 else {
  const config=JSON.parse(readFileSync(dir+'/gitlab-input.json','utf8'));
  if(!state.app){assert(!(await ui.get('/applications')).applications.some(a=>a.name==='gitlab-issue50'),'Reconcile existing application first');await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill('gitlab-issue50');state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();}
  const store=await ensureSecretStore(ui,{name:'GitLab test Vault',backendAddress:'http://vault:8200',workloadAddress:process.env.ORCH_VAULT_WORKLOAD_ADDRESS,mount:'kv',authMount:'kubernetes',tokenFile:dir+'/vault-application-token'},rt);
  const base=`/applications/${state.app.key}/environments/staging`,pins={};for(const row of (await ui.get(base+'/definition-versions')).definitions??[]){if(!row.key.startsWith('complex-'))continue;const versions=row.versions.filter(v=>!v.archived);assert.equal(versions.length,1);pins[row.key]=versions[0].id;}
  assert.equal(Object.keys(pins).length,3);
  await ensureEnvironment(ui,{schemaVersion:3,application:{environment:'staging',variables:{},secretFiles:config.secretFiles}},state.app,store,{definitionRevisions:pins},rt);
  state.prepared=true;save();console.log('PASS GitLab application and approved secret inputs created through UI');
 }
}catch(e){await page.screenshot({path:dir+`/gitlab-${stage}-error.png`,fullPage:true});console.error('GitLab UI stage failed',stage,e.status??'',e.message);process.exitCode=1;}finally{await closeConsole(s,c);}
