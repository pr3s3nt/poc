// Read-only cluster verification plus a real browser guest session in the demo.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {openConsole,closeConsole,readConsoleConfig} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json')),state=JSON.parse(readFileSync(dir+'/state.json'));
assert.equal(state.attempt?.status,'SUCCEEDED','Deploy must finish before acceptance');
assert.equal(state.attempt.requestCount,1);
const c=readConsoleConfig(),s=await openConsole(c,{role:'ADMIN'}),ui=makeUI(s.page,c.baseURL,30000);
try{
 const app=(await ui.get('/applications/'+state.app.key)).application;
 const env=app.environments.find(e=>e.key==='staging'),ns=env.namespaceIdentity;
 assert(ns);assert(!env.activeOperation,'Environment still busy');
 const kube=(...args)=>JSON.parse(execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f','-n',ns,...args,'-o','json'],{encoding:'utf8',timeout:30000,stdio:['ignore','pipe','pipe']}));
 const namespace=JSON.parse(execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f','get','namespace',ns,'-o','json'],{encoding:'utf8',timeout:30000}));assert.equal(namespace.metadata.uid,state.namespaceUid);
 const ds=kube('get','deployments').items;assert.deepEqual(ds.map(x=>x.metadata.name).sort(),['backend','frontend']);
 for(const d of ds){assert(d.status.observedGeneration>=d.metadata.generation);assert.equal(d.status.readyReplicas,1);assert.equal(d.status.updatedReplicas,1);assert.equal(d.spec.template.spec.containers[0].securityContext.privileged,false);}
 const pg=kube('get','statefulsets').items;assert.equal(pg.length,1);assert.equal(pg[0].status.readyReplicas,1);
 const containers=ds.find(d=>d.metadata.name==='backend').spec.template.spec.containers;
 const pw=containers[0].env.find(e=>e.name==='POSTGRES_PASSWORD');assert(pw.valueFrom?.secretKeyRef);assert(!Object.hasOwn(pw,'value'));
 const routes=kube('get','httproutes').items;assert.equal(routes.length,2);
 const paths=routes.map(r=>{assert.deepEqual(r.spec.hostnames,[new URL(input.publicUrl).hostname]);const parent=r.status?.parents?.find(p=>p.parentRef.name===input.gatewayName&&p.parentRef.namespace===input.gatewayNamespace);for(const type of ['Accepted','ResolvedRefs'])assert(parent?.conditions?.some(c=>c.type===type&&c.status==='True'&&c.observedGeneration===r.metadata.generation));return r.spec.rules[0].matches[0].path.value;}).sort();assert.deepEqual(paths,['/','/api']);
 await s.page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=staging`);await s.page.getByRole('heading',{name:'fb-issue47',exact:true}).waitFor();await s.page.getByText('backend',{exact:true}).first().waitFor();await s.page.screenshot({path:dir+'/console-handoff.png',fullPage:true});
 const context=await s.browser.newContext(),page=await context.newPage();
 try{
  const catalogResponse=page.waitForResponse(r=>new URL(r.url()).pathname.startsWith('/api/catalog/entities')&&r.status()===200,{timeout:90000}).catch(e=>({error:e.message}));
  const response=await page.goto(input.publicUrl+'/',{waitUntil:'domcontentloaded'});assert.equal(response.status(),200);
  const heading=page.getByRole('heading',{name:/catalog/i}).first(),enter=page.getByRole('button',{name:/enter|sign in/i}).first(),catalogLink=page.getByRole('link',{name:'Catalog',exact:true}).first();
  await catalogLink.or(enter).first().waitFor({timeout:60000});if(await enter.isVisible())await enter.click();await catalogLink.waitFor({timeout:60000});await catalogLink.click();await heading.waitFor({timeout:60000});
  const catalogResult=await catalogResponse;assert(!catalogResult.error,'Browser did not receive a successful catalog response');const catalogData=await catalogResult.json();
  const catalog={status:catalogResult.status(),items:Array.isArray(catalogData)?catalogData.length:catalogData.items?.length};assert(Number.isInteger(catalog.items),'Catalog response has no entity list');
  await page.getByRole('table').first().waitFor({timeout:30000});
  await page.screenshot({path:dir+'/backstage-handoff.png',fullPage:true});
  writeFileSync(dir+'/verification.json',JSON.stringify({checkedAt:new Date().toISOString(),appKey:state.app.key,namespace:ns,operationId:state.attempt.operationId,deployRequests:state.attempt.requestCount,deployments:ds.map(d=>({name:d.metadata.name,uid:d.metadata.uid,ready:d.status.readyReplicas})),postgres:{name:pg[0].metadata.name,uid:pg[0].metadata.uid,ready:pg[0].status.readyReplicas},routes:routes.map(r=>({name:r.metadata.name,uid:r.metadata.uid,host:r.spec.hostnames,path:r.spec.rules[0].matches[0].path.value})),passwordDelivery:'secretKeyRef',publicUrl:input.publicUrl,browserCatalogDiagnostic:catalog},null,2));
  console.log('PASS two workloads, PostgreSQL, Secret reference, shared endpoint routes and Backstage guest browser');
 }catch(e){await page.screenshot({path:dir+'/backstage-error.png',fullPage:true});throw e;}finally{await context.close();}
}catch(e){await s.page.screenshot({path:dir+'/verification-error.png',fullPage:true});throw e;}finally{await closeConsole(s,c);}
