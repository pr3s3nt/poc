import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { expect, it, vi } from 'vitest';
import { SignInPage } from './SignInPage';
import { ApiError } from '../../shared/api/client';

it('validates required fields inline without calling the API', async () => {
  const onSuccess = vi.fn(async () => undefined);
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
  expect(screen.getByText('Enter your username.')).toBeInTheDocument();
  expect(screen.getByText('Enter your password.')).toBeInTheDocument();
  expect(onSuccess).not.toHaveBeenCalled();
});

it('disables the form while submitting and clears the password', async () => {
  let fail: (error: unknown) => void = () => undefined;
  const onSuccess = vi.fn(() => new Promise<void>((_, reject) => { fail = reject; }));
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  await user.type(screen.getByLabelText('Username'), 'developer');
  await user.type(screen.getByLabelText('Password'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
  expect(onSuccess).toHaveBeenCalledWith('developer', 'test-password');
  expect(screen.getByRole('button', { name: 'Signing in…' })).toBeDisabled();
  expect(screen.getByLabelText('Username')).toBeDisabled();
  expect(screen.getByLabelText('Password')).toHaveValue('');
  fail(new ApiError(401, 'invalid credentials'));
  expect(await screen.findByRole('alert')).toHaveTextContent('Username or password is incorrect.');
  expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();
});

it('shows a retryable API error and keeps the username', async () => {
  const onSuccess = vi.fn(async () => { throw new TypeError('Failed to fetch'); });
  const user = userEvent.setup();
  render(<SignInPage onSuccess={onSuccess} />);
  await user.type(screen.getByLabelText('Username'), 'developer');
  await user.type(screen.getByLabelText('Password'), 'test-password');
  await user.click(screen.getByRole('button', { name: 'Sign in' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('Could not sign in.');
  expect(screen.getByLabelText('Username')).toHaveValue('developer');
  expect(screen.getByRole('button', { name: 'Sign in' })).toBeEnabled();
});

it('shows the session-ended notice', () => {
  render(<SignInPage onSuccess={async () => undefined} notice="Your session ended. Sign in again to continue." />);
  expect(screen.getByRole('status')).toHaveTextContent('Your session ended. Sign in again to continue.');
});
