import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ResourceTypesPage } from './ResourceTypesPage';

describe('UC-02 resource type registration', () => {
  it('registers input/output contract and refreshes the organization catalog', async () => {
    const user = userEvent.setup();
    let created: Record<string, unknown> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === 'POST') { created = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json(created, { status: 201 }); }
      return Response.json({ resourceTypes: created ? [created] : [] });
    }));
    render(<ResourceTypesPage />);
    await user.type(screen.getByLabelText('Resource type ID'), 'redis');
    await user.click(screen.getByRole('button', { name: '+ Add input' }));
    await user.type(screen.getByLabelText('Inputs 1 name'), 'size');
    await user.click(screen.getByRole('button', { name: '+ Add output' }));
    await user.type(screen.getByLabelText('Outputs 1 name'), 'password');
    await user.click(screen.getByLabelText('Secret'));
    await user.click(screen.getByRole('button', { name: 'Register resource type' }));
    expect(await screen.findByText('redis')).toBeInTheDocument();
    expect(created).toMatchObject({ key: 'redis', inputs: [{ name: 'size', type: 'string' }], outputs: [{ name: 'password', secret: true }] });
  });
});
