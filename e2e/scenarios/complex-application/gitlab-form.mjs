import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
const vi=JSON.parse(readFileSync(new URL('../../../frontend/public/locales/vi.json',import.meta.url),'utf8'));
const text=(area,key)=>vi[`workloads.${area}.${key}`];
const exact=name=>({name,exact:true});
const escape=value=>value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
async function expand(page){while(await page.locator('button[data-workload-advanced][aria-expanded=false]').count())await page.locator('button[data-workload-advanced][aria-expanded=false]').first().click();}
async function group(page,scope,title){await expand(page);const g=scope.locator('details.runtime-config-group').filter({has:page.locator('summary strong').getByText(title,{exact:true})});assert.equal(await g.count(),1,'Group '+title);if(await g.getAttribute('open')===null)await g.locator('summary').click();return g;}
export async function authorGitLabWorkload(ui,base,home,spec){
 const {page}=ui;
 await page.goto(home.replace(/\?.*/,'')+`/environments/staging/workloads/new`);
 await page.getByLabel('Tên Workload',{exact:true}).fill(spec.name);
 await page.getByLabel('Ảnh Container',{exact:true}).first().fill(spec.containers[0].image);
 await expand(page);
 const pod=await group(page,page,text('defaults','podSecurity'));
 for(const key of ['runAsUser','runAsGroup','fsGroup'])await pod.getByLabel(key,{exact:true}).fill(String(spec.uid));
 await pod.getByRole('combobox',exact('runAsNonRoot')).selectOption('true');
 await group(page,page,text('defaults','infrequent'));
 await page.getByRole('combobox',exact(text('defaults','controller'))).selectOption(spec.kind);
 if(spec.kind==='Job'){await page.getByLabel('Số lần thử lại khi thất bại',{exact:true}).fill('0');await page.getByLabel('Thời gian chạy tối đa (giây)',{exact:true}).fill('1200');}
 if(spec.service){const net=await group(page,page,'Service & kết nối');await net.getByRole('combobox',exact('Service')).selectOption('true');await net.getByRole('button',exact('+ Thêm cổng')).click();await net.getByLabel('Tên cổng Service',{exact:true}).last().fill(spec.service.name);await net.locator('input[type=number]').last().fill(String(spec.service.port));}
 for(let index=0;index<spec.containers.length;index++){
  const c=spec.containers[index];if(index)await page.getByRole('button',exact('+ Thêm Container')).click();
  await page.getByLabel('Tên Container',{exact:true}).nth(index).fill(c.name);
  await page.getByLabel('Ảnh Container',{exact:true}).nth(index).fill(c.image);
  const section=page.locator('section.content-panel').filter({has:page.getByRole('heading',exact(`Container ${index+1}`))});
  const startup=await group(page,section,text('defaults','startup'));
  if(c.role==='init'){await startup.getByRole('combobox',exact(`Vai trò container ${c.name}`)).selectOption('init');await startup.getByLabel(text('containers','initOrder'),{exact:true}).fill(String(c.order));}
  for(const[key,label]of[['command','command'],['args','args']])if(c[key]?.length){assert(c[key].every(a=>!a.includes('\n')),'Form arguments are newline-separated');await startup.getByLabel(text('templates',label),{exact:true}).fill(c[key].join('\n'));}
  const compute=await group(page,section,text('defaults','compute'));
  const limit=c.resources?.limits?.memory??(c.name==='dependencies'?'1Gi':c.role==='init'?'256Mi':c.name==='webservice'?'4Gi':c.name==='sidekiq'?'3Gi':c.name==='toolbox'||c.name==='migrations'?'2Gi':'1Gi');
  await compute.getByLabel('Giới hạn bộ nhớ',{exact:true}).fill(limit);
  for(const[k,label]of[['cpu','Yêu cầu CPU'],['memory','Yêu cầu bộ nhớ']])if(c.resources?.requests?.[k])await compute.getByLabel(label,{exact:true}).fill(c.resources.requests[k]);
  for(const[key,value]of Object.entries(c.env??{})){
   await section.getByRole('button',exact('+ Thêm biến')).click();const row=section.locator('tr[data-row]').last();await row.locator('td input').first().fill(key);
   if(typeof value==='string')await row.getByRole('textbox').nth(1).fill(value);
   else{await row.getByRole('button',{name:/^Chọn nguồn, dòng /}).click();const label=value.key?`Ứng dụng / ${value.key}`:`${value.service} / ${value.port}`;await page.getByRole('dialog').getByRole('button',{name:new RegExp('^'+escape(label)+'(?: Bí mật)?$')}).click();}
  }
  for(const[key,label]of[['readiness','readinessProbe'],['startup','startupProbe']])if(c[key]){
   const health=await group(page,section,text('defaults','health')),q=c[key];
   const field=health.locator('fieldset').filter({has:page.locator('legend').getByText(text('templates',label),{exact:true})});await field.getByRole('checkbox').check();await field.getByRole('combobox',exact('Loại kiểm tra')).selectOption(q.type);
   if(q.type==='exec')await field.getByLabel('Lệnh kiểm tra (mỗi đối số một dòng)',{exact:true}).fill(q.command.join('\n'));
   else{await field.getByLabel('Cổng kiểm tra',{exact:true}).fill(String(q.port));if(q.type==='http')await field.getByLabel('Đường dẫn HTTP',{exact:true}).fill(q.path);}
   await field.locator('details > summary').click();for(const k of ['initialDelaySeconds','timeoutSeconds','periodSeconds','successThreshold','failureThreshold'])if(q[k]!==undefined)await field.getByLabel(text('probes',k),{exact:true}).fill(String(q[k]));
  }
 }
 if(spec.claim){const claim=spec.claim,g=await group(page,page,text('defaults','statefulStorage'));await g.getByRole('button',exact(text('defaults','addClaim'))).click();await g.getByLabel(text('defaults','claimName'),{exact:true}).fill(claim.name);await g.getByLabel(text('defaults','size'),{exact:true}).fill(claim.size);await g.getByRole('button',exact(text('defaults','addClaimMount'))).click();await g.locator('select').last().selectOption(claim.container);await g.getByLabel('path',{exact:true}).fill(claim.path);}
 const attached=new Set();
 for(const mount of [...spec.mounts,...spec.secrets.map(m=>({...m,secret:true}))]){
  const index=await page.getByLabel('Tên Container',{exact:true}).evaluateAll((es,name)=>es.findIndex(e=>e.value===name),mount.container),section=page.locator('section.content-panel').filter({has:page.getByRole('heading',exact(`Container ${index+1}`))});await group(page,section,text('defaults','volumes'));
  await section.getByRole('button',exact('Thêm volume hoặc file')).click();const dialog=page.getByRole('dialog',exact('Thêm volume hoặc file'));
  if(mount.secret){await dialog.getByRole('button',exact(`Dùng ${mount.key} · File bí mật`)).click();await dialog.getByLabel('Đường dẫn tệp',{exact:true}).fill(mount.path);await dialog.getByRole('combobox',exact('Quyền đọc')).selectOption('288');}
  else{
   const volume=spec.volumes.find(v=>v.name===mount.volume);assert(volume);const kind=volume.files?'configMap':'emptyDir',label=volume.files?'File cấu hình (ConfigMap)':'Volume tạm (emptyDir)';
   if(attached.has(volume.name))await dialog.getByRole('button',exact(`Dùng ${volume.name} · ${label}`)).click();
   else{await dialog.getByRole('button',exact('Tạo nguồn mới')).click();await dialog.getByRole('combobox',exact('Loại')).selectOption(kind);await dialog.getByLabel('Tên volume',{exact:true}).fill(volume.name);if(volume.files){for(let n=0;n<volume.files.length;n++){if(n)await dialog.getByRole('button',exact('Thêm file cấu hình')).click();await dialog.getByLabel('Tên file cấu hình',{exact:true}).nth(n).fill(volume.files[n].name);await dialog.locator('textarea').nth(n).fill(volume.files[n].value);}}else if(volume.emptyDir?.medium)await dialog.getByLabel('Nơi lưu volume tạm',{exact:true}).selectOption(volume.emptyDir.medium);attached.add(volume.name);}
   await dialog.getByLabel('Đường dẫn mount',{exact:true}).fill(mount.path);if(mount.subPath)await dialog.getByLabel('Thư mục con / file (subPath)',{exact:true}).fill(mount.subPath);if(!volume.files)await dialog.getByRole('checkbox',exact('Chỉ đọc')).setChecked(!!mount.readOnly);
  }
  await dialog.getByRole('button',exact(text('containerVolumes','attach'))).click();await dialog.waitFor({state:'hidden'});
 }
 for(const name of spec.wait??[])await page.getByLabel(name,{exact:true}).check();
 await ui.submit(base+'/workloads/'+spec.name,'PUT',page.getByRole('button',exact('Lưu draft Workload')));
 const saved=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===spec.name)?.workload;assert(saved);assert.equal(saved.workloadExecution.kind,spec.kind);assert.equal(Object.keys(saved.containers).length,spec.containers.length);if(spec.wait)assert.deepEqual(saved.workloadExecution.waitForJobs,spec.wait);return saved;
}

