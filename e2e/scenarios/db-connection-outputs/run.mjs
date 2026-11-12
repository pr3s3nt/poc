// DB connection-string outputs on kind — issue #52. See README.md in this directory.
import assert from 'node:assert/strict';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const stage=process.argv[2];assert(['platform','prepare','preview','deploy','wait','status','verify','cleanup'].includes(stage),'stage: platform|prepare|preview|deploy|wait|status|verify|cleanup');
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir,'ORCH_TEST_RUN_DIR');outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json'));assert.match(input.moduleRef,/^[0-9a-f]{40}$/);
const file=dir+'/state.json',state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const rt={state,save,record(k,v){console.log(k,v);save()}};
export const APP_NAME='dbconn-issue52';
export const ENV='staging';
const REPO='https://github.com/pr3s3nt/poc.git';
// Images pinned by digest; each database image is also the client of its check.
const IMAGES={
 postgres:'postgres:16.14-alpine@sha256:57c72fd2a128e416c7fcc499958864df5301e940bca0a56f58fddf30ffc07777',
 redis:'redis:7.4.7-alpine@sha256:02f2cc4882f8bf87c79a220ac958f58c700bdec0dfb9b9ea61b62fb0e8f1bfcf',
 mysql:'mysql:8.4@sha256:a81d043930e51745522bd5c430961cf8286f0a23f0a9a2a6e0108f7ea71b0902',
 mariadb:'mariadb:11.4@sha256:d1891ede5a559f1086f55053bb8cc1569b0298338c4d72d71725a7f74f6ba620',
 mongodb:'mongo:7.0@sha256:8bb4442761a63a8dda452744082cdac9644b429807eb977b42324a63cef59ca4',
};
const keep='exec sleep 2147483647';
// mysql/mariadb CLIs take no URL: split it into its fields (the generated password is hex).
// Shell `${...}` is avoided: the args field passes it to the planner as a placeholder.
const split=`set -- $(echo "$DATABASE_URL" | sed -E 's#^mysql://([^:]*):([^@]*)@([^:/]*):([0-9]+)/(.*)$#\\1 \\2 \\3 \\4 \\5#');`;
const ref=r=>({ref:r});
export const WORKLOADS=[
 {name:'pg-check',type:'postgres',params:{database:'appdb',username:'app'},env:{DATABASE_URL:ref('db.uri'),JDBC_URL:ref('db.jdbcUrl')},
  script:`until psql "$DATABASE_URL" -tAc "select 1" >/dev/null 2>&1; do sleep 3; done; echo CONNECTION_OK postgres; case "$JDBC_URL" in jdbc:postgresql://*:5432/appdb) echo JDBC_OK;; esac; ${keep}`},
 {name:'redis-check',type:'redis',params:{},env:{DATABASE_URL:ref('db.uri')},
  script:`until redis-cli -u "$DATABASE_URL" ping 2>/dev/null | grep -q PONG; do sleep 3; done; echo CONNECTION_OK redis; ${keep}`},
 {name:'mysql-check',type:'mysql',params:{image:IMAGES.mysql,database:'appdb',username:'app'},env:{DATABASE_URL:ref('db.uri'),JDBC_URL:ref('db.jdbcUrl')},
  script:`${split} until MYSQL_PWD="$2" mysql -h "$3" -P "$4" -u "$1" "$5" -e "select 1" >/dev/null 2>&1; do sleep 3; done; echo CONNECTION_OK mysql; case "$JDBC_URL" in jdbc:mysql://*:3306/appdb) echo JDBC_OK;; esac; ${keep}`},
 {name:'mariadb-check',type:'mariadb',params:{image:IMAGES.mariadb,database:'appdb',username:'app'},env:{DATABASE_URL:ref('db.uri'),JDBC_URL:ref('db.jdbcUrl')},
  script:`${split} until MYSQL_PWD="$2" mariadb -h "$3" -P "$4" -u "$1" "$5" -e "select 1" >/dev/null 2>&1; do sleep 3; done; echo CONNECTION_OK mariadb; case "$JDBC_URL" in jdbc:mariadb://*:3306/appdb) echo JDBC_OK;; esac; ${keep}`},
 {name:'mongo-check',type:'mongodb',params:{database:'appdb'},env:{DATABASE_URL:ref('db.connection')},
  script:`until mongosh "$DATABASE_URL" --quiet --eval "quit(db.runCommand({ping:1}).ok===1?0:1)" >/dev/null 2>&1; do sleep 3; done; echo CONNECTION_OK mongodb; ${keep}`},
];
const NAMESPACE_REF="${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
const OUTPUTS={postgres:['host','port','database','username','password','uri','jdbcUrl'],redis:['host','port','username','password','tls','uri'],
 mysql:['host','port','database','username','password','claimName','namespace','uri','jdbcUrl'],mariadb:['host','port','database','username','password','claimName','namespace','uri','jdbcUrl'],
 mongodb:['host','port','database','username','password','connection']};
