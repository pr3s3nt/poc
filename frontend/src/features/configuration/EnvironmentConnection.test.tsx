import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';
import { unconfiguredTarget, type Application, type EnvironmentKey, type EnvironmentTarget } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';

const lab = { key: 'lab', name: 'Lab', kind: 'KUBERNETES', status: 'READY' };
const other = { key: 'other', name: 'Other', kind: 'KUBERNETES', status: 'READY' };
const cloud = { key: 'cloud', name: 'Cloud', kind: 'AWS', status: 'READY' };
const base: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging: unconfiguredTarget, production: unconfiguredTarget }, workloads: { staging: [], production: [] } };

afterEach(() => vi.unstubAllGlobals());

function envJson(key: string, target: EnvironmentTarget) {
  return { key, version: target.version, configured: target.configured, connectionKey: target.connectionKey, connectionName: target.connectionName, connectionKind: target.connectionKind, executionProfile: target.profile, region: target.region, runtimeStatus: target.runtimeStatus, infrastructureScope: target.infrastructureScope };
}

// Settings with the same state ownership as App: the page reports a changed target.
function Host({ initial = base, onChange }: { initial?: Application; onChange?(env: EnvironmentKey, target: EnvironmentTarget): void }) {
  const [application, setApplication] = useState(initial);
  return <SettingsPage application={application} onTargetChange={(_id, env, target) => { onChange?.(env, target); setApplication((current) => ({ ...current, environments: { ...current.environments, [env]: target } })); }} />;
}

type Calls = string[];
function stub(handler: (url: string, init?: RequestInit) => Response | Promise<Response> | undefined, calls: Calls = []) {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    calls.push(`${init?.method ?? 'GET'} ${url}`);
    const custom = await handler(url, init);
    if (custom) return custom;
    if (url.endsWith('/application-connections')) return Response.json({ connections: [lab, other, cloud], defaultConnectionKey: 'other' });
    if (url.includes('/configuration')) return Response.json({ applicationKey: 'catalog', environmentKey: url.includes('/production/') ? 'production' : 'staging', version: 0, keys: [] });
    return Response.json({});
  }));
  return calls;
}

it('lists choices without preselecting the Organization default and sets the Environment once without a dialog', async () => {
  const bound = configuredTarget('lab', { connectionName: 'Lab' });
  const calls = stub((_url, init) => {
    if (init?.method === 'PUT') return Response.json({ environment: envJson('staging', bound) });
  });
  const user = userEvent.setup();
  render(<Host />);
  const select = await screen.findByLabelText('Connection for Staging');
  expect(select).toHaveValue('');
  expect(screen.getByRole('option', { name: /Other \(other\).*default/ })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Set connection' })).toBeDisabled();
  expect(screen.getByText(/You can set this once. It cannot be changed afterwards/)).toBeInTheDocument();
  await user.selectOptions(select, 'lab');
  // Choosing persists nothing.
  expect(calls.some((call) => call.startsWith('PUT'))).toBe(false);
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  expect(await screen.findByText('Locked')).toBeInTheDocument();
  const put = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PUT');
  expect(String(put?.[0])).toBe('/api/v1/applications/catalog/environments/staging/connection');
  expect(JSON.parse(String(put?.[1]?.body))).toEqual({ connectionKey: 'lab', expectedVersion: 1 });
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  // Read-only: no select or button, only the facts.
  expect(screen.queryByLabelText('Connection for Staging')).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Set connection' })).not.toBeInTheDocument();
  expect(screen.getByText('internal-k8s')).toBeInTheDocument();
  expect(screen.getByText('Kubernetes')).toBeInTheDocument();
});

it('keeps the other Environment unset and independent, including a different kind and region', async () => {
  const aws = configuredTarget('cloud', { connectionName: 'Cloud', connectionKind: 'AWS', profile: 'aws-eks', region: 'us-east-1', runtimeStatus: 'PENDING' });
  stub((_url, init) => {
    if (init?.method === 'PUT') return Response.json({ environment: envJson('production', aws) });
  });
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { connectionName: 'Lab' }), production: unconfiguredTarget } }} />);
  expect(await screen.findByText('Locked')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  await user.selectOptions(await screen.findByLabelText('Connection for Production'), 'cloud');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  expect(await screen.findByText('AWS')).toBeInTheDocument();
  expect(screen.getByText('aws-eks')).toBeInTheDocument();
  expect(screen.getByText('us-east-1')).toBeInTheDocument();
  expect(screen.getByText('PENDING')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Staging' }));
  expect(await screen.findByText('Lab')).toBeInTheDocument();
  expect(screen.queryByText('us-east-1')).not.toBeInTheDocument();
});

