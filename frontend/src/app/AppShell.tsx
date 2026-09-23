import type { PropsWithChildren } from 'react';
import { href, navigate } from './routes';
import { Button } from '../shared/ui/Button';

export function AppShell({ children, onSignOut }: PropsWithChildren<{ onSignOut(): void }>) {
  return (
    <div className="app-shell">
      <aside className="sidebar">
        <a className="wordmark" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>
          <span className="wordmark-mark">◆</span> Orchestrator
        </a>
        <nav className="nav" aria-label="Main navigation">
          <a className="nav-item nav-item-active" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>▦ <span>Applications</span></a>
          <span className="nav-item nav-item-disabled">◷ <span>Deployments</span><small>Coming next</small></span>
        </nav>
        <div className="sidebar-footer">
          <div className="user-chip"><span className="avatar">MN</span><span><strong>Minh Nguyen</strong><small>Developer</small></span></div>
          <Button tone="quiet" onClick={onSignOut}>Sign out</Button>
        </div>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
