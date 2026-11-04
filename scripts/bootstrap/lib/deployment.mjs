import { conflict, waitIdle } from './runtime.mjs';
import { envBase } from './environment.mjs';
import { ensureNamespaceAccess } from './namespace-access.mjs';
import { requireConfig } from './bootstrap-config.mjs';
export function hasPending(preview) {
 return Boolean(preview.changes?.length || preview.catalogPending || preview.deliveryPending || preview.routePending || preview.deliveryPolicyOnly);
}
export async function ensureDeployment(ui,config,app,rt) {
 const env=config.application.environment,base=envBase(app.key,env);
 await waitIdle(ui,app.key,env,config.runtime.timeoutMs);
 await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}?environment=${env}`);
 const preview=await ui.submit(base+'/preview','POST',ui.page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 if(config.schemaVersion===3 && rt.state.acceptanceSnapshot)conflict(!hasPending(preview),'Pending changes on previously verified acceptance app');
 if(!hasPending(preview)){await retainResources(ui,base,rt);rt.record('deployment','SKIPPED');return;}
 conflict((preview.changes??[]).every(v=>['backend','worker','frontend'].includes(v.workloadId)&&v.action!=='REMOVE'),'Unexpected Preview changes');
 rt.record('preview','REUSED',{workloads:(preview.changes??[]).map(v=>v.workloadId)});
 const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){const body=request.postDataJSON();rt.state.deployAttempt={reviewRequestId:body?.reviewRequestId,completion:'UNKNOWN'};rt.save();}};
 ui.page.on('request',track);
 let report;
 try {
  const submission=ui.submit(base+'/deploy','POST',ui.page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}));
  [report]=await Promise.all([submission,ensureNamespaceAccess(ui,config,app,rt)]);
  if(rt.state.deployAttempt){rt.state.deployAttempt.completion=report.status==='SUCCEEDED'?'SUCCEEDED':'FAILED';rt.save();}
 }finally{ui.page.off('request',track);}
 requireConfig(report.status==='SUCCEEDED','Deployment incomplete; inspect Console, do not automatically replay');
 await waitIdle(ui,app.key,env,config.runtime.timeoutMs);
 const workloads=(await ui.get(base+'/workloads')).workloads;
 conflict(workloads.length===3&&workloads.every(v=>!v.state&&v.ready),'Deployment workload read-back');
 await retainResources(ui,base,rt);rt.record('deployment','UPDATED');
}

async function retainResources(ui,base,rt) {
 const resources=(await ui.get(base+'/resources')).resources??[];
 const ids=resources.map(v=>v.id).sort();
 requireConfig(ids.every(v=>typeof v==='string'),'Resource IDs unavailable');
 if(rt.state.resourceIds)conflict(rt.state.resourceIds.every(id=>ids.includes(id)),'Resource identities changed');
 rt.state.resourceIds=ids;rt.save();
}
