import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import type { Application } from '../../shared/types/application';
import { DeploymentDetailsPage } from './DeploymentDetailsPage';
import { DeploymentHistoryPage } from './DeploymentHistoryPage';
import { RecentDeployments } from './RecentDeployments';
import { bothConfigured } from '../../test/targets';

const application: Application = { id: 'catalog/app', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };
const deployment = (overrides: Record<string, unknown> = {}) => ({
  id: 'dep-1', applicationKey: 'catalog/app', environmentKey: 'staging', workloadId: 'frontend',
  action: 'DEPLOY', status: 'SUCCEEDED', actorRef: 'developer', startedAt: '2026-09-01T10:00:00Z',
  ...overrides,
});
const detail = (overrides: Record<string, unknown> = {}) => ({
  deployment: deployment(), deploymentSetId: 'set-1', planHash: '0123456789abcdef',
  graph: { nodes: [{ descriptor: 'db', kind: 'resource' }], edges: [] },
  matches: { db: { definitionKey: 'pg-kind', driverType: 'fake' } }, batches: [['db']],
  resources: [{ descriptor: 'db', resourceType: 'postgres', definitionKey: 'pg-kind', status: 'READY', outputs: { host: 'db.internal', password: '***redacted***' } }],
  workloads: [{ workloadId: 'frontend', status: 'RUNNING', manifestDigest: 'sha256:dep-1-manifest', lastDeploymentId: 'dep-1', observedAt: '2026-09-01T10:01:00Z' }],
  ...overrides,
});

afterEach(() => vi.unstubAllGlobals());

const historyEntries = () => screen.getAllByRole('button').filter((button) => button.className === 'deployment-entry');

it('loads scoped history in Vietnamese and sends the original enum to the server filter', async () => {
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('?status=FAILED')) return Response.json({ deployments: [deployment({ id: 'dep-2', workloadId: 'backend', action: 'UPDATE', status: 'FAILED' })] });
    return Response.json({ deployments: [deployment()] });
  });
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="staging" />);

  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  expect(screen.getByRole('status')).toHaveTextContent('Đang tải lịch sử triển khai…');
  expect(await screen.findByText('frontend')).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Lịch sử triển khai', level: 1 })).toBeInTheDocument();
  expect(fetcher.mock.calls[0]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments');
  expect(screen.getByRole('button', { name: 'Tải lại' })).toBeInTheDocument();

  const select = screen.getByRole('combobox', { name: 'Trạng thái' });
  expect(Array.from(select.querySelectorAll('option')).map((option) => [option.value, option.textContent])).toEqual([
    ['ALL', 'Tất cả'], ['SUCCEEDED', 'Thành công'], ['FAILED', 'Thất bại'], ['DEPLOYING', 'Đang triển khai'],
    ['PROVISIONING', 'Đang tạo tài nguyên'], ['PLANNING', 'Đang lập kế hoạch'],
  ]);
  await userEvent.setup().selectOptions(select, 'FAILED');
  expect(await screen.findByText('backend')).toBeInTheDocument();
  expect(screen.queryByText('frontend')).not.toBeInTheDocument();
  expect(historyEntries()[0]).toHaveTextContent('backend · cập nhật');
  expect(historyEntries()[0]).toHaveTextContent('Thất bại');
  expect(fetcher.mock.calls[1]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments?status=FAILED');
});

it.each([
  ['SUCCEEDED', 'DEPLOY', 'Thành công', 'triển khai'],
  ['FAILED', 'UPDATE', 'Thất bại', 'cập nhật'],
  ['DEPLOYING', 'REMOVE', 'Đang triển khai', 'gỡ bỏ'],
  ['PROVISIONING', 'DEPLOY', 'Đang tạo tài nguyên', 'triển khai'],
  ['PLANNING', 'UPDATE', 'Đang lập kế hoạch', 'cập nhật'],
])('shows %s/%s as Vietnamese labels without leaking enums', async (status, action, statusText, actionText) => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ deployments: [deployment({ status, action })] })));
  render(<DeploymentHistoryPage application={application} environment="staging" />);
  await screen.findByText('frontend');
  const entry = historyEntries()[0] as HTMLElement;
  expect(entry).toHaveTextContent(statusText);
  expect(entry).toHaveTextContent(`frontend · ${actionText}`);
  expect(entry).not.toHaveTextContent(status);
  expect(entry).not.toHaveTextContent(action);
});

