import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { ApplicationTarget } from '../../shared/ui/ApplicationTarget';
import { ResourceIntents, rendererText } from '../preview/ResourceIntents';
import { Placements } from '../preview/Placements';
import { deleteWorkload, deployChanges, getWorkloads, previewChanges, undoWorkloadDelete, type DeployReport, type PendingPreview, type WorkloadList } from '../workloads/api';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { RecentDeployments } from '../deployments/RecentDeployments';
import { EnvironmentResources } from '../resources/EnvironmentResources';
import { correlateReviewProgress } from '../preview/reviewObservation';
import { newReviewRequestId } from '../preview/reviewRequestId';
import { DeploymentReview, type DeploymentReviewData } from '../preview/DeploymentReview';
import { getDeployment, listDeployments, listReviewOperations, type DeploymentDetail, type ReviewOperation } from '../deployments/api';

const actions: Record<string, string> = { DEPLOY: 'triển khai', UPDATE: 'cập nhật', REMOVE: 'gỡ bỏ' };
const reports: Record<string, string> = { SUCCEEDED: 'thành công', PARTIAL: 'một phần', FAILED: 'thất bại' };
const results: Record<string, string> = { SUCCEEDED: 'thành công', FAILED: 'thất bại', SKIPPED: 'bỏ qua' };
// Own-key lookup: inherited keys such as "constructor" must never resolve.
const lookup = (map: Record<string, string>, key: string) => (Object.hasOwn(map, key) ? map[key] ?? key : key);
const actionLabel = (action: string) => lookup(actions, action);
const reportLabel = (status: string) => lookup(reports, status);
const resultLabel = (status: string) => lookup(results, status);

// Raw API/provider errors may carry values that must not be shown; every
// failure uses a fixed Vietnamese message chosen by the attempted action.
function failureMessage(kind: Busy): string {
  if (kind === 'preview') return 'Không thể xem trước thay đổi. Hãy thử lại.';
  if (kind === 'deploy') return 'Không thể triển khai. Hãy xem trước lại để kiểm tra trạng thái trước khi thử lại.';
  if (kind.startsWith('undo:')) return 'Không thể hoàn tác việc xóa. Hãy thử lại.';
  return 'Không thể đánh dấu xóa Workload. Hãy thử lại.';
}

// One mutation or read that must not overlap another (UC-07 UI states).
type Busy = '' | 'preview' | 'deploy' | `delete:${string}` | `undo:${string}`;