// Approved storage-provider change, before the first deployment. Every edit is
// made through the existing form; no application mutation request is synthesized.
export async function updateGitLabStorage(ui,base,appKey,spec){
 const {page}=ui;
 await page.goto(ui.baseURL+`/ui/applications/${appKey}/environments/staging/workloads/${spec.name}`);
 await page.getByLabel('Tên Workload',{exact:true}).waitFor();for(let attempt=0;await page.getByLabel('Tên Workload',{exact:true}).inputValue()!==spec.name;attempt++){assert(attempt<150,'Editor did not load expected workload');await page.waitForTimeout(200);}
 const c=spec.containers[0];await page.getByLabel('Ảnh Container',{exact:true}).fill(c.image);
 const oldName=await page.getByLabel('Tên Container',{exact:true}).inputValue();
 if(oldName!==c.name)await page.getByLabel('Tên Container',{exact:true}).fill(c.name);
 const section=page.locator('section.content-panel').filter({has:page.getByRole('heading',exact('Container 1'))});
 const startup=await group(page,section,text('defaults','startup'));await startup.getByLabel(text('templates','command'),{exact:true}).fill(c.command.join('\n'));await startup.getByLabel(text('templates','args'),{exact:true}).fill(c.args.join('\n'));
 const rows=section.locator('tr[data-row]');for(const[from,to]of spec.name==='objectstore'?[['MINIO_ROOT_USER','AWS_ACCESS_KEY_ID'],['MINIO_ROOT_PASSWORD','AWS_SECRET_ACCESS_KEY']]:[['S3_USER','AWS_ACCESS_KEY_ID'],['S3_PASSWORD','AWS_SECRET_ACCESS_KEY']]){const index=await rows.evaluateAll((rs,k)=>rs.findIndex(r=>r.querySelector('td input')?.value===k),from);assert(index>=0);await rows.nth(index).locator('td input').first().fill(to);}
 for(const[key,value]of Object.entries(c.env).filter(([,v])=>typeof v==='string')){assert.equal(await rows.evaluateAll((rs,k)=>rs.some(r=>r.querySelector('td input')?.value===k),key),false);await section.getByRole('button',exact('+ Thêm biến')).click();const row=rows.last();await row.locator('td input').first().fill(key);await row.getByRole('textbox').nth(1).fill(value);}
 if(spec.name==='objectstore'){
  const health=await group(page,section,text('defaults','health')),field=health.locator('fieldset').filter({has:page.locator('legend').getByText(text('templates','readinessProbe'),{exact:true})});await field.getByRole('combobox',exact('Loại kiểm tra')).selectOption('tcp');await field.getByLabel('Cổng kiểm tra',{exact:true}).fill('9000');
  const claim=await group(page,page,text('defaults','statefulStorage'));await claim.locator('select').last().selectOption(c.name);
 }
 await ui.submit(base+'/workloads/'+spec.name,'PUT',page.getByRole('button',exact('Lưu draft Workload')));
 const saved=(await ui.get(base+'/workloads')).workloads.find(w=>w.id===spec.name)?.workload;assert.equal(saved.containers[c.name].image,c.image);return saved;
}