it('formats time with the Vietnamese locale and labels a missing actor', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ deployments: [deployment({ actorRef: '' }), deployment({ id: 'dep-2', startedAt: '2026-09-01T10:00:00Z', actorRef: 'developer' })] })));
  render(<DeploymentHistoryPage application={application} environment="staging" />);
  await screen.findAllByText(/Không rõ người thực hiện/);
  const expected = new Date('2026-09-01T10:00:00Z').toLocaleString('vi-VN');
  expect(historyEntries()[0]).toHaveTextContent(`${expected} · Không rõ người thực hiện`);
  expect(historyEntries()[1]).toHaveTextContent(`${expected} · developer`);
});

it('keeps server order and opens the selected deployment id in the detail route', async () => {
  const items = [deployment({ id: 'dep/z', workloadId: 'zeta' }), deployment({ id: 'dep-a', workloadId: 'alpha' }), deployment({ id: 'dep-m', workloadId: 'mid' })];
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ deployments: items })));
  render(<DeploymentHistoryPage application={application} environment="production" />);
  await screen.findByText('zeta');
  expect(historyEntries().map((entry) => entry.querySelector('strong')?.textContent)).toEqual(['zeta', 'alpha', 'mid']);
  await userEvent.setup().click(historyEntries()[2] as HTMLElement);
  expect(window.location.pathname).toBe('/ui/applications/catalog%2Fapp/environments/production/deployments/dep-m');
  await userEvent.setup().click(historyEntries()[0] as HTMLElement);
  expect(window.location.pathname).toBe('/ui/applications/catalog%2Fapp/environments/production/deployments/dep%2Fz');
});

it('distinguishes empty history from a safe, retryable loading error', async () => {
  const fetcher = vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'token=raw-backend-secret' }), { status: 503 }))
    .mockResolvedValueOnce(Response.json({ deployments: [] }));
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="production" />);

  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải lịch sử triển khai.');
  expect(screen.queryByText(/Chưa có triển khai/)).not.toBeInTheDocument();
  expect(document.body).not.toHaveTextContent('raw-backend-secret');
  await userEvent.setup().click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText('Chưa có triển khai nào trong môi trường này.')).toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it('shows a Vietnamese filtered empty message and re-reads the stored history on Tải lại', async () => {
  const fetcher = vi.fn(async (_input: string | URL | Request, _init?: RequestInit) => Response.json({ deployments: [] }));
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="staging" />);
  await screen.findByText('Chưa có triển khai nào trong môi trường này.');
  await userEvent.setup().selectOptions(screen.getByRole('combobox', { name: 'Trạng thái' }), 'PROVISIONING');
  expect(await screen.findByText('Chưa có triển khai nào ở trạng thái “Đang tạo tài nguyên”.')).toBeInTheDocument();
  expect(document.body).not.toHaveTextContent('PROVISIONING');
  await userEvent.setup().click(screen.getByRole('button', { name: 'Tải lại' }));
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3));
  expect(fetcher.mock.calls[2]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments?status=PROVISIONING');
  expect(fetcher.mock.calls.every(([, init]) => init?.method === undefined || init.method === 'GET')).toBe(true);
});