it('shows a configured Environment read-only without loading choices and explains migrated infrastructure', async () => {
  const calls = stub(() => undefined);
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { infrastructureScope: 'LEGACY_APPLICATION' }), production: configuredTarget('lab') } }} />);
  expect(await screen.findByText('Locked')).toBeInTheDocument();
  expect(screen.getByText('Application-scoped (migrated binding)')).toBeInTheDocument();
  expect(calls.some((call) => call.endsWith('/application-connections'))).toBe(false);
});

it('on an already-configured 409 shows the message and reloads the authoritative Environment', async () => {
  const winner = configuredTarget('other', { connectionName: 'Other', version: 5 });
  let applicationReads = 0;
  stub((url, init) => {
    if (init?.method === 'PUT') return Response.json({ error: 'this environment already has a connection; it cannot be changed', code: 'ALREADY_CONFIGURED' }, { status: 409 });
    if (url.endsWith('/applications/catalog')) { applicationReads += 1; return Response.json({ application: { key: 'catalog', environments: [envJson('staging', winner), envJson('production', unconfiguredTarget)] } }); }
  });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  expect(await screen.findByText('Locked')).toBeInTheDocument();
  expect(applicationReads).toBe(1);
  expect(screen.getByText('Other')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Set connection' })).not.toBeInTheDocument();
});

it('on a stale-version 409 reloads the Environment and lets the user choose again with the new version', async () => {
  const fresh = { ...unconfiguredTarget, version: 4 };
  let puts = 0;
  const bodies: string[] = [];
  stub((url, init) => {
    if (init?.method === 'PUT') {
      bodies.push(String(init.body));
      puts += 1;
      if (puts === 1) return Response.json({ error: 'the environment changed since it was loaded; reload and try again', code: 'STALE_VERSION' }, { status: 409 });
      return Response.json({ environment: envJson('staging', configuredTarget('lab', { version: 5 })) });
    }
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { key: 'catalog', environments: [envJson('staging', fresh), envJson('production', fresh)] } });
  });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('changed since it was loaded');
  // Still unset, selection kept, and the retry carries the reloaded version.
  await waitFor(() => expect(screen.getByLabelText('Connection for Staging')).toHaveValue('lab'));
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  expect(await screen.findByText('Locked')).toBeInTheDocument();
  expect(JSON.parse(bodies[0] ?? '{}').expectedVersion).toBe(1);
  expect(JSON.parse(bodies[1] ?? '{}').expectedVersion).toBe(4);
});

it('treats an unavailable connection as a safe 422 and reloads the choices', async () => {
  let listings = 0;
  stub((url, init) => {
    if (init?.method === 'PUT') return Response.json({ error: 'application: the selected connection is not available', field: 'connectionKey' }, { status: 422 });
    if (url.endsWith('/application-connections')) { listings += 1; return Response.json({ connections: listings === 1 ? [lab, other] : [other], defaultConnectionKey: '' }); }
  });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('no longer available');
  await waitFor(() => expect(listings).toBe(2));
  await waitFor(() => expect(screen.getByLabelText('Connection for Staging')).toHaveValue(''));
  expect(screen.queryByRole('option', { name: /Lab/ })).not.toBeInTheDocument();
});

it('handles an empty list and a failed list with retry', async () => {
  let call = 0;
  stub((url) => {
    if (url.endsWith('/application-connections')) { call += 1; return call === 1 ? Response.json({ error: 'boom' }, { status: 500 }) : call === 2 ? Response.json({ connections: [], defaultConnectionKey: '' }) : Response.json({ connections: [lab], defaultConnectionKey: '' }); }
  });
  const user = userEvent.setup();
  render(<Host />);
  expect(await screen.findByText('Could not load connections.')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText(/No READY connection is available/)).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Set connection' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByLabelText('Connection for Staging')).toBeInTheDocument();
});

