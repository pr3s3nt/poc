import { expect } from '@playwright/test';
import { execFileSync } from 'node:child_process';
import { mkdirSync, writeFileSync } from 'node:fs';
import { openConsole, closeConsole, readConsoleConfig } from '../../lib/console.mjs';
import { registerHooks } from 'node:module';
// Legacy helpers live under frontend/, which has a separate node_modules.
// Resolve their Playwright import to this scenario's declared dependency.
const playwrightURL = import.meta.resolve('@playwright/test');
registerHooks({ resolve(specifier, context, nextResolve) { return specifier === '@playwright/test' ? { url: playwrightURL, shortCircuit: true } : nextResolve(specifier, context); } });
const { createHuman, createApplication } = await import('../../../frontend/test/e2e/human.mjs');
const { makeKit } = await import('../../../frontend/test/e2e/full-app-kit.mjs');
const { onboardInternalAccount, registerTypesUi, registerExistingCluster, registerK8sModuleDefinition } = await import('../../../frontend/test/e2e/full-app-flows.mjs');
const required=name=>{if(!process.env[name])throw new Error(`${name} required`);return process.env[name];};
const config=readConsoleConfig();
const runId=required('WC_RUN_ID');const evidence=required('WC_EVIDENCE');mkdirSync(evidence,{recursive:true,mode:0o700});
const source={url:'https://github.com/pr3s3nt/poc.git',ref:required('WC_SOURCE_REF')};
const input={runId,privateDir:process.env.WC_PRIVATE_DIR??'/tmp/workload-catalog-24/private',applicationName:`Catalog ${runId}`,account:{name:`catalog-${runId}`,kubeconfigFile:required('WC_KUBECONFIG_FILE')},kube:{kubectl:'kubectl',kubeconfig:process.env.WC_HOST_KUBECONFIG??`${process.env.HOME}/.kube/config`,context:required('WC_CONTEXT')}};
const results=[];const record=(id,pass)=>{results.push({id,pass:!!pass});writeFileSync(`${evidence}/assertions.json`,JSON.stringify(results,null,2));if(!pass)throw new Error(`assertion ${id} failed`);console.log(`PASS ${id}`);};
let session;
try{
 session=await openConsole(config,{role:'PLATFORM_ENGINEER'});const {page,context}=session;page.setDefaultTimeout(60000);
 const h=createHuman(page,{fast:true});const rec={record:(id,_phase,_source,pass)=>record(id,pass)};
 let kit=makeKit({page,context,h,input,baseURL:config.baseURL,evidenceDir:evidence,rec,caption(){},phase(){},forwarders:[]});
 const get=async path=>{const r=await page.request.get(`${config.baseURL}/api/v1${path}`);if(!r.ok())throw new Error(`GET ${path}: ${r.status()}`);return r.json();};
 const types=[{key:'workload',inputs:[],outputs:[]},{key:'k8s-cluster',inputs:[],outputs:[['name','string',false],['cluster','any',true]]},{key:'k8s-namespace',inputs:[['name','string',false]],outputs:[['name','string',true]]},{key:'persistent-storage',inputs:[['storage','string',false]],outputs:[['claimName','string',true],['namespace','string',true]]}];
 const storedTypes=await get('/resource-types');
 {
  const accounts=await get('/driver-accounts');
  if(!accounts.accounts.some(account=>account.name===input.account.name&&account.status==='READY')) await onboardInternalAccount(kit);
  if(!storedTypes.resourceTypes.length) await registerTypesUi(kit,types);
  if(!(await get('/resource-definitions')).resourceDefinitions.some(d=>d.key==='catalog-cluster')) await registerExistingCluster(kit,{id:'catalog-cluster',accountName:input.account.name,klass:'internal'});
  for(const spec of [{id:'catalog-namespace',type:'k8s-namespace',module:'k8s-namespace',clusterClass:'internal',presetName:'name'},{id:'catalog-storage',type:'persistent-storage',module:'persistent-storage',clusterClass:'internal',presetNamespace:true,git:{...source,path:'backend/terraform-modules/persistent-storage'}}]){
   if((await get('/resource-definitions')).resourceDefinitions.some(d=>d.key===spec.id))continue;
   await page.goto(`${config.baseURL}/ui/platform/resource-definitions`);await page.getByRole('button',{name:'Tạo cấu hình',exact:true}).click();await registerK8sModuleDefinition(kit,spec);
  }
  if(!(await get('/resource-definitions')).resourceDefinitions.some(d=>d.key==='catalog-renderer')){
  await page.getByRole('link',{name:/(^|\s)Mẫu dựng ứng dụng$/}).click();
  await page.getByLabel('ID mẫu',{exact:true}).fill('catalog-renderer');await page.getByLabel('Tên hiển thị',{exact:true}).fill('Catalog workload');
  await page.getByLabel('Git HTTPS URL',{exact:true}).fill(source.url);await page.getByLabel('Thư mục bundle',{exact:true}).fill('backend/render-bundles/standard');await page.getByLabel('Tag hoặc commit',{exact:true}).fill(source.ref);
  const inspected=page.waitForResponse(r=>r.url().endsWith('/api/v1/render-bundles/inspect'),{timeout:180000});await page.getByRole('button',{name:'Kiểm tra bundle',exact:true}).click();const response=await inspected;if(!response.ok())throw new Error(`bundle inspect ${response.status()}`);
  await page.getByRole('button',{name:'Dùng gợi ý namespace',exact:true}).click();await page.getByRole('button',{name:'Dùng gợi ý cluster',exact:true}).click();await page.getByRole('button',{name:'Đăng ký mẫu dựng ứng dụng',exact:true}).click();await expect(page.getByRole('status').filter({hasText:'Đã đăng ký mẫu dựng ứng dụng catalog-renderer.'})).toBeVisible();
  }
 }
 await page.getByRole('link',{name:/(^|\s)Ứng dụng$/}).click();const existing=(await get('/applications')).applications.find(item=>item.name===input.applicationName);input.applicationKey=existing?.key??await createApplication(h,input.applicationName,`catalog-${runId}`);await page.goto(`${config.baseURL}/ui/applications/${input.applicationKey}`);
 kit=makeKit({page,context,h,input,baseURL:config.baseURL,evidenceDir:evidence,rec,caption(){},phase(){},forwarders:[]});
 const appPath=`/applications/${input.applicationKey}/environments/staging`;const home=()=>page.goto(`${config.baseURL}/ui/applications/${input.applicationKey}`);
 const apps=(await get('/applications')).applications;const app=apps.find(a=>a.key===input.applicationKey);input.namespace=app.environments.find(e=>e.key==='staging').namespaceIdentity ?? `app-${input.applicationKey}-staging`;
 writeFileSync(`${evidence}/scope.json`,JSON.stringify({runId,applicationKey:input.applicationKey,namespace:input.namespace,context:input.kube.context},null,2));
 async function createStateful(){
 await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();await expect(page.getByRole('dialog',{name:'Chọn loại workload'})).toBeVisible();await page.screenshot({path:`${evidence}/picker.png`});
 await page.getByRole('button',{name:/Ứng dụng có dữ liệu/}).click();await page.getByLabel('Tên Workload',{exact:true}).fill('stateful');await page.getByLabel('Ảnh Container',{exact:true}).fill(process.env.WC_IMAGE??'busybox:1.37');
 await page.getByRole('button',{name:'Hiện nâng cao Container 1',exact:true}).click();await page.getByLabel('Lệnh (mỗi dòng một phần tử)',{exact:true}).fill('sh\n-c');await page.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill('test -f /data/proof || echo retained-'+runId+' > /data/proof; exec httpd -f -p 8080 -h /data');
 await page.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();
 // The shared dependency controls use explicit labels; no API fixture creates the PVC.
 await page.getByLabel('Alias tài nguyên', {exact:true}).fill('data');
 await page.getByLabel('Loại tài nguyên', {exact:true}).last().selectOption('persistent-storage');
 await page.getByRole('button',{name:'Thêm volume mount',exact:true}).click();
 await page.getByLabel('Mount path',{exact:true}).fill('/data');
 await page.getByRole('button',{name:'+ Thêm cổng',exact:true}).click();
 await page.getByLabel('Tên cổng Service',{exact:true}).fill('http');
 await page.getByRole('spinbutton',{name:'Cổng Service',exact:true}).fill('8080');

 const saved=page.waitForResponse(r=>r.request().method()==='PUT'&&r.url().endsWith('/workloads/stateful'));
 await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();const savedResponse=await saved;if(!savedResponse.ok())throw new Error(`save stateful ${savedResponse.status()} ${(await savedResponse.text()).slice(0,400)}`);
 record('picker-to-stateful-draft',true);
 }
 if(!(await get(`${appPath}/workloads`)).workloads.some(w=>w.id==='stateful')) await createStateful();
 async function deploy(){
  await home();const pending=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(`${appPath}/preview`),{timeout:180000});
  await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();const response=await pending;const preview=await response.json();if(!response.ok())throw new Error(`preview ${response.status()} ${JSON.stringify(preview).slice(0,600)}`);
  await page.screenshot({path:`${evidence}/preview.png`,fullPage:true});
  const applied=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(`${appPath}/deploy`),{timeout:600000});
  await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();const reply=await applied;const report=await reply.json();if(report.status!=='SUCCEEDED')throw new Error(`deployment ${report.status}: ${JSON.stringify(report).slice(0,600)}`);
 }
 const kubectl=(args)=>execFileSync('kubectl',['--context',input.kube.context,'-n',input.namespace,...args],{encoding:'utf8',timeout:300000}).trim();
 if((await get(`${appPath}/workloads`)).workloads.some(w=>w.state))await deploy();
 const stateful=JSON.parse(kubectl(['get','statefulset','stateful','-o','json']));record('primary-statefulset',stateful.spec.replicas===1);
 const pvcList=JSON.parse(kubectl(['get','pvc','-o','json'])).items;record('terraform-pvc-bound',pvcList.length===1&&pvcList[0].status.phase==='Bound');const pvcUID=pvcList[0].metadata.uid;
 record('stateful-data-written',kubectl(['exec','stateful-0','--','cat','/data/proof'])===`retained-${runId}`);
 kubectl(['delete','pod','stateful-0','--wait=true']);kubectl(['rollout','status','statefulset/stateful','--timeout=180s']);
 record('data-retained-after-pod-recreation',kubectl(['exec','stateful-0','--','cat','/data/proof'])===`retained-${runId}`);
 record('same-pvc-identity',JSON.parse(kubectl(['get','pvc',pvcList[0].metadata.name,'-o','json'])).metadata.uid===pvcUID);
 await page.screenshot({path:`${evidence}/stateful-deployed.png`,fullPage:true});
 await home();await page.locator('.table-row').filter({hasText:'stateful'}).getByRole('button',{name:'Sửa',exact:true}).click();
 await page.getByRole('button',{name:'Hiện nâng cao Container 1',exact:true}).click();
 await page.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill('test -f /data/proof || echo retained-'+runId+' > /data/proof; exec httpd -f -p 8080 -h /data # updated');
 const updated=page.waitForResponse(r=>r.request().method()==='PUT'&&r.url().endsWith('/workloads/stateful'));await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();if(!(await updated).ok())throw new Error('stateful update save failed');await deploy();
 record('stateful-update-retains-data',kubectl(['exec','stateful-0','--','cat','/data/proof'])===`retained-${runId}`);
 record('stateful-update-same-pvc',JSON.parse(kubectl(['get','pvc',pvcList[0].metadata.name,'-o','json'])).metadata.uid===pvcUID);

 await home();
 if(!(await get(`${appPath}/workloads`)).workloads.some(w=>w.id==='scheduled')){
  await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();await page.getByRole('button',{name:/Tác vụ theo lịch/}).click();
  await page.getByLabel('Tên Workload',{exact:true}).fill('scheduled');await page.getByLabel('Ảnh Container',{exact:true}).fill(process.env.WC_IMAGE??'busybox:1.37');
  await page.getByLabel('Lịch cron',{exact:true}).fill('* * * * *');await page.getByLabel('Múi giờ',{exact:true}).fill('UTC');
  await page.getByRole('button',{name:'Hiện nâng cao Container 1',exact:true}).click();await page.getByLabel('Lệnh (mỗi dòng một phần tử)',{exact:true}).fill('sh\n-c');await page.getByLabel('Tham số (mỗi dòng một phần tử)',{exact:true}).fill('echo catalog-scheduled-'+runId);
  const saved=page.waitForResponse(r=>r.request().method()==='PUT'&&r.url().endsWith('/workloads/scheduled'));await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();if(!(await saved).ok())throw new Error('schedule save failed');
 }
 if((await get(`${appPath}/workloads`)).workloads.some(w=>w.state))await deploy();
 const cron=JSON.parse(kubectl(['get','cronjob','scheduled','-o','json']));record('primary-cronjob',cron.spec.schedule==='* * * * *'&&cron.spec.timeZone==='UTC');
 record('cronjob-has-no-deployment',kubectl(['get','deployment','scheduled','--ignore-not-found','-o','name'])==='');
 await expect.poll(()=>JSON.parse(kubectl(['get','jobs','-o','json'])).items.some(job=>job.metadata.ownerReferences?.some(owner=>owner.uid===cron.metadata.uid)&&job.status.conditions?.some(condition=>condition.type==='Complete'&&condition.status==='True')),{timeout:150000,intervals:[2000]}).toBe(true);
 const complete=JSON.parse(kubectl(['get','jobs','-o','json'])).items.find(job=>job.metadata.ownerReferences?.some(owner=>owner.uid===cron.metadata.uid)&&job.status.conditions?.some(condition=>condition.type==='Complete'&&condition.status==='True'));
 record('scheduled-job-completed',kubectl(['logs',`job/${complete.metadata.name}`]).includes('catalog-scheduled-'+runId));
 await home();await page.locator('.table-row').filter({hasText:'scheduled'}).getByRole('button',{name:'Lịch sử Job',exact:true}).click();
 const historyDialog=page.getByRole('dialog',{name:'Lịch sử Job',exact:true});
 await expect(historyDialog.getByText(complete.metadata.name,{exact:true})).toBeVisible();await expect(historyDialog.getByText('Thành công',{exact:true}).first()).toBeVisible();
 await expect.poll(async()=>{const logs=await get(`${appPath}/observability/logs?workload=scheduled`);return logs.logs.some(row=>row.message.includes('catalog-scheduled-'+runId));},{timeout:90000,intervals:[2000]}).toBe(true);
 record('scheduled-log-collected',true);
 record('job-history-owned-success',true);await page.screenshot({path:`${evidence}/job-history.png`});await historyDialog.getByRole('button',{name:'Đóng',exact:true}).click();

 await home();const scheduledRow=page.locator('.table-row').filter({hasText:'scheduled'});await scheduledRow.getByRole('button',{name:'Sửa',exact:true}).click();await page.getByLabel('Tạm dừng lịch',{exact:true}).check();
 const suspended=page.waitForResponse(r=>r.request().method()==='PUT'&&r.url().endsWith('/workloads/scheduled'));await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();if(!(await suspended).ok())throw new Error('suspend save failed');await deploy();
 record('schedule-suspend-applied',JSON.parse(kubectl(['get','cronjob','scheduled','-o','json'])).spec.suspend===true);
 for(const workload of ['scheduled','stateful']){
  await home();page.once('dialog',dialog=>dialog.accept());await page.locator('.table-row').filter({hasText:workload}).getByRole('button',{name:'Xóa',exact:true}).click();await deploy();
 }
 record('workload-removal-retains-pvc',JSON.parse(kubectl(['get','pvc',pvcList[0].metadata.name,'-o','json'])).metadata.uid===pvcUID);
 record('compute-removed',kubectl(['get','statefulset','stateful','--ignore-not-found','-o','name'])==='');

 // Explicitly re-author the same dependency descriptor; the platform must reuse
 // the retained resource instance, never bind an arbitrary PVC by name.
 await home();await createStateful();await deploy();
 record('retained-resource-explicit-reuse',JSON.parse(kubectl(['get','pvc',pvcList[0].metadata.name,'-o','json'])).metadata.uid===pvcUID&&kubectl(['exec','stateful-0','--','cat','/data/proof'])===`retained-${runId}`);
 await home();page.once('dialog',dialog=>dialog.accept());await page.locator('.table-row').filter({hasText:'stateful'}).getByRole('button',{name:'Xóa',exact:true}).click();await deploy();
 const resources=(await get(`${appPath}/resources`)).resources;writeFileSync(`${evidence}/resource-ids.json`,JSON.stringify(resources.map(r=>({id:r.id,type:r.type,descriptor:r.descriptor,status:r.status})),null,2));
 const namespaceResource=resources.find(resource=>resource.type==='k8s-namespace');const nsDetail=await get(`${appPath}/resources/${namespaceResource.id}`);
 record('namespace-retention-guard',JSON.stringify(nsDetail).includes('RETAINED_DATA'));
 await page.screenshot({path:`${evidence}/retained.png`,fullPage:true});
 async function deleteResource(resource){
  await page.goto(`${config.baseURL}/ui/applications/${input.applicationKey}/environments/staging/resources/${resource.id}`);
  await page.getByRole('button',{name:'Xóa tài nguyên',exact:true}).click();
  const dialog=page.getByRole('dialog',{name:'Xóa tài nguyên',exact:true});
  const name=await dialog.getByTestId('root-name').textContent();
  await dialog.getByLabel('Nhập lại tên tài nguyên',{exact:true}).fill(name);
  const admitted=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith(`/resources/${resource.id}/deletions`));
  await dialog.getByRole('button',{name:'Xác nhận xóa vĩnh viễn',exact:true}).click();
  const response=await admitted;if(!response.ok())throw new Error(`cleanup admission ${response.status()}`);
  const {cleanup:admission}=await response.json();
  await expect.poll(async()=>{
   const {cleanup}=await get(`${appPath}/resource-cleanups/${admission.operationId}`);
   if(['FAILED','HELD','CANCELLED'].includes(cleanup.status))throw new Error(`cleanup ${cleanup.status}`);
   return cleanup.status==='SUCCEEDED'&&cleanup.allVerified&&cleanup.remainingIds.length===0&&cleanup.resources.length>0&&cleanup.resources.every(row=>row.outcome==='DESTROYED'&&row.verified&&!row.remainingUnknown&&row.remaining.length===0);
  },{timeout:180000,intervals:[1500]}).toBe(true);
  record(`verified-cleanup-${resource.type}`,true);
  await expect.poll(async()=> (await get(`${appPath}/resources/${resource.id}`)).resource.status,{timeout:180000,intervals:[1500]}).toBe('DESTROYED');
 }
 const storageResource=resources.find(resource=>resource.type==='persistent-storage');
 await deleteResource(storageResource);
 record('explicit-data-deletion',kubectl(['get','pvc',pvcList[0].metadata.name,'--ignore-not-found','-o','name'])==='');
 await deleteResource(namespaceResource);
 record('owned-namespace-cleaned',kubectl(['get','namespace',input.namespace,'--ignore-not-found','-o','name'])==='');
 console.log(`PASS generic acceptance app=${input.applicationKey} namespace=${input.namespace}; owned storage and namespace cleaned`);
}catch(error){if(session)await session.page.screenshot({path:`${evidence}/failure.png`,fullPage:true}).catch(()=>{});throw error;}
finally{if(session)await closeConsole(session,config);}
