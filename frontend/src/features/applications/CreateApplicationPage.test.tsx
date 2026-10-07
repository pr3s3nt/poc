import { StrictMode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CreateApplicationPage } from './CreateApplicationPage';
import { ApiError } from '../../shared/api/client';

const internal = { key: 'internal-cluster', name: 'Internal cluster', kind: 'KUBERNETES', status: 'READY' };
const lab = { key: 'lab', name: 'Lab', kind: 'KUBERNETES', status: 'READY' };
const cloud = { key: 'aws-account', name: 'AWS', kind: 'AWS', status: 'READY' };

type Reply = () => Response | Promise<Response>;

// Stubs GET /application-connections with a queue of replies; the last one repeats.
function stubChoices(...replies: Reply[]) {
  let calls = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    expect(String(input)).toBe('/api/v1/application-connections');
    const reply = replies[Math.min(calls++, replies.length - 1)] as Reply;
    return reply();
  }));
  return () => calls;
}
const choices = (connections: unknown[], defaultConnectionKey = ''): Reply => () => Response.json({ connections, defaultConnectionKey });

afterEach(() => vi.unstubAllGlobals());

async function fill(name: string, subdomain: string) {
  const user = userEvent.setup();
  if (name) await user.type(screen.getByLabelText('Application name'), name);
  if (subdomain) await user.type(screen.getByLabelText('Subdomain'), subdomain);
  await user.click(screen.getByRole('button', { name: 'Create application' }));
}
async function ready() { await screen.findByLabelText('Connection'); }

it('offers name, subdomain and a connection, preselects the default and previews both endpoints', async () => {
  stubChoices(choices([internal, lab, cloud], 'internal-cluster'));
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  await ready();
  expect(screen.getAllByRole('textbox')).toHaveLength(2);
  expect(screen.getByLabelText('Connection')).toHaveValue('internal-cluster');
  expect(screen.getByRole('option', { name: /Internal cluster.*default/ })).toBeInTheDocument();
  expect(screen.getByRole('option', { name: /AWS \(aws-account\) · AWS/ })).toBeInTheDocument();
  expect(screen.getByText('staging.your-app.example.com')).toBeInTheDocument();
});

it('submits a non-default selection', async () => {
  stubChoices(choices([internal, lab], 'internal-cluster'));
  const onCreate = vi.fn().mockResolvedValue('app-1');
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  await userEvent.setup().selectOptions(screen.getByLabelText('Connection'), 'lab');
  await fill('Catalog', 'Catalog');
  expect(onCreate).toHaveBeenCalledWith('Catalog', 'catalog', 'lab');
});

it('requires an explicit choice when no default is eligible', async () => {
  stubChoices(choices([internal, lab], ''));
  const onCreate = vi.fn().mockResolvedValue('app-1');
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  expect(screen.getByLabelText('Connection')).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Create application' })).toBeDisabled();
  await userEvent.setup().selectOptions(screen.getByLabelText('Connection'), 'lab');
  expect(screen.getByRole('button', { name: 'Create application' })).toBeEnabled();
  await fill('Catalog', 'catalog');
  expect(onCreate).toHaveBeenCalledWith('Catalog', 'catalog', 'lab');
});

