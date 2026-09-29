import type { PropsWithChildren } from 'react';
import { href, navigate } from './routes';
import { Button } from '../shared/ui/Button';

export function AppShell({ children, onSignOut, username, role }: PropsWithChildren<{ onSignOut(): void; username: string; role: string }>) {
  const isPlatformEngineer = role === 'PLATFORM_ENGINEER' || role === 'ADMIN';
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="wordmark" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>
          <span className="wordmark-mark">◆</span> Orchestrator
        </a>
        <nav className="nav" aria-label="Main navigation">
          <a className="nav-item nav-item-active" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>▦ <span>Applications</span></a>
          {isPlatformEngineer ? <a className="nav-item" href={href({ name: 'resource-types' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'resource-types' }); }}>◇ <span>Resource types</span></a> : null}
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
