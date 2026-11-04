import { openConsole, closeConsole, signInContext, signOutContext, readPrivateFile } from './console.mjs';
import { createRuntime, makeUI, conflict, equal, waitIdle, digest } from './runtime.mjs';
import { requireConfig, BootstrapError } from './bootstrap-config.mjs';
import { bootstrapPlatform } from './platform.mjs';
import { ensureApplication } from './application.mjs';
import { ensureEnvironment, envBase } from './environment.mjs';
import { ensureWorkloads, desiredScores } from './workloads.mjs';
import { ensureDeployment } from './deployment.mjs';
import { ensureAccess, verifyAcceptance, kubectl } from './verification.mjs';

export function acceptanceInputHash(config) {
 const app=config.application;
 const value={name:app.name,subdomain:app.subdomain,environment:app.environment,variables:app.variables,workloads:desiredScores(config,app),secretHashes:Object.fromEntries(Object.entries(app.secretFiles).map(([name,file])=>[name,digest(readPrivateFile(file,'application secret'))])),jobPayload:config.verification.jobPayload};
 const canonical=v=>Array.isArray(v)?v.map(canonical):v&&typeof v==='object'?Object.fromEntries(Object.keys(v).sort().map(k=>[k,canonical(v[k])])):v;
 return digest(JSON.stringify(canonical(value)));
}
export async function assertDirectSelections(ui,config,app,rt,{applied=false}={}) {
 const selections=await ui.get(envBase(app.key,config.application.environment)+'/definition-versions');
 conflict(!selections.pending,'Pending Definition selection changes');
 for(const [key,pin]of Object.entries(rt.state.definitionPins)) {
  const row=selections.definitions.find(d=>d.key===key);
  conflict(row&&row.desiredRevisionId===pin.revisionId,`Definition ${key} desired revision`);
  if(applied&&row.inUse)conflict(row.appliedRevisionId===pin.revisionId&&row.actualRevisionIds?.length===1&&row.actualRevisionIds[0]===pin.revisionId,`Definition ${key} applied revision`);
 }
 if(applied)for(const row of selections.definitions.filter(d=>d.inUse))requireConfig(rt.state.definitionPins[row.key],'Acceptance selected an unconfigured infrastructure Definition');
}
export async function verifyDirectAcceptance(ui,config,app,rt) {
 const base=envBase(app.key,config.application.environment);
 conflict(!(await ui.get(base+'/catalog')).catalog.desired,'Acceptance Application selected a Catalog');
 await assertDirectSelections(ui,config,app,rt,{applied:true});
 const workloads=(await ui.get(base+'/workloads')).workloads,want=desiredScores(config,config.application);
 conflict(workloads.length===3&&workloads.every(w=>!w.state&&w.ready&&equal(w.workload,want[w.id])),'Acceptance Workload content/readiness');
 const deployments=(await ui.get(base+'/deployments')).deployments;
 requireConfig(deployments.length>0,'No acceptance deployment');
 const latest=deployments[0];requireConfig(latest.status==='SUCCEEDED','Latest acceptance deployment did not succeed');
 const detail=await ui.get(base+'/deployments/'+latest.id);
 for(const name of Object.keys(want)) {
  const rendered=detail.rendering?.[name];requireConfig(rendered?.driverType==='template','Acceptance must use the template driver');
  const pin=rt.state.definitionPins?.[rendered.definitionKey];requireConfig(pin,'Acceptance selected an unconfigured workload Definition');
  const source=Object.values(rt.state.templateSources??{}).find(s=>s.id===rendered.bundle?.id);
  requireConfig(source&&rendered.bundle?.source?.contentSha256===source.pin.contentSha256,'Acceptance template content pin mismatch');
 }
 const repos=kubectl(config,'get','gitrepos.fleet.cattle.io','-A').items.filter(r=>r.metadata.labels?.['orchestrator.io/environment-id']===detail.deployment.environmentId);
 const bundles=kubectl(config,'get','bundles.fleet.cattle.io','-A').items;
 requireConfig(repos.length>0,'No acceptance Fleet repository');
 for(const repo of repos) {
  requireConfig(/^[a-f0-9]{40}$/.test(repo.spec.revision)&&repo.status?.commit===repo.spec.revision,'Fleet commit does not match delivery pin');
  const owned=bundles.filter(b=>b.metadata.namespace===repo.metadata.namespace&&b.metadata.labels?.['fleet.cattle.io/repo-name']===repo.metadata.name);
  requireConfig(owned.length===1&&owned[0].metadata.labels['fleet.cattle.io/commit']===repo.spec.revision&&owned[0].status?.summary?.desiredReady===1&&owned[0].status.summary.ready===1,'Fleet Bundle not ready at pinned revision');
 }
 const snapshot={deploymentIds:deployments.map(d=>d.id).sort(),fleet:repos.map(r=>({uid:r.metadata.uid,revision:r.spec.revision})).sort((a,b)=>a.uid.localeCompare(b.uid))};
 if(rt.state.acceptanceSnapshot)conflict(equal(snapshot,rt.state.acceptanceSnapshot),'Acceptance deployment/Fleet identities changed');
 rt.state.acceptanceSnapshot=snapshot;rt.save();rt.record('acceptance-cd-fleet','REUSED',snapshot);
}
export async function runDirectBootstrap(config,{preflight}) {
 const rt=createRuntime(config);let platform,developer,status='FAILED';
 try {
  if(config.runtime.mode==='all') {const hash=acceptanceInputHash(config);conflict(!rt.state.acceptanceInputHash||rt.state.acceptanceInputHash===hash,'Acceptance input changed; direct bootstrap does not upgrade or overwrite pending changes');rt.state.acceptanceInputHash=hash;rt.save();}
  platform=await openConsole(config.console);requireConfig(['ADMIN','PLATFORM_ENGINEER'].includes(platform.user.role),'Platform Engineer/Admin required');
  const ui=makeUI(platform.page,config.console.baseURL,config.runtime.timeoutMs);
  await preflight(ui,config,rt);
  if(config.runtime.mode==='check'){status='CHECKED';return rt.report;}
  const resources=await bootstrapPlatform(ui,config,rt);
  rt.state.bindingId=resources.binding.id;rt.state.bindingVersion=resources.binding.version;
  const delivery=await ui.get(`/delivery-bindings/${resources.binding.id}?version=${resources.binding.version}`);
  rt.state.deliverySubject={name:delivery.config.serviceAccount,namespace:delivery.config.agentNamespace};rt.save();
  rt.record('platform','BOOTSTRAPPED');
  if(config.runtime.mode==='bootstrap'){status='BOOTSTRAPPED';return rt.report;}
  developer=await signInContext(platform.browser,config.developer,{role:'DEVELOPER'});
  const dev=makeUI(developer.page,config.developer.baseURL,config.runtime.timeoutMs);
  const app=await ensureApplication(dev,config,rt);
  const base=envBase(app.key,config.application.environment);
  conflict(!(await dev.get(base+'/catalog')).catalog.desired,'Existing acceptance Application has a Catalog');
  // An unknown deploy result must be reconciled, never retried as a new deployment.
  await waitIdle(dev,app.key,config.application.environment,config.runtime.timeoutMs);
  if(rt.state.deployAttempt?.completion==='UNKNOWN') {
   const deployments=(await dev.get(base+'/deployments')).deployments;
   const known=deployments.find(d=>d.reviewRequestId===rt.state.deployAttempt.reviewRequestId);
   requireConfig(known?.status==='SUCCEEDED','Unknown deploy outcome; inspect server before resuming');
   rt.state.deployAttempt.completion='SUCCEEDED';rt.save();
  }
  await assertDirectSelections(dev,config,app,rt);
  await ensureEnvironment(dev,config,app,resources.store,config.application,rt);
  await ensureWorkloads(dev,config,app,config.application,rt);
  await ensureDeployment(dev,config,app,rt);
  // Platform Engineer reads immutable render metadata; developer cannot list platform sources.
  await verifyDirectAcceptance(ui,config,app,rt);
  await ensureAccess(dev,config,app,rt);
  await verifyAcceptance(dev,config,rt);
  rt.record('acceptance','SUCCEEDED',{appKey:app.key,environment:config.application.environment,url:config.verification.url});
  status='SUCCEEDED';return rt.report;
 }catch(error){rt.record('run','FAILED',{reason:error instanceof BootstrapError?error.message:'Operation failed; inspect Console and private evidence; no automatic replay',location:error.stack?.match(/scripts\/bootstrap\/[^\s)]+:\d+:\d+/)?.[0]});throw error;}
 finally {
  try { if(developer){try{await signOutContext(developer.context,config.developer);}finally{await developer.context.close();}} }
  finally { try{if(platform)await closeConsole(platform,config.console);}finally{rt.close(status);} }
 }
}