const INPUTS={postgres:['database','username'],redis:[],mysql:['image','database','username','storage'],mariadb:['image','database','username','storage'],mongodb:['database','username','storage']};
const c=readConsoleConfig(),s=await openConsole(c,{}),ui=makeUI(s.page,c.baseURL,1800000),{page}=ui;
const appBase=()=>`/applications/${state.app.key}/environments/${ENV}`;
const call=async(path,method,body)=>{const r=await page.request.fetch(c.baseURL+'/api/v1'+path,{method,data:body});const text=await r.text();assert(r.ok(),`${method} ${path}: HTTP ${r.status()} ${text.slice(0,300)}`);return JSON.parse(text);};
const kubectl=(...args)=>execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f',...args],{encoding:'utf8',timeout:60000,stdio:['ignore','pipe','pipe']});

// UC-02 EXT through the Console: add the optional outputs an older mariadb Type lacks.
async function addMariadbOutputs(){
 const typ=(await ui.get('/resource-types')).resourceTypes.find(t=>t.key==='mariadb');assert(typ,'mariadb Type');
 if(typ.outputs.some(o=>o.name==='uri')){console.log('mariadb Type already has uri');return;}
 await page.goto(c.baseURL+'/ui/platform/resource-types');
 await page.getByRole('button',{name:'Xem schema mariadb',exact:true}).click();
 await page.getByRole('button',{name:'Thêm output cho mariadb',exact:true}).click();
 const form=page.getByRole('form',{name:'Thêm output cho mariadb'});
 await form.getByLabel('Output mới 1 tên',{exact:true}).fill('uri');await form.getByLabel('Output mới 1 bí mật',{exact:true}).check();
 await form.getByRole('button',{name:'+ Thêm tham số đầu ra',exact:true}).click();
 await form.getByLabel('Output mới 2 tên',{exact:true}).fill('jdbcUrl');
 await page.screenshot({path:dir+'/type-add-outputs.png',fullPage:true});
 await ui.submit('/resource-types/mariadb/outputs','POST',form.getByRole('button',{name:'Lưu output',exact:true}));
 await page.getByText('Đã thêm output vào mariadb.').waitFor();
 const after=(await ui.get('/resource-types')).resourceTypes.find(t=>t.key==='mariadb');
 assert.deepEqual(after.outputs.filter(o=>['uri','jdbcUrl'].includes(o.name)),[{name:'uri',type:'string',secret:true},{name:'jdbcUrl',type:'string'}]);
 console.log('PASS mariadb outputs added through the Console');
}
async function ensureDefinition(body){
 const existing=(await ui.get('/resource-definitions')).resourceDefinitions.find(d=>d.key===body.key);
 if(existing){console.log('Definition REUSED',body.key);return existing;}
 const created=await call('/resource-definitions','POST',body);console.log('Definition CREATED',body.key);return created;
}
async function platform(){
 await addMariadbOutputs();
 const source=(await call('/template-sources/inspect','POST',{url:REPO,path:'backend/template-sources/standard',ref:input.moduleRef}));
 const sourceId=(source.templateSource??source).id;assert(sourceId,'template source id');
 await ensureDefinition({key:'issue52-workload',name:'Issue 52 workload',resourceType:'workload',driverType:'template',driverInputs:{values:{variables:{template_source:sourceId,template_inputs:{},namespace:NAMESPACE_REF}}},provision:{},criteria:[{env_type:ENV,class:'git-dev'}]});
 for(const type of Object.keys(OUTPUTS)){
  const capability=(await call('/terraform-modules/inspect','POST',{url:REPO,path:'projects/platform-resources/terraform/'+type,ref:input.moduleRef,resourceType:type})).capability;
  assert(capability?.source,'inspect '+type);state.pins??={};state.pins[type]=capability.source;save();
  const mapping=Object.fromEntries(OUTPUTS[type].map(n=>[n,n]));
  await ensureDefinition({key:'issue52-'+type,name:'Issue 52 '+type,resourceType:type,driverType:'terraform',
   driverInputs:{values:{variables:{namespace:NAMESPACE_REF,...(type==='mongodb'?{image:IMAGES.mongodb,username:'app',storage:'10Gi'}:{})},source:{source_id:capability.source.sourceId,module_id:capability.source.moduleId,revision:capability.source.revision,subdirectory:capability.source.subdirectory,content_sha256:capability.source.contentSha256},
    input_mapping:Object.fromEntries(INPUTS[type].map(n=>[n,{param:n}])),output_mapping:mapping,
    provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.git-dev#environments.@app.@env'}}}},
   provision:{},criteria:[{env_type:ENV,class:'in-cluster'}]});
 }
 console.log('PASS platform prerequisites');
}

async function deliverySubject(){if(!state.deliverySubject){const b=await ui.get('/delivery-default'),delivery=await ui.get(`/delivery-bindings/${b.bindingId}?version=${b.bindingVersion}`);state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};save();}}
// Waits for the recorded attempt. Fleet needs scoped access in the product-created
// namespace (technical setup, not product deployment), granted as soon as it exists.
async function waitOperation(base){
 await deliverySubject();
 const config={runtime:{profile:'dev-test',timeoutMs:1800000,clusterKubeconfigFile:input.clusterKubeconfigFile},application:{environment:ENV},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}};
 const deadline=Date.now()+2400000;
 for(;;){
  const app=(await ui.get('/applications/'+state.app.key)).application;
  const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));
  const op=ops.operations?.find(o=>o.reviewRequestId===state.attempt.reviewRequestId);
  if(op){state.attempt.operationId=op.id;if(['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(op.status)){state.attempt.status=op.status;save();writeFileSync(dir+'/operation.json',JSON.stringify(op,null,2));console.log('DEPLOY',op.status);assert.equal(op.status,'SUCCEEDED');return;}}
  if(!state.namespaceAccessReady){const ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;if(kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim()){await ensureNamespaceAccess(ui,config,app,rt,{wait:false});state.namespaceAccessReady=true;save();}}
  assert(Date.now()<deadline,'Operation remains active; inspect before resuming');await page.waitForTimeout(5000);
 }
}
async function openGroup(scope,title){
 const g=scope.locator('details').filter({has:page.locator('summary').getByText(title,{exact:true})}).first();
 assert(await g.count(),'Missing group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').first().click();return g;
}
async function fillEnv(containerName,vars){
 const table=page.getByRole('region',{name:`Biến môi trường của container ${containerName}`,exact:true}).or(page.locator(`section[aria-label="Biến môi trường của container ${containerName}"]`)).first();
 let row=await table.locator('tr[data-row]').count();
 for(const[name,value]of Object.entries(vars)){
  row++;await table.getByRole('button',{name:'+ Thêm biến',exact:true}).click();
  await table.getByLabel(`Tên biến, dòng ${row}`,{exact:true}).fill(name);
  await table.getByRole('button',{name:`Chọn nguồn, dòng ${row}`,exact:true}).click();
  const dialog=page.getByRole('dialog',{name:'Chọn giá trị có sẵn'});
  await dialog.locator('button.env-entry-option').filter({has:page.locator('code').getByText(value.ref,{exact:true})}).click();
  await dialog.waitFor({state:'hidden'});
 }
}
async function authorWorkload(spec){
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();
 await page.getByRole('button',{name:/Xử lý nền/}).first().click();
 await page.getByLabel('Tên Workload',{exact:true}).fill(spec.name);
 const wmore=page.getByRole('button',{name:'Hiện nâng cao Workload',exact:true});if(await wmore.count())await wmore.click();
 const rare=await openGroup(page,'Cấu hình ít thay đổi');await rare.getByRole('combobox',{name:'Class Workload',exact:true}).selectOption('git-dev');
 const deps=await openGroup(page,'Tài nguyên phụ thuộc');
 await deps.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();
 const dep=deps.locator('.editor-grid').filter({has:page.getByLabel('Alias tài nguyên',{exact:true})}).last();
 await dep.getByLabel('Alias tài nguyên',{exact:true}).fill('db');
 await dep.getByRole('combobox',{name:'Loại tài nguyên',exact:true}).selectOption(spec.type);
 await dep.getByRole('combobox',{name:'Cách cung cấp (class) của db',exact:true}).selectOption('in-cluster');
 for(const[k,v]of Object.entries(spec.params))await dep.getByLabel('Tham số '+k,{exact:true}).fill(v);
 const box=page.locator('section.content-panel').filter({has:page.getByRole('heading',{name:'Container 1',exact:true})});
 await box.getByLabel('Tên Container',{exact:true}).fill('main');
 await box.getByLabel('Ảnh Container',{exact:true}).fill(IMAGES[spec.type]);
 const more=box.getByRole('button',{name:new RegExp('^Hiện nâng cao Container')});if(await more.count())await more.click();
 const startup=await openGroup(box,'Khởi chạy');
 await startup.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill(['sh','-c',spec.script].join('\n'));
 await fillEnv('main',spec.env);
 await page.screenshot({path:`${dir}/authoring-${spec.name}.png`,fullPage:true});
 await ui.submit(appBase()+'/workloads/'+spec.name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 state.savedWorkloads??={};state.savedWorkloads[spec.name]=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===spec.name).workload;save();
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/${ENV}/workloads/${spec.name}`);
 await page.getByLabel('Ảnh Container',{exact:true}).first().waitFor();
 assert.equal(await page.getByLabel('Ảnh Container',{exact:true}).first().inputValue(),IMAGES[spec.type]);
 console.log('Saved/reopened through UI',spec.name);
}
async function prepare(){
 if(!state.app){
  assert(!(await ui.get('/applications')).applications.some(a=>a.name===APP_NAME),'Inspect existing same-name app before adopting it');
  await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(APP_NAME);
  state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();
 }
 const store=(await ui.get('/secret-stores')).secretStores.find(x=>x.key===input.secretStoreKey);assert(store,'Secret Store');
 await ensureEnvironment(ui,{schemaVersion:3,application:{environment:ENV,variables:{},secretFiles:{}}},state.app,store,{},rt);
 for(const spec of WORKLOADS){
  const prior=(await ui.get(appBase()+'/workloads')).workloads.find(w=>w.id===spec.name);
  if(prior){assert.deepEqual(prior.workload,state.savedWorkloads?.[spec.name],'Existing draft differs from the recorded save; inspect before editing');console.log('Reused verified draft',spec.name);continue;}
  await authorWorkload(spec);
 }
 console.log('PASS workload drafts saved and reopened');
}
async function previewAndDeploy(){
 const base=appBase(),names=WORKLOADS.map(w=>w.name).sort();
 if(state.attempt&&stage==='deploy'){
  assert.equal(process.env.ORCH_RETRY_CONFIRMED,'1','Prior attempt exists: diagnose before explicit retry');
  const ops=await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId));
  assert(['FAILED','INTERRUPTED','RECOVERED'].includes(ops.operations.find(o=>o.id===state.attempt.operationId)?.status),'Only a terminal failed attempt can be retried');
  state.previousAttempts??=[];state.previousAttempts.push(state.attempt);delete state.attempt;save();
 }
 const workloads=(await ui.get(base+'/workloads')).workloads;assert.deepEqual(workloads.map(w=>w.id).sort(),names);
 for(const w of workloads)assert.deepEqual(w.workload,state.savedWorkloads[w.id],'Draft changed outside the recorded UI save: '+w.id);
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=${ENV}`);
 const preview=await ui.submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 const n=(state.previewCount=(state.previewCount??0)+1);save();writeFileSync(`${dir}/preview-${n}.json`,JSON.stringify(preview,null,2));
 await page.screenshot({path:`${dir}/preview-${n}.png`,fullPage:true});
 console.log('PREVIEW',JSON.stringify({changes:preview.changes?.map(x=>({workload:x.workloadId,action:x.action})),errors:preview.errors}));
 assert.deepEqual((preview.changes??[]).map(x=>x.workloadId).sort(),names,'Preview must contain the whole five-workload batch');
 if(stage!=='deploy')return;
 let count=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/api/v1'+base+'/deploy'){count++;state.attempt={startedAt:new Date().toISOString(),reviewRequestId:r.postDataJSON()?.reviewRequestId,requestCount:count,status:'UNKNOWN'};save();}});
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 await page.waitForTimeout(3000);assert.equal(count,1,'Exactly one batch Deploy request');assert(state.attempt?.reviewRequestId);
 await waitOperation(base);
}
// Read-only checks. Secret bytes are compared in memory and never printed.
async function verify(){
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 const secrets=JSON.parse(kubectl('-n',ns,'get','secrets','-o','json')).items;
 const passwords=secrets.filter(x=>/-auth$/.test(x.metadata.name)).flatMap(x=>Object.entries(x.data??{}).filter(([k])=>/password/.test(k)).map(([,v])=>Buffer.from(v,'base64').toString()));
 assert(passwords.length>=5,'database auth Secrets');
 const deployments=JSON.parse(kubectl('-n',ns,'get','deployments','-o','json')).items;
 const report={namespace:ns,workloads:{}};
 for(const spec of WORKLOADS){
  const d=deployments.find(x=>x.metadata.name===spec.name||x.metadata.labels?.['app.kubernetes.io/name']===spec.name);assert(d,'Deployment '+spec.name);
  const env=d.spec.template.spec.containers.find(x=>x.name==='main').env??[];
  const url=env.find(e=>e.name==='DATABASE_URL');assert(url?.valueFrom?.secretKeyRef&&url.value===undefined,'DATABASE_URL must be a secretKeyRef: '+spec.name);
  const logs=kubectl('-n',ns,'logs','deploy/'+d.metadata.name,'-c','main');
  assert.match(logs,new RegExp('CONNECTION_OK '+(spec.type==='mongodb'?'mongodb':spec.type)),'connection '+spec.name);
  if(spec.env.JDBC_URL)assert.match(logs,/JDBC_OK/,'jdbc '+spec.name);
  report.workloads[spec.name]={deployment:d.metadata.name,ready:d.status.readyReplicas===1,databaseUrl:'secretKeyRef',connection:'CONNECTION_OK',jdbc:spec.env.JDBC_URL?'JDBC_OK':'n/a'};
 }
 const exposed=[JSON.stringify(state.savedWorkloads),...['preview-1.json','preview-2.json','operation.json'].filter(f=>existsSync(dir+'/'+f)).map(f=>readFileSync(dir+'/'+f,'utf8')),JSON.stringify(deployments)];
 for(const p of passwords)assert(!exposed.some(text=>text.includes(p)),'a database password leaked into a stored document');
 report.passwordLeak='none';writeFileSync(dir+'/verify.json',JSON.stringify(report,null,2));console.log('PASS verify',JSON.stringify(report));
}
// Product cleanup through the Console's guarded deletion; native reads prove absence.
async function cleanup(){
 const app=(await ui.get('/applications/'+state.app.key)).application,ns=app.environments.find(e=>e.key===ENV).namespaceIdentity;
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings`);
 // The Fleet access granted by this scenario (outside the product) would keep the namespace alive.
 if(kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim())console.log(kubectl('-n',ns,'delete','role,rolebinding','-l','bootstrap.orchestrator.dev/application='+state.app.key,'--ignore-not-found').trim());
 const retry=page.getByRole('region',{name:'Tiến trình xóa ứng dụng',exact:true}).getByRole('button',{name:'Thử lại',exact:true});
 // A failed deletion is resumed explicitly with Thử lại; the earlier attempt stays in deletion-attempt*.json.
 if(app.status==='DELETING')await retry.click();
 else await page.getByRole('region',{name:'Xóa ứng dụng',exact:true}).getByRole('button',{name:'Xóa ứng dụng',exact:true}).click();
 const dialog=page.getByRole('dialog',{name:'Xóa ứng dụng',exact:true});
 await dialog.getByRole('textbox').fill(app.name);
 await page.screenshot({path:dir+'/delete-confirmation.png',fullPage:true});
 const deletion=await ui.submit(`/applications/${state.app.key}/deletions`,'POST',dialog.getByRole('button',{name:'Xóa vĩnh viễn ứng dụng',exact:true}));
 const id=deletion.deletion.operationId,deadline=Date.now()+1800000;let progress;
 for(;;){progress=(await ui.get(`/applications/${state.app.key}/deletions/${id}`)).deletion;if(!progress.running)break;assert(Date.now()<deadline,'deletion still running');await page.waitForTimeout(5000);}
 writeFileSync(dir+'/deletion.json',JSON.stringify(progress,null,2));
 assert.equal(progress.status,'SUCCEEDED',progress.failureCode);
 assert.equal(kubectl('get','namespace',ns,'--ignore-not-found','-o','name').trim(),'');
 const pvs=JSON.parse(kubectl('get','persistentvolumes','-o','json')).items.filter(pv=>pv.spec.claimRef?.namespace===ns);
 console.log('PASS cleanup',JSON.stringify({namespace:ns,namespaceAbsent:true,remainingPVs:pvs.length,status:progress.status}));
}
try{
 if(stage==='platform')await platform();
 else if(stage==='prepare')await prepare();
 else{
  assert(state.app);
  if(stage==='status')console.log(JSON.stringify(await ui.get(appBase()+'/operations?reviewRequestId='+encodeURIComponent(state.attempt?.reviewRequestId??''))));
  else if(stage==='wait'){assert(state.attempt?.reviewRequestId,'No recorded attempt');await waitOperation(appBase());}
  else if(stage==='verify')await verify();
  else if(stage==='cleanup')await cleanup();
  else await previewAndDeploy();
 }
}catch(e){await page.screenshot({path:dir+'/'+stage+'-error.png',fullPage:true});console.error(stage,e.message);process.exitCode=1;}finally{await closeConsole(s,c);}
