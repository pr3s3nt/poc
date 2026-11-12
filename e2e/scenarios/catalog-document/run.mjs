import {openConsole,readConsoleConfig,closeConsole} from '../../lib/console.mjs';
import assert from 'node:assert/strict';
const run = Date.now().toString(36);
const c=readConsoleConfig();const s=await openConsole(c,{});const {page}=s;
try {
 for(const kind of ['resource-types','resource-definitions']) {
 await page.goto(c.baseURL+'/ui/platform/'+kind);await page.getByText('Nhập YAML/JSON',{exact:true}).click();
 const type={key:'issue76-browser-'+run,inputs:[{name:'database',type:'string',required:true}],outputs:[{name:'password',type:'string',secret:true}]};
 const def={key:'issue76-edge-'+run,name:'Issue 76 edge',resourceType:'endpoint',driverType:'endpoint',driverInputs:{values:{variables:{rootDomain:'apps.example.com',scheme:'https',gatewayName:'traefik',gatewayNamespace:'traefik'}}},criteria:[{env_type:'staging'}],provision:{}};
 if(kind==='resource-definitions') {
 const typeResponse=await page.request.post(c.baseURL+'/api/v1/resource-types/import',{data:{key:'endpoint',outputs:[{name:'hostname',type:'string'},{name:'url',type:'string'}]}});assert(typeResponse.ok());
 }
 const doc=kind==='resource-types'?type:def;
 const editor=page.getByLabel('Nội dung YAML/JSON',{exact:true}); const register=page.locator('details').filter({has:editor}).getByRole('button',{name:'Đăng ký',exact:true});
 await page.getByLabel('Chọn file',{exact:true}).setInputFiles({name:'object.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify({...doc,key:doc.key+'-upload'}))});
 await assert.doesNotReject(()=>editor.inputValue());
 // Edit uploaded content before registration.
 await editor.fill(JSON.stringify(doc));await register.click(); await page.getByRole('status').filter({hasText:'CREATED'}).waitFor();
 const snapshotPath=kind==='resource-types'?'/resource-types':`/resource-definitions/${doc.key}/versions`;
 const before=await (await page.request.get(c.baseURL+'/api/v1'+snapshotPath)).json();
 await register.click();await page.getByRole('status').filter({hasText:'REUSED'}).waitFor();
 const changed=kind==='resource-types'?{...doc,inputs:[]}:{...doc,name:'Changed'};
 await editor.fill(JSON.stringify(changed));await register.click();await page.getByRole('status').filter({hasText:'CONFLICT'}).waitFor();assert.equal(await editor.inputValue(),JSON.stringify(changed));
 assert.deepEqual(await (await page.request.get(c.baseURL+'/api/v1'+snapshotPath)).json(),before,'reuse/conflict must not mutate Type or revision history');
 await editor.fill('key: [');await register.click();await page.getByRole('alert').waitFor();assert.equal(await editor.inputValue(),'key: [');
 let nonFiniteWrites=0;const record=request=>{if(request.method()==='POST'&&request.url().endsWith('/import'))nonFiniteWrites++;};page.on('request',record);
 await editor.fill('value: .inf');await register.click();await page.getByRole('alert').filter({hasText:'Cú pháp'}).waitFor();assert.equal(await editor.inputValue(),'value: .inf');assert.equal(nonFiniteWrites,0);page.off('request',record);
 if(kind==='resource-types')await page.getByRole('button',{name:'Xem schema '+type.key,exact:true}).click();
 const downloadButton=kind==='resource-types'?page.locator('.catalog-entry').filter({has:page.locator('strong',{hasText:type.key})}).getByRole('button',{name:'Tải file',exact:true}):page.locator(`[data-definition="${doc.key}"]`).getByRole('button',{name:'Tải file · v1',exact:true});
 const [download]=await Promise.all([page.waitForEvent('download'),downloadButton.click()]);const path=await download.path();
 await page.getByLabel('Chọn file',{exact:true}).setInputFiles({name:'roundtrip.yaml',mimeType:'application/yaml',buffer:await (await import('node:fs/promises')).readFile(path)});
 await register.click();await page.getByRole('status').filter({hasText:'REUSED'}).waitFor();
 console.log('PASS',kind,'upload/edit CREATED REUSED CONFLICT syntax retention YAML round-trip');
 if(process.env.ORCH_EVIDENCE_DIR) await page.screenshot({path:process.env.ORCH_EVIDENCE_DIR+'/'+kind+'.png',fullPage:true});
 }
}finally{await closeConsole(s,c)}
