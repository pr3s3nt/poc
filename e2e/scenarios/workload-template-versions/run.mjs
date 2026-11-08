import assert from 'node:assert/strict';
import {writeFileSync} from 'node:fs';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
const app=process.env.ORCH_APPLICATION_KEY,env=process.env.ORCH_ENVIRONMENT??'staging',dir=process.env.ORCH_TEST_RUN_DIR;
assert(app&&dir);assert(['staging','production'].includes(env));outsideRepo(dir);
const c=readConsoleConfig(),s=await openConsole(c),p=s.page,ui=makeUI(p,c.baseURL,30000),base=`/applications/${app}/environments/${env}`;
try{
 const before=await ui.get(base+'/workloads'),{templates}=await ui.get(base+'/workload-templates');
 const versions=templates.filter(item=>item.id==='backend').sort((a,b)=>b.version.localeCompare(a.version,undefined,{numeric:true}));assert(versions.length>1);
 const home=c.baseURL+`/ui/applications/${app}?environment=${env}`;
 await p.goto(home);await p.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();
 const dialog=p.getByRole('dialog'),card=dialog.getByRole('button',{name:/^Backend API/});await card.waitFor();assert.equal(await card.count(),1);assert((await card.innerText()).includes(versions[0].version));
 assert.equal(await dialog.locator('.template-picker-list > .application-card').count(),new Set(templates.map(item=>item.id)).size);
 await card.click();await p.waitForURL(url=>url.pathname.endsWith('/workloads/new'));
 assert.equal(new URL(p.url()).searchParams.get('template'),`backend@${versions[0].version}:${versions[0].sha256}`);
 await p.getByRole('button',{name:'Hủy',exact:true}).click();await p.waitForURL(home);
 await p.getByRole('button',{name:'+ Thêm Workload',exact:true}).click();await card.waitFor();
 const group=dialog.locator('.application-card').filter({has:p.getByRole('button',{name:/^Backend API/})});const disclosure=group.locator('details');assert.equal(await disclosure.getAttribute('open'),null);
 await disclosure.getByText('Phiên bản khác',{exact:true}).click();const select=group.getByRole('combobox',{name:'Phiên bản · Backend API'});const older=versions.at(-1);await select.selectOption(older.version);
 assert.equal(new URL(p.url()).pathname,`/ui/applications/${app}`);await p.screenshot({path:dir+'/grouped-template-picker.png',fullPage:true});
 await card.click();await p.waitForURL(url=>url.pathname.endsWith('/workloads/new'));assert.equal(new URL(p.url()).searchParams.get('template'),`backend@${older.version}:${older.sha256}`);
 await p.getByRole('button',{name:'Hủy',exact:true}).click();await p.waitForURL(home);assert.deepEqual(await ui.get(base+'/workloads'),before);
 writeFileSync(dir+'/grouped-template-picker.json',JSON.stringify({checkedAt:new Date().toISOString(),singleCardPerId:true,latestPin:true,olderPin:true,versionChoiceDoesNotNavigate:true,workloadsUnchanged:true,noBusinessWrites:true},null,2),{mode:0o600});console.log('PASS grouped cards, exact latest/older pins and unchanged drafts');
}catch(error){await p.screenshot({path:dir+'/grouped-picker-error.png',fullPage:true});throw error;}finally{await closeConsole(s,c);}
