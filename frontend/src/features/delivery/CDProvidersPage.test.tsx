import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { CDProvidersPage } from './CDProvidersPage';

const ready = { id: 'binding', version: 1, revision: 1, name: 'Fleet team', provider: 'fleet', status: 'UNVERIFIED', capabilities: { selfHeal: true } };
const fields = [
  { key: 'targetAccountKey', kind: 'account', labelKey: 'cd.fields.targetAccountKey', required: true },
  { key: 'gitConnectionKey', kind: 'gitConnection', labelKey: 'cd.fields.gitConnectionKey', required: false },
  { key: 'workspace', kind: 'text', labelKey: 'cd.fields.workspace', default: 'fleet-local', required: true },
  { key: 'targetCluster', kind: 'text', labelKey: 'cd.fields.targetCluster', default: 'local', required: true },
  { key: 'targetClusterUID', kind: 'text', labelKey: 'cd.fields.targetClusterUID', readOnly: true, advanced: true, required: true },
];
const noGitConnections = { connections: [] };
const account = { key: 'local-k8s', name: 'Internal cluster', type: 'INTERNAL', version: 3, status: 'READY' };
afterEach(() => vi.unstubAllGlobals());

it('scopes same-named targets by workspace and replaces the pinned target identity on reselection', async () => {
  let saved: Record<string, unknown> | undefined;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith('/inspect')) return Response.json({ options: {
      workspace: ['team-a', 'team-b'].map(value => ({ name: value, value, values: { workspace: value } })),
      targetCluster: ['team-a', 'team-b'].map(workspace => ({ name: `${workspace}/local`, value: 'local', values: { workspace, targetCluster: 'local', targetClusterUID: `${workspace}-uid`, targetAccountKey: account.key } })),
    } });
    if (path.endsWith('/delivery-bindings') && init?.method === 'POST') { saved = JSON.parse(String(init.body)); return Response.json(ready, { status: 201 }); }
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json({ connections: [{ key: 'team-git', name: 'Team Git', kind: 'GIT', status: 'READY' }] });
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  await waitFor(() => expect(screen.getByLabelText('Workspace Fleet')).toBeEnabled());
  await user.selectOptions(screen.getByLabelText('Workspace Fleet'), 'team-a');
  expect(screen.getByLabelText('Cluster đã đăng ký')).toHaveValue('local');
  await user.selectOptions(screen.getByLabelText('Workspace Fleet'), 'team-b');
  expect(within(screen.getByLabelText('Cluster đã đăng ký')).queryByRole('option', { name: 'team-a/local' })).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText('Kết nối Git'), 'team-git');
  await user.type(screen.getByLabelText('Tên cấu hình'), 'Team B');
  await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }));
  await waitFor(() => expect(saved?.config).toMatchObject({ workspace: 'team-b', targetCluster: 'local', targetClusterUID: 'team-b-uid' }));
});

it('creates and verifies CD using typed form fields, discovery and write-only credentials', async () => {
  const requests: { path: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>; requests.push({ path, body });
      if (path.endsWith('/inspect')) return Response.json({ options: { workspace: [{ name: 'fleet-local', value: 'fleet-local', values: { workspace: 'fleet-local' } }], targetCluster: [{ name: 'local', value: 'local', values: { targetCluster: 'local', targetClusterUID: 'cluster-incarnation' } }] } });
      if (path.endsWith('/verify')) return Response.json({ ...ready, revision: 2, status: 'READY' });
      return Response.json(ready, { status: 201 });
    }
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json({ connections: [{ key: 'team-git', name: 'Team Git', kind: 'GIT', status: 'READY', config: { url: 'https://github.com/team' } }] });
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  await user.type(screen.getByLabelText('Tên cấu hình'), 'Fleet team');
  await user.selectOptions(screen.getByLabelText('Tài khoản quản lý CD'), account.key);
  await user.click(screen.getByRole('button', { name: 'Tìm cấu hình trên cluster' }));
  await waitFor(() => expect(screen.getByLabelText('Cluster đã đăng ký')).toHaveValue('local'));
  expect(screen.queryByLabelText('Định danh cluster đã xác minh')).not.toBeInTheDocument();
  await user.selectOptions(screen.getByLabelText('Kết nối Git'), 'team-git');
  await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }));
  expect(await screen.findByText('Đã lưu. Xác minh cấu hình trước khi sử dụng.')).toBeInTheDocument();
  expect(screen.queryByDisplayValue('private-token')).not.toBeInTheDocument();
  const create = requests.find(r => r.path.endsWith('/delivery-bindings'));
  expect(create?.body).toMatchObject({ name: 'Fleet team', provider: 'fleet', managementAccountKey: account.key, managementAccountVersion: 3, writerUsername: '', writerPassword: '', config: { targetClusterUID: 'cluster-incarnation', gitConnectionKey: 'team-git' } });
  expect(screen.queryByRole('textbox', { name: /JSON|Workload/ })).not.toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Xác minh' }));
  expect(await screen.findByText('Đã xác minh kết nối CD.')).toBeInTheDocument();
  expect(screen.getByRole('cell', { name: 'Sẵn sàng' })).toBeInTheDocument();
});

