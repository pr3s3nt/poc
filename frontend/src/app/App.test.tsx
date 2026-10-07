import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response> | undefined;

let authenticated: boolean;
type StubEnvironment = { key: string; version: number; configured: boolean; connectionKey: string; connectionName?: string; connectionKind?: string; executionProfile: string; region?: string; runtimeStatus: string; infrastructureScope: string };
type StubApplication = { key: string; name: string; subdomain: string; environments: StubEnvironment[] };
const unset = (key: string): StubEnvironment => ({ key, version: 1, configured: false, connectionKey: '', executionProfile: '', runtimeStatus: 'UNCONFIGURED', infrastructureScope: 'ENVIRONMENT' });
const bound = (key: string, connectionKey: string): StubEnvironment => ({ key, version: 2, configured: true, connectionKey, connectionName: connectionKey, connectionKind: 'KUBERNETES', executionProfile: 'internal-k8s', runtimeStatus: 'READY', infrastructureScope: 'ENVIRONMENT' });
let applications: StubApplication[];
let requests: { url: string; method: string; body?: string }[];
let override: Handler | undefined;

function stubBackend() {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    requests.push({ url, method: init?.method ?? 'GET', body: init?.body ? String(init.body) : undefined });
    const custom = await override?.(url, init);
    if (custom) return custom;
    if (url.endsWith('/auth/session')) return authenticated ? Response.json({ user: { Username: 'developer', Role: 'DEVELOPER' } }) : Response.json({ error: 'unauthorized' }, { status: 401 });
    if (url.endsWith('/auth/sign-in')) { authenticated = true; return Response.json({ user: { Username: 'developer', Role: 'DEVELOPER' } }); }
    if (url.endsWith('/auth/sign-out')) { authenticated = false; return new Response(null, { status: 204 }); }
    if (!authenticated) return Response.json({ error: 'unauthorized' }, { status: 401 });
    if (url.endsWith('/applications') && init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as { name: string; subdomain: string };
      const created: StubApplication = { key: 'generated-id', name: body.name, subdomain: body.subdomain, environments: [unset('staging'), unset('production')] };
      applications.push(created);
      return Response.json({ application: created }, { status: 201 });
    }
    const set = url.match(/\/applications\/([^/]+)\/environments\/([^/]+)\/connection$/);
    if (set && init?.method === 'PUT') {
      const body = JSON.parse(String(init.body)) as { connectionKey: string; expectedVersion: number };
      const env = applications.find((item) => item.key === set[1])?.environments.find((item) => item.key === set[2]);
      if (!env) return Response.json({ error: 'not found' }, { status: 404 });
      if (env.configured) return Response.json({ error: 'this environment already has a connection; it cannot be changed', code: 'ALREADY_CONFIGURED' }, { status: 409 });
      Object.assign(env, bound(env.key, body.connectionKey));
      return Response.json({ environment: env });
    }
    if (url.endsWith('/application-connections')) return Response.json({ connections: [{ key: 'internal-cluster', name: 'Internal cluster', kind: 'KUBERNETES', status: 'READY' }, { key: 'lab', name: 'Lab', kind: 'KUBERNETES', status: 'READY' }], defaultConnectionKey: 'internal-cluster' });
    if (url.endsWith('/applications')) return Response.json({ applications });
    if (url.includes('/configuration')) return Response.json({ applicationKey: 'x', environmentKey: 'staging', version: 0, keys: [] });
    if (url.includes('/workloads')) return Response.json({ draftVersion: 0, workloads: [] });
    if (url.includes('/deployments')) return Response.json({ deployments: [] });
    return Response.json({});
  }));
}

