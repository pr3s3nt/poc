import { test } from 'node:test';
import assert from 'node:assert/strict';
import { backendClaim } from './verification.mjs';
test('backend PVC proof requires explicit storage enablement and an actual writable claim mount',()=>{
 const deployment={spec:{template:{spec:{containers:[{name:'main',env:[{name:'ACCEPTANCE_STORAGE_DIR',value:'/data'}],volumeMounts:[{name:'data',mountPath:'/data'}]}],volumes:[{name:'data',persistentVolumeClaim:{claimName:'owned-data'}}]}}}};
 assert.equal(backendClaim(deployment),'owned-data');
 const main=deployment.spec.template.spec.containers[0];main.volumeMounts[0].readOnly=true;assert.throws(()=>backendClaim(deployment),/no managed PVC/);main.volumeMounts[0].readOnly=false;
 main.env=[];assert.throws(()=>backendClaim(deployment),/directory is not enabled/);
});
