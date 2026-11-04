import { conflict, equal, digest } from './runtime.mjs';
import { requireConfig } from './bootstrap-config.mjs';

export function sameDefinitionPin(recorded, definition) {
 return recorded.version===definition.version && (recorded.sourceFingerprint??null)===(definition.sourceFingerprint??null) && (recorded.revisionId===undefined||recorded.revisionId===definition.revisionId) && (recorded.generation===undefined||recorded.generation===definition.generation);
}
export function samePin(actual, desired) {
 return actual && Object.entries(desired).every(([key,value])=>actual[key]===value);
}
export async function readDefinitionPin(ui, definition) {
 const {history}=await ui.get(`/resource-definitions/${encodeURIComponent(definition.key)}/versions`);
 const active=history.versions.find(v=>v.id===history.activeId);
 conflict(active&&!active.archived&&equal(definitionDocument(active.definition),definitionDocument(definition)),`Definition ${definition.key} active revision differs`);
 return {version:definition.version,sourceFingerprint:definition.sourceFingerprint??null,revisionId:history.activeId,generation:history.generation};
}
export function sourceMatches(actual, spec) {
 if(!samePin(actual,spec.pin))return false;
 if(spec.kind==='installed')return true;
 if(actual.kind!==spec.kind)return false;
 if(spec.kind==='git')return actual.url===spec.url&&actual.path===spec.path;
 if(spec.kind==='zip')return actual.path===(spec.path??'.');
 return actual.path==='.';
}
export function definitionDocument(definition) {
 return {key:definition.key,name:definition.name,resourceType:definition.resourceType,driverType:definition.driverType,...(definition.driverAccount?{driverAccount:definition.driverAccount}:{}),driverInputs:definition.driverInputs,provision:definition.provision??{},criteria:definition.criteria};
}
export function resolveDefinition(entry, accountKey, sources) {
 const definition=structuredClone(entry.definition);
 if(definition.driverAccount==='${account.key}')definition.driverAccount=accountKey;
 if(definition.driverType==='template') {
  const variables=definition.driverInputs.values.variables;
  const name=variables.template_source.match(/^\$\{templateSources\.([a-z0-9-]+)\}$/)?.[1];
  requireConfig(sources[name], 'Template source unresolved');variables.template_source=sources[name].id;
 }
 return definitionDocument(definition);
}
export async function openDefinition(ui,type) {
 await ui.page.goto(ui.baseURL+'/ui/platform/resource-definitions?resourceType='+encodeURIComponent(type));
 const id=ui.page.getByLabel('ID cấu hình (Definition ID)',{exact:true});
 const create=ui.page.getByRole('button',{name:'Tạo cấu hình',exact:true});
 await id.or(create).first().waitFor({state:'visible'});
 if(!await id.isVisible())await create.click();
 await id.waitFor({state:'visible'});
 await ui.page.locator('.definition-create-identity select').selectOption(type);
}
export async function submitRegistration(ui,rt,key,path,button) {
 requireConfig(!rt.state.pendingRegistrations?.[key],'Unknown registration result; inspect server before retry');
 const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+path){rt.state.pendingRegistrations??={};rt.state.pendingRegistrations[key]={path,inputHash:digest(request.postData()??'')};rt.save();}};
 ui.page.on('request',track);
 try{return await ui.submit(path,'POST',button);}catch(error){if(error.status>=400&&error.status<500)confirmRegistration(rt,key);throw error;}finally{ui.page.off('request',track);}
}
export function confirmRegistration(rt,key) {if(rt.state.pendingRegistrations?.[key]){delete rt.state.pendingRegistrations[key];rt.save();}}
async function prepareSource(ui,source,rt,{template=false,type}={}) {
 const registration=template?`template:${source.name}`:`module:${type}`;
 const {page}=ui;
 const prefix=template?'/template-sources':'/terraform-modules';
 if(source.kind==='installed')throw new Error('Installed source missing');
 if(source.kind==='git') {
  await page.getByRole('button',{name:template?'Git':'Thêm từ Git',exact:true}).click();
  await page.getByLabel('Git HTTPS URL',{exact:true}).filter({visible:true}).fill(source.url);
  await page.getByLabel(template?'Thư mục template':'Thư mục module',{exact:true}).fill(source.path);
  await page.getByLabel('Tag hoặc commit',{exact:true}).filter({visible:true}).fill(source.ref);
  return submitRegistration(ui,rt,registration,prefix+'/inspect',page.getByRole('button',{name:template?'Kiểm tra template':'Kiểm tra module',exact:true}));
 }
 if(source.kind==='zip') {
  await page.getByRole('button',{name:template?'ZIP':'Tải ZIP',exact:true}).click();
  await page.getByLabel('Tệp ZIP',{exact:true}).setInputFiles(source.file);
  await page.getByLabel('Thư mục trong ZIP',{exact:true}).fill(source.path??'.');
  return submitRegistration(ui,rt,registration,prefix+'/upload',page.getByRole('button',{name:'Tải lên và kiểm tra',exact:true}));
 }
 requireConfig(template&&source.kind==='inline','Unsupported source preparation');
 await page.getByRole('button',{name:'Nhập trực tiếp',exact:true}).click();
 for(const [name,content]of Object.entries(source.files).filter(([name])=>name.endsWith('.tpl'))) {
  if(!await page.getByLabel(name,{exact:true}).count()) {await page.getByLabel('Tên file template',{exact:true}).fill(name);await page.getByRole('button',{name:'Thêm template',exact:true}).click();}
  await page.getByLabel(name,{exact:true}).fill(content);
 }
 await page.getByText('Input schema và mặc định',{exact:true}).click();
 await page.getByLabel('Input schema (JSON)',{exact:true}).fill(source.files['inputs.schema.json']);
 await page.getByLabel('Mặc định (YAML)',{exact:true}).fill(source.files['defaults.yaml']);
 return submitRegistration(ui,rt,registration,'/template-sources/inline',page.getByRole('button',{name:'Kiểm tra template',exact:true}));
}
export async function ensureTemplateSources(ui,config,rt) {
 const result={};
 for(const source of config.platform.templateSources) {
  let matches=(await ui.get('/template-sources')).templateSources.filter(s=>sourceMatches(s.source,source));
  const recorded=rt.state.templateSources?.[source.name];
  if(recorded) {conflict(samePin(recorded.pin,source.pin),'Template input pin changed');matches=matches.filter(s=>s.id===recorded.id);}
  conflict(matches.length<=1,'Ambiguous template source pin');
  let found=matches[0];
  if(!found) {
   conflict(!recorded,'Recorded template source missing');
   requireConfig(source.kind!=='installed','Pinned template source unavailable');
   await openDefinition(ui,'workload');
   const prepared=await prepareSource(ui,source,rt,{template:true});found=prepared.templateSource;
   conflict(sourceMatches(found?.source,source),'Prepared template source pin');
   found=(await ui.get('/template-sources')).templateSources.find(s=>s.id===found.id);
   rt.record(`template-source:${source.name}`,'CREATED',{id:found?.id,pin:source.pin});
  }else rt.record(`template-source:${source.name}`,'REUSED',{id:found.id,pin:source.pin});
  requireConfig(found?.status==='AVAILABLE'&&sourceMatches(found.source,source),'Template source unavailable after read-back');
  confirmRegistration(rt,`template:${source.name}`);
  rt.state.templateSources??={};rt.state.templateSources[source.name]={id:found.id,pin:source.pin};rt.save();result[source.name]=found;
 }
 return result;
}
async function fillCriteria(page,criteria) {
 await page.getByRole('tab',{name:/^Điều kiện áp dụng/}).click();
 for(const [i,rule]of criteria.entries()) {
  if(i>0)await page.getByRole('button',{name:'+ Thêm trường hợp áp dụng',exact:true}).click();
  for(const [j,[field,value]]of Object.entries(rule).entries()) {
   await page.getByRole('button',{name:'+ Thêm điều kiện',exact:true}).nth(i).click();
   await page.getByRole('combobox',{name:`Trường hợp ${i+1} thuộc tính ${j+1}`,exact:true}).selectOption(field);
   const control=page.getByRole('textbox',{name:new RegExp(`^Điều kiện ${i+1} `)}).nth(j);
   await control.fill(value);
  }
 }
}
async function fillTemplateInputs(page,source,inputs) {
 for(const [name,value]of Object.entries(inputs)) {
  // Locate the schema field by its stable data-input identity; title is user-authored.
  const row=page.locator(`[data-input="${name}"]`);
  const schema=source.inputsSchema?.properties?.[name];requireConfig(schema,'Unknown template input');
  if(schema.type==='boolean')await row.getByRole('checkbox').setChecked(value);
  else if(schema.enum)await row.getByRole('combobox').selectOption(JSON.stringify(value));
  else await row.locator('input,textarea').fill(typeof value==='string'?value:JSON.stringify(value));
 }
}
export async function ensureDirectDefinitions(ui,config,account,sources,rt) {
 const entries=config.platform.definitions.map(entry=>({...entry,desired:resolveDefinition(entry,account.key,sources)}));
 const existing=(await ui.get('/resource-definitions')).resourceDefinitions;
 // Detect every existing Definition conflict before creating another Definition.
 for(const entry of entries) {
  const found=existing.find(d=>d.key===entry.desired.key);
  if(found) {conflict(equal(definitionDocument(found),entry.desired),`Definition ${entry.desired.key}`);const previous=rt.state?.definitionPins?.[found.key];if(previous)conflict(sameDefinitionPin(previous,await readDefinitionPin(ui,found)),`Definition ${found.key} revision changed`);}
 }
 for(const entry of entries) {
  const want=entry.desired;
  let found=existing.find(d=>d.key===want.key);
  if(found){confirmRegistration(rt,`definition:${want.key}`);rt.record(`definition:${want.key}`,'REUSED',{version:found.version,sourceFingerprint:found.sourceFingerprint});continue;}
  await openDefinition(ui,want.resourceType);
  const {page}=ui;
  if(want.driverType==='existing-cluster')await page.getByRole('combobox',{name:/^Cách cung cấp cluster/}).selectOption('existing-cluster');
  if(want.driverType==='terraform') {
   const capabilities=(await ui.get('/resource-definition-capabilities')).capabilities;
   const matches=capabilities.filter(c=>c.resourceType===want.resourceType&&c.driverType==='terraform'&&samePin(c.source,entry.source.pin));
   conflict(matches.length<=1,'Ambiguous Terraform source');
   if(entry.source.kind==='installed'||rt.state.pendingRegistrations?.[`module:${want.resourceType}`]&&matches.length===1) {
    confirmRegistration(rt,`module:${want.resourceType}`);
    await page.getByRole('button',{name:'Module đã đăng ký',exact:true}).click();
    requireConfig(matches.length===1,'Pinned Terraform module unavailable');
    const select=page.getByRole('combobox',{name:/^Module đã kiểm chứng/});
    const options=await select.locator('option').evaluateAll(nodes=>nodes.map(n=>({value:n.value,text:n.textContent})));
    const option=options.find(o=>o.value.includes(entry.source.pin.contentSha256));
    if(!await select.isDisabled()){requireConfig(option,'Pinned module option missing');await select.selectOption(option.value);}
   } else {
    const response=await prepareSource(ui,entry.source,rt,{type:want.resourceType});
    conflict(samePin(response.capability?.source,entry.source.pin),'Prepared Terraform source pin');confirmRegistration(rt,`module:${want.resourceType}`);
   }
   await page.getByRole('button',{name:'Tham số: JSON nâng cao',exact:true}).click();
   await page.getByLabel('Tham số cấu hình (JSON object)',{exact:true}).fill(JSON.stringify(want.driverInputs.values.variables??{}));
   await page.locator('details.definition-advanced > summary').click();
   await page.getByRole('button',{name:'Nâng cao: JSON nâng cao',exact:true}).click();
   const values=want.driverInputs.values;
   await page.getByLabel('Ánh xạ module JSON',{exact:true}).fill(JSON.stringify({input_mapping:values.input_mapping??{},output_mapping:values.output_mapping??{},provider_bindings:values.provider_bindings??{}}));
   await page.getByLabel('Quy tắc tạo tài nguyên liên quan (JSON object)',{exact:true}).fill(JSON.stringify(want.provision));
  }
  if(want.driverAccount)await page.getByRole('combobox',{name:/^Tài khoản/}).selectOption(want.driverAccount);
  if(want.driverType==='template') {
   const values=want.driverInputs.values.variables;
   await page.getByRole('combobox',{name:/^Nguồn đã xác thực/}).selectOption(values.template_source);
   await fillTemplateInputs(page,Object.values(sources).find(s=>s.id===values.template_source),values.template_inputs??{});
   await page.getByLabel('Tham chiếu namespace',{exact:true}).fill(values.namespace??'');
   await page.getByLabel('Tham chiếu cluster (khi cần)',{exact:true}).fill(values.cluster??'');
   await page.getByRole('textbox',{name:/^Tham chiếu output \(JSON\)/}).fill(JSON.stringify(values.resource_references??{}));
   if(Object.keys(want.provision).length){await page.locator('details').filter({has:page.getByText('Tài nguyên đi kèm',{exact:true})}).locator('summary').click();await page.getByLabel('Quy tắc tạo tài nguyên liên quan (JSON object)',{exact:true}).fill(JSON.stringify(want.provision));}
  }
  await page.getByLabel('ID cấu hình (Definition ID)',{exact:true}).fill(want.key);
  await page.getByLabel('Tên hiển thị',{exact:true}).fill(want.name);
  await fillCriteria(page,want.criteria);
  const review=page.locator('details.definition-review');await review.locator('summary').click();
  const document=JSON.parse(await review.locator('pre').innerText());
  conflict(equal(definitionDocument(document),want),'Definition form differs from declared input');
  await submitRegistration(ui,rt,`definition:${want.key}`,'/resource-definitions',page.getByRole('button',{name:'Đăng ký cấu hình tài nguyên',exact:true}));
  found=(await ui.get('/resource-definitions')).resourceDefinitions.find(d=>d.key===want.key);
  conflict(found&&equal(definitionDocument(found),want),'Definition read-back');
  confirmRegistration(rt,`definition:${want.key}`);
  rt.record(`definition:${want.key}`,'CREATED',{version:found.version,sourceFingerprint:found.sourceFingerprint});
 }
 const registered=(await ui.get('/resource-definitions')).resourceDefinitions.filter(d=>entries.some(e=>e.desired.key===d.key));
 rt.state.definitionPins=Object.fromEntries(await Promise.all(registered.map(async d=>[d.key,await readDefinitionPin(ui,d)])));rt.save();
 for(const [key,pin]of Object.entries(rt.state.definitionPins))rt.record(`definition-pin:${key}`,'REUSED',pin);
}

