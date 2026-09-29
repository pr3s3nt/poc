import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';

describe('UC-03 definition registration', () => {
  it('submits a wildcard criterion and embedded driver variables', async () => {
    const user = userEvent.setup();
    let created: Record<string, unknown> | undefined;
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'k8s-namespace' }] });
      if (init?.method === 'POST') { created = JSON.parse(String(init.body)) as Record<string, unknown>; return Response.json(created, { status: 201 }); }
      return Response.json({ resourceDefinitions: created ? [created] : [] });
    }));
    render(<ResourceDefinitionsPage />);
    await screen.findByText('No resource definitions registered yet.');
    await user.type(screen.getByLabelText('Definition ID'), 'namespace-custom');
    await user.selectOptions(screen.getByLabelText('Resource Type'), 'k8s-namespace');
    fireEvent.change(screen.getByLabelText('Driver variables (JSON object)'), { target: { value: '{"name":"orch"}' } });
    await user.click(screen.getByRole('button', { name: 'Register resource definition' }));
    expect(await screen.findByText('namespace-custom')).toBeInTheDocument();
    expect(created).toMatchObject({ resourceType: 'k8s-namespace', criteria: [{}], driverInputs: { values: { variables: { name: 'orch' } } } });
  });
});
