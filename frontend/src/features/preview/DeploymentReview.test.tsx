import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { DeploymentReview, diffLines, type DeploymentReviewData } from './DeploymentReview';

const review: DeploymentReviewData = { version: 1, planHash: 'plan', nodes: [
  { id: 'cluster', kind: 'resource', name: 'Cluster', resourceType: 'k8s-cluster', class: 'internal', ownership: 'PREEXISTING', intent: 'RETAIN', placement: { cluster: 'cluster', account: 'internal', source: 'existing' } },
  { id: 'api', kind: 'workload', name: 'API', resourceType: 'workload', class: 'default', intent: 'UPDATE', renderer: 'score-k8s@0.15.0' },
], edges: [{ consumer: 'api', provider: 'cluster', reason: 'provider-binding' }], batches: [['cluster'], ['api']], artifacts: [{ id: 'api:deployment', nodeId: 'api', kind: 'kubernetes-manifest', path: 'Deployment/api', language: 'yaml', before: { availability: 'AVAILABLE', text: 'image: old\nreplicas: 1' }, after: { availability: 'PARTIAL', text: 'image: new\nreplicas: 2', redactedPaths: ['env.PASSWORD'], unknownPaths: [] } }] };

it('shows a bounded red and green code diff with line numbers and copies after-only text', async () => {
  const clipboard = vi.fn(async () => undefined);
  const user = userEvent.setup();
  Object.defineProperty(navigator, 'clipboard', { value: { writeText: clipboard }, configurable: true });
  render(<DeploymentReview review={review} />);
  await user.click(screen.getByRole('button', { name: /API/ }));
  await user.click(screen.getByRole('tab', { name: 'Code' }));
  const diff = screen.getByRole('table', { name: 'Diff Deployment/api' });
  expect(within(diff).getAllByText('−').length).toBeGreaterThan(0);
  expect(within(diff).getAllByText('+').length).toBeGreaterThan(0);
  expect(diff.querySelector('.review-diff-old')).toHaveTextContent('image: old');
  expect(diff.querySelector('.review-diff-new')).toHaveTextContent('image: new');
  await user.click(screen.getByRole('button', { name: 'Sao chép bản sau' }));
  expect(clipboard).toHaveBeenCalledWith('image: new\nreplicas: 2');
  await user.click(screen.getByRole('button', { name: 'Sau thay đổi' }));
  expect(screen.queryByRole('table', { name: 'Diff Deployment/api' })).not.toBeInTheDocument();
  expect(screen.getByText(/image: new/)).toBeInTheDocument();
});

it('declines quadratic diff work without discarding safe after text', () => {
  expect(diffLines('a\n'.repeat(800), 'b\n'.repeat(800))).toBeUndefined();
});

it('shows real blocking provider and unknown state without advancing it', async () => {
  const user = userEvent.setup();
  render(<DeploymentReview review={review} progress={[{ nodeId: 'cluster', state: 'FAILED', blockingProviderIds: [], observedAt: '2026-10-23T00:00:00Z' }, { nodeId: 'api', state: 'BLOCKED', blockingProviderIds: ['cluster'], observedAt: '2026-10-23T00:00:00Z' }]} />);
  await user.click(screen.getByRole('button', { name: /API/ }));
  expect(screen.getByText(/Đang chờ: cluster/)).toBeInTheDocument();
  expect(screen.getAllByText('Bị chặn').length).toBeGreaterThan(0);
});

it('draws only real edges and keeps managed resources outside Kubernetes', () => {
 const cloud = {...review, nodes:[...review.nodes,{id:'db',kind:'resource',name:'Aurora',resourceType:'postgres',class:'managed',account:'aws-demo',region:'us-east-1'}]};
 render(<DeploymentReview review={cloud}/>);
 expect(document.querySelectorAll('.review-edges>path')).toHaveLength(review.edges.length);
 const managed = screen.getByRole('button',{name:/Aurora/});
 expect(managed).toHaveAttribute('data-node-id','db');
 expect(document.querySelector('[data-cluster="outside"]')).toHaveTextContent('Hạ tầng ngoài Kubernetes');
});
it('renders removal lines even when the after side is absent', async () => {
 const user=userEvent.setup();
 const removed={...review,artifacts:review.artifacts.map(a=>({...a,after:{availability:'ABSENT' as const}}))};
 render(<DeploymentReview review={removed}/>);
 await user.click(screen.getByRole('button',{name:/API/}));await user.click(screen.getByRole('tab',{name:'Code'}));
 expect(screen.getByRole('table',{name:'Diff Deployment/api'}).querySelectorAll('.review-diff-old').length).toBeGreaterThan(0);
 expect(diffLines('', 'new')).toEqual([{type:'new',next:1,value:'new'}]);
});
it('provides keyboard tabs and reports failed clipboard access without an unhandled rejection', async () => {
 const user=userEvent.setup();Object.defineProperty(navigator,'clipboard',{value:{writeText:vi.fn(async()=>{throw Error('blocked');})},configurable:true});
 render(<DeploymentReview review={review}/>);await user.click(screen.getByRole('button',{name:/API/}));
 screen.getByRole('tab',{name:'Tổng quan'}).focus();await user.keyboard('{End}');expect(screen.getByRole('tab',{name:'Code'})).toHaveFocus();
 await user.click(screen.getByRole('button',{name:'Sao chép bản sau'}));expect(screen.getByRole('status')).toHaveTextContent('Không thể sao chép tự động');
});
