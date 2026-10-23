import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { DependencyGraph } from '../../shared/ui/DependencyGraph';
import { getDeployment, type DeploymentDetail } from './api';
import { ApiError } from '../../shared/api/client';
import { DeploymentReview } from '../preview/DeploymentReview';

const actions: Record<string, string> = { DEPLOY: 'triển khai', UPDATE: 'cập nhật', REMOVE: 'gỡ bỏ' };
const deploymentStatuses: Record<string, string> = { SUCCEEDED: 'Thành công', FAILED: 'Thất bại', DEPLOYING: 'Đang triển khai', PROVISIONING: 'Đang tạo tài nguyên', PLANNING: 'Đang lập kế hoạch' };
const itemStatuses: Record<string, string> = { READY: 'Sẵn sàng', PENDING: 'Đang chờ', FAILED: 'Thất bại', SUCCEEDED: 'Thành công', RUNNING: 'Đang chạy', INTERRUPTED: 'Bị gián đoạn', ACTIVE: 'Đang hoạt động', PROVISIONING: 'Đang tạo tài nguyên', DEPLOYING: 'Đang triển khai', PLANNING: 'Đang lập kế hoạch', UNCONFIGURED: 'Chưa cấu hình', UNREFERENCED: 'Không còn được tham chiếu', APPLYING: 'Đang áp dụng', REMOVING: 'Đang gỡ bỏ', REMOVED: 'Đã gỡ bỏ' };
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);
// Progress values come from persisted strings: only known codes are shown, anything else is a generic label.
const known = (map: Record<string, string>, key: string, generic: string) => (Object.hasOwn(map, key) ? map[key] ?? generic : generic);
const redactedMarker = '***redacted***';
const stageLabels: Record<string, string> = {
  INTENT: 'Ghi nhận ý định', INIT: 'Khởi tạo module', PLANNING: 'Đang lập kế hoạch Terraform', PLANNED: 'Đã lập kế hoạch', APPLYING: 'Đang áp dụng', APPLIED: 'Đã áp dụng', OUTPUTS: 'Đọc đầu ra',
  READINESS: 'Kiểm tra sẵn sàng', READY: 'Sẵn sàng', NO_CHANGE: 'Không có thay đổi', CHECKING: 'Đang kiểm tra', DESTROYING: 'Đang hủy', VERIFYING: 'Đang xác minh', DESTROYED: 'Đã hủy', RECOVERY: 'Đang khôi phục',
};
const scopeLabels: Record<string, string> = { APPLICATION: 'Ứng dụng', ENVIRONMENT: 'Môi trường', WORKLOAD: 'Workload', SHARED: 'Dùng chung' };
const actionLabels: Record<string, string> = { CREATE: 'Tạo mới', UPDATE: 'Cập nhật', DESTROY: 'Hủy' };
const ownershipLabels: Record<string, string> = { CREATED_BY_RUN: 'Do lần chạy này tạo', PREEXISTING: 'Đã có trước', UPDATED_BY_RUN: 'Do lần chạy này cập nhật' };
const attemptStatuses: Record<string, string> = { RUNNING: 'Đang chạy', SUCCEEDED: 'Thành công', FAILED: 'Thất bại', INTERRUPTED: 'Bị gián đoạn', NEEDS_ATTENTION: 'Cần xử lý' };
const errorLabels: Record<string, string> = {
  DESTRUCTIVE_PLAN: 'Kế hoạch sẽ hủy tài nguyên nên bị chặn', OUTPUT_CONTRACT: 'Đầu ra module không đúng hợp đồng', NOT_READY: 'Tài nguyên chưa sẵn sàng', READINESS_UNAVAILABLE: 'Chưa kiểm tra được độ sẵn sàng',
  OUTCOME_UNKNOWN: 'Chưa rõ kết quả; cần kiểm tra trước khi thử lại', STATE_NOT_ACKNOWLEDGED: 'Trạng thái chưa được xác nhận lưu', STATE_BACKEND: 'Lỗi kho trạng thái', TRANSIENT: 'Lỗi tạm thời', AUTH: 'Lỗi xác thực với nhà cung cấp',
  INPUT: 'Đầu vào không hợp lệ', TIMEOUT: 'Quá thời gian', CANCELLED: 'Đã hủy', FAILED: 'Thất bại', PHYSICAL_RESOURCE_REMAINS: 'Vẫn còn tài nguyên vật lý', SECRET_UNAVAILABLE: 'Không đọc được bí mật cần thiết',
};

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
  const [stale, setStale] = useState(false);
  const [notFound, setNotFound] = useState(false);
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let cancelled = false; let timer: number | undefined; let hadDetail = false;
    setLoading(true); setError(''); setStale(false); setNotFound(false); setDetail(undefined);
    const refresh = async () => {
      let terminal = false;
      try {
        const value = await getDeployment(application.id, environment, deploymentId);
        if (cancelled) return;
        hadDetail = true; setDetail(value); setStale(false); setError('');
        terminal = value.deployment.status === 'SUCCEEDED' || value.deployment.status === 'FAILED';
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404 && !hadDetail) { setNotFound(true); terminal = true; }
        else if (hadDetail) setStale(true);
        else setError('Không thể tải chi tiết triển khai.');
      } finally { if (!cancelled) setLoading(false); }
      if (!cancelled && !terminal) timer = window.setTimeout(() => void refresh(), 2000);
    };
    void refresh();
    return () => { cancelled = true; if (timer) window.clearTimeout(timer); };
  }, [application.id, environment, deploymentId, attempt]);
  const reload = () => setAttempt((value) => value + 1);
  const failed = detail && (detail.deployment.status === 'FAILED' || Boolean(detail.deployment.failureReason));
  const rendered = detail?.workloads.flatMap((w) => w.manifestReview?.version === 1 && w.manifestReview.planHash === detail.planHash ? w.manifestReview.artifacts : [] ) ?? [];
  const review = detail?.review ? { ...detail.review, artifacts: [...detail.review.artifacts.filter((artifact) => !rendered.some((actual) => actual.id === artifact.id)), ...rendered] } : undefined;
  return <section className="page deployment-details-page">
    <button className="back-link" onClick={() => navigate({ name: 'deployments', applicationId: application.id, environment })}>← Lịch sử triển khai {environment}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>Chi tiết triển khai</h1><p>Kế hoạch và trạng thái thực thi đã lưu. Không làm mới cluster.</p></div><Button onClick={reload}>Tải lại</Button></header>
    {loading ? <p role="status" aria-busy="true">Đang tải chi tiết triển khai…</p> : notFound ? <section className="content-panel"><h2>Không tìm thấy triển khai</h2><p>Triển khai này không tồn tại trong ứng dụng và môi trường đã chọn.</p></section> : error && !detail ? <div role="alert" className="form-error">{error} <Button onClick={reload}>Thử lại</Button></div> : detail ? <>
      {stale ? <div className="form-error" role="alert">Trạng thái hiển thị là lần quan sát cuối; không thể làm mới. <Button onClick={reload}>Thử lại</Button></div> : null}
      <section className="content-panel"><p>{detail.workloads.some((w) => w.status === 'READY') ? 'Manifest của Workload sẵn sàng đã được áp dụng; các bản khác chỉ là ý định đã dựng.' : 'Manifest hiển thị là ý định đã dựng trước Apply; chưa có Workload nào được xác nhận sẵn sàng.'}</p><DeploymentReview review={review} progress={detail.nodeProgress} title="Bản đồ và trạng thái triển khai" /></section>
      <section className="content-panel"><div className="section-header"><div><h2>{detail.deployment.workloadId} · {lookup(actions, detail.deployment.action)}</h2><p>{detail.deployment.id}</p></div><Status tone={detail.deployment.status === 'SUCCEEDED' ? 'good' : detail.deployment.status === 'FAILED' ? 'draft' : 'neutral'}>{lookup(deploymentStatuses, detail.deployment.status)}</Status></div>
        {failed ? <div className="form-error" role="alert">Triển khai không hoàn tất. Xem trước các thay đổi rồi triển khai lại; liên hệ quản trị viên nếu lỗi lặp lại.</div> : null}
        <dl className="detail-grid"><div><dt>Bắt đầu</dt><dd>{date(detail.deployment.startedAt)}</dd></div><div><dt>Kết thúc</dt><dd>{date(detail.deployment.finishedAt)}</dd></div><div><dt>Người thực hiện</dt><dd>{detail.deployment.actorRef || 'Không rõ người thực hiện'}</dd></div><div><dt>Tập triển khai</dt><dd>{detail.deploymentSetId || '—'}</dd></div></dl></section>
      <section className="content-panel"><h2>Workload</h2>{detail.workloads?.length ? <div className="detail-list">{detail.workloads.map((item) => <div key={item.workloadId}><strong>{item.workloadId}</strong><span>{lookup(itemStatuses, item.status)}</span><small>{item.manifestDigest}</small></div>)}</div> : <p>Chưa ghi nhận trạng thái Workload nào.</p>}</section>
      <section className="content-panel"><h2>Tài nguyên</h2>{detail.resources?.length ? <div className="detail-list">{detail.resources.map((item) => <div key={item.descriptor}><strong>{item.resourceType || item.descriptor}</strong><span>{lookup(itemStatuses, item.status)} · {item.definitionKey || 'chưa có cấu hình tài nguyên'}</span><small>{item.descriptor}</small>{Object.keys(item.outputs ?? {}).length ? <small>Đầu ra: {Object.entries(item.outputs).map(([key, value]) => `${key}=${outputValue(value)}`).join(', ')}</small> : null}</div>)}</div> : <p>Chưa ghi nhận trạng thái tài nguyên nào.</p>}</section>
      <section className="content-panel" aria-label="Tiến trình tạo tài nguyên"><h2>Tiến trình tạo tài nguyên</h2>{detail.resourceProgress === undefined
        ? <p className="form-warning" role="note">Không đọc được tiến trình thực thi đã lưu của lần triển khai này nên chưa thể kết luận tài nguyên nào đã chạy. Bấm Tải lại để đọc lại.</p>
        : detail.resourceProgress.length === 0 ? <p>Chưa có bước thực thi Terraform nào được ghi cho lần triển khai này.</p>
          : <table className="plain-table"><thead><tr><th>Tài nguyên</th><th>Module</th><th>Lần</th><th>Hành động</th><th>Giai đoạn</th><th>Trạng thái</th><th>Quyền sở hữu</th><th>Cập nhật</th></tr></thead><tbody>{detail.resourceProgress.map((item) => <tr key={`${item.descriptor}|${item.scopeType}|${item.attempt}|${item.startedAt}`}>
            <td>{item.descriptor}<br /><small>{known(scopeLabels, item.scopeType, 'Phạm vi không rõ')}</small>{item.resourceInstanceId ? <><br /><a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'resource', applicationId: application.id, environment, resourceId: item.resourceInstanceId as string }); }}>Chi tiết tài nguyên</a></> : null}</td><td>{item.moduleId} · {item.moduleRevision}</td><td>{item.attempt}</td><td>{known(actionLabels, item.action, 'Hành động không rõ')}</td><td>{known(stageLabels, item.stage, 'Giai đoạn không rõ')}</td>
            <td>{known(attemptStatuses, item.status, 'Trạng thái không rõ')}{item.safeErrorCode ? <><br /><small>{known(errorLabels, item.safeErrorCode, 'Lỗi không xác định')}</small></> : null}</td><td>{known(ownershipLabels, item.ownership, 'Quyền sở hữu không rõ')}</td><td>{date(item.updatedAt)}</td></tr>)}</tbody></table>}
        <p><small>Chỉ hiển thị giai đoạn, trạng thái và mã lỗi an toàn đã lưu. Không có lệnh Terraform, đầu vào đã phân giải, bí mật, trạng thái hay đường dẫn khôi phục.</small></p></section>
      <section className="content-panel"><h2>Kế hoạch tạo tài nguyên</h2>{!detail.planHash ? <p>Không có kế hoạch tạo tài nguyên được lưu cho triển khai này.</p> : <><p>kế hoạch {detail.planHash.slice(0, 12)}</p>
        <DependencyGraph nodes={detail.graph?.nodes ?? []} edges={detail.graph?.edges ?? []} blocked={detail.blocked ?? []} definitionOf={(descriptor) => detail.matches?.[descriptor]?.definitionKey} />
        <h3>Các đợt tạo tài nguyên</h3>{detail.batches?.length ? <ol>{detail.batches.map((batch, index) => <li key={index}>{batch.join(', ')}</li>)}</ol> : <p>Chưa ghi nhận đợt tạo tài nguyên nào.</p>}</>}</section>
    </> : null}
  </section>;
}
