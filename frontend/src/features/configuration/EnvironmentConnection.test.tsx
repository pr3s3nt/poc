import { useState } from 'react';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';
import { unconfiguredTarget, type Application, type EnvironmentTarget } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';

const lab = { key: 'lab', name: 'Lab', kind: 'KUBERNETES', status: 'READY' };
const other = { key: 'other', name: 'Other', kind: 'KUBERNETES', status: 'READY' };
const cloud = { key: 'cloud', name: 'Cloud', kind: 'AWS', status: 'READY' };
const base: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging: unconfiguredTarget, production: unconfiguredTarget }, workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

function envJson(key: string, target: EnvironmentTarget) {
  return { key, version: target.version, configured: target.configured, connectionKey: target.connectionKey, connectionName: target.connectionName, connectionKind: target.connectionKind, executionProfile: target.profile, region: target.region, runtimeStatus: target.runtimeStatus, infrastructureScope: target.infrastructureScope, secretStoreKey: target.secretStoreKey, targetGeneration: target.targetGeneration, runtimeExists: target.runtimeExists, activeOperation: target.activeOperation };
}

function Host({ initial = base }: { initial?: Application }) {
  const [application, setApplication] = useState(initial);
  return <SettingsPage application={application} onTargetChange={(_id, env, target) => setApplication((current) => ({ ...current, environments: { ...current.environments, [env]: target } }))} />;
}

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response> | undefined;
function stub(handler: Handler = () => undefined) {
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    calls.push(`${init?.method ?? 'GET'} ${url}`);
    const custom = await handler(url, init);
    if (custom) return custom;
    if (url.endsWith('/application-connections')) return Response.json({ connections: [lab, other, cloud] });
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    if (url.includes('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: url.includes('/production/') ? 'production' : 'staging', version: 0, keys: [] });
    return Response.json({});
  }));
  return calls;
}
const putOf = () => vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PUT');

it('lists choices without a preselection and saves the first selection with the current version', async () => {
  const bound = configuredTarget('lab', { connectionName: 'Lab', version: 2 });
  const calls = stub((_url, init) => init?.method === 'PUT' ? Response.json({ environment: envJson('staging', bound) }) : undefined);
  const user = userEvent.setup();
  render(<Host />);
  const select = await screen.findByLabelText('Connection for Staging');
  expect(select).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Save connection' })).toBeDisabled();
  await user.selectOptions(select, 'lab');
  expect(calls.some((call) => call.startsWith('PUT'))).toBe(false);
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  expect(await screen.findByText('Generation 0')).toBeInTheDocument();
  expect(String(putOf()?.[0])).toBe('/api/v1/applications/catalog/environments/staging/connection');
  expect(JSON.parse(String(putOf()?.[1]?.body))).toEqual({ connectionKey: 'lab', expectedVersion: 1 });
  // Not locked: the selector stays editable, and saving the same key is disabled.
  expect(screen.getByLabelText('Connection for Staging')).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Save connection' })).toBeDisabled();
});

it('changes a configured connection before runtime exists using the new version and a distinct kind', async () => {
  const start = configuredTarget('lab', { connectionName: 'Lab', version: 3 });
  const aws = configuredTarget('cloud', { connectionName: 'Cloud', connectionKind: 'AWS', profile: 'aws-eks', region: 'us-east-1', runtimeStatus: 'PENDING', version: 4 });
  stub((_url, init) => init?.method === 'PUT' ? Response.json({ environment: envJson('staging', aws) }) : undefined);
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: start, production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'cloud');
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  expect(await screen.findByText('aws-eks')).toBeInTheDocument();
  expect(JSON.parse(String(putOf()?.[1]?.body))).toEqual({ connectionKey: 'cloud', expectedVersion: 3 });
  expect(screen.getByText('us-east-1')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByLabelText('Connection for Production')).toHaveValue('');
});

