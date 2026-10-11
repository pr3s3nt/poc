import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { statusLabel } from './labels';
import { cleanupSource, executeTransition, fetchEnvironment, getTransition, listConnectionChoices, listTransitions, previewTransition, type Mode, type TransitionDetail, type TransitionPreview } from './api';

type Choice = { key: string; name: string; kind: string };
const stageLabel: Record<string, string> = {
  PREFLIGHT: 'Kiểm tra trước', QUIESCING: 'Dừng nguồn', BACKUP: 'Sao lưu', PROVISIONING: 'Tạo tài nguyên', RESTORING: 'Khôi phục dữ liệu',
  DEPLOYING: 'Triển khai', VERIFYING: 'Xác minh', CUTOVER: 'Chuyển sang đích',
};
const stages = ['PREFLIGHT', 'QUIESCING', 'BACKUP', 'PROVISIONING', 'RESTORING', 'DEPLOYING', 'VERIFYING', 'CUTOVER'];

const actionLabel: Record<string, string> = {
  UNCHANGED: 'Triển khai lại, không đổi',
  UPDATED: 'Triển khai lại kèm thay đổi nháp đang chờ',
  ADDED: 'Thêm từ bản nháp đang chờ',
  REMOVED: 'Bị gỡ — không có ở đích (nguồn không bị đụng tới)',
};

// ImpactList shows what the destination will actually contain: each workload with
// its reviewed action and whether it picks up newer configuration. Removed
// workloads are never listed as redeployed.
function ImpactList({ workloads }: { workloads: TransitionPreview['workloads'] }) {
  return <>
    <h4>Workload tại đích</h4>
    <ul aria-label="Tác động lên workload">{workloads.map((item) => <li key={item.workloadId} data-action={item.action}><strong>{item.workloadId}</strong> — {actionLabel[item.action] ?? item.action}{item.configChanged && item.action !== 'REMOVED' ? ' · nhận cấu hình mới hơn' : ''}</li>)}</ul>
    {workloads.some((item) => item.action === 'UPDATED' || item.action === 'ADDED' || item.action === 'REMOVED') ? <p className="warning-text">Các thay đổi nháp đang chờ được đánh dấu ở trên sẽ được áp dụng bởi lần chuyển đích này và được dùng hết khi chuyển thành công.</p> : null}
    {workloads.some((item) => item.configChanged) ? <p className="warning-text">Bản cấu hình mới nhất được áp dụng cho các workload đánh dấu ở trên.</p> : null}
  </>;
}

// authorityText derives who is live from the persisted binding, not the status.
function authorityText(detail: TransitionDetail): string {
  if (detail.authority === 'DESTINATION') return 'Cutover đã được ghi nhận: đích đang chạy và là nguồn chuẩn.';
  if (detail.sourceState === 'NEEDS_ATTENTION') return 'Thế hệ nguồn vẫn là đích được ghi nhận nhưng CHƯA được khôi phục. Cần người vận hành xử lý.';
  return 'Thế hệ nguồn vẫn là nguồn chuẩn.';
}

