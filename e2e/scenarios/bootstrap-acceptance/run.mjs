import { parseArgs } from 'node:util';
import { spawnSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve, join } from 'node:path';
import { isDeepStrictEqual } from 'node:util';
import { loadBootstrapConfig, requireConfig } from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import { openConsole, closeConsole } from '../../lib/console.mjs';
import { makeUI } from '../../../scripts/bootstrap/lib/runtime.mjs';
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
  return {appKey:app.key,environment:app.environments,configuration,workloads,resources:resources.map(r=>({id:r.id,status:r.status})).sort((a,b)=>a.id.localeCompare(b.id)),deployments:deployments.map(d=>d.id??d.key).sort(),catalog,definitions};
 }finally{await closeConsole(s,config.console);}
}
function run(){const r=spawnSync(process.execPath,[new URL('../../../scripts/bootstrap/run.mjs',import.meta.url).pathname,'--config',configFile],{stdio:'inherit'});requireConfig(r.status===0,'Bootstrap run failed; inspect report before resuming');}
run();const first=await snapshot();run();const second=await snapshot();
requireConfig(isDeepStrictEqual(first,second),'Idempotency failed: server state differs after unchanged rerun');
// Store only safe identities/counts, not configuration values or API bodies.
writeFileSync(join(config.runtime.reportDir,`idempotency-${Date.now()}.json`),JSON.stringify({status:'PASS',appKey:first.appKey,resources:first.resources,deploymentCount:first.deployments.length,checks:['bootstrap and configured upgrade','business checks','unchanged rerun: no revision/deployment/resource changes']} ,null,2),{mode:0o600});
console.log('PASS: bootstrap/upgrade and unchanged rerun preserve server configuration, IDs and deployment count');
