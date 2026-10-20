import { useCallback, useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import { ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { getResource, isTerminalStatus, type Admission, type ResourceDetail } from './api';
import { CleanupProgress } from './CleanupProgress';
import { DeleteResourceDialog } from './DeleteResourceDialog';
import { consumerKindLabel, operationStatusLabel, refusalLabel, resourceStatusLabel, scopeLabel, stageLabel } from './labels';

const redactedMarker = '***redacted***';
const outputText = (value: string) => (value === redactedMarker ? 'Đã che' : value);
const date = (value: string) => { const d = new Date(value); return Number.isNaN(d.getTime()) ? '—' : d.toLocaleString('vi-VN'); };
// Not final = still holding the Environment or of unknown outcome: deletion stays blocked.
const running = (status: string) => !isTerminalStatus(status);

// Every state is tied to the complete scope (application | Environment | resource) it was loaded for.
type Load = ({ state: 'loading' } | { state: 'ready'; detail: ResourceDetail } | { state: 'notFound' } | { state: 'error' }) & { scope: string };

/**
 * Resource detail of one Environment (UC-09 RES-01..03) and the entry to the confirmed deletion
 * (UC-07 DEL-MS-01). Only a Platform Engineer or Admin sees the delete action; a Developer reads
 * the same safe metadata. A failed or missing load is never shown as an empty resource, and a reply
 * for another resource, Environment or reload is ignored.
 */
export function ResourceDetailPage({ application, environment, resourceId, canDelete, pollMs }: { application: Application; environment: EnvironmentKey; resourceId: string; canDelete: boolean; pollMs?: number }) {
  const scope = `${application.id}|${environment}|${resourceId}`;
  const [stored, setLoad] = useState<Load>({ state: 'loading', scope });
  const [attempt, setAttempt] = useState(0);
  const [dialogScope, setDialogScope] = useState<string>();
  const [acceptedFor, setAcceptedFor] = useState<{ scope: string; id: string }>();
  const [noticeFor, setNoticeFor] = useState<{ scope: string; text: string }>();
  const latest = useRef(0);
  const reloadedFor = useRef('');

  useEffect(() => {
    const current = ++latest.current;
    // An explicit reload of the SAME complete scope keeps showing its detail (and the progress mounted under
    // it) while the new reply is awaited; a first load or any other application, Environment or resource
    // shows the loading state, never a previous scope's detail.
    setLoad((previous) => (previous.state === 'ready' && previous.scope === scope ? previous : { state: 'loading', scope }));
    getResource(application.id, environment, resourceId).then((detail) => { if (current === latest.current) setLoad({ state: 'ready', detail, scope }); })
      .catch((err: unknown) => { if (current === latest.current) setLoad(err instanceof ApiError && err.status === 404 ? { state: 'notFound', scope } : { state: 'error', scope }); });
    return () => { latest.current += 1; };
  }, [application.id, environment, resourceId, scope, attempt]);
  // Leaving the scope (or losing the right to delete) closes the dialog and drops the admission and notice;
  // a reload of the same scope does not.
  useEffect(() => { setDialogScope(undefined); setAcceptedFor(undefined); setNoticeFor(undefined); reloadedFor.current = ''; }, [scope, canDelete]);
  const load: Load = stored.scope === scope ? stored : { state: 'loading', scope };
  const dialog = canDelete && dialogScope === scope;
  const accepted = acceptedFor?.scope === scope ? acceptedFor.id : undefined;
  const notice = noticeFor?.scope === scope ? noticeFor.text : '';
  const setNotice = (text: string) => setNoticeFor({ scope, text });
  const reload = useCallback(() => setAttempt((n) => n + 1), []);

  const detail = load.state === 'ready' ? load.detail : undefined;
  const operationId = accepted ?? detail?.cleanup?.operationId;
  const operationRunning = detail?.cleanup ? running(detail.cleanup.status) : accepted !== undefined;
  const blocked = !detail ? ['LOADING'] : detail.status === 'DESTROYED' ? ['DESTROYED'] : detail.eligibility.reasons;
  const canOpen = canDelete && detail !== undefined && detail.eligibility.eligible && !operationRunning && !dialog;

  function onAccepted(admission: Admission) {
    setDialogScope(undefined); setAcceptedFor({ scope, id: admission.operationId }); setNotice('Yêu cầu xóa đã được ghi nhận. Tiến trình bên dưới lấy từ dữ liệu đã lưu.');
    reload();
  }
  function onLost() { setDialogScope(undefined); setNotice('Đã tải lại trạng thái từ máy chủ. Nếu có thao tác xóa thì tiến trình hiển thị bên dưới; nếu không, yêu cầu chưa được ghi nhận và chưa có gì bị xóa.'); reload(); }

  return <section className="page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id, environment })}>← {application.name} · {environment}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>Chi tiết tài nguyên</h1><p>Siêu dữ liệu an toàn đã lưu. Trang này không làm mới nhà cung cấp.</p></div><Button onClick={reload}>Tải lại</Button></header>
    {notice ? <div className="form-info" role="status">{notice}</div> : null}
    {load.state === 'loading' ? <p role="status" aria-busy="true">Đang tải chi tiết tài nguyên…</p> : null}
    {load.state === 'notFound' ? <section className="content-panel"><h2>Không tìm thấy tài nguyên</h2><p>Tài nguyên này không tồn tại trong ứng dụng và môi trường đã chọn.</p></section> : null}
    {load.state === 'error' ? <div className="form-error" role="alert">Không thể tải chi tiết tài nguyên. <Button onClick={reload}>Thử lại</Button></div> : null}
    {detail ? <>
      <section className="content-panel">
        <div className="section-header"><div><h2>Tên tài nguyên</h2><p><code data-testid="resource-name">{detail.descriptor}</code></p></div><Status tone={detail.status === 'READY' ? 'good' : detail.status === 'UNREFERENCED' ? 'draft' : 'neutral'}>{resourceStatusLabel(detail.status)}</Status></div>
        <dl className="detail-grid">
          <div><dt>Loại · class</dt><dd>{detail.type} · {detail.class}</dd></div>
          <div><dt>Phạm vi sở hữu</dt><dd>{scopeLabel(detail.scope.type)}</dd></div>
          <div><dt>Thế hệ đích</dt><dd>{detail.generation}</dd></div>
          <div><dt>Module</dt><dd>{detail.module.id} · {detail.module.revision}</dd></div>
          <div data-testid="original-target"><dt>Đích gốc</dt><dd>{detail.target.known ? <>{detail.target.connectionName ?? detail.target.connectionKey} ({detail.target.connectionKey}){detail.target.connectionKind ? ` · ${detail.target.connectionKind}` : ''}{detail.target.accountId ? ` · tài khoản ${detail.target.accountId}` : ''}{detail.target.region ? ` · ${detail.target.region}` : ''} · thế hệ {detail.target.generation}</> : 'Chưa xác định được từ lịch sử đã lưu. Console không thay bằng đích hiện tại của môi trường.'}</dd></div>
          <div><dt>Phiên bản bản ghi</dt><dd>{detail.version}</dd></div>
          {detail.lastAttempt ? <div><dt>Lần thực thi gần nhất</dt><dd>{detail.lastAttempt.number} · {stageLabel(detail.lastAttempt.stage)} · {detail.lastAttempt.status}{detail.lastAttempt.errorCode ? ` · ${detail.lastAttempt.errorCode}` : ''} · {date(detail.lastAttempt.updatedAt)}</dd></div> : null}
        </dl>
      </section>
      <section className="content-panel"><h2>Đầu ra an toàn</h2>{Object.keys(detail.outputs).length ? <dl className="detail-grid">{Object.entries(detail.outputs).map(([key, value]) => <div key={key}><dt>{key}</dt><dd>{outputText(value)}</dd></div>)}</dl> : <p>Tài nguyên chưa ghi nhận đầu ra nào.</p>}</section>
      <section className="content-panel"><h2>Bên đang dùng</h2>{detail.consumers.length ? <ul>{detail.consumers.map((c) => <li key={`${c.kind}|${c.name}|${c.resourceId ?? ''}`}>{consumerKindLabel(c.kind)}: <code>{c.name}</code>{c.status ? ` · ${resourceStatusLabel(c.status)}` : ''}{c.resourceId ? <> · <a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'resource', applicationId: application.id, environment, resourceId: c.resourceId as string }); }}>mở</a></> : null}</li>)}</ul> : <p>Không còn Workload hay tài nguyên nào đang dùng tài nguyên này.</p>}</section>
      <section className="content-panel" aria-label="Xóa tài nguyên"><h2>Xóa tài nguyên</h2>
        {!canDelete ? <p className="muted">Chỉ Kỹ sư nền tảng hoặc Quản trị viên được xóa tài nguyên.</p> : <>
          {!canOpen && !dialog ? <div className="form-info" role="note" data-testid="delete-blockers">{operationRunning ? 'Một thao tác xóa đang chạy trên môi trường này.' : detail.status === 'DESTROYED' ? 'Tài nguyên này đã được xóa và xác minh; lịch sử được giữ lại.' : `Chưa thể xóa: ${blocked.length ? blocked.map(refusalLabel).join(' ') : 'chưa đủ điều kiện.'}`}</div> : null}
          <Button tone="danger" disabled={!canOpen} onClick={() => setDialogScope(scope)}>Xóa tài nguyên</Button></>}
      </section>
      {operationId ? <CleanupProgress applicationId={application.id} environment={environment} operationId={operationId} intervalMs={pollMs} onSettled={(settledOperation) => { if (reloadedFor.current !== settledOperation.operationId) { reloadedFor.current = settledOperation.operationId; reload(); } }} /> : null}
      {detail.cleanup && !accepted ? <p className="muted">Thao tác xóa gần nhất: {operationStatusLabel(detail.cleanup.status)}.</p> : null}
    </> : null}
    {dialog && detail ? <DeleteResourceDialog applicationId={application.id} environment={environment} resourceId={detail.id} onClose={() => setDialogScope(undefined)} onAccepted={onAccepted} onLost={onLost} /> : null}
  </section>;
}