it('requires an explicit transition once runtime exists and never sends a silent PUT', async () => {
  const deployed = configuredTarget('lab', { connectionName: 'Lab', runtimeExists: true });
  const calls = stub();
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: deployed, production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'other');
  expect(screen.queryByRole('button', { name: 'Save connection' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Review transition…' }));
  expect(await screen.findByRole('region', { name: 'Connection transition' })).toBeInTheDocument();
  expect(screen.getByLabelText('Destination connection')).toHaveValue('other');
  expect(calls.some((call) => call.startsWith('PUT') && call.includes('/connection'))).toBe(false);
});

it('blocks edits while an operation owns the Environment', async () => {
  const busy = configuredTarget('lab', { activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } });
  stub();
  render(<Host initial={{ ...base, environments: { staging: busy, production: unconfiguredTarget } }} />);
  expect(await screen.findByLabelText('Connection for Staging')).toBeDisabled();
  expect(screen.getByRole('region', { name: 'Environment operation' })).toHaveTextContent('Operation in progress');
});

it('on a stale-version 409 shows the message, reloads the latest Environment and resubmits only on request', async () => {
  const latest = configuredTarget('other', { connectionName: 'Other', version: 7 });
  let puts = 0;
  stub((url, init) => {
    if (init?.method === 'PUT') { puts += 1; return puts === 1 ? Response.json({ error: 'the environment changed since it was loaded; reload and try again', code: 'STALE_VERSION' }, { status: 409 }) : Response.json({ environment: envJson('staging', configuredTarget('lab', { version: 8 })) }); }
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson('staging', latest), envJson('production', unconfiguredTarget)] } });
  });
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('cloud', { version: 3 }), production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('changed since it was loaded');
  await waitFor(() => expect(screen.getByText('Generation 0')).toBeInTheDocument());
  expect(puts).toBe(1);
  await user.selectOptions(screen.getByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  await waitFor(() => expect(puts).toBe(2));
  expect(JSON.parse(String(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT')[1]?.[1]?.body))).toEqual({ connectionKey: 'lab', expectedVersion: 7 });
});

it('shows ENVIRONMENT_BUSY as a temporary message', async () => {
  stub((_url, init) => init?.method === 'PUT' ? Response.json({ error: 'the environment is busy with another operation; wait for it to finish or review its status', code: 'ENVIRONMENT_BUSY' }, { status: 409 }) : undefined);
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { version: 3 }), production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'other');
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('busy with another operation');
});

it('treats an unavailable connection as a safe 422 and reloads the choices', async () => {
  let loads = 0;
  stub((url, init) => {
    if (url.endsWith('/application-connections')) { loads += 1; }
    if (init?.method === 'PUT') return Response.json({ error: 'the selected connection is not available' }, { status: 422 });
  });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('no longer available');
  await waitFor(() => expect(loads).toBeGreaterThan(1));
});

it('handles an empty list and a failed list with retry', async () => {
  let mode: 'empty' | 'error' | 'ok' = 'error';
  stub((url) => {
    if (url.endsWith('/application-connections')) return mode === 'error' ? Response.json({ error: 'x' }, { status: 500 }) : Response.json({ connections: mode === 'empty' ? [] : [lab] });
  });
  const user = userEvent.setup();
  render(<Host />);
  const region = () => within(screen.getByRole('region', { name: 'Execution connection' }));
  expect(await screen.findByText('Could not load connections.')).toBeInTheDocument();
  mode = 'empty';
  await user.click(region().getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText(/No READY connection is available/)).toBeInTheDocument();
  mode = 'ok';
  await user.click(region().getByRole('button', { name: 'Retry' }));
  expect(await screen.findByLabelText('Connection for Staging')).toBeInTheDocument();
});

it('ignores a late set result for an Environment the user left, but caches it for its own Environment', async () => {
  const bound = configuredTarget('lab', { connectionName: 'Lab', version: 2 });
  let release: (response: Response) => void = () => undefined;
  stub((_url, init) => init?.method === 'PUT' ? new Promise<Response>((resolve) => { release = resolve; }) : undefined);
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Save connection' }));
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  release(Response.json({ environment: envJson('staging', bound) }));
  expect(await screen.findByLabelText('Connection for Production')).toHaveValue('');
  await user.click(screen.getByRole('tab', { name: 'Staging' }));
  await waitFor(() => expect(screen.getByLabelText('Connection for Staging')).toHaveValue('lab'));
});
