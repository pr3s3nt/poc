import { readFileSync } from 'node:fs';
import { readPrivateFile } from './console.mjs';
import { assertSameResourceType } from './catalog-config.mjs';
import { ensureGitAccess } from './git-access-ui.mjs';
import { ensureInfrastructureAccount } from './infrastructure-ui.mjs';
import { ensureCatalog } from './catalog-ui.mjs';
import { conflict, equal } from './runtime.mjs';
import { BootstrapError, requireConfig } from './bootstrap-config.mjs';

export async function ensureTypes(ui,types,rt) {
 const existing=(await ui.get('/resource-types')).resourceTypes;
 for(const type of types) {
  const found=existing.find(v=>v.key===type.key);
  if(found){assertSameResourceType(found,type);rt.record(`type:${type.key}`,'REUSED');continue;}
  const {page}=ui;
  await page.goto(ui.baseURL+'/ui/platform/resource-types');
  const form=page.locator('form').filter({has:page.getByLabel('ID loại tài nguyên',{exact:true})});
  await form.getByLabel('ID loại tài nguyên',{exact:true}).fill(type.key);
  for(const [kind,label,noun] of [['inputs','Đầu vào','tham số đầu vào'],['outputs','Đầu ra','tham số đầu ra']])for(const [i,f]of type[kind].entries()) {
   await form.getByRole('button',{name:`+ Thêm ${noun}`,exact:true}).click();
   const prefix=`${label} ${i+1}`;
   await form.getByLabel(prefix+' tên',{exact:true}).fill(f.name);
   await form.getByLabel(prefix+' kiểu',{exact:true}).selectOption(f.type);
   await form.getByLabel(prefix+' bắt buộc',{exact:true}).setChecked(f.required);
   if(kind==='outputs')await form.getByLabel(prefix+' bí mật',{exact:true}).setChecked(f.secret);
  }
  await ui.submit('/resource-types','POST',form.getByRole('button',{name:'Đăng ký loại tài nguyên',exact:true}));
  assertSameResourceType((await ui.get('/resource-types')).resourceTypes.find(v=>v.key===type.key),type);
  rt.record(`type:${type.key}`,'CREATED');
 }
}
export function assertStore(store,want) {
 conflict(['name','backendAddress','workloadAddress','mount','authMount'].every(k=>store[k]===want[k]),'Secret Store');
 conflict(store.status==='READY' && store.verification?.verified===true && store.verification?.kubernetesAuth==='CONFIGURED','Secret Store readiness/Kubernetes auth');
 conflict(Boolean(store.tlsCustomCa)===Boolean(want.tlsCaPemFile),'Secret Store CA');
}
export async function ensureSecretStore(ui,want,rt) {
 let stores=(await ui.get('/secret-stores')).secretStores??[];
 const matches=stores.filter(v=>v.name===want.name);conflict(matches.length<=1,'Secret Store ambiguous');
 if(matches.length){assertStore(matches[0],want);rt.record('secret-store','REUSED',{key:matches[0].key});return matches[0];}
 const {page}=ui;
 await page.goto(ui.baseURL+'/ui/platform/secret-stores');
 for(const [label,key]of [['Tên kho','name'],['Địa chỉ workload','workloadAddress']])await page.getByLabel(label,{exact:true}).fill(want[key]);
 await page.getByRole('textbox',{name:'Địa chỉ backend'}).fill(want.backendAddress);
 await page.getByRole('button',{name:'Hiện nâng cao',exact:true}).click();
 await page.getByLabel('Mount KV v2',{exact:true}).fill(want.mount);
 await page.getByLabel('Mount xác thực Kubernetes',{exact:true}).fill(want.authMount);
 if(want.tlsCaPemFile)await page.getByLabel('CA PEM',{exact:true}).fill(readFileSync(want.tlsCaPemFile,'utf8'));
 await page.getByLabel('Token Vault',{exact:true}).fill(readPrivateFile(want.tokenFile,'Vault token'));
 await ui.submit('/secret-stores','POST',page.getByRole('button',{name:'Xác minh và đăng ký',exact:true}));
 stores=(await ui.get('/secret-stores')).secretStores;
 const saved=stores.find(v=>v.name===want.name);requireConfig(saved,'Secret Store save unconfirmed');assertStore(saved,want);
 rt.record('secret-store','CREATED',{key:saved.key});return saved;
}
export async function ensureDelivery(ui,want,account,rt) {
 const providers=(await ui.get('/delivery-providers')).providers;
 const provider=providers.find(v=>v.kind===want.provider);requireConfig(provider,'CD provider unavailable');
 requireConfig(Object.keys(want.config).every(k=>provider.fields.some(f=>f.key===k&&!f.readOnly)),'Unknown or read-only CD config field');
 const bindings=(await ui.get('/delivery-bindings')).bindings??[];
 const matches=bindings.filter(v=>v.name===want.name);
 conflict(matches.length<=1,'CD binding ambiguous');
 let binding=matches[0];
 const {page}=ui;
 if(binding) {
  const details=await ui.get(`/delivery-bindings/${binding.id}?version=${binding.version}`);
  conflict(binding.provider===want.provider && details.managementAccountKey===account.key && details.managementAccountVersion===account.version && Boolean(details.usePlatformGit)===want.usePlatformGit,'CD binding');
  conflict(Object.entries(want.config).every(([k,v])=>details.config[k]===v),'CD provider config');
 } else {
  await page.goto(ui.baseURL+'/ui/platform/cd-providers');
  const inspected=ui.responseFor(`/delivery-providers/${want.provider}/inspect`,'POST');
  await page.getByRole('button',{name:'Thêm cấu hình CD',exact:true}).click();
  const form=page.locator('form.secret-store-form');
  await form.locator('input').first().fill(want.name);
  if(await form.locator('#cd-provider-kind').count()) await form.locator('#cd-provider-kind').selectOption(want.provider);
  await form.locator('#cd-management-account').selectOption(account.key);
  const discovered=await inspected;requireConfig(discovered.ok(),'Fleet discovery failed');
  const result=await discovered.json();
  const eligible=(result.options?.targetCluster??[]).filter(v=>!v.disabled);
  requireConfig(eligible.length===1 || want.config.targetCluster,'Ambiguous or unsupported Fleet discovery; specify workspace/targetCluster');
  if(await form.locator('details summary').count()) await form.locator('details summary').click();
  for(const [key,value]of Object.entries(want.config)) {
   const field=form.locator(`#cd-${key}`);
   if(await field.count()) { if(await field.evaluate(n=>n.tagName)==='SELECT')await field.selectOption(value);else await field.fill(value); }
  }
  const git=form.locator('#cd-gitConnectionKey');
  if(want.usePlatformGit) await git.selectOption('__platform');
  else if(want.config.gitConnectionKey) await git.selectOption(want.config.gitConnectionKey);
  else {
   await git.selectOption('__new');
   await form.getByLabel('Tên kết nối Git',{exact:true}).fill(want.name+' Git');
   await form.getByLabel('URL phạm vi Git (tài khoản/nhóm)',{exact:true}).fill(want.writerURL);
   await form.getByLabel('Tên đăng nhập Git',{exact:true}).fill(want.writerUsername);
   await form.getByLabel('Token hoặc mật khẩu Git',{exact:true}).fill(readPrivateFile(want.writerPasswordFile,'CD credential'));
   await ui.submit('/connections/git','POST',form.getByRole('button',{name:'Lưu kết nối Git',exact:true}));
  }
  binding=await ui.submit('/delivery-bindings','POST',form.getByRole('button',{name:'Lưu cấu hình',exact:true}));
  rt.record('delivery-binding','CREATED',{id:binding.id,version:binding.version});
 }
 if(binding.status!=='READY') {
  await page.goto(ui.baseURL+'/ui/platform/cd-providers');
  const row=page.locator('tbody tr').filter({has:page.getByRole('cell',{name:binding.name,exact:true})});
  binding=await ui.submit(`/delivery-bindings/${binding.id}/verify`,'POST',row.getByRole('button',{name:'Xác minh',exact:true}));
 }
 requireConfig(binding.status==='READY',`CD verification failed: ${binding.reasonCode??'UNKNOWN'}`);
 const current=await ui.get('/delivery-default');
 if(want.setDefault && (current.bindingId!==binding.id || current.bindingVersion!==binding.version)) {
  conflict(!current.bindingId,'Existing platform CD default');
  await page.goto(ui.baseURL+'/ui/platform/cd-providers');
  const row=page.locator('tbody tr').filter({has:page.getByRole('cell',{name:binding.name,exact:true})});
  await ui.submit('/delivery-default','PUT',row.getByRole('button',{name:'Dùng cho ứng dụng mới',exact:true}));
  const after=await ui.get('/delivery-default');conflict(after.bindingId===binding.id && after.bindingVersion===binding.version,'CD default read-back');
  rt.record('delivery-default','UPDATED');
 } else rt.record('delivery-default','REUSED');
 return binding;
}
export async function bootstrapPlatform(ui,config,rt) {
 const p=config.platform;
 const beforeGit=(await ui.get('/platform/settings/git')).settings;
 await ensureGitAccess({...ui,desired:p.gitAccess,update:false});rt.record('git-access',p.gitAccess?(beforeGit.version?'REUSED':'CREATED'):'SKIPPED');
 const beforeAccounts=(await ui.get('/driver-accounts')).accounts;
 const account=await ensureInfrastructureAccount({...ui,account:p.account,kubeconfigFile:p.account.kubeconfigFile});rt.record('infrastructure-account',beforeAccounts.some(v=>v.key===account.key)?'REUSED':'CREATED',{key:account.key});
 await ensureTypes(ui,p.resourceTypes,rt);
 const beforeSources=(await ui.get('/platform/catalog/sources')).sources;
 await ensureCatalog({...ui,source:p.catalog,requiredTags:[config.application.catalogTag,...(config.upgrade.enabled?[config.upgrade.catalogTag]:[])]});rt.record('catalog',beforeSources.some(v=>v.id===p.catalog.id)?'REUSED':'CREATED');
 const store=await ensureSecretStore(ui,p.secretStore,rt);
 const binding=await ensureDelivery(ui,p.delivery,account,rt);
 return {account,store,binding};
}
