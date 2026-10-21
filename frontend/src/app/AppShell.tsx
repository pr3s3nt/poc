import type { PropsWithChildren } from 'react';
import { href, navigate } from './routes';
import { Button } from '../shared/ui/Button';

function roleLabel(role: string) {
  if (role === 'ADMIN') return 'Quản trị viên';
  if (role === 'PLATFORM_ENGINEER') return 'Kỹ sư nền tảng';
  return 'Nhà phát triển';
}

export function AppShell({ children, onSignOut, username, role, activeRoute }: PropsWithChildren<{ onSignOut(): void; username: string; role: string; activeRoute: string }>) {
  const isPlatformEngineer = role === 'PLATFORM_ENGINEER' || role === 'ADMIN';
  const navClass = (name: string) => `nav-item${activeRoute === name ? ' nav-item-active' : ''}`;
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="wordmark wordmark-light" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>
          <span className="wordmark-mark">◆</span> Orchestrator
        </a>
        <nav className="nav" aria-label="Điều hướng chính">
          <a className={navClass('applications')} href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>▦ <span>Ứng dụng</span></a>
          {isPlatformEngineer ? <a className={navClass('resource-types')} href={href({ name: 'resource-types' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-types' }); }}>◇ <span>Loại tài nguyên</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('resource-definitions')} href={href({ name: 'resource-definitions' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-definitions' }); }}>▣ <span>Cấu hình tài nguyên</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('rendering-templates')} href={href({ name: 'rendering-templates' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'rendering-templates' }); }}>▤ <span>Mẫu dựng ứng dụng</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('driver-accounts')} href={href({ name: 'driver-accounts' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'driver-accounts' }); }}>⌬ <span>Tài khoản hạ tầng</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('connections')} href={href({ name: 'connections' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'connections' }); }}>⌘ <span>Kết nối</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('secret-stores')} href={href({ name: 'secret-stores' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'secret-stores' }); }}>⚿ <span>Kho bí mật</span></a> : null}
          <span className="nav-item nav-item-disabled">◷ <span>Lịch sử triển khai</span><small>Sắp có</small></span>
        </nav>
        <div className="sidebar-footer">
          <div className="user-chip"><span className="avatar">{username.slice(0, 2).toUpperCase()}</span><span><strong>{username}</strong><small>{roleLabel(role)}</small></span></div>
          <Button tone="quiet" onClick={onSignOut}>Đăng xuất</Button>
        </div>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