it('edits an immutable version while retaining credentials and displays environment usage', async () => {
  const requests: Record<string, unknown>[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === 'POST') { requests.push(JSON.parse(String(init.body)) as Record<string, unknown>); return Response.json({ ...ready, version: 2, name: requests[0]?.name }); }
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json(noGitConnections);
    if (path.includes('?version=')) return Response.json({ binding: ready, managementAccountKey: account.key, managementAccountVersion: 3, config: { workspace: 'fleet-local', targetCluster: 'local', targetClusterUID: 'uid', repositoryOwner: 'platform-team' }, credentialConfigured: true, usage: [{ applicationKey: 'shop', environmentKey: 'staging', applied: true, desired: true }] });
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

it('sets the platform default using its revision and reloads after a lost acknowledgement', async () => {
  const verified = { ...ready, status: 'READY' };
  let selected = false;
  let writes = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith('/delivery-default')) {
      if (init?.method === 'PUT') {
        writes++;
        expect(JSON.parse(String(init.body))).toEqual({ bindingId: 'binding', bindingVersion: 1, expectedRevision: 0 });
        selected = true;
        throw new TypeError('response lost');
      }
      return Response.json(selected ? { bindingId: 'binding', bindingVersion: 1, revision: 1 } : { bindingId: '', bindingVersion: 0, revision: 0 });
    }
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json(noGitConnections);
    return Response.json({ bindings: [verified] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  const button = await screen.findByRole('button', { name: 'Dùng cho ứng dụng mới' });
  await user.click(button);
  await waitFor(() => expect(button).toBeDisabled());
  expect(writes).toBe(1);
  await user.click(screen.getByRole('button', { name: 'Tải lại' }));
  expect(await screen.findByText('Mặc định')).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Dùng cho ứng dụng mới' })).toBeDisabled();
  expect(writes).toBe(1);
});

it('reuses platform Git without exposing token fields and discovers Fleet on account selection', async () => {
  const requests: { path: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>;
      requests.push({ path, body });
      if (path.endsWith('/inspect')) return Response.json({ options: { workspace: [{ name: 'fleet-local', value: 'fleet-local', values: { workspace: 'fleet-local' } }], targetCluster: [{ name: 'local', value: 'local', values: { targetCluster: 'local', targetClusterUID: 'discovered-uid', workspace: 'fleet-local' } }] } });
      return Response.json(ready, { status: 201 });
    }
    if (path.endsWith('/platform/settings/git')) return Response.json({ settings: { tokenConfigured: true, url: 'https://github.com/platform-team', username: 'platform-writer', repositoryOwner: 'platform-team' } });
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json(noGitConnections);
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  expect(await screen.findByLabelText('Kết nối Git')).toHaveValue('__platform');
  expect(screen.queryByLabelText('Token hoặc mật khẩu Git')).not.toBeInTheDocument();
  await user.type(screen.getByLabelText('Tên cấu hình'), 'Shared Git');
  await user.selectOptions(screen.getByLabelText('Tài khoản quản lý CD'), account.key);
  await waitFor(() => expect(requests.some(r => r.path.endsWith('/inspect'))).toBe(true));
  expect(screen.queryByLabelText('Tài khoản hoặc tổ chức GitHub')).not.toBeInTheDocument();
  expect(screen.getByText(/Nơi tạo repository.*platform-team/)).toBeInTheDocument();
  await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }));
  await waitFor(() => expect(requests.find(r => r.path.endsWith('/delivery-bindings'))?.body).toMatchObject({ usePlatformGit: true, writerUsername: '', writerPassword: '', config: { targetClusterUID: 'discovered-uid' } }));
});

