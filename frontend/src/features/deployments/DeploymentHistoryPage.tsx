import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listDeployments, type Deployment, type DeploymentStatus } from './api';

const date = (value: string) => value ? new Date(value).toLocaleString() : '—';

export function DeploymentHistoryPage({ application, environment }: { application: Application; environment: EnvironmentKey }) {
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [filter, setFilter] = useState<'ALL' | DeploymentStatus>('ALL');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setLoading(true); setError('');
    setDeployments([]);
    listDeployments(application.id, environment, filter === 'ALL' ? undefined : filter).then((response) => { if (!cancelled) setDeployments(response.deployments); })
      .catch(() => { if (!cancelled) setError('Could not load deployment history.'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, filter, attempt]);
  return <section className="page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header"><div><p className="eyebrow">{environment} · Deployment history</p><h1>Deployments</h1><p>Persisted results for this Environment. No cluster refresh is performed.</p></div><Button onClick={() => setAttempt((value) => value + 1)}>Refresh</Button></header>
    <section className="content-panel"><div className="section-header"><h2>History</h2><label>Status <select aria-label="Filter deployment status" value={filter} onChange={(event) => setFilter(event.target.value as 'ALL' | DeploymentStatus)}><option value="ALL">All</option><option value="SUCCEEDED">Succeeded</option><option value="FAILED">Failed</option><option value="DEPLOYING">Deploying</option><option value="PROVISIONING">Provisioning</option><option value="PLANNING">Planning</option></select></label></div>
      {loading ? <p role="status" aria-busy="true">Loading deployments…</p> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : deployments.length ? <div className="deployment-list">{deployments.map((item) => <button className="deployment-entry" key={item.id} onClick={() => navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: item.id })}><span><strong>{item.workloadId}</strong> · {item.action.toLowerCase()}<small>{date(item.startedAt)} · {item.actorRef || 'unknown actor'}</small></span><Status tone={item.status === 'SUCCEEDED' ? 'good' : item.status === 'FAILED' ? 'draft' : 'neutral'}>{item.status}</Status></button>)}</div> : <p>{filter === 'ALL' ? 'No deployments in this Environment yet.' : `No ${filter.toLowerCase()} deployments.`}</p>}
    </section>
  </section>;
}
