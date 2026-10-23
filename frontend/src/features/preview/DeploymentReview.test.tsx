import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { DeploymentReview, diffLines, type DeploymentReviewData } from './DeploymentReview';

const review: DeploymentReviewData = { version: 1, planHash: 'plan', nodes: [
  { id: 'cluster', kind: 'resource', name: 'Cluster', resourceType: 'k8s-cluster', class: 'internal', ownership: 'PREEXISTING', intent: 'RETAIN', placement: { cluster: 'cluster', account: 'internal', source: 'existing' } },
  { id: 'api', kind: 'workload', name: 'API', resourceType: 'workload', class: 'default', intent: 'UPDATE', renderer: 'score-k8s@0.15.0' },
], edges: [{ consumer: 'api', provider: 'cluster', reason: 'provider-binding' }], batches: [['cluster'], ['api']], artifacts: [{ id: 'api:deployment', nodeId: 'api', kind: 'kubernetes-manifest', path: 'Deployment/api', language: 'yaml', before: { availability: 'AVAILABLE', text: 'image: old\nreplicas: 1' }, after: { availability: 'PARTIAL', text: 'image: new\nreplicas: 2', redactedPaths: ['env.PASSWORD'], unknownPaths: [] } }] };

it.each([
  { version: 0, planHash: '', nodes: null, edges: null, batches: null, artifacts: null },
  { version: 2 },
  ...(['nodes', 'edges', 'batches', 'artifacts'] as const).map((key) => ({ ...review, [key]: null })),
])('keeps unavailable wire reviews from crashing the page', (wireReview) => {
  render(<DeploymentReview review={wireReview as unknown as DeploymentReviewData} />);
  expect(screen.getByText('Bản review của kế hoạch này chưa có.')).toBeVisible();
  expect(screen.queryByLabelText('Sơ đồ triển khai')).not.toBeInTheDocument();
});

it('can move from an unavailable review to a valid plan and back without changing hook order', () => {
  const zero = { version: 0, nodes: null, edges: null, batches: null, artifacts: null } as unknown as DeploymentReviewData;
  const { rerender } = render(<DeploymentReview review={zero} />);
  rerender(<DeploymentReview review={review} />);
  expect(screen.getByLabelText('Sơ đồ triển khai')).toBeVisible();
  rerender(<DeploymentReview />);
  expect(screen.getByText('Bản review của kế hoạch này chưa có.')).toBeVisible();
});

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

it('can isolate the selected node connections and restore the complete graph', async () => {
  const user = userEvent.setup();
  const plan = { ...review, nodes: [...review.nodes, { id: 'db', kind: 'resource', name: 'Database', resourceType: 'postgres', class: 'default' }], edges: [...review.edges, { provider: 'cluster', consumer: 'db', reason: 'provider-binding' }] };
  render(<DeploymentReview review={plan} />);
  const visible = () => [...document.querySelectorAll<SVGPathElement>('.review-edges>path')].filter(path => path.style.display !== 'none');
  expect(visible()).toHaveLength(2);
  await user.click(screen.getByLabelText('Chỉ đường nối của node đang chọn'));
  expect(visible()).toHaveLength(1);
  expect(visible()[0]).toHaveAttribute('data-consumer', 'api');
  await user.click(screen.getByLabelText('Chỉ đường nối của node đang chọn'));
  expect(visible()).toHaveLength(2);
});

it('shows real blocking provider and unknown state without advancing it', async () => {
  const user = userEvent.setup();
  render(<DeploymentReview review={review} progress={[{ nodeId: 'cluster', state: 'FAILED', blockingProviderIds: [], observedAt: '2026-10-23T00:00:00Z' }, { nodeId: 'api', state: 'BLOCKED', blockingProviderIds: ['cluster'], observedAt: '2026-10-23T00:00:00Z' }]} />);
  await user.click(screen.getByRole('button', { name: /API/ }));
  expect(screen.getByText(/Đang chờ: Cluster/)).toBeInTheDocument();
  expect(screen.getAllByText('Bị chặn').length).toBeGreaterThan(0);
});

it('draws only real edges and keeps managed resources outside Kubernetes', () => {
 const cloud = {...review, nodes:[...review.nodes,{id:'db',kind:'resource',name:'Aurora',resourceType:'postgres',class:'managed',account:'aws-demo',region:'us-east-1'}]};
 render(<DeploymentReview review={cloud}/>);
 expect(document.querySelectorAll('.review-edges>path')).toHaveLength(review.edges.length);
 const managed = screen.getByRole('button',{name:/Aurora/});
 expect(managed).toHaveAttribute('data-node-id','db');
 expect(document.querySelector('[data-cluster="outside"]')).toHaveTextContent('Hạ tầng ngoài Kubernetes');
 expect(document.querySelector('[data-cluster="unresolved"]')).toHaveTextContent('Vị trí chưa xác định');
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

it('keeps the selected file scoped to its node and presents one artifact at a time', async () => {
  const user = userEvent.setup();
  const multiple = { ...review, artifacts: [...review.artifacts, { ...review.artifacts[0]!, id: 'api:service', path: 'Service/api', after: { availability: 'PARTIAL' as const, text: 'kind: Service' } }] };
  render(<DeploymentReview review={multiple} />);
  await user.click(screen.getByRole('tab', { name: 'Code' }));
  expect(screen.getByRole('table', { name: 'Diff Deployment/api' })).toBeInTheDocument();
  expect(screen.queryByRole('table', { name: 'Diff Service/api' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Service/api' }));
  expect(screen.getByRole('table', { name: 'Diff Service/api' })).toBeInTheDocument();
  expect(screen.queryByRole('table', { name: 'Diff Deployment/api' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: /Cluster/ }));
  await user.click(screen.getByRole('tab', { name: 'Code' }));
  expect(screen.getByText('Chưa có code an toàn cho nút này.')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: /API/ }));
  await user.click(screen.getByRole('tab', { name: 'Code' }));
  expect(screen.getByRole('table', { name: 'Diff Deployment/api' })).toBeInTheDocument();
});
