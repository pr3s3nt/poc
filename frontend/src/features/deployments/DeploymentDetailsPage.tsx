import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { getDeployment, type DeploymentDetail } from './api';
import { ApiError } from '../../shared/api/client';

const actions: Record<string, string> = { DEPLOY: 'triển khai', UPDATE: 'cập nhật', REMOVE: 'gỡ bỏ' };
const deploymentStatuses: Record<string, string> = { SUCCEEDED: 'Thành công', FAILED: 'Thất bại', DEPLOYING: 'Đang triển khai', PROVISIONING: 'Đang tạo tài nguyên', PLANNING: 'Đang lập kế hoạch' };
const itemStatuses: Record<string, string> = { READY: 'Sẵn sàng', PENDING: 'Đang chờ', FAILED: 'Thất bại', SUCCEEDED: 'Thành công', RUNNING: 'Đang chạy', INTERRUPTED: 'Bị gián đoạn', ACTIVE: 'Đang hoạt động', PROVISIONING: 'Đang tạo tài nguyên', DEPLOYING: 'Đang triển khai', PLANNING: 'Đang lập kế hoạch', UNCONFIGURED: 'Chưa cấu hình', UNREFERENCED: 'Không còn được tham chiếu', APPLYING: 'Đang áp dụng', REMOVING: 'Đang gỡ bỏ', REMOVED: 'Đã gỡ bỏ' };
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);
const redactedMarker = '***redacted***';

const date = (value?: string) => {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString('vi-VN');
};

// Strings and numbers are shown as persisted; the backend marker never becomes data and
// nested values are never dumped, because they may carry secrets.
const outputValue = (value: unknown) => {
  if (value === redactedMarker) return 'Đã che';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  return '[giá trị đã ẩn]';
};

export function DeploymentDetailsPage({ application, environment, deploymentId }: { application: Application; environment: EnvironmentKey; deploymentId: string }) {
  const [detail, setDetail] = useState<DeploymentDetail>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notFound, setNotFound] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setLoading(true); setError(''); setNotFound(false); setDetail(undefined);
    getDeployment(application.id, environment, deploymentId).then((value) => { if (!cancelled) setDetail(value); })
      .catch((err: unknown) => {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) setNotFound(true);
        else setError('Không thể tải chi tiết triển khai.');
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, deploymentId, attempt]);
  const reload = () => setAttempt((value) => value + 1);
  const failed = detail && (detail.deployment.status === 'FAILED' || Boolean(detail.deployment.failureReason));
  return <section className="page">
    <button className="back-link" onClick={() => navigate({ name: 'deployments', applicationId: application.id, environment })}>← Lịch sử triển khai {environment}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>Chi tiết triển khai</h1><p>Kế hoạch và trạng thái thực thi đã lưu. Không làm mới cluster.</p></div><Button onClick={reload}>Tải lại</Button></header>
    {loading ? <p role="status" aria-busy="true">Đang tải chi tiết triển khai…</p> : notFound ? <section className="content-panel"><h2>Không tìm thấy triển khai</h2><p>Triển khai này không tồn tại trong ứng dụng và môi trường đã chọn.</p></section> : error ? <div role="alert" className="form-error">{error} <Button onClick={reload}>Thử lại</Button></div> : detail ? <>
      <section className="content-panel"><div className="section-header"><div><h2>{detail.deployment.workloadId} · {lookup(actions, detail.deployment.action)}</h2><p>{detail.deployment.id}</p></div><Status tone={detail.deployment.status === 'SUCCEEDED' ? 'good' : detail.deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{lookup(deploymentStatuses, detail.deployment.status)}</Status></div>
        {failed ? <div className="form-error" role="alert">Triển khai không hoàn tất. Xem trước các thay đổi rồi triển khai lại; liên hệ quản trị viên nếu lỗi lặp lại.</div> : null}
        <dl className="detail-grid"><div><dt>Bắt đầu</dt><dd>{date(detail.deployment.startedAt)}</dd></div><div><dt>Kết thúc</dt><dd>{date(detail.deployment.finishedAt)}</dd></div><div><dt>Người thực hiện</dt><dd>{detail.deployment.actorRef || 'Không rõ người thực hiện'}</dd></div><div><dt>Tập triển khai</dt><dd>{detail.deploymentSetId || '—'}</dd></div></dl></section>
      <section className="content-panel"><h2>Workload</h2>{detail.workloads?.length ? <div className="detail-list">{detail.workloads.map((item) => <div key={item.workloadId}><strong>{item.workloadId}</strong><span>{lookup(itemStatuses, item.status)}</span><small>{item.manifestDigest}</small></div>)}</div> : <p>Chưa ghi nhận trạng thái Workload nào.</p>}</section>
      <section className="content-panel"><h2>Tài nguyên</h2>{detail.resources?.length ? <div className="detail-list">{detail.resources.map((item) => <div key={item.descriptor}><strong>{item.resourceType || item.descriptor}</strong><span>{lookup(itemStatuses, item.status)} · {item.definitionKey || 'chưa có cấu hình tài nguyên'}</span><small>{item.descriptor}</small>{Object.keys(item.outputs ?? {}).length ? <small>Đầu ra: {Object.entries(item.outputs).map(([key, value]) => `${key}=${outputValue(value)}`).join(', ')}</small> : null}</div>)}</div> : <p>Chưa ghi nhận trạng thái tài nguyên nào.</p>}</section>
      <section className="content-panel"><h2>Kế hoạch tạo tài nguyên</h2>{!detail.planHash ? <p>Không có kế hoạch tạo tài nguyên được lưu cho triển khai này.</p> : <><p>{detail.graph?.nodes?.length ?? 0} nút đồ thị · {detail.graph?.edges?.length ?? 0} phụ thuộc · kế hoạch {detail.planHash.slice(0, 12)}</p>
        {detail.graph?.nodes?.length ? <><h3>Nút</h3><ul>{detail.graph.nodes.map((node) => <li key={node.descriptor}>{node.descriptor} · {node.kind}{detail.matches?.[node.descriptor]?.definitionKey ? ` · ${detail.matches?.[node.descriptor]?.definitionKey}` : ''}</li>)}</ul></> : null}
        {detail.graph?.edges?.length ? <><h3>Phụ thuộc</h3><ul>{detail.graph.edges.map((edge, index) => <li key={`${edge.consumer}-${edge.provider}-${index}`}>{edge.consumer} → {edge.provider}</li>)}</ul></> : null}
        <h3>Các đợt tạo tài nguyên</h3>{detail.batches?.length ? <ol>{detail.batches.map((batch, index) => <li key={index}>{batch.join(', ')}</li>)}</ol> : <p>Chưa ghi nhận đợt tạo tài nguyên nào.</p>}</>}</section>
    </> : null}
  </section>;
}
