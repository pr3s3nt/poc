import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listDeployments, type Deployment } from './api';
import { intlLocale, t } from '../../shared/i18n';

const actions: Record<string, string> = { DEPLOY: t('deployments.recentDeployments.deploy'), UPDATE: t('deployments.recentDeployments.update'), REMOVE: t('deployments.recentDeployments.remove') };
const statuses: Record<string, string> = { SUCCEEDED: t('deployments.recentDeployments.succeeded'), FAILED: t('deployments.recentDeployments.failed'), PLANNING: t('deployments.recentDeployments.planning'), PROVISIONING: t('deployments.recentDeployments.provisioningResources'), DEPLOYING: t('deployments.recentDeployments.deploying') };
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);
const actionLabel = (action: string) => lookup(actions, action);
const statusLabel = (status: string) => lookup(statuses, status);

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
      .catch(() => { if (!cancelled) setError(t('deployments.recentDeployments.couldNotLoadRecentDeployments')); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [applicationId, environment, refreshKey, attempt]);
  return <section className="content-panel"><div className="section-header"><div><h2>{t('deployments.recentDeployments.recentDeployments')}</h2><p>{t('deployments.recentDeployments.latestResultsOfEnvironment', { environment })}</p></div><Button onClick={() => navigate({ name: 'deployments', applicationId, environment })}>{t('deployments.recentDeployments.viewAll')}</Button></div>
    {loading ? <p role="status" aria-busy="true">{t('deployments.recentDeployments.loadingDeployments')}</p> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>{t('deployments.recentDeployments.retry')}</Button></div> : deployments.length ? <div className="deployment-list">{deployments.map((deployment) => <button className="deployment-entry" key={deployment.id} onClick={() => navigate({ name: 'deployment', applicationId, environment, deploymentId: deployment.id })}><span><strong>{deployment.workloadId}</strong> · {actionLabel(deployment.action)}<small>{new Date(deployment.startedAt).toLocaleString(intlLocale())}</small></span><Status tone={deployment.status === 'SUCCEEDED' ? 'good' : deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{statusLabel(deployment.status)}</Status></button>)}</div> : <p>{t('deployments.recentDeployments.noDeploymentsInEnvironmentYet', { environment })}</p>}
  </section>;
}
