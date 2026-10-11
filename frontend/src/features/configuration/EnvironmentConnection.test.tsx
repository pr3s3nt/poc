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
  const select = await screen.findByLabelText('Kết nối cho Staging');
  expect(select).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
  await user.selectOptions(select, 'lab');
  expect(calls.some((call) => call.startsWith('PUT'))).toBe(false);
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByText('Thế hệ 0')).toBeInTheDocument();
  expect(String(putOf()?.[0])).toBe('/api/v1/applications/catalog/environments/staging/connection');
  expect(JSON.parse(String(putOf()?.[1]?.body))).toEqual({ connectionKey: 'lab', expectedVersion: 1 });
  // Not locked: the selector stays editable, and saving the same key is disabled.
  expect(screen.getByLabelText('Kết nối cho Staging')).toBeEnabled();
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
});

it('changes a configured connection before runtime exists using the new version and a distinct kind', async () => {
  const start = configuredTarget('lab', { connectionName: 'Lab', version: 3 });
  const aws = configuredTarget('cloud', { connectionName: 'Cloud', connectionKind: 'AWS', profile: 'aws-eks', region: 'us-east-1', runtimeStatus: 'PENDING', version: 4 });
  stub((_url, init) => init?.method === 'PUT' ? Response.json({ environment: envJson('staging', aws) }) : undefined);
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: start, production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'cloud');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByText('AWS (aws-eks)')).toBeInTheDocument();
  expect(JSON.parse(String(putOf()?.[1]?.body))).toEqual({ connectionKey: 'cloud', expectedVersion: 3 });
  expect(screen.getByText('us-east-1')).toBeInTheDocument();
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  expect(await screen.findByLabelText('Kết nối cho Production')).toHaveValue('');
});

it('requires an explicit transition once runtime exists and never sends a silent PUT', async () => {
  const deployed = configuredTarget('lab', { connectionName: 'Lab', runtimeExists: true });
  const calls = stub();
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: deployed, production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'other');
  expect(screen.queryByRole('button', { name: 'Lưu kết nối' })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xem lại chuyển đích…' }));
  expect(await screen.findByRole('region', { name: 'Chuyển đích kết nối' })).toBeInTheDocument();
  expect(screen.getByLabelText('Kết nối đích')).toHaveValue('other');
  expect(calls.some((call) => call.startsWith('PUT') && call.includes('/connection'))).toBe(false);
});

