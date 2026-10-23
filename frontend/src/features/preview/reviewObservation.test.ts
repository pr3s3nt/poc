import {expect,it} from 'vitest';
import {correlateReviewProgress} from './reviewObservation';
import type {DeploymentDetail} from '../deployments/api';
import type {DeploymentReviewData} from './DeploymentReview';
const review:DeploymentReviewData={version:1,planHash:'pending',nodes:[{id:'db',kind:'resource',name:'db',resourceType:'postgres',class:'managed',definitionKey:'pg',definitionVersion:2}],edges:[],batches:[],artifacts:[]};
const detail=(version:number,state:string,start:string)=>({deployment:{id:start,startedAt:start,applicationKey:'app',environmentKey:'staging',action:'DEPLOY',workloadId:'api',actorRef:'test',status:'PROVISIONING'},deploymentSetId:'set',planHash:'plan',resources:[],workloads:[],review:{...review,nodes:review.nodes.map(n=>({...n,definitionVersion:version}))},nodeProgress:[{nodeId:'db',state,blockingProviderIds:[],observedAt:''}]} as DeploymentDetail);
it('does not display unrelated node success or mask conflicting pins',()=>{
 expect(correlateReviewProgress(review,[detail(1,'READY','2026-01-02')])[0]?.state).toBe('UNKNOWN');
 expect(correlateReviewProgress(review,[detail(2,'READY','2026-01-01'),detail(1,'FAILED','2026-01-02')])[0]?.state).toBe('UNKNOWN');
});
it('uses the exact latest scoped observation without allowing earlier success to win',()=>{
 expect(correlateReviewProgress(review,[detail(2,'READY','2026-01-01'),detail(2,'FAILED','2026-01-02')])[0]?.state).toBe('FAILED');
 expect(correlateReviewProgress(review,[],'FAILED')[0]?.state).toBe('UNKNOWN');
 expect(correlateReviewProgress(review,[],'ACTIVE')[0]?.state).toBe('PENDING');
});

it('rejects a different renderer digest even when the bundle version is equal',()=>{
 const desired={...review,nodes:review.nodes.map(n=>({...n,renderer:'bundle@1',rendererDigest:'expected'}))};
 const observed=detail(2,'READY','2026-01-01');observed.review={...desired,nodes:desired.nodes.map(n=>({...n,rendererDigest:'different'}))};
 expect(correlateReviewProgress(desired,[observed])[0]?.state).toBe('UNKNOWN');
});

it('keeps the applied workload observation when a later batch peer has no snapshot for it',()=>{
 const desired={...review,nodes:review.nodes.map(n=>({...n,kind:'workload',name:'api'}))};
 const own=detail(2,'READY','2026-01-01');own.deployment.workloadId='api';own.review=desired;
 const peer=detail(2,'UNKNOWN','2026-01-02');peer.deployment.workloadId='web';peer.review=desired;
 expect(correlateReviewProgress(desired,[own,peer],'SUCCEEDED')[0]?.state).toBe('READY');
 own.nodeProgress![0]!.state='UNKNOWN';
 expect(correlateReviewProgress(desired,[own,peer],'FAILED')[0]?.state).toBe('UNKNOWN');
});
