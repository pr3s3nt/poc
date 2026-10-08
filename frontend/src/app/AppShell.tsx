import type { PropsWithChildren } from 'react';
import { href, navigate } from './routes';
import { Button } from '../shared/ui/Button';

export function AppShell({ children, onSignOut, username, role, activeRoute }: PropsWithChildren<{ onSignOut(): void; username: string; role: string; activeRoute: string }>) {
  const isPlatformEngineer = role === 'PLATFORM_ENGINEER' || role === 'ADMIN';
  const navClass = (name: string) => `nav-item${activeRoute === name ? ' nav-item-active' : ''}`;
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="wordmark" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>
          <span className="wordmark-mark">◆</span> Orchestrator
        </a>
        <nav className="nav" aria-label="Main navigation">
          <a className={navClass('applications')} href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>▦ <span>Applications</span></a>
          {isPlatformEngineer ? <a className={navClass('resource-types')} href={href({ name: 'resource-types' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-types' }); }}>◇ <span>Resource types</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('resource-definitions')} href={href({ name: 'resource-definitions' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-definitions' }); }}>▣ <span>Resource definitions</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('connections')} href={href({ name: 'connections' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'connections' }); }}>⌘ <span>Connections</span></a> : null}
          {isPlatformEngineer ? <a className={navClass('secret-stores')} href={href({ name: 'secret-stores' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'secret-stores' }); }}>⚿ <span>Secret stores</span></a> : null}
          <span className="nav-item nav-item-disabled">◷ <span>Deployments</span><small>Coming next</small></span>
        </nav>
        <div className="sidebar-footer">
          <div className="user-chip"><span className="avatar">{username.slice(0, 2).toUpperCase()}</span><span><strong>{username}</strong><small>{isPlatformEngineer ? 'Platform Engineer' : 'Developer'}</small></span></div>
          <Button tone="quiet" onClick={onSignOut}>Sign out</Button>
        </div>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
