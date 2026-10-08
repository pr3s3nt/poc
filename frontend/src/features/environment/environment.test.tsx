import { useState } from 'react';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SecretStoreSelection } from './SecretStoreSelection';
import { TransitionPanel } from './TransitionPanel';
import { OperationBanner } from './OperationBanner';
import { unconfiguredTarget, type Application, type EnvironmentTarget } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

const app = (staging: EnvironmentTarget): Application => ({ id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging, production: unconfiguredTarget }, workloads: { staging: [], production: [] } });
const envJson = (t: EnvironmentTarget) => ({ key: 'staging', version: t.version, configured: t.configured, connectionKey: t.connectionKey, executionProfile: t.profile, runtimeStatus: t.runtimeStatus, infrastructureScope: t.infrastructureScope, secretStoreKey: t.secretStoreKey, targetGeneration: t.targetGeneration, runtimeExists: t.runtimeExists, activeOperation: t.activeOperation });
const stores = [{ key: 'vault-a', name: 'Vault A', provider: 'VAULT_KV_V2', status: 'READY' }, { key: 'vault-b', name: 'Vault B', provider: 'VAULT_KV_V2', status: 'READY' }];

type Handler = (url: string, init?: RequestInit) => Response | Promise<Response> | undefined;
function stub(handler: Handler) {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    return (await handler(url, init)) ?? Response.json({});
  }));
}
function Store({ target, onConfig = () => undefined }: { target: EnvironmentTarget; onConfig?(): void }) {
  const [application, setApplication] = useState(app(target));
  return <SecretStoreSelection application={application} environment="staging" configVersion={4} onConfigurationChanged={onConfig} onTargetChange={(_id, env, next) => setApplication((c) => ({ ...c, environments: { ...c.environments, [env]: next } }))} />;
}

it('selects a store with both versions, reports the copy and keeps the selector editable', async () => {
  const saved = configuredTarget('lab', { secretStoreKey: 'vault-b', version: 6 });
  const onConfig = vi.fn();
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return Response.json({ environment: envJson(saved), changed: true, copiedSecrets: 2 });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 })} onConfig={onConfig} />);
  await user.selectOptions(await screen.findByLabelText('Secret store for Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Save secret store' }));
  expect(await screen.findByRole('status')).toHaveTextContent('2 secret(s) were copied and verified');
  const put = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PUT');
  expect(String(put?.[0])).toBe('/api/v1/applications/catalog/environments/staging/secret-store');
  expect(JSON.parse(String(put?.[1]?.body))).toEqual({ secretStoreKey: 'vault-b', expectedVersion: 5, expectedConfigVersion: 4 });
  expect(onConfig).toHaveBeenCalled();
  expect(screen.getByLabelText('Secret store for Staging')).toBeEnabled();
});

it('shows a copy failure without changing the selection and a stale 409 with the reloaded selection', async () => {
  let mode: 'copy' | 'stale' = 'copy';
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return mode === 'copy' ? Response.json({ error: 'copy failed', code: 'SECRET_COPY_FAILED' }, { status: 502 }) : Response.json({ error: 'the environment changed since it was loaded; reload and try again', code: 'STALE_VERSION' }, { status: 409 });
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson(configuredTarget('lab', { secretStoreKey: 'vault-b', version: 9 }))] } });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 })} />);
  await user.selectOptions(await screen.findByLabelText('Secret store for Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Save secret store' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('previous store is still selected and nothing changed');
  expect(screen.getByLabelText('Secret store for Staging')).toHaveValue('vault-b');
  mode = 'stale';
  await user.click(screen.getByRole('button', { name: 'Save secret store' }));
  await waitFor(() => expect(screen.getByLabelText('Secret store for Staging')).toHaveValue('vault-b'));
  expect(screen.getByRole('alert')).toHaveTextContent('changed since it was loaded');
});

it('hides secret-store details until choices load and offers retry when none exist', async () => {
  let ok = false;
  stub((url) => url.endsWith('/secret-store-choices') ? (ok ? Response.json({ secretStores: stores }) : Response.json({ secretStores: [] })) : undefined);
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab')} />);
  expect(await screen.findByText(/No READY secret store is available/)).toBeInTheDocument();
  ok = true;
  await user.click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByLabelText('Secret store for Staging')).toBeInTheDocument();
});

