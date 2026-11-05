import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { backendClaim, verifyBackendStorage } from './verification.mjs';
test('backend PVC proof requires explicit storage enablement and an actual writable claim mount',()=>{
 const deployment={spec:{template:{spec:{containers:[{name:'main',env:[{name:'ACCEPTANCE_STORAGE_DIR',value:'/data'}],volumeMounts:[{name:'data',mountPath:'/data'}]}],volumes:[{name:'data',persistentVolumeClaim:{claimName:'owned-data'}}]}}}};
 assert.equal(backendClaim(deployment),'owned-data');
 const main=deployment.spec.template.spec.containers[0];main.volumeMounts[0].readOnly=true;assert.throws(()=>backendClaim(deployment),/no managed PVC/);main.volumeMounts[0].readOnly=false;
 main.env=[];assert.throws(()=>backendClaim(deployment),/directory is not enabled/);
});

test('a missing previously verified backend file is a failure, never reseeded',async()=>{
 const value='retained-file',hash=createHash('sha256').update(value).digest('hex');let writes=0;
 const page={locator:()=>({count:async()=>0}),getByLabel:()=>{writes++;throw Error('unexpected write');}};
 const config={runtime:{profile:'console'},application:{environment:'staging'},verification:{storage:{value}}};
 const rt={state:{appKey:'app',backendStorage:{verification:'browser-file',hash}},save(){},record(){}};
 await assert.rejects(()=>verifyBackendStorage(page,config,rt),/Baseline file missing after upgrade/);assert.equal(writes,0);
});
