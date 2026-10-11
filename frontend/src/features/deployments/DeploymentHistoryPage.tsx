import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { listDeployments, type Deployment, type DeploymentStatus } from './api';

const actions: Record<string, string> = { DEPLOY: 'triển khai', UPDATE: 'cập nhật', REMOVE: 'gỡ bỏ' };
const statuses: Record<string, string> = { SUCCEEDED: 'Thành công', FAILED: 'Thất bại', DEPLOYING: 'Đang triển khai', PROVISIONING: 'Đang tạo tài nguyên', PLANNING: 'Đang lập kế hoạch' };
const filterOrder: DeploymentStatus[] = ['SUCCEEDED', 'FAILED', 'DEPLOYING', 'PROVISIONING', 'PLANNING'];
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);

const date = (value: string) => {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('vi-VN');
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
      .catch(() => { if (!cancelled) setError('Không thể tải lịch sử triển khai.'); })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, filter, attempt]);
  return <section className="page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header"><div><p className="eyebrow">{environment} · Lịch sử triển khai</p><h1>Lịch sử triển khai</h1><p>Kết quả đã lưu của môi trường này. Không làm mới cluster.</p></div><Button onClick={() => setAttempt((value) => value + 1)}>Tải lại</Button></header>
    <section className="content-panel"><div className="section-header"><h2>Các lần triển khai</h2><label>Trạng thái <select value={filter} onChange={(event) => setFilter(event.target.value as 'ALL' | DeploymentStatus)}><option value="ALL">Tất cả</option>{filterOrder.map((status) => <option key={status} value={status}>{statuses[status]}</option>)}</select></label></div>
      {loading ? <p role="status" aria-busy="true">Đang tải lịch sử triển khai…</p> : error ? <div role="alert" className="form-error">{error} <Button onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div> : deployments.length ? <div className="deployment-list">{deployments.map((item) => <button className="deployment-entry" key={item.id} onClick={() => navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: item.id })}><span><strong>{item.workloadId}</strong> · {lookup(actions, item.action)}<small>{date(item.startedAt)} · {item.actorRef || 'Không rõ người thực hiện'}</small></span><Status tone={item.status === 'SUCCEEDED' ? 'good' : item.status === 'FAILED' ? 'draft' : 'neutral'}>{lookup(statuses, item.status)}</Status></button>)}</div> : <p>{filter === 'ALL' ? 'Chưa có triển khai nào trong môi trường này.' : `Chưa có triển khai nào ở trạng thái “${statuses[filter]}”.`}</p>}
    </section>
  </section>;
}
