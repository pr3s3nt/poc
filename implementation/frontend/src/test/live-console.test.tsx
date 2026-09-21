// Runtime smoke test: render the real console components against a running Go
// orchestrator instead of a mock. jsdom has no same-origin server, so fetch is
// rewritten onto the live base URL; the request path and payload stay the ones
// the console itself produces.
//
// ORCHESTRATOR_LIVE_URL            enables the suite.
// ORCHESTRATOR_LIVE_DEPLOYMENT_ID  switches to read-only verification of an
//                                  existing deployment (kind and AWS runs).
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DeployPage } from '../features/deploy/pages/DeployPage';
import { DeploymentDetailsPage } from '../features/deployment-details/pages/DeploymentDetailsPage';

const liveURL = process.env.ORCHESTRATOR_LIVE_URL;
const existingDeploymentId = process.env.ORCHESTRATOR_LIVE_DEPLOYMENT_ID;
const suite = liveURL ? describe : describe.skip;

suite('web console against the live Go API', () => {
  beforeEach(() => {
    const realFetch = globalThis.fetch.bind(globalThis);
    vi.stubGlobal('fetch', (input: RequestInfo | URL, init?: RequestInit) =>
      realFetch(new URL(String(input), liveURL), init),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    window.history.pushState({}, '', '/ui/');
  });

  it.runIf(!existingDeploymentId)(
    'loads the catalog, deploys a workload and shows the deployment details',
    async () => {
      render(<DeployPage />);
      await screen.findByRole('heading', { name: 'Deploy workload' });

      const textarea = await screen.findByLabelText('Score document (JSON)');
      expect((textarea as HTMLTextAreaElement).value).toContain('apiVersion');

      await userEvent.click(screen.getByRole('button', { name: 'Deploy' }));
      await waitFor(() => expect(window.location.pathname).toMatch(/^\/ui\/deployments\//), { timeout: 30_000 });

      const deploymentId = window.location.pathname.split('/').pop() ?? '';
      expect(deploymentId).not.toBe('');

      render(<DeploymentDetailsPage deploymentId={deploymentId} />);
      expect(await screen.findAllByText('SUCCEEDED')).not.toHaveLength(0);
      const resources = await screen.findByTestId('resources-table');
      expect(resources).toHaveTextContent('postgres');
      expect(resources).toHaveTextContent('***redacted***');
    },
    60_000,
  );

  it.runIf(Boolean(existingDeploymentId))(
    'shows the deployment, its resources and its workloads for a real cluster run',
    async () => {
      render(<DeploymentDetailsPage deploymentId={existingDeploymentId ?? ''} />);

      expect(await screen.findAllByText('SUCCEEDED')).not.toHaveLength(0);

      const resources = await screen.findByTestId('resources-table');
      expect(resources).toHaveTextContent('postgres');
      expect(resources).toHaveTextContent('k8s-namespace');
      expect(resources).toHaveTextContent('***redacted***');

      const workloads = await screen.findByTestId('workloads-table');
      for (const workload of ['frontend', 'backend', 'worker']) {
        expect(workloads).toHaveTextContent(workload);
      }
      expect(screen.getByText('Batch 0')).toBeInTheDocument();
    },
    60_000,
  );
});
