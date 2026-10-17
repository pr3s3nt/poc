import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { apiError, stubApi, type Call } from '../../../test/api';
import { DriverAccountsPage } from './DriverAccountsPage';
import type { Account } from './accountsApi';

// Synthetic fixture credentials: never real.
const keyId = 'AKIAFIXTUREKEY000001';
const secret = 'fixture/secret/value/0000000000000000000';
const kubeDoc = 'fixture-kubeconfig-document-body';

const aws: Account = {
  id: 'id-1', key: 'lab-aws', name: 'Lab AWS', type: 'AWS', authenticationType: 'AWS_ACCESS_KEY', status: 'READY', version: 3,
  config: { accountId: '123456789012', principalArn: 'arn:aws:iam::123456789012:user/orch', verificationRegions: ['us-east-1'] },
  verification: { verified: true, verifiedAt: '2026-10-17T07:00:00Z' },
};
const internal: Account = {
  id: 'id-2', key: 'lab-cluster', name: 'Lab cluster', type: 'INTERNAL', authenticationType: 'KUBECONFIG', status: 'READY', version: 1,
  config: { cluster: 'lab', kubeContext: 'lab-ctx', endpoint: 'https://lab.example:6443', tls: { mode: 'CUSTOM_CA' } },
  verification: { verified: true, verifiedAt: '2026-10-17T07:00:00Z' },
};

const is = (call: Call, method: string, url: string) => call.method === method && call.url === `/api/v1${url}`;
const pageText = () => document.body.textContent ?? '';
afterEach(() => vi.unstubAllGlobals());

function list(accounts: Account[]) {
  let current = accounts;
  return {
    set: (next: Account[]) => { current = next; },
    handler: (call: Call) => is(call, 'GET', '/driver-accounts') ? Response.json({ accounts: current }) : undefined,
    detail: (call: Call) => {
      const match = /^\/api\/v1\/driver-accounts\/([^/]+)$/.exec(call.url);
      const found = match && call.method === 'GET' ? current.find((account) => account.key === match[1]) : undefined;
      return found ? Response.json(found) : undefined;
    },
  };
}