const preview = { token: 'tok1', mode: 'MIGRATE_POSTGRES', source: { connectionKey: 'lab', connectionName: 'Lab', executionProfile: 'internal-k8s', generation: 0, namespace: 'ns' }, destination: { connectionKey: 'lab2', connectionName: 'Lab 2', executionProfile: 'internal-k8s', generation: 1, namespace: 'ns-g1-abcd1234' }, workloads: [{ workloadId: 'backend', action: 'UNCHANGED' }], mappings: [{ sourceDescriptor: 'postgres.default#shared.db', destinationDescriptor: 'postgres.default#shared.db' }], downtimeRequired: true, notes: ['Application writers stop before the backup.'] };
const detail = (over: object) => ({ id: 't1', operationId: 'op1', mode: 'MIGRATE_POSTGRES', stage: 'BACKUP', status: 'RUNNING', source: preview.source, destination: preview.destination, sourceState: 'AUTHORITATIVE', stages: [{ stage: 'PREFLIGHT', status: 'SUCCEEDED', startedAt: '' }], canCleanupSource: false, createdAt: '', updatedAt: '', ...over });

function Panel({ target }: { target: EnvironmentTarget }) {
  const [application, setApplication] = useState(app(target));
  return <TransitionPanel application={application} environment="staging" initialDestination="lab2" pollMs={20} onTargetChange={(_id, env, next) => setApplication((c) => ({ ...c, environments: { ...c.environments, [env]: next } }))} />;
}

it('previews, requires the downtime acknowledgement, executes with the exact token and follows persisted progress', async () => {
  let polls = 0;
  stub((url, init) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [{ key: 'lab', name: 'Lab', kind: 'KUBERNETES', status: 'READY' }, { key: 'lab2', name: 'Lab 2', kind: 'KUBERNETES', status: 'READY' }] });
    if (url.endsWith('/connection-transition/preview')) return Response.json({ preview });
    if (url.endsWith('/connection-transitions') && init?.method === 'POST') return Response.json({ transition: detail({}) }, { status: 202 });
    const finished = detail({ status: 'SUCCEEDED', stage: 'SUCCEEDED', sourceState: 'RETAINED_QUIESCED', stages: [{ stage: 'PREFLIGHT', status: 'SUCCEEDED', startedAt: '' }, { stage: 'CUTOVER', status: 'SUCCEEDED', startedAt: '' }], canCleanupSource: true });
    if (url.endsWith('/connection-transitions/t1')) { polls += 1; return Response.json({ transition: polls < 2 ? detail({ stage: 'RESTORING' }) : finished }); }
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: polls >= 2 ? [finished] : [] });
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson(configuredTarget('lab2', { targetGeneration: 1, version: 8 }))] } });
  });
  const user = userEvent.setup();
  render(<Panel target={configuredTarget('lab', { runtimeExists: true })} />);
  await user.click(await screen.findByRole('button', { name: 'Preview transition' }));
  const impact = await screen.findByLabelText('Transition preview');
  expect(within(impact).getByText(/backend/)).toBeInTheDocument();
  expect(within(impact).getByLabelText('Database mapping')).toHaveTextContent('postgres.default#shared.db → postgres.default#shared.db');
  expect(within(impact).getByRole('button', { name: 'Start transition' })).toBeDisabled();
  await user.click(within(impact).getByLabelText(/application is stopped while its data is copied/));
  await user.click(within(impact).getByRole('button', { name: 'Start transition' }));
  const post = vi.mocked(fetch).mock.calls.find(([url, init]) => String(url).endsWith('/connection-transitions') && init?.method === 'POST');
  expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({ destinationKey: 'lab2', mode: 'MIGRATE_POSTGRES', token: 'tok1', acknowledgeDowntime: true });
  expect(await screen.findByText('The destination is live. The source generation is retained and quiesced (data kept) until you clean it up.')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Clean up source generation' })).toBeInTheDocument();
});

