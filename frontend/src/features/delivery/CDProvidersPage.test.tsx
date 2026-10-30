import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CDProvidersPage } from './CDProvidersPage';

const ready = { id: 'binding', version: 1, revision: 1, name: 'Fleet team', provider: 'fleet', status: 'UNVERIFIED', capabilities: { selfHeal: true } };
const fields = [
  { key: 'workspace', kind: 'text', labelKey: 'cd.fields.workspace', default: 'fleet-local', required: true },
  { key: 'targetCluster', kind: 'text', labelKey: 'cd.fields.targetCluster', default: 'local', required: true },
  { key: 'targetClusterUID', kind: 'text', labelKey: 'cd.fields.targetClusterUID', readOnly: true, advanced: true, required: true },
  { key: 'repositoryURL', kind: 'url', labelKey: 'cd.fields.repositoryURL', required: true },
];
const account = { key: 'local-k8s', name: 'Internal cluster', type: 'INTERNAL', version: 3, status: 'READY' };
afterEach(() => vi.unstubAllGlobals());

it('creates and verifies CD using typed form fields, discovery and write-only credentials', async () => {
  const requests: { path: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>; requests.push({ path, body });
      if (path.endsWith('/inspect')) return Response.json({ options: { targetCluster: [{ name: 'local', value: 'local', values: { targetCluster: 'local', targetClusterUID: 'cluster-incarnation' } }] } });
      if (path.endsWith('/verify')) return Response.json({ ...ready, revision: 2, status: 'READY' });
      return Response.json(ready, { status: 201 });
    }
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  await user.type(screen.getByLabelText('Tên cấu hình'), 'Fleet team');
  await user.selectOptions(screen.getByLabelText('Tài khoản quản lý CD'), account.key);
  await user.click(screen.getByRole('button', { name: 'Tìm cấu hình trên cluster' }));
  await waitFor(() => expect(screen.getByLabelText('Định danh cluster đã xác minh')).toHaveValue('cluster-incarnation'));
  await user.type(screen.getByLabelText('Git repository HTTPS'), 'https://git.example/config.git');
  await user.click(screen.getByText('Thông tin ghi Git', { exact: true }));
  await user.type(screen.getByLabelText('Tên đăng nhập Git'), 'writer');
  await user.type(screen.getByLabelText('Token hoặc mật khẩu Git'), 'private-token');
  await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }));
  expect(await screen.findByText('Đã lưu. Xác minh cấu hình trước khi sử dụng.')).toBeInTheDocument();
  expect(screen.queryByDisplayValue('private-token')).not.toBeInTheDocument();
  const create = requests.find(r => r.path.endsWith('/delivery-bindings'));
  expect(create?.body).toMatchObject({ name: 'Fleet team', provider: 'fleet', managementAccountKey: account.key, managementAccountVersion: 3, writerUsername: 'writer', writerPassword: 'private-token', config: { targetClusterUID: 'cluster-incarnation', repositoryURL: 'https://git.example/config.git' } });
  expect(screen.queryByRole('textbox', { name: /JSON|Score/ })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xác minh' }));
  expect(await screen.findByText('Đã xác minh kết nối CD.')).toBeInTheDocument();
  expect(screen.getByRole('cell', { name: 'Sẵn sàng' })).toBeInTheDocument();
});

it('edits an immutable version while retaining credentials and displays environment usage', async () => {
  const requests: Record<string, unknown>[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === 'POST') { requests.push(JSON.parse(String(init.body)) as Record<string, unknown>); return Response.json({ ...ready, version: 2, name: requests[0]?.name }); }
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.includes('?version=')) return Response.json({ binding: ready, managementAccountKey: account.key, managementAccountVersion: 3, config: { workspace: 'fleet-local', targetCluster: 'local', targetClusterUID: 'uid', repositoryURL: 'https://git.example/config.git' }, credentialConfigured: true, usage: [{ applicationKey: 'shop', environmentKey: 'staging', applied: true, desired: true }] });
    return Response.json({ bindings: [ready] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Xem chi tiết' }));
  expect(await screen.findByRole('button', { name: 'shop / staging' })).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Sửa thành phiên bản mới' }));
  const name = await screen.findByLabelText('Tên cấu hình'); await user.clear(name); await user.type(name, 'Fleet revised');
  expect(screen.getByRole('checkbox', { name: 'Giữ thông tin Git hiện tại' })).toBeChecked();
  expect(screen.queryByLabelText('Token hoặc mật khẩu Git')).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }));
  await screen.findByText('Đã lưu. Xác minh cấu hình trước khi sử dụng.');
  expect(requests[0]).toMatchObject({ name: 'Fleet revised', previousVersion: 1, expectedRevision: 1, retainWriterCredential: true, writerUsername: '', writerPassword: '' });
  const table = screen.getByRole('table'); expect(within(table).getByText('Fleet team')).toBeInTheDocument(); expect(within(table).getByRole('cell', { name: '2' })).toBeInTheDocument();
});
