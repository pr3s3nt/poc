import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {openConsole,readConsoleConfig,closeConsole} from '../../lib/console.mjs';
import {makeUI, outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const file=dir+'/runtime.json';const state=existsSync(file)?JSON.parse(readFileSync(file)):{};const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const preparedPrefix=process.env.ORCH_PREPARED_PREFIX??'issue76-prepared';
const importedPrefix=state.importPrefix??=(process.env.ORCH_IMPORT_PREFIX??'imported-'+Date.now().toString(36));save();
const c=readConsoleConfig(),s=await openConsole(c,{}),{page}=s,ui=makeUI(page,c.baseURL,30000);page.setDefaultTimeout(30000);const stage=process.argv[2];assert(['prepare','author','deploy','verify','cleanup'].includes(stage));assert(!state.cleaned,'Use a new private run directory after cleanup');const base=()=>`/applications/${state.app.key}/environments/staging`;
const kube=(...args)=>execFileSync('kubectl',['--kubeconfig',process.env.KUBECONFIG,'--context','kind-k8s-4f',...args],{encoding:'utf8',timeout:15000,stdio:['ignore','pipe','pipe']});
try {
if(stage==='prepare') {
 if(!state.app) {await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(process.env.ORCH_APP_NAME??'import76-'+Date.now().toString(36));state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();}
 const bindings=(await ui.get('/delivery-bindings')).bindings;const selected=bindings.find(x=>x.name==='dev-fleet');assert(selected?.status==='READY');const binding=await ui.get('/delivery-bindings/'+selected.id+'?version='+selected.version);state.deliverySubject={name:binding.config.serviceAccount,namespace:binding.config.agentNamespace};save();
 const defs=(await ui.get('/resource-definitions')).resourceDefinitions.filter(x=>x.key.startsWith(preparedPrefix+'-'));
 assert.deepEqual(defs.map(x=>x.resourceType).sort(),['k8s-cluster','k8s-namespace','postgres','workload']);
 for(const source of defs) {
  const fields=['key','name','resourceType','driverType','driverAccount','driverInputs','criteria','provision'];const doc=Object.fromEntries(fields.filter(k=>source[k]!==undefined).map(k=>[k,source[k]]));
  doc.key=source.key.replace(preparedPrefix,importedPrefix);doc.name=source.name.replace('prepared','imported');doc.criteria=doc.criteria.map(x=>({...x,app_id:state.app.key}));
  if(doc.resourceType==='postgres')doc.driverInputs.values.variables.username='fallback76';
  await page.goto(c.baseURL+'/ui/platform/resource-definitions');await page.getByRole('button',{name:'Tạo cấu hình',exact:true}).click();await page.getByRole('tab',{name:'Nhập YAML/JSON',exact:true}).click();await page.getByLabel('Nội dung YAML/JSON',{exact:true}).fill(JSON.stringify(doc));
  await page.getByRole('tabpanel').filter({has:page.getByLabel('Nội dung YAML/JSON',{exact:true})}).getByRole('button',{name:'Đăng ký',exact:true}).click();
  await page.getByRole('status').filter({hasText:/CREATED|REUSED/}).waitFor();console.log('IMPORTED',doc.key);
 }
 state.namespace=state.app.environments.find(x=>x.key==='staging').namespaceIdentity;save();console.log('PASS prepared imported definitions');
}
if(stage==='author') {
 const name='import-check';const existing=(await ui.get(base()+'/workloads')).workloads;assert.equal(existing.length,0);
 await page.goto(c.baseURL+`/ui/applications/${state.app.key}/environments/staging/workloads/new`);console.log('OPEN workload');await page.getByLabel('Tên Workload',{exact:true}).fill(name);await page.getByLabel('Ảnh Container',{exact:true}).fill('postgres:16-alpine');
 await page.getByRole('button',{name:'Hiện nâng cao Container main',exact:true}).click();
 await page.locator('summary strong').getByText('Khởi chạy',{exact:true}).click();await page.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill('sh\n-c\nexec sleep 2147483647');
 console.log('CONFIGURED container');for(const [index,alias] of ['fallback','override'].entries()) {
  await page.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();await page.getByLabel('Alias tài nguyên',{exact:true}).nth(index).fill(alias);await page.getByLabel('Loại tài nguyên',{exact:true}).nth(index).selectOption('postgres');
  await page.getByLabel('Tham số database',{exact:true}).nth(index).fill(alias+'76');await page.getByLabel('Tham số username',{exact:true}).nth(index).fill(index?'override76':'required76');
 }
 console.log('SAVING workload'); await ui.submit(base()+'/workloads/'+name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));state.authored=true;save();console.log('PASS authored workload with required param overrides');
}
if(stage==='deploy') {
 assert(!state.attempt,'Inspect recorded attempt before retry');await page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=staging`);
 const preview=await ui.submit(base()+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));assert.deepEqual(preview.changes.map(x=>x.workloadId),['import-check']);
 writeFileSync(dir+'/preview.json',JSON.stringify(preview),{mode:0o600});
 page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/deploy')){state.attempt=r.postDataJSON();save()}});
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 const deadline=Date.now()+1200000;
 for(;;) {
  if(!state.namespaceUid&&kube('get','namespace',state.namespace,'--ignore-not-found','-o','name').trim()) {await ensureNamespaceAccess(ui,{runtime:{profile:'dev-test',timeoutMs:1200000,clusterKubeconfigFile:process.env.KUBECONFIG},application:{environment:'staging'},platform:{account:{context:'kind-k8s-4f'},delivery:{namespaceAccess:{enabled:true,rulesFile:new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url).pathname}}}},state.app,{state,save,record:console.log},{wait:false});}
  const op=(await ui.get(base()+'/operations?reviewRequestId='+state.attempt?.reviewRequestId)).operations?.find(x=>x.reviewRequestId===state.attempt?.reviewRequestId);
  if(op&&['SUCCEEDED','FAILED','RECOVERED','INTERRUPTED'].includes(op.status)){state.operation=op.id;state.status=op.status;save();assert.equal(op.status,'SUCCEEDED');break;}
  assert(Date.now()<deadline,'Deploy timeout');await page.waitForTimeout(5000);
 }
 console.log('PASS deploy');
}
if(stage==='verify') {
 assert.equal(state.status,'SUCCEEDED');const resources=(await ui.get(base()+'/resources')).resources;assert.deepEqual(resources.map(x=>x.type).sort(),['k8s-namespace','postgres','postgres']);assert(resources.every(x=>x.status==='READY'));
 const deployment=JSON.parse(kube('-n',state.namespace,'get','deployment','import-check','-o','json'));assert.equal(deployment.status.readyReplicas,1);
 const pg=JSON.parse(kube('-n',state.namespace,'get','statefulset','-o','json')).items;assert.equal(pg.length,2);
 const usernames=pg.map(x=>{const env=x.spec.template.spec.containers[0].env.find(e=>e.name==='POSTGRES_USER');if(env.value!==undefined)return env.value;const ref=env.valueFrom.secretKeyRef;return Buffer.from(kube('-n',state.namespace,'get','secret',ref.name,'-o','jsonpath={.data.'+ref.key+'}'),'base64').toString('utf8')}).sort();assert.deepEqual(usernames,['override76','required76']);
 const preview=JSON.parse(readFileSync(dir+'/preview.json'));for(const suffix of ['cluster','k8s-namespace','postgres','workload'])assert(JSON.stringify(preview).includes(importedPrefix+'-'+suffix));const caps=(await ui.get('/resource-definition-capabilities')).capabilities;
 const def=(await ui.get('/resource-definitions')).resourceDefinitions.find(x=>x.key===importedPrefix+'-postgres');const source=def.driverInputs.values.source;
 const cap=caps.find(x=>x.source?.sourceId===source.source_id&&x.source?.moduleId===source.module_id&&x.source?.revision===source.revision&&x.source?.contentSha256===source.content_sha256);
 assert(cap);const image=cap.variables.find(x=>x.name==='image');assert(image?.hasDefault);assert.equal(def.driverInputs.values.variables.image,undefined);assert(pg.every(x=>x.spec.template.spec.containers[0].image===image.default));
 const historyPath=`/resource-definitions/${def.key}/versions`;const history=await ui.get(historyPath);
 const fields=['key','name','resourceType','driverType','driverAccount','driverInputs','criteria','provision'];const doc=Object.fromEntries(fields.filter(k=>def[k]!==undefined).map(k=>[k,structuredClone(def[k])]));doc.driverInputs.values.variables.image=image.default;
 await page.goto(c.baseURL+'/ui/platform/resource-definitions');await page.getByRole('button',{name:'Tạo cấu hình',exact:true}).click();await page.getByRole('tab',{name:'Nhập YAML/JSON',exact:true}).click();const editor=page.getByLabel('Nội dung YAML/JSON',{exact:true});await editor.fill(JSON.stringify(doc));await page.getByRole('tabpanel').filter({has:editor}).getByRole('button',{name:'Đăng ký',exact:true}).click();await page.getByRole('status').filter({hasText:'REUSED'}).waitFor();assert.deepEqual(await ui.get(historyPath),history);
 console.log('PASS explicit module default equals omission, without a revision write');
 console.log('PASS imported pins/mappings/binding, module defaults and required params override deployed');
}
if(stage==='cleanup') await cleanup();
}catch(error){await page.screenshot({path:dir+'/runtime-error.png',fullPage:true});throw error;}finally{await closeConsole(s,c)}

async function cleanup() {
  const namespace = JSON.parse(kube('get', 'namespace', state.namespace, '-o', 'json'));
  assert.equal(namespace.metadata.uid, state.namespaceUid, 'Owned namespace UID must match');
  assert.equal(namespace.metadata.labels['app.kubernetes.io/managed-by'], 'orchestrator');
  // These two objects are technical setup owned by this scenario, not product
  // resources. Remove them before the product's foreign-child namespace guard.
  for (const kind of ['rolebindings', 'roles']) {
    const object = JSON.parse(kube('-n', state.namespace, 'get', kind, 'bootstrap-fleet-access', '--ignore-not-found', '-o', 'json') || 'null');
    if (!object) continue;
    assert.equal(object.metadata.labels['bootstrap.orchestrator.dev/application'], state.app.key);
    const options = { apiVersion: 'v1', kind: 'DeleteOptions', preconditions: { uid: object.metadata.uid } };
    execFileSync('kubectl', ['--kubeconfig', process.env.KUBECONFIG, '--context', 'kind-k8s-4f', 'delete', '--raw', `/apis/rbac.authorization.k8s.io/v1/namespaces/${state.namespace}/${kind}/bootstrap-fleet-access`, '-f', '-'], { input: JSON.stringify(options), encoding: 'utf8', timeout: 60_000, stdio: ['pipe', 'pipe', 'pipe'] });
    assert.equal(kube('-n', state.namespace, 'get', kind, 'bootstrap-fleet-access', '--ignore-not-found', '-o', 'name'), '');
  }
  await page.goto(c.baseURL + `/ui/applications/${state.app.key}/settings?environment=staging`);
  const app = (await ui.get('/applications/' + state.app.key)).application;
  if (app.status === 'DELETING') await page.getByRole('region', { name: 'Tiến trình xóa ứng dụng', exact: true }).getByRole('button', { name: 'Thử lại', exact: true }).click();
  else await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  await dialog.getByRole('textbox').fill(state.app.name);
  const response = await ui.submit(`/applications/${state.app.key}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
  const operation = response.deletion.operationId; state.deletionId = operation; save();
  const deadline = Date.now() + 1_800_000;
  for (;;) { const result = (await ui.get(`/applications/${state.app.key}/deletions/${operation}`)).deletion; if (!result.running) { assert.equal(result.status, 'SUCCEEDED'); break; } assert(Date.now() < deadline); await page.waitForTimeout(3000); }
  assert.equal(kube('get', 'namespace', state.namespace, '--ignore-not-found', '-o', 'name'), ''); state.cleaned = true; save(); console.log('PASS cleanup');
}