it('rejects a stale preview with the server message and requires a new preview', async () => {
  stub((url, init) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [{ key: 'lab2', name: 'Lab 2', kind: 'KUBERNETES', status: 'READY' }] });
    if (url.endsWith('/connection-transition/preview')) return Response.json({ preview });
    if (url.endsWith('/connection-transitions') && init?.method === 'POST') return Response.json({ error: 'transition: the preview is stale; preview the transition again', code: 'STALE_PREVIEW' }, { status: 409 });
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: [] });
  });
  const user = userEvent.setup();
  render(<Panel target={configuredTarget('lab', { runtimeExists: true })} />);
  await user.click(await screen.findByRole('button', { name: 'Preview transition' }));
  await user.click(await screen.findByLabelText(/application is stopped/));
  await user.click(screen.getByRole('button', { name: 'Start transition' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('preview is stale');
  expect(screen.queryByLabelText('Transition preview')).not.toBeInTheDocument();
});

it('resumes a running transition after a refresh and shows failure with compensation problems', async () => {
  stub((url) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.endsWith('/connection-transitions/t1')) return Response.json({ transition: detail({ status: 'FAILED', stage: 'RESTORING', failure: 'the database restore failed', compensation: ['stopped destination workload backend'], compensationFailed: ['source NOT restored: the destination could not be fully stopped'], stages: [{ stage: 'RESTORING', status: 'FAILED', message: 'the database restore failed', startedAt: '' }] }) });
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: [detail({})] });
  });
  render(<Panel target={configuredTarget('lab', { runtimeExists: true })} />);
  expect(await screen.findByLabelText('Transition progress')).toHaveTextContent('Transition in progress');
  expect(await screen.findByText(/the database restore failed/, { selector: '[role="alert"]' })).toBeInTheDocument();
  expect(screen.getByLabelText('Recovery problems')).toHaveTextContent('source NOT restored');
  expect(screen.getByLabelText('Recovery steps')).toHaveTextContent('stopped destination workload backend');
});

it('offers explicit recovery only after the stop confirmation and reports an incomplete recovery', async () => {
  const interrupted = configuredTarget('lab', { activeOperation: { id: 'op9', kind: 'TRANSITION', status: 'INTERRUPTED', stage: 'DEPLOYING', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: true } });
  stub((url, init) => {
    if (url.endsWith('/operations/op9/recover') && init?.method === 'POST') return Response.json({ error: 'recovery did not complete', code: 'RECOVERY_INCOMPLETE', recovery: { operationId: 'op9', kind: 'TRANSITION', outcome: 'recovery did not complete: x', compensationFailed: ['stop destination workload web'] } }, { status: 409 });
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson(interrupted)] } });
  });
  const user = userEvent.setup();
  function Host() {
    const [application, setApplication] = useState(app(interrupted));
    return <OperationBanner application={application} environment="staging" pollMs={50} onTargetChange={(_i, env, t) => setApplication((c) => ({ ...c, environments: { ...c.environments, [env]: t } }))} />;
  }
  render(<Host />);
  expect(screen.getByRole('heading', { name: 'Operation interrupted' })).toBeInTheDocument();
  const button = screen.getByRole('button', { name: 'Recover environment' });
  expect(button).toBeDisabled();
  await user.click(screen.getByLabelText('I confirm the interrupted operation has stopped'));
  await user.click(button);
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  const post = vi.mocked(fetch).mock.calls.find(([url]) => String(url).endsWith('/recover'));
  expect(JSON.parse(String(post?.[1]?.body))).toEqual({ priorExecutionStopped: true });
});

it('keeps polling after a failed recovery so later status changes of the same operation appear', async () => {
  const op = (status: 'INTERRUPTED' | 'ACTIVE') => ({ id: 'op9', kind: 'TRANSITION' as const, status, stage: 'DEPLOYING', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: status === 'INTERRUPTED' });
  const interrupted = configuredTarget('lab', { activeOperation: op('INTERRUPTED') });
  let latest = interrupted;
  stub((url, init) => {
    if (url.endsWith('/operations/op9/recover') && init?.method === 'POST') return Response.json({ error: 'recovery did not complete', code: 'RECOVERY_INCOMPLETE' }, { status: 409 });
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson(latest)] } });
  });
  const user = userEvent.setup();
  function Host() {
    const [application, setApplication] = useState(app(interrupted));
    return <OperationBanner application={application} environment="staging" pollMs={30} onTargetChange={(_i, env, t) => setApplication((c) => ({ ...c, environments: { ...c.environments, [env]: t } }))} />;
  }
  render(<Host />);
  await user.click(screen.getByLabelText('I confirm the interrupted operation has stopped'));
  await user.click(screen.getByRole('button', { name: 'Recover environment' }));
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Recover environment' })).toBeEnabled();
  latest = configuredTarget('lab', { activeOperation: op('ACTIVE') });
  expect(await screen.findByRole('heading', { name: 'Operation in progress' })).toBeInTheDocument();
  latest = configuredTarget('lab');
  await waitFor(() => expect(screen.queryByRole('region', { name: 'Environment operation' })).not.toBeInTheDocument());
});

