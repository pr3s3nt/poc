import { useState } from 'react';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SecretStoreSelection } from './SecretStoreSelection';
import { OperationBanner } from './OperationBanner';
import { emptyEnvironment, type Application, type EnvironmentTarget } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

const app = (staging: EnvironmentTarget): Application => ({ id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging, production: emptyEnvironment }, workloads: { staging: [], production: [] } });
const envJson = (t: EnvironmentTarget) => ({ key: 'staging', version: t.version, secretStoreKey: t.secretStoreKey, targetGeneration: t.targetGeneration, activeOperation: t.activeOperation });
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
  const saved = configuredTarget( { secretStoreKey: 'vault-b', version: 6 });
  const onConfig = vi.fn();
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return Response.json({ environment: envJson(saved), changed: true, copiedSecrets: 2 });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget( { secretStoreKey: 'vault-a', version: 5 })} onConfig={onConfig} />);
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
  render(<Store target={configuredTarget( { secretStoreKey: '', version: 5 })} />);
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
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson(configuredTarget( { secretStoreKey: 'vault-b', version: 9 }))] } });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget( { secretStoreKey: 'vault-a', version: 5 })} />);
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
  render(<Store target={configuredTarget()} />);
  expect(await screen.findByText(/Chưa có kho bí mật Sẵn sàng/)).toBeInTheDocument();
  ok = true;
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByLabelText('Kho bí mật cho Staging')).toBeInTheDocument();
});

it('polls an active operation and clears the banner when it ends', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  let done = false;
  const active = configuredTarget( { activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } });
  stub((url) => url.endsWith('/applications/catalog') ? Response.json({ application: { environments: [envJson(done ? configuredTarget() : active)] } }) : undefined);
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



it('learns about a started and a released operation by polling when the page was opened earlier', async () => {
  const { useEnvironmentBusy } = await import('./useEnvironmentBusy');
  let latest = configuredTarget();
  stub((url) => url.endsWith('/applications/catalog') ? Response.json({ application: { environments: [envJson(latest)] } }) : undefined);
  function Probe() {
    const busy = useEnvironmentBusy(app(configuredTarget()), 'staging', undefined, 20);
    return <span>{busy ? 'busy' : 'idle'}</span>;
  }
  render(<Probe />);
  expect(screen.getByText('idle')).toBeInTheDocument();
  latest = configuredTarget( { activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'x', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } });
  expect(await screen.findByText('busy')).toBeInTheDocument();
  latest = configuredTarget();
  expect(await screen.findByText('idle')).toBeInTheDocument();
});

const pendingStore = { key: 'vault-p', name: 'Vault P', provider: 'VAULT_KV_V2', status: 'PENDING' };

it('suggests the only READY store as an unsaved draft and persists nothing before Save', async () => {
  const saved = configuredTarget( { secretStoreKey: 'vault-a', version: 6 });
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [pendingStore, stores[0]] });
    if (init?.method === 'PUT') return Response.json({ environment: envJson(saved), changed: true, copiedSecrets: 0 });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget()} />);
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
  const first = render(<Store target={configuredTarget()} />);
  expect(await screen.findByLabelText('Kho bí mật cho Staging')).toHaveValue('');
  expect(screen.queryByText(/Gợi ý chưa lưu/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
  first.unmount();
  stub((url) => url.endsWith('/secret-store-choices') ? Response.json({ secretStores: [stores[1]] }) : undefined);
  render(<Store target={configuredTarget( { secretStoreKey: 'vault-a', secretStoreName: 'Vault A' })} />);
  const select = await screen.findByLabelText('Kho bí mật cho Staging');
  expect(select).toHaveValue('vault-a');
  expect(screen.getByRole('button', { name: 'Lưu kho bí mật' })).toBeDisabled();
});

it('keeps the user pick when the Environment target is refreshed', async () => {
  stub((url) => url.endsWith('/secret-store-choices') ? Response.json({ secretStores: stores }) : undefined);
  const user = userEvent.setup();
  const view = render(<Store target={configuredTarget()} />);
  await user.selectOptions(await screen.findByLabelText('Kho bí mật cho Staging'), 'vault-b');
  view.rerender(<Store target={configuredTarget( { version: 3 })} />);
  expect(screen.getByLabelText('Kho bí mật cho Staging')).toHaveValue('vault-b');
});

it('refreshes the configuration only for the scope that saved the store, but still reports the saved target', async () => {
  const saved = configuredTarget( { secretStoreKey: 'vault-b', version: 6 });
  let release: (response: Response) => void = () => undefined;
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return new Promise<Response>((resolve) => { release = resolve; });
  });
  const onConfig = vi.fn();
  const onTarget = vi.fn();
  function Switch() {
    const [environment, setEnvironment] = useState<'staging' | 'production'>('staging');
    const application = { ...app(configuredTarget( { secretStoreKey: 'vault-a', version: 5 })), environments: { staging: configuredTarget( { secretStoreKey: 'vault-a', version: 5 }), production: configuredTarget( { secretStoreKey: 'vault-a', version: 5 }) } };
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


it('treats ENVIRONMENT_BUSY as busy, not stale: no reload, selection and Save stay', async () => {
  stub((url, init) => {
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: stores });
    if (init?.method === 'PUT') return Response.json({ error: 'busy', code: 'ENVIRONMENT_BUSY' }, { status: 409 });
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget( { secretStoreKey: 'vault-a', version: 5 })} />);
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
    if (url.endsWith('/applications/catalog')) { reloads += 1; return reloads === 1 ? Response.json({ error: 'down' }, { status: 503 }) : Response.json({ application: { environments: [envJson(configuredTarget( { secretStoreKey: 'vault-a', version: 8 }))] } }); }
  });
  const user = userEvent.setup();
  render(<Store target={configuredTarget( { secretStoreKey: 'vault-a', version: 5 })} />);
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
