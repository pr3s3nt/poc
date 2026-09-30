import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import type { Application } from '../../shared/types/application';
import { DeploymentDetailsPage } from './DeploymentDetailsPage';
import { DeploymentHistoryPage } from './DeploymentHistoryPage';
import { RecentDeployments } from './RecentDeployments';

const application: Application = { id: 'catalog/app', name: 'Catalog', subdomain: 'catalog', workloads: { staging: [], production: [] } };
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

it('loads scoped history and asks the server to filter by status', async () => {
  const fetcher = vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('?status=FAILED')) return Response.json({ deployments: [deployment({ id: 'dep-2', workloadId: 'backend', action: 'UPDATE', status: 'FAILED' })] });
    return Response.json({ deployments: [deployment()] });
  });
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="staging" />);

  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  expect(await screen.findByText('frontend')).toBeInTheDocument();
  expect(fetcher.mock.calls[0]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments');

  await userEvent.setup().selectOptions(screen.getByLabelText('Filter deployment status'), 'FAILED');
  expect(await screen.findByText('backend')).toBeInTheDocument();
  expect(screen.queryByText('frontend')).not.toBeInTheDocument();
  expect(fetcher.mock.calls[1]?.[0]).toBe('/api/v1/applications/catalog%2Fapp/environments/staging/deployments?status=FAILED');
});

it('distinguishes empty history from a retryable loading error', async () => {
  const fetcher = vi.fn()
    .mockResolvedValueOnce(new Response(JSON.stringify({ error: 'temporary' }), { status: 503 }))
    .mockResolvedValueOnce(Response.json({ deployments: [] }));
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="production" />);

  expect(await screen.findByRole('alert')).toHaveTextContent('Could not load deployment history.');
  await userEvent.setup().click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText('No deployments in this Environment yet.')).toBeInTheDocument();
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it('uses the same scoped endpoint and explicit states for recent deployments', async () => {
  const fetcher = vi.fn(async (_input: string | URL | Request) => Response.json({ deployments: [] }));
  vi.stubGlobal('fetch', fetcher);
  render(<RecentDeployments applicationId="catalog/app" environment="staging" />);
  expect(screen.getByRole('status')).toHaveAttribute('aria-busy', 'true');
  expect(await screen.findByText('No deployments yet in staging.')).toBeInTheDocument();
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