it('blocks edits while an operation owns the Environment', async () => {
  const busy = configuredTarget('lab', { activeOperation: { id: 'op1', kind: 'DEPLOY', status: 'ACTIVE', stage: 'DEPLOY web', startedAt: '', updatedAt: '', heartbeatAt: '', recoverable: false } });
  stub();
  render(<Host initial={{ ...base, environments: { staging: busy, production: unconfiguredTarget } }} />);
  expect(await screen.findByLabelText('Kết nối cho Staging')).toBeDisabled();
  expect(screen.getByRole('region', { name: 'Thao tác trên môi trường' })).toHaveTextContent('Thao tác đang chạy');
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
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Dữ liệu đã thay đổi kể từ lần tải trước');
  await waitFor(() => expect(screen.getByText('Thế hệ 0')).toBeInTheDocument());
  expect(puts).toBe(1);
  await user.selectOptions(screen.getByLabelText('Kết nối cho Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  await waitFor(() => expect(puts).toBe(2));
  expect(JSON.parse(String(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT')[1]?.[1]?.body))).toEqual({ connectionKey: 'lab', expectedVersion: 7 });
});

it('shows ENVIRONMENT_BUSY as a temporary message', async () => {
  stub((_url, init) => init?.method === 'PUT' ? Response.json({ error: 'the environment is busy with another operation; wait for it to finish or review its status', code: 'ENVIRONMENT_BUSY' }, { status: 409 }) : undefined);
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { version: 3 }), production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'other');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Môi trường đang bận');
  // Busy is not a version conflict: no reload of the Environment and the pick stays.
  expect(vi.mocked(fetch).mock.calls.some(([url]) => String(url).endsWith('/applications/catalog'))).toBe(false);
  expect(screen.getByLabelText('Kết nối cho Staging')).toHaveValue('other');
  expect(screen.getByRole('alert')).not.toHaveTextContent('Đã tải lại');
});

it('chooses by code, not status: unknown or hostile 409 codes show the safe status fallback and never raw text', async () => {
  stub((_url, init) => init?.method === 'PUT' ? Response.json({ error: 'raw english boom', code: '__proto__', field: 'constructor', params: { stage: 'x' } }, { status: 409 }) : undefined);
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { version: 3 }), production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'other');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('xung đột');
  expect(alert).not.toHaveTextContent(/raw english|boom|constructor/);
});

it('treats an unavailable connection as a safe 422 and reloads the choices', async () => {
  let loads = 0;
  stub((url, init) => {
    if (url.endsWith('/application-connections')) { loads += 1; }
    if (init?.method === 'PUT') return Response.json({ error: 'the selected connection is not available', code: 'CONNECTION_UNAVAILABLE' }, { status: 422 });
  });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('không còn khả dụng');
  await waitFor(() => expect(loads).toBeGreaterThan(1));
});

it('handles an empty list and a failed list with retry', async () => {
  let mode: 'empty' | 'error' | 'ok' = 'error';
  stub((url) => {
    if (url.endsWith('/application-connections')) return mode === 'error' ? Response.json({ error: 'x' }, { status: 500 }) : Response.json({ connections: mode === 'empty' ? [] : [lab] });
  });
  const user = userEvent.setup();
  render(<Host />);
  const region = () => within(screen.getByRole('region', { name: 'Kết nối triển khai' }));
  expect(await screen.findByText('Không tải được danh sách kết nối.')).toBeInTheDocument();
  mode = 'empty';
  await user.click(region().getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByText(/Chưa có kết nối Sẵn sàng/)).toBeInTheDocument();
  mode = 'ok';
  await user.click(region().getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByLabelText('Kết nối cho Staging')).toBeInTheDocument();
});

it('ignores a late set result for an Environment the user left, but caches it for its own Environment', async () => {
  const bound = configuredTarget('lab', { connectionName: 'Lab', version: 2 });
  let release: (response: Response) => void = () => undefined;
  stub((_url, init) => init?.method === 'PUT' ? new Promise<Response>((resolve) => { release = resolve; }) : undefined);
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  await user.click(screen.getByRole('tab', { name: 'Production' }));
  release(Response.json({ environment: envJson('staging', bound) }));
  expect(await screen.findByLabelText('Kết nối cho Production')).toHaveValue('');
  await user.click(screen.getByRole('tab', { name: 'Staging' }));
  await waitFor(() => expect(screen.getByLabelText('Kết nối cho Staging')).toHaveValue('lab'));
});

const pending = { key: 'pending', name: 'Pending', kind: 'KUBERNETES', status: 'PENDING' };
const unknownKind = { key: 'odd', name: 'Odd', kind: 'OTHER', status: 'READY' };

it('suggests the only eligible READY connection as an unsaved draft and persists nothing until Save', async () => {
  const bound = configuredTarget('lab', { connectionName: 'Lab', version: 2 });
  const calls = stub((url, init) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections: [pending, unknownKind, lab] });
    if (init?.method === 'PUT') return Response.json({ environment: envJson('staging', bound) });
  });
  const user = userEvent.setup();
  render(<Host />);
  const select = await screen.findByLabelText('Kết nối cho Staging');
  expect(select).toHaveValue('lab');
  expect(screen.getByText(/Gợi ý chưa lưu/)).toHaveTextContent('không phải mặc định của tổ chức');
  expect(within(select).queryByRole('option', { name: /Pending/ })).not.toBeInTheDocument();
  expect(within(select).queryByRole('option', { name: /Odd/ })).not.toBeInTheDocument();
  expect(calls.some((call) => call.startsWith('PUT'))).toBe(false);
  expect(screen.getByText('Chưa cấu hình')).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByText('Thế hệ 0')).toBeInTheDocument();
  expect(JSON.parse(String(putOf()?.[1]?.body))).toEqual({ connectionKey: 'lab', expectedVersion: 1 });
  expect(screen.queryByText(/Gợi ý chưa lưu/)).not.toBeInTheDocument();
});

