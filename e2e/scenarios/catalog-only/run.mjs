import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve, join, relative } from 'node:path';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';
import { ensureCatalog } from '../../../scripts/bootstrap/lib/catalog-ui.mjs';

const config=readConsoleConfig();
const evidence=resolve(process.env.ORCH_EVIDENCE_DIR??'');
assert.ok(process.env.ORCH_EVIDENCE_DIR && relative(process.cwd(),evidence).startsWith('..'),'Evidence must be outside the repository');
mkdirSync(evidence,{recursive:true,mode:0o700});
const tag=process.env.ORCH_CATALOG_TAG;
assert.ok(tag,'ORCH_CATALOG_TAG is required');
const source={id:process.env.ORCH_CATALOG_SOURCE??'catalog-only-20261102',repository:process.env.ORCH_CATALOG_REPOSITORY??'https://github.com/pr3s3nt/poc.git',branch:process.env.ORCH_CATALOG_BRANCH??'main',tagPattern:process.env.ORCH_CATALOG_TAG_PATTERN??'v0.20261102.*',manifestPath:'catalog.yaml',includePrereleases:false,accountBindings:{internal:process.env.ORCH_CATALOG_ACCOUNT??'k8s-4f'}};
let session,appKey=process.env.ORCH_CATALOG_APP;
const result={source:source.id,tag,status:'FAILED'};
try{
 session=await openConsole(config);
 assert.ok(['ADMIN','PLATFORM_ENGINEER'].includes(session.user.role));
 const {page}=session;
 const get=async path=>{const response=await page.request.get(config.baseURL+'/api/v1'+path);assert.equal(response.status(),200,`GET ${path}`);return response.json();};
 const act=async(locator,method,...args)=>locator[method](...args);
 const responseFor=(path,method)=>page.waitForResponse(reply=>new URL(reply.url()).pathname==='/api/v1'+path && reply.request().method()===method,{timeout:360_000});
 await ensureCatalog({page,baseURL:config.baseURL,get,act,responseFor,source});
 await page.getByRole('button',{name:'Các phiên bản',exact:true}).click();
 const release=page.locator('.content-panel').filter({has:page.getByRole('heading',{name:'Các phiên bản',exact:true})}).locator('div').filter({has:page.locator('strong').filter({hasText:new RegExp('^'+tag.replaceAll('.','\\.')+'$')})}).first();
 await release.getByRole('button',{name:'Xem thành phần',exact:true}).click();
 const detail=page.getByRole('region',{name:`Thành phần ${tag}`,exact:true});
 await detail.waitFor();
 const template=detail.locator('details').filter({has:page.locator('strong').filter({hasText:'bootstrap-standard-workload'})});
 await template.locator('summary').click();
 const snapshot=JSON.parse(await template.locator('pre').textContent());
 assert.equal(snapshot.driverType,'template');assert.ok(snapshot.criteria.length);
 assert.equal(await detail.getByRole('textbox').count(),0);
 assert.equal(await page.getByRole('link',{name:/Mẫu dựng ứng dụng|Cấu hình tài nguyên/}).count(),0);
 await page.screenshot({path:join(evidence,'release.png'),fullPage:true});
 const beforeApps=(await get('/applications')).applications.map(value=>value.key).sort();
 if(!appKey){
  await page.goto(config.baseURL+'/ui/applications/new');
  await page.getByLabel('Tên ứng dụng',{exact:true}).fill('Catalog live '+Date.now());
  await page.getByLabel('Subdomain',{exact:true}).fill('catalog-live-'+Date.now());
  const created=responseFor('/applications','POST');
  await page.getByRole('button',{name:'Tạo ứng dụng',exact:true}).click();
  const reply=await created;assert.equal(reply.status(),201);
  appKey=(await reply.json()).application.key;
 }
 result.application=appKey;
 writeFileSync(join(evidence,'result.json'),JSON.stringify(result,null,2)+'\n',{mode:0o600});
 const base=`/applications/${encodeURIComponent(appKey)}/environments`;
 const production=(await get(base+'/production/catalog')).catalog;
 const beforeResources=(await get(base+'/staging/resources')).resources??[];
 await page.goto(`${config.baseURL}/ui/applications/${appKey}/settings?environment=staging`);
 const panel=page.getByRole('region',{name:'Catalog',exact:true});
 await panel.getByRole('combobox').selectOption(JSON.stringify([source.id,tag]));
 const selected=responseFor(base+'/staging/catalog','PUT');
 await panel.getByRole('button',{name:'Lưu lựa chọn',exact:true}).click();
 const saved=await selected;assert.equal(saved.status(),200);
 await panel.getByRole('status').filter({hasText:'Đã lưu mục tiêu.'}).waitFor();
 const desired=(await get(base+'/staging/catalog')).catalog;
 assert.equal(desired.desired.tag,tag);
 if(!beforeResources.length){
  const workload={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'web',class:'pvc'},containers:{main:{image:'nginx:1.27-alpine'}},service:{ports:{http:{port:80,targetPort:80}}}};
  const file=join(evidence,'web.json');writeFileSync(file,JSON.stringify(workload,null,2)+'\n',{mode:0o600});
  await page.goto(`${config.baseURL}/ui/applications/${appKey}/environments/staging/workloads/new`);
  await page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
  await page.getByLabel('File Workload',{exact:true}).setInputFiles(file);
  await page.getByText('Đã phân tích Workload: web',{exact:false}).waitFor();
  const draft=responseFor(base+'/staging/workloads/web','PUT');
  await page.getByRole('button',{name:'Lưu draft Workload',exact:true}).click();
  assert.equal((await draft).status(),200);
 }
 await page.goto(`${config.baseURL}/ui/applications/${appKey}?environment=staging`);
 const previewReply=responseFor(base+'/staging/preview','POST');
 await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();
 const reply=await previewReply;assert.equal(reply.status(),200);
 const preview=await reply.json();assert.equal(preview.desiredCatalog.tag,tag);assert.ok(preview.catalogPin);assert.equal(preview.catalogPending,true);
 await page.screenshot({path:join(evidence,'preview.png'),fullPage:true});
 const deploying=responseFor(base+'/staging/deploy','POST');
 await page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true}).click();
 const completed=await deploying;assert.equal(completed.status(),200);const report=await completed.json();assert.equal(report.status,'SUCCEEDED');
 const applied=(await get(base+'/staging/catalog')).catalog;
 assert.equal(applied.applied.tag,tag);assert.equal(applied.status,'APPLIED');
 assert.deepEqual((await get(base+'/production/catalog')).catalog,production);
 const resources=(await get(base+'/staging/resources')).resources??[];
 const ids=new Set(resources.map(value=>value.id));assert.ok(beforeResources.every(value=>ids.has(value.id)));
 const afterApps=(await get('/applications')).applications.map(value=>value.key);
 assert.ok(beforeApps.every(key=>afterApps.includes(key)));
 await page.screenshot({path:join(evidence,'deployed.png'),fullPage:true});
 Object.assign(result,{status:'SUCCEEDED',resourceIds:[...ids],changes:preview.changes.map(value=>({workloadId:value.workloadId,action:value.action})),reportStatus:report.status});
 console.log('PASS: real Git discovery, release details, UI selection, Preview/Deploy; production and existing apps preserved',appKey,tag);
}finally{
 writeFileSync(join(evidence,'result.json'),JSON.stringify(result,null,2)+'\n',{mode:0o600});
 if(session){if(result.status!=='SUCCEEDED')await session.page.screenshot({path:join(evidence,'failed.png'),fullPage:true}).catch(()=>{});await closeConsole(session,config);}
}
