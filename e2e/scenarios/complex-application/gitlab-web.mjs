// Inspect the actual GitLab page after a successful Console batch. Credentials
// and browser artifacts stay private; this does not synthesize app mutations.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const state=JSON.parse(readFileSync(dir+'/gitlab-state.json','utf8'));
const input=JSON.parse(readFileSync(dir+'/gitlab-input.json','utf8'));
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'});
let context;
try{
 const ui=makeUI(s.page,c.baseURL,180000),base=`/applications/${state.app.key}/environments/staging`;
 const operations=(await ui.get(base+'/operations?reviewRequestId='+encodeURIComponent(state.attempt.reviewRequestId))).operations;
 assert.equal(operations.find(o=>o.id===state.attempt.operationId)?.status,'SUCCEEDED','Complete the recorded batch before functional inspection');
 const origin=`https://${input.endpoint.name}.${input.endpoint.rootDomain}`;
 context=await s.browser.newContext({viewport:{width:1440,height:1000}});const page=await context.newPage();page.setDefaultTimeout(30000);
 const response=await page.goto(origin,{waitUntil:'domcontentloaded',timeout:60000});assert.equal(new URL(page.url()).origin,origin,'Unexpected redirect origin');
 const report={at:new Date().toISOString(),url:page.url(),status:response?.status(),title:await page.title(),headings:await page.locator('h1,h2').allTextContents(),controls:await page.locator('input,textarea,button').evaluateAll(es=>es.map(e=>({tag:e.tagName,type:e.getAttribute('type'),name:e.getAttribute('name'),label:e.getAttribute('aria-label'),text:e.tagName==='BUTTON'?e.textContent?.trim():undefined,labels:e.labels?[...e.labels].map(l=>l.textContent?.trim()):undefined})))};
 writeFileSync(dir+'/gitlab-web-inspect.json',JSON.stringify(report,null,2));await page.screenshot({path:dir+'/gitlab-web-inspect.png',fullPage:true});assert(response?.ok(),'GitLab page failed; inspect private browser evidence');console.log('GitLab page inspected; functional acceptance remains separate');
}finally{await context?.close();await closeConsole(s,c);}
