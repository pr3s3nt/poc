import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import type { Application } from '../../shared/types/application';
import { DeploymentDetailsPage } from './DeploymentDetailsPage';
import { DeploymentHistoryPage } from './DeploymentHistoryPage';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('shows deployments for the selected environment and filters failures', async () => {
  const fetcher = vi.fn(async (_input: string | URL | Request) => Response.json({ deployments: [
    { id: 'dep-1', applicationKey: 'catalog', environmentKey: 'staging', workloadId: 'frontend', action: 'DEPLOY', status: 'SUCCEEDED', startedAt: '2026-09-01T10:00:00Z' },
    { id: 'dep-2', applicationKey: 'catalog', environmentKey: 'staging', workloadId: 'backend', action: 'UPDATE', status: 'FAILED', startedAt: '2026-09-02T10:00:00Z' },
  ] }));
  vi.stubGlobal('fetch', fetcher);
  render(<DeploymentHistoryPage application={application} environment="staging" />);
  expect(await screen.findByText('backend')).toBeInTheDocument();
  expect(fetcher.mock.calls[0]?.[0]).toContain('application=catalog&environment=staging');
  await userEvent.setup().selectOptions(screen.getByLabelText('Filter deployment status'), 'FAILED');
  expect(screen.queryByText('frontend')).not.toBeInTheDocument();
  expect(screen.getByText('backend')).toBeInTheDocument();
});

it('shows a failed deployment and hides raw secret-bearing plan fields', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({
    deployment: { id: 'dep-2', applicationKey: 'catalog', environmentKey: 'staging', workloadId: 'backend', action: 'UPDATE', status: 'FAILED', failureReason: 'timed out', startedAt: '2026-09-02T10:00:00Z' },
    deploymentSetId: 'set-1', planHash: '0123456789abcdef', graph: { nodes: [{ descriptor: 'db' }], edges: [] }, batches: [['db']],
    resources: [{ descriptor: 'db', resourceType: 'postgres', definitionKey: 'pg-kind', status: 'READY', outputs: { host: 'db.internal', password: '***redacted***' }, resolvedInputs: { password: 'raw-secret' } }],
    workloads: [{ workloadId: 'backend', status: 'FAILED', lastDeploymentId: 'dep-2' }],
  })));
  render(<DeploymentDetailsPage application={application} environment="staging" deploymentId="dep-2" />);
  expect(await screen.findByText('timed out')).toBeInTheDocument();
  expect(screen.getByText(/pg-kind/)).toBeInTheDocument();
  expect(screen.queryByText('raw-secret')).not.toBeInTheDocument();
});
