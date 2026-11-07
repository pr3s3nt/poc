import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);const file=dir+'/state.json',state=JSON.parse(readFileSync(file));
assert(['PARTIAL','UNKNOWN'].includes(state.deployAttempt?.completion),'Only inspect/recover a recorded partial or unknown attempt after confirming the stopped operation in Console');
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000);
try{
s.page.on('response',async r=>{if(r.url().endsWith('/recover')&&!r.ok())console.error('Recovery response',JSON.stringify(await r.json()))});
const base=`/applications/${state.app.key}/environments/staging`,app=(await ui.get('/applications/'+state.app.key)).application;
const op=app.environments.find(e=>e.key==='staging').activeOperation;assert.equal(op?.status,'INTERRUPTED');
await s.page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings?environment=staging`);
await s.page.getByRole('checkbox',{name:'Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn',exact:true}).check();
const result=await ui.submit(base+`/operations/${op.id}/recover`,'POST',s.page.getByRole('button',{name:'Khôi phục môi trường',exact:true}));
writeFileSync(dir+'/recovery.json',JSON.stringify(result,null,2),{mode:0o600});assert(!result.recovery.compensationFailed?.length);
const after=(await ui.get('/applications/'+state.app.key)).application.environments.find(e=>e.key==='staging');assert(!after.activeOperation);
state.previousAttempts??=[];state.previousAttempts.push(state.deployAttempt);delete state.deployAttempt;writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600});
console.log('PASS stopped partial operation recovered through UI');
}catch(e){console.error(e.message);process.exitCode=1}finally{await closeConsole(s,c)}