export async function preflightDirect(ui,config,rt) {
 const capabilities=(await ui.get('/resource-definition-capabilities')).capabilities;
 requireConfig(capabilities.some(c=>c.resourceType==='workload'&&c.driverType==='template'),'Runtime must support schema 8 Generic Workload/template driver');
 const registered=(await ui.get('/template-sources')).templateSources;
 const sources={};
 for(const source of config.platform.templateSources) {
  const matches=registered.filter(s=>sourceMatches(s.source,source));
  conflict(matches.length<=1,'Ambiguous prepared template source');
  if(matches.length){requireConfig(matches[0].status==='AVAILABLE','Pinned template source unavailable');sources[source.name]=matches[0];}
  if(source.kind==='installed')requireConfig(matches.length===1,'Installed template source missing');
 }
 for(const entry of config.platform.definitions)if(entry.source?.kind==='installed') {
  requireConfig(capabilities.some(c=>c.resourceType===entry.definition.resourceType&&c.driverType==='terraform'&&samePin(c.source,entry.source.pin)),'Installed Terraform source pin unavailable');
 }
 const accounts=(await ui.get('/driver-accounts')).accounts;
 const account=accounts.find(a=>a.name===config.platform.account.name);
 const existing=(await ui.get('/resource-definitions')).resourceDefinitions;
 for(const entry of config.platform.definitions) {
  const found=existing.find(d=>d.key===entry.definition.key);if(!found)continue;
  const needsAccount=entry.definition.driverAccount==='${account.key}';
  if(needsAccount)requireConfig(account,'Definition account identity unavailable');
  const desired=resolveDefinition(entry,account?.key,sources);
  conflict(equal(definitionDocument(found),desired),`Definition ${desired.key}`);
  const recorded=rt.state.definitionPins?.[found.key];
  if(recorded)conflict(sameDefinitionPin(recorded,await readDefinitionPin(ui,found)),`Definition ${found.key} revision changed`);
 }
 const defaultBinding=await ui.get('/delivery-default');
 const bindings=(await ui.get('/delivery-bindings')).bindings;
 const selected=bindings.find(b=>b.name===config.platform.delivery.name);
 if(config.platform.delivery.setDefault&&defaultBinding.bindingId)conflict(selected&&defaultBinding.bindingId===selected.id&&defaultBinding.bindingVersion===selected.version,'Existing platform CD default');
 rt.record('direct-source-definition-pins','REUSED');
}
