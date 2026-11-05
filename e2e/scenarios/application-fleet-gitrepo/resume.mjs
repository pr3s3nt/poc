import assert from 'node:assert/strict';
import {parseArgs} from 'node:util';
import {loadBootstrapConfig} from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import {openConsole,closeConsole,signInContext,signOutContext} from '../../lib/console.mjs';
import {makeUI,createRuntime,waitIdle} from '../../../scripts/bootstrap/lib/runtime.mjs';
import {ensureEnvironment} from '../../../scripts/bootstrap/lib/environment.mjs';
import {ensureWorkloads} from '../../../scripts/bootstrap/lib/workloads.mjs';
import {ensureDeployment} from '../../../scripts/bootstrap/lib/deployment.mjs';
import {verifyDirectAcceptance} from '../../../scripts/bootstrap/lib/direct-runner.mjs';
import {ensureAccess,verifyAcceptance} from '../../../scripts/bootstrap/lib/verification.mjs';
const {values:a}=parseArgs({options:{config:{type:'string'},prepare:{type:'boolean'}}});assert(a.config);
const c=loadBootstrapConfig(a.config),rt=createRuntime(c);let p,d,status='FAILED';
try{
 p=await openConsole(c.console);const pu=makeUI(p.page,c.console.baseURL,c.runtime.timeoutMs);
 d=await signInContext(p.browser,c.developer,{role:'DEVELOPER'});const u=makeUI(d.page,c.console.baseURL,c.runtime.timeoutMs);
 const app=(await u.get('/applications/'+rt.state.appKey)).application;assert.equal(app.name,c.application.name);
 await waitIdle(u,app.key,c.application.environment,c.runtime.timeoutMs);
 if(a.prepare){
  assert(!rt.state.acceptanceSnapshot,'Prepare only an unverified acceptance Environment');
  const stores=(await pu.get('/secret-stores')).secretStores;const matches=stores.filter(s=>s.name===c.platform.secretStore.name);assert.equal(matches.length,1);
  await ensureEnvironment(u,c,app,matches[0],c.application,rt);await ensureWorkloads(u,c,app,c.application,rt);
 }
 await ensureDeployment(u,c,app,rt);await verifyDirectAcceptance(pu,c,app,rt);await ensureAccess(u,c,app,rt);await verifyAcceptance(u,c,rt);
 status='SUCCEEDED';console.log('PASS: recovered acceptance Environment deployed and verified');
}finally{if(d){await signOutContext(d.context,c.developer);await d.context.close();}if(p)await closeConsole(p,c.console);rt.close(status);}
