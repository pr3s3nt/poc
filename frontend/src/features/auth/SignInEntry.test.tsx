import { render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import userEvent from '@testing-library/user-event';
import { SignInEntry } from './SignInEntry';

afterEach(() => { vi.unstubAllGlobals(); window.history.replaceState({}, '', '/'); });
it('shows SSO without local credentials and uses a safe callback error', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ mode: 'oidc' })));
  window.history.replaceState({}, '', '/ui/?sso_error=1&error_description=private-provider-error');
  render(<SignInEntry onSuccess={vi.fn()} />);
  expect(await screen.findByRole('button', { name: 'Đăng nhập SSO' })).toBeEnabled();
  expect(screen.queryByLabelText('Mật khẩu')).not.toBeInTheDocument();
  expect(screen.getByRole('alert')).not.toHaveTextContent('private-provider-error');
});
it('fails closed on config errors and can retry', async () => {
  const fetch = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(Response.json({ mode: 'local' }));
  vi.stubGlobal('fetch', fetch);
  render(<SignInEntry onSuccess={vi.fn()} />);
  expect(await screen.findByRole('alert')).toHaveTextContent('Không thể tải cấu hình đăng nhập.');
  expect(screen.queryByLabelText('Mật khẩu')).not.toBeInTheDocument();
  await userEvent.click(screen.getByRole('button', { name: 'Thử lại' }));
  expect(await screen.findByLabelText('Mật khẩu')).toBeInTheDocument();
});
it('offers a local test disclosure alongside SSO only when enabled', async () => {
  vi.stubGlobal('fetch', vi.fn(async () => Response.json({ mode: 'oidc', localLoginAllowed: true })));
  const login = vi.fn(async () => undefined);
  render(<SignInEntry onSuccess={login} />);
  expect(await screen.findByRole('button', { name: 'Đăng nhập SSO' })).toBeEnabled();
  await userEvent.click(screen.getByText('Tài khoản test'));
  await userEvent.type(screen.getByLabelText('Tên đăng nhập'), 'local-admin');
  await userEvent.type(screen.getByLabelText('Mật khẩu'), 'private-test-password');
  await userEvent.click(screen.getByRole('button', { name: 'Đăng nhập local' }));
  expect(login).toHaveBeenCalledWith('local-admin', 'private-test-password');
});
