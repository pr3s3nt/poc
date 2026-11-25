import { render, screen, within } from '@testing-library/react';
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
  expect(screen.getByRole('link', { name: 'Dịch vụ' })).toHaveAttribute('aria-current', 'page');
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
  expect(screen.getByRole('link', { name: 'Dịch vụ' })).toBeVisible();
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
  expect(screen.getByRole('link', { name: 'Cấu hình CD' })).toHaveAttribute('href', '/ui/platform/cd-providers');
  expect(screen.getByRole('link', { name: 'Cấu hình CD' })).toHaveAttribute('aria-current', 'page');
  expect(screen.getByRole('link', { name: 'Cấu hình Git' })).toHaveAttribute('href', '/ui/platform/settings');
  expect(screen.getByRole('link', { name: 'Người dùng' })).toHaveAttribute('href', '/ui/platform/users');
  expect(screen.getByRole('button', { name: 'Đổi mật khẩu' })).toBeVisible();
});

it('keeps Definition navigation and hides Catalog authoring entry', () => {
 render(<AppShell {...props}><div /></AppShell>);
 expect(screen.getByRole('link',{name:'Cấu hình tài nguyên'})).toBeVisible();
 expect(screen.queryByRole('link',{name:'Catalog'})).not.toBeInTheDocument();
});

it('keeps platform role navigation but hides local account controls for SSO admins', () => {
  render(<AppShell {...props} role="ADMIN" externalIdentity><p>Content</p></AppShell>);
  expect(screen.getByRole('link', { name: 'Loại tài nguyên' })).toBeInTheDocument();
  expect(screen.queryByRole('link', { name: 'Người dùng' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Đổi mật khẩu' })).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Đăng xuất' })).toBeInTheDocument();
});
it('allows personal password change but hides employee management for a local test admin', () => {
  render(<AppShell {...props} role="ADMIN" manageUsers={false}><p>Content</p></AppShell>);
  expect(screen.getByRole('button', { name: 'Đổi mật khẩu' })).toBeInTheDocument();
  expect(screen.queryByRole('link', { name: 'Người dùng' })).not.toBeInTheDocument();
});

it('opens Platform on child routes and preserves the approved keyboard navigation order', async () => {
  const user = userEvent.setup();
  const { rerender } = render(<AppShell {...props} role="ADMIN">Content</AppShell>);
  const platform = screen.getByRole('button', { name: 'Nền tảng' });
  platform.focus();
  await user.keyboard('{Enter}');
  expect(platform).toHaveAttribute('aria-expanded', 'false');
  expect(screen.queryByRole('link', { name: 'Loại tài nguyên' })).not.toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Hiệu quả nền tảng' })).toBeVisible();

  rerender(<AppShell {...props} role="ADMIN" activeRoute="resource-definitions">Content</AppShell>);
  expect(platform).toHaveAttribute('aria-expanded', 'true');
  const nav = within(screen.getByRole('navigation', { name: 'Điều hướng chính' }));
  expect(nav.getByRole('link', { name: 'Cấu hình tài nguyên' })).toHaveAttribute('aria-current', 'page');
  expect(nav.getAllByRole('link').map(link => link.textContent)).toEqual([
    'Dịch vụ', 'Loại tài nguyên', 'Cấu hình tài nguyên', 'Tài khoản hạ tầng',
    'Cấu hình Git', 'Cấu hình CD', 'Kho bí mật', 'Khởi tạo nền tảng',
    'Hiệu quả nền tảng', 'Người dùng', 'Tài liệu',
  ]);
  expect(nav.queryByText('Lịch sử triển khai')).not.toBeInTheDocument();
});
