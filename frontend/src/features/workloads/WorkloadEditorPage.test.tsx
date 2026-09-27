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
