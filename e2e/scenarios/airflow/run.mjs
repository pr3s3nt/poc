// Apache Airflow through Console UI — issue #51. See README.md in this directory.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureTypes} from '../../../scripts/bootstrap/lib/platform.mjs';
import {ensureDirectDefinitions,ensureTemplateSources} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
import {ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const stage=process.argv[2];assert(['platform','prepare','preview','deploy','wait','status'].includes(stage),'stage: platform|prepare|preview|deploy|wait|status');
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir,'ORCH_TEST_RUN_DIR');outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json'));
assert.equal(input.publicUrl,`http://${input.endpointName}.${input.rootDomain}`);
const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const rt={state,save,record(k,v){console.log(k,v);save()}};
export const APP_NAME='airflow-kiemthu';
export const ENV='staging';
// Official image released from https://github.com/apache/airflow, pinned by digest.
const TEMPLATE_SOURCE={name:'standard-main',kind:'git',url:'https://github.com/pr3s3nt/poc.git',path:'backend/template-sources/standard',ref:'2755c8646fd70f8bdc409e3d59897804ca436817',pin:{revision:'2755c8646fd70f8bdc409e3d59897804ca436817'}};
export const IMAGE='apache/airflow:3.4.0@sha256:a8d9d19c563c768b2bc605ef26440f28a9164ea886064e99c599b057432ff572';
// Airflow 3 topology (LocalExecutor): api-server runs migrations and creates the
// FAB admin in an init container; the other components wait for migrations.
// Every value is entered in the Console form; secrets come from Environment keys
// and the shared PostgreSQL resource outputs, never literals.
const ref=r=>({ref:r}),key=k=>({key:k});
const COMMON={
 DB_HOST:ref('db.host'),DB_PORT:ref('db.port'),DB_NAME:ref('db.database'),DB_USER:ref('db.username'),DB_PASSWORD:ref('db.password'),
 AIRFLOW__DATABASE__SQL_ALCHEMY_CONN_CMD:`sh -c 'printf "postgresql+psycopg2://%s:%s@%s:%s/%s" "$DB_USER" "$DB_PASSWORD" "$DB_HOST" "$DB_PORT" "$DB_NAME"'`,
 AIRFLOW__CORE__EXECUTOR:'LocalExecutor',AIRFLOW__CORE__LOAD_EXAMPLES:'true',
 AIRFLOW__CORE__AUTH_MANAGER:'airflow.providers.fab.auth_manager.fab_auth_manager.FabAuthManager',
 AIRFLOW__CORE__EXECUTION_API_SERVER_URL:'http://api-server:8080/execution/',
 // Task logs are served by the scheduler's log server and fetched by Pod IP.
 AIRFLOW__CORE__HOSTNAME_CALLABLE:'airflow.utils.net.get_host_ip_address',
 AIRFLOW__API_AUTH__JWT_SECRET:key('AIRFLOW_JWT_SECRET'),AIRFLOW__API__SECRET_KEY:key('AIRFLOW_API_SECRET_KEY'),AIRFLOW__CORE__FERNET_KEY:key('AIRFLOW_FERNET_KEY'),
};
const WAIT={name:'wait-migrations',args:['db','check-migrations','--migration-wait-timeout','600'],env:COMMON};
export const WORKLOADS=[
 {name:'api-server',template:'Backend API',args:['api-server'],cpu:['250m','2'],memory:['1Gi','2Gi'],port:8080,endpoint:true,probe:{port:'8080',path:'/api/v2/monitor/health'},
  init:{name:'migrate',args:['version'],env:{...COMMON,_AIRFLOW_DB_MIGRATE:'true',_AIRFLOW_WWW_USER_CREATE:'true',_AIRFLOW_WWW_USER_USERNAME:'admin',_AIRFLOW_WWW_USER_PASSWORD:key('AIRFLOW_ADMIN_PASSWORD')}},newDb:true},
 {name:'scheduler',template:'Xử lý nền',args:['scheduler'],cpu:['250m','2'],memory:['1Gi','2Gi'],init:WAIT},
 {name:'dag-processor',template:'Xử lý nền',args:['dag-processor'],cpu:['100m','1'],memory:['512Mi','1536Mi'],init:WAIT},
 {name:'triggerer',template:'Xử lý nền',args:['triggerer'],cpu:['100m','1'],memory:['512Mi','1536Mi'],init:WAIT},
];
const c=readConsoleConfig(),s=await openConsole(c,{}),ui=makeUI(s.page,c.baseURL,1200000),{page}=ui;
const appBase=()=>`/applications/${state.app.key}/environments/${ENV}`;