it('blocks submission while loading, shows an error with retry and keeps entered fields', async () => {
  let release: (response: Response) => void = () => undefined;
  const pending = new Promise<Response>((resolve) => { release = resolve; });
  const calls = stubChoices(() => Response.json({ error: 'boom' }, { status: 500 }), () => pending);
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  expect(screen.getByText('Loading connections…')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Create application' })).toBeDisabled();
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not load connections.');
  await user.type(screen.getByLabelText('Application name'), 'Catalog');
  expect(screen.getByRole('button', { name: 'Create application' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  expect(screen.getByText('Loading connections…')).toBeInTheDocument();
  release(Response.json({ connections: [internal, lab], defaultConnectionKey: 'internal-cluster' }));
  await ready();
  expect(calls()).toBe(2);
  expect(screen.getByLabelText('Application name')).toHaveValue('Catalog');
  expect(screen.getByLabelText('Connection')).toHaveValue('internal-cluster');
});

it('shows an empty state without fixtures and keeps submit disabled', async () => {
  stubChoices(choices([]));
  render(<CreateApplicationPage onCreate={vi.fn()} />);
  expect(await screen.findByText(/No READY connection is available/)).toBeInTheDocument();
  expect(screen.queryByLabelText('Connection')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Create application' })).toBeDisabled();
});

it('ignores the obsolete reply when two loads overlap', async () => {
  let releaseOld: (response: Response) => void = () => undefined;
  const old = new Promise<Response>((resolve) => { releaseOld = resolve; });
  // StrictMode starts two loads; the first one answers last with other data.
  stubChoices(() => old, choices([internal, lab], 'internal-cluster'));
  render(<StrictMode><CreateApplicationPage onCreate={vi.fn()} /></StrictMode>);
  await ready();
  expect(screen.getByLabelText('Connection')).toHaveValue('internal-cluster');
  releaseOld(Response.json({ connections: [cloud], defaultConnectionKey: 'aws-account' }));
  await new Promise((resolve) => setTimeout(resolve, 20));
  expect(screen.getByLabelText('Connection')).toHaveValue('internal-cluster');
  expect(screen.queryByRole('option', { name: /aws-account/ })).not.toBeInTheDocument();
});

it('never replaces a vanished explicit choice with the default', async () => {
  const calls = stubChoices(choices([internal, lab], 'internal-cluster'), choices([internal], 'internal-cluster'));
  const onCreate = vi.fn().mockRejectedValueOnce(new ApiError(422, 'not available', 'connectionKey'));
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  await userEvent.setup().selectOptions(screen.getByLabelText('Connection'), 'lab');
  await fill('Catalog', 'catalog');
  await waitFor(() => expect(calls()).toBe(2));
  await waitFor(() => expect(screen.getByLabelText('Connection')).toHaveValue(''));
  expect(screen.getByText(/selected connection is no longer available/)).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Create application' })).toBeDisabled();
  expect(screen.getByLabelText('Application name')).toHaveValue('Catalog');
  await userEvent.setup().selectOptions(screen.getByLabelText('Connection'), 'internal-cluster');
  expect(screen.getByRole('button', { name: 'Create application' })).toBeEnabled();
  expect(onCreate).toHaveBeenCalledTimes(1);
});

it('validates inline without calling the API and keeps the URL preview', async () => {
  stubChoices(choices([internal], 'internal-cluster'));
  const onCreate = vi.fn();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  await fill('', 'bad_label');
  expect(screen.getByText('Enter an application name.')).toBeInTheDocument();
  expect(screen.getByText(/Use lowercase letters, numbers and hyphens;/)).toBeInTheDocument();
  expect(screen.getByText('staging.bad_label.example.com')).toBeInTheDocument();
  expect(onCreate).not.toHaveBeenCalled();
});

it('prevents duplicate submit and maps a duplicate subdomain to its field', async () => {
  stubChoices(choices([internal, lab], 'internal-cluster'));
  let fail: (error: unknown) => void = () => undefined;
  const onCreate = vi.fn(() => new Promise<string>((_, reject) => { fail = reject; }));
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  await userEvent.setup().selectOptions(screen.getByLabelText('Connection'), 'lab');
  await fill('Catalog', 'Catalog');
  expect(onCreate).toHaveBeenCalledWith('Catalog', 'catalog', 'lab');
  expect(screen.getByRole('button', { name: 'Creating application…' })).toBeDisabled();
  fail(new ApiError(409, 'application: subdomain is already in use', 'subdomain'));
  expect(await screen.findByText('This subdomain is already in use.')).toBeInTheDocument();
  expect(screen.getByLabelText('Application name')).toHaveValue('Catalog');
  expect(screen.getByLabelText('Subdomain')).toHaveValue('Catalog');
  expect(screen.getByLabelText('Connection')).toHaveValue('lab');
  expect(onCreate).toHaveBeenCalledTimes(1);
});

it('maps a duplicate name and shows form-level API errors', async () => {
  stubChoices(choices([internal], 'internal-cluster'));
  const onCreate = vi.fn()
    .mockRejectedValueOnce(new ApiError(409, 'application: application name already exists in the organization', 'name'))
    .mockRejectedValueOnce(new ApiError(500, 'boom'));
  const user = userEvent.setup();
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  await fill('Catalog', 'catalog');
  expect(await screen.findByText('An application with this name already exists.')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Create application' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not create the application. Try again.');
  expect(screen.getByLabelText('Subdomain')).toHaveValue('catalog');
});

it('maps an unavailable connection to its field, reloads choices and keeps the form', async () => {
  const calls = stubChoices(choices([internal, lab], 'internal-cluster'), choices([internal], 'internal-cluster'));
  const onCreate = vi.fn().mockRejectedValueOnce(new ApiError(422, 'application: the selected connection is not available', 'connectionKey'));
  render(<CreateApplicationPage onCreate={onCreate} />);
  await ready();
  await userEvent.setup().selectOptions(screen.getByLabelText('Connection'), 'lab');
  await fill('Catalog', 'catalog');
  expect(await screen.findByText(/selected connection is no longer available/)).toBeInTheDocument();
  await waitFor(() => expect(calls()).toBe(2));
  await ready();
  expect(screen.getByLabelText('Connection')).toHaveValue('');
  expect(screen.getByLabelText('Application name')).toHaveValue('Catalog');
  expect(screen.getByLabelText('Subdomain')).toHaveValue('catalog');
});
