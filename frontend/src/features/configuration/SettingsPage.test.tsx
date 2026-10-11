import { useState } from 'react';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { SettingsPage } from './SettingsPage';
import type { Application } from '../../shared/types/application';
import { configuredTarget } from '../../test/targets';

const application: Application = { id: 'catalog', name: 'Catalog', subdomain: 'catalog', environments: { staging: configuredTarget('internal-cluster', { secretStoreKey: 'vault-a', secretStoreName: 'Vault A' }), production: configuredTarget('internal-cluster') }, workloads: { staging: [], production: [] } };

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

it('shows separate environment tabs and never displays saved secret values', async () => {
  const calls: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    calls.push(`${init?.method ?? 'GET'} ${url}`);
    if (url.includes('/production/')) return Response.json({ applicationKey: 'catalog', environmentKey: 'production', version: 0, keys: [] });
    if (init?.method === 'PUT') return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, keys: [{ name: 'API_TOKEN', kind: 'SECRET', configured: true, usedBy: ['frontend'] }, { name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  expect(await screen.findByText('LOG_LEVEL')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: '+ Thêm bí mật' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'API_TOKEN');
  await user.type(screen.getByLabelText('Giá trị bí mật mới'), 'private-value');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
  expect(await screen.findByText('API_TOKEN')).toBeInTheDocument();
  expect(screen.queryByText('private-value')).not.toBeInTheDocument();
  expect(screen.getByText('Đã cấu hình')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByText('Chưa có bí mật nào trong production.')).toBeInTheDocument();
  expect(calls.some((call) => call.includes('/production/configuration'))).toBe(true);
});

it('lets ordinary variables be added without a store but asks for a store before a Secret', async () => {
  const unselected: Application = { ...application, environments: { staging: configuredTarget('internal-cluster'), production: configuredTarget('internal-cluster') } };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [{ key: 'vault-a', name: 'Vault A', provider: 'VAULT_KV_V2', status: 'READY' }] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 0, keys: [] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={unselected} />);
  await screen.findByText('Chưa có biến nào trong staging.');
  await user.click(screen.getByRole('button', { name: '+ Thêm biến' }));
  expect(screen.getByPlaceholderText('API_URL')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Hủy' }));
  await user.click(screen.getByRole('button', { name: '+ Thêm bí mật' }));
  expect(screen.queryByPlaceholderText('API_URL')).not.toBeInTheDocument();
  expect(screen.getByText(/Hãy chọn kho bí mật ở trên trước khi thêm Bí mật/)).toBeInTheDocument();
});

it('shows the server sentence and reloads on a 409 conflict', async () => {
  let puts = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method === 'PUT' && url.includes('/configuration/')) { puts += 1; return Response.json({ error: 'the environment is busy with another operation', code: 'ENVIRONMENT_BUSY' }, { status: 409 }); }
    if (url.endsWith('/secret-store-choices') || url.endsWith('/application-connections')) return Response.json({ secretStores: [], connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await user.click(await screen.findByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'MODE');
  await user.type(screen.getByLabelText('Giá trị'), 'blue');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('busy with another operation');
  expect(puts).toBe(1);
});

it('holds every configuration write while an operation owns the environment, keeps open editor text and re-enables after release', async () => {
  const writes: string[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (init?.method && init.method !== 'GET' && url.includes('/configuration')) writes.push(`${init.method} ${url}`);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const user = userEvent.setup();
  const view = render(<SettingsPage application={application} />);
  await screen.findByText('LOG_LEVEL');
  await user.click(screen.getByRole('button', { name: 'Sửa' }));
  await user.clear(screen.getByLabelText('Giá trị'));
  await user.type(screen.getByLabelText('Giá trị'), 'trace');
  // The operation starts after the editor was opened.
  view.rerender(<SettingsPage application={busyApp} />);
  expect(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeDisabled();
  for (const name of ['+ Thêm biến', '+ Thêm bí mật', 'Sửa', 'Đổi tên', 'Xóa']) expect(screen.getByRole('button', { name })).toBeDisabled();
  expect(screen.getByLabelText('Giá trị')).toHaveValue('trace');
  expect(writes).toEqual([]);
  view.rerender(<SettingsPage application={application} />);
  expect(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeEnabled();
  expect(screen.getByLabelText('Giá trị')).toHaveValue('trace');
  expect(screen.getByRole('button', { name: 'Đổi tên' })).toBeEnabled();
});

it('holds an open rename or delete confirmation while busy', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  const busyApp: Application = { ...application, environments: { ...application.environments, staging: { ...application.environments.staging, activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } } } };
  const user = userEvent.setup();
  const view = render(<SettingsPage application={application} />);
  await screen.findByText('LOG_LEVEL');
  await user.click(screen.getByRole('button', { name: 'Đổi tên' }));
  await user.clear(screen.getByLabelText('Tên key mới'));
  await user.type(screen.getByLabelText('Tên key mới'), 'LOGS');
  view.rerender(<SettingsPage application={busyApp} />);
  expect(screen.getByRole('button', { name: 'Tiếp tục' })).toBeDisabled();
  expect(screen.getByLabelText('Tên key mới')).toHaveValue('LOGS');
  view.rerender(<SettingsPage application={application} />);
  expect(screen.getByRole('button', { name: 'Tiếp tục' })).toBeEnabled();
});

it('notices an operation started elsewhere by polling an idle editor, holds submission and releases it, keeping the text', async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const op = { id: 'op7', kind: 'DEPLOY' as const, status: 'ACTIVE' as const, stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false };
  let remote = application.environments.staging;
  const env = (t: typeof remote) => ({ key: 'staging', version: t.version, configured: t.configured, connectionKey: t.connectionKey, executionProfile: t.profile, runtimeStatus: t.runtimeStatus, infrastructureScope: t.infrastructureScope, secretStoreKey: t.secretStoreKey, targetGeneration: t.targetGeneration, runtimeExists: t.runtimeExists, activeOperation: t.activeOperation });
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [env(remote)] } });
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: [] }] });
  }));
  function Host() {
    const [current, setCurrent] = useState(application);
    return <SettingsPage application={current} onTargetChange={(_id, key, target) => setCurrent((c) => ({ ...c, environments: { ...c.environments, [key]: target } }))} />;
  }
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  render(<Host />);
  await screen.findByText('LOG_LEVEL');
  await user.click(screen.getByRole('button', { name: 'Sửa' }));
  await user.clear(screen.getByLabelText('Giá trị'));
  await user.type(screen.getByLabelText('Giá trị'), 'trace');
  expect(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeEnabled();
  // Another user or process starts an operation: only the server knows.
  remote = { ...remote, activeOperation: op };
  await act(async () => { await vi.advanceTimersByTimeAsync(3500); });
  expect(await screen.findByRole('heading', { name: 'Thao tác đang chạy' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Đổi tên' })).toBeDisabled();
  expect(screen.getByLabelText('Giá trị')).toHaveValue('trace');
  // The operation ends.
  remote = { ...remote, activeOperation: undefined };
  await act(async () => { await vi.advanceTimersByTimeAsync(3500); });
  expect(screen.queryByRole('heading', { name: 'Thao tác đang chạy' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' })).toBeEnabled();
  expect(screen.getByLabelText('Giá trị')).toHaveValue('trace');
});

it('keeps an open editor and its text when the configuration is refreshed after a secret-store save', async () => {
  const unbound: Application = { ...application, environments: { staging: configuredTarget('internal-cluster'), production: configuredTarget('internal-cluster') } };
  let configGets = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [{ key: 'vault-a', name: 'Vault A', provider: 'VAULT_KV_V2', status: 'READY' }] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    if (url.endsWith('/secret-store') && init?.method === 'PUT') return Response.json({ environment: { key: 'staging', version: 3, configured: true, connectionKey: 'internal-cluster', secretStoreKey: 'vault-a', runtimeStatus: 'READY' }, changed: true, copiedSecrets: 0 });
    configGets += 1;
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: configGets, keys: [] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={unbound} />);
  await user.click(await screen.findByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByLabelText('Giá trị'), 'đang gõ dở');
  const before = configGets;
  await user.click(await screen.findByRole('button', { name: 'Lưu kho bí mật' }));
  await waitFor(() => expect(configGets).toBeGreaterThan(before));
  expect(screen.getByLabelText('Giá trị')).toHaveValue('đang gõ dở');
});

it('drops a late save reply that belongs to the environment the user left', async () => {
  let release: (response: Response) => void = () => undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    if (init?.method === 'PUT') return new Promise<Response>((resolve) => { release = resolve; });
    if (url.includes('/production/')) return Response.json({ applicationKey: 'catalog', environmentKey: 'production', version: 5, keys: [{ name: 'PROD_ONLY', kind: 'VARIABLE', value: '1', configured: true, usedBy: [] }] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await user.click(await screen.findByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'STAGING_KEY');
  await user.type(screen.getByLabelText('Giá trị'), 'x');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByText('PROD_ONLY')).toBeInTheDocument();
  await act(async () => { release(Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, keys: [{ name: 'STAGING_KEY', kind: 'VARIABLE', value: 'x', configured: true, usedBy: [] }] })); });
  expect(screen.getByText('PROD_ONLY')).toBeInTheDocument();
  expect(screen.queryByText('STAGING_KEY')).not.toBeInTheDocument();
  expect(screen.getByText('Phiên bản cấu hình v5')).toBeInTheDocument();
});

it('warns on rename and delete that references are not rewritten and names the key technical values unchanged', async () => {
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [{ name: 'LOG_LEVEL', kind: 'VARIABLE', value: 'debug', configured: true, usedBy: ['frontend'] }] });
  }));
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await screen.findByText('LOG_LEVEL');
  await user.click(screen.getByRole('button', { name: 'Đổi tên' }));
  expect(screen.getByRole('dialog', { name: 'Đổi tên LOG_LEVEL' })).toHaveTextContent('Đang được tham chiếu bởi frontend. Các tham chiếu sẽ không được cập nhật tự động');
  await user.click(screen.getByRole('button', { name: 'Hủy' }));
  await user.click(screen.getByRole('button', { name: 'Xóa' }));
  expect(screen.getByRole('dialog', { name: 'Xóa LOG_LEVEL' })).toHaveTextContent('không được cập nhật tự động');
  expect(screen.getByRole('tab', { name: 'Staging' })).toBeInTheDocument();
});

