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
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  expect(await screen.findByRole('status')).toHaveTextContent('2 bí mật đã được sao chép và xác minh');
  const put = vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PUT');
  expect(String(put?.[0])).toBe('/api/v1/applications/catalog/environments/staging/secret-store');
  expect(JSON.parse(String(put?.[1]?.body))).toEqual({ secretStoreKey: 'vault-b', expectedVersion: 5, expectedConfigVersion: 4 });
  expect(onConfig).toHaveBeenCalled();
  expect(screen.getByLabelText('Kho bí mật cho Staging')).toBeEnabled();
});

it('offers ordinary stores by name and key only: a stale legacy flag adds no label', async () => {
  stub((url) => { if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [{ ...stores[0], legacy: true }, stores[1]] }); });
  render(<Store target={configuredTarget('lab', { secretStoreKey: '', version: 5 })} />);
  const select = await screen.findByLabelText('Kho bí mật cho Staging');
  const labels = within(select).getAllByRole('option').map((option) => option.textContent);
  expect(labels).toContain('Vault A (vault-a)');
  expect(labels.join('|')).not.toMatch(/cũ|legacy/i);
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
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Kho cũ vẫn đang được chọn và không có gì thay đổi');
  expect(screen.getByLabelText('Kho bí mật cho Staging')).toHaveValue('vault-b');
  mode = 'stale';
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  await waitFor(() => expect(screen.getByLabelText('Kho bí mật cho Staging')).toHaveValue('vault-b'));
  expect(screen.getByRole('alert')).toHaveTextContent('Dữ liệu đã thay đổi kể từ lần tải trước');
});

it('hides secret-store details until choices load and offers retry when none exist', async () => {
  let ok = false;
  stub((url) => url.endsWith('/secret-store-choices') ? (ok ? Response.json({ secretStores: stores }) : Response.json({ secretStores: [] })) : undefined);
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab')} />);
  expect(await screen.findByText(/Chưa có kho bí mật Sẵn sàng/)).toBeInTheDocument();
  ok = true;
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByLabelText('Kho bí mật cho Staging')).toBeInTheDocument();
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
  await user.click(await screen.findByRole('button', { name: 'Xem trước chuyển đích' }));
  const impact = await screen.findByLabelText('Xem trước chuyển đích');
  expect(within(impact).getByText(/backend/)).toBeInTheDocument();
  expect(within(impact).getByLabelText('Ánh xạ cơ sở dữ liệu')).toHaveTextContent('postgres.default#shared.db → postgres.default#shared.db');
  expect(within(impact).getByRole('button', { name: 'Bắt đầu chuyển đích' })).toBeDisabled();
  await user.click(within(impact).getByLabelText(/ứng dụng sẽ bị dừng trong lúc dữ liệu được sao chép/));
  await user.click(within(impact).getByRole('button', { name: 'Bắt đầu chuyển đích' }));
  const post = vi.mocked(fetch).mock.calls.find(([url, init]) => String(url).endsWith('/connection-transitions') && init?.method === 'POST');
  expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({ destinationKey: 'lab2', mode: 'MIGRATE_POSTGRES', token: 'tok1', acknowledgeDowntime: true });
  expect(await screen.findByText('Đích đang chạy. Thế hệ nguồn được giữ lại ở trạng thái dừng (dữ liệu còn nguyên) cho đến khi bạn dọn dẹp.')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Dọn dẹp thế hệ nguồn' })).toBeInTheDocument();
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
  await user.click(await screen.findByRole('button', { name: 'Xem trước chuyển đích' }));
  await user.click(await screen.findByLabelText(/ứng dụng sẽ bị dừng/));
  await user.click(screen.getByRole('button', { name: 'Bắt đầu chuyển đích' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Bản xem trước đã cũ');
  expect(screen.queryByLabelText('Xem trước chuyển đích')).not.toBeInTheDocument();
});

it('resumes a running transition after a refresh and shows failure with compensation problems', async () => {
  stub((url) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.endsWith('/connection-transitions/t1')) return Response.json({ transition: detail({ status: 'FAILED', stage: 'RESTORING', failure: 'the database restore failed', compensation: ['stopped destination workload backend'], compensationFailed: ['source NOT restored: the destination could not be fully stopped'], stages: [{ stage: 'RESTORING', status: 'FAILED', message: 'the database restore failed', startedAt: '' }] }) });
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: [detail({})] });
  });
  render(<Panel target={configuredTarget('lab', { runtimeExists: true })} />);
  expect(await screen.findByLabelText('Tiến trình chuyển đích')).toHaveTextContent('Chuyển đích đang chạy');
  expect(await screen.findByText(/Chuyển đích không thành công và chưa hoàn tất/, { selector: '[role="alert"]' })).toBeInTheDocument();
  // Backend failure/compensation sentences are English summaries: only counts are shown.
  expect(screen.getByLabelText('Vấn đề khi khôi phục')).toHaveTextContent('Cần xử lý thủ công 1 bước');
  expect(screen.getByLabelText('Các bước khôi phục')).toHaveTextContent('Đã thực hiện 1 bước');
  expect(document.body).not.toHaveTextContent(/database restore failed|source NOT restored|stopped destination/);
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
  expect(screen.getByRole('heading', { name: 'Thao tác bị gián đoạn' })).toBeInTheDocument();
  const button = screen.getByRole('button', { name: 'Khôi phục môi trường' });
  expect(button).toBeDisabled();
  await user.click(screen.getByLabelText('Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn'));
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
  await user.click(screen.getByLabelText('Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn'));
  await user.click(screen.getByRole('button', { name: 'Khôi phục môi trường' }));
  expect(await screen.findByRole('alert')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Khôi phục môi trường' })).toBeEnabled();
  latest = configuredTarget('lab', { activeOperation: op('ACTIVE') });
  expect(await screen.findByRole('heading', { name: 'Thao tác đang chạy' })).toBeInTheDocument();
  latest = configuredTarget('lab');
  await waitFor(() => expect(screen.queryByRole('region', { name: 'Thao tác trên môi trường' })).not.toBeInTheDocument());
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
  expect(screen.getByRole('heading', { name: 'Thao tác đang chạy' })).toBeInTheDocument();
  done = true;
  await act(async () => { await vi.advanceTimersByTimeAsync(300); });
  await waitFor(() => expect(screen.queryByRole('region', { name: 'Thao tác trên môi trường' })).not.toBeInTheDocument());
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
  await user.click(await screen.findByRole('button', { name: 'Xem trước chuyển đích' }));
  const list = await screen.findByRole('list', { name: 'Tác động lên workload' });
  const row = (name: string) => within(list).getByText(name).closest('li') as HTMLElement;
  expect(row('backend')).toHaveTextContent('Triển khai lại, không đổi · nhận cấu hình mới hơn');
  expect(row('frontend')).toHaveTextContent('Triển khai lại kèm thay đổi nháp đang chờ');
  expect(row('extra')).toHaveTextContent('Thêm từ bản nháp đang chờ');
  expect(row('worker')).toHaveTextContent('Bị gỡ — không có ở đích (nguồn không bị đụng tới)');
  expect(row('worker')).not.toHaveTextContent('Triển khai lại');
  expect(row('worker')).toHaveAttribute('data-action', 'REMOVED');
  expect(screen.getByText(/Các thay đổi nháp đang chờ được đánh dấu ở trên/)).toBeInTheDocument();
  expect(screen.getByText(/Bản cấu hình mới nhất được áp dụng/)).toBeInTheDocument();
});

