import { useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';

export function ApplicationHomePage({ application }: { application: Application }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>('staging');
  const workloads = application.workloads[environment];
  return <section className="page application-home"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>← Applications</button>
    <header className="page-header application-header"><div><p className="eyebrow">Application</p><h1>{application.name}</h1><p>{endpointFor(application, 'production')}</p></div><Button disabled title="Available when UC-06 endpoint is ready">Open application ↗</Button></header>
    <div className="tabs" role="tablist"><button className={environment === 'staging' ? 'tab tab-active' : 'tab'} onClick={() => setEnvironment('staging')}>Staging<span>{endpointFor(application, 'staging')}</span></button><button className={environment === 'production' ? 'tab tab-active' : 'tab'} onClick={() => setEnvironment('production')}>Production<span>{endpointFor(application, 'production')}</span></button></div>
    <section className="content-panel"><div className="section-header"><div><h2>Workloads</h2><p>Services running in {environment}.</p></div><Button disabled title="Available when UC-05 is implemented">+ Add workload</Button></div>
      {workloads.length ? <div className="workload-table"><div className="table-head"><span>Name</span><span>Status</span><span>Actions</span></div>{workloads.map((workload) => <div className="table-row" key={workload.id}><span className="workload-name"><span className="workload-icon">◫</span>{workload.name}</span><Status tone={workload.status === 'Ready' ? 'good' : 'draft'}>{workload.status}</Status><span><Button tone="quiet" disabled title="Available when UC-05 is implemented">Edit</Button><Button tone="quiet" disabled title="Available when UC-07 is implemented">Delete</Button></span></div>)}</div> : <div className="section-empty">No workloads in this environment yet.</div>}
      <p className="feature-note">Workload editing is available in the next use case.</p>
    </section>
    <section className="content-panel"><div className="section-header"><div><h2>Recent deployments</h2><p>Deployment activity will appear here.</p></div><Button tone="quiet" disabled>View all</Button></div><div className="section-empty">No deployments yet.</div></section>
  </section>;
}
