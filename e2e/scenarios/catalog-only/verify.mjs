// Read-only live UI check after container recreation; never refreshes sources,
// changes desired state or invokes Deploy.
import assert from 'node:assert/strict';
import { mkdirSync, writeFileSync } from 'node:fs';
import { resolve, relative, join } from 'node:path';
import { closeConsole, openConsole, readConsoleConfig } from '../../lib/console.mjs';
const config=readConsoleConfig();
const app=process.env.ORCH_CATALOG_APP, tag=process.env.ORCH_CATALOG_TAG;
const source=process.env.ORCH_CATALOG_SOURCE??'catalog-only-20261102';
const replicas=Number(process.env.ORCH_EXPECTED_REPLICAS??'2');
assert.ok(app&&tag,'ORCH_CATALOG_APP and ORCH_CATALOG_TAG are required');
const evidence=resolve(process.env.ORCH_EVIDENCE_DIR??'');
assert.ok(process.env.ORCH_EVIDENCE_DIR&&relative(process.cwd(),evidence).startsWith('..'),'Evidence must be outside Git');
mkdirSync(evidence,{recursive:true,mode:0o700});
const escape=value=>value.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
let session;
try{
 session=await openConsole(config);const {page}=session;
 await page.goto(`${config.baseURL}/ui/applications/${app}?environment=staging`);
 const observed=page.waitForResponse(reply=>new URL(reply.url()).pathname===`/api/v1/applications/${app}/environments/staging/preview`&&reply.request().method()==='POST');
 await page.getByRole('button',{name:'Xem trước thay đổi',exact:true}).click();
 const response=await observed;assert.equal(response.status(),200);const preview=await response.json();
 assert.equal(preview.changes.length,0);assert.equal(Boolean(preview.catalogPending),false);
 assert.equal(preview.appliedCatalog.tag,tag);assert.equal(preview.desiredCatalog.tag,tag);
 await page.screenshot({path:join(evidence,'after-recreate.png'),fullPage:true});
 await page.goto(config.baseURL+'/ui/platform/catalog');
 const sourceRow=page.locator('.section-header').filter({has:page.locator('strong').filter({hasText:new RegExp('^'+escape(source)+'$')})});
 await sourceRow.getByRole('button',{name:'Các phiên bản',exact:true}).click();
 const row=page.locator('.content-panel').filter({has:page.getByRole('heading',{name:'Các phiên bản',exact:true})}).locator('div').filter({has:page.locator('strong').filter({hasText:new RegExp('^'+escape(tag)+'$')})}).first();
 await row.getByRole('button',{name:'Xem thành phần',exact:true}).click();
 const detail=page.getByRole('region',{name:`Thành phần ${tag}`,exact:true});
 const component=detail.locator('details').filter({has:page.locator('strong').filter({hasText:'bootstrap-standard-workload'})});
 await component.locator('summary').click();
 const snapshot=JSON.parse(await component.locator('pre').textContent());
 assert.equal(snapshot.driverInputs.values.variables.template_inputs.defaultReplicas,replicas);
 assert.equal(await detail.getByRole('textbox').count(),0);
 await page.screenshot({path:join(evidence,'read-only-policy.png'),fullPage:true});
 writeFileSync(join(evidence,'after-recreate.json'),JSON.stringify({status:'SUCCEEDED',application:app,applied:tag,changes:0,catalogPending:false,defaultReplicas:replicas,readOnly:true},null,2)+'\n',{mode:0o600});
 console.log('PASS: real UI after recreation; applied release retained, Preview has no changes, Git policy visible read-only');
}finally{if(session)await closeConsole(session,config);}
