import { useState, type PropsWithChildren, type ReactNode } from 'react';
import { href, navigate } from './routes';
import { Button } from '../shared/ui/Button';
import { LanguageSelect } from '../shared/ui/LanguageSelect';
import { t } from '../shared/i18n';

function roleLabel(role: string) {
  if (role === 'ADMIN') return t('app.appShell.administrator');
  if (role === 'PLATFORM_ENGINEER') return t('app.appShell.platformEngineer');
  return t('app.appShell.developer');
}

function NavigationIcon({ name }: { name: string }) {
  const paths: Record<string, ReactNode> = {
    applications: <><rect x="3" y="3" width="5" height="5" rx="1" /><rect x="12" y="3" width="5" height="5" rx="1" /><rect x="3" y="12" width="5" height="5" rx="1" /><rect x="12" y="12" width="5" height="5" rx="1" /></>,
    'resource-types': <><path d="m10 2 7 4v8l-7 4-7-4V6l7-4Zm-7 4 7 4 7-4m-7 4v8" /></>,
    'resource-definitions': <><path d="m2 6 8-4 8 4-8 4-8-4Zm0 4 8 4 8-4M2 14l8 4 8-4" /></>,
    'rendering-templates': <><rect x="3" y="2" width="14" height="16" rx="2" /><path d="M3 6h14m-9 4-2 2 2 2m4-4 2 2-2 2" /></>,
    'driver-accounts': <><path d="m10 2 7 3v5c0 4-4 7-7 8-3-1-7-4-7-8V5l7-3Z" /><circle cx="10" cy="8" r="2" /><path d="M6 14c1-4 7-4 8 0" /></>,
    'platform-settings': <><circle cx="10" cy="10" r="3" /><path d="M10 2v3m0 10v3M2 10h3m10 0h3M4 4l2 2m8 8 2 2M4 16l2-2M14 6l2-2" /></>,
    'secret-stores': <><rect x="4" y="8" width="12" height="10" rx="2" /><path d="M6 8V6a4 4 0 0 1 8 0v2m-4 4v2" /></>,
    users: <><circle cx="7" cy="6" r="2.5" /><circle cx="14" cy="7" r="2" /><path d="M2 16c1-4 9-4 10 0" /><path d="M11 15c.8-3 6.6-3 7.4 0" /></>,
    effectiveness: <><path d="M3 17h14M5 14V9m4 5V5m4 9v-3m4 3V7" /></>,
    history: <><circle cx="10" cy="10" r="7" /><path d="M10 6v4l3 2" /></>,
  };
  return <svg viewBox="0 0 20 20" aria-hidden="true">{paths[name]}</svg>;
}