it('ignores a late choices reply and keeps a late set result bound to its own Environment after a tab switch', async () => {
  let releaseChoices: (response: Response) => void = () => undefined;
  const lateChoices = new Promise<Response>((resolve) => { releaseChoices = resolve; });
  let releasePut: (response: Response) => void = () => undefined;
  const latePut = new Promise<Response>((resolve) => { releasePut = resolve; });
  let listing = 0;
  const changes: string[] = [];
  stub((url, init) => {
    if (init?.method === 'PUT') return latePut;
    if (url.endsWith('/application-connections')) { listing += 1; return listing === 1 ? Response.json({ connections: [lab], defaultConnectionKey: '' }) : lateChoices; }
  });
  const user = userEvent.setup();
  render(<Host onChange={(env, target) => changes.push(`${env}:${target.connectionKey}`)} />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  // While the staging set is in flight the user opens Production, whose choices are still loading.
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByText('Loading connections…')).toBeInTheDocument();
  releasePut(Response.json({ environment: envJson('staging', configuredTarget('lab', { connectionName: 'Lab' })) }));
  await waitFor(() => expect(changes).toEqual(['staging:lab']));
  // Production is untouched: still loading, not locked, and its late list only fills production.
  expect(screen.queryByText('Locked')).not.toBeInTheDocument();
  releaseChoices(Response.json({ connections: [other], defaultConnectionKey: '' }));
  const production = await screen.findByLabelText('Connection for Production');
  expect(production).toHaveValue('');
  expect(screen.getByRole('option', { name: /Other/ })).toBeInTheDocument();
  expect(screen.queryByRole('option', { name: /Lab/ })).not.toBeInTheDocument();
});

it('applies a set confirmed after the view unmounted to its own Environment and ignores a late error', async () => {
  let releasePut: (response: Response) => void = () => undefined;
  const latePut = new Promise<Response>((resolve) => { releasePut = resolve; });
  const changes: string[] = [];
  stub((_url, init) => { if (init?.method === 'PUT') return latePut; });
  const user = userEvent.setup();
  const { unmount } = render(<Host onChange={(env, target) => changes.push(`${env}:${target.connectionKey}`)} />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  unmount();
  releasePut(Response.json({ environment: envJson('staging', configuredTarget('lab', { connectionName: 'Lab' })) }));
  await waitFor(() => expect(changes).toEqual(['staging:lab']));
});

it('does not show a late set error under the Environment the user moved to', async () => {
  let failPut: (response: Response) => void = () => undefined;
  const latePut = new Promise<Response>((resolve) => { failPut = resolve; });
  stub((_url, init) => { if (init?.method === 'PUT') return latePut; });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Connection for Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Set connection' }));
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByLabelText('Connection for Production')).toHaveValue('');
  failPut(Response.json({ error: 'boom' }, { status: 500 }));
  await new Promise((resolve) => setTimeout(resolve, 30));
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  expect(screen.getByLabelText('Connection for Production')).toHaveValue('');
});

it('ignores a stale choices reply after Retry started a newer load', async () => {
  let releaseFirst: (response: Response) => void = () => undefined;
  const first = new Promise<Response>((resolve) => { releaseFirst = resolve; });
  let call = 0;
  stub((url) => {
    if (!url.endsWith('/application-connections')) return undefined;
    call += 1;
    if (call === 1) return Response.json({ error: 'boom' }, { status: 500 });
    if (call === 2) return first;
    return Response.json({ connections: [other], defaultConnectionKey: '' });
  });
  const user = userEvent.setup();
  render(<Host />);
  await screen.findByText('Could not load connections.');
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  // The slow load is superseded by a newer one (a second Retry after the first error is not possible,
  // so the Environment is re-entered: tab switch away and back starts load 3).
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByRole('option', { name: /Other/ })).toBeInTheDocument();
  releaseFirst(Response.json({ connections: [lab], defaultConnectionKey: '' }));
  await new Promise((resolve) => setTimeout(resolve, 30));
  expect(screen.queryByRole('option', { name: /Lab/ })).not.toBeInTheDocument();
});
