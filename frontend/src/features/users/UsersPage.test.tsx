import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { UsersPage } from './UsersPage';

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function stubFetch(handler: (url: string, init?: RequestInit) => Response | undefined) {
  const fetchMock = vi.fn(async (input: string | URL | Request, init?: RequestInit) => handler(String(input), init) ?? jsonResponse(404, { code: 'NOT_FOUND' }));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const accounts = { users: [
  { id: '1', username: 'admin', role: 'ADMIN', status: 'ACTIVE' },
  { id: '2', username: 'developer', role: 'DEVELOPER', status: 'ACTIVE' },
] };

it('lists the Organization accounts without any password material', async () => {
  stubFetch((url) => url.endsWith('/users') ? jsonResponse(200, accounts) : undefined);
  render(<UsersPage />);
  expect(await screen.findByText('admin')).toBeInTheDocument();
  expect(screen.getByText('developer')).toBeInTheDocument();
  expect(screen.getAllByText('Quản trị viên').length).toBeGreaterThan(0);
  expect(screen.queryByText(/passwordHash/i)).not.toBeInTheDocument();
});

it('creates an account and refreshes the list', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url.endsWith('/users') && init?.method === 'POST') return jsonResponse(201, { user: { id: '3', username: 'newdev', role: 'DEVELOPER', status: 'ACTIVE' } });
    if (url.endsWith('/users')) return jsonResponse(200, accounts);
    return undefined;
  });
  const user = userEvent.setup();
  render(<UsersPage />);
  await screen.findByText('developer');
  await user.type(screen.getByLabelText('Tên đăng nhập'), 'newdev');
  await user.type(screen.getByLabelText('Mật khẩu'), 'password-1');
  await user.click(screen.getByRole('button', { name: 'Tạo tài khoản' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Đã tạo tài khoản newdev.');
  const create = fetchMock.mock.calls.find(([, init]) => init?.method === 'POST');
  expect(JSON.parse(String(create?.[1]?.body))).toEqual({ username: 'newdev', password: 'password-1', role: 'DEVELOPER' });
  // The password field is cleared immediately after submit.
  expect(screen.getByLabelText('Mật khẩu')).toHaveValue('');
});

it('validates username and password inline without calling the API', async () => {
  const fetchMock = stubFetch((url) => url.endsWith('/users') ? jsonResponse(200, accounts) : undefined);
  const user = userEvent.setup();
  render(<UsersPage />);
  await screen.findByText('developer');
  await user.type(screen.getByLabelText('Tên đăng nhập'), 'No Good!');
  await user.type(screen.getByLabelText('Mật khẩu'), 'short');
  await user.click(screen.getByRole('button', { name: 'Tạo tài khoản' }));
  expect(screen.getByText(/Chỉ dùng chữ thường/)).toBeInTheDocument();
  expect(screen.getByText('Mật khẩu cần ít nhất 8 ký tự.')).toBeInTheDocument();
  expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'POST')).toHaveLength(0);
});

it('maps a duplicate username to the username field', async () => {
  stubFetch((url, init) => {
    if (url.endsWith('/users') && init?.method === 'POST') return jsonResponse(409, { code: 'DUPLICATE', field: 'username' });
    if (url.endsWith('/users')) return jsonResponse(200, accounts);
    return undefined;
  });
  const user = userEvent.setup();
  render(<UsersPage />);
  await screen.findByText('developer');
  await user.type(screen.getByLabelText('Tên đăng nhập'), 'developer');
  await user.type(screen.getByLabelText('Mật khẩu'), 'password-1');
  await user.click(screen.getByRole('button', { name: 'Tạo tài khoản' }));
  expect(await screen.findByText('Tên đăng nhập đã tồn tại.')).toBeInTheDocument();
});

