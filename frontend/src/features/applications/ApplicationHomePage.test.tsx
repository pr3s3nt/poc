import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ApplicationHomePage } from './ApplicationHomePage';
import type { Application } from '../../shared/types/application';
import { bothConfigured, configuredTarget } from '../../test/targets';
import { unconfiguredTarget } from '../../shared/types/application';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };

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
  expect(within(screen.getByLabelText('Deployment preview')).getByLabelText('Execution target')).toHaveTextContent('Staging · Connection internal-cluster (internal-cluster) · profile internal-k8s');
  await user.click(screen.getByRole('button', { name: 'Deploy these changes' }));
  expect(await screen.findByRole('heading', { name: 'Deploy succeeded' })).toBeInTheDocument();
  expect(within(screen.getByLabelText('Deployment result')).getByLabelText('Execution target')).toHaveTextContent('internal-cluster');
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

const list = (env: string, workloads: unknown[], draftVersion = 1) => Response.json({ applicationKey: 'catalog', environmentKey: env, draftVersion, workloads });

it('confirms deletion by name and Environment, disables duplicate actions while busy, and undoes', async () => {
  let release: () => void = () => undefined;
  let deletes = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/workloads/api') && init?.method === 'DELETE') {
      deletes += 1;
      await new Promise<void>((done) => { release = done; });
      return list('staging', [{ id: 'api', state: 'PENDING_DELETE', ready: true }], 2);
    }
    if (url.endsWith('/workloads/api/undo')) return list('staging', [{ id: 'api', state: '', ready: true, score: {} }], 3);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  expect(confirm).toHaveBeenLastCalledWith('Mark api for deletion in staging? Running workloads will not change until Deploy.');
  expect(deletes).toBe(0);
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  expect(await screen.findByRole('button', { name: 'Marking…' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Preview changes' })).toBeDisabled();
  release();
  await user.click(await screen.findByRole('button', { name: 'Undo' }));
  expect(await screen.findByText('Ready')).toBeInTheDocument();
  expect(deletes).toBe(1);
  confirm.mockRestore();
});

it('reloads the list on a version conflict and never retries the mutation', async () => {
  let deletes = 0;
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'DELETE') { deletes += 1; return Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }); }
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    lists += 1;
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }, ...(lists > 1 ? [{ id: 'web', state: 'PENDING_UPSERT', ready: false, score: {} }] : [])], lists);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  expect(await screen.findByText(/changed since this page loaded/)).toBeInTheDocument();
  expect(await screen.findByText('web')).toBeInTheDocument();
  expect(deletes).toBe(1);
  vi.restoreAllMocks();
});

it('keeps the deploy outcome when the list reload fails and blocks mutations until a retry succeeds', async () => {
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    if (url.endsWith('/deploy')) return Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'UPDATE', status: 'SUCCEEDED', deploymentId: 'dep-1' }] });
    lists += 1;
    if (lists === 2) return Response.json({ error: 'unavailable' }, { status: 500 });
    return list('staging', [{ id: 'api', state: lists === 1 ? 'PENDING_UPSERT' : '', ready: true, score: {} }], lists);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  await user.click(await screen.findByRole('button', { name: 'Deploy these changes' }));
  expect(await screen.findByRole('heading', { name: 'Deploy succeeded' })).toBeInTheDocument();
  expect(await screen.findByText(/could not be reloaded/)).toBeInTheDocument();
  expect(screen.getByRole('heading', { name: 'Deploy succeeded' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Preview changes' })).toBeDisabled();
  expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText('Ready')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Preview changes' })).toBeEnabled();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});

it('keeps controls busy until the post-deploy reload finishes', async () => {
  let lists = 0;
  let releaseReload: (value: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.endsWith('/preview')) return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'UPDATE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'p' }] });
    if (url.endsWith('/deploy')) return Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'UPDATE', status: 'SUCCEEDED' }] });
    lists += 1;
    if (lists === 2) return new Promise<Response>((done) => { releaseReload = done; });
    return list('staging', [{ id: 'api', state: 'PENDING_UPSERT', ready: true, score: {} }], 1);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  await user.click(await screen.findByRole('button', { name: 'Deploy these changes' }));
  expect(await screen.findByRole('heading', { name: 'Deploy succeeded' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Preview changes' })).toBeDisabled();
  releaseReload(list('staging', [{ id: 'api', state: '', ready: true, score: {} }], 2));
  expect(await screen.findByText('Ready')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
});

it('does not show a staging conflict notice after switching to production', async () => {
  let releaseReload: (value: Response) => void = () => undefined;
  let stagingLists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'DELETE') return Response.json({ error: 'conflict' }, { status: 409 });
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.includes('/production/')) return list('production', [{ id: 'prod-only', state: '', ready: true, score: {} }]);
    stagingLists += 1;
    if (stagingLists === 2) return new Promise<Response>((done) => { releaseReload = done; });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }]);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  await user.click(screen.getByRole('button', { name: /^Production/ }));
  await screen.findByText('prod-only');
  releaseReload(list('staging', [{ id: 'api', state: '', ready: true, score: {} }], 2));
  await new Promise((done) => setTimeout(done, 20));
  expect(screen.queryByText(/changed since this page loaded/)).not.toBeInTheDocument();
  expect(screen.getByText('prod-only')).toBeInTheDocument();
  vi.restoreAllMocks();
});

