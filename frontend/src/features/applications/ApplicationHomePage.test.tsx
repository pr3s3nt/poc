import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ApplicationHomePage } from './ApplicationHomePage';
import type { Application } from '../../shared/types/application';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('previews pending changes and deploys only after confirmation', async () => {
  let deployToken = '';
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 1, workloads: [{ id: 'frontend', state: 'PENDING_UPSERT', ready: false, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({ token: 'pinned-preview', draftVersion: 1, configRevisionId: 'revision-1', changes: [{ workloadId: 'frontend', action: 'DEPLOY', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'plan-1' }] });
    if (url.endsWith('/deploy')) {
      deployToken = (JSON.parse(String(init?.body)) as { token: string }).token;
      return Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'frontend', action: 'DEPLOY', status: 'SUCCEEDED' }] });
    }
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('frontend');
  expect(screen.queryByRole('button', { name: 'Deploy these changes' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  expect(await screen.findByRole('heading', { name: 'Preview for staging' })).toBeInTheDocument();
  expect(deployToken).toBe('');
  await user.click(screen.getByRole('button', { name: 'Deploy these changes' }));
  expect(await screen.findByRole('heading', { name: 'Deploy succeeded' })).toBeInTheDocument();
  expect(deployToken).toBe('pinned-preview');
});

it('offers route-only retry when workloads have already been applied', async () => {
  let deployCalls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/workloads')) return Response.json({ draftVersion: 2, workloads: [{ id: 'frontend', state: '', ready: true, score: {} }] });
    if (url.endsWith('/preview')) return Response.json({ token: 'route-retry', draftVersion: 2, routePending: true, changes: [] });
    if (url.endsWith('/deploy')) { deployCalls += 1; return Response.json({ status: 'SUCCEEDED', results: [] }); }
    return Response.json({}, { status: 404 });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('frontend');
  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  expect(await screen.findByText('Public routes need reconciliation; workloads will not restart.')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Retry public routes' }));
  expect(await screen.findByRole('heading', { name: 'Deploy succeeded' })).toBeInTheDocument();
  expect(deployCalls).toBe(1);
});