it('does not suggest with several READY connections, none, or when a binding exists', async () => {
  stub();
  const first = render(<Host />);
  expect(await screen.findByLabelText('Kết nối cho Staging')).toHaveValue('');
  expect(screen.queryByText(/Gợi ý chưa lưu/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
  first.unmount();
  stub((url) => url.endsWith('/application-connections') ? Response.json({ connections: [lab, other] }) : undefined);
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('cloud', { connectionName: 'Cloud', connectionKind: 'AWS' }), production: unconfiguredTarget } }} />);
  // The bound Connection is kept in the list and is never retargeted to another one.
  const select = await screen.findByLabelText('Kết nối cho Staging');
  expect(select).toHaveValue('cloud');
  expect(screen.queryByText(/Gợi ý chưa lưu/)).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
});

it('never retargets a configured binding although exactly one other READY connection exists', async () => {
  stub((url) => url.endsWith('/application-connections') ? Response.json({ connections: [other] }) : undefined);
  const calls = vi.mocked(fetch);
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { connectionName: 'Lab' }), production: unconfiguredTarget } }} />);
  expect(await screen.findByLabelText('Kết nối cho Staging')).toHaveValue('lab');
  expect(calls.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false);
});

it('guides registration when no eligible connection exists and offers retry', async () => {
  stub((url) => url.endsWith('/application-connections') ? Response.json({ connections: [pending, unknownKind] }) : undefined);
  render(<Host />);
  expect(await screen.findByText(/Chưa có kết nối Sẵn sàng/)).toHaveTextContent('Kỹ sư nền tảng đăng ký kết nối');
  expect(screen.queryByLabelText('Kết nối cho Staging')).not.toBeInTheDocument();
});

it('keeps the user pick when the list is reloaded and flags a pick that is no longer READY', async () => {
  let connections = [lab, other];
  stub((url, init) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections });
    if (init?.method === 'PUT') return Response.json({ error: 'the selected connection is not available', code: 'CONNECTION_UNAVAILABLE' }, { status: 422 });
  });
  const user = userEvent.setup();
  render(<Host />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'other');
  connections = [lab, other];
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('không còn khả dụng');
  // The reload finished: the pick is still what the user chose, not a refreshed default.
  await waitFor(() => expect(screen.getByLabelText('Kết nối cho Staging')).toHaveValue('other'));
  connections = [lab];
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  await waitFor(() => expect(screen.getByText(/không còn Sẵn sàng/)).toBeInTheDocument());
  expect(screen.getByLabelText('Kết nối cho Staging')).toHaveValue('');
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
});

