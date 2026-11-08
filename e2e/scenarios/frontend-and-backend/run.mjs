import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureTypes} from '../../../scripts/bootstrap/lib/platform.mjs';
import {openDefinition,ensureDirectDefinitions} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
import {selectDefinitionRevisions,ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const stage=process.argv[2];assert(['prepare','preview','deploy','status'].includes(stage));
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json'));
assert.equal(input.publicUrl,`http://${input.endpointName}.${input.rootDomain}`);
const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const rt={state,save,record(k,v){console.log(k,v);save()}};
const images={frontend:'ghcr.io/mathieu-benoit/backstage-frontend@sha256:417be2df85e981434866736cc52c3892f6678adc9076faf925b3e4dc113f44b5',backend:'ghcr.io/mathieu-benoit/backstage-backend@sha256:d82b9e20cbb87e78114060ecc0139b26dc7b6990bcc5cbde174fd673ade98a12'};
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,1200000),{page}=ui;
async function group(title){await page.getByLabel('Ảnh Container',{exact:true}).first().waitFor();while(await page.locator('button[data-workload-advanced][aria-expanded=false]').count())await page.locator('button[data-workload-advanced][aria-expanded=false]').first().click();const g=page.locator('details.runtime-config-group').filter({has:page.locator('summary strong').getByText(title,{exact:true})});assert.equal(await g.count(),1,'Missing group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').click();return g;}
try{
if(stage==='prepare'){
 if(!state.app){
  assert(!(await ui.get('/applications')).applications.some(a=>a.name==='fb-issue47'),'Inspect existing same-name app before adopting it');
  await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill('fb-issue47');
  state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();
 }
 const app=state.app,base=`/applications/${app.key}/environments/staging`;
 await ensureTypes(ui,[{key:'endpoint',inputs:[{name:'name',type:'string',required:true},{name:'port',type:'string',required:true},{name:'path',type:'string',required:false}],outputs:[{name:'hostname',type:'string',required:true,secret:false},{name:'url',type:'string',required:true,secret:false}]}],rt);
 const endpoint={key:'fb47-endpoint',name:'Frontend backend test endpoint',resourceType:'endpoint',driverType:'endpoint',driverInputs:{values:{variables:{rootDomain:input.rootDomain,scheme:'http',gatewayName:input.gatewayName,gatewayNamespace:input.gatewayNamespace,gatewaySectionName:input.gatewaySectionName}}},provision:{},criteria:[{app_id:app.key}]};
 await ensureDirectDefinitions(ui,{platform:{definitions:[{definition:endpoint}]}},{},{},rt);
 if(!(await ui.get('/resource-definitions')).resourceDefinitions.some(d=>d.key==='fb47-postgres')){
  await openDefinition(ui,'postgres');
  await page.getByRole('textbox',{name:'Git HTTPS URL',exact:true}).fill(input.repository);
  await page.getByLabel('Thư mục module',{exact:true}).fill('terraform/postgres');
  await page.getByLabel('Tag hoặc commit',{exact:true}).filter({visible:true}).fill('v0.3.0');
  state.postgresSource=await ui.submit('/terraform-modules/inspect','POST',page.getByRole('button',{name:'Kiểm tra module',exact:true}));save();
  await page.getByRole('button',{name:'Tham số: JSON nâng cao',exact:true}).click();
  await page.getByLabel('Tham số cấu hình (JSON object)',{exact:true}).fill(JSON.stringify({namespace:"${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"}));
  await page.locator('details.definition-advanced > summary').click();await page.getByRole('button',{name:'Nâng cao: JSON nâng cao',exact:true}).click();
  await page.getByLabel('Ánh xạ module JSON',{exact:true}).fill(JSON.stringify({input_mapping:{database:{param:'database'},username:{param:'username'}},output_mapping:Object.fromEntries(['host','port','database','username','password'].map(k=>[k,k])),provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.default#environments.@app.@env'}}}));
  await page.getByLabel('ID cấu hình (Definition ID)',{exact:true}).fill('fb47-postgres');await page.getByLabel('Tên hiển thị',{exact:true}).fill('Frontend backend test PostgreSQL');
  await page.getByRole('tab',{name:/^Điều kiện áp dụng/}).click();await page.getByRole('button',{name:'+ Thêm điều kiện',exact:true}).first().click();await page.getByRole('combobox',{name:'Trường hợp 1 thuộc tính 1',exact:true}).selectOption('app_id');await page.getByRole('textbox',{name:/^Điều kiện 1 /}).first().fill(app.key);
  await ui.submit('/resource-definitions','POST',page.getByRole('button',{name:'Đăng ký cấu hình tài nguyên',exact:true}).first());
 }
 const current=(await ui.get('/applications/'+app.key)).application.environments.find(e=>e.key==='staging');assert(!current.activeOperation,'Recover or finish the current Operation before preparing another batch');
 if(!current.secretStoreKey){const store=(await ui.get('/secret-stores')).secretStores.find(x=>x.name==='Boutique runtime Vault');assert(store);await page.goto(c.baseURL+`/ui/applications/${app.key}/settings?environment=staging`);await page.getByLabel('Kho bí mật cho Staging',{exact:true}).selectOption(store.key);await ui.submit(base+'/secret-store','PUT',page.getByRole('button',{name:'Lưu kho bí mật',exact:true}));}
 assert(input.templateRevisionId,'An available renderer revision must be selected explicitly');
 await page.goto(c.baseURL+`/ui/applications/${app.key}/settings?environment=staging`);
 await selectDefinitionRevisions(ui,base,{'boutique-workload':input.templateRevisionId},rt);
 const store=(await ui.get('/secret-stores')).secretStores.find(x=>x.name==='Boutique runtime Vault');assert(store);
 await ensureEnvironment(ui,{schemaVersion:3,application:{environment:'staging',variables:{PUBLIC_ORIGIN:input.publicUrl},secretFiles:{}}},app,store,{definitionRevisions:{'boutique-workload':input.templateRevisionId}},rt);
 for(const name of ['backend','frontend']){
  const prior=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===name);
  if(prior){assert.deepEqual(prior.workload,state.savedWorkloads?.[name],'Existing draft differs; inspect before editing');console.log('Reused verified draft',name);continue;}
  await page.goto(c.baseURL+`/ui/applications/${app.key}?environment=staging`);await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();await page.getByRole('button',{name:/Backend API/}).click();
  await page.getByLabel('Tên Workload',{exact:true}).fill(name);await page.getByLabel('Ảnh Container',{exact:true}).fill(images[name]);
  const network=await group('Service & kết nối');await network.getByRole('button',{name:'+ Thêm cổng',exact:true}).click();await network.getByLabel('Tên cổng Service',{exact:true}).last().fill('http');await network.locator('input[type=number]').last().fill(name==='frontend'?'3000':'7007');
  if(name==='frontend'){await network.getByRole('button',{name:/Hiện.*http/}).click();await network.getByLabel('Cổng đích của Container',{exact:true}).fill('8080');}
  const deps=await group('Tài nguyên phụ thuộc');
  async function dependency(alias,type,params){await deps.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();const row=deps.locator('.editor-grid').filter({has:page.getByLabel('Alias tài nguyên',{exact:true})}).last();await row.getByLabel('Alias tài nguyên',{exact:true}).fill(alias);await row.getByRole('combobox',{name:'Loại tài nguyên',exact:true}).selectOption(type);for(const[k,v]of Object.entries(params)){const control=row.getByLabel('Tham số '+k,{exact:true});if(await control.evaluate(e=>e.tagName)==='SELECT')await control.selectOption(v);else await control.fill(v);}}
  await dependency('public','endpoint',{name:input.endpointName,port:'http',path:name==='frontend'?'/':'/api'});
  if(name==='backend')await dependency('pg','postgres',{database:'backstage',username:'backstage'});
  const vars={APP_CONFIG_app_baseUrl:{key:'PUBLIC_ORIGIN'},APP_CONFIG_backend_baseUrl:{key:'PUBLIC_ORIGIN'},...(name==='backend'?{APP_CONFIG_auth_providers_guest_dangerouslyAllowOutsideDevelopment:'true',APP_CONFIG_backend_cors_origin:{key:'PUBLIC_ORIGIN'},APP_CONFIG_techRadar_url:'https://github.com/mathieu-benoit/humanitec-ref-arch/blob/main/tech-radar.json',POSTGRES_HOST:{ref:'pg.host'},POSTGRES_PORT:{ref:'pg.port'},POSTGRES_USER:{ref:'pg.username'},POSTGRES_PASSWORD:{ref:'pg.password'}}:{APP_CONFIG_app_title:'Scaffolded Backstage App',APP_CONFIG_organization_name:'My Company'})};
  let i=0;for(const[key,value]of Object.entries(vars)){i++;await page.getByRole('button',{name:'+ Thêm biến',exact:true}).click();const row=page.locator('tr[data-row]').nth(i-1);await row.locator('td input').first().fill(key);if(typeof value==='string')await row.getByRole('textbox').nth(1).fill(value);else{await row.getByRole('button',{name:`Chọn nguồn, dòng ${i}`,exact:true}).click();await page.getByRole('button',{name:new RegExp(value.key?value.key+'$':'^'+value.ref.replace('.','\\.'))}).click();}}
  const compute=await group('Tài nguyên tính toán');for(const[label,value]of [['Yêu cầu CPU','100m'],['Yêu cầu bộ nhớ',name==='backend'?'512Mi':'64Mi'],['Giới hạn CPU','1000m'],['Giới hạn bộ nhớ',name==='backend'?'2Gi':'256Mi']])await compute.getByLabel(label,{exact:true}).fill(value);
  const health=await group('Kiểm tra sức khỏe');for(const title of ['Readiness HTTP path','Liveness HTTP path']){const p=health.locator('fieldset').filter({has:page.locator('legend').getByText(title,{exact:true})});await p.getByRole('checkbox').check();await p.getByLabel('Cổng kiểm tra',{exact:true}).fill(name==='frontend'?'8080':'7007');await p.getByLabel('Đường dẫn HTTP',{exact:true}).fill(name==='frontend'?'/healthcheck':'/.backstage/health/v1/'+(title.startsWith('Readiness')?'readiness':'liveness'));}
  if(name==='backend'){const startup=await group('Khởi chạy');await startup.locator('textarea').nth(0).fill('node');await startup.locator('textarea').nth(1).fill('packages/backend\n--config\napp-config.yaml\n--config\napp-config.production.yaml');}
  await ui.submit(base+'/workloads/'+name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
  state.savedWorkloads??={};state.savedWorkloads[name]=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===name).workload;save();
  await page.goto(c.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/${name}`);await page.getByLabel('Ảnh Container',{exact:true}).waitFor();assert.equal(await page.getByLabel('Ảnh Container',{exact:true}).inputValue(),images[name]);console.log('Saved/reopened through UI',name);
 }
 // Upgrade earlier literal trial drafts through the same form, preserving all prior attempts.
 for(const name of ['backend','frontend']){
  const vars=state.savedWorkloads[name].containers.main.variables;
  const desired={APP_CONFIG_app_baseUrl:{key:'PUBLIC_ORIGIN'},APP_CONFIG_backend_baseUrl:{key:'PUBLIC_ORIGIN'},...(name==='backend'?{APP_CONFIG_backend_cors_origin:{key:'PUBLIC_ORIGIN'}}:{APP_CONFIG_app_title:'Scaffolded Backstage App',APP_CONFIG_organization_name:'My Company'})};
  if(Object.entries(desired).every(([k,v])=>vars[k]===(typeof v==='string'?v:'${resources.env.'+v.key+'}')))continue;
  await page.goto(c.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/${name}`);await page.getByLabel('Ảnh Container',{exact:true}).waitFor();
  for(const[key,value]of Object.entries(desired)){
   const rows=page.locator('tr[data-row]');let index=await rows.evaluateAll((es,k)=>es.findIndex(e=>e.querySelector('td input')?.value===k),key);
   if(index<0){await page.getByRole('button',{name:'+ Thêm biến',exact:true}).click();index=await rows.count()-1;await rows.nth(index).locator('td input').first().fill(key);}
   const row=rows.nth(index);
   if(typeof value==='string')await row.getByRole('textbox').nth(1).fill(value);
   else{await row.getByRole('button',{name:new RegExp('^(Chọn|Đổi) nguồn, dòng '+(index+1)+'$')}).click();await page.getByRole('button',{name:new RegExp(value.key+'$')}).click();}
  }
  await ui.submit(base+'/workloads/'+name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));state.savedWorkloads[name]=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===name).workload;save();
 }
 if(!state.savedWorkloads.backend.containers.main.startupProbe){
  await page.goto(c.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/backend`);
  const health=await group('Kiểm tra sức khỏe');const p=health.locator('fieldset').filter({has:page.locator('legend').getByText('Startup HTTP path',{exact:true})});
  await p.getByRole('checkbox').check();await p.getByLabel('Cổng kiểm tra',{exact:true}).fill('7007');await p.getByLabel('Đường dẫn HTTP',{exact:true}).fill('/.backstage/health/v1/readiness');
  await ui.submit(base+'/workloads/backend','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));state.savedWorkloads.backend=(await ui.get(base+'/workloads')).workloads.find(w=>w.id==='backend').workload;save();
 }
 console.log('PASS both workload drafts ready before the batch deployment',state.app.key);
}else{
 assert(state.app);const base=`/applications/${state.app.key}/environments/staging`;
 if(stage==='status'){console.log(JSON.stringify(await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt?.reviewRequestId??''))));}
 else{
  if(state.attempt){
   assert.equal(process.env.ORCH_RETRY_CONFIRMED,'1','Prior attempt exists: diagnose before explicit retry');
   assert.equal(state.attempt.status,'FAILED','Only confirmed failed operations can be retried');
   const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));assert.equal(ops.operations.find(o=>o.id===state.attempt.operationId)?.status,'FAILED');
   assert(!(await ui.get('/applications/'+state.app.key)).application.environments.find(e=>e.key==='staging').activeOperation);
   if(stage==='deploy'){state.previousAttempts??=[];state.previousAttempts.push(state.attempt);delete state.attempt;save();}
  }
  const workloads=(await ui.get(base+'/workloads')).workloads;assert.deepEqual(workloads.map(w=>w.id).sort(),['backend','frontend']);for(const w of workloads)assert.deepEqual(w.workload,state.savedWorkloads[w.id]);
  const history=(await ui.get(base+'/deployments')).deployments,recorded=[...(state.previousAttempts??[]),...(state.attempt?[state.attempt]:[])];assert(history.every(d=>recorded.some(a=>a.reviewRequestId===d.reviewRequestId)), 'Unrecorded previous deployment; inspect ownership before continuing');
  await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=staging`);
  const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));writeFileSync(dir+'/preview.json',JSON.stringify(preview,null,2));
  console.log('PREVIEW',JSON.stringify({changes:preview.changes?.map(x=>({workload:x.workloadId,action:x.action,endpoint:x.endpoint})),errors:preview.errors}));assert.deepEqual(preview.changes.map(x=>x.workloadId).sort(),['backend','frontend']);assert(preview.changes.every(x=>['DEPLOY','UPDATE'].includes(x.action)));
  if(stage==='deploy'){
   const app=(await ui.get('/applications/'+state.app.key)).application;const b=await ui.get('/delivery-default'),delivery=await ui.get(`/delivery-bindings/${b.bindingId}?version=${b.bindingVersion}`);state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};save();
   const config={runtime:{profile:'dev-test',timeoutMs:1200000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:'staging'},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
   let count=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+base+'/deploy'){count++;state.attempt={startedAt:new Date().toISOString(),reviewRequestId:r.postDataJSON()?.reviewRequestId,requestCount:count,status:'UNKNOWN'};save();}});
   await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();assert.equal(count,1);assert(state.attempt?.reviewRequestId);
   const deadline=Date.now()+1200000;for(;;){const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));const op=ops.operations?.find(o=>o.reviewRequestId===state.attempt.reviewRequestId);if(op&&['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(op.status)){state.attempt.status=op.status;state.attempt.operationId=op.id;save();writeFileSync(dir+'/operation.json',JSON.stringify(op,null,2));console.log('DEPLOY',op.status);assert.equal(op.status,'SUCCEEDED');break;}if(!state.namespaceAccessReady){const ns=app.environments.find(e=>e.key==='staging').namespaceIdentity;const found=execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f','get','namespace',ns,'--ignore-not-found','-o','name'],{encoding:'utf8',timeout:15000,stdio:['ignore','pipe','pipe']});if(found.trim()){await ensureNamespaceAccess(ui,config,app,rt,{wait:false});state.namespaceAccessReady=true;save();}}assert(Date.now()<deadline,'Operation remains active; inspect before resuming');await page.waitForTimeout(2000);}
  }
 }
}
}catch(e){await page.screenshot({path:dir+'/'+stage+'-error.png',fullPage:true});console.error(stage,e.message);process.exitCode=1;}finally{await closeConsole(s,c);}
