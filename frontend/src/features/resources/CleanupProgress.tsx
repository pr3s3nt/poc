import { useCallback, useEffect, useRef, useState } from 'react';
import { Button } from '../../shared/ui/Button';
import type { EnvironmentKey } from '../../shared/types/application';
import { getCleanup, isTerminalStatus, provenSuccess, type CleanupOperation } from './api';
import { failureLabel, operationStatusLabel, outcomeLabel, reasonCodeLabel, stageLabel } from './labels';

const stages = ['REQUESTED', 'CHECKING', 'DESTROYING', 'VERIFYING'] as const;
export const pollIntervalMs = 1500;

// Only these statuses are final. A running, held (INTERRUPTED) or unknown status is indeterminate: it keeps being
// followed, never settles the page and never clears the Environment busy state.
const pending = (status: string) => !isTerminalStatus(status);
const running = (status: string) => status === 'ACTIVE' || status === 'RECOVERING';

/**
 * Follows one persisted cleanup operation (UC-09 RES-03). It polls only that operation in only that
 * scope, ignores a reply that arrives for another one, never calls a provider, and never reports
 * success before every confirmed resource is persisted as verified absent. `onSettled` is called
 * once when the operation leaves the running states so the page can reload the resource detail.
 */
export function CleanupProgress({ applicationId, environment, operationId, intervalMs = pollIntervalMs, onSettled }: { applicationId: string; environment: EnvironmentKey; operationId: string; intervalMs?: number; onSettled?(operation: CleanupOperation): void }) {
  const [operation, setOperation] = useState<CleanupOperation>();
  const [error, setError] = useState('');
  const [tick, setTick] = useState(0);
  const settled = useRef('');
  const latest = useRef(0);
  const callback = useRef(onSettled);
  callback.current = onSettled;

  useEffect(() => {
    setOperation(undefined); setError(''); settled.current = '';
  }, [applicationId, environment, operationId]);

  useEffect(() => {
    const current = ++latest.current;
    let timer: ReturnType<typeof setTimeout> | undefined;
    getCleanup(applicationId, environment, operationId).then((value) => {
      if (current !== latest.current) return;
      setOperation(value); setError('');
      if (pending(value.status)) { timer = setTimeout(() => setTick((n) => n + 1), value.status === 'INTERRUPTED' ? intervalMs * 4 : intervalMs); return; }
      if (settled.current !== operationId) { settled.current = operationId; callback.current?.(value); }
    }).catch(() => {
      // The last known progress stays on screen; a read failure never means the cleanup stopped or finished.
      if (current === latest.current) setError('Không đọc được tiến trình xóa. Tiến trình đã hiển thị có thể đã cũ; thao tác xóa vẫn được lưu và không tự gửi lại.');
    });
    return () => { latest.current += 1; if (timer) clearTimeout(timer); };
  }, [applicationId, environment, operationId, intervalMs, tick]);

  const reload = useCallback(() => { setError(''); setTick((n) => n + 1); }, []);
  return <section className="content-panel" aria-label="Tiến trình xóa tài nguyên" data-testid="cleanup-progress">
    <h2>Tiến trình xóa tài nguyên</h2>
    {error ? <div className="form-error" role="alert">{error} <Button onClick={reload}>Tải lại tiến trình</Button></div> : null}
    {!operation ? (error ? null : <p role="status" aria-busy="true">Đang tải tiến trình xóa…</p>) : <>
      <ol className="stage-list" aria-label="Các giai đoạn">{stages.map((stage) => <li key={stage} data-state={stage === operation.stage && running(operation.status) ? 'current' : undefined} aria-current={stage === operation.stage && running(operation.status) ? 'step' : undefined}>{stageLabel(stage)}</li>)}</ol>
      <p role="status">Trạng thái: <strong>{operationStatusLabel(operation.status)}</strong>{running(operation.status) ? ` · ${stageLabel(operation.stage)}` : ''}</p>
      {provenSuccess(operation) ? <div className="form-success" role="status">Tất cả tài nguyên đã chọn đã được xác minh là không còn. Dữ liệu đã bị xóa vĩnh viễn; lịch sử triển khai được giữ nguyên.</div> : null}
      {operation.status === 'SUCCEEDED' && !provenSuccess(operation) ? <div className="form-error" role="alert">Thao tác báo hoàn tất nhưng bằng chứng từng tài nguyên thiếu hoặc mâu thuẫn (chưa xác minh đã hết, còn đối tượng hoặc chưa rõ) nên không được coi là thành công.</div> : null}
      {!isTerminalStatus(operation.status) && !running(operation.status) && operation.status !== 'INTERRUPTED' ? <div className="form-error" role="alert">Trạng thái thao tác xóa không nhận ra được. Môi trường có thể vẫn đang bị giữ; chưa thể coi thao tác đã xong, tài nguyên đã bị xóa hay còn nguyên.</div> : null}
      {operation.status === 'FAILED' ? <div className="form-error" role="alert">{failureLabel(operation.failureCode ?? 'CLEANUP_FAILED')} Tài nguyên chưa được xác minh là đã hết vẫn có thể còn tồn tại; không có hoàn tác hay thử lại tự động.</div> : null}
      {operation.status === 'INTERRUPTED' ? <div className="form-error" role="alert">Thao tác bị gián đoạn hoặc chưa rõ kết quả. Môi trường được giữ cho tới khi người vận hành xác nhận tiến trình đã dừng; chưa thể coi tài nguyên đã bị xóa hay còn nguyên.</div> : null}
      <table className="plain-table"><thead><tr><th>Tài nguyên</th><th>Kết quả</th><th>Đã xác minh hết</th><th>Còn lại</th></tr></thead><tbody>{operation.resources.map((item) => <tr key={item.id}>
        <td>{item.descriptor}</td>
        <td>{outcomeLabel(item.outcome)}{item.reasonCode ? <><br /><small>{reasonCodeLabel(item.reasonCode)}</small></> : null}</td>
        <td>{item.verified ? 'Có' : 'Chưa'}</td>
        <td>{item.verified ? '—' : item.remaining.length ? <ul>{item.remaining.map((id) => <li key={id}><code>{id}</code></li>)}</ul> : item.remainingUnknown ? 'Chưa rõ — không thể coi là đã hết' : '—'}</td></tr>)}</tbody></table>
      {!operation.allVerified && operation.remainingIds.length ? <p>Còn {operation.remainingIds.length} đối tượng vật lý chưa được xác minh là đã hết.</p> : null}
    </>}
  </section>;
}