const deliverySubject=async()=>{if(!state.deliverySubject){const b=await ui.get('/delivery-default'),delivery=await ui.get(`/delivery-bindings/${b.bindingId}?version=${b.bindingVersion}`);state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};save();}};
// Waits for the recorded attempt. Fleet needs scoped access in the product-created
// namespace (technical setup, not product deployment), granted as soon as it exists.
async function waitOperation(base){
   await deliverySubject();
   const config={runtime:{profile:'dev-test',timeoutMs:1200000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:ENV},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
   const deadline=Date.now()+1800000;
   for(;;){
    const app=(await ui.get('/applications/'+state.app.key)).application;
    const ops=await ui.get(appBase()+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));
    const op=ops.operations?.find(o=>o.reviewRequestId===state.attempt.reviewRequestId);
    if(op){state.attempt.operationId=op.id;if(['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(op.status)){state.attempt.status=op.status;save();writeFileSync(dir+'/operation.json',JSON.stringify(op,null,2));console.log('DEPLOY',op.status);assert.equal(op.status,'SUCCEEDED');return;}}
    if(!state.namespaceAccessReady){const ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;const found=execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f','get','namespace',ns,'--ignore-not-found','-o','name'],{encoding:'utf8',timeout:15000,stdio:['ignore','pipe','pipe']});if(found.trim()){await ensureNamespaceAccess(ui,config,app,rt,{wait:false});state.namespaceAccessReady=true;save();}}
    assert(Date.now()<deadline,'Operation remains active; inspect before resuming');await page.waitForTimeout(5000);
   }
}
const containerBox=n=>page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:`Container ${n}`,exact:true})});
async function openGroup(scope,title){
 const g=scope.locator('details').filter({has:page.locator('summary').getByText(title,{exact:true})}).first();
 assert(await g.count(),'Missing group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').first().click();return g;
}
async function fillEnv(n,containerName,vars){
 const table=page.getByRole('region',{name:`Biến môi trường của container ${containerName}`,exact:true}).or(page.locator(`section[aria-label="Biến môi trường của container ${containerName}"]`)).first();
 let row=await table.locator('tr[data-row]').count();
 for(const[name,value]of Object.entries(vars)){
  row++;await table.getByRole('button',{name:'+ Thêm biến',exact:true}).click();
  await table.getByLabel(`Tên biến, dòng ${row}`,{exact:true}).fill(name);
  if(typeof value==='string'){await table.getByLabel(`Giá trị nhập, dòng ${row}`,{exact:true}).fill(value);continue;}
  await table.getByRole('button',{name:`Chọn nguồn, dòng ${row}`,exact:true}).click();
  const dialog=page.getByRole('dialog',{name:'Chọn giá trị có sẵn'});
  const label=value.key?`Ứng dụng / ${value.key}`:value.ref;
  await dialog.locator('button.env-entry-option').filter({has:page.locator('code').getByText(label,{exact:true})}).click();
  await dialog.waitFor({state:'hidden'});
 }
}
async function container(n,spec){
 const box=containerBox(n);
 await box.getByLabel('Tên Container',{exact:true}).fill(spec.name);
 await box.getByLabel('Ảnh Container',{exact:true}).fill(IMAGE);
 const more=box.getByRole('button',{name:new RegExp('^Hiện nâng cao Container')});if(await more.count())await more.click();
 const startup=await openGroup(box,'Khởi chạy');
 if(spec.role)await startup.getByRole('combobox',{name:/^Vai trò container/}).selectOption(spec.role);
 if(spec.role==='init')await startup.locator('input[type=number]').fill('1');
 await startup.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill(spec.args.join('\n'));
 if(spec.cpu){const compute=await openGroup(box,'Tài nguyên tính toán');for(const[label,v]of[['Yêu cầu CPU',spec.cpu[0]],['Giới hạn CPU',spec.cpu[1]],['Yêu cầu bộ nhớ',spec.memory[0]],['Giới hạn bộ nhớ',spec.memory[1]]])await compute.getByLabel(label,{exact:true}).fill(v);}
 if(spec.port){const ports=await openGroup(box,'Cổng container');await ports.getByRole('button',{name:'Thêm cổng container',exact:true}).click();await ports.getByLabel('Tên cổng container',{exact:true}).fill('http');await ports.getByLabel('Số cổng container',{exact:true}).fill(String(spec.port));}
 if(spec.probe){const health=await openGroup(box,'Kiểm tra sức khỏe');for(const title of['Readiness HTTP path','Liveness HTTP path','Startup HTTP path']){const p=health.locator('fieldset').filter({has:page.locator('legend').getByText(title,{exact:true})});await p.getByRole('checkbox').check();await p.getByLabel('Cổng kiểm tra',{exact:true}).fill(spec.probe.port);await p.getByLabel('Đường dẫn HTTP',{exact:true}).fill(spec.probe.path);}}
 await fillEnv(n,spec.name,spec.env);
}
async function authorWorkload(spec){
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();
 await page.getByRole('button',{name:new RegExp(spec.template)}).first().click();
 await page.getByLabel('Tên Workload',{exact:true}).fill(spec.name);
 // Workload-level groups: class, shared PostgreSQL, Service and public endpoint.
 const wmore=page.getByRole('button',{name:'Hiện nâng cao Workload',exact:true});if(await wmore.count())await wmore.click();
 const rare=await openGroup(page,'Cấu hình ít thay đổi');await rare.getByRole('combobox',{name:'Class Workload',exact:true}).selectOption('git-dev');
 const deps=await openGroup(page,'Tài nguyên phụ thuộc');
 await deps.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();
 const dep=deps.locator('.editor-grid').filter({has:page.getByLabel('Alias tài nguyên',{exact:true})}).last();
 if(spec.newDb){
  await dep.getByLabel('Alias tài nguyên',{exact:true}).fill('db');
  await dep.getByRole('combobox',{name:'Loại tài nguyên',exact:true}).selectOption('postgres');
  await dep.getByRole('combobox',{name:'Cách cung cấp (class) của db',exact:true}).selectOption('in-cluster');
  await dep.getByLabel('Tham số database',{exact:true}).fill('airflow');await dep.getByLabel('Tham số username',{exact:true}).fill('airflow');
 }else{
  const source=dep.getByRole('combobox',{name:'Nguồn tài nguyên',exact:true});
  const options=await source.locator('optgroup option:not([disabled])').evaluateAll(o=>o.map(x=>({value:x.value,text:x.textContent})));
  assert.equal(options.length,1,'Expected exactly one shared PostgreSQL to reuse: '+JSON.stringify(options));
  await source.selectOption(options[0].value);await dep.getByLabel('Alias tài nguyên',{exact:true}).fill('db');
 }
 if(spec.port){
  const net=await openGroup(page,'Service & kết nối');
  await net.getByRole('combobox',{name:'Service',exact:true}).selectOption('true');
  await net.getByRole('button',{name:'+ Thêm cổng',exact:true}).click();
  await net.getByLabel('Tên cổng Service',{exact:true}).fill('http');await net.getByLabel('Cổng Service',{exact:true}).fill(String(spec.port));
  if(spec.endpoint){await net.getByRole('button',{name:'Tạo endpoint từ cổng http',exact:true}).click();const ep=net.getByRole('group',{name:'Endpoint công khai'});await ep.getByLabel('Tên endpoint',{exact:true}).fill(input.endpointName);const path=ep.getByLabel('Đường dẫn endpoint',{exact:true});if(await path.count())await path.fill('/');}
 }
 // Containers: the main App container, then one init container.
 await container(1,{name:'main',args:spec.args,cpu:spec.cpu,memory:spec.memory,port:spec.port,probe:spec.probe,env:COMMON});
 await page.getByRole('button',{name:'+ Thêm Container',exact:true}).click();
 await container(2,{...spec.init,role:'init'});
 await page.screenshot({path:`${dir}/authoring-${spec.name}.png`,fullPage:true});
 await ui.submit(appBase()+'/workloads/'+spec.name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 state.savedWorkloads??={};state.savedWorkloads[spec.name]=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===spec.name).workload;save();
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/${ENV}/workloads/${spec.name}`);
 await page.getByLabel('Ảnh Container',{exact:true}).first().waitFor();
 assert.equal(await page.getByLabel('Ảnh Container',{exact:true}).first().inputValue(),IMAGE);
 console.log('Saved/reopened through UI',spec.name);
}
try{
if(stage==='platform'){
 // Platform Engineer prerequisites: public endpoint Type and an app-scoped endpoint Definition.
 await ensureTypes(ui,[{key:'endpoint',inputs:[{name:'name',type:'string',required:true},{name:'port',type:'string',required:true},{name:'path',type:'string',required:false}],outputs:[{name:'hostname',type:'string',required:true,secret:false},{name:'url',type:'string',required:true,secret:false}]}],rt);
 assert(state.app,'Create the Application first (prepare) so the endpoint Definition can be scoped to it');
 const endpoint={key:'af51-endpoint',name:'Airflow trial endpoint',resourceType:'endpoint',driverType:'endpoint',driverInputs:{values:{variables:{rootDomain:input.rootDomain,scheme:'http',gatewayName:input.gatewayName,gatewayNamespace:input.gatewayNamespace,gatewaySectionName:input.gatewaySectionName}}},provision:{},criteria:[{app_id:state.app.key}]};
 // The seeded git-dev-workload pins a template source that this backend reports
 // UNAVAILABLE (renderer-unavailable on Preview). An app-scoped workload Definition
 // uses the current standard template source; {app_id,class} outranks {env_type,class}.
 const sources=await ensureTemplateSources(ui,{platform:{templateSources:[TEMPLATE_SOURCE]}},rt);
 const workload={key:'af51-workload',name:'Airflow trial workload',resourceType:'workload',driverType:'template',driverInputs:{values:{variables:{template_source:'${templateSources.standard-main}',template_inputs:{},namespace:"${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"}}},provision:{},criteria:[{app_id:state.app.key,class:'git-dev'}]};
 await ensureDirectDefinitions(ui,{platform:{definitions:[{definition:endpoint},{definition:workload}]}},{},sources,rt);
 console.log('PASS platform prerequisites');
}else if(stage==='prepare'){
 if(!state.app){
  assert(!(await ui.get('/applications')).applications.some(a=>a.name===APP_NAME),'Inspect existing same-name app before adopting it');
  await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(APP_NAME);
  state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();
 }
 const store=(await ui.get('/secret-stores')).secretStores.find(x=>x.key===input.secretStoreKey);assert(store,'Secret Store');
 const secretFiles={AIRFLOW_JWT_SECRET:dir+'/secrets/jwt-secret',AIRFLOW_API_SECRET_KEY:dir+'/secrets/api-secret-key',AIRFLOW_FERNET_KEY:dir+'/secrets/fernet-key',AIRFLOW_ADMIN_PASSWORD:dir+'/secrets/admin-password'};
 await ensureEnvironment(ui,{schemaVersion:3,application:{environment:ENV,variables:{},secretFiles}},state.app,store,{},rt);
 console.log('PASS application and environment configuration',state.app.key);
 const only=process.argv[3];
 for(const spec of WORKLOADS){
  if(only&&spec.name!==only)continue;
  const prior=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===spec.name);
  if(prior){assert.deepEqual(prior.workload,state.savedWorkloads?.[spec.name],'Existing draft differs from the recorded save; inspect before editing');console.log('Reused verified draft',spec.name);continue;}
  await authorWorkload(spec);
 }
 console.log('PASS workload drafts saved and reopened',Object.keys(state.savedWorkloads??{}).join(','));
}else{
 assert(state.app);const base=appBase(),names=WORKLOADS.map(w=>w.name).sort();
 if(stage==='status'){console.log(JSON.stringify(await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt?.reviewRequestId??''))));}
 else if(stage==='wait'){assert(state.attempt?.reviewRequestId,'No recorded attempt');await waitOperation(base);}
 else{
  if(state.attempt&&stage==='deploy'){
   // A recorded attempt is never replayed implicitly; only a confirmed, released failure may be retried.
   assert.equal(process.env.ORCH_RETRY_CONFIRMED,'1','Prior attempt exists: diagnose before explicit retry');
   const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));
   assert(['FAILED','INTERRUPTED','RECOVERED'].includes(ops.operations.find(o=>o.id===state.attempt.operationId)?.status),'Only a terminal failed attempt can be retried');
   assert(!(await ui.get('/applications/'+state.app.key)).application.environments.find(e=>e.key===ENV).activeOperation,'Environment still busy');
   state.previousAttempts??=[];state.previousAttempts.push(state.attempt);delete state.attempt;save();
  }
  const workloads=(await ui.get(base+'/workloads')).workloads;assert.deepEqual(workloads.map(w=>w.id).sort(),names);
  for(const w of workloads)assert.deepEqual(w.workload,state.savedWorkloads[w.id],'Draft changed outside the recorded UI save: '+w.id);
  const history=(await ui.get(base+'/deployments')).deployments,recorded=[...(state.previousAttempts??[]),...(state.attempt?[state.attempt]:[])];
  assert(history.every(d=>recorded.some(a=>a.reviewRequestId===d.reviewRequestId)),'Unrecorded previous deployment; inspect before continuing');
  await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
  const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
  const n=(state.previewCount=(state.previewCount??0)+1);save();writeFileSync(`${dir}/preview-${n}.json`,JSON.stringify(preview,null,2));
  await page.screenshot({path:`${dir}/preview-${n}.png`,fullPage:true});
  console.log('PREVIEW',JSON.stringify({changes:preview.changes?.map(x=>({workload:x.workloadId,action:x.action})),errors:preview.errors}));
  assert.deepEqual((preview.changes??[]).map(x=>x.workloadId).sort(),names,'Preview must contain the whole four-workload batch');
  assert((preview.changes??[]).every(x=>['DEPLOY','UPDATE'].includes(x.action)));
  if(stage==='deploy'){
   let count=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+base+'/deploy'){count++;state.attempt={startedAt:new Date().toISOString(),reviewRequestId:r.postDataJSON()?.reviewRequestId,requestCount:count,status:'UNKNOWN'};save();}});
   await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
   await page.waitForTimeout(3000);assert.equal(count,1,'Exactly one batch Deploy request');assert(state.attempt?.reviewRequestId);
   await waitOperation(base);
  }
 }
}
}catch(e){await page.screenshot({path:dir+'/'+stage+'-error.png',fullPage:true});console.error(stage,e.message);process.exitCode=1;}finally{await closeConsole(s,c);}
