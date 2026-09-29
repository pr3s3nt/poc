import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ConnectionsPage } from './ConnectionsPage';

describe('UC-04 Kubernetes connections', () => {
  it('registers only cluster identity and host kube context', async () => {
    const user = userEvent.setup();
    let submitted: Record<string, string> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (_input: string | URL | Request, init?: RequestInit) => {
      if (init?.method === 'POST') { submitted = JSON.parse(String(init.body)) as Record<string, string>; return Response.json({ key: submitted.key }, { status: 201 }); }
      return Response.json({ connections: submitted ? [{ key: submitted.key, kind: 'KUBERNETES', status: 'READY', config: { cluster: submitted.clusterId, kubeContext: submitted.kubeContext } }] : [] });
    }));
    render(<ConnectionsPage />);
    await screen.findByText('No connections registered yet.');
    await user.type(screen.getByLabelText('Connection ID'), 'internal');
    await user.type(screen.getByLabelText('Cluster ID'), 'kind-internal');
    await user.type(screen.getByLabelText('Kube context'), 'kind-idp-internal');
    await user.click(screen.getByRole('button', { name: 'Register cluster' }));
    expect(await screen.findByText('internal')).toBeInTheDocument();
    expect(submitted).toEqual({ key: 'internal', clusterId: 'kind-internal', kubeContext: 'kind-idp-internal' });
  });
});
