import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ReactElement } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ConnectionsPage } from './ConnectionsPage';
import { ResourceDefinitionsPage } from './ResourceDefinitionsPage';
import { ResourceTypesPage } from './ResourceTypesPage';

type User = ReturnType<typeof userEvent.setup>;

// One registration page: its list endpoint/field, how to fill it, the field
// that must be kept on failure and a created row.
const pages: { name: string; page: () => ReactElement; list: string; field: string; item: Record<string, unknown>; idLabel: string; fill: (user: User) => Promise<void>; submit: string; empty: string }[] = [
  { name: 'UC-02 types', page: () => <ResourceTypesPage />, list: '/resource-types', field: 'resourceTypes', item: { key: 'cache', inputs: [], outputs: [] }, idLabel: 'Resource type ID', fill: async (user) => { await user.type(screen.getByLabelText('Resource type ID'), 'cache'); }, submit: 'Register resource type', empty: 'No resource types registered yet.' },
  { name: 'UC-03 definitions', page: () => <ResourceDefinitionsPage />, list: '/resource-definitions', field: 'resourceDefinitions', item: { key: 'pg-fast', resourceType: 'postgres', driverType: 'kubernetes', criteria: [{}] }, idLabel: 'Definition ID', fill: async (user) => { await user.type(screen.getByLabelText('Definition ID'), 'pg-fast'); await user.selectOptions(screen.getByLabelText('Resource Type'), 'postgres'); }, submit: 'Register resource definition', empty: 'No resource definitions registered yet.' },
  { name: 'UC-04 connections', page: () => <ConnectionsPage />, list: '/connections', field: 'connections', item: { key: 'fast', kind: 'KUBERNETES', status: 'READY', config: { cluster: 'c', kubeContext: 'kind-fast' } }, idLabel: 'Connection ID', fill: async (user) => { await user.type(screen.getByLabelText('Connection ID'), 'fast'); await user.type(screen.getByLabelText('Cluster ID'), 'c'); await user.type(screen.getByLabelText('Kube context'), 'kind-fast'); }, submit: 'Register cluster', empty: 'No connections registered yet.' },
];

afterEach(() => vi.unstubAllGlobals());

// stub serves the page's list from `lists` (a queue of responses) and POST
// from `post`; resource types are always available for the Definition form.
function stub(list: string, lists: (() => Response)[], post: () => Response) {
  const calls = { posts: 0 };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'POST') { calls.posts += 1; return post(); }
    if (url.endsWith(list)) return (lists.shift() ?? lists[0] ?? (() => Response.json({})))();
    if (url.endsWith('/resource-types')) return Response.json({ resourceTypes: [{ key: 'postgres' }] });
    return Response.json({}, { status: 404 });
  }));
  return calls;
}

describe.each(pages)('$name registration states', ({ page, list, field, item, idLabel, fill, submit, empty }) => {
  const listed = (items: unknown[]) => () => Response.json({ [field]: items });
  const failed = () => Response.json({ error: 'unavailable' }, { status: 500 });

  it('shows an initial load failure with Retry, never an empty list', async () => {
    const user = userEvent.setup();
    stub(list, [failed, listed([])], () => Response.json({}, { status: 201 }));
    render(page());
    const alert = await screen.findByRole('alert', { name: 'List error' });
    expect(screen.queryByText(empty)).not.toBeInTheDocument();
    await user.click(within(alert).getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText(empty)).toBeInTheDocument();
  });

  it('keeps the submitted form when registration fails', async () => {
    const user = userEvent.setup();
    const calls = stub(list, [listed([])], () => Response.json({ error: 'duplicate id' }, { status: 409 }));
    render(page());
    await screen.findByText(empty);
    await fill(user);
    await user.click(screen.getByRole('button', { name: submit }));
    expect(await screen.findByText('duplicate id')).toBeInTheDocument();
    expect(screen.getByLabelText(idLabel)).not.toHaveValue('');
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
    expect(calls.posts).toBe(1);
  });

  it('freezes the form while registration is in flight, so no new input is lost on success', async () => {
    const user = userEvent.setup();
    let release: (response: Response) => void = () => undefined;
    const calls = stub(list, [listed([]), listed([item])], () => undefined as unknown as Response);
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = String(input);
      if (init?.method === 'POST') { calls.posts += 1; return new Promise<Response>((done) => { release = done; }); }
      if (url.endsWith(list)) return listed(calls.posts ? [item] : [])();
      return Response.json({ resourceTypes: [{ key: 'postgres' }] });
    }));
    render(page());
    await screen.findByText(empty);
    await fill(user);
    await user.click(screen.getByRole('button', { name: submit }));
    const idField = screen.getByLabelText(idLabel);
    expect(idField).toBeDisabled();
    await user.type(idField, 'next-draft');
    expect(idField).not.toHaveValue(expect.stringContaining('next-draft'));
    release(Response.json(item, { status: 201 }));
    expect(await screen.findByRole('status')).toHaveTextContent(/Registered/);
    expect(screen.getByLabelText(idLabel)).toBeEnabled();
    expect(screen.getByLabelText(idLabel)).toHaveValue('');
    await user.type(screen.getByLabelText(idLabel), 'next-draft');
    expect(screen.getByLabelText(idLabel)).toHaveValue('next-draft');
    expect(calls.posts).toBe(1);
  });

  it('reports a committed registration even when the list reload fails, and retries only the list', async () => {
    const user = userEvent.setup();
    const calls = stub(list, [listed([]), failed, listed([item])], () => Response.json(item, { status: 201 }));
    render(page());
    await screen.findByText(empty);
    await fill(user);
    await user.click(screen.getByRole('button', { name: submit }));
    expect(await screen.findByRole('status')).toHaveTextContent(/Registered/);
    expect(screen.getByLabelText(idLabel)).toHaveValue('');
    const alert = await screen.findByRole('alert', { name: 'List error' });
    expect(screen.getByRole('button', { name: submit })).toBeEnabled();
    await user.click(within(alert).getByRole('button', { name: 'Retry' }));
    expect(await screen.findByText(String(item.key))).toBeInTheDocument();
    expect(calls.posts).toBe(1);
  });
});

describe('UC-04 connection list', () => {
  it('shows cluster ID and kube context for READY connections only', async () => {
    stub('/connections', [() => Response.json({ connections: [
      { key: 'ready-one', kind: 'KUBERNETES', status: 'READY', config: { cluster: 'kind-a', kubeContext: 'kind-ctx-a' } },
      { key: 'pending-one', kind: 'KUBERNETES', status: 'VERIFYING', config: { cluster: 'kind-b', kubeContext: 'kind-ctx-b' } },
    ] })], () => Response.json({}, { status: 201 }));
    render(<ConnectionsPage />);
    expect(await screen.findByText('ready-one')).toBeInTheDocument();
    expect(screen.getByText(/cluster kind-a · context kind-ctx-a · READY/)).toBeInTheDocument();
    expect(screen.queryByText('pending-one')).not.toBeInTheDocument();
  });
});
