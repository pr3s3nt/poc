import assert from 'node:assert/strict';
import {parseArgs} from 'node:util';
import {readFileSync,writeFileSync,existsSync,mkdirSync} from 'node:fs';
import {join} from 'node:path';
import {loadBootstrapConfig} from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import {openConsole,closeConsole} from '../../lib/console.mjs';
import {outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
const {values:a}=parseArgs({options:{config:{type:'string'},evidence:{type:'string'},phase:{type:'string'},prometheus:{type:'string'},alertmanager:{type:'string'},grafana:{type:'string'},'cluster-uid':{type:'string'}}});
for(const k of ['config','evidence','phase'])assert(a[k],`Missing --${k}`);
assert(['connection','binding'].includes(a.phase));outsideRepo(a.evidence);mkdirSync(a.evidence,{recursive:true,mode:0o700});
const c=loadBootstrapConfig(a.config);const identity=JSON.parse(readFileSync(join(c.runtime.stateDir,'identity.json')));assert(identity.appKey,'Requires bootstrap-owned app');
const statePath=join(a.evidence,'monitoring.json'),state=existsSync(statePath)?JSON.parse(readFileSync(statePath)):{};
if(state.appKey)assert.equal(state.appKey,identity.appKey);state.appKey=identity.appKey;
const save=()=>writeFileSync(statePath,JSON.stringify(state,null,2),{mode:0o600});
const ui=await openConsole(c.console,{role:'PLATFORM_ENGINEER'});
try{
 if(a.phase==='connection'){
  if(state.connectionId){const r=await ui.page.request.get(c.console.baseURL+'/api/v1/monitoring/connections');assert(r.ok());const list=(await r.json()).connections;assert(list.some(x=>x.id===state.connectionId));console.log('REUSED connection checkpoint');}
  else{
   for(const k of ['prometheus','alertmanager','grafana','cluster-uid'])assert(a[k],`Missing --${k}`);
   await ui.page.goto(c.console.baseURL+'/ui/platform/settings');
   const section=ui.page.getByRole('region',{name:'Kết nối giám sát'});await section.getByLabel('Tên',{exact:true}).fill('Slice 1 dev acceptance');
   for(const [label,key]of [['Prometheus endpoint','prometheus'],['Alertmanager endpoint','alertmanager'],['Grafana URL','grafana'],['UID cluster','cluster-uid']])await section.getByLabel(label,{exact:true}).fill(a[key]);
   await section.getByText('Cấu hình cảnh báo',{exact:true}).click();await section.getByLabel('Thời gian thiếu replicas',{exact:true}).fill('30');
   const reply=ui.page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/api/v1/monitoring/connections'));await section.getByRole('button',{name:'Lưu kết nối',exact:true}).click();const r=await reply;assert(r.ok(),`Connection create ${r.status()}`);state.connectionId=(await r.json()).id;save();
  }
  await ui.page.goto(c.console.baseURL+'/ui/platform/settings');const section=ui.page.getByRole('region',{name:'Kết nối giám sát'});
  const reply=ui.page.waitForResponse(r=>r.url().endsWith(`/monitoring/connections/${state.connectionId}/test`));await section.getByRole('button',{name:'Kiểm tra',exact:true}).click();const r=await reply;assert(r.ok(),`Connection test ${r.status()}`);state.coverage=await r.json();assert(state.coverage.connectivity&&state.coverage.metrics&&state.coverage.cluster);save();
  await ui.page.screenshot({path:join(a.evidence,'connection.png'),fullPage:true});console.log('PASS connection: real query coverage and cluster identity');
 }else{
  assert(state.connectionId&&state.coverage?.cluster);await ui.page.goto(c.console.baseURL+`/ui/applications/${identity.appKey}/settings?environment=${c.application.environment}`);
  const section=ui.page.locator('section.content-panel').filter({has:ui.page.getByRole('heading',{name:'Giám sát môi trường',exact:true})});await section.getByRole('combobox').selectOption(state.connectionId);
  const reply=ui.page.waitForResponse(r=>r.request().method()==='PUT'&&r.url().endsWith('/monitoring/binding'));await section.getByRole('button',{name:'Gắn kết nối',exact:true}).click();const r=await reply;assert(r.ok(),`Binding ${r.status()}`);state.binding=await r.json();assert.equal(state.binding.connectionId,state.connectionId);assert.equal(state.binding.application,identity.appKey);save();await ui.page.screenshot({path:join(a.evidence,'binding.png'),fullPage:true});console.log('PASS verified Environment binding');
 }
}finally{await closeConsole(ui,c.console)}
