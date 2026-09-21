import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { DeployPage } from './DeployPage';
import { applicationsFixture, scoreSamplesFixture } from '../../../test/fixtures';

function stubApi(handlers: Record<string, () => Response>) {
  const spy = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    for (const [path, handler] of Object.entries(handlers)) {
      if (url.startsWith(path)) {
        return handler();
      }
    }
    throw new Error(`unexpected request ${url}`);
  });
  vi.stubGlobal('fetch', spy);
  return spy;
}

function ok(body: unknown, status = 200) {
  return () => new Response(JSON.stringify(body), { status });
}

afterEach(() => {
  vi.unstubAllGlobals();
  window.history.pushState({}, '', '/ui/');
});

describe('DeployPage', () => {
  it('shows a loading state before the catalog arrives', async () => {
    stubApi({
      '/api/v1/applications': ok(applicationsFixture),
      '/api/v1/score-samples': ok(scoreSamplesFixture),
    });
    render(<DeployPage />);
    expect(screen.getByTestId('deploy-loading')).toBeInTheDocument();
    await screen.findByRole('heading', { name: 'Deploy workload' });
  });

  it('prefills the form and navigates to the deployment after a successful submit', async () => {
    stubApi({
      '/api/v1/applications': ok(applicationsFixture),
      '/api/v1/score-samples': ok(scoreSamplesFixture),
      '/api/v1/deployments': ok({ deploymentId: 'dep-42', status: 'SUCCEEDED', planHash: 'h', workloadId: 'backend' }, 201),
    });
    render(<DeployPage />);

    await screen.findByRole('heading', { name: 'Deploy workload' });
    const textarea = await screen.findByLabelText('Score document (JSON)');
    expect((textarea as HTMLTextAreaElement).value).toContain('acceptance-backend:dev');

    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }));

    await waitFor(() => expect(window.location.pathname).toBe('/ui/deployments/dep-42'));
  });

  it('shows the API error returned by the backend', async () => {
    stubApi({
      '/api/v1/applications': ok(applicationsFixture),
      '/api/v1/score-samples': ok(scoreSamplesFixture),
      '/api/v1/deployments': ok({ error: 'planning: no resource definition matches postgres' }, 400),
    });
    render(<DeployPage />);
    await screen.findByRole('heading', { name: 'Deploy workload' });

    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }));

    expect(await screen.findByText(/no resource definition matches postgres/)).toBeInTheDocument();
  });

  it('blocks submission and lists validation errors for an invalid Score', async () => {
    stubApi({
      '/api/v1/applications': ok(applicationsFixture),
      '/api/v1/score-samples': ok(scoreSamplesFixture),
    });
    render(<DeployPage />);
    const textarea = await screen.findByLabelText('Score document (JSON)');

    await userEvent.clear(textarea);
    await userEvent.type(textarea, '{{not json');
    await userEvent.click(screen.getByRole('button', { name: 'Deploy' }));

    expect(await screen.findByText(/not valid JSON/)).toBeInTheDocument();
  });

  it('reports an empty catalog', async () => {
    stubApi({
      '/api/v1/applications': ok({ applications: [] }),
      '/api/v1/score-samples': ok(scoreSamplesFixture),
    });
    render(<DeployPage />);
    expect(await screen.findByText('No application is registered yet')).toBeInTheDocument();
  });
});
