import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listDeployments, type Deployment } from './api';

const actions: Record<string, string> = { DEPLOY: 'triển khai', UPDATE: 'cập nhật', REMOVE: 'gỡ bỏ' };
const statuses: Record<string, string> = { SUCCEEDED: 'Thành công', FAILED: 'Thất bại', PLANNING: 'Đang lập kế hoạch', PROVISIONING: 'Đang tạo tài nguyên', DEPLOYING: 'Đang triển khai' };
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
      .catch(() => { if (!cancelled) setError('Không thể tải các triển khai gần đây.'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [applicationId, environment, refreshKey, attempt]);
  return <section className="content-panel"><div className="section-header"><div><h2>Triển khai gần đây</h2><p>Kết quả mới nhất của {environment}.</p></div><Button onClick={() => navigate({ name: 'deployments', applicationId, environment })}>Xem tất cả</Button></div>
    {loading ? <p role="status" aria-busy="true">Đang tải triển khai…</p> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div> : deployments.length ? <div className="deployment-list">{deployments.map((deployment) => <button className="deployment-entry" key={deployment.id} onClick={() => navigate({ name: 'deployment', applicationId, environment, deploymentId: deployment.id })}><span><strong>{deployment.workloadId}</strong> · {actionLabel(deployment.action)}<small>{new Date(deployment.startedAt).toLocaleString()}</small></span><Status tone={deployment.status === 'SUCCEEDED' ? 'good' : deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{statusLabel(deployment.status)}</Status></button>)}</div> : <p>Chưa có triển khai nào trong {environment}.</p>}
  </section>;
}