it('clears old rows while loading and ignores late replies from an old filter or Environment', async () => {
  const pending: Record<string, (value: Response) => void> = {};
  vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/staging/deployments')) return Promise.resolve(Response.json({ deployments: [deployment({ id: 'dep-all', workloadId: 'all-row' })] }));
    return new Promise<Response>((done) => { pending[url] = done; });
  }));
  const view = render(<DeploymentHistoryPage application={application} environment="staging" />);
  await screen.findByText('all-row');
  await userEvent.setup().selectOptions(screen.getByRole('combobox', { name: 'Trạng thái' }), 'FAILED');
  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  expect(screen.queryByText('all-row')).not.toBeInTheDocument();
  view.rerender(<DeploymentHistoryPage application={application} environment="production" />);
  const prodKey = Object.keys(pending).find((key) => key.includes('/production/')) as string;
  await act(async () => { pending[prodKey]?.(Response.json({ deployments: [deployment({ id: 'dep-p', workloadId: 'prod-row', environmentKey: 'production', status: 'FAILED' })] })); });
  expect(await screen.findByText('prod-row')).toBeInTheDocument();
  const stagingKey = Object.keys(pending).find((key) => key.includes('/staging/') && key.includes('FAILED')) as string;
  await act(async () => { pending[stagingKey]?.(Response.json({ deployments: [deployment({ id: 'dep-s', workloadId: 'stale-staging-row' })] })); });
  expect(screen.queryByText('stale-staging-row')).not.toBeInTheDocument();
  expect(screen.getByText('prod-row')).toBeInTheDocument();
});

it('ignores a late reply of an old filter after returning to the same Application list', async () => {
  let releaseFailed: (value: Response) => void = () => undefined;
  const fetcher = vi.fn((input: string | URL | Request) => String(input).endsWith('?status=FAILED')
    ? new Promise<Response>((done) => { releaseFailed = done; })
    : Promise.resolve(Response.json({ deployments: [deployment({ id: 'dep-all', workloadId: 'all-row' })] })));
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="staging" />);
  await screen.findByText('all-row');
  const user = userEvent.setup();
  const select = screen.getByRole('combobox', { name: 'Trạng thái' });
  await user.selectOptions(select, 'FAILED');
  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  expect(screen.queryByText('all-row')).not.toBeInTheDocument();
  await user.selectOptions(select, 'ALL');
  expect(await screen.findByText('all-row')).toBeInTheDocument();
  await act(async () => { releaseFailed(Response.json({ deployments: [deployment({ id: 'dep-late', workloadId: 'late-failed', status: 'FAILED' })] })); });
  expect(screen.queryByText('late-failed')).not.toBeInTheDocument();
  expect(screen.getByText('all-row')).toBeInTheDocument();
  expect(select).toHaveValue('ALL');
  expect(screen.queryByRole('status')).not.toBeInTheDocument();
  expect(fetcher.mock.calls.map(([url]) => String(url))).toEqual([
    '/api/v1/applications/catalog%2Fapp/environments/staging/deployments',
    '/api/v1/applications/catalog%2Fapp/environments/staging/deployments?status=FAILED',
    '/api/v1/applications/catalog%2Fapp/environments/staging/deployments',
  ]);
});

it('ignores a late reply and a late failure of the previous Application after switching Application', async () => {
  const other: Application = { ...application, id: 'billing/app', name: 'Billing' };
  const pending: Record<string, { resolve: (value: Response) => void; reject: (reason?: unknown) => void }> = {};
  vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('billing%2Fapp')) return Promise.resolve(Response.json({ deployments: [deployment({ id: 'dep-b', applicationKey: 'billing/app', workloadId: 'billing-row' })] }));
    return new Promise<Response>((resolve, reject) => { pending[url] = { resolve, reject }; });
  }));
  const view = render(<DeploymentHistoryPage application={application} environment="staging" />);
  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  const oldKey = Object.keys(pending)[0] as string;
  expect(oldKey).toContain('catalog%2Fapp');
  view.rerender(<DeploymentHistoryPage application={other} environment="staging" />);
  expect(await screen.findByText('billing-row')).toBeInTheDocument();
  await act(async () => { pending[oldKey]?.resolve(Response.json({ deployments: [deployment({ id: 'dep-old', workloadId: 'catalog-stale-row' })] })); });
  expect(screen.queryByText('catalog-stale-row')).not.toBeInTheDocument();
  expect(screen.getByText('billing-row')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: '← Billing' })).toBeInTheDocument();
  expect(historyEntries()).toHaveLength(1);
  // A late failure of the old Application must not raise an alert either.
  view.rerender(<DeploymentHistoryPage application={application} environment="production" />);
  const prodKey = Object.keys(pending).find((key) => key.includes('/production/')) as string;
  view.rerender(<DeploymentHistoryPage application={other} environment="production" />);
  expect(await screen.findByText('billing-row')).toBeInTheDocument();
  await act(async () => { pending[prodKey]?.reject(new Error('offline')); });
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(screen.getByText('billing-row')).toBeInTheDocument();
});

