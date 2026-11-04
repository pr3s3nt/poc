import { readPrivateFile } from './console.mjs';
import { conflict, digest } from './runtime.mjs';
import { requireConfig } from './bootstrap-config.mjs';
export const envBase=(app,env)=>`/applications/${encodeURIComponent(app)}/environments/${env}`;
export async function selectCatalog(ui,base,sourceId,tag,allowedPrevious,rt) {
 const current=await ui.get(base+'/catalog');
 const desired=current.catalog.desired;
 if(desired?.sourceId===sourceId&&desired.tag===tag){rt.record('environment-catalog','REUSED',{tag});return;}
 conflict(!desired || allowedPrevious?.some(v=>v.sourceId===desired.sourceId&&v.tag===desired.tag),'Environment Catalog pin');
 const option=current.releases.find(v=>v.sourceId===sourceId&&v.tag===tag);
 requireConfig(option&&!option.blockers?.length,'Catalog target release missing/blocked');
 const panel=ui.page.getByRole('region',{name:'Catalog',exact:true});
 await panel.getByRole('combobox').selectOption(JSON.stringify([sourceId,tag]));
 await ui.submit(base+'/catalog','PUT',panel.getByRole('button',{name:'Lưu lựa chọn',exact:true}));
 const after=(await ui.get(base+'/catalog')).catalog.desired;
 conflict(after?.sourceId===sourceId&&after.tag===tag,'Catalog selection read-back');rt.record('environment-catalog','UPDATED',{tag});
}
export async function selectDefinitionRevisions(ui,base,revisions,rt) {
 for(const [key,id]of Object.entries(revisions??{})) {
  const view=await ui.get(base+'/definition-versions');const row=view.definitions?.find(v=>v.key===key);
  requireConfig(row&&row.versions.some(v=>v.id===id&&!v.archived),'Definition target revision missing/archived');
  if(row.desiredRevisionId===id){rt.record(`definition:${key}`,'REUSED',{revision:id});continue;}
  // Selection is an explicit upgrade operation; product validates pin/artifact/data guards.
  const details=ui.page.locator('details').filter({has:ui.page.locator('summary strong').filter({hasText:'Phiên bản tài nguyên'})}).first();
  if(!await details.getAttribute('open'))await details.locator('summary').click();
  const section=details.locator('section').filter({has:ui.page.getByRole('heading',{name:key,exact:true})});
  await section.getByRole('combobox').selectOption(id);
  await ui.submit(base+'/definition-versions','PUT',section.getByRole('button',{name:'Chọn để nâng cấp',exact:true}));
  conflict((await ui.get(base+'/definition-versions')).definitions.find(v=>v.key===key)?.desiredRevisionId===id,'Definition selection read-back');
  rt.record(`definition:${key}`,'UPDATED',{revision:id});await ui.page.reload();
 }
}
export async function ensureEnvironment(ui,config,app,store,version,rt,{upgrade=false}={}) {
 const env=config.application.environment,base=envBase(app.key,env);
 const allowedKeys=new Set([...Object.keys(config.application.variables),...Object.keys(config.application.secretFiles),'ACCEPTANCE_SECRET_SHA256']);
 conflict((await ui.get(base+'/configuration')).keys.every(v=>allowedKeys.has(v.name)),'Unmanaged configuration keys/pending changes');
 await ui.page.goto(`${ui.baseURL}/ui/applications/${app.key}/settings?environment=${env}`);
 const current=(await ui.get('/applications/'+app.key)).application.environments.find(v=>v.key===env);
 conflict(!current.secretStoreKey||current.secretStoreKey===store.key,'Environment Secret Store');
 if(!current.secretStoreKey){
  const label=env==='production'?'Production':'Staging';
  await ui.page.getByLabel(`Kho bí mật cho ${label}`,{exact:true}).selectOption(store.key);
  await ui.submit(base+'/secret-store','PUT',ui.page.getByRole('button',{name:'Lưu kho bí mật',exact:true}));
  conflict((await ui.get('/applications/'+app.key)).application.environments.find(v=>v.key===env).secretStoreKey===store.key,'Secret Store selection read-back');rt.record('environment-store','UPDATED');
 }else rt.record('environment-store','REUSED');
 await ui.page.reload();
 if(config.schemaVersion===3) conflict(!(await ui.get(base+'/catalog')).catalog.desired,'Direct acceptance must not have a Catalog');
 else await selectCatalog(ui,base,config.platform.catalog.id,version.catalogTag,upgrade?[{sourceId:config.platform.catalog.id,tag:config.application.catalogTag}]:[],rt);
 await selectDefinitionRevisions(ui,base,version.definitionRevisions,rt);
 const vars=Object.fromEntries(Object.entries(config.application.variables).map(([k,v])=>[k,v.replaceAll('${applicationKey}',app.key).replaceAll('${environment}',env)]));
 const secrets=Object.fromEntries(Object.entries(config.application.secretFiles).map(([k,v])=>[k,readPrivateFile(v,'application secret')]));
 // Hash is derived from the configured persistent secret, never generated per run.
 if(secrets.ACCEPTANCE_SECRET)vars.ACCEPTANCE_SECRET_SHA256=digest(secrets.ACCEPTANCE_SECRET);
 for(const [name,value]of Object.entries({...vars,...secrets})) {
  const kind=Object.hasOwn(secrets,name)?'SECRET':'VARIABLE';
  const currentConfig=await ui.get(base+'/configuration');const found=(currentConfig.keys??[]).find(v=>v.name===name);
  rt.state.secretHashes??={};
  if(found) {
   conflict(found.kind===kind,'Configuration key kind');
   conflict(kind==='SECRET' ? found.configured&&rt.state.secretHashes[name]===digest(value) : found.value===value,`Configuration key ${name}`);
   rt.record(`key:${name}`,'REUSED');continue;
  }
  if(kind==='SECRET'){rt.state.secretHashes[name]=digest(value);rt.save();}
  const section=ui.page.locator(`section[aria-label="${kind==='SECRET'?'Bí mật':'Biến'}"]`);
  await section.getByRole('button',{name:kind==='SECRET'?'+ Thêm bí mật':'+ Thêm biến',exact:true}).click();
  await ui.page.getByLabel('Tên key',{exact:true}).fill(name);
  await ui.page.getByLabel(kind==='SECRET'?'Giá trị bí mật mới':'Giá trị',{exact:true}).fill(value);
  await ui.submit(base+'/configuration/keys/'+encodeURIComponent(name),'PUT',ui.page.getByRole('button',{name:'Lưu thay đổi chờ triển khai',exact:true}));
  const saved=(await ui.get(base+'/configuration')).keys.find(v=>v.name===name);
  conflict(saved?.kind===kind&&(kind==='SECRET'?saved.configured:saved.value===value),'Configuration read-back');
  rt.record(`key:${name}`,'CREATED');
 }
 return base;
}
