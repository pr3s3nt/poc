// Product scans, review and confirmation use ordinary Console buttons/dialogs.
// The only native mutation is one explicitly authorized reversible owned drift.
import {expect} from '@playwright/test';
import {join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {readPrivateFile} from '../../lib/console.mjs';

export async function runInfrastructureDrift({page,baseURL,run,state,saveState,step,business}) {
 const {kubectl,home}=business;
 const project=readPrivateFile(join(run,'drift-compose-project'));
 expect(project).toBe('orch-reconciliation-drift-20261029');
 const database=`${project}-postgres-1`;
 const sql=query=>execFileSync('docker',['exec',database,'psql','-U','orchestrator','-d','orchestrator','-Atc',query],{encoding:'utf8',stdio:['ignore','pipe','pipe']}).trim();
 expect(state.appID).toMatch(/^[a-f0-9-]{36}$/);
 const stateSnapshot=()=>sql(`SELECT coalesce(jsonb_agg(jsonb_build_object('instance',s.resource_instance_id,'serial',s.serial,'lineage',s.lineage,'version',s.version,'ciphertext',md5(s.ciphertext)) ORDER BY s.resource_instance_id),'[]') FROM terraform_states s JOIN terraform_resource_instances i ON i.id=s.resource_instance_id JOIN applications a ON a.id=i.application_id WHERE a.application_key='${state.appID}';`);
 const currentSet=()=>sql(`SELECT coalesce(current_deployment_set_id::text,'') FROM environments e JOIN applications a ON a.id=e.application_id WHERE a.application_key='${state.appID}' AND environment_key='staging';`);
 const namespace=()=>JSON.parse(kubectl('get','namespace',state.namespace,'-o','json'));
 expect(namespace().metadata.uid).toBe(state.namespaceUID);
 async function scan() {
  await home();const panel=page.getByRole('region',{name:'Sai lệch hạ tầng',exact:true});
  const queued=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname.endsWith('/drift-scans'),{timeout:30_000});
  const button=panel.getByRole('button',{name:'Kiểm tra sai lệch',exact:true});await expect(button).toBeEnabled({timeout:12*60_000});await button.click();const response=await queued;expect(response.status()).toBe(202);const {scanId}=await response.json();
  let result;
  await expect.poll(async()=>{const reply=await page.request.get(`${baseURL}/api/v1/applications/${state.appID}/environments/staging/drift-scans/${scanId}`);expect(reply.ok()).toBe(true);result=(await reply.json()).scan;return result.status;},{timeout:12*60_000,intervals:[2000]}).toBe('SUCCEEDED');
  await panel.getByRole('button',{name:'Tải lại',exact:true}).click();
  await expect(panel.getByRole('status')).toContainText('Đã kiểm tra');
  return result;
 }
 const assertNoProposedActions=result=>{
  expect(result.findings.length).toBeGreaterThanOrEqual(3);
  for(const finding of result.findings){expect(finding.report.complete).toBe(true);expect(finding.report.proposedActions).toHaveLength(0);expect(finding.report.outputNames).toHaveLength(0);expect(finding.report.outputsUnknown).toBe(false);}
  const ns=result.findings.find(finding=>finding.descriptor.startsWith('k8s-namespace.'));expect(ns?.result).toBe('IN_SYNC');
 };
 await step('Complete Terraform baseline scan through UI leaves canonical state and current Set unchanged',async()=>{
  if(state.infrastructureCleanScanPassed)return;
  const before=stateSnapshot(),set=currentSet();expect(JSON.parse(before).length).toBeGreaterThanOrEqual(3);
  const result=await scan();assertNoProposedActions(result);
  expect(stateSnapshot()).toBe(before);expect(currentSet()).toBe(set);
  state.infrastructureCleanScanPassed=true;state.infrastructureCleanScanID=result.id;saveState();
  await page.screenshot({path:join(run,'ui-evidence','infrastructure-clean-scan.png'),fullPage:true});
 });
 await step('Owned infrastructure drift detected through UI without state adoption',async()=>{
  if(state.infrastructureDriftDetected)return;
  const original=namespace();expect(original.metadata.uid).toBe(state.namespaceUID);
  const key='orchestrator-created-by-run';const value=original.metadata.annotations?.[key];expect(typeof value).toBe('string');expect(value.length).toBeGreaterThan(0);
  state.infrastructureOriginalAnnotation={key,value};state.infrastructureStateBefore=stateSnapshot();state.infrastructureSetBefore=currentSet();saveState();
  kubectl('patch','namespace',state.namespace,'--type=json','--patch',JSON.stringify([{op:'test',path:'/metadata/uid',value:state.namespaceUID},{op:'test',path:`/metadata/annotations/${key}`,value},{op:'replace',path:`/metadata/annotations/${key}`,value:`${value}-controlled-drift`}]));
  const result=await scan();expect(result.result).toBe('DRIFTED');
  const changed=result.findings.find(f=>f.report.externalDrift?.some(item=>item.type==='kubernetes_namespace_v1'));
  expect(changed).toBeTruthy();expect(changed.report.proposedActions.some(item=>item.actions.includes('update'))).toBe(true);
  expect(stateSnapshot()).toBe(state.infrastructureStateBefore);expect(currentSet()).toBe(state.infrastructureSetBefore);
  state.infrastructureScanID=result.id;state.infrastructureInstanceID=changed.instanceId;state.infrastructureDriftDetected=true;saveState();
  await page.screenshot({path:join(run,'ui-evidence','infrastructure-drift-detected.png'),fullPage:true});
 });
 await step('Review and confirm in-place infrastructure repair through UI with stable identity and outputs',async()=>{
  if(state.infrastructureRepairPassed)return;
  // Resume verification only when the original UI confirmation has a durable
  // consumed review and a succeeded operation for this exact scan and instance.
  const acknowledged=sql(`SELECT count(*) FROM drift_repair_reviews r JOIN environment_operations o ON o.id=r.consumed_operation_id WHERE r.scan_id='${state.infrastructureScanID}' AND r.status='CONSUMED' AND o.kind='DRIFT_REPAIR' AND o.status='SUCCEEDED' AND r.safe_review->'instances' @> '[{"instanceId":"${state.infrastructureInstanceID}"}]'::jsonb;`)==='1';
  if(!acknowledged){
  await home();const panel=page.getByRole('region',{name:'Sai lệch hạ tầng',exact:true});
  await panel.getByRole('button',{name:'Xem phương án sửa',exact:true}).click();const dialog=page.getByRole('dialog',{name:'Xem phương án sửa',exact:true});
  for(const checkbox of await dialog.getByRole('checkbox').all()){
   const label=await checkbox.locator('..').innerText();await checkbox.setChecked(label.includes('k8s-namespace.'));
  }
  await dialog.getByRole('button',{name:'Chuẩn bị phương án',exact:true}).click();
  const confirm=dialog.getByRole('button',{name:'Xác nhận sửa',exact:true});await expect(confirm).toBeVisible({timeout:12*60_000});expect(await confirm.isEnabled()).toBe(false);
  await expect(dialog).toContainText('kubernetes_namespace_v1.this');await page.screenshot({path:join(run,'ui-evidence','infrastructure-repair-review.png'),fullPage:true});
  await dialog.getByRole('checkbox',{name:'Tôi đã xem phương án sửa',exact:true}).check();await confirm.click();
  await expect(dialog.getByText('Đã sửa hạ tầng và xác nhận trạng thái.',{exact:true})).toBeVisible({timeout:12*60_000});
  await dialog.getByRole('button',{name:'Hoàn tất',exact:true}).click();
  }
  const repaired=namespace();expect(repaired.metadata.uid).toBe(state.namespaceUID);expect(repaired.metadata.annotations[state.infrastructureOriginalAnnotation.key]).toBe(state.infrastructureOriginalAnnotation.value);
  expect(currentSet()).toBe(state.infrastructureSetBefore);
  const after=JSON.parse(stateSnapshot()),before=JSON.parse(state.infrastructureStateBefore);
  expect(after.map(({instance,lineage})=>({instance,lineage}))).toEqual(before.map(({instance,lineage})=>({instance,lineage})));
  expect(after.some((row,index)=>row.serial>before[index].serial)).toBe(true);
  const result=await scan();assertNoProposedActions(result);
  await business.checkBusiness({afterTransfer:true});
  state.infrastructureRepairPassed=true;saveState();await page.screenshot({path:join(run,'ui-evidence','infrastructure-repair-pass.png'),fullPage:true});
 });
}