it('ignores a late failure from an obsolete scope', async () => {
  let rejectOld: (reason?: unknown) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => String(input).includes('/staging/')
    ? new Promise<Response>((_done, reject) => { rejectOld = reject; })
    : Promise.resolve(Response.json({ deployments: [deployment({ id: 'dep-p', workloadId: 'prod-row' })] }))));
  const view = render(<DeploymentHistoryPage application={application} environment="staging" />);
  view.rerender(<DeploymentHistoryPage application={application} environment="production" />);
  expect(await screen.findByText('prod-row')).toBeInTheDocument();
  await act(async () => { rejectOld(new Error('offline')); });
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(screen.getByText('prod-row')).toBeInTheDocument();
});

it('uses the same scoped endpoint and explicit states for recent deployments', async () => {
  const fetcher = vi.fn(async (_input: string | URL | Request) => Response.json({ deployments: [] }));
  vi.stubGlobal('fetch', fetcher);
  render(<RecentDeployments applicationId="catalog/app" environment="staging" />);
  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  expect(await screen.findByText('Chưa có triển khai nào trong staging.')).toBeInTheDocument();
  expect(fetcher.mock.calls[0]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments');
});

it('shows persisted detail, redacted output and never renders resolved inputs', async () => {
  const fetcher = vi.fn(async (_input: string | URL | Request) => Response.json(detail({
    resources: [{ descriptor: 'db', resourceType: 'postgres', definitionKey: 'pg-kind', status: 'READY', outputs: { host: 'db.internal', password: '***redacted***' }, resolvedInputs: { password: 'raw-secret' } }],
  })));
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentDetailsPage application={application} environment="staging" deploymentId="dep/1" />);

  expect(screen.getByRole('status')).toHaveTextContent('Loading deployment…');
  expect(await screen.findAllByText(/pg-kind/)).toHaveLength(2);
  expect(screen.getByText(/password=\*\*\*redacted\*\*\*/)).toBeInTheDocument();
  expect(screen.queryByText('raw-secret')).not.toBeInTheDocument();
  expect(screen.getByText('sha256:dep-1-manifest')).toBeInTheDocument();
  expect(fetcher.mock.calls[0]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments/dep%2F1');
});

it('shows planning failure without inventing a provision plan', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(detail({
    deployment: deployment({ id: 'dep-3', workloadId: 'api', status: 'FAILED', failureReason: 'no matching definition' }),
    deploymentSetId: '', planHash: '', graph: null, matches: null, batches: null, resources: [], workloads: [],
  }))));
  render(<DeploymentDetailsPage application={application} environment="staging" deploymentId="dep-3" />);

  expect(await screen.findByText('no matching definition')).toBeInTheDocument();
  expect(screen.getByText('No provision plan was saved for this deployment.')).toBeInTheDocument();
  expect(screen.queryByText(/graph nodes/)).not.toBeInTheDocument();
});

it('shows scoped not-found without offering a retry', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ error: 'not found' }), { status: 404 })));
  render(<DeploymentDetailsPage application={application} environment="staging" deploymentId="missing" />);

  expect(await screen.findByRole('heading', { name: 'Deployment not found' })).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
});

