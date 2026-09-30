import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listDeployments, type Deployment } from './api';

export function RecentDeployments({ applicationId, environment, refreshKey }: { applicationId: string; environment: EnvironmentKey; refreshKey?: string }) {
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setLoading(true); setError('');
    setDeployments([]);
    listDeployments(applicationId, environment).then((result) => { if (!cancelled) setDeployments(result.deployments.slice(0, 5)); })
      .catch(() => { if (!cancelled) setError('Could not load recent deployments.'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [applicationId, environment, refreshKey, attempt]);
  return <section className="content-panel"><div className="section-header"><div><h2>Recent deployments</h2><p>Latest results for {environment}.</p></div><Button onClick={() => navigate({ name: 'deployments', applicationId, environment })}>View all</Button></div>
    {loading ? <p role="status" aria-busy="true">Loading deployments…</p> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : deployments.length ? <div className="deployment-list">{deployments.map((deployment) => <button className="deployment-entry" key={deployment.id} onClick={() => navigate({ name: 'deployment', applicationId, environment, deploymentId: deployment.id })}><span><strong>{deployment.workloadId}</strong> · {deployment.action.toLowerCase()}<small>{new Date(deployment.startedAt).toLocaleString()}</small></span><Status tone={deployment.status === 'SUCCEEDED' ? 'good' : deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{deployment.status}</Status></button>)}</div> : <p>No deployments yet in {environment}.</p>}
  </section>;
}
