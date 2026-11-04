import { isDevTest } from './execution-profile.mjs';
import { preflightDirect } from './direct-platform.mjs';
import { runDirectBootstrap } from './direct-runner.mjs';
import { openConsole, closeConsole, signInContext, signOutContext } from './console.mjs';
import { createRuntime, makeUI, conflict, equal, waitIdle } from './runtime.mjs';
import { requireConfig, BootstrapError } from './bootstrap-config.mjs';
import { bootstrapPlatform, assertStore, assertDelivery } from './platform.mjs';
import { ensureApplication } from './application.mjs';
import { ensureEnvironment, envBase } from './environment.mjs';
import { ensureWorkloads, desiredScores } from './workloads.mjs';
import { ensureDeployment } from './deployment.mjs';
import { recoverCompletedDeploy } from './recovery.mjs';
import { inspectCluster, ensureAccess, verifyAcceptance, kubectl } from './verification.mjs';
import { assertSameResourceType } from './catalog-config.mjs';
import { assertAccountTarget } from './infrastructure-config.mjs';

export async function preflight(ui,config,rt) {
 const checks=[['console-health',async()=>ui.get('/healthz')]];
 if(isDevTest(config))checks.push(['cluster-capabilities',async()=>inspectCluster(config)]);
 else rt.record('verification-scope','REUSED',{profile:'console',nativeClusterChecks:false});
 const p=config.platform;
 checks.push(['existing-config',async()=>{
  const accounts=(await ui.get('/driver-accounts')).accounts;
  const matches=accounts.filter(v=>v.name===p.account.name);conflict(matches.length<=1,'Account ambiguous');if(matches[0])assertAccountTarget(matches[0],p.account);
  const types=(await ui.get('/resource-types')).resourceTypes;
  for(const t of p.resourceTypes){const found=types.find(v=>v.key===t.key);if(found)assertSameResourceType(found,t);}
  const stores=(await ui.get('/secret-stores')).secretStores??[];
  const store=stores.filter(v=>v.name===p.secretStore.name);conflict(store.length<=1,'Secret Store ambiguous');if(store[0])assertStore(store[0],p.secretStore);
  if(config.schemaVersion===3) {const bindings=(await ui.get('/delivery-bindings')).bindings;const binding=bindings.find(b=>b.name===p.delivery.name);if(binding){requireConfig(matches[0],'CD account target missing');assertDelivery(binding,await ui.get(`/delivery-bindings/${binding.id}?version=${binding.version}`),p.delivery,matches[0]);}}
  const providers=(await ui.get('/delivery-providers')).providers;requireConfig(providers.some(v=>v.kind===p.delivery.provider),'CD provider disabled or unavailable on backend');if(config.schemaVersion===3) await preflightDirect(ui,config,rt);else await ui.get('/platform/catalog/sources');
 }]);
 let blocked=false;
 for(const [name,fn]of checks){try{await fn();rt.record(name,'REUSED');}catch(e){blocked=true;rt.record(name,'BLOCKED',{reason:e instanceof BootstrapError?e.message:'Prerequisite mismatch; inspect configured target'});}}
 requireConfig(!blocked,'Preflight blocked; see report');
}
async function verifyPins(ui,config,app,version) {
 const base=envBase(app.key,config.application.environment),catalog=(await ui.get(base+'/catalog')).catalog;
 conflict(catalog.desired?.sourceId===config.platform.catalog.id&&catalog.desired.tag===version.catalogTag,'Environment Catalog target');
 const actual=(await ui.get(base+'/workloads')).workloads,want=desiredScores(config,version);
 conflict(actual.length===3&&actual.every(v=>!v.state&&equal(v.workload,want[v.id])),'Workload version target');
 for(const [key,id]of Object.entries(version.definitionRevisions??{}))conflict((await ui.get(base+'/definition-versions')).definitions.find(v=>v.key===key)?.desiredRevisionId===id,'Definition target pin');
}
export async function runBootstrap(config) {
 if(config.schemaVersion===3)return runDirectBootstrap(config,{preflight});
 const rt=createRuntime(config);let platform,developer;let status='FAILED';
 try {
  platform=await openConsole(config.console);requireConfig(['ADMIN','PLATFORM_ENGINEER'].includes(platform.user.role),'Platform Engineer/Admin required');
  const ui=makeUI(platform.page,config.console.baseURL,config.runtime.timeoutMs);
  await preflight(ui,config,rt);
  if(config.runtime.mode==='check'){status='CHECKED';return rt.report;}
  if(config.runtime.mode==='verify') {
   const apps=(await ui.get('/applications')).applications;
   const app=apps.find(v=>v.key===(config.application.expectedKey??rt.state.appKey));
   requireConfig(app,'Verify requires an existing confirmed Application key');
   const base=envBase(app.key,config.application.environment);
   const actual=(await ui.get(base+'/workloads')).workloads,target=desiredScores(config,config.upgrade);
   const atTarget=actual.length===3&&actual.every(v=>equal(v.workload,target[v.id]));
   await verifyPins(ui,config,app,atTarget?config.upgrade:config.application);
   if(config.verification.enabled)await verifyAcceptance(ui,config,rt,{submitJob:false});
   status='VERIFIED';return rt.report;
  }
  const resources=await bootstrapPlatform(ui,config,rt);
  rt.state.bindingId=resources.binding.id;rt.state.bindingVersion=resources.binding.version;
  const deliveryDetails=await ui.get(`/delivery-bindings/${resources.binding.id}?version=${resources.binding.version}`);
  rt.state.deliverySubject={name:deliveryDetails.config.serviceAccount,namespace:deliveryDetails.config.agentNamespace};rt.save();
  if(config.runtime.mode==='bootstrap'){status='BOOTSTRAPPED';return rt.report;}
  developer=await signInContext(platform.browser,config.developer,{role:'DEVELOPER'});
  const dev=makeUI(developer.page,config.developer.baseURL,config.runtime.timeoutMs);
  const app=await ensureApplication(dev,config,rt);
  await recoverCompletedDeploy(ui,config,app,rt);
  const base=envBase(app.key,config.application.environment);
  // Never downgrade to baseline on a rerun: server-applied Catalog and exact scores determine phase.
  const catalog=(await dev.get(base+'/catalog')).catalog;
  const scores=(await dev.get(base+'/workloads')).workloads;
  const targetScores=desiredScores(config,config.upgrade);
  const atTarget= catalog.desired?.tag===config.upgrade.catalogTag && scores.length===3&&scores.every(v=>!v.state&&equal(v.workload,targetScores[v.id]))&&catalog.status==='APPLIED';
  const baselineScores=desiredScores(config,config.application);
  const upgradeInProgress=catalog.desired?.tag===config.upgrade.catalogTag&&config.upgrade.catalogTag!==config.application.catalogTag || scores.some(v=>!equal(v.workload,baselineScores[v.id])&&equal(v.workload,targetScores[v.id]));
  if(upgradeInProgress&&!atTarget)requireConfig(config.upgrade.enabled,'Upgrade partially selected; enable upgrade to resume');
  if(config.runtime.mode==='verify') {
   await verifyPins(dev,config,app,atTarget?config.upgrade:config.application);
   await ensureAccess(dev,config,app,rt);if(config.verification.enabled)await verifyAcceptance(dev,config,rt,{submitJob:false});
  } else {
   if(!atTarget && !upgradeInProgress && config.runtime.mode!=='upgrade') {
    await ensureEnvironment(dev,config,app,resources.store,config.application,rt);
    await ensureWorkloads(dev,config,app,config.application,rt);
    await ensureDeployment(dev,config,app,rt);
    await verifyPins(dev,config,app,config.application);
    await ensureAccess(dev,config,app,rt);
    if(config.verification.enabled)await verifyAcceptance(dev,config,rt);
   }
   if(config.upgrade.enabled && (['all','upgrade'].includes(config.runtime.mode)||upgradeInProgress)) {
    await ensureEnvironment(dev,config,app,resources.store,config.upgrade,rt,{upgrade:true});
    await ensureWorkloads(dev,config,app,config.upgrade,rt,{upgrade:true});
    await ensureDeployment(dev,config,app,rt);
    await verifyPins(dev,config,app,config.upgrade);
    await ensureAccess(dev,config,app,rt);
    if(config.verification.enabled)await verifyAcceptance(dev,config,rt,{submitJob:false});
   } else if(atTarget) {
    await verifyPins(dev,config,app,config.upgrade);await ensureAccess(dev,config,app,rt);
    if(config.verification.enabled)await verifyAcceptance(dev,config,rt,{submitJob:false});
   } else rt.record('upgrade','SKIPPED');
  }
  const data=(await dev.get(base+'/resources')).resources??[];
  const ids=data.map(v=>v.id).sort();
  if(rt.state.resourceIds)conflict(rt.state.resourceIds.every(id=>ids.includes(id)),'Resource identities changed');
  rt.state.resourceIds=ids;rt.save();
  rt.record('handoff','REUSED',{consoleUrl:`${config.console.baseURL}/ui/applications/${app.key}?environment=${config.application.environment}`,applicationUrl:config.verification.url});
  status='SUCCEEDED';return rt.report;
 } catch(e) {
  rt.record('run','FAILED',{reason:e instanceof BootstrapError?e.message:'Operation failed; inspect Console. No automatic replay.',location:e.stack?.match(/scripts\/bootstrap\/[^\s)]+:\d+:\d+/)?.[0]});throw e;
 } finally {
  if(developer){await signOutContext(developer.context,config.developer);await developer.context.close();}
  if(platform)await closeConsole(platform,config.console);
  rt.close(status);
 }
}
