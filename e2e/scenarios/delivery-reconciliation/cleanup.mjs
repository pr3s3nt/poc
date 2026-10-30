import {expect} from '@playwright/test';
import {readFileSync,writeFileSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {execFileSync} from 'node:child_process';
import {openConsole,closeConsole,readConsoleConfig,readPrivateFile} from '../../lib/console.mjs';
const run=resolve(process.env.ORCH_RECON_RUN_DIR??'');
if(!process.env.ORCH_RECON_RUN_DIR||run.includes('/projects/thieu/poc/')) throw Error('Private run directory required');
const statePath=resolve(run,process.env.ORCH_RECON_STATE_FILE??'ui-state.json');
if(!statePath.startsWith(`${run}/`))throw Error('State file must remain inside private run directory');
const state=JSON.parse(readFileSync(statePath,'utf8'));
const cluster=readPrivateFile(join(run,'cluster-name'));
expect(cluster).toMatch(/^orch-reconciliation-/);
expect(execFileSync('docker',['inspect','--format','{{.Id}}',`${cluster}-control-plane`],{encoding:'utf8'}).trim()).toBe(readPrivateFile(join(run,'container-id')));
const kube=(...args)=>execFileSync('kubectl',['--kubeconfig',join(run,'kubeconfig'),...args],{encoding:'utf8',stdio:['ignore','pipe','pipe']});
expect(kube('get','namespace','kube-system','-o','jsonpath={.metadata.uid}')).toBe(readPrivateFile(join(run,'kube-system-uid')));
const config=readConsoleConfig(),session=await openConsole(config,{role:'PLATFORM_ENGINEER'}),{page}=session;
try {
 const reply=await page.request.get(`${config.baseURL}/api/v1/applications/${state.appID}`);
 if(reply.status()!==404){
  expect(reply.ok()).toBe(true);const body=await reply.json();expect(body.application.name).toBe(state.appName);
  if (!state.workloadRemovalPassed) {
   await page.goto(`${config.baseURL}/ui/applications/${state.appID}?environment=staging`);
   await expect(page.locator('.workload-name').filter({hasText:'backend'})).toBeVisible();
   page.on('dialog', dialog => void dialog.accept());
   for (const name of ['backend','frontend','worker']) {
    const row=page.locator('.table-row').filter({has:page.locator('.workload-name',{hasText:name})});
    const remove=row.getByRole('button',{name:'Xóa',exact:true});if(await remove.count()) await remove.click();
    await expect(row.getByRole('button',{name:'Hoàn tác',exact:true})).toBeVisible();
   }
   await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();
   await expect(page.getByLabel('Xem trước triển khai')).toBeVisible({timeout:120_000});
   await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
   await Promise.race([
    expect(page.getByLabel('Kết quả triển khai')).toContainText('Triển khai thành công',{timeout:20*60_000}),
    page.locator('.review-section-status').filter({hasText:/^INTERRUPTED/}).waitFor({state:'visible',timeout:20*60_000}).then(()=>{throw Error('Removal interrupted; recover the original held operation before retrying');}),
   ]);
   expect(JSON.parse(kube('get','deployments','-n',state.namespace,'-o','json')).items).toHaveLength(0);
   state.workloadRemovalPassed=true;writeFileSync(statePath,JSON.stringify(state,null,2),{mode:0o600});
   console.log('PASS UI workload removal; Terraform dependencies retained for separate application cleanup');
  }
  for(const kind of ['RoleBinding','Role']) {
   const raw=kube('get',kind,'orch-reconciliation-workloads','-n',state.namespace,'--ignore-not-found','-o','json');if(!raw.trim())continue;
   const object=JSON.parse(raw);expect(object.metadata.uid).toBe(state.platformRBAC[kind]);expect(object.metadata.labels['orchestrator.test/run']).toBe(state.runID);
   execFileSync('kubectl',['--kubeconfig',join(run,'kubeconfig'),'delete','--raw',`/apis/rbac.authorization.k8s.io/v1/namespaces/${state.namespace}/${kind.toLowerCase()}s/orch-reconciliation-workloads`,'-f','-'],{input:JSON.stringify({apiVersion:'v1',kind:'DeleteOptions',preconditions:{uid:object.metadata.uid}}),stdio:['pipe','pipe','pipe']});
  }
  await page.goto(`${config.baseURL}/ui/applications/${state.appID}/settings?environment=staging`);
  if (body.application.status === 'DELETING') {
   await page.getByRole('region',{name:'Tiến trình xóa ứng dụng',exact:true}).getByRole('button',{name:'Thử lại',exact:true}).click();
  } else {
   await page.getByRole('region',{name:'Xóa ứng dụng',exact:true}).getByRole('button',{name:'Xóa ứng dụng',exact:true}).click();
  }
  const dialog=page.getByRole('dialog',{name:'Xóa ứng dụng',exact:true});
  await expect(dialog.getByRole('region',{name:'staging',exact:true})).toContainText('Workload sẽ bị gỡ: 0');
  await expect(dialog.getByRole('region',{name:'production',exact:true})).toContainText('Tài nguyên sẽ bị xóa: 0');
  await dialog.getByRole('textbox').fill(state.appName);
  await page.screenshot({path:join(run,'ui-evidence','deletion-confirmation.png'),fullPage:true});
  await dialog.getByRole('button',{name:'Xóa vĩnh viễn ứng dụng',exact:true}).click();
  await expect(page.getByRole('heading',{name:'Ứng dụng của bạn',exact:true})).toBeVisible({timeout:20*60_000});
  await expect(page.getByRole('status').filter({hasText:`Đã xóa ứng dụng ${state.appName}.`})).toBeVisible();
 }
 expect((await page.request.get(`${config.baseURL}/api/v1/applications/${state.appID}`)).status()).toBe(404);
 expect(kube('get','namespace',state.namespace,'--ignore-not-found','-o','name').trim()).toBe('');
 await expect.poll(()=>JSON.parse(kube('get','persistentvolumes','-o','json')).items.filter(p=>p.spec.claimRef?.namespace===state.namespace).length,{timeout:120_000,intervals:[2000]}).toBe(0);
 const parent=JSON.parse(kube('get','gitrepo','orch-reconciliation-existing','-n','fleet-local','-o','json'));
 expect(parent.metadata.uid).toBe(state.existingGitRepoUID);expect(parent.spec).toEqual(state.existingGitRepoSpec);
 const sibling=JSON.parse(kube('get','configmap','orch-reconciliation-sibling','-n','orch-recon-sibling','-o','json'));expect(sibling.metadata.uid).toBe(state.siblingUID);
 expect(sibling.data.message).toBe('retained company sibling');
 state.cleanupPassed=true;writeFileSync(statePath,JSON.stringify(state,null,2),{mode:0o600});
 await page.screenshot({path:join(run,'ui-evidence','deletion-pass.png'),fullPage:true});console.log('PASS UI application deletion; namespace/PVs absent; existing GitRepo and sibling retained');
} catch(error){await page.screenshot({path:join(run,'ui-evidence',`cleanup-failure-${Date.now()}.png`),fullPage:true}).catch(()=>{});console.error(error.message);process.exitCode=1;}finally{await closeConsole(session,config)}
