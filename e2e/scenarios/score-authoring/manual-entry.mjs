import {createRequire} from 'node:module';
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {openConsole,readConsoleConfig,closeConsole} from '../../lib/console.mjs';
const {expect}=createRequire(new URL('../../../scripts/bootstrap/package.json',import.meta.url))('@playwright/test');
const config=readConsoleConfig(),evidence=resolve(process.env.ORCH_EVIDENCE_DIR??'');
assert(process.env.ORCH_EVIDENCE_DIR&&!evidence.startsWith(resolve('.')+'/'),'Private evidence directory outside Git required');
mkdirSync(evidence,{recursive:true,mode:0o700});
const receipt={name:'score83manual-'+Date.now().toString(36),checks:[]};
const save=()=>writeFileSync(join(evidence,'receipt.json'),JSON.stringify(receipt,null,2),{mode:0o600});
const session=await openConsole(config,{}),{page}=session;page.setDefaultTimeout(30_000);
let failure;
try {
 await page.goto(config.baseURL+'/ui/applications/new');
 await page.getByLabel('Tên ứng dụng',{exact:true}).fill(receipt.name);
 const created=page.waitForResponse(r=>r.request().method()==='POST'&&new URL(r.url()).pathname==='/api/v1/applications');
 await page.getByRole('button',{name:'Tạo ứng dụng',exact:true}).click();
 const creation=await created;assert(creation.ok());const {application}=await creation.json();receipt.appId=application.key??application.id;save();
 const base=`/api/v1/applications/${receipt.appId}/environments/staging`;
 await page.goto(config.baseURL+`/ui/applications/${receipt.appId}/environments/staging/workloads/new`);
 await page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
 const score=page.getByRole('textbox',{name:'Nội dung Score (YAML/JSON)',exact:true});
 const extension=page.getByRole('textbox',{name:'Nội dung extension (tùy chọn)',exact:true});
 const source=readFileSync(new URL('./deployment.yaml',import.meta.url),'utf8');
 const extra=readFileSync(new URL('./deployment.extension.yaml',import.meta.url),'utf8');
 const check=()=>page.getByRole('button',{name:'Kiểm tra',exact:true});
 const saveButton=()=>page.getByRole('button',{name:'Lưu draft Workload',exact:true});
 const exportButton=()=>page.getByRole('button',{name:'Tải Score + extension',exact:true});
 let parseCalls=0;page.on('request',r=>{if(r.method()==='POST'&&r.url().endsWith('/workloads/parse'))parseCalls++;});
 await score.fill(source);await extension.fill(extra);
 assert.equal(parseCalls,0);await expect(saveButton()).toBeDisabled();await expect(exportButton()).toBeDisabled();
 async function validate(status=200){const pending=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/workloads/parse'));await check().click();const response=await pending;assert.equal(response.status(),status);return response.json();}
 const first=await validate();assert.equal(first.workload.metadata.name,'database-client');await expect(saveButton()).toBeEnabled();
 receipt.checks.push('typed Score/extension uses real parse API with no file selection');save();
 const edited=source.replace('    variables:\n','    variables:\n      MANUAL_INPUT: edited\n');
 const callsBeforeEdit=parseCalls;await score.fill(edited);assert.equal(parseCalls,callsBeforeEdit);await expect(saveButton()).toBeDisabled();await expect(exportButton()).toBeDisabled();
 await score.fill('apiVersion: [');await validate(400);await expect(page.getByRole('alert')).toBeVisible();assert.equal(await score.inputValue(),'apiVersion: [');await expect(saveButton()).toBeDisabled();
 await score.fill(edited);const valid=await validate();assert.equal(valid.workload.containers.main.variables.MANUAL_INPUT,'edited');
 receipt.checks.push('editing invalidates old result; invalid text retained and stale Save/export blocked');save();
 // Existing file selection fills the same editable text and automatically checks.
 const fileCheck=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/workloads/parse'));
 await page.getByLabel('File Workload',{exact:true}).setInputFiles({name:'score.yaml',mimeType:'text/yaml',buffer:Buffer.from(source)});
 assert((await fileCheck).ok());assert.equal(await score.inputValue(),source);
 await score.fill(edited);await validate();
 receipt.checks.push('file content remains directly editable after loading');save();
 const downloads=[];const onDownload=async d=>{const path=join(evidence,d.suggestedFilename());await d.saveAs(path);downloads.push({name:d.suggestedFilename(),path});};page.on('download',onDownload);
 await exportButton().click();await expect.poll(()=>downloads.length).toBe(2);page.off('download',onDownload);
 const roundtrip=await page.request.post(config.baseURL+base+'/workloads/parse',{data:{content:readFileSync(downloads.find(x=>x.name==='score.yaml').path,'utf8'),extensions:readFileSync(downloads.find(x=>x.name==='humanitec.score.yaml').path,'utf8')}});assert(roundtrip.ok());assert.deepEqual((await roundtrip.json()).workload,valid.workload);
 const saved=page.waitForResponse(r=>r.request().method()==='PUT'&&r.url().endsWith('/workloads/database-client'));await saveButton().click();assert((await saved).ok());
 const list=await(await page.request.get(config.baseURL+base+'/workloads')).json();assert.equal(list.workloads[0].workload.containers.main.variables.MANUAL_INPUT,'edited');assert.equal(list.workloads[0].state,'PENDING_UPSERT');
 receipt.checks.push('edited input export/reimport and explicit Save persist exact normalized data and pins');save();
 await page.goto(config.baseURL+`/ui/applications/${receipt.appId}/environments/staging/workloads/database-client`);await page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
 await score.fill(edited);await extension.fill(extra);await validate();await page.screenshot({path:join(evidence,'manual-vi.png'),fullPage:true});
 await page.locator('.language-select select').selectOption('en');await page.getByRole('button',{name:'Import Workload',exact:true}).click();
 await page.getByRole('textbox',{name:'Score content (YAML/JSON)',exact:true}).fill(edited);await page.getByRole('textbox',{name:'Extension content (optional)',exact:true}).fill(extra);
 const englishCheck=page.getByRole('button',{name:'Check',exact:true});await englishCheck.focus();const englishParsed=page.waitForResponse(r=>r.request().method()==='POST'&&r.url().endsWith('/workloads/parse'));await englishCheck.press('Enter');assert((await englishParsed).ok());await expect(page.getByRole('button',{name:'Save Workload draft',exact:true})).toBeEnabled();
 await page.screenshot({path:join(evidence,'manual-en.png'),fullPage:true});receipt.checks.push('VI/EN labels and keyboard Check work on real backend');receipt.status='PASS';save();
 console.log('PASS: manual Score/extension entry, validation gate, file-to-text edit, export/reimport, Save, VI/EN and keyboard');
} catch(error) {failure=error;receipt.status='FAILED';receipt.error=error.message;save();await page.screenshot({path:join(evidence,'failure.png'),fullPage:true}).catch(()=>{});}
finally {
 if(receipt.appId) {
  try {
   const a=await(await page.request.get(config.baseURL+'/api/v1/applications/'+receipt.appId)).json();assert.equal(a.application.name,receipt.name);
   const preview=await page.request.post(config.baseURL+`/api/v1/applications/${receipt.appId}/deletion-preview`,{data:{}});assert(preview.ok());const {preview:p}=await preview.json();assert(!p.blocked&&p.token);
   const deletion=await page.request.post(config.baseURL+`/api/v1/applications/${receipt.appId}/deletions`,{data:{token:p.token,confirmationName:receipt.name}});assert(deletion.ok());const {deletion:d}=await deletion.json();receipt.deletionId=d.operationId;save();
   await expect.poll(async()=>{const r=await page.request.get(config.baseURL+`/api/v1/applications/${receipt.appId}/deletions/${d.operationId}`);assert(r.ok());const {deletion:v}=await r.json();receipt.cleanup=v.status;save();return v.status;},{timeout:90_000,intervals:[1000]}).toBe('SUCCEEDED');
   console.log('PASS: receipt-owned manual-entry Application cleaned through product API; no workload deployment performed');
  } catch(error) {receipt.cleanupError=error.message;save();failure??=error;}
 }
 await closeConsole(session,config);
}
if(failure)throw failure;