export function AppShell({ children, onSignOut, username, role, activeRoute }: PropsWithChildren<{ onSignOut(): void; username: string; role: string; activeRoute: string }>) {
  const [collapsed, setCollapsed] = useState(() => {
    try { return window.localStorage.getItem('orchestrator.sidebar.collapsed') === 'true'; }
    catch { return false; }
  });
  function toggleSidebar() {
    const next = !collapsed;
    setCollapsed(next);
    try { window.localStorage.setItem('orchestrator.sidebar.collapsed', String(next)); }
    catch { /* Navigation still works when local storage is unavailable. */ }
  }
  const isPlatformEngineer = role === 'PLATFORM_ENGINEER' || role === 'ADMIN';
  const isAdmin = role === 'ADMIN';
  const activeNav = ['application', 'create-application', 'settings', 'workload', 'score-preview', 'deployments', 'deployment', 'resource', 'observability'].includes(activeRoute) ? 'applications' : activeRoute;
  const navClass = (name: string) => `nav-item${activeNav === name ? ' nav-item-active' : ''}`;
  const links = [
    { route: 'applications', label: t('app.appShell.applications'), tone: 'blue', platform: false, admin: false },
    { route: 'resource-types', label: t('app.appShell.resourceTypes'), tone: 'teal', platform: true, admin: false },
    { route: 'resource-definitions', label: t('app.appShell.resourceDefinitions'), tone: 'violet', platform: true, admin: false },
    { route: 'rendering-templates', label: t('app.appShell.renderingTemplates'), tone: 'amber', platform: true, admin: false },
    { route: 'driver-accounts', label: t('app.appShell.infrastructureAccounts'), tone: 'blue', platform: true, admin: false },
    { route: 'platform-settings', label: t('platform.git.title'), tone: 'blue', platform: true, admin: false },
    { route: 'secret-stores', label: t('app.appShell.secretStores'), tone: 'rose', platform: true, admin: false },
    { route: 'effectiveness', label: t('app.appShell.platformEffectiveness'), tone: 'violet', platform: true, admin: false },
    { route: 'users', label: t('app.appShell.users'), tone: 'teal', platform: false, admin: true },
  ] as const;
  return (
    <div className={`app-shell${collapsed ? ' app-shell-collapsed' : ''}`}>
      <aside className="sidebar">
        <a className="wordmark wordmark-light" aria-label="Orchestrator" title="Orchestrator" href={href({ name: 'applications' })} onClick={(event) => { event.preventDefault(); navigate({ name: 'applications' }); }}>
          <span className="wordmark-mark" aria-hidden="true">◆</span><span className="sidebar-label">Orchestrator</span>
        </a>
        <button type="button" className="sidebar-toggle" onClick={toggleSidebar} aria-expanded={!collapsed} aria-controls="primary-navigation" aria-label={collapsed ? t('app.appShell.expandNavigation') : t('app.appShell.collapseNavigation')} title={collapsed ? t('app.appShell.expandNavigation') : t('app.appShell.collapseNavigation')}>
          <svg viewBox="0 0 20 20" aria-hidden="true"><rect x="2" y="3" width="16" height="14" rx="2" /><path d="M7 3v14" /><path d={collapsed ? 'm10 7 3 3-3 3' : 'm14 7-3 3 3 3'} /></svg><span className="sidebar-label">{t('app.appShell.collapseSidebar')}</span>
        </button>
        <nav className="nav" id="primary-navigation" aria-label={t('app.appShell.mainNavigation')}>
          {links.filter((item) => (!item.platform || isPlatformEngineer) && (!item.admin || isAdmin)).map((item) => <a key={item.route} className={navClass(item.route)} aria-current={activeNav === item.route ? 'page' : undefined} title={item.label} href={href({ name: item.route })} onClick={(event) => { event.preventDefault(); navigate({ name: item.route }); }}><span className="nav-icon" data-tone={item.tone} aria-hidden="true"><NavigationIcon name={item.route} /></span><span className="sidebar-label">{item.label}</span></a>)}
          <span className="nav-item nav-item-disabled" aria-disabled="true" aria-label={t('app.appShell.deploymentHistoryComingSoon')} title={t('app.appShell.deploymentHistoryComingSoon')}><span className="nav-icon" aria-hidden="true"><NavigationIcon name="history" /></span><span className="sidebar-label">{t('app.appShell.deploymentHistory')}</span><small className="sidebar-label">{t('app.appShell.comingSoon')}</small></span>
        </nav>
        <div className="sidebar-footer">
          <div className="user-chip" aria-label={`${username} · ${roleLabel(role)}`} title={`${username} · ${roleLabel(role)}`}><span className="avatar" aria-hidden="true">{username.slice(0, 2).toUpperCase()}</span><span className="sidebar-label"><strong>{username}</strong><small>{roleLabel(role)}</small></span></div>
          <Button tone="quiet" onClick={() => navigate({ name: 'account' })} aria-label={t('app.appShell.changePassword')} title={t('app.appShell.changePassword')}><svg className="sign-out-icon" viewBox="0 0 20 20" aria-hidden="true"><circle cx="7" cy="10" r="3" /><path d="M10 10h8m-3-2.5v5m-3-5v3" /></svg><span className="sidebar-label">{t('app.appShell.changePassword')}</span></Button>
          <Button tone="quiet" onClick={onSignOut} aria-label={t('app.appShell.signOut')} title={t('app.appShell.signOut')}><svg className="sign-out-icon" viewBox="0 0 20 20" aria-hidden="true"><path d="M8 3H3v14h5m4-11 4 4-4 4m-5-4h9" /></svg><span className="sidebar-label">{t('app.appShell.signOut')}</span></Button>
          <LanguageSelect />
        </div>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
