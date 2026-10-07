import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';
import type { Application } from '../../shared/types/application';
import { bothConfigured } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: bothConfigured('internal-cluster'), workloads: { staging: [], production: [] } };

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
