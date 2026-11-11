// Refs pr3s3nt/poc#75: real Console Deploy/failover/redeploy/cleanup on kind.
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import { ensureNamespaceAccess } from '../../../scripts/bootstrap/lib/namespace-access.mjs';
process.umask(0o077);
const stage=process.argv[2]; assert(['prepare','author','deploy','verify','redeploy','cleanup'].includes(stage));
const dir=process.env.ORCH_TEST_RUN_DIR; assert(dir); outsideRepo(dir);
const file=dir+'/state.json', state=existsSync(file)?JSON.parse(readFileSync(file)):{};
const save=()=>writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
const config=readConsoleConfig(),session=await openConsole(config,{}),{page}=session;
const ui=makeUI(page,config.baseURL,1_800_000);
const base=()=>`/applications/${state.app.key}/environments/staging`;
const kube=(...args)=>execFileSync('kubectl',['--kubeconfig',process.env.KUBECONFIG,'--context','kind-k8s-4f',...args],{encoding:'utf8',timeout:60_000,stdio:['ignore','pipe','pipe']}).trim();
async function prepare(){
 if(!state.app){
  assert(!(await ui.get('/applications')).applications.some(a=>a.name==='postgres-ha-issue75'),'Inspect existing trial before retry');
  await page.goto(config.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill('postgres-ha-issue75');
  state.app=(await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}))).application;save();
 }
 const stores=(await ui.get('/secret-store-choices')).secretStores;const store=stores.find(s=>s.name==='Dev Platform Vault');assert(store);
 const app=(await ui.get('/applications/'+state.app.key)).application;
 if(!app.environments.find(e=>e.key==='staging').secretStoreKey){
  await page.goto(config.baseURL+`/ui/applications/${state.app.key}/settings?environment=staging`);
  await page.getByLabel('Kho bí mật cho Staging',{exact:true}).selectOption(store.key);
  await ui.submit(base()+'/secret-store','PUT',page.getByRole('button',{name:'Lưu kho bí mật',exact:true}));
 }
 const def=await ui.get('/delivery-default');assert(def.bindingId);
 const binding=await ui.get(`/delivery-bindings/${def.bindingId}?version=${def.bindingVersion}`);
 state.deliverySubject={name:binding.config.serviceAccount,namespace:binding.config.agentNamespace};save();console.log('PASS prepared');
}
async function author(){
 const existing=(await ui.get(base()+'/workloads')).workloads;assert.equal(existing.length,0,'Inspect existing draft before retry');
 await page.goto(config.baseURL+`/ui/applications/${state.app.key}/environments/staging/workloads/new`);
 await page.getByLabel('Tên Workload',{exact:true}).fill('client');await page.getByLabel('Ảnh Container',{exact:true}).fill('postgres:16-alpine');
 await page.getByRole('button',{name:'Hiện nâng cao Container main',exact:true}).click();
 const startup=page.locator('details').filter({has:page.locator('summary strong').getByText('Khởi chạy',{exact:true})}).first();
 if(await startup.getAttribute('open')===null)await startup.locator('summary').first().click();
 await startup.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill('sh\n-c\nexec sleep 2147483647');
 for(const [i,[alias,cls,db]] of [['db-ha','ha','ha_app'],['db-single','in-cluster','single_app']].entries()){
  await page.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();
  await page.getByLabel('Alias tài nguyên',{exact:true}).nth(i).fill(alias);await page.getByLabel('Loại tài nguyên',{exact:true}).nth(i).selectOption('postgres');
  await page.getByLabel('Cách cung cấp (class) của '+alias,{exact:true}).selectOption(cls);
  await page.getByLabel('Tham số database',{exact:true}).nth(i).fill(db);await page.getByLabel('Tham số username',{exact:true}).nth(i).fill('app');
 }
 await page.getByRole('button',{name:'Nhập từ file .env',exact:true}).click();
 await page.getByLabel('Nội dung .env',{exact:true}).fill(['PGHOST=@db-ha.host','PGUSER=@db-ha.username','PGDATABASE=@db-ha.database','PGPASSWORD=@db-ha.password','HA_URI=@db-ha.uri','SINGLE_URI=@db-single.uri'].join('\n'));
 await ui.submit(base()+'/workloads/client','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));console.log('PASS saved all resources before one Environment Deploy');
}
function cluster(){
 const items=JSON.parse(kube('-n',state.namespace,'get','clusters.postgresql.cnpg.io','-o','json')).items;assert.equal(items.length,1);return items[0];
}
function client(){const items=JSON.parse(kube('-n',state.namespace,'get','pods','-l','app.kubernetes.io/name=client','-o','json')).items;assert.equal(items.length,1);return items[0].metadata.name;}
function sql(query,which='host'){
 const uri=which==='ha'?'"$HA_URI" ':which==='single'?'"$SINGLE_URI" ':'';
 return kube('-n',state.namespace,'exec',client(),'--','sh','-c','psql '+uri+'-XAt -v ON_ERROR_STOP=1 -c "$1"','sh',query);
}
async function verify(){
 const c=cluster();assert.equal(c.status.readyInstances,2);assert.equal(c.spec.instances,2);
 state.clusterName=c.metadata.name;state.clusterUID=c.metadata.uid;state.primaryBefore=c.status.currentPrimary;save();
 assert.equal(sql('select current_database()'),'ha_app');
 sql("create table issue75_probe (id integer primary key,value text); insert into issue75_probe values(1,'before')");
 assert.equal(sql('select value from issue75_probe where id=1','ha'),'before');
 assert.equal(sql('select current_database()','single'),'single_app');
 sql("create table issue75_probe (id integer primary key); insert into issue75_probe values(1)",'single');
 const standby=c.status.instancesStatus.healthy.find(p=>p!==state.primaryBefore);assert(standby);
 assert.equal(kube('-n',state.namespace,'exec',standby,'--','psql','-U','postgres','-d','ha_app','-XAt','-c','select value from issue75_probe where id=1'),'before');
 const start=Date.now();kube('-n',state.namespace,'delete','pod',state.primaryBefore,'--wait=false');
 for(;;){try{if(cluster().status.currentPrimary!==state.primaryBefore&&sql("insert into issue75_probe values(2,'after') on conflict do nothing; select count(*) from issue75_probe",'ha').endsWith('2'))break;}catch{}assert(Date.now()-start<90_000,'failover timeout');await page.waitForTimeout(1000);}
 state.failoverMs=Date.now()-start;assert(state.failoverMs<=60_000);assert.equal(sql('select value from issue75_probe where id=1'),'before');
 state.primaryAfter=cluster().status.currentPrimary;save();console.log('PASS host/URI, in-cluster regression, failover ms',state.failoverMs);
}
async function redeploy(){
 const openClient=async()=>{await page.goto(config.baseURL+`/ui/applications/${state.app.key}/environments/staging/workloads/client`);await page.getByRole('heading',{name:'Sửa client',exact:true}).waitFor();};
 await openClient();
 await ui.submit(base()+'/workloads/client','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 await page.goto(config.baseURL+`/ui/applications/${state.app.key}?environment=staging`);
 const unchanged=await ui.submit(base()+'/preview','POST',page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 assert.deepEqual(unchanged.changes,[]);assert(await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).isDisabled());
 assert.equal(cluster().metadata.uid,state.clusterUID);assert.equal(sql('select count(*) from issue75_probe','ha'),'2');
 state.unchangedPreviewVerified=true;save();
 // The product deliberately disables empty Deploy. Exercise real resource reuse
 // with an environment-only client update; both database inputs remain identical.
 await openClient();await page.getByRole('button',{name:'Nhập từ file .env',exact:true}).click();
 const text=page.getByLabel('Nội dung .env',{exact:true});const value=await text.inputValue();assert(!value.includes('REDEPLOY_PROBE='),'Inspect previous redeploy before retry');await text.fill(value+'\nREDEPLOY_PROBE=second');
 await ui.submit(base()+'/workloads/client','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 state.firstAttempt??=state.attempt;state.attempt=null;save();await deploy();
 assert.equal(cluster().metadata.uid,state.clusterUID);assert.equal(sql('select count(*) from issue75_probe','ha'),'2');assert.equal(sql('select count(*) from issue75_probe','single'),'1');state.redeployVerified=true;save();console.log('PASS unchanged Preview and client update Deploy retained Cluster UID and both databases');
}
async function deploy(expected = ['client']) {
  assert(!state.attempt, 'Recorded Deploy must be inspected before another attempt');
  await page.goto(config.baseURL + `/ui/applications/${state.app.key}?environment=staging`);
  const preview = await ui.submit(base() + '/preview', 'POST', page.getByRole('button', { name: 'Xem trước thay đổi', exact: true }));
  assert.deepEqual((preview.changes ?? []).map(item => item.workloadId).sort(), expected);
  assert(JSON.stringify(preview).includes('staging-k8s-4f-postgres-ha'), 'Preview must choose the HA Definition');
  page.on('request', request => { if (request.method() === 'POST' && new URL(request.url()).pathname === '/api/v1' + base() + '/deploy') { assert(!state.attempt); state.attempt = { reviewRequestId: request.postDataJSON()?.reviewRequestId }; save(); } });
  await page.getByRole('button', { name: 'Triển khai các thay đổi này', exact: true }).click();
  const deadline = Date.now() + 1_800_000;
  for (;;) {
    const app = (await ui.get('/applications/' + state.app.key)).application;
    state.namespace = app.environments.find(env => env.key === 'staging').namespaceIdentity; save();
    if (!state.namespaceAccessReady && kube('get', 'namespace', state.namespace, '--ignore-not-found', '-o', 'name')) {
      await ensureNamespaceAccess(ui, { runtime: { profile: 'dev-test', timeoutMs: 1_800_000, clusterKubeconfigFile: process.env.KUBECONFIG }, application: { environment: 'staging' }, platform: { account: { context: 'kind-k8s-4f' }, delivery: { namespaceAccess: { enabled: true, rulesFile: new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json', import.meta.url).pathname } } } }, app, { state, save, record: console.log }, { wait: false }); state.namespaceAccessReady = true; save();
    }
    const operation = (await ui.get(base() + '/operations?reviewRequestId=' + state.attempt.reviewRequestId)).operations?.find(item => item.reviewRequestId === state.attempt.reviewRequestId);
    if (operation && ['SUCCEEDED', 'FAILED', 'RECOVERED', 'INTERRUPTED'].includes(operation.status)) { state.operationId = operation.id; state.status = operation.status; save(); assert.equal(operation.status, 'SUCCEEDED'); break; }
    assert(Date.now() < deadline, 'Deploy still active; inspect before retry'); await page.waitForTimeout(5000);
  }
  console.log('PASS one Environment deploy');
}
async function cleanup() {
  if (!state.cleanupInventory) {
    const currentCluster = cluster();
    assert.equal(currentCluster.metadata.uid, state.clusterUID, 'Owned Cluster UID must match');
    state.cleanupInventory = Object.fromEntries(['clusters.postgresql.cnpg.io', 'pods', 'persistentvolumeclaims', 'services'].map(kind => [kind, JSON.parse(kube('-n', state.namespace, 'get', kind, '-o', 'json')).items.map(item => ({name:item.metadata.name,uid:item.metadata.uid}))])); save();
  }
  const namespace = JSON.parse(kube('get', 'namespace', state.namespace, '--ignore-not-found', '-o', 'json') || 'null');
  if (namespace) {
    assert.equal(namespace.metadata.uid, state.namespaceUid, 'Owned namespace UID must match');
    assert.equal(namespace.metadata.labels['app.kubernetes.io/managed-by'], 'orchestrator');
  } else {
    assert(state.deletionId && state.cleanupInventory, 'Absence requires an earlier recorded product deletion');
  }
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
  await page.goto(config.baseURL + `/ui/applications/${state.app.key}/settings?environment=staging`);
  const app = (await ui.get('/applications/' + state.app.key)).application;
  if (app.status === 'DELETING') await page.getByRole('region', { name: 'Tiến trình xóa ứng dụng', exact: true }).getByRole('button', { name: 'Thử lại', exact: true }).click();
  else await page.getByRole('region', { name: 'Xóa ứng dụng', exact: true }).getByRole('button', { name: 'Xóa ứng dụng', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: 'Xóa ứng dụng', exact: true });
  await dialog.getByRole('textbox').fill(state.app.name);
  const response = await ui.submit(`/applications/${state.app.key}/deletions`, 'POST', dialog.getByRole('button', { name: 'Xóa vĩnh viễn ứng dụng', exact: true }));
  const operation = response.deletion.operationId; state.deletionId = operation; save();
  const deadline = Date.now() + 1_800_000;
  for (;;) { const result = (await ui.get(`/applications/${state.app.key}/deletions/${operation}`)).deletion; if (!result.running) { assert.equal(result.status, 'SUCCEEDED'); break; } assert(Date.now() < deadline); await page.waitForTimeout(3000); }
  assert.equal(kube('get', 'namespace', state.namespace, '--ignore-not-found', '-o', 'name'), ''); for (const kind of Object.keys(state.cleanupInventory)) assert.equal(JSON.parse(kube('-n', state.namespace, 'get', kind, '-o', 'json')).items.length, 0, kind + ' must be absent');
  const pvDeadline = Date.now() + 60_000;
  for (;;) { const remaining = JSON.parse(kube('get','persistentvolumes','-o','json')).items.filter(pv => pv.spec.claimRef?.namespace === state.namespace); if (!remaining.length) break; assert(Date.now() < pvDeadline, 'Run-owned PVs remain'); await page.waitForTimeout(1000); }
  state.cleaned = true; save(); console.log('PASS product cleanup, Cluster/Pod/PVC/Service and PV absence');
}
try{await({prepare,author,deploy,verify,redeploy,cleanup})[stage]();}
catch(error){console.log('UI_STATE',page.url(),await page.getByRole('heading').allTextContents());throw error;}
finally{await closeConsole(session,config);}
