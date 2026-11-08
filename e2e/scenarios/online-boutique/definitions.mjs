import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';import {readFileSync,writeFileSync,existsSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureTypes} from '../../../scripts/bootstrap/lib/platform.mjs';
import {openDefinition} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
process.umask(0o077);const directory=process.env.ORCH_TEST_RUN_DIR;if(!directory)throw Error('ORCH_TEST_RUN_DIR required');
outsideRepo(directory);
const file=directory+'/state.json',state=JSON.parse(readFileSync(file));
const rt={state,save(){writeFileSync(file,JSON.stringify(state,null,2))},record(key,status,value){state[key]=value??status;this.save();console.log(key,status)}};
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000),{page}=ui;
const repository='https://github.com/pr3s3nt/orchestrator-workload-templates',ref='v0.3.0';
try {
await ensureTypes(ui,[{key:'boutique-cache',inputs:[],outputs:[{name:'connection_string',type:'string',required:true,secret:true}]}],rt);
let definitions=(await ui.get('/resource-definitions')).resourceDefinitions;
for(const [key,type,folder]of [['boutique-cluster','k8s-cluster',null],['boutique-namespace','k8s-namespace','namespace'],['boutique-cache','boutique-cache','redis'],['boutique-workload','workload','standard']]){
const prior=definitions.find(d=>d.key===key);
if(prior){assert.equal(prior.resourceType,type);assert.equal(prior.driverType,type==='k8s-cluster'?'existing-cluster':type==='workload'?'template':'terraform');if(type==='k8s-cluster')assert.equal(prior.driverAccount,state.account.key);console.log('Definition reused',key);continue;}
await openDefinition(ui,type);
if(type==='k8s-cluster'){
 await page.getByRole('combobox',{name:/^Cách cung cấp cluster/}).selectOption('existing-cluster');
 await page.getByRole('combobox',{name:/^Tài khoản/}).selectOption(state.account.key);
}else{
const template=type==='workload';
const group=template?page.getByRole('group',{name:'Thêm template từ Git',exact:true}):page.locator('section').filter({has:page.getByRole('textbox',{name:'Git HTTPS URL',exact:true})}).last();
await page.getByRole('textbox',{name:'Git HTTPS URL',exact:true}).fill(repository);
await page.getByLabel(template?'Thư mục template':'Thư mục module',{exact:true}).fill((template?'templates/':'terraform/')+folder);
await page.getByLabel('Tag hoặc commit',{exact:true}).filter({visible:true}).fill(ref);
const prepared=await ui.submit(template?'/template-sources/inspect':'/terraform-modules/inspect','POST',page.getByRole('button',{name:template?'Kiểm tra template':'Kiểm tra module',exact:true}));
if(template){state.source=prepared.templateSource;rt.save();await page.getByLabel('Tham chiếu namespace',{exact:true}).fill("${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}");}
else{
 await page.getByRole('button',{name:'Tham số: JSON nâng cao',exact:true}).click();
 const variables=type==='k8s-namespace'?{name:'${context.env.namespace}'}:{namespace:"${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}"};
 await page.getByLabel('Tham số cấu hình (JSON object)',{exact:true}).fill(JSON.stringify(variables));
 await page.locator('details.definition-advanced > summary').click();await page.getByRole('button',{name:'Nâng cao: JSON nâng cao',exact:true}).click();
 const mappings={input_mapping:{},output_mapping:type==='k8s-namespace'?{name:'name'}:{connection_string:'connection_string'},provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.default#environments.@app.@env'}}};
 await page.getByLabel('Ánh xạ module JSON',{exact:true}).fill(JSON.stringify(mappings));
}
}
await page.getByLabel('ID cấu hình (Definition ID)',{exact:true}).fill(key);await page.getByLabel('Tên hiển thị',{exact:true}).fill(key);
const saved=await ui.submit('/resource-definitions','POST',page.getByRole('button',{name:'Đăng ký cấu hình tài nguyên',exact:true}).first());console.log('Saved Definition via UI',key);
}
if(!state.app){
 await page.goto(c.baseURL+'/ui/applications/new');await page.getByLabel('Tên ứng dụng',{exact:true}).fill(process.env.ORCH_BOUTIQUE_APP_NAME||'boutique-issue47');
 const r=await ui.submit('/applications','POST',page.getByRole('button',{name:'Tạo ứng dụng',exact:true}));state.app=r.application;rt.save();console.log('Created Application via UI',state.app.key);
}
console.log('PASS UI Definitions and Application creation');
}catch(e){await page.screenshot({path:directory+'/definitions-error.png',fullPage:true});console.error('Definition UI error',e.message);process.exitCode=1}finally{await closeConsole(s,c)}
