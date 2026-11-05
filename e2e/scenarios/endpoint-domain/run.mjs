// Public endpoint resource (EP-01..06) through the real Console, Fleet,
// Traefik Gateway and a real DNS name. See README.md in this directory.
import assert from 'node:assert/strict';
import {parseArgs} from 'node:util';
import {readFileSync,writeFileSync,statSync} from 'node:fs';
import {join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {verifyApplicationFleet} from '../../../scripts/bootstrap/lib/fleet-verification.mjs';
import {openConsole,closeConsole,readConsoleConfig,signInContext,signOutContext} from '../../lib/console.mjs';
import {createRuntime,makeUI,equal,digest,waitIdle} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureApplication} from '../../../scripts/bootstrap/lib/application.mjs';
import {ensureTypes} from '../../../scripts/bootstrap/lib/platform.mjs';
import {ensureTemplateSources,ensureDirectDefinitions} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
import {ensureEnvironment,envBase} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';

const {values}=parseArgs({options:{config:{type:'string'}}});assert(values.config,'Private --config required');
assert.equal(statSync(values.config).mode&0o077,0,'Config must be private');
const input=JSON.parse(readFileSync(values.config,'utf8'));
assert(/^[a-z0-9-]{1,40}$/.test(input.name));
assert(input.console.url===input.developer.url,'Use one Console');
const url=new URL(input.url);assert(!['localhost','127.0.0.1','0.0.0.0'].includes(url.hostname));assert.equal(url.pathname,'/');
const [primary,...root]=url.hostname.split('.');const rootDomain=root.join('.');
assert(rootDomain.includes('.'),'url hostname must be <name>.<root domain>');
assert(/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/.test(input.renamedName)&&input.renamedName!==primary,'renamedName must be another DNS label');
const auth=account=>readConsoleConfig({ORCH_PLATFORM_URL:account.url,ORCH_PLATFORM_USERNAME:account.username,ORCH_PLATFORM_PASSWORD_FILE:account.passwordFile,ORCH_HEADLESS:'1'});
const config={schemaVersion:3,environment:'dev',console:auth(input.console),developer:auth(input.developer),runtime:{profile:'dev-test',mode:'all',headless:true,timeoutMs:600000,stateDir:input.stateDir,reportDir:input.reportDir,clusterKubeconfigFile:input.kubeconfigFile,recordVideo:false},platform:{account:{name:input.accountName,context:input.context},delivery:{}},application:{name:input.name,subdomain:input.name,environment:'staging',variables:{},secretFiles:{},definitionRevisions:{}},verification:{url:input.url}};
const rt=createRuntime(config);let session,developer,status='FAILED';
const identity={name:input.name,url:input.url,renamedName:input.renamedName,gateway:input.gateway,source:input.templateSource,image:input.image,account:input.accountName,store:input.storeName};
if(rt.state.testInputHash)assert.equal(rt.state.testInputHash,digest(JSON.stringify(identity)),'Test input changed');
rt.state.testInputHash=digest(JSON.stringify(identity));rt.save();
const kube=(...args)=>JSON.parse(execFileSync('kubectl',['--kubeconfig',input.kubeconfigFile,'--context',input.context,...args,'-o','json'],{encoding:'utf8',timeout:30000,stdio:['ignore','pipe','pipe']}));
// Read-only ledger proof from the Console database of this isolated stack.
const sql=query=>execFileSync('docker',['exec',input.database.container,'psql','-U',input.database.user,'-d',input.database.name,'-Atc',query],{encoding:'utf8'}).trim();
const reservations=appKey=>sql(`SELECT COALESCE(string_agg(r.hostname||'|'||r.workload_id,',' ORDER BY r.hostname),'') FROM endpoint_reservations r JOIN applications a ON a.id=r.application_id WHERE a.application_key='${appKey}'`);
const resultFile=join(input.reportDir,'endpoint-domain.json');
// Reruns keep earlier phase evidence; completed phases are never repeated.
const result=(()=>{try{return {phases:{},...JSON.parse(readFileSync(resultFile,'utf8'))};}catch{return {phases:{}};}})();const saveResult=()=>writeFileSync(resultFile,JSON.stringify(result,null,2),{mode:0o600});
const typeClass=input.name,prefix=input.name;
const endpointType={key:'endpoint',inputs:[{name:'name',type:'string',required:true},{name:'port',type:'string',required:true},{name:'path',type:'string',required:false}],outputs:[{name:'hostname',type:'string',required:true,secret:false},{name:'url',type:'string',required:true,secret:false}]};
const workloadFor=name=>({apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'frontend',class:typeClass},containers:{main:{image:input.image,readinessProbe:{path:'/',port:80}}},resources:{public:{type:'endpoint',params:{name,port:'http',path:'/'}}},service:{ports:{http:{port:80,targetPort:80}}}});