function deferredPut() {
  const state: { release: (response: Response) => void; configGets: number } = { release: () => undefined, configGets: 0 };
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const url = String(input);
    if (url.endsWith('/secret-store-choices')) return Response.json({ secretStores: [] });
    if (url.endsWith('/application-connections')) return Response.json({ connections: [] });
    if (url.includes('/connection-transitions')) return Response.json({ transitions: [] });
    if (init?.method === 'PUT') return new Promise<Response>((resolve) => { state.release = resolve; });
    state.configGets += 1;
    return Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 1, keys: [] });
  }));
  return state;
}
const saved = (name: string) => Response.json({ applicationKey: 'catalog', environmentKey: 'staging', version: 2, keys: [{ name, kind: 'VARIABLE', value: 'x', configured: true, usedBy: [] }] });

it('keeps a form the user opened while a save was pending, and still shows the saved data', async () => {
  const state = deferredPut();
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await user.click(await screen.findByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'FIRST');
  await user.type(screen.getByLabelText('Giá trị'), 'x');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
  await user.click(screen.getByRole('button', { name: '+ Thêm bí mật' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'SECOND');
  await user.type(screen.getByLabelText('Giá trị bí mật mới'), 'typing');
  await act(async () => { state.release(saved('FIRST')); });
  expect(await screen.findByText('FIRST')).toBeInTheDocument();
  expect(screen.getByPlaceholderText('API_URL')).toHaveValue('SECOND');
  expect(screen.getByLabelText('Giá trị bí mật mới')).toHaveValue('typing');
});

it('closes the form that was saved when the user changed nothing meanwhile', async () => {
  const state = deferredPut();
  const user = userEvent.setup();
  render(<SettingsPage application={application} />);
  await user.click(await screen.findByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'ONLY');
  await user.type(screen.getByLabelText('Giá trị'), 'x');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
  await act(async () => { state.release(saved('ONLY')); });
  expect(await screen.findByText('ONLY')).toBeInTheDocument();
  expect(screen.queryByPlaceholderText('API_URL')).not.toBeInTheDocument();
});

it('ignores a conflict reply after the page unmounted, without reloading the configuration', async () => {
  const state = deferredPut();
  const user = userEvent.setup();
  const view = render(<SettingsPage application={application} />);
  await user.click(await screen.findByRole('button', { name: '+ Thêm biến' }));
  await user.type(screen.getByPlaceholderText('API_URL'), 'GONE');
  await user.type(screen.getByLabelText('Giá trị'), 'x');
  await user.click(screen.getByRole('button', { name: 'Lưu thay đổi chờ triển khai' }));
  const before = state.configGets;
  view.unmount();
  await act(async () => { state.release(Response.json({ error: 'stale', code: 'STALE_VERSION' }, { status: 409 })); });
  expect(state.configGets).toBe(before);
});