describe('V01 infrastructure accounts page', () => {
  it('lists AWS and Internal with type apart from authentication, honest status and no assignment controls', async () => {
    const accounts = list([aws, { ...internal, status: 'VERIFYING' }]);
    stubApi(accounts.handler, accounts.detail);
    render(<DriverAccountsPage />);
    const table = await screen.findByRole('table', { name: 'Tài khoản hạ tầng' });
    expect(within(table).getByText('Access key')).toBeInTheDocument();
    expect(within(table).getByText('Kubeconfig')).toBeInTheDocument();
    expect(within(table).getByText('Sẵn sàng')).toBeInTheDocument();
    expect(within(table).getByText('Chưa xác minh')).toBeInTheDocument();
    expect(screen.queryByText(/Môi trường:|Gán cho môi trường|Phạm vi áp dụng/)).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/executionProfile|Vùng được phép/)).not.toBeInTheDocument();
  });

  it('keeps list load failure apart from an empty list and retries only the GET', async () => {
    const calls = stubApi(() => apiError(500, 'INTERNAL_ERROR'));
    render(<DriverAccountsPage />);
    expect(await screen.findByRole('alert', { name: 'Lỗi tải danh sách' })).toBeInTheDocument();
    expect(screen.queryByText('Chưa có tài khoản hạ tầng nào được đăng ký.')).not.toBeInTheDocument();
    expect(calls.every((call) => call.method === 'GET')).toBe(true);
  });

  it('creates an AWS account with verificationRegions, conceals and clears keys, labels Vùng kiểm tra', async () => {
    const user = userEvent.setup();
    const accounts = list([]);
    const calls = stubApi(accounts.handler, accounts.detail, (call) => {
      if (!is(call, 'POST', '/driver-accounts/aws')) return undefined;
      accounts.set([aws]);
      return Response.json(aws, { status: 201 });
    });
    render(<DriverAccountsPage />);
    await screen.findByText('Chưa có tài khoản hạ tầng nào được đăng ký.');
    await user.click(screen.getByLabelText('AWS'));
    expect(screen.getByLabelText('Access key ID')).toHaveAttribute('type', 'password');
    expect(screen.getByLabelText('Secret access key')).toHaveAttribute('type', 'password');
    await user.type(screen.getByLabelText('Tên tài khoản'), 'Lab AWS');
    await user.type(screen.getByLabelText('Access key ID'), keyId);
    await user.type(screen.getByLabelText('Secret access key'), secret);
    await user.type(screen.getByLabelText('Vùng kiểm tra'), 'us-east-1');
    await user.click(screen.getByRole('button', { name: 'Kiểm tra và lưu' }));
    await screen.findByText(/Đã đăng ký tài khoản Lab AWS/);
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.body).toEqual({ name: 'Lab AWS', accessKeyId: keyId, secretAccessKey: secret, verificationRegions: ['us-east-1'] });
    const createForm = screen.getByRole('group', { name: 'Đăng ký tài khoản mới' });
    expect(within(createForm).getByLabelText('Access key ID')).toHaveValue('');
    expect(within(createForm).getByLabelText('Secret access key')).toHaveValue('');
    expect(pageText()).not.toContain(secret);
    expect(pageText()).not.toContain(keyId);
  });

  it('creates an Internal account only from an inspected context and clears the pasted document', async () => {
    const user = userEvent.setup();
    const accounts = list([]);
    const calls = stubApi(accounts.handler, accounts.detail,
      (call) => is(call, 'POST', '/driver-accounts/internal/inspect') ? Response.json({ contexts: [{ name: 'lab-ctx', cluster: 'lab', endpoint: 'https://lab.example:6443' }] }) : undefined,
      (call) => { if (!is(call, 'POST', '/driver-accounts/internal')) return undefined; accounts.set([internal]); return Response.json(internal, { status: 201 }); });
    render(<DriverAccountsPage />);
    await screen.findByText('Chưa có tài khoản hạ tầng nào được đăng ký.');
    await user.type(screen.getByLabelText('Tên tài khoản'), 'Lab cluster');
    await user.click(screen.getByLabelText('Dán nội dung kubeconfig'));
    await user.click(screen.getByRole('button', { name: 'Kiểm tra và lưu' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('chọn context');
    expect(calls.some((call) => call.method === 'POST' && call.url.endsWith('/internal'))).toBe(false);
    await user.type(screen.getByLabelText('Nội dung kubeconfig'), kubeDoc);
    await screen.findByRole('group', { name: 'Đích đã chọn' }).catch(() => screen.findByLabelText('Đích đã chọn'));
    await user.click(screen.getByRole('button', { name: 'Kiểm tra và lưu' }));
    await screen.findByText(/Đã đăng ký tài khoản Lab cluster/);
    expect(calls.find((call) => call.url.endsWith('/internal') && call.method === 'POST')?.body).toEqual({ name: 'Lab cluster', kubeconfig: kubeDoc, context: 'lab-ctx' });
    expect(screen.getByLabelText('Nội dung kubeconfig')).toHaveValue('');
  });

  it('after a lost create response reloads the list by GET and never resubmits', async () => {
    const user = userEvent.setup();
    const accounts = list([]);
    let posts = 0;
    stubApi(accounts.handler, accounts.detail, (call) => { if (call.method === 'POST') { posts += 1; throw new TypeError('network'); } return undefined; });
    render(<DriverAccountsPage />);
    await screen.findByText('Chưa có tài khoản hạ tầng nào được đăng ký.');
    await user.click(screen.getByLabelText('AWS'));
    await user.type(screen.getByLabelText('Tên tài khoản'), 'Lab AWS');
    await user.type(screen.getByLabelText('Access key ID'), keyId);
    await user.type(screen.getByLabelText('Secret access key'), secret);
    await user.type(screen.getByLabelText('Vùng kiểm tra'), 'us-east-1');
    await user.click(screen.getByRole('button', { name: 'Kiểm tra và lưu' }));
    expect(await screen.findByText(/chưa biết thao tác đã được lưu hay chưa/)).toBeInTheDocument();
    await waitFor(() => expect(screen.getByText(/Chưa biết lần đăng ký trước/)).toBeInTheDocument());
    expect(posts).toBe(1);
    expect(screen.getByLabelText('Secret access key')).toHaveValue('');
  });

  async function openDetail(user: ReturnType<typeof userEvent.setup>, name = 'Lab AWS') {
    await user.click(await screen.findByRole('button', { name: `Xem ${name}` }));
    return screen.findByRole('region', { name: `Chi tiết tài khoản ${name}` });
  }

  it('renames with the loaded version, preserves the draft on 409 and reloads only by GET', async () => {
    const user = userEvent.setup();
    const accounts = list([aws]);
    const calls = stubApi(accounts.handler, accounts.detail, (call) => is(call, 'PUT', '/driver-accounts/lab-aws') ? apiError(409, 'STALE_VERSION') : undefined);
    render(<DriverAccountsPage />);
    const detail = await openDetail(user);
    const field = within(detail).getByLabelText('Tên tài khoản');
    await user.clear(field);
    await user.type(field, 'Tên mới');
    accounts.set([{ ...aws, name: 'Người khác đổi', version: 4 }]);
    await user.click(within(detail).getByRole('button', { name: 'Lưu tên' }));
    expect(await within(detail).findByRole('alert')).toHaveTextContent('phiên bản không còn khớp');
    expect(calls.find((call) => call.method === 'PUT')?.body).toEqual({ name: 'Tên mới', version: 3 });
    expect(field).toHaveValue('Tên mới');
    await user.click(within(detail).getByRole('button', { name: 'Tải lại bản mới nhất' }));
    await within(detail).findByText(/phiên bản hiện tại 4/);
    expect(within(detail).getByLabelText('Tên tài khoản')).toHaveValue('Tên mới');
    expect(calls.filter((call) => call.method === 'PUT')).toHaveLength(1);
  });

  it('shows a failed verification apart from the last successful READY and leaves the record unchanged', async () => {
    const user = userEvent.setup();
    const accounts = list([aws]);
    stubApi(accounts.handler, accounts.detail, (call) => is(call, 'POST', '/driver-accounts/lab-aws/verify') ? apiError(422, 'VERIFICATION_CREDENTIAL_REJECTED') : undefined);
    render(<DriverAccountsPage />);
    const detail = await openDetail(user);
    expect(within(detail).getByText(/không phải tình trạng kết nối hiện tại/)).toBeInTheDocument();
    await user.click(within(detail).getByRole('button', { name: 'Kiểm tra kết nối' }));
    const failure = await within(detail).findByRole('alert', { name: 'Lần kiểm tra vừa thất bại' });
    expect(failure).toHaveTextContent('thông tin xác thực bị từ chối');
    expect(failure).toHaveTextContent('lần kiểm tra thành công gần nhất');
    expect(within(detail).getByText('Sẵn sàng')).toBeInTheDocument();
    expect(within(detail).queryByText(/Kiểm tra kết nối thành công/)).not.toBeInTheDocument();
  });

  it('verify success updates version and disables duplicate clicks while pending', async () => {
    const user = userEvent.setup();
    const accounts = list([aws]);
    let release: (r: Response) => void = () => undefined;
    let posts = 0;
    stubApi(accounts.handler, accounts.detail, (call) => {
      if (!is(call, 'POST', '/driver-accounts/lab-aws/verify')) return undefined;
      posts += 1;
      return new Promise<Response>((resolve) => { release = resolve; }) as unknown as Response;
    });
    render(<DriverAccountsPage />);
    const detail = await openDetail(user);
    await user.click(within(detail).getByRole('button', { name: 'Kiểm tra kết nối' }));
    const pending = await within(detail).findByRole('button', { name: 'Đang kiểm tra…' });
    expect(pending).toBeDisabled();
    await user.click(pending);
    expect(posts).toBe(1);
    release(Response.json({ ...aws, version: 4 }));
    await within(detail).findByText(/Kiểm tra kết nối thành công/);
    expect(within(detail).getByText(/· phiên bản 4/)).toBeInTheDocument();
  });

  it('replaces an AWS credential: keys concealed, cleared on send and on success, never shown', async () => {
    const user = userEvent.setup();
    const accounts = list([aws]);
    const calls = stubApi(accounts.handler, accounts.detail, (call) => is(call, 'PUT', '/driver-accounts/lab-aws/credentials') ? Response.json({ ...aws, version: 4 }) : undefined);
    render(<DriverAccountsPage />);
    const detail = await openDetail(user);
    const form = within(detail).getByRole('group', { name: 'Thay credential' });
    expect(within(form).getByLabelText('Access key ID')).toHaveValue('');
    await user.type(within(form).getByLabelText('Access key ID'), keyId);
    await user.type(within(form).getByLabelText('Secret access key'), secret);
    await user.click(within(form).getByRole('button', { name: 'Xác minh và thay credential' }));
    await within(form).findByText(/Đã thay credential/);
    expect(calls.find((call) => call.method === 'PUT')?.body).toEqual({ version: 3, accessKeyId: keyId, secretAccessKey: secret, verificationRegions: ['us-east-1'] });
    expect(within(form).getByLabelText('Access key ID')).toHaveValue('');
    expect(pageText()).not.toContain(secret);
  });

  it('clears keys on a target mismatch rejection and explains the old credential is kept; cancel clears too', async () => {
    const user = userEvent.setup();
    const accounts = list([aws]);
    stubApi(accounts.handler, accounts.detail, (call) => is(call, 'PUT', '/driver-accounts/lab-aws/credentials') ? apiError(422, 'ACCOUNT_TARGET_MISMATCH') : undefined);
    render(<DriverAccountsPage />);
    const detail = await openDetail(user);
    const form = within(detail).getByRole('group', { name: 'Thay credential' });
    await user.type(within(form).getByLabelText('Access key ID'), keyId);
    await user.type(within(form).getByLabelText('Secret access key'), secret);
    await user.click(within(form).getByRole('button', { name: 'Xác minh và thay credential' }));
    expect(await within(form).findByRole('alert')).toHaveTextContent('Credential cũ vẫn được giữ');
    expect(within(form).getByLabelText('Secret access key')).toHaveValue('');
    await user.type(within(form).getByLabelText('Access key ID'), keyId);
    await user.click(within(form).getByRole('button', { name: 'Hủy' }));
    expect(within(form).getByLabelText('Access key ID')).toHaveValue('');
  });

  it('after an unknown replace outcome blocks mutations until a GET reload succeeds', async () => {
    const user = userEvent.setup();
    const accounts = list([aws]);
    let puts = 0;
    let failGet = true;
    stubApi((call) => is(call, 'GET', '/driver-accounts/lab-aws') && failGet && puts > 0 ? apiError(500, 'INTERNAL_ERROR') : undefined,
      accounts.handler, accounts.detail, (call) => { if (call.method === 'PUT') { puts += 1; throw new TypeError('network'); } return undefined; });
    render(<DriverAccountsPage />);
    const detail = await openDetail(user);
    const form = within(detail).getByRole('group', { name: 'Thay credential' });
    await user.type(within(form).getByLabelText('Access key ID'), keyId);
    await user.type(within(form).getByLabelText('Secret access key'), secret);
    await user.click(within(form).getByRole('button', { name: 'Xác minh và thay credential' }));
    await within(form).findByText(/chưa biết thao tác đã được lưu hay chưa/);
    expect(within(detail).getByRole('button', { name: 'Kiểm tra kết nối' })).toBeDisabled();
    failGet = false;
    await user.click(within(detail).getByRole('button', { name: 'Tải lại bản mới nhất' }));
    // Loaded, but still blocked until the user explicitly reviewed the result.
    await within(detail).findByRole('button', { name: 'Đã xem kết quả, cho phép thao tác tiếp' });
    expect(within(detail).getByRole('button', { name: 'Kiểm tra kết nối' })).toBeDisabled();
    await user.click(within(detail).getByRole('button', { name: 'Đã xem kết quả, cho phép thao tác tiếp' }));
    await waitFor(() => expect(within(detail).getByRole('button', { name: 'Kiểm tra kết nối' })).toBeEnabled());
    expect(puts).toBe(1);
  });

  it('shows Internal TLS metadata and a committed change even when the list refresh fails', async () => {
    const user = userEvent.setup();
    const accounts = list([internal]);
    let listFails = false;
    stubApi((call) => is(call, 'GET', '/driver-accounts') && listFails ? apiError(500, 'INTERNAL_ERROR') : undefined,
      accounts.handler, accounts.detail, (call) => is(call, 'PUT', '/driver-accounts/lab-cluster') ? Response.json({ ...internal, name: 'Đổi tên', version: 2 }) : undefined);
    render(<DriverAccountsPage />);
    const detail = await openDetail(user, 'Lab cluster');
    expect(within(detail).getByText('CA tùy chỉnh')).toBeInTheDocument();
    listFails = true;
    const field = within(detail).getByLabelText('Tên tài khoản');
    await user.clear(field);
    await user.type(field, 'Đổi tên');
    await user.click(within(detail).getByRole('button', { name: 'Lưu tên' }));
    await within(detail).findByText(/Đã đổi tên thành “Đổi tên”; phiên bản 2/);
    expect(await screen.findByRole('alert', { name: 'Lỗi tải danh sách' })).toBeInTheDocument();
  });

  it('ignores a late detail response after another account was selected', async () => {
    const user = userEvent.setup();
    const accounts = list([aws, internal]);
    let releaseAws: (r: Response) => void = () => undefined;
    stubApi(accounts.handler, (call) => is(call, 'GET', '/driver-accounts/lab-aws') ? new Promise<Response>((resolve) => { releaseAws = resolve; }) as unknown as Response : undefined, accounts.detail);
    render(<DriverAccountsPage />);
    await user.click(await screen.findByRole('button', { name: 'Xem Lab AWS' }));
    await user.click(screen.getByRole('button', { name: 'Xem Lab cluster' }));
    await screen.findByRole('region', { name: 'Chi tiết tài khoản Lab cluster' });
    releaseAws(Response.json(aws));
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(screen.queryByRole('region', { name: 'Chi tiết tài khoản Lab AWS' })).not.toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Chi tiết tài khoản Lab cluster' })).toBeInTheDocument();
  });

  describe('create after an unknown outcome', () => {
    async function fillAwsCreate(user: ReturnType<typeof userEvent.setup>) {
      await screen.findByText('Chưa có tài khoản hạ tầng nào được đăng ký.');
      await user.click(screen.getByLabelText('AWS'));
      await user.type(screen.getByLabelText('Tên tài khoản'), 'Lab AWS');
      await user.type(screen.getByLabelText('Access key ID'), keyId);
      await user.type(screen.getByLabelText('Secret access key'), secret);
      await user.type(screen.getByLabelText('Vùng kiểm tra'), 'us-east-1');
    }
    const submit = (user: ReturnType<typeof userEvent.setup>) => user.click(within(screen.getByRole('group', { name: 'Đăng ký tài khoản mới' })).getByRole('button', { name: 'Kiểm tra và lưu' }));

    it.each([
      ['a lost response', () => { throw new TypeError('network'); }],
      ['a server-reported uncertain commit', () => apiError(503, 'COMMIT_UNCERTAIN')],
    ])('stays blocked while the recovery GET is pending and after it fails, and recovers only after a successful GET and explicit review (%s)', async (_name, failure) => {
      const user = userEvent.setup();
      let posts = 0;
      let gate: 'pending' | 'fail' | 'ok' = 'pending';
      let release: (r: Response) => void = () => undefined;
      let listCalls = 0;
      stubApi((call) => {
        if (is(call, 'GET', '/driver-accounts')) {
          listCalls += 1;
          if (listCalls === 1) return Response.json({ accounts: [] });
          if (gate === 'pending') return new Promise<Response>((resolve) => { release = resolve; }) as unknown as Response;
          return gate === 'fail' ? apiError(500, 'INTERNAL_ERROR') : Response.json({ accounts: [aws] });
        }
        if (call.method === 'POST') { posts += 1; return failure(); }
        return undefined;
      });
      render(<DriverAccountsPage />);
      await fillAwsCreate(user);
      await submit(user);
      expect(await screen.findByText(/chưa biết thao tác đã được lưu hay chưa/)).toBeInTheDocument();
      const button = () => within(screen.getByRole('group', { name: 'Đăng ký tài khoản mới' })).getByRole('button', { name: 'Kiểm tra và lưu' });
      // GET pending: a second user click must not post.
      expect(button()).toBeDisabled();
      await user.click(button());
      expect(posts).toBe(1);
      // GET fails: still blocked, with an explicit retry that only GETs.
      gate = 'fail';
      release(apiError(500, 'INTERNAL_ERROR'));
      const createGroup = () => screen.getByRole('group', { name: 'Đăng ký tài khoản mới' });
      const retry = await within(createGroup()).findByRole('button', { name: 'Tải lại danh sách' });
      expect(button()).toBeDisabled();
      await user.click(button());
      expect(posts).toBe(1);
      // GET succeeds: still blocked until the user reviewed the list.
      gate = 'ok';
      await user.click(retry);
      const review = await screen.findByRole('button', { name: 'Đã xem danh sách, cho phép đăng ký lại' });
      expect(button()).toBeDisabled();
      expect(posts).toBe(1);
      await user.click(review);
      expect(button()).toBeEnabled();
      // Draft survived; the keys were cleared and must be typed again.
      expect(screen.getByLabelText('Tên tài khoản')).toHaveValue('Lab AWS');
      expect(within(screen.getByRole('group', { name: 'Đăng ký tài khoản mới' })).getByLabelText('Secret access key')).toHaveValue('');
      expect(pageText()).not.toContain(secret);
    });
  });
});
