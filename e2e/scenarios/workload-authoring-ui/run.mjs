import assert from 'node:assert/strict';
import {writeFileSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
const app=process.env.ORCH_APPLICATION_KEY,env=process.env.ORCH_ENVIRONMENT??'staging',dir=process.env.ORCH_TEST_RUN_DIR;
assert(app&&dir);assert(['staging','production'].includes(env));outsideRepo(dir);
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,30000),p=s.page,base=`/applications/${app}/environments/${env}`;
try{
 const before=await ui.get(base+'/workloads');
 await p.goto(c.baseURL+`/ui/applications/${app}?environment=${env}`);
 const add=p.getByRole('button',{name:'+ Thêm Workload',exact:true});
 await add.click();await p.getByRole('dialog').getByRole('button',{name:'Hủy',exact:true}).click();await p.getByRole('dialog').waitFor({state:'hidden',timeout:5000});
 console.log('PASS picker Cancel');
 await add.click();await p.keyboard.press('Escape');await p.getByRole('dialog').waitFor({state:'hidden',timeout:5000});
 console.log('PASS picker Escape/reopen');
 await add.click();await p.getByRole('dialog').getByRole('button',{name:/^Tự cấu hình/}).click();await p.getByLabel('Tên Workload',{exact:true}).waitFor();
 await p.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).waitFor();await p.getByRole('button',{name:'+ Thêm cổng',exact:true}).waitFor();
 await p.getByLabel('Tên Workload',{exact:true}).fill('endpoint-ui-check');await p.getByRole('button',{name:'+ Thêm cổng',exact:true}).click();
 await p.getByLabel('Tên cổng Service',{exact:true}).fill('http');await p.getByLabel('Cổng Service',{exact:true}).fill('8080');assert.equal(await p.getByLabel('Tên endpoint',{exact:true}).count(),0);
 await p.getByRole('button',{name:'Tạo endpoint từ cổng http',exact:true}).click();assert.equal(await p.getByLabel('Tên endpoint',{exact:true}).inputValue(),'endpoint-ui-check');
 await p.getByLabel('Đường dẫn endpoint',{exact:true}).fill('/api');assert.equal(await p.getByLabel('Cổng endpoint',{exact:true}).inputValue(),'http');
 await p.screenshot({path:dir+'/workload-endpoint-form.png',fullPage:true});
 await p.getByRole('button',{name:'Hủy',exact:true}).click();await p.waitForURL(c.baseURL+`/ui/applications/${app}?environment=${env}`);
 const after=await ui.get(base+'/workloads');assert.equal(after.draftVersion,before.draftVersion);assert.deepEqual(after.workloads,before.workloads);
 writeFileSync(dir+'/workload-authoring-ui.json',JSON.stringify({checkedAt:new Date().toISOString(),app,environment:env,pickerCancel:true,pickerEscape:true,reopenCustom:true,basicGroupsVisible:true,explicitEndpoint:true,cancelPreservesEnvironment:true,draftWrites:0},null,2),{mode:0o600});
 console.log('PASS Cancel/Escape/reopen, default groups, endpoint fields and unchanged drafts');
}catch(e){await p.screenshot({path:dir+'/workload-authoring-error.png',fullPage:true});throw e;}finally{await closeConsole(s,c);}