it('polls an active operation and clears the banner when it ends', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  let done = false;
  const active = configuredTarget('lab', { activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } });
  stub((url) => url.endsWith('/applications/catalog') ? Response.json({ application: { environments: [envJson(done ? configuredTarget('lab') : active)] } }) : undefined);
  function Host() {
    const [application, setApplication] = useState(app(active));
    return <OperationBanner application={application} environment="staging" pollMs={100} onTargetChange={(_i, env, t) => setApplication((c) => ({ ...c, environments: { ...c.environments, [env]: t } }))} />;
  }
  render(<Host />);
  expect(screen.getByRole('heading', { name: 'Operation in progress' })).toBeInTheDocument();
  done = true;
  await act(async () => { await vi.advanceTimersByTimeAsync(300); });
  await waitFor(() => expect(screen.queryByRole('region', { name: 'Environment operation' })).not.toBeInTheDocument());
});

it('renders the reviewed draft and configuration impact and keeps removed workloads apart from redeployed ones', async () => {
  const reviewed = { ...preview, workloads: [
    { workloadId: 'backend', action: 'UNCHANGED', configChanged: true },
    { workloadId: 'frontend', action: 'UPDATED' },
    { workloadId: 'extra', action: 'ADDED' },
    { workloadId: 'worker', action: 'REMOVED' },
  ] };
  stub((url) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [{ key: 'lab2', name: 'Lab 2', kind: 'KUBERNETES', status: 'READY' }] });
    if (url.endsWith('/connection-transition/preview')) return Response.json({ preview: reviewed });
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: [] });
  });
  const user = userEvent.setup();
  render(<Panel target={configuredTarget('lab', { runtimeExists: true })} />);
  await user.click(await screen.findByRole('button', { name: 'Preview transition' }));
  const list = await screen.findByRole('list', { name: 'Workload impact' });
  const row = (name: string) => within(list).getByText(name).closest('li') as HTMLElement;
  expect(row('backend')).toHaveTextContent('Redeployed unchanged · picks up newer configuration');
  expect(row('frontend')).toHaveTextContent('Redeployed with a pending draft change');
  expect(row('extra')).toHaveTextContent('Added from a pending draft');
  expect(row('worker')).toHaveTextContent('Removed — absent at the destination (the source is not touched)');
  expect(row('worker')).not.toHaveTextContent('Redeployed');
  expect(row('worker')).toHaveAttribute('data-action', 'REMOVED');
  expect(screen.getByText(/Pending draft changes marked above are applied by this transition/)).toBeInTheDocument();
  expect(screen.getByText(/newest configuration revision is applied/)).toBeInTheDocument();
});

it('reports a source that could not be stopped and an interruption after the cutover truthfully', async () => {
  let current = detail({ status: 'SUCCEEDED', stage: 'SUCCEEDED', sourceState: 'NEEDS_ATTENTION', authority: 'DESTINATION', compensationFailed: ['quiesce source workload backend'], canCleanupSource: false });
  stub((url) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.endsWith('/connection-transitions/t1')) return Response.json({ transition: current });
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: [detail({ status: 'RUNNING' })] });
  });
  const { unmount } = render(<Panel target={configuredTarget('lab2', { runtimeExists: true })} />);
  expect(await screen.findByText(/source workloads could not be stopped/)).toBeInTheDocument();
  expect(screen.queryByText(/retained and quiesced/)).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Clean up source generation' })).not.toBeInTheDocument();
  unmount();
  current = detail({ status: 'INTERRUPTED', stage: 'CUTOVER', authority: 'DESTINATION', failure: 'the process stopped' });
  render(<Panel target={configuredTarget('lab2', { runtimeExists: true })} />);
  expect(await screen.findByText(/destination is live and authoritative/)).toBeInTheDocument();
  expect(screen.queryByText(/source generation remains authoritative/)).not.toBeInTheDocument();
});

it('learns about a started and a released operation by polling when the page was opened earlier', async () => {
  const { useEnvironmentBusy } = await import('./useEnvironmentBusy');
  let latest = configuredTarget('lab');
  stub((url) => url.endsWith('/applications/catalog') ? Response.json({ application: { environments: [envJson(latest)] } }) : undefined);
  function Probe() {
    const busy = useEnvironmentBusy(app(configuredTarget('lab')), 'staging', undefined, 20);
    return <span>{busy ? 'busy' : 'idle'}</span>;
  }
  render(<Probe />);
  expect(screen.getByText('idle')).toBeInTheDocument();
  latest = configuredTarget('lab', { activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'x', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } });
  expect(await screen.findByText('busy')).toBeInTheDocument();
  latest = configuredTarget('lab');
  expect(await screen.findByText('idle')).toBeInTheDocument();
});
