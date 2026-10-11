import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { statusLabel } from './labels';
import { fetchEnvironment, recoverOperation, type RecoveryResult } from './api';

// Shows the operation that owns the Environment, keeps it fresh while it runs
// and offers explicit recovery of an interrupted one. Recovery is never
// automatic: the user must confirm the interrupted work has stopped.
export function OperationBanner({ application, environment, onTargetChange, pollMs = 2000 }: {
  application: Application; environment: EnvironmentKey;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void; pollMs?: number;
}) {
  const operation = application.environments[environment].activeOperation;
  const [confirmed, setConfirmed] = useState(false);
  const [recovering, setRecovering] = useState(false);
  const [result, setResult] = useState<RecoveryResult>();
  const [error, setError] = useState('');
  // scope changes only when the Application/Environment changes or the banner unmounts;
  // recovery attempts never touch it, so polling keeps working after a failed recovery.
  const scope = useRef(0);
  useEffect(() => { scope.current += 1; setConfirmed(false); setResult(undefined); setError(''); setRecovering(false); }, [application.id, environment]);
  useEffect(() => () => { scope.current += 1; }, []);
  useEffect(() => {
    if (!operation) return;
    const current = scope.current;
    const timer = window.setInterval(() => {
      fetchEnvironment(application.id, environment).then((latest) => { if (latest && current === scope.current) onTargetChange(application.id, environment, latest); }).catch(() => undefined);
    }, pollMs);
    return () => window.clearInterval(timer);
  }, [operation?.id, application.id, environment, pollMs]); // eslint-disable-line react-hooks/exhaustive-deps

  async function recover() {
    if (!operation || !confirmed || recovering) return;
    const current = scope.current;
    setRecovering(true); setError('');
    try {
      const response = await recoverOperation(application.id, environment, operation.id);
      if (current !== scope.current) return;
      setResult(response.recovery);
      const latest = await fetchEnvironment(application.id, environment);
      if (current !== scope.current) return;
      if (latest) onTargetChange(application.id, environment, latest);
    } catch (err) {
      if (current !== scope.current) return;
      setError(err instanceof ApiError ? err.message : 'Chưa nhận được phản hồi từ máy chủ nên chưa rõ việc khôi phục đã bắt đầu hay chưa. Hãy tải lại trạng thái trước khi thử lại; Console không tự gửi lại.');
    } finally {
      if (current === scope.current) setRecovering(false);
    }
  }

  if (!operation && !result) return null;
  return <section className="content-panel operation-banner" aria-label="Thao tác trên môi trường">
    {operation ? <>
      <div className="section-header"><div><h2>{operation.status === 'INTERRUPTED' ? 'Thao tác bị gián đoạn' : 'Thao tác đang chạy'}</h2><p>{operation.kind.replace('_', ' ').toLowerCase()} · {statusLabel(operation.status)}{operation.stage ? ` · ${operation.stage}` : ''}. Cài đặt, cấu hình và thay đổi nháp bị chặn cho đến khi thao tác kết thúc.</p></div><span className="pending-pill">{statusLabel(operation.status)}</span></div>
      {operation.status === 'INTERRUPTED' ? <div className="recovery">
        <p>Tiến trình sở hữu thao tác này đã ngừng gửi heartbeat. Heartbeat cũ không chứng minh tiến trình đã dừng: hãy xác nhận điều đó trước khi khôi phục đưa nguồn về trạng thái cũ và giải phóng môi trường.</p>
        <label className="checkbox"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} /> Tôi xác nhận thao tác bị gián đoạn đã dừng hẳn</label>
        <div className="form-actions"><Button tone="primary" disabled={!confirmed || recovering} onClick={() => void recover()}>{recovering ? 'Đang khôi phục…' : 'Khôi phục môi trường'}</Button></div>
      </div> : null}
    </> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {result ? <div className="form-info" role="status"><strong>Kết quả khôi phục:</strong> {result.compensationFailed?.length ? `Khôi phục chưa trọn vẹn: còn ${result.compensationFailed.length} bước cần xử lý thủ công. Môi trường vẫn được giữ cho tới khi xử lý xong.` : 'Khôi phục đã hoàn tất.'}</div> : null}
  </section>;
}
