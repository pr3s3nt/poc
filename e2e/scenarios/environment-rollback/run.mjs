import assert from 'node:assert/strict';
import {writeFileSync,mkdirSync,existsSync,readFileSync} from 'node:fs';
import {outsideRepo,makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {readConsoleConfig,openConsole,closeConsole} from '../../lib/console.mjs';

const dir=process.env.ORCH_TEST_RUN_DIR,app=process.env.ORCH_ROLLBACK_APP;
const environment=process.env.ORCH_ROLLBACK_ENV??'staging';
assert(dir&&app,'ORCH_TEST_RUN_DIR and ORCH_ROLLBACK_APP required');
assert(['staging','production'].includes(environment));outsideRepo(dir);mkdirSync(dir,{recursive:true,mode:0o700});
const config=readConsoleConfig();const session=await openConsole(config,{role:'ADMIN'});
const ui=makeUI(session.page,config.baseURL,2400000);const page=session.page;
const base=`/applications/${encodeURIComponent(app)}/environments/${environment}`;
try{
 const data=(await ui.get('/applications/'+encodeURIComponent(app))).application;
 assert(!data.environments.find(item=>item.key===environment)?.activeOperation,'Inspect/recover the prior operation before rollback');
 await page.goto(config.baseURL+`/ui/applications/${encodeURIComponent(app)}?environment=${environment}`);
 const preview=await ui.submit(base+'/rollback-preview','POST',page.getByRole('button',{name:'Xem trước Rollback',exact:true}));
 writeFileSync(dir+'/rollback-preview.json',JSON.stringify(preview,null,2),{mode:0o600});
 if(existsSync(dir+'/before-stage.json')){const baseline=JSON.parse(readFileSync(dir+'/before-stage.json'));if(baseline.applicationKey)assert.equal(baseline.applicationKey,app);if(baseline.targetOperationId)assert.equal(preview.targetOperationId,baseline.targetOperationId,'Partial per-workload successes must not replace the Environment baseline');}
 await page.getByRole('dialog',{name:'Xác nhận Rollback môi trường'}).waitFor();
 assert(Array.isArray(preview.restore)&&Array.isArray(preview.remove));
 await page.screenshot({path:dir+'/rollback-review.png',fullPage:true});
 if(process.env.ORCH_ROLLBACK_CONFIRM!=='1'){
  console.log('PASS rollback preview only; no confirmation submitted');
 }else{
  await page.getByRole('textbox',{name:`Nhập ${app} để xác nhận`,exact:true}).fill(app);
  const report=await ui.submit(base+'/rollback','POST',page.getByRole('button',{name:'Rollback môi trường',exact:true}));
  writeFileSync(dir+'/rollback-result.json',JSON.stringify(report,null,2),{mode:0o600});
  assert.equal(report.status,'SUCCEEDED');
  const after=(await ui.get('/applications/'+encodeURIComponent(app))).application;
  assert(!after.environments.find(item=>item.key===environment)?.activeOperation,'Successful rollback still holds the claim');
  await page.getByText('Đã khôi phục toàn bộ môi trường.',{exact:true}).waitFor();
  await page.screenshot({path:dir+'/rollback-complete.png',fullPage:true});
  console.log('PASS explicit whole-Environment rollback through Console');
 }
}catch(error){await page.screenshot({path:dir+'/rollback-error.png',fullPage:true}).catch(()=>{});console.error(error.message);process.exitCode=1}
finally{await closeConsole(session,config)}
