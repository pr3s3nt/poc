import { requireConfig } from './bootstrap-config.mjs';
import { waitIdle } from './runtime.mjs';
import { envBase } from './environment.mjs';
import { ensureNamespaceAccess } from './namespace-access.mjs';
// Only a response-confirmed failed deployment owned by this runner can recover automatically.
// Unknown/lost requests do not prove that the previous executor stopped.
export async function recoverCompletedDeploy(ui,config,app,rt) {
 const base=envBase(app.key,config.application.environment);
 const ops=(await ui.get(base+'/operations')).operations;
 const held=ops.find(v=>['ACTIVE','INTERRUPTED','RECOVERING'].includes(v.status));
 if(held?.status==='INTERRUPTED') {
  const attempt=rt.state.deployAttempt;
  requireConfig(config.runtime.recoverCompletedDeploy && attempt?.completion==='FAILED' && attempt.reviewRequestId===held.reviewRequestId && held.recoverable,'Interrupted/unknown operation needs explicit operator recovery; previous execution not proven stopped');
  await ensureNamespaceAccess(ui,config,app,rt,{wait:false});
  await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}/settings?environment=${config.application.environment}`);
  await ui.page.getByRole('checkbox',{name:'Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn',exact:true}).check();
  const result=await ui.submit(base+`/operations/${held.id}/recover`,'POST',ui.page.getByRole('button',{name:'Khôi phục môi trường',exact:true}));
  requireConfig(!result.recovery?.compensationFailed?.length,'Recovery incomplete; inspect Console');
  rt.record('recovery','UPDATED',{operationId:held.id});
 }
 await waitIdle(ui,app.key,config.application.environment,config.runtime.timeoutMs);
}