it('reports a source that could not be stopped and an interruption after the cutover truthfully', async () => {
  let current = detail({ status: 'SUCCEEDED', stage: 'SUCCEEDED', sourceState: 'NEEDS_ATTENTION', authority: 'DESTINATION', compensationFailed: ['quiesce source workload backend'], canCleanupSource: false });
  stub((url) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.endsWith('/connection-transitions/t1')) return Response.json({ transition: current });
    if (url.endsWith('/connection-transitions')) return Response.json({ transitions: [detail({ status: 'RUNNING' })] });
  });
  const { unmount } = render(<Panel target={configuredTarget('lab2', { runtimeExists: true })} />);
  expect(await screen.findByText(/workload nguồn không dừng được/)).toBeInTheDocument();
  expect(screen.queryByText(/giữ lại ở trạng thái dừng/)).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Dọn dẹp thế hệ nguồn' })).not.toBeInTheDocument();
  unmount();
  current = detail({ status: 'INTERRUPTED', stage: 'CUTOVER', authority: 'DESTINATION', failure: 'the process stopped' });
  render(<Panel target={configuredTarget('lab2', { runtimeExists: true })} />);
  expect(await screen.findByText(/đích đang chạy và là nguồn chuẩn/)).toBeInTheDocument();
  expect(screen.queryByText(/Thế hệ nguồn vẫn là nguồn chuẩn/)).not.toBeInTheDocument();
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

const pendingStore = { key: 'vault-p', name: 'Vault P', provider: 'VAULT_KV_V2', status: 'PENDING' };

it('suggests the only READY store as an unsaved draft and persists nothing before Save', async () => {
  const saved = configuredTarget('lab', { secretStoreKey: 'vault-a', version: 6 });
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [pendingStore, stores[0]] });
    if (init?.method === 'PUT') return Response.json({ environment: envJson(saved), changed: true, copiedSecrets: 0 });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab')} />);
  const select = await screen.findByLabelText('Kho bí mật cho Staging');
  expect(select).toHaveValue('vault-a');
  expect(screen.getByText(/Gợi ý chưa lưu/)).toHaveTextContent('không phải mặc định của tổ chức');
  expect(within(select).queryByRole('option', { name: /Vault P/ })).not.toBeInTheDocument();
  expect(vi.mocked(fetch).mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false);
  expect(screen.getByText('Chưa chọn')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Đã lưu kho bí mật');
  expect(JSON.parse(String(vi.mocked(fetch).mock.calls.find(([, init]) => init?.method === 'PUT')?.[1]?.body))).toMatchObject({ secretStoreKey: 'vault-a', expectedVersion: 2 });
});