// Explicit destination change of a deployed Environment (UC-01 ES-05, ADR-012):
// preview, acknowledged execution, persisted progress that survives a refresh,
// recovery results and explicit cleanup of the retained source generation.
export function TransitionPanel({ application, environment, initialDestination = '', onTargetChange, pollMs = 1500 }: {
  application: Application; environment: EnvironmentKey; initialDestination?: string;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void; pollMs?: number;
}) {
  const target = application.environments[environment];
  const [choices, setChoices] = useState<readonly Choice[]>([]);
  const [choicesLoaded, setChoicesLoaded] = useState(false);
  const [destination, setDestination] = useState(initialDestination);
  const [mode, setMode] = useState<Mode>('MIGRATE_POSTGRES');
  const [preview, setPreview] = useState<TransitionPreview>();
  const [acknowledged, setAcknowledged] = useState(false);
  const [detail, setDetail] = useState<TransitionDetail>();
  const [history, setHistory] = useState<readonly TransitionDetail[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);
  useEffect(() => () => { generation.current += 1; }, []);
  useEffect(() => { generation.current += 1; setPreview(undefined); setDetail(undefined); setError(''); setAcknowledged(false); setBusy(false); setDestination(initialDestination); }, [application.id, environment, initialDestination]);

  // Persisted progress: a refresh finds the latest transition again.
  useEffect(() => {
    const current = generation.current;
    listConnectionChoices().then((items) => { if (current === generation.current) { setChoices(items.filter((item) => item.key !== target.connectionKey && item.status === 'READY' && (item.kind === 'KUBERNETES' || item.kind === 'AWS'))); setChoicesLoaded(true); } }).catch(() => undefined);
    listTransitions(application.id, environment).then((items) => {
      if (current !== generation.current) return;
      setHistory(items);
      const running = items.find((item) => item.status === 'RUNNING');
      if (running) setDetail(running);
    }).catch(() => undefined);
  }, [application.id, environment, target.connectionKey, target.targetGeneration]);

  const running = detail?.status === 'RUNNING';
  useEffect(() => {
    if (!detail || detail.status !== 'RUNNING') return;
    const current = generation.current;
    const timer = window.setInterval(() => {
      getTransition(application.id, environment, detail.id).then(async (latest) => {
        if (current !== generation.current) return;
        setDetail(latest);
        if (latest.status !== 'RUNNING') {
          const env = await fetchEnvironment(application.id, environment);
          if (env && current === generation.current) onTargetChange(application.id, environment, env);
          const items = await listTransitions(application.id, environment);
          if (current === generation.current) setHistory(items);
        }
      }).catch(() => undefined);
    }, pollMs);
    return () => window.clearInterval(timer);
  }, [detail?.id, detail?.status, application.id, environment, pollMs]); // eslint-disable-line react-hooks/exhaustive-deps

  function explain(err: unknown): string {
    if (err instanceof ApiError) {
      if (err.status === 409) return err.message;
      if (err.status === 422) return err.message;
    }
    return 'Yêu cầu không thành công. Hãy thử lại.';
  }

  // A destination that is no longer an eligible choice (not READY, or now the source) is never reviewed or executed.
  const validDestination = choices.some((item) => item.key === destination);
  const lostDestination = choicesLoaded && Boolean(destination) && !validDestination;

  async function runPreview() {
    if (!validDestination || busy) return;
    const current = ++generation.current;
    setBusy(true); setError(''); setPreview(undefined); setAcknowledged(false);
    try {
      const response = await previewTransition(application.id, environment, destination, mode);
      if (current === generation.current) setPreview(response.preview);
    } catch (err) {
      if (current === generation.current) setError(explain(err));
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  async function execute() {
    if (!preview || busy) return;
    const current = ++generation.current;
    setBusy(true); setError('');
    try {
      const response = await executeTransition(application.id, environment, destination, mode, preview.token, acknowledged, preview.mappings);
      if (current !== generation.current) return;
      setDetail(response.transition);
      setPreview(undefined);
      const env = await fetchEnvironment(application.id, environment);
      if (env && current === generation.current) onTargetChange(application.id, environment, env);
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 409) {
        // Stale preview or a busy environment: the old token is useless; review again.
        setPreview(undefined);
        const env = await fetchEnvironment(application.id, environment).catch(() => undefined);
        if (env && current === generation.current) onTargetChange(application.id, environment, env);
      }
      setError(explain(err));
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  async function cleanup(id: string) {
    if (busy) return;
    const current = ++generation.current;
    setBusy(true); setError('');
    try {
      const response = await cleanupSource(application.id, environment, id);
      if (current === generation.current) { setDetail(response.transition); setHistory((items) => items.map((item) => item.id === id ? response.transition : item)); }
    } catch (err) {
      if (current === generation.current) setError(explain(err));
    } finally {
      if (current === generation.current) setBusy(false);
    }
  }

  const finished = detail && detail.status !== 'RUNNING' ? detail : undefined;
  const retained = history.find((item) => item.canCleanupSource);
  return <section className="content-panel transition-panel" aria-label="Chuyển đích kết nối">
    <div className="section-header"><div><h2>Đổi đích triển khai</h2><p>Môi trường này đã có tài nguyên runtime. Đích mới có một thế hệ mới, tách biệt. Không có gì bị ghi đè và thế hệ cũ được giữ cho đến khi bạn dọn dẹp.</p></div></div>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {!detail || finished ? <>
      <label htmlFor="transition-destination">Kết nối đích</label>
      <select id="transition-destination" value={validDestination ? destination : ''} disabled={busy || Boolean(target.activeOperation)} onChange={(event) => { setDestination(event.target.value); setPreview(undefined); }}>
        <option value="" disabled>Chọn một kết nối đích</option>
        {choices.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key})</option>)}
      </select>
      {lostDestination ? <div className="form-info" role="status">Kết nối đích đã chọn không còn Sẵn sàng. Hãy chọn lại.</div> : null}
      <fieldset className="mode-choice"><legend>Cách chuyển</legend>
        <label><input type="radio" name="transition-mode" checked={mode === 'DEPLOY_NEW'} onChange={() => { setMode('DEPLOY_NEW'); setPreview(undefined); }} /> Triển khai mới (tài nguyên trống, không sao chép dữ liệu)</label>
        <label><input type="radio" name="transition-mode" checked={mode === 'MIGRATE_POSTGRES'} onChange={() => { setMode('MIGRATE_POSTGRES'); setPreview(undefined); }} /> Chuyển dữ liệu PostgreSQL (có downtime)</label>
      </fieldset>
      <div className="form-actions"><Button disabled={!validDestination || busy || Boolean(target.activeOperation)} onClick={() => void runPreview()}>{busy && !preview ? 'Đang kiểm tra…' : 'Xem trước chuyển đích'}</Button></div>
    </> : null}
    {preview ? <div className="transition-preview" aria-label="Xem trước chuyển đích">
      <h3>Tác động</h3>
      <p><strong>{preview.source.connectionName || preview.source.connectionKey}</strong> (thế hệ {preview.source.generation}, namespace {preview.source.namespace}) → <strong>{preview.destination.connectionName || preview.destination.connectionKey}</strong> (thế hệ {preview.destination.generation}, namespace {preview.destination.namespace})</p>
      <ImpactList workloads={preview.workloads} />
      {preview.mappings.length ? <ul aria-label="Ánh xạ cơ sở dữ liệu">{preview.mappings.map((item) => <li key={item.sourceDescriptor}>{item.sourceDescriptor} → {item.destinationDescriptor}</li>)}</ul> : null}
      <ul>{preview.notes.map((note) => <li key={note}>{note}</li>)}</ul>
      {preview.downtimeRequired ? <label className="checkbox"><input type="checkbox" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} /> Tôi hiểu ứng dụng sẽ bị dừng trong lúc dữ liệu được sao chép</label> : null}
      <div className="form-actions"><Button tone="primary" disabled={busy || (preview.downtimeRequired && !acknowledged)} onClick={() => void execute()}>{busy ? 'Đang bắt đầu…' : 'Bắt đầu chuyển đích'}</Button></div>
    </div> : null}
    {detail ? <div className="transition-progress" aria-label="Tiến trình chuyển đích">
      <h3>Chuyển đích {detail.status === 'RUNNING' ? 'đang chạy' : statusLabel(detail.status)}</h3>
      <ol className="stage-list">{stages.filter((stage) => detail.mode === 'MIGRATE_POSTGRES' || !['QUIESCING', 'BACKUP', 'RESTORING'].includes(stage)).map((stage) => {
        const result = detail.stages.find((item) => item.stage === stage);
        return <li key={stage} data-status={result?.status ?? 'PENDING'}><strong>{stageLabel[stage] ?? stage}</strong> <code>{stage}</code> <span>{statusLabel(result?.status ?? 'PENDING')}</span>{result?.message ? <em> — {result.message}</em> : null}</li>;
      })}</ol>
      {running ? <p role="status" aria-busy="true">Đang xử lý… tiến trình được lưu và vẫn còn sau khi tải lại trang.</p> : null}
      {detail.status === 'SUCCEEDED' && detail.sourceState === 'RETAINED_QUIESCED' ? <div className="form-success" role="status">Đích đang chạy. Thế hệ nguồn được giữ lại ở trạng thái dừng (dữ liệu còn nguyên) cho đến khi bạn dọn dẹp.</div> : null}
      {detail.status === 'SUCCEEDED' && detail.sourceState === 'CLEANED' ? <div className="form-success" role="status">Đích đang chạy và thế hệ nguồn đã được dọn dẹp.</div> : null}
      {detail.status === 'SUCCEEDED' && detail.sourceState === 'NEEDS_ATTENTION' ? <div className="form-error" role="alert">Đích đang chạy nhưng một hoặc nhiều workload nguồn không dừng được. Hãy tự dừng chúng trước khi dọn dẹp: nguồn CHƯA ở trạng thái dừng.</div> : null}
      {detail.status === 'FAILED' || detail.status === 'INTERRUPTED' ? <div className="form-error" role="alert">{detail.failure || 'Chuyển đích chưa hoàn tất.'} {authorityText(detail)}</div> : null}
      {detail.compensation?.length ? <ul aria-label="Các bước khôi phục">{detail.compensation.map((item) => <li key={item}>{item}</li>)}</ul> : null}
      {detail.compensationFailed?.length ? <ul className="warning-text" aria-label="Vấn đề khi khôi phục">{detail.compensationFailed.map((item) => <li key={item}>Cần xử lý: {item}</li>)}</ul> : null}
      {detail.backupRetained ? <p className="warning-text">Một bản sao lưu riêng vẫn còn trên Pod nguồn và cần người vận hành xử lý.</p> : null}
    </div> : null}
    {retained && !running ? <div className="retained-source" aria-label="Nguồn được giữ lại">
      <h3>Thế hệ nguồn được giữ lại {retained.source.generation}</h3>
      <p>{retained.source.connectionName || retained.source.connectionKey} · namespace {retained.source.namespace} vẫn giữ dữ liệu. Chỉ xóa sau khi bạn đã xem lại đích.</p>
      <Button tone="danger" disabled={busy} onClick={() => void cleanup(retained.id)}>Dọn dẹp thế hệ nguồn</Button>
    </div> : null}
  </section>;
}