it('reports truthfully when the reload after a conflict fails', async () => {
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'DELETE') return Response.json({ error: 'conflict' }, { status: 409 });
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    lists += 1;
    if (lists === 2) return Response.json({ error: 'unavailable' }, { status: 500 });
    return list('staging', [{ id: 'api', state: '', ready: true, score: {} }], lists);
  }));
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Delete' }));
  expect(await screen.findByText(/could not be reloaded/)).toBeInTheDocument();
  expect(screen.queryByText(/The list was reloaded/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Preview changes' })).toBeDisabled();
  vi.restoreAllMocks();
});

it('ignores a late Preview after switching Environment and blocks switching during Deploy', async () => {
  let resolvePreview: (value: Response) => void = () => undefined;
  let resolveDeploy: (value: Response) => void = () => undefined;
  let previews = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    if (url.endsWith('/preview')) {
      previews += 1;
      if (previews === 1) return new Promise<Response>((done) => { resolvePreview = done; });
      return Response.json({ token: 't', draftVersion: 1, changes: [{ workloadId: 'api', action: 'REMOVE', resources: { existing: [], new: [], unreferenced: ['postgres.default#shared.db'] }, planHash: 'p' }] });
    }
    if (url.endsWith('/deploy')) return new Promise<Response>((done) => { resolveDeploy = done; });
    return list(url.includes('/production/') ? 'production' : 'staging', [{ id: url.includes('/production/') ? 'prod-only' : 'api', state: 'PENDING_DELETE', ready: true }]);
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={application} />);
  await screen.findByText('api');
  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  await user.click(screen.getByRole('button', { name: /^Production/ }));
  await screen.findByText('prod-only');
  resolvePreview(Response.json({ token: 'staging-token', draftVersion: 1, changes: [{ workloadId: 'api', action: 'REMOVE', resources: { existing: [], new: [], unreferenced: [] }, planHash: 'x' }] }));
  await new Promise((done) => setTimeout(done, 20));
  expect(screen.queryByLabelText('Deployment preview')).not.toBeInTheDocument();

  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  expect(await screen.findByText(/1 resource\(s\) will become unreferenced \(not destroyed\)/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Deploy these changes' }));
  expect(screen.getByRole('button', { name: /^Staging/ })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Deploying…' })).toBeDisabled();
  resolveDeploy(Response.json({ status: 'SUCCEEDED', results: [{ workloadId: 'api', action: 'REMOVE', status: 'SUCCEEDED', deploymentId: 'dep-9' }] }));
  expect(await screen.findByRole('link', { name: 'view deployment' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: /^Staging/ })).toBeEnabled();
});

it('shows the target of the selected Environment only and switches with the tab', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ draftVersion: 0, workloads: [], deployments: [] })));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={{ ...application, environments: { staging: configuredTarget('lab', { connectionName: 'Lab' }), production: configuredTarget('cloud', { connectionName: 'Cloud', connectionKind: 'AWS', profile: 'aws-eks', region: 'us-east-1', runtimeStatus: 'PENDING' }) } }} />);
  expect(screen.getAllByLabelText('Execution target')[0]).toHaveTextContent('Staging · Connection Lab (lab) · profile internal-k8s');
  await screen.findByText('No workloads in this environment yet.');
  await user.click(screen.getByRole('button', { name: /^Production/ }));
  expect(screen.getAllByLabelText('Execution target')[0]).toHaveTextContent('Production · Connection Cloud (cloud) · profile aws-eks · region us-east-1');
  expect(screen.getAllByLabelText('Execution target')[0]).not.toHaveTextContent('Lab');
});

it('guides an UNCONFIGURED Environment to its settings and surfaces the safe 422 from Preview', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/preview') && init?.method === 'POST') return Response.json({ error: 'the Environment has no execution connection; set one in Environment Settings', field: 'connectionKey', code: 'ENVIRONMENT_UNCONFIGURED' }, { status: 422 });
    return Response.json({ draftVersion: 0, workloads: [], deployments: [] });
  }));
  const user = userEvent.setup();
  render(<ApplicationHomePage application={{ ...application, environments: { staging: unconfiguredTarget, production: configuredTarget('lab') } }} />);
  expect(screen.getAllByLabelText('Execution target')[0]).toHaveTextContent('Staging has no execution connection yet');
  await screen.findByText('No workloads in this environment yet.');
  await user.click(screen.getByRole('button', { name: 'Preview changes' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('set one in Environment Settings');
  await user.click(within(alert).getByRole('button', { name: 'Open Environment settings' }));
  expect(window.location.pathname).toBe('/ui/applications/catalog/settings');
  expect(window.location.search).toBe('?environment=staging');
});

it('disables draft Delete and Undo while an environment operation runs and enables them after release', async () => {
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method && init.method !== 'GET') calls.push(`${init.method} ${url}`);
    if (url.endsWith('/workloads')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 1, workloads: [{ id: 'frontend', ready: true, score: {} }, { id: 'old', state: 'PENDING_DELETE', ready: true }] });
    return Response.json({}, { status: 404 });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const view = render(<ApplicationHomePage application={application} />);
  await screen.findByText('frontend');
  expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
  view.rerender(<ApplicationHomePage application={busyApp} />);
  expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Undo' })).toBeDisabled();
  expect(screen.getByText(/Draft changes are paused/)).toBeInTheDocument();
  view.rerender(<ApplicationHomePage application={application} />);
  expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Undo' })).toBeEnabled();
  expect(calls).toEqual([]);
});