it('offers a retry when the list fails to load', async () => {
  let fail = true;
  stubFetch((url) => {
    if (url.endsWith('/users')) return fail ? jsonResponse(500, { code: 'INTERNAL_ERROR' }) : jsonResponse(200, accounts);
    return undefined;
  });
  const user = userEvent.setup();
  render(<UsersPage />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Không tải được danh sách người dùng.');
  fail = false;
  await user.click(screen.getByRole('button', { name: 'Thử lại' }));
  await waitFor(() => expect(screen.getByText('admin')).toBeInTheDocument());
});

function developerResetButton(): HTMLElement {
  const row = screen.getByText('developer').closest('.settings-table-row');
  if (!(row instanceof HTMLElement)) throw new Error('developer row not found');
  return within(row).getByRole('button', { name: 'Đặt lại mật khẩu' });
}

it('resets the password of an account from its row action', async () => {
  const fetchMock = stubFetch((url, init) => {
    if (url.includes('/reset-password') && init?.method === 'POST') return new Response(null, { status: 204 });
    if (url.endsWith('/users')) return jsonResponse(200, accounts);
    return undefined;
  });
  const user = userEvent.setup();
  render(<UsersPage />);
  await screen.findByText('developer');
  await user.click(developerResetButton());
  const dialog = await screen.findByRole('dialog', { name: 'Đặt lại mật khẩu của developer' });
  await user.type(within(dialog).getByLabelText('Mật khẩu mới'), 'reset-secret-1');
  await user.type(within(dialog).getByLabelText('Xác nhận mật khẩu mới'), 'reset-secret-1');
  await user.click(within(dialog).getByRole('button', { name: 'Đặt lại mật khẩu' }));
  expect(await screen.findByRole('status')).toHaveTextContent('Đã đặt lại mật khẩu của developer.');
  expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  const reset = fetchMock.mock.calls.find(([url]) => String(url).includes('/reset-password'));
  expect(reset?.[0]).toContain('/users/2/reset-password');
  expect(JSON.parse(String(reset?.[1]?.body))).toEqual({ newPassword: 'reset-secret-1', confirmPassword: 'reset-secret-1' });
});

it('validates the reset form inline without calling the API', async () => {
  const fetchMock = stubFetch((url) => url.endsWith('/users') ? jsonResponse(200, accounts) : undefined);
  const user = userEvent.setup();
  render(<UsersPage />);
  await screen.findByText('developer');
  await user.click(developerResetButton());
  const dialog = await screen.findByRole('dialog');
  await user.type(within(dialog).getByLabelText('Mật khẩu mới'), 'short');
  await user.type(within(dialog).getByLabelText('Xác nhận mật khẩu mới'), 'different-1');
  await user.click(within(dialog).getByRole('button', { name: 'Đặt lại mật khẩu' }));
  expect(within(dialog).getByText('Mật khẩu cần ít nhất 8 ký tự.')).toBeInTheDocument();
  expect(within(dialog).getByText('Xác nhận mật khẩu không khớp.')).toBeInTheDocument();
  expect(fetchMock.mock.calls.filter(([url]) => String(url).includes('/reset-password'))).toHaveLength(0);
});

it('reloads the list when the reset target no longer exists', async () => {
  stubFetch((url, init) => {
    if (url.includes('/reset-password') && init?.method === 'POST') return jsonResponse(404, { code: 'NOT_FOUND' });
    if (url.endsWith('/users')) return jsonResponse(200, accounts);
    return undefined;
  });
  const user = userEvent.setup();
  render(<UsersPage />);
  await screen.findByText('developer');
  await user.click(developerResetButton());
  const dialog = await screen.findByRole('dialog');
  await user.type(within(dialog).getByLabelText('Mật khẩu mới'), 'reset-secret-1');
  await user.type(within(dialog).getByLabelText('Xác nhận mật khẩu mới'), 'reset-secret-1');
  await user.click(within(dialog).getByRole('button', { name: 'Đặt lại mật khẩu' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Tài khoản không còn tồn tại');
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});
