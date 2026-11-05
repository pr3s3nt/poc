import test from 'node:test';
import assert from 'node:assert/strict';
import {verifyApplicationFleet} from './fleet-verification.mjs';
const commit='a'.repeat(40);
function fixture(){
 const pins={staging:{path:'environments/staging/backend/rollout',environmentId:'stage-id',revision:'stage-rev',digest:'b'.repeat(64)},production:{path:'environments/production/backend/rollout',environmentId:'prod-id',revision:'prod-rev',digest:'c'.repeat(64)}};
 const repo={metadata:{name:'app',namespace:'fleet-local',labels:{'orchestrator.io/application-id':'app-id'},annotations:{'orchestrator.io/unit-pins':JSON.stringify(pins)}},spec:{revision:commit,paths:Object.values(pins).map(p=>p.path)},status:{commit}};
 const bundles=Object.keys(pins).map(release=>({metadata:{namespace:'fleet-local',labels:{'fleet.cattle.io/repo-name':'app','fleet.cattle.io/commit':commit}},spec:{helm:{releaseName:release}},status:{summary:{desiredReady:1,ready:1}}}));
 return {repo,bundles};
}
test('one Application controller proves both Environment bundles at a common SHA',()=>{
 const {repo,bundles}=fixture();const proof=verifyApplicationFleet([repo],bundles,'app-id','stage-id');assert.equal(proof.repos.length,1);assert.equal(proof.bundles.length,2);assert.deepEqual(proof.units.map(u=>u.revision),['stage-rev']);
});
test('rejects duplicate controllers, stale sibling Bundles and missing Environment',()=>{
 const {repo,bundles}=fixture();assert.throws(()=>verifyApplicationFleet([repo,repo],bundles,'app-id','stage-id'),/exactly one/);
 assert.throws(()=>verifyApplicationFleet([repo],bundles,'app-id','foreign-id'),/No admitted/);
 bundles[1].metadata.labels['fleet.cattle.io/commit']='d'.repeat(40);assert.throws(()=>verifyApplicationFleet([repo],bundles,'app-id','stage-id'),/not ready/);
});
