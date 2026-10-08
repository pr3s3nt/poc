import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SecretStoresPage } from './SecretStoresPage';

afterEach(() => vi.unstubAllGlobals());

const store = { key: 'team-vault', name: 'Team Vault', provider: 'VAULT_KV_V2', backendAddress: 'http://vault:8200', workloadAddress: 'http://vault.vault.svc:8200', mount: 'kv', authMount: 'kubernetes', tlsCustomCa: false, status: 'READY', verification: { kubernetesAuth: 'CONFIGURED' } };

it('registers a store with a concealed token, shows only safe metadata and clears the token', async () => {
  let created = false;
  vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
    if (init?.method === 'POST') { created = true; return Response.json(store, { status: 201 }); }
    return Response.json({ secretStores: created ? [store] : [] });
  }));
  const user = userEvent.setup();
  render(<SecretStoresPage />);
  expect(await screen.findByText('No secret store is registered yet.')).toBeInTheDocument();
  const submit = screen.getByRole('button', { name: 'Verify and register' });
  expect(submit).toBeDisabled();
  await user.type(screen.getByLabelText(/^Name/), 'Team Vault');
  await user.type(screen.getByLabelText(/^Backend address/), 'http://vault:8200');
  await user.type(screen.getByLabelText(/^Workload address/), 'http://vault.vault.svc:8200');
  const token = screen.getByLabelText(/^Token/);
  expect(token).toHaveAttribute('type', 'password');
  await user.type(token, 'hvs.secret-token');
  await user.click(submit);
  expect(await screen.findByRole('status')).toHaveTextContent('Team Vault" is READY');
  const post = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'POST');
  expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({ name: 'Team Vault', mount: 'kv', authMount: 'kubernetes', token: 'hvs.secret-token' });
  expect(await screen.findByText('Team Vault')).toBeInTheDocument();
  expect(screen.getByLabelText(/^Token/)).toHaveValue('');
  expect(document.body.textContent).not.toContain('hvs.secret-token');
});

it('keeps the form, clears the token and shows the verification failure', async () => {
  vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => init?.method === 'POST'
    ? Response.json({ error: 'secret store: verification failed: the secret store rejected the token' }, { status: 422 })
    : Response.json({ secretStores: [] })));
  const user = userEvent.setup();
  render(<SecretStoresPage />);
  await screen.findByText('No secret store is registered yet.');
  await user.type(screen.getByLabelText(/^Name/), 'Bad');
  await user.type(screen.getByLabelText(/^Backend address/), 'http://v:8200');
  await user.type(screen.getByLabelText(/^Workload address/), 'http://v:8200');
  await user.type(screen.getByLabelText(/^Token/), 'wrong');
  await user.click(screen.getByRole('button', { name: 'Verify and register' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('rejected the token');
  expect(screen.getByLabelText(/^Name/)).toHaveValue('Bad');
  expect(screen.getByLabelText(/^Token/)).toHaveValue('');
});