it('clears stale detail while loading a new deployment and retries API errors', async () => {
  let rejectSecond: ((reason?: unknown) => void) | undefined;
  const second = new Promise<Response>((_resolve, reject) => { rejectSecond = reject; });
  const fetcher = vi.fn()
    .mockResolvedValueOnce(Response.json(detail()))
    .mockReturnValueOnce(second)
    .mockResolvedValueOnce(Response.json(detail({ deployment: deployment({ id: 'dep-2', workloadId: 'backend' }) })));
  vi.stubGlobal('fetch', fetcher);
  const view = render(<DeploymentDetailsPage application={application} environment="staging" deploymentId="dep-1" />);
  expect(await screen.findByText('frontend · deploy')).toBeInTheDocument();

  view.rerender(<DeploymentDetailsPage application={application} environment="staging" deploymentId="dep-2" />);
  expect(screen.getByRole('status')).toHaveTextContent('Loading deployment…');
  expect(screen.queryByText('frontend · deploy')).not.toBeInTheDocument();
  rejectSecond?.(new Error('offline'));
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not load deployment details.');
  expect(screen.queryByText('frontend · deploy')).not.toBeInTheDocument();

  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText('backend · deploy')).toBeInTheDocument();
  await waitFor(() => expect(fetcher).toHaveBeenCalledTimes(3));
});

it('localizes recent deployment actions and statuses and hides raw load errors', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ deployments: [deployment({ id: 'dep-1', workloadId: 'api', status: 'SUCCEEDED' })] })));
  const view = render(<RecentDeployments applicationId="catalog" environment="staging" />);
  expect(await screen.findByText('Thành công')).toBeInTheDocument();
  view.unmount();
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ error: 'token=abc' }, { status: 500 })));
  render(<RecentDeployments applicationId="catalog" environment="staging" />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải các triển khai gần đây.');
  expect(screen.getByRole('button', { name: 'Thử lại' })).toBeInTheDocument();
  expect(document.body).not.toHaveTextContent('abc');
});

it('retries a failed recent-deployments load with a safe message and then shows the list', async () => {
  const fetcher = vi.fn()
    .mockResolvedValueOnce(Response.json({ error: 'password=hunter2' }, { status: 500 }))
    .mockResolvedValueOnce(Response.json({ deployments: [deployment({ id: 'dep-1', workloadId: 'api' })] }));
  vi.stubGlobal('fetch', fetcher);
  render(<RecentDeployments applicationId="catalog" environment="staging" />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải các triển khai gần đây.');
  expect(document.body).not.toHaveTextContent('hunter2');
  await userEvent.setup().click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText('api')).toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it('shows at most five recent deployments in server order and links each to its detail', async () => {
  const items = Array.from({ length: 7 }, (_, index) => deployment({ id: `dep-${index + 1}`, workloadId: `w${index + 1}` }));
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ deployments: items })));
  render(<RecentDeployments applicationId="catalog" environment="staging" />);
  await screen.findByText('w1');
  const entries = screen.getAllByRole('button').filter((button) => button.className === 'deployment-entry');
  expect(entries.map((entry) => entry.querySelector('strong')?.textContent)).toEqual(['w1', 'w2', 'w3', 'w4', 'w5']);
  expect(screen.queryByText('w6')).not.toBeInTheDocument();
  await userEvent.setup().click(entries[1] as HTMLElement);
  expect(window.location.pathname).toBe('/ui/applications/catalog/environments/staging/deployments/dep-2');
});

it('ignores an obsolete recent-deployments response after the Environment changes', async () => {
  let resolveStaging: (value: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn((input: string | URL | Request) => String(input).includes('/staging/')
    ? new Promise<Response>((done) => { resolveStaging = done; })
    : Promise.resolve(Response.json({ deployments: [deployment({ id: 'dep-p', workloadId: 'prod-api', environmentKey: 'production' })] }))));
  const view = render(<RecentDeployments applicationId="catalog" environment="staging" />);
  view.rerender(<RecentDeployments applicationId="catalog" environment="production" />);
  expect(await screen.findByText('prod-api')).toBeInTheDocument();
  await act(async () => { resolveStaging(Response.json({ deployments: [deployment({ id: 'dep-s', workloadId: 'staging-api' })] })); });
  expect(screen.queryByText('staging-api')).not.toBeInTheDocument();
  expect(screen.getByText('prod-api')).toBeInTheDocument();
});
