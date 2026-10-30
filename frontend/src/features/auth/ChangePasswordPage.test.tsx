import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { ChangePasswordPage } from './ChangePasswordPage';

afterEach(() => vi.unstubAllGlobals());

function jsonResponse(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

function stubFetch(handler: (init?: RequestInit) => Response) {
  const fetchMock = vi.fn(async (_input: string | URL | Request, init?: RequestInit) => handler(init));
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

async function fillAndSubmit(user: ReturnType<typeof userEvent.setup>, current = 'old-secret-1', next = 'new-secret-1', confirm = 'new-secret-1') {
  await user.type(screen.getByLabelText('Mật khẩu hiện tại'), current);
  await user.type(screen.getByLabelText('Mật khẩu mới', { exact: true }), next);
  await user.type(screen.getByLabelText('Xác nhận mật khẩu mới'), confirm);
  await user.click(screen.getByRole('button', { name: 'Đổi mật khẩu' }));
}

it('validates the fields inline without calling the API', async () => {
  const fetchMock = stubFetch(() => jsonResponse(204, null));
  const user = userEvent.setup();
  render(<ChangePasswordPage onChanged={() => undefined} />);
  await user.click(screen.getByRole('button', { name: 'Đổi mật khẩu' }));
  expect(screen.getByText('Nhập mật khẩu hiện tại.')).toBeInTheDocument();
  expect(screen.getByText('Nhập mật khẩu mới.')).toBeInTheDocument();
  await user.type(screen.getByLabelText('Mật khẩu mới', { exact: true }), 'new-secret-1');
  await user.click(screen.getByRole('button', { name: 'Đổi mật khẩu' }));
  expect(screen.getByText('Xác nhận không khớp mật khẩu mới.')).toBeInTheDocument();
  expect(fetchMock).not.toHaveBeenCalled();
});

it('posts the change and ends the local session on success', async () => {
  const fetchMock = stubFetch(() => new Response(null, { status: 204 }));
  const onChanged = vi.fn();
  const user = userEvent.setup();
  render(<ChangePasswordPage onChanged={onChanged} />);
  await fillAndSubmit(user);
  expect(onChanged).toHaveBeenCalledTimes(1);
  const [, init] = fetchMock.mock.calls[0] ?? [];
  expect(JSON.parse(String(init?.body))).toEqual({ currentPassword: 'old-secret-1', newPassword: 'new-secret-1', confirmPassword: 'new-secret-1' });
});

it('maps a wrong current password to its field', async () => {
  stubFetch(() => jsonResponse(401, { code: 'INVALID_CREDENTIALS' }));
  const onChanged = vi.fn();
  const user = userEvent.setup();
  render(<ChangePasswordPage onChanged={onChanged} />);
  await fillAndSubmit(user);
  expect(await screen.findByText('Mật khẩu hiện tại không đúng.')).toBeInTheDocument();
  expect(onChanged).not.toHaveBeenCalled();
});

it('masks every password input and never shows raw backend errors', async () => {
  stubFetch(() => jsonResponse(500, { code: 'INTERNAL_ERROR' }));
  const user = userEvent.setup();
  render(<ChangePasswordPage onChanged={() => undefined} />);
  expect(screen.getByLabelText('Mật khẩu hiện tại')).toHaveAttribute('type', 'password');
  expect(screen.getByLabelText('Mật khẩu mới', { exact: true })).toHaveAttribute('type', 'password');
  expect(screen.getByLabelText('Xác nhận mật khẩu mới')).toHaveAttribute('type', 'password');
  await fillAndSubmit(user);
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể đổi mật khẩu.');
});
