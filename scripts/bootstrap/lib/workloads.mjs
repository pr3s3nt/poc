import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { parse, stringify } from 'yaml';
import { conflict, equal } from './runtime.mjs';
import { envBase } from './environment.mjs';
export function desiredScores(config,version) {
 return Object.fromEntries(['backend','worker','frontend'].map(name=>{
  const raw=readFileSync(config.application.scores[name],'utf8').replace(/@@(\w+)_IMAGE@@/g,(_,w)=>version.images[w.toLowerCase()]);
  const workload=parse(raw);if(config.application.workloadClass)workload.metadata.class=config.application.workloadClass;
  return [name,workload];
 }));
}
export function workloadAction(existing,desired,previous) {
 if(!existing)return 'CREATE';
 conflict(existing.state!=='PENDING_DELETE','Workload pending deletion');
 if(equal(existing.workload,desired))return 'REUSE';
 conflict(previous&&equal(existing.workload,previous),'Workload differs from both baseline and upgrade target');
 return 'UPDATE';
}
export async function ensureWorkloads(ui,config,app,version,rt,{upgrade=false}={}) {
 const base=envBase(app.key,config.application.environment);
 const wanted=desiredScores(config,version),previous=desiredScores(config,config.application);
 const workloads=(await ui.get(base+'/workloads')).workloads;
 conflict(workloads.every(v=>Object.hasOwn(wanted,v.id)),'Unexpected workload in acceptance Application');
 // Compare all workloads before any mutation.
 const decisions=Object.entries(wanted).map(([name,workload])=>[name,workload,workloadAction(workloads.find(v=>v.id===name),workload,upgrade?previous[name]:undefined)]);
 for(const [name,workload,action]of decisions){
  if(action==='REUSE'){rt.record(`workload:${name}`,'REUSED');continue;}
  await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}/environments/${config.application.environment}/workloads/${action==='CREATE'?'new':encodeURIComponent(name)}`);
  const custom=ui.page.getByRole('button',{name:/Tự cấu hình \/ nhập Workload/});if(await custom.count())await custom.click();
  await ui.page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
  const file=join(config.runtime.stateDir,`${name}.json`);writeFileSync(file,JSON.stringify(workload),{mode:0o600});
  await ui.page.getByLabel('File Workload',{exact:true}).setInputFiles(file);
  await ui.page.getByText(`Đã phân tích Workload: ${name}`,{exact:false}).waitFor();
  await ui.submit(base+'/workloads/'+name,'PUT',ui.page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
  conflict(equal((await ui.get(base+'/workloads')).workloads.find(v=>v.id===name)?.workload,workload),'Workload read-back');
  rt.record(`workload:${name}`,action==='CREATE'?'CREATED':'UPDATED');
 }
}
