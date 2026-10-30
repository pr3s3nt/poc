import type { DeploymentDetail } from '../deployments/api';
import type { DeploymentReviewData, ReviewNode, ReviewProgress } from './DeploymentReview';

// Candidate hashes can change per workload in a batch; the actual node pins must agree.
function pins(node: ReviewNode) {
  return JSON.stringify([node.kind, node.resourceType, node.class, node.definitionKey, node.definitionVersion,
    node.account, node.region, node.scope, node.placement?.cluster, node.placement?.account,
    node.placement?.region, node.placement?.namespace, node.placement?.source, node.module?.subdirectory, node.module?.sourceId, node.module?.module, node.module?.revision,
    node.module?.contentSha256, node.renderer, node.rendererDigest]);
}
export function correlateReviewProgress(review: DeploymentReviewData, details: DeploymentDetail[], operationStatus?: string): ReviewProgress[] {
  // A delivery-only transfer has no infrastructure/workload execution graph.
  // Keep its persisted operation visible without inventing node observations.
  if (review.version !== 1 || !Array.isArray(review.nodes)) return [];
  return review.nodes.map(node => {
    const unknown:ReviewProgress = { nodeId:node.id, state:'UNKNOWN', blockingProviderIds:[], observedAt:'' };
    const observations = details.flatMap(detail => {
      // A later workload's plan includes unchanged peers, but is not a new
      // execution observation of those peers. Keep their own exact run facts.
      if (node.kind === 'workload' && detail.deployment.workloadId !== node.name && !detail.workloads.some(w => w.workloadId === node.name)) return [];
      const observation = detail.nodeProgress?.find(p => p.nodeId === node.id);
      if (!observation) return [];
      const observedNode = detail.review?.nodes.find(n => n.id === node.id);
      return [{ observation, matches: observedNode !== undefined && pins(node) === pins(observedNode), startedAt: detail.deployment.startedAt, id: detail.deployment.id }];
    }).sort((a,b)=>b.startedAt.localeCompare(a.startedAt)||b.id.localeCompare(a.id));
    if (!observations.length) return { ...unknown, state:['SUCCEEDED','FAILED','RECOVERED'].includes(operationStatus??'')?'UNKNOWN':'PENDING' };
    // Never let an unrelated or contradictory node pin be hidden by an older success.
    if (observations.some(item=>!item.matches)) return unknown;
    return observations[0]?.observation ?? unknown;
  });
}
