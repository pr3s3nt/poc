import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listDeployments, type Deployment, type DeploymentStatus } from './api';
import { intlLocale, t } from '../../shared/i18n';

const actions: Record<string, string> = { DEPLOY: t('deployments.deploymentHistory.deploy'), UPDATE: t('deployments.deploymentHistory.update'), REMOVE: t('deployments.deploymentHistory.remove') };
const statuses: Record<string, string> = { SUCCEEDED: t('deployments.deploymentHistory.succeeded'), FAILED: t('deployments.deploymentHistory.failed'), DEPLOYING: t('deployments.deploymentHistory.deploying'), PROVISIONING: t('deployments.deploymentHistory.provisioningResources'), PLANNING: t('deployments.deploymentHistory.planning') };
const filterOrder: DeploymentStatus[] = ['SUCCEEDED', 'FAILED', 'DEPLOYING', 'PROVISIONING', 'PLANNING'];
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);

const date = (value: string) => {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString(intlLocale());
};

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
      .catch(() => { if (!cancelled) setError(t('deployments.deploymentHistory.couldNotLoadTheDeployment')); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, filter, attempt]);
  return <section className="page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header"><div><p className="eyebrow">{t('deployments.deploymentHistory.environmentDeploymentHistory', { environment })}</p><h1>{t('deployments.deploymentHistory.deploymentHistory')}</h1><p>{t('deployments.deploymentHistory.savedResultsOfThisEnvironment')}</p></div><Button onClick={() => setAttempt((value) => value + 1)}>{t('deployments.deploymentHistory.reload')}</Button></header>
    <section className="content-panel"><div className="section-header"><h2>{t('deployments.deploymentHistory.deployments')}</h2><label>{t('deployments.deploymentHistory.status')}{' '} <select value={filter} onChange={(event) => setFilter(event.target.value as 'ALL' | DeploymentStatus)}><option value="ALL">{t('deployments.deploymentHistory.all')}</option>{filterOrder.map((status) => <option key={status} value={status}>{statuses[status]}</option>)}</select></label></div>
      {loading ? <p role="status" aria-busy="true">{t('deployments.deploymentHistory.loadingDeploymentHistory')}</p> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>{t('deployments.deploymentHistory.retry')}</Button></div> : deployments.length ? <div className="deployment-list">{deployments.map((item) => <button className="deployment-entry" key={item.id} onClick={() => navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: item.id })}><span><strong>{item.workloadId}</strong> · {lookup(actions, item.action)}<small>{date(item.startedAt)} · {item.actorRef || t('deployments.deploymentHistory.unknownActor')}</small></span><Status tone={item.status === 'SUCCEEDED' ? 'good' : item.status === 'FAILED' ? 'draft' : 'neutral'}>{lookup(statuses, item.status)}</Status></button>)}</div> : <p>{filter === 'ALL' ? t('deployments.deploymentHistory.noDeploymentsInThisEnvironment') : t('deployments.deploymentHistory.noDeploymentsWithStatusValue', { value: statuses[filter] })}</p>}
    </section>
  </section>;
}
