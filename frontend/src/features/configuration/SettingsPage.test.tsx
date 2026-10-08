import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';
import type { Application } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging: configuredTarget('internal-cluster', { secretStoreKey: 'vault-a', secretStoreName: 'Vault A' }), production: configuredTarget('internal-cluster') }, workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

it('shows separate environment tabs and never displays saved secret values', async () => {
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    calls.push(`${init?.method ?? 'GET'} ${url}`);
    if (url.includes('/production/')) return Response.json({ applicationKey: 'catalog', environmentKey: 'production', version: 0, keys: [] });
    if (init?.method === 'PUT') return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, keys: [{ name: 'API_TOKEN', kind: 'SECRET', configured: true, usedBy: ['frontend'] }, { name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  expect(await screen.findByText('LOG_LEVEL')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: '+ Add secret' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'API_TOKEN');
  await user.type(screen.getByLabelText('New secret value'), 'private-value');
  await user.click(screen.getByRole('button', { name: 'Save pending change' }));
  expect(await screen.findByText('API_TOKEN')).toBeInTheDocument();
  expect(screen.queryByText('private-value')).not.toBeInTheDocument();
  expect(screen.getByText('Configured')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByText('No secrets configured in production.')).toBeInTheDocument();
  expect(calls.some((call) => call.includes('/production/configuration'))).toBe(true);
});

it('lets ordinary variables be added without a store but asks for a store before a Secret', async () => {
  const unselected: Application = { ...application, environments: { staging: configuredTarget('internal-cluster'), production: configuredTarget('internal-cluster') } };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [{ key: 'vault-a', name: 'Vault A', provider: 'VAULT_KV_V2', status: 'READY' }] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 0, keys: [] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={unselected} />);
  await screen.findByText('No environment variables configured in staging.');
  await user.click(screen.getByRole('button', { name: '+ Add variable' }));
  expect(screen.getByPlaceholderText('API_URL')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Cancel' }));
  await user.click(screen.getByRole('button', { name: '+ Add secret' }));
  expect(screen.queryByPlaceholderText('API_URL')).not.toBeInTheDocument();
  expect(screen.getByText(/Select a secret store above before adding a Secret/)).toBeInTheDocument();
});

it('shows the server sentence and reloads on a 409 conflict', async () => {
  let puts = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'PUT' && url.includes('/configuration/')) { puts += 1; return Response.json({ error: 'the environment is busy with another operation', code: 'ENVIRONMENT_BUSY' }, { status: 409 }); }
    if (url.endsWith('/secret-store-choices') || url.endsWith('/application-connections')) return Response.json({ secretStores: [], connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await user.click(await screen.findByRole('button', { name: '+ Add variable' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'MODE');
  await user.type(screen.getByLabelText('Value'), 'blue');
  await user.click(screen.getByRole('button', { name: 'Save pending change' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('busy with another operation');
  expect(puts).toBe(1);
});

it('holds every configuration write while an operation owns the environment, keeps open editor text and re-enables after release', async () => {
  const writes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method && init.method !== 'GET' && url.includes('/configuration')) writes.push(`${init.method} ${url}`);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const user = userEvent.setup();
  const view = render(<SettingsPage application={application} />);
  await screen.findByText('LOG_LEVEL');
  await user.click(screen.getByRole('button', { name: 'Edit' }));
  await user.clear(screen.getByLabelText('Value'));
  await user.type(screen.getByLabelText('Value'), 'trace');
  // The operation starts after the editor was opened.
  view.rerender(<SettingsPage application={busyApp} />);
  expect(screen.getByRole('button', { name: 'Save pending change' })).toBeDisabled();
  for (const name of ['+ Add variable', '+ Add secret', 'Edit', 'Rename', 'Delete']) expect(screen.getByRole('button', { name })).toBeDisabled();
  expect(screen.getByLabelText('Value')).toHaveValue('trace');
  expect(writes).toEqual([]);
  view.rerender(<SettingsPage application={application} />);
  expect(screen.getByRole('button', { name: 'Save pending change' })).toBeEnabled();
  expect(screen.getByLabelText('Value')).toHaveValue('trace');
  expect(screen.getByRole('button', { name: 'Rename' })).toBeEnabled();
});

it('holds an open rename or delete confirmation while busy', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const user = userEvent.setup();
  const view = render(<SettingsPage application={application} />);
  await screen.findByText('LOG_LEVEL');
  await user.click(screen.getByRole('button', { name: 'Rename' }));
  await user.clear(screen.getByLabelText('New key name'));
  await user.type(screen.getByLabelText('New key name'), 'LOGS');
  view.rerender(<SettingsPage application={busyApp} />);
  expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();
  expect(screen.getByLabelText('New key name')).toHaveValue('LOGS');
  view.rerender(<SettingsPage application={application} />);
  expect(screen.getByRole('button', { name: 'Continue' })).toBeEnabled();
});
