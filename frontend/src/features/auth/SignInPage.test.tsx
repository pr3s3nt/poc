import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { SignInPage } from './SignInPage';
import { ApiError } from '../../shared/api/client';

it('validates required fields inline without calling the API', async () => {
  const onSuccess = vi.fn(async () => undefined);
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  await user.click(screen.getByRole('button', { name: 'Đăng nhập' }));
  expect(screen.getByText('Nhập tên đăng nhập.')).toBeInTheDocument();
  expect(screen.getByText('Nhập mật khẩu.')).toBeInTheDocument();
  expect(onSuccess).not.toHaveBeenCalled();
});

it('disables the form while submitting and clears the password', async () => {
  let fail: (error: unknown) => void = () => undefined;
  const onSuccess = vi.fn(() => new Promise<void>((_, reject) => { fail = reject; }));
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  await user.type(screen.getByLabelText('Tên đăng nhập'), 'developer');
  await user.type(screen.getByLabelText('Mật khẩu'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Đăng nhập' }));
  expect(onSuccess).toHaveBeenCalledWith('developer', 'test-password');
  expect(screen.getByRole('button', { name: 'Đang đăng nhập…' })).toBeDisabled();
  expect(screen.getByLabelText('Tên đăng nhập')).toBeDisabled();
  expect(screen.getByLabelText('Mật khẩu')).toHaveValue('');
  fail(new ApiError(401, 'INVALID_CREDENTIALS'));
  expect(await screen.findByRole('alert')).toHaveTextContent('Tên đăng nhập hoặc mật khẩu không đúng.');
  expect(screen.getByRole('button', { name: 'Đăng nhập' })).toBeEnabled();
});

it('shows a retryable API error and keeps the username', async () => {
  const onSuccess = vi.fn(async () => { throw new TypeError('Failed to fetch'); });
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  await user.type(screen.getByLabelText('Tên đăng nhập'), 'developer');
  await user.type(screen.getByLabelText('Mật khẩu'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Đăng nhập' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể đăng nhập. Kiểm tra kết nối và thử lại.');
  expect(screen.getByLabelText('Tên đăng nhập')).toHaveValue('developer');
  expect(screen.getByRole('button', { name: 'Đăng nhập' })).toBeEnabled();
});

it('shows the session-ended notice', () => {
  render(<SignInPage onSuccess={async () => undefined} notice="Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục." />);
  expect(screen.getByRole('status')).toHaveTextContent('Phiên đăng nhập đã kết thúc. Đăng nhập lại để tiếp tục.');
});

it('masks the password input and never shows raw backend errors', async () => {
  const onSuccess = vi.fn(async () => { throw new ApiError(500, 'INTERNAL_ERROR'); });
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  expect(screen.getByLabelText('Mật khẩu')).toHaveAttribute('type', 'password');
  await user.type(screen.getByLabelText('Tên đăng nhập'), 'developer');
  await user.type(screen.getByLabelText('Mật khẩu'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Đăng nhập' }));
  expect(await screen.findByRole('alert')).not.toHaveTextContent('pq:');
  expect(screen.getByLabelText('Mật khẩu')).toHaveValue('');
});
