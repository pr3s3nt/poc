import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,readdirSync,statSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
const config=readConsoleConfig();
const dir=resolve(process.env.ORCH_TEST_RUN_DIR??'');assert(process.env.ORCH_TEST_RUN_DIR&&!dir.startsWith(process.cwd()+'/'));assert.equal(statSync(dir).mode&0o077,0);
const name=process.env.ORCH_TEST_NAME;assert(name&&/^[a-z0-9-]{1,48}$/.test(name));
const kubeconfig=process.env.ORCH_CLUSTER_KUBECONFIG_FILE;assert(kubeconfig);
const account=process.env.ORCH_TEST_ACCOUNT;assert(account);
const mode=process.argv[2]??'run';assert(['run','cleanup'].includes(mode));
const receipt=join(dir,'flow.json');let state={};try{state=JSON.parse(readFileSync(receipt))}catch(e){if(e.code!=='ENOENT')throw e}
const save=()=>writeFileSync(receipt,JSON.stringify(state,null,2),{mode:0o600});
const native=(args,input)=>execFileSync('kubectl',['--kubeconfig',kubeconfig,...args],{input,encoding:'utf8',timeout:30000,stdio:['pipe','pipe','pipe']});
const json=args=>JSON.parse(native([...args,'-o','json']));
const session=await openConsole(config,{role:'PLATFORM_ENGINEER'});const {page}=session;
page.setDefaultTimeout(30000);page.on('pageerror',e=>{state.browserError=e.message;save()});
const get=async path=>{const r=await page.request.get(config.baseURL+'/api/v1'+path);assert(r.ok(),`${path} ${r.status()}`);return r.json()};
const post=async(path,data)=>{const r=await page.request.post(config.baseURL+'/api/v1'+path,{data});const b=await r.json();assert(r.ok(),`${path} ${r.status()} ${JSON.stringify(b)}`);return b};
const response=(path,method)=>page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1'+path&&r.request().method()===method,{timeout:method==='PUT'?30000:600000});
const submit=async(path,method,button)=>{const pending=response(path,method);await button.click();if(method==='PUT')await screenshot('after-save');const r=await pending;const b=await r.json();assert(r.ok(),`${path} ${r.status()} ${JSON.stringify(b)}`);return b};
const screenshot=phase=>page.screenshot({path:join(dir,phase+'.png'),fullPage:true});
let app,base,namespace;
const assertNamespace=()=>{const ns=json(['get','namespace',namespace]);assert.equal(ns.metadata.labels['app.kubernetes.io/managed-by'],'orchestrator');state.namespaceUid??=ns.metadata.uid;assert.equal(ns.metadata.uid,state.namespaceUid);save()};
async function importWorkload(document,existing){
 await page.goto(config.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/${existing?'web':'new'}`);
 await page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
 const file=join(dir,'workload.json');writeFileSync(file,JSON.stringify(document),{mode:0o600});await page.getByLabel('File Workload',{exact:true}).setInputFiles(file);await page.getByText('Đã phân tích Workload: web',{exact:false}).waitFor();
 await submit(base+'/workloads/web','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
}
async function deploy(phase){
 await page.goto(config.baseURL+`/ui/applications/${app.key}?environment=staging`);
 let before;try{before=json(['-n',namespace,'get','deployments','-o','json']).items.map(x=>[x.metadata.uid,x.metadata.resourceVersion])}catch{before=[]}
 const preview=await submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 state.preview=preview;save();assert.equal(preview.changes.length,1);
 let after;try{after=json(['-n',namespace,'get','deployments','-o','json']).items.map(x=>[x.metadata.uid,x.metadata.resourceVersion])}catch{after=[]}
 assert.deepEqual(after,before,'Preview mutated cluster');await screenshot(phase+'-preview');
 const pending=response(base+'/deploy','POST');await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 state.deliverySubject={name:'orchestrator-delivery',namespace:'cattle-fleet-system'};save();
 await ensureNamespaceAccess({page},{runtime:{profile:'dev-test',clusterKubeconfigFile:kubeconfig,timeoutMs:300000},platform:{account:{context:native(['config','current-context']).trim()},delivery:{namespaceAccess:{enabled:true,rulesFile:resolve('e2e/scenarios/workload-defaults/fixtures/fleet-access.json')}}},application:{environment:'staging'}},app,{state,save,record(){}});
 const r=await pending;const result=await r.json();state.lastDeploy={status:r.status(),result,phase};save();assert(r.ok(),JSON.stringify(result));assert.equal(result.status,'SUCCEEDED');assertNamespace();await screenshot(phase+'-deployed');
}
function verifyWeb(replicas,policy){
 const d=json(['-n',namespace,'get','deployment','web']);assert.equal(d.spec.replicas,replicas);assert.equal(d.status.availableReplicas,replicas);state.deploymentUid??=d.metadata.uid;assert.equal(d.metadata.uid,state.deploymentUid);
 if(policy){const pod=d.spec.template.spec;const c=pod.containers[0];assert.equal(pod.automountServiceAccountToken,false);assert.equal(c.securityContext.readOnlyRootFilesystem,true);assert.equal(c.securityContext.runAsNonRoot,true);assert.equal(d.spec.revisionHistoryLimit,0);assert.deepEqual(d.spec.strategy,{type:'RollingUpdate',rollingUpdate:{maxUnavailable:0,maxSurge:1}});assert.equal(c.resources.requests.cpu,'100m');assert.equal(c.resources.requests.memory,'128Mi');assert.equal(c.resources.limits.memory,'512Mi');assert.equal(c.resources.limits.cpu,undefined);assert.equal(c.volumeMounts.length,2)}
 const repos=json(['get','gitrepos.fleet.cattle.io','-A']).items.filter(r=>r.metadata.labels?.['orchestrator.io/application']===app.key||r.metadata.labels?.['orchestrator.io/application-id']===app.key);assert(repos.length>0,'Fleet repository missing');state.repositories=repos.map(r=>({name:r.metadata.name,namespace:r.metadata.namespace,uid:r.metadata.uid,commit:r.status?.commit}));save();
}
try{
 app=(await get('/applications')).applications.find(a=>a.name===name);
 if(!app&&mode==='run'){
 await page.goto(config.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(name);const created=await submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}));app=created.application;
 }
 if(mode==='cleanup'&&!app&&state.cleanupPassed){console.log('PASS recorded cleanup');}
 else{
 assert(app?.key);if(state.app)assert.equal(app.key,state.app);state.app=app.key;save();base=`/applications/${app.key}/environments/staging`;namespace=`app-${app.key}-staging`;
 if(mode==='run'){
 const definitions=(await get('/resource-definitions')).resourceDefinitions;
 const definitionPrefix=`issue37-${app.key.split('-')[0]}`;
 const register=async draft=>{const existing=definitions.find(d=>d.key===draft.key);if(!existing)await post('/resource-definitions',draft);else if(draft.resourceType==='workload'&&existing.driverInputs.values.variables.template_source!==draft.driverInputs.values.variables.template_source){const r=await page.request.put(config.baseURL+'/api/v1/resource-definitions/'+draft.key,{data:{...draft,version:existing.version}});assert(r.ok(),'Explicit template source publication failed');state.definitionUpgrade=await r.json();save()}};
 await register({key:definitionPrefix+'-cluster',name:'Issue 37 cluster',resourceType:'k8s-cluster',driverType:'existing-cluster',driverAccount:account,driverInputs:{values:{variables:{}}},criteria:[{app_id:app.key,class:'issue37'}]});
 const cap=(await get('/resource-definition-capabilities')).capabilities.find(c=>c.resourceType==='k8s-namespace'&&c.source);assert(cap);const p=cap.source;
 await register({key:definitionPrefix+'-namespace',name:'Issue 37 namespace',resourceType:'k8s-namespace',driverType:'terraform',driverInputs:{values:{source:{source_id:p.sourceId,module_id:p.moduleId,revision:p.revision,subdirectory:p.subdirectory,content_sha256:p.contentSha256},variables:{name:'${context.env.namespace}'},input_mapping:{},output_mapping:{name:'name'},provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.issue37#environments.@app.@env'}}}},criteria:[{app_id:app.key}]});
 {const root=resolve('backend/template-sources/standard');const files={};function read(path=''){for(const entry of readdirSync(join(root,path),{withFileTypes:true})){const name=path?path+'/'+entry.name:entry.name;if(entry.isDirectory())read(name);else if(!name.startsWith('examples/')&&!name.endsWith('README.md'))files[name]=readFileSync(join(root,name),'utf8')}}read();state.source=(await post('/template-sources/inline',{name:'issue37-standard',files})).templateSource;save()}
 await register({key:definitionPrefix+'-workload',name:'Issue 37 workload defaults',resourceType:'workload',driverType:'template',driverInputs:{values:{variables:{template_source:state.source.id,namespace:"${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"}}},criteria:[{app_id:app.key}]});
 // Existing environments retain their immutable Definition revision until explicit selection.
 const versions=await get(base+'/definition-versions');
 const row=versions.definitions.find(d=>d.key===definitionPrefix+'-workload');
 const latest=row?.versions.at(-1);
 if(latest&&row.desiredRevisionId!==latest.id){
  const r=await page.request.put(config.baseURL+'/api/v1'+base+'/definition-versions',{data:{definitionKey:row.key,revisionId:latest.id,expectedRevision:versions.revision,expectedEnvironmentVersion:versions.environmentVersion}});
  assert(r.ok(),'Explicit Environment Definition selection failed');state.selectedRevision=latest.id;save();
 }
 if(!state.legacyPassed){await importWorkload({apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'web'},replicas:1,containers:{main:{image:'nginx:1.27-alpine'}},service:{ports:{http:{port:80,targetPort:80}}}},false);await deploy('legacy');verifyWeb(1,false);state.legacyPassed=true;save()}
 if(!state.noopPassed){const before=(await get(base+'/workloads')).workloads.find(w=>w.id==='web').workload;await page.goto(config.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/web`);await submit(base+'/workloads/web','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));const after=(await get(base+'/workloads')).workloads.find(w=>w.id==='web').workload;assert.deepEqual(after,before);await page.goto(config.baseURL+`/ui/applications/${app.key}?environment=staging`);const preview=await submit(base+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));assert.equal(preview.changes.length,0);state.noopPassed=true;save()}
 if(!state.templatePassed){const template=(await get(base+'/workload-templates')).templates.find(t=>t.id==='nginx-readonly');assert(template);state.profile=template;save();await importWorkload({apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'web'},replicas:2,containers:{main:{}},workloadExecution:{version:1,kind:'Deployment',template:{id:template.id,version:template.version,sha256:template.sha256},automountServiceAccountToken:false,revisionHistoryLimit:0,podDisruptionBudget:{enabled:true,maxUnavailable:1,unhealthyPodEvictionPolicy:'AlwaysAllow'}}},true);await deploy('policy');verifyWeb(2,true);assert.equal(json(['-n',namespace,'get','pdb','web']).spec.maxUnavailable,1);assert.match(JSON.stringify(state.preview),/EffectiveConfig/);state.templatePassed=true;save()}
 if(!state.pdbPruned){await page.goto(config.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/web`);await page.getByRole('button',{name:'Hiện nâng cao Workload',exact:true}).click();const scaling=page.locator('details').filter({has:page.locator('summary strong').filter({hasText:/^Mở rộng & cập nhật$/})});await scaling.waitFor();if(!await scaling.evaluate(el=>el.open))await scaling.locator('summary').click();await scaling.locator('label').filter({hasText:/^PodDisruptionBudget/}).locator('select').selectOption('off');await submit(base+'/workloads/web','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));await deploy('pdb-off');verifyWeb(2,true);assert.equal(native(['-n',namespace,'get','pdb','web','--ignore-not-found','-o','name']).trim(),'');state.pdbPruned=true;save()}
 assert(!state.browserError,state.browserError);state.acceptancePassed=true;state.completedAt=new Date().toISOString();if(state.failure){state.previousFailure=state.failure;delete state.failure}save();console.log('PASS legacy no-op, explicit pinned-template adoption, non-root/read-only/two emptyDirs, Preview purity/provenance, real Fleet deploy/update/PDB prune');
 }else{
 const workloads=(await get(base+'/workloads')).workloads;
 if(workloads.some(w=>w.id==='web')){page.on('dialog',d=>d.accept());await page.goto(config.baseURL+`/ui/applications/${app.key}?environment=staging`);const row=page.locator('.table-row').filter({has:page.locator('.workload-name').filter({hasText:/web$/})});await submit(base+'/workloads/web','DELETE',row.getByRole('button',{name:'Xóa',exact:true}));await deploy('remove')}
 for(const kind of ['rolebinding','role']){const text=native(['-n',namespace,'get',kind,'bootstrap-fleet-access','--ignore-not-found','-o','json']);if(!text.trim())continue;const object=JSON.parse(text);assert.equal(object.metadata.labels['bootstrap.orchestrator.dev/application'],app.key);native(['delete','--raw',`/apis/rbac.authorization.k8s.io/v1/namespaces/${namespace}/${kind}s/bootstrap-fleet-access`,'-f','-'],JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:object.metadata.uid}}))}
 await page.goto(config.baseURL+`/ui/applications/${app.key}/settings?environment=staging`);await page.getByRole('region',{name:'Xóa ứng dụng',exact:true}).getByRole('button',{name:'Xóa ứng dụng',exact:true}).click();const dialog=page.getByRole('dialog',{name:'Xóa ứng dụng',exact:true});await dialog.getByRole('textbox').fill(name);await submit(`/applications/${app.key}/deletions`,'POST',dialog.getByRole('button',{name:'Xóa vĩnh viễn ứng dụng',exact:true}));await page.getByRole('heading',{name:'Ứng dụng của bạn',exact:true}).waitFor({timeout:600000});assert.equal(native(['get','namespace',namespace,'--ignore-not-found','-o','name']).trim(),'');state.cleanupPassed=true;save();console.log('PASS product cleanup and namespace absence');
 }
 }
}catch(error){state.failure={message:error.message,time:new Date().toISOString()};save();await screenshot('failure');throw error}finally{await closeConsole(session,config)}
