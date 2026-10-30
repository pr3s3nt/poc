import {expect} from '@playwright/test';
import {cpSync,mkdirSync,readFileSync,writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {readPrivateFile} from '../../lib/console.mjs';
import {execFileSync} from 'node:child_process';

// The platform author uploads a render bundle. Application users still use
// ordinary workload forms; no manifest/Score/JSON editor or mutation API.
export async function prepareSemantics({page,baseURL,run,state,saveState,retire=false}){
 const key=retire?'semanticRetirementPrepared':'semanticRendererPrepared';if(state[key])return;
 const dir=join(run,retire?'semantic-source-retired':'semantic-source');mkdirSync(dir,{recursive:true,mode:0o700});
 cpSync('backend/render-bundles/standard',join(dir,'bundle'),{recursive:true});
 cpSync('e2e/scenarios/delivery-reconciliation/fixtures/lifecycle-resources.tpl',join(dir,'bundle/lifecycle-resources.tpl'));
 cpSync('e2e/scenarios/delivery-reconciliation/fixtures/lifecycle-patch.tpl',join(dir,'bundle/lifecycle-patch.tpl'));
 if(retire){const path=join(dir,'bundle/lifecycle-resources.tpl');writeFileSync(path,readFileSync(path,'utf8').replace(/- apiVersion: v1\n  kind: ConfigMap[\s\S]*?(?=\{\{- else if)/,''));}
 const manifest=join(dir,'bundle/bundle.yaml');writeFileSync(manifest,readFileSync(manifest,'utf8')+'  - lifecycle-patch.tpl\nresources:\n  - lifecycle-resources.tpl\n');
 const archive=join(dir,'bundle.zip');execFileSync('zip',['-q','-r',archive,'bundle'],{cwd:dir,stdio:'pipe'});
 await page.goto(`${baseURL}/ui/platform/rendering-templates`);
 await page.getByRole('button',{name:'Sửa mẫu staging-k8s-4f-renderer',exact:true}).click();
 await page.getByRole('button',{name:'Tải ZIP',exact:true}).click();await page.getByLabel('Tệp ZIP',{exact:true}).setInputFiles(archive);await page.getByLabel('Thư mục trong ZIP',{exact:true}).fill('bundle');
 const inspected=page.waitForResponse(r=>new URL(r.url()).pathname.endsWith('/render-bundles/upload'));
 await page.getByRole('button',{name:'Tải lên và kiểm tra',exact:true}).click();expect((await inspected).status()).toBe(200);
 await expect(page.getByText('Đã xác minh ZIP.',{exact:true})).toBeVisible();
 const saved=page.waitForResponse(r=>r.request().method()==='PUT'&&new URL(r.url()).pathname.endsWith('/resource-definitions/staging-k8s-4f-renderer'));
 await page.getByRole('button',{name:'Lưu thay đổi',exact:true}).click();expect((await saved).ok()).toBe(true);state[key]=true;saveState();
}

export async function runSemantics({page,baseURL,run,state,saveState,step,business}){
 const {kubectl,home,deploy}=business;
 const get=(kind,name)=>JSON.parse(kubectl('get',kind,name,'-n',state.namespace,'-o','json'));
 const migration=()=>get('job','backend-migration');
 await step('Completed Kubernetes migration Job retained across update and provider handoffs',async()=>{
  const job=migration();expect(job.status.succeeded).toBe(1);expect(job.status.failed??0).toBe(0);
  expect(job.metadata.uid).toBe(state.migrationJobUID);expect(job.spec.template).toEqual(state.migrationJobTemplate);
 });
 await step('CronJob owns scheduled successful child jobs; Fleet does not inventory children',async()=>{
  const cron=get('cronjob','backend-maintenance');const owner=cron.metadata.uid;
  await expect.poll(()=>JSON.parse(kubectl('get','jobs','-n',state.namespace,'-o','json')).items.filter(j=>j.metadata.ownerReferences?.some(ref=>ref.uid===owner)&&j.status.succeeded===1).length,{timeout:180_000,intervals:[3000]}).toBeGreaterThan(0);
  const reply=await page.request.get(`${baseURL}/api/v1/applications/${state.appID}/environments/staging/reconciliation`);expect(reply.ok()).toBe(true);
  const body=await reply.json();expect(JSON.stringify(body)).not.toContain('backend-maintenance-');
  expect(readPrivateFile(join(run,'drift-compose-project'))).toBe('orch-reconciliation-drift-20261029');
  const inventoryChildren=execFileSync('docker',['exec','orch-reconciliation-drift-20261029-postgres-1','psql','-U','orchestrator','-d','orchestrator','-Atc',`SELECT count(*) FROM delivery_intents i JOIN delivery_units u ON u.id=i.unit_id CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(i.inventory)='array' THEN i.inventory ELSE '[]'::jsonb END) item WHERE u.environment_id=(SELECT e.id FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key='${state.appID}' AND e.environment_key='staging') AND item->>'name' LIKE 'backend-maintenance-%';`],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
  expect(inventoryChildren).toBe('0');
 });
 await step('HPA keeps replica authority after opt-in self-heal and a Fleet reconciliation',async()=>{
  await expect.poll(()=>get('deployment','frontend').spec.replicas,{timeout:180_000,intervals:[3000]}).toBe(2);
  const uid=get('deployment','frontend').metadata.uid;
  await page.goto(`${baseURL}/ui/applications/${state.appID}/settings?environment=staging`);const heal=page.getByRole('checkbox',{name:'Tự sửa sai lệch',exact:true});
  if(!await heal.isChecked()){await heal.check();await page.getByRole('button',{name:'Lưu thay đổi',exact:true}).click();await expect(page.getByText('Đã lưu. Xem trước để áp dụng.',{exact:false})).toBeVisible();await home();await deploy({policyOnly:true});}
  expect(get('deployment','frontend').metadata.uid).toBe(uid);expect(get('deployment','frontend').spec.replicas).toBe(2);
  state.semanticHpaPassed=true;saveState();
 });
 await step('Removing a rendered extra object prunes its exact UID without rerunning migration',async()=>{
  if(state.semanticPrunePassed)return;
  const before=get('configmap','backend-retired-marker');expect(before.metadata.labels['orchestrator.io/application']).toBe(state.appID);
  await prepareSemantics({page,baseURL,run,state,saveState,retire:true});await home();await deploy();
  expect(kubectl('get','configmap','backend-retired-marker','-n',state.namespace,'--ignore-not-found','-o','name').trim()).toBe('');
  expect(migration().metadata.uid).toBe(state.migrationJobUID);expect(migration().status.succeeded).toBe(1);expect(get('deployment','frontend').spec.replicas).toBe(2);
  await business.checkBusiness({afterTransfer:true});state.semanticPrunePassed=true;saveState();
 });
}
