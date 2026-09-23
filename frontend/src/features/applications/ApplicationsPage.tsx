import type { Application } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { navigate } from '../../app/routes';

export function ApplicationsPage({ applications }: { applications: readonly Application[] }) {
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">Workspace</p><h1>Your applications</h1><p>Create an application, then deploy to staging or production.</p></div><Button tone="primary" onClick={() => navigate({ name: 'create-application' })}>+ Create application</Button></header>
    {applications.length === 0 ? <div className="empty-state"><div className="empty-icon">◇</div><h2>Start a new application</h2><p>Create an application to get staging and production environments.</p><Button tone="primary" onClick={() => navigate({ name: 'create-application' })}>+ Create application</Button></div> :
      <div className="application-grid">{applications.map((application) => <button className="application-card" key={application.id} onClick={() => navigate({ name: 'application', applicationId: application.id })}>
        <div className="card-title"><span className="application-icon">⌘</span><span><strong>{application.name}</strong><small>{endpointFor(application, 'production')}</small></span><span className="card-arrow">→</span></div>
        <div className="environment-summary"><span><small>Staging</small><Status tone="draft">Ready to configure</Status></span><span><small>Production</small><Status tone="draft">Ready to configure</Status></span></div>
      </button>)}</div>}
  </section>;
}