async function importWorkload(dev,app,base,workload,exists){
 await dev.page.goto(dev.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/${exists?'frontend':'new'}`);
 if(!exists){const custom=dev.page.getByRole('button',{name:/Tự cấu hình/});if(await custom.count())await custom.click();}
 await dev.page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
 const file=join(input.stateDir,'workload.json');writeFileSync(file,JSON.stringify(workload),{mode:0o600});
 await dev.page.getByLabel('File Workload',{exact:true}).setInputFiles(file);await dev.page.getByText('Đã phân tích Workload: frontend',{exact:false}).waitFor();
 await dev.submit(base+'/workloads/frontend','PUT',dev.page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
}
async function routeOf(dev,namespace,hostname){
 const deadline=Date.now()+180000;
 for(;;){
  const routes=kube('-n',namespace,'get','httproute').items.filter(r=>r.metadata.labels?.['orchestrator.io/workload']==='frontend');
  const route=routes[0];const parent=route?.status?.parents?.find(p=>p.parentRef.name===input.gateway.name&&p.parentRef.namespace===input.gateway.namespace);
  if(routes.length===1&&route.spec.hostnames?.[0]===hostname&&['Accepted','ResolvedRefs'].every(type=>parent?.conditions?.some(c=>c.type===type&&c.status==='True'&&c.observedGeneration===route.metadata.generation)))return route;
  assert(Date.now()<deadline,'endpoint HTTPRoute not Accepted/ResolvedRefs for '+hostname);await dev.page.waitForTimeout(2000);
 }
}
// Each attempt uses a fresh context: a cached earlier 200 must never pass a 404 check.
async function browse(browser,target,expectStatus,check){
 const deadline=Date.now()+120000;
 for(;;){
  const context=await browser.newContext();const page=await context.newPage();
  try{
   const response=await page.goto(target,{waitUntil:'domcontentloaded'}).catch(()=>null);
   if(response?.status()===expectStatus&&!response.fromServiceWorker()){await check?.(page);return {context,page};}
   assert(Date.now()<deadline,`${target} did not answer ${expectStatus} (last ${response?.status()})`);
  }catch(error){await context.close();throw error;}
  await context.close();await new Promise(r=>setTimeout(r,2000));
 }
}

try{
 session=await openConsole(config.console,{role:'PLATFORM_ENGINEER'});const pe=makeUI(session.page,config.console.baseURL,config.runtime.timeoutMs);
 const account=(await pe.get('/driver-accounts')).accounts.find(a=>a.name===input.accountName);assert.equal(account?.status,'READY');
 const store=(await pe.get('/secret-stores')).secretStores.find(s=>s.name===input.storeName);assert(store,'Secret Store missing');
 const binding=await pe.get('/delivery-default');assert(binding.bindingId);
 const delivery=await pe.get(`/delivery-bindings/${binding.bindingId}?version=${binding.bindingVersion}`);assert.equal(delivery.managementAccountKey,account.key);
 rt.state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};rt.save();
 await ensureTypes(pe,[endpointType],rt);
 developer=await signInContext(session.browser,config.developer,{role:'DEVELOPER'});const dev=makeUI(developer.page,config.developer.baseURL,config.runtime.timeoutMs);
 const app=await ensureApplication(dev,config,rt);const base=envBase(app.key,'staging'),namespace=`app-${app.key}-staging`;
 const source=(await pe.get('/resource-definition-capabilities')).capabilities.find(c=>c.resourceType==='k8s-namespace'&&c.driverType==='terraform'&&c.source.sourceId.startsWith('git-'))?.source;assert(source,'Verified Git namespace module missing');
 const modulePin={sourceId:source.sourceId,moduleId:source.moduleId,revision:source.revision,subdirectory:source.subdirectory,contentSha256:source.contentSha256};
 const namespaceRef="${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
 config.platform.templateSources=[input.templateSource];
 config.platform.definitions=[
  {definition:{key:prefix+'-cluster',name:'Endpoint test cluster',resourceType:'k8s-cluster',driverType:'existing-cluster',driverAccount:'${account.key}',driverInputs:{values:{variables:{}}},provision:{},criteria:[{app_id:app.key,class:typeClass}]}},
  {source:{kind:'git',url:source.repositoryUrl,path:source.subdirectory,ref:source.revision,pin:modulePin},definition:{key:prefix+'-namespace',name:'Endpoint test namespace',resourceType:'k8s-namespace',driverType:'terraform',driverInputs:{values:{source:{source_id:modulePin.sourceId,module_id:modulePin.moduleId,revision:modulePin.revision,subdirectory:modulePin.subdirectory,content_sha256:modulePin.contentSha256},variables:{name:'${context.env.namespace}'},input_mapping:{},output_mapping:{name:'name'},provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.'+typeClass+'#environments.@app.@env'}}}},provision:{},criteria:[{app_id:app.key}]}},
  {definition:{key:prefix+'-workload',name:'Endpoint test workload template',resourceType:'workload',driverType:'template',driverInputs:{values:{variables:{template_source:'${templateSources.endpoint}',template_inputs:{},namespace:namespaceRef}}},provision:{},criteria:[{app_id:app.key,class:typeClass}]}},
  {definition:{key:prefix+'-endpoint',name:'Endpoint test public address',resourceType:'endpoint',driverType:'endpoint',driverInputs:{values:{variables:{rootDomain,scheme:url.protocol.slice(0,-1),gatewayName:input.gateway.name,gatewayNamespace:input.gateway.namespace,gatewaySectionName:input.gateway.listener}}},provision:{},criteria:[{app_id:app.key}]}},
 ];
 const sources=await ensureTemplateSources(pe,config,rt);await ensureDirectDefinitions(pe,config,account,sources,rt);
 await ensureEnvironment(dev,config,app,{key:store.key},config.application,rt);
 const rules=JSON.parse(readFileSync(new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url)));
 rules.rules.push({apiGroups:['gateway.networking.k8s.io'],resources:['httproutes'],verbs:['get','list','watch','create','update','patch','delete']});
 const rulesFile=join(input.stateDir,'fleet-access.json');writeFileSync(rulesFile,JSON.stringify(rules),{mode:0o600});
 config.platform.delivery.namespaceAccess={enabled:true,rulesFile};
 rt.state.phases??={};rt.save();
 let routeName;
 // Main flow: publish the real domain, rename it (old address stops routing,
 // claim moves), then restore the real domain for the user to open.
 for(const [phase,name] of [['publish',primary],['rename',input.renamedName],['restore',primary]]){
  if(rt.state.phases[phase]==='SUCCEEDED'){result.phases[phase]??='SUCCEEDED (earlier run)';continue;}
  const hostname=`${name}.${rootDomain}`,expected=`${url.protocol}//${hostname}/`;
  const workloads=(await dev.get(base+'/workloads')).workloads;assert(workloads.every(w=>w.id==='frontend'),'Unexpected workload');
  const current=workloads.find(w=>w.id==='frontend');
  if(!current||!equal(current.workload,workloadFor(name)))await importWorkload(dev,app,base,workloadFor(name),Boolean(current));
  await waitIdle(dev,app.key,'staging',config.runtime.timeoutMs);
  if(rt.state.deployAttempt)assert.equal(rt.state.deployAttempt.completion,'SUCCEEDED','Previous deployment requires operator reconciliation');
  if(rt.state.deployAttempt&&rt.state.deployAttempt.phase!==phase)delete rt.state.deployAttempt;
  // A deploy of this phase that already SUCCEEDED resumes at verification only.
  const resumed=rt.state.deployAttempt?.phase===phase&&rt.state.deployAttempt.completion==='SUCCEEDED';
  let before=null;
  if(!resumed){
  await dev.page.goto(dev.baseURL+`/ui/applications/${app.key}?environment=staging`);
  const preview=await dev.submit(base+'/preview','POST',dev.page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
  assert(preview.changes?.length===1&&preview.changes[0].workloadId==='frontend','Preview must change only frontend');
  assert.equal(preview.changes[0].endpoint?.url,expected);assert.equal(preview.changes[0].endpoint?.hostname,hostname);
  await dev.page.getByRole('status',{name:'Địa chỉ public',exact:true}).getByText(expected,{exact:true}).waitFor();
  await dev.page.screenshot({path:join(input.reportDir,`${phase}-preview.png`),fullPage:true});
  before=reservations(app.key);
  const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){rt.state.deployAttempt={phase,reviewRequestId:request.postDataJSON().reviewRequestId,completion:'UNKNOWN'};rt.save();}};
  dev.page.on('request',track);
  try{
   const [deployed]=await Promise.all([dev.submit(base+'/deploy','POST',dev.page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true})),ensureNamespaceAccess(dev,config,app,rt)]);
   rt.state.deployAttempt.completion=deployed.status;rt.save();assert.equal(deployed.status,'SUCCEEDED');
  }finally{dev.page.off('request',track);}
  }
  const after=reservations(app.key);assert.equal(after,`${hostname}|frontend`,'Only the current hostname stays reserved');
  const route=await routeOf(dev,namespace,hostname);
  if(routeName)assert.equal(route.metadata.name,routeName,'Route name is stable per owner');
  routeName=route.metadata.name;assert.match(routeName,/^endpoint-[0-9a-f]{20}$/);
  assert.equal(route.metadata.labels['orchestrator.io/application'],app.key);assert.equal(route.spec.parentRefs[0].sectionName,input.gateway.listener);
  assert.equal(route.spec.rules[0].backendRefs[0].name,'frontend');assert.equal(route.spec.rules[0].backendRefs[0].port,80);
  assert.equal(route.spec.rules[0].matches[0].path.value,'/');
  const latest=(await dev.get(base+'/deployments')).deployments[0];assert.equal(latest.status,'SUCCEEDED');const detail=await pe.get(base+'/deployments/'+latest.id);
  const proof=verifyApplicationFleet(kube('get','gitrepos.fleet.cattle.io','-A').items,kube('get','bundles.fleet.cattle.io','-A').items,app.key,detail.deployment.environmentId);
  const published=proof.bundles.some(bundle=>bundle.spec.resources?.some(file=>file.content?.includes('HTTPRoute')&&file.content.includes(hostname)));
  assert(published,'HTTPRoute must be in the actual Fleet-published manifest');
  // The real domain routes only while the endpoint holds it.
  const opened=name===primary
   ?await browse(session.browser,input.url,200,async page=>{await page.getByRole('heading',{name:'Welcome to nginx!',exact:true}).waitFor();assert.equal(new URL(page.url()).hostname,url.hostname);})
   :await browse(session.browser,input.url,404);
  await opened.page.screenshot({path:join(input.reportDir,`${phase}-browser.png`),fullPage:true});await opened.context.close();
  result.phases[phase]={hostname,reservationsBefore:before,reservationsAfter:after,deploymentId:latest.id,route:{name:route.metadata.name,uid:route.metadata.uid,generation:route.metadata.generation,hostnames:route.spec.hostnames},fleet:proof.repos.map(r=>({name:r.metadata.name,uid:r.metadata.uid,commit:r.status.commit}))};
  rt.state.phases[phase]='SUCCEEDED';delete rt.state.deployAttempt;rt.save();saveResult();rt.record('endpoint-domain:'+phase,'SUCCEEDED',{hostname});
 }
 result.status='PASS';result.url=input.url;result.applicationKey=app.key;result.namespace=namespace;result.checkedAt=new Date().toISOString();
 result.gatewayUid=kube('-n',input.gateway.namespace,'get','gateway',input.gateway.name).metadata.uid;saveResult();status='SUCCEEDED';
 console.log('PASS: endpoint reserved, routed through Fleet/Traefik, renamed and restored on the real domain');
}catch(error){rt.record('endpoint-domain','FAILED',{reason:error.code??error.name??'UNKNOWN'});throw error;}
finally{
 try{if(developer){await signOutContext(developer.context,config.developer);await developer.context.close();}}finally{try{if(session)await closeConsole(session,config.console);}finally{rt.close(status);}}
}