it('does not suggest several stores and never retargets a bound store; same store is a no-op', async () => {
  stub((url) => url.endsWith('/secret-store-choices') ? Response.json({ secretStores: stores }) : undefined);
  const first = render(<Store target={configuredTarget('lab')} />);
  expect(await screen.findByLabelText('Kho bí mật cho Staging')).toHaveValue('');
  expect(screen.queryByText(/Gợi ý chưa lưu/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
  first.unmount();
  stub((url) => url.endsWith('/secret-store-choices') ? Response.json({ secretStores: [stores[1]] }) : undefined);
  render(<Store target={configuredTarget('lab', { secretStoreKey: 'vault-a', secretStoreName: 'Vault A' })} />);
  const select = await screen.findByLabelText('Kho bí mật cho Staging');
  expect(select).toHaveValue('vault-a');
  expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
});

it('keeps the user pick when the Environment target is refreshed', async () => {
  stub((url) => url.endsWith('/secret-store-choices') ? Response.json({ secretStores: stores }) : undefined);
  const user = userEvent.setup();
  const view = render(<Store target={configuredTarget('lab')} />);
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  view.rerender(<Store target={configuredTarget('lab', { version: 3 })} />);
  expect(screen.getByLabelText('Kho bí mật cho Staging')).toHaveValue('vault-b');
});

it('refreshes the configuration only for the scope that saved the store, but still reports the saved target', async () => {
  const saved = configuredTarget('lab', { secretStoreKey: 'vault-b', version: 6 });
  let release: (response: Response) => void = () => undefined;
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return new Promise<Response>((resolve) => { release = resolve; });
  });
  const onConfig = vi.fn();
  const onTarget = vi.fn();
  function Switch() {
    const [environment, setEnvironment] = useState<'staging' | 'production'>('staging');
    const application = { ...app(configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 })), environments: { staging: configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 }), production: configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 }) } };
    return <><button onClick={() => setEnvironment('production')}>Sang production</button><SecretStoreSelection application={application} environment={environment} configVersion={4} onConfigurationChanged={onConfig} onTargetChange={onTarget} /></>;
  }
  const user = userEvent.setup();
  render(<Switch />);
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  await user.click(screen.getByRole('button', { name: 'Sang production' }));
  await screen.findByLabelText('Kho bí mật cho Production');
  await act(async () => { release(Response.json({ environment: envJson(saved), changed: true, copiedSecrets: 1 })); });
  expect(onTarget).toHaveBeenCalledWith('catalog', 'staging', expect.objectContaining({ secretStoreKey: 'vault-b' }));
  expect(onConfig).not.toHaveBeenCalled();
  expect(screen.queryByText(/Đã lưu kho bí mật/)).not.toBeInTheDocument();
});

it('does not review a destination that is no longer an eligible READY choice', async () => {
  stub((url) => url.endsWith('/application-connections') ? Response.json({ connections: [{ key: 'lab3', name: 'Lab 3', kind: 'KUBERNETES', status: 'READY' }] }) : undefined);
  render(<Panel target={configuredTarget('lab', { runtimeExists: true })} />);
  expect(await screen.findByText('Kết nối đích đã chọn không còn Sẵn sàng. Hãy chọn lại.')).toBeInTheDocument();
  expect(screen.getByLabelText('Kết nối đích')).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Xem trước chuyển đích' })).toBeDisabled();
});

it('treats ENVIRONMENT_BUSY as busy, not stale: no reload, selection and Save stay', async () => {
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return Response.json({ error: 'busy', code: 'ENVIRONMENT_BUSY' }, { status: 409 });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 })} />);
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Môi trường đang bận');
  expect(alert).not.toHaveTextContent('Đã tải lại');
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).endsWith('/applications/catalog'))).toBe(false);
  expect(screen.getByLabelText('Kho bí mật cho Staging')).toHaveValue('vault-b');
  expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeEnabled();
});

it('claims a reload only after the GET succeeds: a failed stale reload keeps the pick, blocks Save and needs an explicit reload', async () => {
  let reloads = 0;
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return Response.json({ error: 'stale', code: 'STALE_VERSION' }, { status: 409 });
    if (url.endsWith('/applications/catalog')) { reloads += 1; return reloads === 1 ? Response.json({ error: 'down' }, { status: 503 }) : Response.json({ application: { environments: [envJson(configuredTarget('lab', { secretStoreKey: 'vault-a', version: 8 }))] } }); }
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget('lab', { secretStoreKey: 'vault-a', version: 5 })} />);
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  await user.click(screen.getByRole('button', { name: 'Lưu kho bí mật' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Chưa tải lại được lựa chọn mới nhất');
  expect(alert).not.toHaveTextContent('Đã tải lại lựa chọn mới nhất');
  expect(screen.getByLabelText('Kho bí mật cho Staging')).toHaveValue('vault-b');
  expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Tải lại trạng thái' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeEnabled());
  expect(screen.getByRole('status')).toHaveTextContent('Đã tải lại lựa chọn mới nhất');
  const puts = vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT');
  expect(puts).toHaveLength(1);
});
