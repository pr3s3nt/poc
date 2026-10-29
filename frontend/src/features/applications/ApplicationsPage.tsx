import type { Application } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { navigate } from '../../app/routes';
import { t } from '../../shared/i18n';

export type ApplicationsLoadState = 'loading' | 'ready' | 'error';

export function ApplicationsPage({ applications, state = 'ready', onRetry, notice }: { applications: readonly Application[]; state?: ApplicationsLoadState; onRetry?(): void; notice?: string }) {
  return <section className="page">
    {notice ? <div className="form-info" role="status">{notice}</div> : null}
    <header className="page-header"><div><p className="eyebrow">{t('applications.applications.workspace')}</p><h1>{t('applications.applications.yourApplications')}</h1><p>{t('applications.applications.createAnApplicationThenDeploy')}</p></div><Button tone="primary" onClick={() => navigate({ name: 'create-application' })}>{t('applications.applications.createApplication')}</Button></header>
    {state === 'loading' ? <div className="application-grid" role="status" aria-busy="true" aria-label={t('applications.applications.loadingApplications')}><div className="application-card-skeleton" /><div className="application-card-skeleton" /></div> :
      state === 'error' ? <div className="form-error" role="alert">{t('applications.applications.couldNotLoadTheApplication')}<Button onClick={onRetry}>{t('applications.applications.retry')}</Button></div> :
      applications.length === 0 ? <div className="empty-state"><div className="empty-icon">◇</div><h2>{t('applications.applications.startWithANewApplication')}</h2><p>{t('applications.applications.createAnApplicationToGet')}</p><Button tone="primary" onClick={() => navigate({ name: 'create-application' })}>{t('applications.applications.createApplication')}</Button></div> :
      <div className="application-grid">{applications.map((application) => <button className="application-card" key={application.id} onClick={() => navigate({ name: 'application', applicationId: application.id })}>
        <div className="card-title"><span className="application-icon">⌘</span><span><strong>{application.name}</strong><small>{endpointFor(application, 'production')}</small></span><span className="card-arrow">→</span></div>
        <div className="environment-summary">{application.availableEnvironments !== undefined && !application.availableEnvironments.some(key => key === 'staging' || key === 'production') ? <span><small>{application.availableEnvironments.join(', ') || t('applications.applications.noEnvironmentsYet')}</small><Status tone="neutral">{t('applications.applications.legacyUnsupportedEnvironment')}</Status></span> : (['staging', 'production'] as const).filter(key => application.availableEnvironments === undefined || application.availableEnvironments.includes(key)).map(key => <span key={key}><small>{key}</small><Status tone="draft">{t('applications.applications.readyToConfigure')}</Status></span>)}</div>
      </button>)}</div>}
  </section>;
}