it('registers a Git connection inline and binds it to the CD configuration', async () => {
  const requests: { path: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body)) as Record<string, unknown>; requests.push({ path, body });
      if (path.endsWith('/connections/git')) return Response.json({ key: 'git-team' }, { status: 201 });
      if (path.endsWith('/inspect')) return Response.json({ options: { workspace: [{ name: 'fleet-local', value: 'fleet-local', values: { workspace: 'fleet-local' } }], targetCluster: [{ name: 'local', value: 'local', values: { targetCluster: 'local', targetClusterUID: 'discovered-uid', workspace: 'fleet-local' } }] } });
      return Response.json(ready, { status: 201 });
    }
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json(noGitConnections);
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  await user.type(screen.getByLabelText('Tên cấu hình'), 'Fleet git');
  await user.selectOptions(screen.getByLabelText('Tài khoản quản lý CD'), account.key);
  await user.selectOptions(await screen.findByLabelText('Kết nối Git'), '__new');
  await user.type(screen.getByLabelText('Tên kết nối Git'), 'Team Git');
  await user.type(screen.getByLabelText('URL phạm vi Git (tài khoản/nhóm)'), 'https://github.com/my-team');
  await user.type(screen.getByLabelText('Tên đăng nhập Git'), 'writer');
  await user.type(screen.getByLabelText('Token hoặc mật khẩu Git'), 'private-token');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối Git' }));
  await waitFor(() => expect(requests.find(r => r.path.endsWith('/connections/git'))?.body).toMatchObject({ name: 'Team Git', url: 'https://github.com/my-team', username: 'writer', token: 'private-token' }));
  expect(screen.queryByDisplayValue('private-token')).not.toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText('Kết nối Git')).toHaveValue('git-team'));
  await user.click(screen.getByRole('button', { name: 'Lưu cấu hình' }));
  await waitFor(() => expect(requests.find(r => r.path.endsWith('/delivery-bindings'))?.body).toMatchObject({ usePlatformGit: false, writerUsername: '', writerPassword: '', config: { gitConnectionKey: 'git-team', targetClusterUID: 'discovered-uid' } }));
});

it('shows discovery failure inside the form and permits retry', async () => {
  let inspections = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
    const path = String(input);
    if (path.endsWith('/inspect')) { inspections++; return Response.json({ code: 'SERVICE_UNAVAILABLE' }, { status: 503 }); }
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json(noGitConnections);
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  expect(screen.getByLabelText('Tài khoản quản lý CD')).toHaveValue(account.key);
  const alert = await screen.findByRole('alert');
  expect(alert.closest('form')).not.toBeNull();
  expect(alert).toHaveTextContent('Kiểm tra cluster đã cài Fleet');
  await user.click(screen.getByRole('button', { name: 'Tìm cấu hình trên cluster' }));
  await waitFor(() => expect(inspections).toBe(2));
});

it('clears Git token and holds registration after a lost response until explicit reload/review', async () => {
  let writes = 0;
  vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith('/connections/git') && init?.method === 'POST') { writes++; throw new TypeError('acknowledgement lost'); }
    if (path.endsWith('/delivery-providers')) return Response.json({ providers: [{ kind: 'fleet', fields }] });
    if (path.endsWith('/driver-accounts')) return Response.json({ accounts: [account] });
    if (path.endsWith('/connections')) return Response.json({ connections: writes ? [{ key: 'observed-git', kind: 'GIT', status: 'READY', name: 'Team Git' }] : [] });
    if (path.endsWith('/delivery-default')) return Response.json({ bindingId: '', bindingVersion: 0, revision: 0 });
    if (path.endsWith('/inspect')) return Response.json({ options: { workspace: [], targetCluster: [] } });
    return Response.json({ bindings: [] });
  }));
  const user = userEvent.setup(); render(<CDProvidersPage />);
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  await user.selectOptions(await screen.findByLabelText('Kết nối Git'), '__new');
  await user.type(screen.getByLabelText('Tên kết nối Git'), 'Team Git');
  await user.type(screen.getByLabelText('URL phạm vi Git (tài khoản/nhóm)'), 'https://github.com/team');
  await user.type(screen.getByLabelText('Tên đăng nhập Git'), 'writer');
  await user.type(screen.getByLabelText('Token hoặc mật khẩu Git'), 'private-token');
  await user.click(screen.getByRole('button', { name: 'Lưu kết nối Git' }));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Lưu kết nối Git' })).toBeDisabled());
  expect(screen.getByLabelText('Token hoặc mật khẩu Git')).toHaveValue('');
  expect(screen.getByLabelText('Token hoặc mật khẩu Git')).toBeDisabled();
  await user.click(screen.getByRole('button', { name: 'Tải lại' }));
  await user.click(await screen.findByRole('button', { name: 'Thêm cấu hình CD' }));
  expect(await screen.findByRole('option', { name: 'Team Git' })).toHaveValue('observed-git');
  expect(writes).toBe(1);
});