it('shows a placeholder, not the bound option, when the pick of a configured Environment disappears', async () => {
  let connections = [lab, other];
  stub((url, init) => {
    if (url.endsWith('/application-connections')) return Response.json({ connections });
    if (init?.method === 'PUT') return Response.json({ error: 'the selected connection is not available', code: 'CONNECTION_UNAVAILABLE' }, { status: 422 });
  });
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { connectionName: 'Lab' }), production: unconfiguredTarget } }} />);
  const select = await screen.findByLabelText('Kết nối cho Staging');
  expect(select).toHaveValue('lab');
  expect(within(select).queryByRole('option', { name: 'Chọn một kết nối' })).not.toBeInTheDocument();
  await user.selectOptions(select, 'other');
  connections = [lab];
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  await waitFor(() => expect(screen.getByText(/không còn Sẵn sàng/)).toBeInTheDocument());
  const lost = screen.getByLabelText('Kết nối cho Staging') as HTMLSelectElement;
  expect(lost).toHaveValue('');
  expect(within(lost).getByRole('option', { name: 'Chọn một kết nối' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
});

it('keeps a pending pick after a stale 409 reload and saves again with the new version', async () => {
  const latest = configuredTarget('cloud', { connectionName: 'Cloud', version: 9 });
  stub((url, init) => {
    if (init?.method === 'PUT') return Response.json({ error: 'stale', code: 'STALE_VERSION' }, { status: 409 });
    if (url.endsWith('/applications/catalog')) return Response.json({ application: { environments: [envJson('staging', latest), envJson('production', unconfiguredTarget)] } });
  });
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('lab', { version: 3 }), production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'other');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Đã tải lại trạng thái mới nhất');
  await waitFor(() => expect(screen.getByText('Cloud')).toBeInTheDocument());
  expect(screen.getByLabelText('Kết nối cho Staging')).toHaveValue('other');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  const puts = vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT');
  await waitFor(() => expect(puts.length).toBeGreaterThan(0));
  await waitFor(() => expect(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT').length).toBe(2));
  expect(JSON.parse(String(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT')[1]?.[1]?.body))).toEqual({ connectionKey: 'other', expectedVersion: 9 });
});

it('shows profile and region as read-only facts without any control', async () => {
  stub();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('cloud', { connectionName: 'Cloud', connectionKind: 'AWS', profile: 'aws-eks', region: 'us-east-1', runtimeStatus: 'PENDING' }), production: unconfiguredTarget } }} />);
  const panel = within(await screen.findByRole('region', { name: 'Kết nối triển khai' }));
  expect(panel.getByText('AWS (aws-eks)')).toBeInTheDocument();
  expect(panel.getByText('Vùng')).toBeInTheDocument();
  expect(panel.queryByText('Region')).not.toBeInTheDocument();
  expect(panel.getByText('us-east-1')).toBeInTheDocument();
  expect(panel.getByText('Đang chờ (PENDING)')).toBeInTheDocument();
  expect(panel.queryByLabelText(/Phạm vi triển khai|Region/)).not.toBeInTheDocument();
  expect(panel.getAllByRole('combobox')).toHaveLength(1);
});

it('claims a reload only after the GET succeeds: a failed stale reload keeps the pick, blocks Save and needs an explicit reload', async () => {
  let reloads = 0;
  stub((url, init) => {
    if (init?.method === 'PUT') return Response.json({ error: 'stale', code: 'STALE_VERSION' }, { status: 409 });
    if (url.endsWith('/applications/catalog')) { reloads += 1; return reloads === 1 ? Response.json({ error: 'down' }, { status: 503 }) : Response.json({ application: { environments: [envJson('staging', configuredTarget('cloud', { version: 7 })), envJson('production', unconfiguredTarget)] } }); }
  });
  const user = userEvent.setup();
  render(<Host initial={{ ...base, environments: { staging: configuredTarget('cloud', { version: 3 }), production: unconfiguredTarget } }} />);
  await user.selectOptions(await screen.findByLabelText('Kết nối cho Staging'), 'lab');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối' }));
  const alert = await screen.findByRole('alert');
  expect(alert).toHaveTextContent('Chưa tải lại được trạng thái mới nhất');
  expect(alert).not.toHaveTextContent('Đã tải lại trạng thái mới nhất');
  expect(screen.getByLabelText('Kết nối cho Staging')).toHaveValue('lab');
  expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Tải lại trạng thái' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Lưu kết nối' })).toBeEnabled());
  expect(vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === 'PUT')).toHaveLength(1);
});