async function signIn(user: ReturnType<typeof userEvent.setup>) {
  await user.type(await screen.findByLabelText('Username'), 'developer');
  await user.type(screen.getByLabelText('Password'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
  await screen.findByRole('heading', { name: 'Your applications' });
}

describe('developer onboarding shell (UC-00/UC-01)', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/ui/sign-in');
    authenticated = false;
    applications = [{ key: 'payment', name: 'Payment', subdomain: 'payment', environments: [bound('staging', 'internal-cluster'), bound('production', 'internal-cluster')] }];
    requests = [];
    override = undefined;
    stubBackend();
  });
  afterEach(() => vi.unstubAllGlobals());

  it('signs in to the applications home', async () => {
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    expect(await screen.findByText('Payment')).toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/applications');
  });

  it('restores an existing session without showing sign-in', async () => {
    authenticated = true;
    render(<App />);
    expect(await screen.findByRole('heading', { name: 'Your applications' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/applications');
  });

  it('redirects protected routes to sign-in without a session', async () => {
    window.history.replaceState({}, '', '/ui/applications/payment');
    render(<App />);
    expect(await screen.findByLabelText('Username')).toBeInTheDocument();
    expect(requests.some((request) => request.url.endsWith('/applications'))).toBe(false);
  });

  it('shows a loading state, then the empty state', async () => {
    authenticated = true;
    applications = [];
    let release: () => void = () => undefined;
    override = (url) => url.endsWith('/applications') ? new Promise<Response>((resolve) => { release = () => resolve(Response.json({ applications: [] })); }) as unknown as Response : undefined;
    render(<App />);
    expect(await screen.findByRole('status', { name: 'Loading applications' })).toBeInTheDocument();
    release();
    expect(await screen.findByRole('heading', { name: 'Start a new application' })).toBeInTheDocument();
  });

  it('shows a retryable list error without substituting data', async () => {
    authenticated = true;
    let failures = 1;
    override = (url) => url.endsWith('/applications') && failures-- > 0 ? Response.json({ error: 'database unavailable' }, { status: 500 }) : undefined;
    const user = userEvent.setup();
    render(<App />);
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load applications.');
    expect(screen.queryByText('Payment')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText('Payment')).toBeInTheDocument();
  });

  it('creates an application without a connection and sets each environment once in Settings', async () => {
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    await user.click(screen.getAllByRole('button', { name: /create application/i })[0]!);
    await user.type(screen.getByLabelText('Application name'), 'Catalog');
    await user.type(screen.getByLabelText('Subdomain'), 'Catalog');
    expect(screen.queryByLabelText('Connection')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Create application' }));

    expect(await screen.findByRole('heading', { name: 'Catalog' })).toBeInTheDocument();
    expect(screen.getByLabelText('Execution target')).toHaveTextContent('Staging has no execution connection yet');
    expect(screen.getByRole('status')).toHaveTextContent('Application created.');
    expect(screen.getByRole('button', { name: /Staging/ })).toHaveClass('tab-active');
    const create = requests.find((request) => request.method === 'POST' && request.url.endsWith('/applications'));
    expect(JSON.parse(create?.body ?? '{}')).toEqual({ name: 'Catalog', subdomain: 'catalog' });

    // Staging picks lab in Settings; production stays unset.
    await user.click(screen.getByRole('button', { name: 'Environment settings' }));
    await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
    await user.click(screen.getByRole('button', { name: 'Set connection' }));
    expect(await screen.findByText('Locked')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Set connection' })).not.toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    const put = requests.find((request) => request.method === 'PUT' && request.url.endsWith('/environments/staging/connection'));
    expect(JSON.parse(put?.body ?? '{}')).toEqual({ connectionKey: 'lab', expectedVersion: 1 });
    await user.click(screen.getByRole('tab', { name: 'Production' }));
    expect(await screen.findByLabelText('Connection for Production')).toHaveValue('');
    expect(screen.getByText('Not configured')).toBeInTheDocument();
    expect(requests.some((request) => /\/(deploy|preview|deployments)$/.test(request.url) && request.method === 'POST')).toBe(false);
    // Back home the cached target shows the staging binding only.
    await user.click(screen.getByRole('button', { name: /← Catalog/ }));
    expect(await screen.findByLabelText('Execution target')).toHaveTextContent('Staging · Connection lab (lab)');
    await user.click(screen.getByRole('button', { name: /^Production/ }));
    expect(screen.getByLabelText('Execution target')).toHaveTextContent('Production has no execution connection yet');
  });

  it('keeps the Settings Environment aligned with the URL across tab clicks and back/forward', async () => {
    const user = userEvent.setup();
    window.history.replaceState({}, '', '/ui/applications/payment/settings?environment=production');
    authenticated = true;
    render(<App />);
    expect(await screen.findByRole('tab', { name: 'Production' })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByLabelText('Execution connection')).toHaveTextContent('Production deploys to this connection');
    await waitFor(() => expect(requests.some((request) => request.url.includes('/environments/production/configuration'))).toBe(true));
    // A tab click keeps the URL aligned, so a reload opens the same scope.
    await user.click(screen.getByRole('tab', { name: 'Staging' }));
    await waitFor(() => expect(window.location.search).toBe('?environment=staging'));
    expect(screen.getByRole('tab', { name: 'Staging' })).toHaveAttribute('aria-selected', 'true');
    // Back/forward to another scope re-scopes both the connection panel and the configuration.
    window.history.pushState({}, '', '/ui/applications/payment/settings?environment=production');
    await act(async () => { window.dispatchEvent(new PopStateEvent('popstate')); });
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Production' })).toHaveAttribute('aria-selected', 'true'));
    expect(screen.getByLabelText('Execution connection')).toHaveTextContent('Production deploys to this connection');
    window.history.pushState({}, '', '/ui/applications/payment/settings');
    await act(async () => { window.dispatchEvent(new PopStateEvent('popstate')); });
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Staging' })).toHaveAttribute('aria-selected', 'true'));
    expect(screen.getByLabelText('Execution connection')).toHaveTextContent('Staging deploys to this connection');
  });

  it('returns to sign-in with a notice when the session expires', async () => {
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    authenticated = false;
    await user.click(screen.getByText('Payment'));
    expect(await screen.findByText('Your session ended. Sign in again to continue.')).toBeInTheDocument();
    expect(window.location.pathname).toBe('/ui/sign-in');
    expect(screen.queryByText('Payment')).not.toBeInTheDocument();
  });

  it('signs out and clears local context even if the request fails', async () => {
    override = (url) => url.endsWith('/auth/sign-out') ? Response.json({ error: 'offline' }, { status: 503 }) : undefined;
    const user = userEvent.setup();
    render(<App />);
    await signIn(user);
    await user.click(screen.getByRole('button', { name: 'Sign out' }));
    expect(await screen.findByLabelText('Username')).toBeInTheDocument();
    expect(screen.queryByText('Your session ended. Sign in again to continue.')).not.toBeInTheDocument();
    expect(screen.queryByText('Payment')).not.toBeInTheDocument();
    expect(requests.some((request) => request.url.endsWith('/auth/sign-out'))).toBe(true);
  });

  it('keeps existing deployment routes reachable after sign-in', async () => {
    authenticated = true;
    window.history.replaceState({}, '', '/ui/applications/payment/environments/staging/deployments');
    render(<App />);
    await waitFor(() => expect(requests.some((request) => request.url.includes('/deployments'))).toBe(true));
    expect(screen.queryByText('Application not found')).not.toBeInTheDocument();
  });
});
