import { render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeploymentDetailsPage } from './DeploymentDetailsPage';
import { deploymentViewFixture } from '../../../test/fixtures';

function stubFetch(body: unknown, status = 200) {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status })));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('DeploymentDetailsPage', () => {
  it('shows a loading state first', async () => {
    stubFetch(deploymentViewFixture);
    render(<DeploymentDetailsPage deploymentId="dep-1" />);
    expect(screen.getByTestId('details-loading')).toBeInTheDocument();
    await screen.findByText('SUCCEEDED');
  });

  it('renders status, batches, resources and workloads', async () => {
    stubFetch(deploymentViewFixture);
    render(<DeploymentDetailsPage deploymentId="dep-1" />);

    expect(await screen.findByText('SUCCEEDED')).toBeInTheDocument();
    expect(screen.getByText('Batch 0')).toBeInTheDocument();
    expect(screen.getByText('postgres-internal-statefulset')).toBeInTheDocument();
    const workloads = screen.getByTestId('workloads-table');
    expect(workloads).toHaveTextContent('backend');
    expect(workloads).toHaveTextContent('acceptance-dev');
  });

  it('shows redacted secret outputs and never a password value', async () => {
    stubFetch(deploymentViewFixture);
    render(<DeploymentDetailsPage deploymentId="dep-1" />);

    const resources = await screen.findByTestId('resources-table');
    expect(resources).toHaveTextContent('***redacted***');
    expect(resources).not.toHaveTextContent('s3cret');
  });

  it('shows an error state when the API fails', async () => {
    stubFetch({ error: 'persistence: not found: deployment "dep-x"' }, 404);
    render(<DeploymentDetailsPage deploymentId="dep-x" />);
    expect(await screen.findByText(/not found/)).toBeInTheDocument();
  });
});
