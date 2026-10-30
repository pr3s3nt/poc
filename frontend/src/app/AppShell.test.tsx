import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, expect, it, vi } from 'vitest';
import { AppShell } from './AppShell';

afterEach(() => { vi.restoreAllMocks(); window.localStorage.clear(); });
const props = { onSignOut: vi.fn(), username: 'platform-engineer', role: 'PLATFORM_ENGINEER', activeRoute: 'application' };

it('collapses by keyboard, preserves the mounted draft and keeps navigation and sign-out named', async () => {
  const user = userEvent.setup();
  const { container } = render(<AppShell {...props}><input aria-label="Draft" defaultValue="" /></AppShell>);
  const draft = screen.getByLabelText('Draft');
  await user.type(draft, 'keep this edit');
  screen.getByRole('button', { name: 'Thu gọn thanh điều hướng' }).focus();
  await user.keyboard('{Enter}');
  expect(container.firstChild).toHaveClass('app-shell-collapsed');
  expect(screen.getByRole('button', { name: 'Mở rộng thanh điều hướng' })).toHaveAttribute('aria-expanded', 'false');
  expect(screen.getByLabelText('Draft')).toBe(draft);
  expect(draft).toHaveValue('keep this edit');
  expect(screen.getByRole('link', { name: 'Ứng dụng' })).toHaveAttribute('aria-current', 'page');
  expect(screen.getByRole('link', { name: 'Loại tài nguyên' })).toHaveAttribute('title', 'Loại tài nguyên');
  expect(window.localStorage.getItem('orchestrator.sidebar.collapsed')).toBe('true');
  await user.click(screen.getByRole('button', { name: 'Mở rộng thanh điều hướng' }));
  expect(container.firstChild).not.toHaveClass('app-shell-collapsed');
  expect(draft).toHaveValue('keep this edit');
  await user.click(screen.getByRole('button', { name: 'Đăng xuất' }));
  expect(props.onSignOut).toHaveBeenCalledOnce();
});

it('restores the rail preference and preserves Developer navigation gates', () => {
  window.localStorage.setItem('orchestrator.sidebar.collapsed', 'true');
  const { container } = render(<AppShell {...props} role="DEVELOPER">Content</AppShell>);
  expect(container.firstChild).toHaveClass('app-shell-collapsed');
  expect(screen.getByRole('link', { name: 'Ứng dụng' })).toBeVisible();
  expect(screen.queryByRole('link', { name: 'Loại tài nguyên' })).not.toBeInTheDocument();
  expect(screen.queryByRole('link', { name: 'Kho bí mật' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Đăng xuất' })).toBeVisible();
});

it('keeps toggling usable when browser storage is blocked', async () => {
  vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('blocked'); });
  vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('blocked'); });
  const user = userEvent.setup();
  render(<AppShell {...props}>Content</AppShell>);
  await user.click(screen.getByRole('button', { name: 'Thu gọn thanh điều hướng' }));
  expect(screen.getByRole('button', { name: 'Mở rộng thanh điều hướng' })).toBeVisible();
});

it('keeps CD, platform Git and user administration navigation together after integration', () => {
  render(<AppShell {...props} role="ADMIN" activeRoute="cd-providers">Content</AppShell>);
  expect(screen.getByRole('link', { name: 'Cấu hình CD', exact: true })).toHaveAttribute('href', '/ui/platform/cd-providers');
  expect(screen.getByRole('link', { name: 'Cấu hình CD', exact: true })).toHaveAttribute('aria-current', 'page');
  expect(screen.getByRole('link', { name: 'Cài đặt nền tảng', exact: true })).toHaveAttribute('href', '/ui/platform/settings');
  expect(screen.getByRole('link', { name: 'Người dùng', exact: true })).toHaveAttribute('href', '/ui/platform/users');
  expect(screen.getByRole('button', { name: 'Đổi mật khẩu', exact: true })).toBeVisible();
});