export function ApplicationHomePage({ application, initialEnvironment = 'staging', created = false, onTargetChange }: { application: Application; initialEnvironment?: EnvironmentKey; created?: boolean; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>(initialEnvironment);
  // Draft Delete/Undo are Environment writes: held while an operation owns it.
  const environmentBusy = useEnvironmentBusy(application, environment, onTargetChange);
  const [data, setData] = useState<WorkloadList>();
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState(true);
  const [preview, setPreview] = useState<PendingPreview>();
  const [busy, setBusy] = useState<Busy>('');
  const [deployReport, setDeployReport] = useState<DeployReport>();
  const [reviewRequestId, setReviewRequestId] = useState('');
  const [activeReview, setActiveReview] = useState<DeploymentReviewData>();
  const [observed, setObserved] = useState<DeploymentDetail[]>([]);
  const [reviewOperation, setReviewOperation] = useState<ReviewOperation>();
  const [observationStale, setObservationStale] = useState(false);
  const [reload, setReload] = useState(0);
  // Bumped on every Environment switch: a response started for an earlier
  // scope is ignored instead of being shown under the new Environment.
  const scope = useRef(0);

  useEffect(() => {
    const current = ++scope.current;
    setData(undefined); setPreview(undefined); setDeployReport(undefined); setActiveReview(undefined); setReviewRequestId(''); setObserved([]); setReviewOperation(undefined); setObservationStale(false); setLoading(true); setError(''); setNotice(''); setBusy('');
    getWorkloads(application.id, environment)
      .then((result) => { if (current === scope.current) setData(result); })
      .catch(() => { if (current === scope.current) setError(`Không thể tải danh sách Workload của ${environment}.`); })
      .finally(() => { if (current === scope.current) setLoading(false); });
  }, [application.id, environment]);

  useEffect(() => {
    if (!reviewRequestId) return;
    let cancelled = false; let timer: number | undefined;
    const refresh = async () => {
      let terminal = false;
      try {
        const [list, operations] = await Promise.all([listDeployments(application.id, environment, undefined, reviewRequestId), listReviewOperations(application.id, environment, reviewRequestId)]);
        const exact = await Promise.all(list.deployments.map((entry) => getDeployment(application.id, environment, entry.id)));
        if (cancelled) return;
        const operation = operations.operations.find((entry) => entry.reviewRequestId === reviewRequestId);
        setObserved(exact); setReviewOperation(operation); setObservationStale(false);
        terminal = Boolean(operation && ['SUCCEEDED', 'FAILED', 'RECOVERED'].includes(operation.status) && exact.every((entry) => ['SUCCEEDED', 'FAILED'].includes(entry.deployment.status)));
      } catch {
        if (cancelled) return;
        setObservationStale(true);
      }
      if (!cancelled && !terminal) timer = window.setTimeout(() => void refresh(), 2000);
    };
    void refresh();
    return () => { cancelled = true; if (timer) window.clearTimeout(timer); };
  }, [application.id, environment, reviewRequestId]);

  // Runs one guarded request for the current scope. A 409 explains the stale
  // state and reloads the list; the mutation is never retried silently.
  // `apply` may reload the list; controls stay busy until it finishes, so no
  // mutation can run against the pre-change list or race the reload.
  async function run<T>(kind: Busy, request: () => Promise<T>, apply: (value: T) => void | Promise<void>) {
    const current = scope.current;
    setBusy(kind); setError(''); setNotice('');
    try {
      const value = await request();
      if (current === scope.current) await apply(value);
    } catch (err) {
      if (current !== scope.current) return;
      setPreview(undefined);
      if (kind === 'deploy' && err instanceof ApiError && err.status >= 400 && err.status < 500) { setReviewRequestId(''); setActiveReview(undefined); }
      if (isBusy(err)) {
        setError((err as ApiError).message);
      } else if (isStaleConflict(err)) {
        const reloaded = await reloadList(current);
        if (reloaded && current === scope.current) setNotice(`Workload trong ${environment} đã thay đổi kể từ khi trang được tải hoặc xem trước. Danh sách đã được tải lại; hãy kiểm tra và xem trước lại.`);
      } else {
        setError(err instanceof ApiError && err.code ? `${failureMessage(kind)} ${err.message}` : failureMessage(kind));
      }
    } finally {
      if (current === scope.current) setBusy('');
    }
  }

  // Reloads the list after a mutation outcome. On failure the stale list is
  // dropped, so no further mutation uses an old version, and Retry reloads.
  async function reloadList(current: number): Promise<boolean> {
    try {
      const fresh = await getWorkloads(application.id, environment);
      if (current === scope.current) { setData(fresh); setError(''); setLoading(false); }
      return true;
    } catch {
      if (current === scope.current) { setData(undefined); setError(`Không thể tải lại danh sách Workload của ${environment}. Hãy thử lại trước khi thay đổi tiếp.`); }
      return false;
    }
  }

  function changeDeletion(id: string, undo: boolean) {
    if (!data || environmentBusy) return;
    if (!undo && !window.confirm(`Đánh dấu xóa ${id} trong ${environment}? Workload đang chạy chỉ thay đổi sau khi Triển khai.`)) return;
    const version = data.draftVersion;
    setDeployReport(undefined);
    void run(undo ? `undo:${id}` : `delete:${id}`, () => undo ? undoWorkloadDelete(application.id, environment, id, version) : deleteWorkload(application.id, environment, id, version), (list) => { setPreview(undefined); setData(list); });
  }
  function loadPreview() {
    setReviewRequestId(''); setActiveReview(undefined); setObserved([]); setReviewOperation(undefined);
    setPreview(undefined); setDeployReport(undefined);
    void run('preview', () => previewChanges(application.id, environment), setPreview);
  }
  function deployPreview() {
    if (!preview || (preview.changes.length === 0 && !preview.routePending)) return;
    const token = preview.token;
    const requestId = newReviewRequestId();
    setActiveReview(preview.review); setObserved([]); setReviewOperation(undefined); setReviewRequestId(requestId); setObservationStale(false);
    const current = scope.current;
    void run('deploy', () => deployChanges(application.id, environment, token, requestId), async (report) => {
      // The outcome is shown at once; the list reload is a separate guarded
      // step whose failure never hides the outcome.
      setDeployReport(report); setPreview(undefined); setReload((value) => value + 1);
      await reloadList(current);
    });
  }

  const workloads = data?.workloads ?? [];
  const progress = activeReview ? correlateReviewProgress(activeReview, observed, reviewOperation?.status) : undefined;
  const locked = busy !== '' || loading || !data;
  return <section className="page application-home"><button className="back-link" disabled={busy === 'deploy'} onClick={() => navigate({ name: 'applications' })}>← Ứng dụng</button>
    <header className="page-header application-header"><div><p className="eyebrow">Ứng dụng</p><h1>{application.name}</h1><p>{endpointFor(application, 'production')}</p><ApplicationTarget application={application} environment={environment} /></div><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>Cài đặt môi trường</Button></header>
    {created ? <div className="form-success" role="status">Đã tạo ứng dụng. Đích triển khai do Kỹ sư nền tảng cấu hình qua Resource Definition (cluster, namespace, mẫu dựng); chưa có gì được triển khai.</div> : null}
    <div className="tabs" role="tablist"><button className={environment === 'staging' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('staging')}>staging<span>{endpointFor(application, 'staging')}</span></button><button className={environment === 'production' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('production')}>production<span>{endpointFor(application, 'production')}</span></button></div>
    <section className="content-panel"><div className="section-header"><div><h2>Workload</h2><p>Cấu hình của {environment}; lưu ở đây không triển khai.</p></div><span><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment })}>Xem trước Score</Button><Button disabled={locked} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment })}>+ Thêm Workload</Button></span></div>
      {error ? <div className="form-error" role="alert">{error}{!data && !loading ? <> <Button onClick={() => void reloadList(scope.current)}>Thử lại</Button></> : null}</div> : null}
      {environmentBusy ? <div className="form-info" role="status">Một thao tác môi trường đang chạy trong {environment}. Thay đổi bản nháp tạm dừng cho đến khi thao tác kết thúc.</div> : null}
      {notice ? <div className="form-info" role="status">{notice}</div> : null}
      {loading ? <p role="status" aria-busy="true">Đang tải Workload…</p> : workloads.length ? <div className="workload-table"><div className="table-head"><span>Tên</span><span>Trạng thái</span><span>Thao tác</span></div>{workloads.map((workload) => <div className="table-row" key={workload.id}><span className="workload-name"><span className="workload-icon">◫</span>{workload.id}</span><Status tone={workload.state ? 'draft' : 'good'}>{workload.state === 'PENDING_DELETE' ? 'Chờ xóa' : workload.state === 'PENDING_UPSERT' ? 'Chờ thay đổi' : 'Sẵn sàng'}</Status><span>{workload.state === 'PENDING_DELETE' ? <Button tone="quiet" disabled={locked || environmentBusy} onClick={() => changeDeletion(workload.id, true)}>{busy === `undo:${workload.id}` ? 'Đang hoàn tác…' : 'Hoàn tác'}</Button> : <><Button tone="quiet" disabled={locked || !workload.score} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment, workloadId: workload.id })}>Sửa</Button><Button tone="danger" disabled={locked || environmentBusy} onClick={() => changeDeletion(workload.id, false)}>{busy === `delete:${workload.id}` ? 'Đang đánh dấu…' : 'Xóa'}</Button></>}</span></div>)}</div> : data ? <div className="section-empty">Môi trường này chưa có Workload nào.</div> : null}
      <div className="form-actions"><Button tone="primary" disabled={locked} onClick={loadPreview}>{busy === 'preview' ? 'Đang tính toán xem trước…' : 'Xem trước thay đổi'}</Button></div>
    </section>
      {preview && !reviewRequestId ? <div className="content-panel preview-container" aria-label="Xem trước triển khai"><header className="review-section-heading"><div><h2>Xem trước thay đổi</h2><ApplicationTarget application={application} environment={environment} /></div><span className="review-section-status">Chờ triển khai</span></header><DeploymentReview review={preview.review} />{preview.changes.length ? <details className="review-plan-details" open={preview.review ? undefined : true}><summary>Chi tiết kỹ thuật của kế hoạch</summary><ul className="preview-changes">{preview.changes.map((change) => <li key={change.workloadId}><strong>{change.workloadId}</strong> · {actionLabel(change.action)} · trình dựng: {rendererText(change.rendering)} · {change.resources.new.length} tài nguyên mới{change.resources.unreferenced.length ? ` · ${change.resources.unreferenced.length} tài nguyên sẽ không còn được tham chiếu (không bị hủy)` : ''}<Placements placements={change.placements} label={`Đích triển khai của ${change.workloadId}`} /><ResourceIntents intents={change.resourceIntents} label={`Tài nguyên của ${change.workloadId}`} /></li>)}</ul></details> : <p>{preview.routePending ? 'Đường dẫn công khai cần được đồng bộ; Workload sẽ không khởi động lại.' : 'Không có thay đổi Workload nào để triển khai.'}</p>}<footer className="review-action-bar"><div><strong>Sẵn sàng để review</strong><small>{preview.changes.length} Workload bị ảnh hưởng · kiểm tra thay đổi trước khi triển khai.</small></div><Button tone="primary" disabled={busy !== '' || (preview.changes.length === 0 && !preview.routePending)} onClick={deployPreview}>{busy === 'deploy' ? 'Đang triển khai…' : preview.routePending && preview.changes.length === 0 ? 'Thử lại đường dẫn công khai' : 'Triển khai các thay đổi này'}</Button></footer></div> : null}
      {reviewRequestId ? <div className="content-panel preview-container" aria-label="Theo dõi triển khai"><header className="review-section-heading"><div><h2>Trạng thái triển khai</h2><p>{environment} · Theo dõi trạng thái sẵn sàng của từng thành phần.</p></div><span className="review-section-status">{reviewOperation ? `${reviewOperation.status} · ${reviewOperation.stage || 'đang chờ bản ghi'}` : 'Đang chờ bản ghi thực thi'}</span></header>{observationStale ? <div className="form-error" role="alert">Không thể làm mới trạng thái. Dữ liệu hiển thị là lần quan sát cuối.</div> : null}<DeploymentReview review={activeReview} progress={progress} title="Bản đồ và tiến độ thực tế" />{busy === 'deploy' ? <footer className="review-action-bar"><div><strong>Đang thực hiện triển khai</strong><small>Trạng thái sẽ được cập nhật theo từng thành phần.</small></div><Button tone="primary" disabled>Đang triển khai…</Button></footer> : null}{observed.length ? <ul>{observed.map((item) => <li key={item.deployment.id}><a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: item.deployment.id }); }}>{item.deployment.workloadId} · {item.deployment.status}</a></li>)}</ul> : <p>Chưa có Deployment cho yêu cầu này.</p>}</div> : null}
      {deployReport ? <div className="content-panel" aria-label="Kết quả triển khai"><h3>Triển khai {reportLabel(deployReport.status)}</h3><ApplicationTarget application={application} environment={environment} /><ul>{deployReport.results.map((result) => <li key={result.workloadId}>{result.workloadId} · {actionLabel(result.action)}: {resultLabel(result.status)}{result.deploymentId ? <> · <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: result.deploymentId as string }); }}>xem triển khai</a></> : null}{result.error ? ' — Workload này không triển khai được. Xem lại cấu hình và xem trước lại.' : ''}</li>)}</ul>{deployReport.status !== 'SUCCEEDED' ? <p>Thay đổi chưa hoàn tất vẫn ở trạng thái chờ. Hãy xem trước lại để thử lại; không có gì được hoàn tác tự động.</p> : null}</div> : null}
    <EnvironmentResources applicationId={application.id} environment={environment} refreshKey={`${reload}`} />
    <RecentDeployments applicationId={application.id} environment={environment} refreshKey={`${reload}`} />
  </section>;
}
