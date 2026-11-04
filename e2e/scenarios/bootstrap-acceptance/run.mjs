import { randomUUID } from 'node:crypto';
import { parseArgs } from 'node:util';
import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { loadBootstrapConfig, requireConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { openConsole, closeConsole } from '../../lib/console.mjs';
import { makeUI, outsideRepo } from '../../../scripts/bootstrap/lib/runtime.mjs';
const {values}=parseArgs({options:{config:{type:'string'}}});
requireConfig(values.config,'--config required');
const configFile=resolve(values.config),config=loadBootstrapConfig(configFile);
requireConfig(config.environment==='dev','This mutating acceptance runner currently accepts dev only');
async function snapshot(){
 const s=await openConsole(config.console);
 try{
  const ui=makeUI(s.page,config.console.baseURL,config.runtime.timeoutMs);
  const app=(await ui.get('/applications')).applications.find(a=>a.name===config.application.name&&a.subdomain===config.application.subdomain);
  requireConfig(app,'Acceptance Application missing');
  const base=`/applications/${app.key}/environments/${config.application.environment}`;
  const configuration=await ui.get(base+'/configuration'),workloads=await ui.get(base+'/workloads');
  const resources=(await ui.get(base+'/resources')).resources??[];
  const deployments=(await ui.get(base+'/deployments')).deployments??[];
  const catalog=await ui.get(base+'/catalog'),definitions=await ui.get(base+'/definition-versions');
  const platform = config.schemaVersion===3 ? {
   definitions:(await ui.get('/resource-definitions')).resourceDefinitions,
   histories:await Promise.all(config.platform.definitions.map(async entry=>{const {history}=await ui.get(`/resource-definitions/${entry.definition.key}/versions`);return {key:entry.definition.key,activeId:history.activeId,generation:history.generation,revisions:history.versions.map(v=>v.id)};})),
   sources:(await ui.get('/template-sources')).templateSources.map(s=>({id:s.id,source:s.source})),
   accounts:(await ui.get('/driver-accounts')).accounts.map(a=>({key:a.key,version:a.version})),
   bindings:(await ui.get('/delivery-bindings')).bindings.map(b=>({id:b.id,version:b.version,revision:b.revision,status:b.status})),
   default:await ui.get('/delivery-default')
  } : null;
  if(config.schemaVersion===3) requireConfig(!catalog.catalog.desired,'Direct acceptance must not select Catalog');
  return {platform,appKey:app.key,environment:app.environments,configuration,workloads,resources:resources.map(r=>({id:r.id,status:r.status})).sort((a,b)=>a.id.localeCompare(b.id)),deployments:deployments.map(d=>d.id??d.key).sort(),catalog,definitions};
 }finally{await closeConsole(s,config.console);}
}
function run(file=configFile){const r=spawnSync(process.execPath,[new URL('../../../scripts/bootstrap/run.mjs',import.meta.url).pathname,'--config',file],{stdio:'inherit'});requireConfig(r.status===0,'Bootstrap run failed; inspect report before resuming');}
if(config.schemaVersion===3) {
 outsideRepo(dirname(configFile));
 const raw=JSON.parse(readFileSync(configFile,'utf8'));
 // Keep relative file resolution by placing the private partial config beside the original.
 raw.runtime.mode='bootstrap';delete raw.application;delete raw.developer;delete raw.verification;
 const partial=join(dirname(configFile),`platform-only-${randomUUID()}.json`);
 writeFileSync(partial,JSON.stringify(raw,null,2),{mode:0o600,flag:'wx'});run(partial);
}
run();const first=await snapshot();run();const second=await snapshot();
requireConfig(isDeepStrictEqual(first,second),'Idempotency failed: server state differs after unchanged rerun');
// Store only safe identities/counts, not configuration values or API bodies.
writeFileSync(join(config.runtime.reportDir,`idempotency-${Date.now()}.json`),JSON.stringify({status:'PASS',appKey:first.appKey,resources:first.resources,deploymentCount:first.deployments.length,checks:[config.schemaVersion===3?'platform-only → resume full acceptance without Catalog':'bootstrap and configured upgrade','business checks','unchanged rerun: no revision/deployment/resource changes']} ,null,2),{mode:0o600});
console.log('PASS: bootstrap/acceptance and unchanged rerun preserve server configuration, IDs and deployment count');
