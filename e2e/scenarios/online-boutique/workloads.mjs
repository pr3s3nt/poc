import { outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
import assert from 'node:assert/strict';import {readFileSync,writeFileSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
const directory=process.env.ORCH_TEST_RUN_DIR;if(!directory)throw Error('ORCH_TEST_RUN_DIR required');
outsideRepo(directory);
const stateFile=directory+'/state.json',state=JSON.parse(readFileSync(stateFile));
const fixture=JSON.parse(readFileSync(new URL('./fixtures/workloads.json',import.meta.url)));
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,180000),{page}=ui;
const app=state.app.key,base=`/applications/${app}/environments/staging`;
const save=()=>writeFileSync(stateFile,JSON.stringify(state,null,2),{mode:0o600});
async function advanced(){while(await page.locator('button[data-workload-advanced][aria-expanded=false]').count())await page.locator('button[data-workload-advanced][aria-expanded=false]').first().click()}
async function group(title){await advanced();const section=page.locator('details.runtime-config-group').filter({has:page.locator('summary strong').getByText(title,{exact:true})});if(await section.count()){if(!await section.getAttribute('open').then(x=>x!==null))await section.locator('summary').first().click();return section;}throw Error('Missing configuration group '+title)}
try {
 const application=(await ui.get('/applications/'+app)).application;const env=application.environments.find(e=>e.key==='staging');
 if(!env.secretStoreKey){const store=(await ui.get('/secret-stores')).secretStores.find(s=>s.name==='Boutique runtime Vault');assert(store);await page.goto(c.baseURL+`/ui/applications/${app}/settings?environment=staging`);await page.getByLabel('Kho bí mật cho Staging',{exact:true}).selectOption(store.key);await ui.submit(base+'/secret-store','PUT',page.getByRole('button',{name:'Lưu kho bí mật',exact:true}));}
 const existing=(await ui.get(base+'/workloads')).workloads;
 for(const w of fixture.workloads){
  const prior=existing.find(x=>x.id===w.name);
  if(prior){
   const main=prior.workload.containers.main;
   assert.equal(main.image,w.image,w.name+' existing image');
   assert.deepEqual(main.resources,w.resources,w.name+' existing resources');
   for(const [key,value]of Object.entries(w.variables))assert.equal(main.variables[key],value,w.name+'/'+key);
   if(w.redis)assert.equal(main.variables.REDIS_ADDR,'${resources.redis-cart.connection_string}');
   assert.deepEqual(prior.workload.service,w.service,w.name+' existing Service');
   console.log('Workload reused and checked',w.name);continue;
  }
  await page.goto(c.baseURL+`/ui/applications/${app}?environment=staging`);
  await page.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();
  await page.getByRole('button',{name:/Backend API/}).click();
  await page.getByLabel('Tên Workload',{exact:true}).fill(w.name);await page.getByLabel('Ảnh Container',{exact:true}).fill(w.image);
  if(w.redis){const dependencies=await group('Tài nguyên phụ thuộc');await dependencies.getByRole('button',{name:'+ Thêm tài nguyên',exact:true}).click();await dependencies.getByLabel('Alias tài nguyên',{exact:true}).fill('redis-cart');await dependencies.getByRole('combobox',{name:'Loại tài nguyên',exact:true}).selectOption('boutique-cache');}
  let i=0;
  for(const [name,value]of Object.entries({...w.variables,...(w.redis?{REDIS_ADDR:null}:{})})){
   i++;await page.getByRole('button',{name:'+ Thêm biến',exact:true}).click();
   const row=page.locator('tr[data-row]').nth(i-1);await row.locator('td input').first().fill(name);
   if(value===null){await row.getByRole('button',{name:`Chọn nguồn, dòng ${i}`,exact:true}).click();await page.getByRole('button',{name:/^redis-cart\.connection_string/}).click();}
   else await row.getByRole('textbox').nth(1).fill(value);
  }
  const compute=await group('Tài nguyên tính toán');
  for(const [label,value]of [['Yêu cầu CPU',w.resources.requests.cpu],['Yêu cầu bộ nhớ',w.resources.requests.memory],['Giới hạn CPU',w.resources.limits.cpu],['Giới hạn bộ nhớ',w.resources.limits.memory]])await compute.getByLabel(label,{exact:true}).fill(value);
  if(w.service){const networking=await group('Service & kết nối');for(const [name,p]of Object.entries(w.service.ports)){await networking.getByRole('button',{name:'+ Thêm cổng',exact:true}).click();await networking.getByLabel('Tên cổng Service',{exact:true}).last().fill(name);await networking.locator('input[type=number]').last().fill(String(p.port));if(p.targetPort!==p.port){const advancedPort=networking.getByRole('button',{name:new RegExp('Hiện.*'+name)});if(await advancedPort.count())await advancedPort.click();await networking.getByLabel('Cổng đích của Container',{exact:true}).fill(String(p.targetPort));}}}
  if(w.httpHealth){const health=await group('Kiểm tra sức khỏe');for(const title of ['Readiness HTTP path','Liveness HTTP path']){const field=health.locator('fieldset').filter({has:page.locator('legend').getByText(title,{exact:true})});await field.getByRole('checkbox').check();await field.getByLabel('Cổng kiểm tra',{exact:true}).fill('8080');await field.getByLabel('Đường dẫn HTTP',{exact:true}).fill('/_healthz');}}
  await ui.submit(base+'/workloads/'+w.name,'PUT',page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
  state.workloads??=[];state.workloads.push(w.name);save();console.log('Workload saved through form',w.name);
 }
 console.log('PASS all eleven workloads authored through Console form');
}catch(e){await page.screenshot({path:directory+'/workload-error.png',fullPage:true});console.error('Workload UI error',e.message);process.exitCode=1}finally{await closeConsole(s,c)}
