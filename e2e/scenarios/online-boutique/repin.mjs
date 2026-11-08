import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
// Explicit renderer upgrade after a rebuilt backend. All writes are UI controls.
import assert from 'node:assert/strict';import{readFileSync,writeFileSync}from'node:fs';
import{openConsole,closeConsole,readConsoleConfig}from'../../lib/console.mjs';
import{makeUI}from'../../../scripts/bootstrap/lib/runtime.mjs';
import{selectDefinitionRevisions}from'../../../scripts/bootstrap/lib/environment.mjs';
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);const file=dir+'/state.json',state=JSON.parse(readFileSync(file));const rt={state,save(){writeFileSync(file,JSON.stringify(state,null,2),{mode:0o600})},record(k,v){console.log(k,v)}};
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000),{page}=ui,key='boutique-workload';
try{
if(process.env.ORCH_SELECT_ONLY!=='1'){
await page.goto(c.baseURL+'/ui/platform/resource-definitions');
await page.getByRole('button',{name:'Sửa cấu hình '+key,exact:true}).click();
await page.getByRole('button',{name:'Đổi nguồn',exact:true}).click();
await page.getByRole('textbox',{name:'Git HTTPS URL',exact:true}).fill('https://github.com/pr3s3nt/orchestrator-workload-templates');
await page.getByLabel('Thư mục template',{exact:true}).fill('templates/standard');
await page.getByLabel('Tag hoặc commit',{exact:true}).filter({visible:true}).fill('v0.3.0');
const source=await ui.submit('/template-sources/inspect','POST',page.getByRole('button',{name:'Kiểm tra template',exact:true}));state.source=source.templateSource;rt.save();
await ui.submit('/resource-definitions/'+key,'PUT',page.getByRole('button',{name:'Lưu thay đổi',exact:true}));
}
const history=await ui.get('/resource-definitions/'+key+'/versions');console.log('Version response keys',Object.keys(history));
const revision=history.history.versions.filter(v=>!v.archived).at(-1);assert(revision);
await page.goto(c.baseURL+`/ui/applications/${state.app.key}/settings?environment=staging`);
await selectDefinitionRevisions(ui,`/applications/${state.app.key}/environments/staging`,{[key]:revision.id},rt);
console.log('PASS renderer Definition updated and selected through UI');
}catch(e){await page.screenshot({path:dir+'/repin-error.png',fullPage:true});console.error(e.message);process.exitCode=1}finally{await closeConsole(s,c)}
