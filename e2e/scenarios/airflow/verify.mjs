// Read-only cluster verification, then a real browser session in Airflow through
// the public endpoint: sign in, see example DAGs, run one and read a task log.
import assert from 'node:assert/strict';
import {readFileSync,writeFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {openConsole,closeConsole,readConsoleConfig,readPrivateFile} from '../../lib/console.mjs';
import {makeUI,outsideRepo} from '../../../scripts/bootstrap/lib/runtime.mjs';
process.umask(0o077);
const dir=process.env.ORCH_TEST_RUN_DIR;assert(dir);outsideRepo(dir);
const input=JSON.parse(readFileSync(dir+'/config.json')),state=JSON.parse(readFileSync(dir+'/state.json'));
assert.equal(state.attempt?.status,'SUCCEEDED','Deploy must finish before acceptance');
assert.equal(state.attempt.requestCount,1);
const NAMES=['api-server','dag-processor','scheduler','triggerer'];
const c=readConsoleConfig(),s=await openConsole(c,{}),ui=makeUI(s.page,c.baseURL,30000);
const result={checkedAt:new Date().toISOString(),appKey:state.app.key,operationId:state.attempt.operationId,deployRequests:state.attempt.requestCount,publicUrl:input.publicUrl};
try{
 const app=(await ui.get('/applications/'+state.app.key)).application;
 const env=app.environments.find(e=>e.key==='staging'),ns=env.namespaceIdentity;
 assert(ns);assert(!env.activeOperation,'Environment still busy');result.namespace=ns;
 const kubectl=args=>JSON.parse(execFileSync('kubectl',['--kubeconfig',input.clusterKubeconfigFile,'--context','kind-k8s-4f',...args,'-o','json'],{encoding:'utf8',timeout:30000,stdio:['ignore','pipe','pipe']}));
 const kube=(...args)=>kubectl(['-n',ns,...args]);
 assert.equal(kubectl(['get','namespace',ns]).metadata.uid,state.namespaceUid,'Namespace UID');
 const ds=kube('get','deployments').items;assert.deepEqual(ds.map(x=>x.metadata.name).sort(),NAMES);
 for(const d of ds){assert(d.status.observedGeneration>=d.metadata.generation);assert.equal(d.status.readyReplicas,1,d.metadata.name);assert.equal(d.status.updatedReplicas,1);
  const spec=d.spec.template.spec,pw=[...spec.containers,...(spec.initContainers??[])].flatMap(x=>x.env??[]).filter(e=>e.name==='DB_PASSWORD');
  assert(pw.length>=2&&pw.every(e=>e.valueFrom?.secretKeyRef&&!Object.hasOwn(e,'value')),'DB_PASSWORD must be a Secret reference in '+d.metadata.name);}
 const pods=kube('get','pods').items.filter(p=>p.status.phase==='Running');
 const restarts=Object.fromEntries(pods.map(p=>[p.metadata.name,(p.status.containerStatuses??[]).reduce((n,x)=>n+x.restartCount,0)]));
 result.restarts=restarts;assert(Object.values(restarts).every(n=>n===0),'Pods restarted: '+JSON.stringify(restarts));
 const pg=kube('get','statefulsets').items;assert.equal(pg.length,1);assert.equal(pg[0].status.readyReplicas,1);
 const routes=kube('get','httproutes').items;assert.equal(routes.length,1);const host=new URL(input.publicUrl).hostname;
 const parent=routes[0].status?.parents?.find(p=>p.parentRef.name===input.gatewayName&&p.parentRef.namespace===input.gatewayNamespace);
 assert.deepEqual(routes[0].spec.hostnames,[host]);for(const type of ['Accepted','ResolvedRefs'])assert(parent?.conditions?.some(x=>x.type===type&&x.status==='True'));
 result.deployments=ds.map(d=>({name:d.metadata.name,uid:d.metadata.uid,ready:d.status.readyReplicas}));result.postgres={name:pg[0].metadata.name,uid:pg[0].metadata.uid};result.route={name:routes[0].metadata.name,uid:routes[0].metadata.uid,host};result.passwordDelivery='secretKeyRef';
 await s.page.goto(c.baseURL+`/ui/applications/${state.app.key}?environment=staging`);await s.page.getByText('api-server',{exact:true}).first().waitFor();await s.page.screenshot({path:dir+'/console-handoff.png',fullPage:true});
 // Airflow itself, through the same public URL a user opens, in a separate browser context.
 const context=await s.browser.newContext({viewport:{width:1400,height:900}}),page=await context.newPage();
 try{
 const health=await (await page.request.get(input.publicUrl+'/api/v2/monitor/health')).json();
 for(const k of ['metadatabase','scheduler','triggerer','dag_processor'])assert.equal(health[k]?.status,'healthy',k);result.health='healthy';
 await page.goto(input.publicUrl+'/');
 await page.locator('input[name=username]').fill('admin');await page.locator('input[name=password]').fill(readPrivateFile(dir+'/secrets/admin-password','Airflow admin password'));
 await Promise.all([page.waitForURL(u=>!u.pathname.includes('/login'),{timeout:60000}),page.locator('input[name=password]').press('Enter')]);
 await page.goto(input.publicUrl+'/dags');await page.getByText('example_bash_operator',{exact:true}).first().waitFor({timeout:60000});
 await page.screenshot({path:dir+'/airflow-dags.png',fullPage:true});
 // Trigger through the authenticated browser session's own API token.
 const token=(await (await page.request.post(input.publicUrl+'/auth/token',{data:{username:'admin',password:readPrivateFile(dir+'/secrets/admin-password','Airflow admin password')}})).json()).access_token;assert(token);
 const H={Authorization:'Bearer '+token};const dag='example_bash_operator',run='verify-'+Date.now();
 assert((await page.request.patch(`${input.publicUrl}/api/v2/dags/${dag}`,{headers:H,data:{is_paused:false}})).ok());
 assert((await page.request.post(`${input.publicUrl}/api/v2/dags/${dag}/dagRuns`,{headers:H,data:{dag_run_id:run,logical_date:null}})).ok());
 let st;for(let i=0;i<60;i++){st=(await (await page.request.get(`${input.publicUrl}/api/v2/dags/${dag}/dagRuns/${run}`,{headers:H})).json()).state;if(['success','failed'].includes(st))break;await page.waitForTimeout(5000);}
 assert.equal(st,'success','DAG run state');result.dagRun={dag,run,state:st};
 await page.goto(`${input.publicUrl}/dags/${dag}/runs/${run}/tasks/runme_0`);
 await page.getByText(/example_bash_operator__runme_0__/).first().waitFor({timeout:60000});
 await page.screenshot({path:dir+'/airflow-task-log.png',fullPage:true});result.taskLog='visible in UI';
 writeFileSync(dir+'/verification.json',JSON.stringify(result,null,2));
 console.log('PASS Airflow deployed through Console: four workloads, PostgreSQL, Secret reference, endpoint, sign-in, DAG run and task log');
}catch(e){await page.screenshot({path:dir+'/airflow-error.png',fullPage:true});writeFileSync(dir+'/verification-partial.json',JSON.stringify(result,null,2));throw e;}finally{await context.close();}
}finally{await closeConsole(s,c);}
