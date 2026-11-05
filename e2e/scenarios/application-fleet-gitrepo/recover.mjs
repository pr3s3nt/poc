import assert from 'node:assert/strict';
import {parseArgs} from 'node:util';
import {readFileSync} from 'node:fs';
import {loadBootstrapConfig} from '../../../scripts/bootstrap/lib/bootstrap-config.mjs';
import {openConsole,closeConsole} from '../../lib/console.mjs';
import {makeUI} from '../../../scripts/bootstrap/lib/runtime.mjs';
const {values:a}=parseArgs({options:{config:{type:'string'},operation:{type:'string'},'prior-stopped':{type:'boolean'}}});
assert(a.config&&a.operation&&a['prior-stopped'],'Require exact config, operation and verified stopped prior execution');
const c=loadBootstrapConfig(a.config),s=JSON.parse(readFileSync(c.runtime.stateDir+'/identity.json'));
const p=await openConsole(c.console);
try{
 const u=makeUI(p.page,c.console.baseURL,c.runtime.timeoutMs),base=`/applications/${s.appKey}/environments/${c.application.environment}`;
 const op=(await u.get(base+'/operations')).operations.find(o=>o.id===a.operation);
 assert(op?.status==='INTERRUPTED'&&op.recoverable,'Exact interrupted recoverable operation required');
 await p.page.goto(`${c.console.baseURL}/ui/applications/${s.appKey}/settings?environment=${c.application.environment}`);
 const banner=p.page.locator('.operation-banner');await banner.getByRole('checkbox').check();
 const result=await u.submit(base+'/operations/'+op.id+'/recover','POST',banner.getByRole('button',{name:'Khôi phục môi trường',exact:true}));
 console.log(JSON.stringify(result));
 const after=(await u.get(base+'/operations')).operations.find(o=>o.id===op.id);assert.equal(after.status,'RECOVERED');
 console.log('PASS: original stopped operation recovered via Console');
}finally{await closeConsole(p,c.console);}
