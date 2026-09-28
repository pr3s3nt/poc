import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { WorkloadEditorPage } from './WorkloadEditorPage';
import type { Application } from '../../shared/types/application';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', workloads: { staging: [], production: [] } };

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
  await user.click(screen.getByRole('button', { name: '+ Add binding' }));
  await user.type(screen.getByLabelText('Container variable name'), 'BACKEND_URL');
  await user.selectOptions(screen.getByLabelText('Application key'), 'API_URL');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect(saved).toBeDefined();
  const score = saved?.score as { containers: { main: { variables: Record<string, string> } }; resources: { env: { type: string } } };
  expect(score.containers.main.variables.BACKEND_URL).toBe('${resources.env.API_URL}');
  expect(score.resources.env.type).toBe('environment');
  expect(JSON.stringify(saved)).not.toContain('https://internal.example');
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

it('saves the selected public Service port without deploying', async () => {
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
  await user.selectOptions(screen.getByLabelText('Public access port'), 'http');
  await user.click(screen.getByRole('button', { name: 'Save pending workload' }));
  expect((saved?.score as { service: { publicPort: string } }).service.publicPort).toBe('http');
});
