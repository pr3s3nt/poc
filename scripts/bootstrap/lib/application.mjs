import { conflict, waitIdle } from './runtime.mjs';
import { requireConfig } from './bootstrap-config.mjs';
export async function ensureApplication(ui,config,rt) {
 const want=config.application;
 const apps=(await ui.get('/applications')).applications;
 const candidates=apps.filter(v=>v.name===want.name||v.key===rt.state.appKey||v.key===want.expectedKey);
 conflict(candidates.length<=1,'Application identity');
 let app=candidates[0];
 if(app) {
  conflict(app.name===want.name && (app.status??'ACTIVE')==='ACTIVE','Application');
  conflict((want.expectedKey??rt.state.appKey)===app.key || rt.state.applicationIntent===true,'Application ownership; specify expectedKey to reuse on another machine');
  if(want.expectedKey)conflict(app.key===want.expectedKey,'Application expectedKey');
  rt.record('application','REUSED',{key:app.key});
 } else {
  requireConfig(config.runtime.mode!=='upgrade','Upgrade requires an existing baseline Application');
  conflict(!want.expectedKey&&!rt.state.appKey,'Previously recorded Application missing');
  rt.state.applicationIntent=true;rt.save();
  await ui.page.goto(ui.baseURL+'/ui/applications/new');
  await ui.page.getByLabel('Tên ứng dụng',{exact:true}).fill(want.name);
  const result=await ui.submit('/applications','POST',ui.page.getByRole('button',{name:'Tạo ứng dụng',exact:true}));
  app=result.application;
  requireConfig(app?.key,'Application creation unconfirmed');
  const actual=(await ui.get('/applications/'+app.key)).application;
  conflict(actual.name===want.name,'Application read-back');
  rt.record('application','CREATED',{key:app.key});
 }
 rt.state.appKey=app.key;rt.state.applicationIntent=false;rt.save();
 return app;
}
