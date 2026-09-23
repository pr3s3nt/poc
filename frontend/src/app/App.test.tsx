import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { App } from './App';

describe('developer console shell', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/ui/sign-in');
    let authenticated = false;
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/auth/session')) return authenticated ? Response.json({ user: {} }) : Response.json({ error: 'unauthorized' }, { status: 401 });
      if (url.endsWith('/auth/sign-in')) { authenticated = true; return Response.json({ user: {} }); }
      if (url.endsWith('/applications') && init?.method === 'POST') return Response.json({ application: { key: 'catalog', name: 'Catalog', subdomain: 'catalog' } }, { status: 201 });
      if (url.endsWith('/applications')) return Response.json({ applications: [{ key: 'payment', name: 'Payment', subdomain: 'payment' }] });
      return Response.json({});
    }));
  });

  it('signs in to the applications home with the local prototype flow', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.type(screen.getByLabelText('Username'), 'developer');
    await user.type(screen.getByLabelText('Password'), 'test-password');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));

    expect(await screen.findByRole('heading', { name: 'Your applications' })).toBeInTheDocument();
    expect(screen.getByText('Payment')).toBeInTheDocument();
  });

  it('creates an application with staging and production endpoints', async () => {
    const user = userEvent.setup();
    render(<App />);

    await user.type(screen.getByLabelText('Username'), 'developer');
    await user.type(screen.getByLabelText('Password'), 'test-password');
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    await screen.findByRole('heading', { name: 'Your applications' });
    await user.click(screen.getByRole('button', { name: /create application/i }));
    await user.type(screen.getByLabelText('Application name'), 'Catalog');
    await user.type(screen.getByLabelText('Subdomain'), 'catalog');
    await user.click(screen.getByRole('button', { name: 'Create application' }));

    expect(await screen.findByRole('heading', { name: 'Catalog' })).toBeInTheDocument();
    expect(screen.getByText('staging.catalog.example.com')).toBeInTheDocument();
    expect(screen.getAllByText('catalog.example.com')).toHaveLength(2);
  });
});
