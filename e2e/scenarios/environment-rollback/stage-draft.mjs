// Prepare reversible pending edits; only UI controls perform business writes.
import assert from 'node:assert/strict';
import {mkdirSync,writeFileSync} from 'node:fs';
import {outsideRepo,makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {readConsoleConfig,openConsole,closeConsole} from '../../lib/console.mjs';
const app=process.env.ORCH_ROLLBACK_APP,dir=process.env.ORCH_TEST_RUN_DIR;
assert(app&&dir&&process.env.ORCH_ROLLBACK_STAGE==='1','Explicit application, evidence directory and ORCH_ROLLBACK_STAGE=1 required');
outsideRepo(dir);mkdirSync(dir,{recursive:true,mode:0o700});
const config=readConsoleConfig(),session=await openConsole(config,{role:'ADMIN'});
const ui=makeUI(session.page,config.baseURL,180000),page=session.page;
const base=`/applications/${encodeURIComponent(app)}/environments/staging`;
try{
 const current=(await ui.get('/applications/'+encodeURIComponent(app))).application;
 assert(!current.environments.find(e=>e.key==='staging')?.activeOperation,'Environment must be idle');
 const before=await ui.get(base+'/workloads');assert.equal(before.workloads.length,11);
 assert(before.workloads.every(w=>!w.state),'Existing drafts require inspection');
 const beforeConfig=await ui.get(base+'/configuration');
 assert(!beforeConfig.keys.some(k=>k.name==='ROLLBACK_REVIEW_MARKER'),'Marker already exists; inspect prior run');
 writeFileSync(dir+'/before-stage.json',JSON.stringify({workloads:before,configuration:beforeConfig},null,2),{mode:0o600});
 await page.goto(config.baseURL+`/ui/applications/${app}/settings?environment=staging`);
 const variables=page.locator('section[aria-label="Biến"]');
 await variables.getByRole('button',{name:'+ Thêm biến',exact:true}).click();
 await page.getByLabel('Tên key',{exact:true}).fill('ROLLBACK_REVIEW_MARKER');
 await page.getByLabel('Giá trị',{exact:true}).fill('issue49-pending-edit');
 await ui.submit(base+'/configuration/keys/ROLLBACK_REVIEW_MARKER','PUT',page.getByRole('button',{name:'Lưu thay đổi chờ triển khai',exact:true}));
 await page.goto(config.baseURL+`/ui/applications/${app}/environments/staging/workloads/loadgenerator`);
 const original=before.workloads.find(w=>w.id==='loadgenerator').workload.containers.main.image;
 await page.getByLabel('Ảnh Container',{exact:true}).fill(original+'-pending-rollback-test');
 await ui.submit(base+'/workloads/loadgenerator','PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 const after=await ui.get(base+'/workloads');
 assert.equal(after.workloads.find(w=>w.id==='loadgenerator').workload.containers.main.image,original+'-pending-rollback-test');
 assert((await ui.get(base+'/configuration')).keys.some(k=>k.name==='ROLLBACK_REVIEW_MARKER'));
 await page.screenshot({path:dir+'/staged-draft.png',fullPage:true});
 console.log('PASS staged workload/configuration edits through UI; no Deploy submitted');
}catch(error){await page.screenshot({path:dir+'/stage-error.png',fullPage:true}).catch(()=>{});console.error(error.message);process.exitCode=1}
finally{await closeConsole(session,config)}