export async function updateGitLabConnectionFiles(ui,base,appKey,spec,previous){
 const {page}=ui;await page.goto(ui.baseURL+`/ui/applications/${appKey}/environments/staging/workloads/${spec.name}`);await page.getByLabel('Tên Workload',{exact:true}).waitFor();for(let attempt=0;await page.getByLabel('Tên Workload',{exact:true}).inputValue()!==spec.name;attempt++){assert(attempt<150,'Editor did not load expected workload');await page.waitForTimeout(200);}
 let changed=0;
 for(const volume of spec.volumes.filter(v=>v.files)){
  const prior=previous.workloadExecution.volumes.find(v=>v.name===volume.name)?.configMap;assert(prior);
  if(JSON.stringify(prior.files)===JSON.stringify(volume.files))continue;
  const mount=spec.mounts.find(m=>m.volume===volume.name);assert(mount);const index=await page.getByLabel('Tên Container',{exact:true}).evaluateAll((es,name)=>es.findIndex(e=>e.value===name),mount.container),section=page.locator('section.content-panel').filter({has:page.getByRole('heading',exact(`Container ${index+1}`))});await group(page,section,text('defaults','volumes'));
  const fields=section.getByRole('group',exact(`Volume và file của ${mount.container}`)),cards=fields.locator(':scope > div.content-panel');await cards.first().waitFor();let card;
  for(let i=0;i<await cards.count();i++)if(await cards.nth(i).locator('select').first().inputValue()===volume.name){card=cards.nth(i);break;}
  assert(card,'Mounted ConfigMap source '+volume.name);await card.locator('details > summary').click();const names=card.getByLabel('Tên file cấu hình',{exact:true});assert.equal(await names.count(),volume.files.length);
  for(let n=0;n<volume.files.length;n++){assert.equal(await names.nth(n).inputValue(),volume.files[n].name);assert.equal(await card.locator('textarea').nth(n).inputValue(),prior.files[n].value);if(prior.files[n].value!==volume.files[n].value){await card.locator('textarea').nth(n).fill(volume.files[n].value);changed++;}}
 }
 assert(changed>0,'Expected URL-normalization corrections');await ui.submit(base+'/workloads/'+spec.name,'PUT',page.getByRole('button',exact('Lưu draft Workload')));return (await ui.get(base+'/workloads')).workloads.find(w=>w.id===spec.name).workload;
}
