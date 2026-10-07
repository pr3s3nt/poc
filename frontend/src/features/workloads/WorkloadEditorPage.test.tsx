import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { Application } from '../../shared/types/application';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', connectionKey: 'internal-cluster', profile: 'internal-k8s', workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('saves an Application key as a Score reference, not a copied value', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'API_URL', kind: 'VARIABLE', value: 'https://internal.example', configured: true, usedBy: [] }] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') {
      saved = JSON.parse(String(init.body)) as Record<string, unknown>;
      return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 1, workloads: [] });
    }
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.type(screen.getByLabelText('Workload name'), 'frontend');
  await user.type(screen.getByLabelText('Image'), 'example.invalid/frontend:test');
  await user.click(within(screen.getByRole('group', { name: 'Application variables for main' })).getByRole('checkbox', { name: 'API_URL' }));
  await user.click(screen.getByRole('checkbox', { name: 'Use a different container name for API_URL' }));
  await user.clear(screen.getByLabelText('Container name for API_URL'));
  await user.type(screen.getByLabelText('Container name for API_URL'), 'BACKEND_URL');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(saved).toBeDefined();
  const score = saved?.score as { containers: { main: { variables: Record<string, string> } }; resources: { env: { type: string } } };
  expect(score.containers.main.variables).toEqual({ BACKEND_URL: '${resources.env.API_URL}' });
  expect(score.resources.env.type).toBe('environment');
  expect(JSON.stringify(saved)).not.toContain('https://internal.example');
});

