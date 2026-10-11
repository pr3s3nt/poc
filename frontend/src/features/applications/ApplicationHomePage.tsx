import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { ApplicationTarget } from '../../shared/ui/ApplicationTarget';
import { deleteWorkload, deployChanges, getWorkloads, previewChanges, undoWorkloadDelete, type DeployReport, type PendingPreview, type WorkloadList } from '../workloads/api';
import { useEnvironmentBusy } from '../environment/useEnvironmentBusy';
import { RecentDeployments } from '../deployments/RecentDeployments';

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

export function ApplicationHomePage({ application, created = false, onTargetChange }: { application: Application; created?: boolean; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>('staging');
  // Draft Delete/Undo are Environment writes: held while an operation owns it.
  const environmentBusy = useEnvironmentBusy(application, environment, onTargetChange);
  const [data, setData] = useState<WorkloadList>();
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [loading, setLoading] = useState(true);
  const [preview, setPreview] = useState<PendingPreview>();
  const [busy, setBusy] = useState<Busy>('');
  const [deployReport, setDeployReport] = useState<DeployReport>();
  const [reload, setReload] = useState(0);
  const [needsTarget, setNeedsTarget] = useState(false);
  // Bumped on every Environment switch: a response started for an earlier
  // scope is ignored instead of being shown under the new Environment.
  const scope = useRef(0);

  useEffect(() => {
    const current = ++scope.current;
    setData(undefined); setPreview(undefined); setDeployReport(undefined); setLoading(true); setError(''); setNotice(''); setBusy(''); setNeedsTarget(false);
    getWorkloads(application.id, environment)
      .then((result) => { if (current === scope.current) setData(result); })
      .catch(() => { if (current === scope.current) setError(`Không thể tải danh sách Workload của ${environment}.`); })
      .finally(() => { if (current === scope.current) setLoading(false); });
  }, [application.id, environment]);

  // Runs one guarded request for the current scope. A 409 explains the stale
  // state and reloads the list; the mutation is never retried silently.
  // `apply` may reload the list; controls stay busy until it finishes, so no
  // mutation can run against the pre-change list or race the reload.
  async function run<T>(kind: Busy, request: () => Promise<T>, apply: (value: T) => void | Promise<void>) {
    const current = scope.current;
    setBusy(kind); setError(''); setNotice(''); setNeedsTarget(false);
    try {
      const value = await request();
      if (current === scope.current) await apply(value);
    } catch (err) {
      if (current !== scope.current) return;
      setPreview(undefined);
      if (err instanceof ApiError && err.status === 409) {
        const reloaded = await reloadList(current);
        if (reloaded && current === scope.current) setNotice(`Workload trong ${environment} đã thay đổi kể từ khi trang được tải hoặc xem trước. Danh sách đã được tải lại; hãy kiểm tra và xem trước lại.`);
      } else if (err instanceof ApiError && err.status === 422 && err.field === 'connectionKey') {
        // UNCONFIGURED Environment: nothing was planned or deployed.
        setError('Môi trường chưa có kết nối triển khai. Hãy chọn kết nối trong Cài đặt môi trường; chưa có gì được lập kế hoạch hoặc triển khai.'); setNeedsTarget(true);
      } else {
        setError(failureMessage(kind));
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
    setPreview(undefined); setDeployReport(undefined);
    void run('preview', () => previewChanges(application.id, environment), setPreview);
  }
  function deployPreview() {
    if (!preview || (preview.changes.length === 0 && !preview.routePending)) return;
    const token = preview.token;
    const current = scope.current;
    void run('deploy', () => deployChanges(application.id, environment, token), async (report) => {
      // The outcome is shown at once; the list reload is a separate guarded
      // step whose failure never hides the outcome.
      setDeployReport(report); setPreview(undefined); setReload((value) => value + 1);
      await reloadList(current);
    });
  }

  const workloads = data?.workloads ?? [];
  const locked = busy !== '' || loading || !data;
  return <section className="page application-home"><button className="back-link" disabled={busy === 'deploy'} onClick={() => navigate({ name: 'applications' })}>← Ứng dụng</button>
    <header className="page-header application-header"><div><p className="eyebrow">Ứng dụng</p><h1>{application.name}</h1><p>{endpointFor(application, 'production')}</p><ApplicationTarget application={application} environment={environment} /></div><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>Cài đặt môi trường</Button></header>
    {created ? <div className="form-success" role="status">Đã tạo ứng dụng. Hãy chọn kết nối cho từng môi trường trong Cài đặt môi trường trước khi xem trước hoặc triển khai; chưa có gì được triển khai.</div> : null}
    <div className="tabs" role="tablist"><button className={environment === 'staging' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('staging')}>staging<span>{endpointFor(application, 'staging')}</span></button><button className={environment === 'production' ? 'tab tab-active' : 'tab'} disabled={busy === 'deploy'} onClick={() => setEnvironment('production')}>production<span>{endpointFor(application, 'production')}</span></button></div>
    <section className="content-panel"><div className="section-header"><div><h2>Workload</h2><p>Cấu hình của {environment}; lưu ở đây không triển khai.</p></div><span><Button disabled={busy === 'deploy'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment })}>Xem trước Score</Button><Button disabled={locked} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment })}>+ Thêm Workload</Button></span></div>
      {error ? <div className="form-error" role="alert">{error}{needsTarget ? <> <Button onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>Mở Cài đặt môi trường</Button></> : null}{!data && !loading ? <> <Button onClick={() => void reloadList(scope.current)}>Thử lại</Button></> : null}</div> : null}
      {environmentBusy ? <div className="form-info" role="status">Một thao tác môi trường đang chạy trong {environment}. Thay đổi bản nháp tạm dừng cho đến khi thao tác kết thúc.</div> : null}
      {notice ? <div className="form-info" role="status">{notice}</div> : null}
      {loading ? <p role="status" aria-busy="true">Đang tải Workload…</p> : workloads.length ? <div className="workload-table"><div className="table-head"><span>Tên</span><span>Trạng thái</span><span>Thao tác</span></div>{workloads.map((workload) => <div className="table-row" key={workload.id}><span className="workload-name"><span className="workload-icon">◫</span>{workload.id}</span><Status tone={workload.state ? 'draft' : 'good'}>{workload.state === 'PENDING_DELETE' ? 'Chờ xóa' : workload.state === 'PENDING_UPSERT' ? 'Chờ thay đổi' : 'Sẵn sàng'}</Status><span>{workload.state === 'PENDING_DELETE' ? <Button tone="quiet" disabled={locked || environmentBusy} onClick={() => changeDeletion(workload.id, true)}>{busy === `undo:${workload.id}` ? 'Đang hoàn tác…' : 'Hoàn tác'}</Button> : <><Button tone="quiet" disabled={locked || !workload.score} onClick={() => navigate({ name: 'workload', applicationId: application.id, environment, workloadId: workload.id })}>Sửa</Button><Button tone="danger" disabled={locked || environmentBusy} onClick={() => changeDeletion(workload.id, false)}>{busy === `delete:${workload.id}` ? 'Đang đánh dấu…' : 'Xóa'}</Button></>}</span></div>)}</div> : data ? <div className="section-empty">Môi trường này chưa có Workload nào.</div> : null}
      <div className="form-actions"><Button tone="primary" disabled={locked} onClick={loadPreview}>{busy === 'preview' ? 'Đang tính toán xem trước…' : 'Xem trước thay đổi'}</Button></div>
      {preview ? <div className="content-panel" aria-label="Xem trước triển khai"><h3>Xem trước cho {environment}</h3><ApplicationTarget application={application} environment={environment} /><p>{preview.changes.length} Workload bị ảnh hưởng · bản nháp v{preview.draftVersion} · cấu hình {preview.configRevisionId ? preview.configRevisionId.slice(0, 8) : 'trống'}</p>{preview.changes.length ? <ul>{preview.changes.map((change) => <li key={change.workloadId}><strong>{change.workloadId}</strong> · {actionLabel(change.action)} · {change.rendering?.driverType ? `${change.rendering.driverType} ${change.rendering.bundle.version} (${change.rendering.definitionKey})` : 'Kubernetes tích hợp sẵn'} · {change.resources.new.length} tài nguyên mới{change.resources.unreferenced.length ? ` · ${change.resources.unreferenced.length} tài nguyên sẽ không còn được tham chiếu (không bị hủy)` : ''}</li>)}</ul> : <p>{preview.routePending ? 'Đường dẫn công khai cần được đồng bộ; Workload sẽ không khởi động lại.' : 'Không có thay đổi Workload nào để triển khai.'}</p>}<Button tone="primary" disabled={busy !== '' || (preview.changes.length === 0 && !preview.routePending)} onClick={deployPreview}>{busy === 'deploy' ? 'Đang triển khai…' : preview.routePending && preview.changes.length === 0 ? 'Thử lại đường dẫn công khai' : 'Triển khai các thay đổi này'}</Button></div> : null}
      {deployReport ? <div className="content-panel" aria-label="Kết quả triển khai"><h3>Triển khai {reportLabel(deployReport.status)}</h3><ApplicationTarget application={application} environment={environment} /><ul>{deployReport.results.map((result) => <li key={result.workloadId}>{result.workloadId} · {actionLabel(result.action)}: {resultLabel(result.status)}{result.deploymentId ? <> · <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'deployment', applicationId: application.id, environment, deploymentId: result.deploymentId as string }); }}>xem triển khai</a></> : null}{result.error ? ' — Workload này không triển khai được. Xem lại cấu hình và xem trước lại.' : ''}</li>)}</ul>{deployReport.status !== 'SUCCEEDED' ? <p>Thay đổi chưa hoàn tất vẫn ở trạng thái chờ. Hãy xem trước lại để thử lại; không có gì được hoàn tác tự động.</p> : null}</div> : null}
    </section>
    <RecentDeployments applicationId={application.id} environment={environment} refreshKey={`${reload}`} />
  </section>;
}
