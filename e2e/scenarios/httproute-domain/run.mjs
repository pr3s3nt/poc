import assert from 'node:assert/strict';
import {parseArgs} from 'node:util';
import {readFileSync,writeFileSync,statSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {execFileSync} from 'node:child_process';
import {openConsole,closeConsole,readConsoleConfig,signInContext,signOutContext} from '../../lib/console.mjs';
import {createRuntime,makeUI,equal,digest,waitIdle} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureApplication} from '../../../scripts/bootstrap/lib/application.mjs';
import {ensureTemplateSources,ensureDirectDefinitions} from '../../../scripts/bootstrap/lib/direct-platform.mjs';
import {ensureEnvironment,envBase} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureNamespaceAccess} from '../../../scripts/bootstrap/lib/namespace-access.mjs';
import {openWorkloadEvidence} from '../../../scripts/bootstrap/lib/video.mjs';
const {values}=parseArgs({options:{config:{type:'string'}}});assert(values.config,'Private --config required');
assert.equal(statSync(values.config).mode&0o077,0,'Config must be private');
const input=JSON.parse(readFileSync(values.config,'utf8'));
assert(/^[a-z0-9-]{1,40}$/.test(input.name));
assert(input.console.url===input.developer.url,'Use one Console');
const auth=account=>readConsoleConfig({ORCH_PLATFORM_URL:account.url,ORCH_PLATFORM_USERNAME:account.username,ORCH_PLATFORM_PASSWORD_FILE:account.passwordFile,ORCH_HEADLESS:'1'});
const config={schemaVersion:3,environment:'dev',console:auth(input.console),developer:auth(input.developer),runtime:{profile:'dev-test',mode:'all',headless:true,timeoutMs:600000,stateDir:input.stateDir,reportDir:input.reportDir,clusterKubeconfigFile:input.kubeconfigFile,recordVideo:true},platform:{account:{name:input.accountKey,context:input.context,endpoint:input.endpoint},delivery:{}},application:{name:input.name,subdomain:input.name,environment:'staging',variables:{},secretFiles:{},definitionRevisions:{}},verification:{url:input.url}};
const url=new URL(input.url);assert(!['localhost','127.0.0.1','0.0.0.0'].includes(url.hostname));assert.equal(url.pathname,'/');
const rt=createRuntime(config);let session,developer,evidence,status='FAILED';
const identity={name:input.name,url:input.url,gateway:input.gateway,source:input.templateSource,image:input.image,account:input.accountKey};
if(rt.state.testInputHash)assert.equal(rt.state.testInputHash,digest(JSON.stringify(identity)),'Test input changed');
rt.state.testInputHash=digest(JSON.stringify(identity));rt.save();
const kube=(...args)=>JSON.parse(execFileSync('kubectl',['--kubeconfig',input.kubeconfigFile,'--context',input.context,...args,'-o','json'],{encoding:'utf8',timeout:30000,stdio:['ignore','pipe','pipe']}));
const saveResult=value=>writeFileSync(join(input.reportDir,'httproute-domain.json'),JSON.stringify(value,null,2),{mode:0o600});
try{
 session=await openConsole(config.console,{role:'PLATFORM_ENGINEER'});const pe=makeUI(session.page,config.console.baseURL,config.runtime.timeoutMs);
 const account=(await pe.get('/driver-accounts')).accounts.find(a=>a.key===input.accountKey);assert.equal(account?.status,'READY');
 const binding=await pe.get('/delivery-default');assert(binding.bindingId);
 const delivery=await pe.get(`/delivery-bindings/${binding.bindingId}?version=${binding.bindingVersion}`);assert.equal(delivery.managementAccountKey,account.key);
 rt.state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};rt.save();
 developer=await signInContext(session.browser,config.developer,{role:'DEVELOPER'});const dev=makeUI(developer.page,config.developer.baseURL,config.runtime.timeoutMs);
 const app=await ensureApplication(dev,config,rt);const base=envBase(app.key,'staging'),namespace=`app-${app.key}-staging`;
 const typeClass=input.name;const prefix=input.name;
 const source=(await pe.get('/resource-definition-capabilities')).capabilities.find(c=>c.resourceType==='k8s-namespace'&&c.driverType==='terraform'&&c.source.sourceId.startsWith('git-'))?.source;assert(source);
 const modulePin={sourceId:source.sourceId,moduleId:source.moduleId,revision:source.revision,subdirectory:source.subdirectory,contentSha256:source.contentSha256};
 const namespaceRef="${resources['k8s-namespace.default#environments.@app.@env'].outputs.name}";
 config.platform.templateSources=[input.templateSource];
 config.platform.definitions=[
  {definition:{key:prefix+'-cluster',name:'HTTPRoute test cluster',resourceType:'k8s-cluster',driverType:'existing-cluster',driverAccount:'${account.key}',driverInputs:{values:{variables:{}}},provision:{},criteria:[{app_id:app.key,class:typeClass}]}},
  {source:{kind:'git',url:source.repositoryUrl,path:source.subdirectory,ref:source.revision,pin:modulePin},definition:{key:prefix+'-namespace',name:'HTTPRoute test namespace',resourceType:'k8s-namespace',driverType:'terraform',driverInputs:{values:{source:{source_id:modulePin.sourceId,module_id:modulePin.moduleId,revision:modulePin.revision,subdirectory:modulePin.subdirectory,content_sha256:modulePin.contentSha256},variables:{name:'${context.env.namespace}'},input_mapping:{},output_mapping:{name:'name'},provider_bindings:{kubernetes:{target:'resource',resource_ref:'k8s-cluster.'+typeClass+'#environments.@app.@env'}}}},provision:{},criteria:[{app_id:app.key}]}},
  {definition:{key:prefix+'-workload',name:'HTTPRoute Git workload template',resourceType:'workload',driverType:'template',driverInputs:{values:{variables:{template_source:'${templateSources.route}',template_inputs:{httpRouteEnabled:true,httpRouteGatewayName:input.gateway.name,httpRouteGatewayNamespace:input.gateway.namespace,httpRouteGatewaySectionName:input.gateway.listener},namespace:namespaceRef}}},provision:{},criteria:[{app_id:app.key,class:typeClass}]}}
 ];
 const sources=await ensureTemplateSources(pe,config,rt);await ensureDirectDefinitions(pe,config,account,sources,rt);
 await ensureEnvironment(dev,config,app,{key:input.storeKey},config.application,rt);
 const workload={apiVersion:'orchestrator.dev/workload/v1',metadata:{name:'frontend',class:typeClass,annotations:{'routing.orchestrator.dev/hostname':url.hostname,'routing.orchestrator.dev/path':'/','routing.orchestrator.dev/service-port':'http'}},containers:{main:{image:input.image,readinessProbe:{path:'/',port:80}}},resources:{},service:{ports:{http:{port:80,targetPort:80}}}};
 const existing=(await dev.get(base+'/workloads')).workloads;assert(existing.every(w=>w.id==='frontend'),'Unexpected workload');
 const found=existing.find(w=>w.id==='frontend');
 if(found)assert(equal(found.workload,workload),'Existing test workload differs');
 else{
  await dev.page.goto(dev.baseURL+`/ui/applications/${app.key}/environments/staging/workloads/new`);
  const custom=dev.page.getByRole('button',{name:/Tự cấu hình/});if(await custom.count())await custom.click();
  await dev.page.getByRole('button',{name:'Nhập Workload',exact:true}).click();
  const file=join(input.stateDir,'workload.json');writeFileSync(file,JSON.stringify(workload),{mode:0o600});
  await dev.page.getByLabel('File Workload',{exact:true}).setInputFiles(file);await dev.page.getByText('Đã phân tích Workload: frontend',{exact:false}).waitFor();
  await dev.submit(base+'/workloads/frontend','PUT',dev.page.getByRole('button',{name:'Lưu draft Workload',exact:true}));
 }
 await waitIdle(dev,app.key,'staging',config.runtime.timeoutMs);
 if(rt.state.deployAttempt)assert.equal(rt.state.deployAttempt.completion,'SUCCEEDED','Previous deployment requires operator reconciliation');
 await dev.page.goto(dev.baseURL+`/ui/applications/${app.key}?environment=staging`);
 const preview=await dev.submit(base+'/preview','POST',dev.page.getByRole('button',{name:'Xem trước thay đổi',exact:true}));
 if(preview.changes?.length||preview.deliveryPending||preview.routePending){
  assert(!rt.state.deployed,'Previously completed test has unexpected pending changes');
  assert(preview.changes?.length===1&&preview.changes[0].workloadId==='frontend'&&preview.changes[0].action!=='REMOVE');
  const rules=JSON.parse(readFileSync(new URL('../../../scripts/bootstrap/applications/acceptance/fleet-access.json',import.meta.url)));
  rules.rules.push({apiGroups:['gateway.networking.k8s.io'],resources:['httproutes'],verbs:['get','list','watch','create','update','patch','delete']});
  const rulesFile=join(input.stateDir,'fleet-access.json');writeFileSync(rulesFile,JSON.stringify(rules),{mode:0o600});
  config.platform.delivery.namespaceAccess={enabled:true,rulesFile};
  const track=request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/api/v1'+base+'/deploy'){rt.state.deployAttempt={reviewRequestId:request.postDataJSON().reviewRequestId,completion:'UNKNOWN'};rt.save();}};
  dev.page.on('request',track);
  try{
   const [result]=await Promise.all([dev.submit(base+'/deploy','POST',dev.page.getByRole('button',{name:'Triển khai các thay đổi này',exact:true})),ensureNamespaceAccess(dev,config,app,rt)]);
   rt.state.deployAttempt.completion=result.status;rt.save();assert.equal(result.status,'SUCCEEDED');rt.state.deployed=true;rt.save();
  }finally{dev.page.off('request',track);}
 }
 const latest=(await dev.get(base+'/deployments')).deployments[0];assert.equal(latest.status,'SUCCEEDED');const detail=await pe.get(base+'/deployments/'+latest.id);
 assert.equal(detail.rendering.frontend.driverType,'template');assert.equal(detail.rendering.frontend.definitionKey,prefix+'-workload');assert.equal(detail.rendering.frontend.bundle.source.contentSha256,input.templateSource.pin.contentSha256);
 const deadline=Date.now()+180000;let route;
 for(;;){
  route=kube('-n',namespace,'get','httproute','frontend');
  const parent=route.status?.parents?.find(p=>p.parentRef.name===input.gateway.name&&p.parentRef.namespace===input.gateway.namespace);
  if(['Accepted','ResolvedRefs'].every(type=>parent?.conditions?.some(c=>c.type===type&&c.status==='True'&&c.observedGeneration===route.metadata.generation)))break;
  assert(Date.now()<deadline,'HTTPRoute not Accepted/ResolvedRefs at current generation');await dev.page.waitForTimeout(2000);
 }
 assert.equal(route.metadata.labels['orchestrator.io/application'],app.key);assert.equal(route.metadata.labels['orchestrator.io/workload'],'frontend');
 assert.equal(route.spec.hostnames[0],url.hostname);assert.equal(route.spec.parentRefs[0].sectionName,input.gateway.listener);
 assert.equal(route.spec.rules[0].backendRefs[0].name,'frontend');assert.equal(route.spec.rules[0].backendRefs[0].port,80);
 const service=kube('-n',namespace,'get','service','frontend'),deployment=kube('-n',namespace,'get','deployment','frontend');assert(deployment.status.availableReplicas>=1);assert.equal(service.spec.ports[0].port,80);
 const repos=kube('get','gitrepos.fleet.cattle.io','-A').items.filter(r=>r.metadata.labels?.['orchestrator.io/environment-id']===detail.deployment.environmentId);assert(repos.length);
 const bundles=kube('get','bundles.fleet.cattle.io','-A').items;
 let publishedRoute;
 for(const repo of repos){assert.match(repo.spec.revision,/^[a-f0-9]{40}$/);assert.equal(repo.status.commit,repo.spec.revision);const owned=bundles.filter(b=>b.metadata.namespace===repo.metadata.namespace&&b.metadata.labels?.['fleet.cattle.io/repo-name']===repo.metadata.name);assert.equal(owned.length,1);assert.equal(owned[0].metadata.labels['fleet.cattle.io/commit'],repo.spec.revision);assert.equal(owned[0].status.summary.ready,1);assert.equal(owned[0].status.summary.desiredReady,1);const routeFile=owned[0].spec.resources?.find(file=>file.content?.includes('HTTPRoute')&&file.content.includes(url.hostname));if(routeFile)publishedRoute={bundle:owned[0].metadata.name,namespace:owned[0].metadata.namespace,file:routeFile.name,commit:repo.spec.revision};}
 assert(publishedRoute,'HTTPRoute must be present in the actual Fleet-published manifest');
 evidence=await openWorkloadEvidence(session.browser,config,'httproute-domain');const page=evidence.page;
 const response=await page.goto(input.url);assert.equal(response.status(),200);await page.getByRole('heading',{name:'Welcome to nginx!',exact:true}).waitFor();assert.equal(new URL(page.url()).hostname,url.hostname);
 await page.screenshot({path:join(input.reportDir,'httproute-domain.png'),fullPage:true});await page.waitForTimeout(1500);
 const result={status:'PASS',url:input.url,checkedAt:new Date().toISOString(),applicationKey:app.key,namespace,namespaceUid:kube('get','namespace',namespace).metadata.uid,deploymentId:latest.id,route:{name:route.metadata.name,uid:route.metadata.uid,generation:route.metadata.generation,spec:route.spec,status:route.status},service:{name:service.metadata.name,uid:service.metadata.uid,port:service.spec.ports[0].port},gateway:input.gateway,source:input.templateSource,fleet:repos.map(r=>({name:r.metadata.name,namespace:r.metadata.namespace,uid:r.metadata.uid,commit:r.status.commit})),publishedRoute,browser:{httpStatus:response.status(),heading:'Welcome to nginx!',hostOverride:false,dnsOverride:false}};
 saveResult(result);rt.state.verified=true;rt.save();status='SUCCEEDED';rt.record('httproute-domain','SUCCEEDED',{url:input.url,routeUid:route.metadata.uid});console.log('PASS: real HTTPRoute domain, Traefik Gateway, exact Fleet commit and browser content');
}catch(error){rt.record('httproute-domain','FAILED',{reason:error.code??error.name??'UNKNOWN'});throw error;}
finally{
 try{if(evidence)await evidence.close(status);}finally{try{if(developer){await signOutContext(developer.context,config.developer);await developer.context.close();}}finally{try{if(session)await closeConsole(session,config.console);}finally{rt.close(status);}}}
}