it('keeps edits after a save conflict, blocks Save until an explicit reload, and never resends on its own', async () => {
  const puts: number[] = [];
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') {
      puts.push((JSON.parse(String(init.body)) as { version: number }).version);
      return puts.length === 1 ? Response.json({ error: 'workloads changed since they were loaded; reload and try again' }, { status: 409 }) : Response.json({ draftVersion: 6, workloads: [] });
    }
    lists += 1;
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', draftVersion: lists === 1 ? 4 : 5, workloads: lists === 1 ? [] : [{ id: 'web', state: 'PENDING_UPSERT', ready: false, score: { metadata: { name: 'web' }, containers: { main: { image: 'other:v9' } } } }] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.type(screen.getByLabelText('Workload name'), 'web');
  await user.type(screen.getByLabelText('Image'), 'mine:v1');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(await screen.findByText(/changed after this page loaded/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Save pending workload' })).toBeDisabled();
  expect(screen.getByLabelText('Image')).toHaveValue('mine:v1');
  expect(puts).toEqual([4]);
  await user.click(screen.getByRole('button', { name: 'Reload current state' }));
  expect(await screen.findByText(/Reloaded draft version 5/)).toBeInTheDocument();
  expect(screen.getByText(/pending change/)).toBeInTheDocument();
  expect(screen.getByLabelText('Image')).toHaveValue('mine:v1');
  expect(puts).toEqual([4]);
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(puts).toEqual([4, 5]);
});

it('keeps the stale panel and retry when reloading the current state fails', async () => {
  let lists = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') return Response.json({ error: 'conflict' }, { status: 409 });
    lists += 1;
    return lists === 2 ? Response.json({ error: 'unavailable' }, { status: 500 }) : Response.json({ draftVersion: lists, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.type(screen.getByLabelText('Workload name'), 'web');
  await user.type(screen.getByLabelText('Image'), 'mine:v1');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  await user.click(await screen.findByRole('button', { name: 'Reload current state' }));
  const panel = screen.getByLabelText('Stale workload drafts');
  expect(await within(panel).findByText(/Could not reload the current state/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Save pending workload' })).toBeDisabled();
  await user.click(within(panel).getByRole('button', { name: 'Reload current state' }));
  expect(await screen.findByText(/Reloaded draft version 3/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Stale workload drafts')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Image')).toHaveValue('mine:v1');
});

it('offers retry when the editor cannot load', async () => {
  let calls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    calls += 1;
    return calls === 1 ? Response.json({ error: 'unavailable' }, { status: 500 }) : Response.json({ draftVersion: 1, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await user.click(await screen.findByRole('button', { name: 'Retry' }));
  expect(await screen.findByRole('heading', { name: 'Basic information' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Save pending workload' })).toBeEnabled();
});

it('requires PostgreSQL inputs and saves them as Score resource params', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', inputs: [{ name: 'database', type: 'string', required: true }, { name: 'username', type: 'string', required: true }], outputs: [{ name: 'host' }] }] });
    if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json({ draftVersion: 1, workloads: [] }); }
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.type(screen.getByLabelText('Workload name'), 'backend');
  await user.type(screen.getByLabelText('Image'), 'example.invalid/backend:test');
  await user.click(screen.getByRole('button', { name: '+ Add resource' }));
  await user.type(screen.getByLabelText('Resource alias'), 'db');
  await user.selectOptions(screen.getByLabelText('Resource type'), 'postgres');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('database is required');
  expect(saved).toBeUndefined();
  await user.type(screen.getByLabelText('Resource database'), 'catalog');
  await user.type(screen.getByLabelText('Resource username'), 'app');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(saved).toBeDefined();
  const score = saved?.score as { resources: { db: { type: string; params: Record<string, string> } } };
  expect(score.resources.db).toEqual({ type: 'postgres', params: { database: 'catalog', username: 'app' } });
});

it('preserves PostgreSQL params when editing a workload on the form', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', inputs: [{ name: 'database', type: 'string', required: true }, { name: 'username', type: 'string', required: true }], outputs: [] }] });
    if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json({ draftVersion: 2, workloads: [] }); }
    return Response.json({ draftVersion: 1, workloads: [{ id: 'backend', score: { apiVersion: 'score.dev/v1b1', metadata: { name: 'backend' }, containers: { main: { image: 'example.invalid/backend:v1' } }, resources: { db: { type: 'postgres', params: { database: 'catalog', username: 'app' } } } } }] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" workloadId="backend" />);
  expect(await screen.findByLabelText('Resource database')).toHaveValue('catalog');
  expect(screen.getByLabelText('Resource username')).toHaveValue('app');
  await user.clear(screen.getByLabelText('Resource database'));
  await user.type(screen.getByLabelText('Resource database'), 'catalog_v2');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  const score = saved?.score as { resources: { db: { params: Record<string, string> } } };
  expect(score.resources.db.params).toEqual({ database: 'catalog_v2', username: 'app' });
});

it('saves a public path and Service port without deploying', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (init?.method === 'PUT') { saved = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json({ draftVersion: 1, workloads: [] }); }
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.type(screen.getByLabelText('Workload name'), 'frontend');
  await user.type(screen.getByLabelText('Image'), 'example.invalid/frontend:v1');
  await user.click(screen.getByRole('button', { name: '+ Add port' }));
  await user.type(screen.getByLabelText('Service port name'), 'http');
  await user.type(screen.getByLabelText('Service port', { exact: true }), '8080');
  await user.click(screen.getByRole('button', { name: '+ Add public path' }));
  await user.type(screen.getByLabelText('Public path'), '/');
  await user.selectOptions(screen.getByLabelText('Public Service port'), 'http');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect((saved?.score as { service: { publicRoutes: { path: string; port: string }[] } }).service.publicRoutes).toEqual([{ path: '/', port: 'http' }]);
});

it('shows a backend resource param rejection, keeps the form and does not report a saved draft', async () => {
  const puts: unknown[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres', inputs: [{ name: 'database', type: 'string', required: true }], outputs: [{ name: 'host' }] }] });
    if (init?.method === 'PUT') { puts.push(JSON.parse(String(init.body))); return Response.json({ error: 'workloadconfig: invalid input: resources.db.params.database is required by resource type postgres' }, { status: 400 }); }
    return Response.json({ draftVersion: 3, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.type(screen.getByLabelText('Workload name'), 'backend');
  await user.type(screen.getByLabelText('Image'), 'example.invalid/backend:test');
  await user.click(screen.getByRole('button', { name: '+ Add resource' }));
  await user.type(screen.getByLabelText('Resource alias'), 'db');
  await user.selectOptions(screen.getByLabelText('Resource type'), 'postgres');
  await user.type(screen.getByLabelText('Resource database'), 'catalog');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('resources.db.params.database is required');
  expect(puts).toHaveLength(1);
  expect((puts[0] as { version: number }).version).toBe(3);
  expect(screen.getByRole('heading', { name: 'Add workload' })).toBeInTheDocument();
  expect(screen.getByLabelText('Workload name')).toHaveValue('backend');
  expect(screen.getByLabelText('Resource database')).toHaveValue('catalog');
  expect(screen.getByRole('button', { name: 'Save pending workload' })).toBeEnabled();
});

it('shows a Score import rejection without presenting the file as parsed', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/configuration')) return Response.json({ keys: [] });
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [] });
    if (url.endsWith('/workloads/parse')) return Response.json({ error: 'workloadconfig: invalid input: resources.cache.type is not a registered resource type' }, { status: 400 });
    return Response.json({ draftVersion: 0, workloads: [] });
  }));
  const user = userEvent.setup();
  render(<WorkloadEditorPage application={application} environment="staging" />);
  await screen.findByRole('heading', { name: 'Basic information' });
  await user.click(screen.getByRole('button', { name: 'Import Score' }));
  const content = 'apiVersion: score.dev/v1b1\nmetadata:\n  name: api\n';
  // jsdom File has no text(); provide it so the page reads the upload.
  const file = Object.assign(new File([content], 'score.yaml', { type: 'text/yaml' }), { text: async () => content });
  await user.upload(screen.getByLabelText('Score file'), file);
  expect(await screen.findByRole('alert')).toHaveTextContent('resources.cache.type is not a registered resource type');
  expect(screen.queryByText(/Parsed workload/)).not.toBeInTheDocument();
});
