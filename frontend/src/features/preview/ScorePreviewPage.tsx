import { useEffect, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import { ApplicationTarget } from '../../shared/ui/ApplicationTarget';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { ApiError } from '../../shared/api/client';
import { previewScore, type PreviewAction, type ScorePreview, type ScorePreviewRequest } from './api';
import { parseScoreText } from './parseScore';
import { ResourceIntents, rendererText } from './ResourceIntents';
import { DependencyGraph } from '../../shared/ui/DependencyGraph';
import { Placements } from './Placements';

const actions: { value: PreviewAction; label: string; hint: string }[] = [
  { value: 'deploy', label: 'Thêm Workload', hint: 'Chỉ cần Score sau; Workload chưa tồn tại.' },
  { value: 'update', label: 'Cập nhật Workload', hint: 'Score trước phải trùng Score đang triển khai.' },
  { value: 'remove', label: 'Xóa Workload', hint: 'Chỉ cần Score trước; phải trùng Score đang triển khai.' },
];

const actionLabel = (action: string) => actions.find((item) => item.value === action)?.label ?? action;

type Failure = { kind: 'validation' | 'error'; message: string };

export function ScorePreviewPage({ application, environment }: { application: Application; environment: EnvironmentKey }) {
  const [action, setAction] = useState<PreviewAction>('deploy');
  const [workloadId, setWorkloadId] = useState('');
  const [runId, setRunId] = useState('');
  const [before, setBefore] = useState('');
  const [after, setAfter] = useState('');
  const [result, setResult] = useState<ScorePreview>();
  const [failure, setFailure] = useState<Failure>();
  const [previewing, setPreviewing] = useState(false);
  // Every input or scope change bumps the generation, so an obsolete
  // response can never be shown as the preview of the current input.
  const generation = useRef(0);
  const needsBefore = action !== 'deploy';
  const needsAfter = action !== 'remove';

  useEffect(() => { generation.current += 1; setResult(undefined); setFailure(undefined); setPreviewing(false); }, [application.id, environment]);

  function changed<T>(set: (value: T) => void) {
    return (value: T) => { generation.current += 1; set(value); setResult(undefined); setFailure(undefined); setPreviewing(false); };
  }

  async function submit() {
    const problems: string[] = [];
    if (!workloadId.trim()) problems.push('ID Workload là bắt buộc.');
    if (!runId.trim()) problems.push('ID lần chạy là bắt buộc.');
    const request: ScorePreviewRequest = { workloadId, action, runId };
    if (needsBefore) {
      const parsed = parseScoreText(before);
      if (parsed.ok) request.scoreBefore = parsed.score; else problems.push(`Score trước ${parsed.error}`);
    }
    if (needsAfter) {
      const parsed = parseScoreText(after);
      if (parsed.ok) request.scoreAfter = parsed.score; else problems.push(`Score sau ${parsed.error}`);
    }
    setResult(undefined);
    if (problems.length) { setFailure({ kind: 'validation', message: problems.join(' ') }); return; }
    const current = ++generation.current;
    setFailure(undefined); setPreviewing(true);
    try {
      const response = await previewScore(application.id, environment, request);
      if (current === generation.current) setResult(response);
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 401) return;
      // Only the stable code selects text; raw backend/parser text is never shown.
      if (err instanceof ApiError && (err.code === 'ENVIRONMENT_UNCONFIGURED')) setFailure({ kind: 'error', message: `Môi trường ${environment} chưa có kết nối triển khai. Hãy mở Cài đặt môi trường của ${environment} và chọn kết nối; chưa có gì được lập kế hoạch hoặc triển khai.` });
      else if (err instanceof ApiError && (err.status === 400 || err.status === 413)) setFailure({ kind: 'validation', message: err.message });
      else if (err instanceof ApiError) setFailure({ kind: 'error', message: err.message });
      else setFailure({ kind: 'error', message: 'Không thể xem trước Score. Hãy thử lại.' });
    } finally {
      if (current === generation.current) setPreviewing(false);
    }
  }

  const hint = actions.find((item) => item.value === action)?.hint;
  return <section className="page preview-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header"><div><p className="eyebrow">{application.name} · {environment}</p><h1>Xem trước Score</h1><p>Kiểm tra Score và xem các thay đổi dự kiến cho <strong>{application.name}</strong> trong <strong>{environment}</strong>. Không có gì được lưu hay triển khai.</p><ApplicationTarget application={application} environment={environment} /></div></header>
    <div className="tabs" role="tablist" aria-label="Môi trường">{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={environment === env} className={environment === env ? 'tab tab-active' : 'tab'} onClick={() => navigate({ name: 'score-preview', applicationId: application.id, environment: env })}>{env}</button>)}</div>
    <form className="content-panel preview-form" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
      <p className="form-note" role="note">Không dán giá trị bí mật. Hãy tham chiếu cấu hình bằng placeholder như <code>{'${resources.env.API_TOKEN}'}</code>.</p>
      <div className="form-grid">
        <label>Thao tác<select value={action} aria-describedby="preview-action-hint" onChange={(event) => changed(setAction)(event.target.value as PreviewAction)}>{actions.map((item) => <option key={item.value} value={item.value}>{item.label}</option>)}</select></label>
        <label>ID Workload<input value={workloadId} onChange={(event) => changed(setWorkloadId)(event.target.value)} placeholder="phải trùng metadata.name" autoComplete="off" /></label>
        <label>ID lần chạy<input value={runId} aria-describedby="preview-run-hint" onChange={(event) => changed(setRunId)(event.target.value)} placeholder="ví dụ preview-1" autoComplete="off" /></label>
      </div>
      <p className="form-hints"><small id="preview-action-hint">{hint}</small> <small id="preview-run-hint">ID lần chạy là ngữ cảnh lập kế hoạch dùng trong tên được tạo; dùng lại ID để so sánh các lần chạy.</small></p>
      {needsBefore ? <label className="score-editor">Score trước (YAML hoặc JSON)<textarea value={before} onChange={(event) => changed(setBefore)(event.target.value)} rows={12} spellCheck={false} /></label> : null}
      {needsAfter ? <label className="score-editor">Score sau (YAML hoặc JSON)<textarea value={after} onChange={(event) => changed(setAfter)(event.target.value)} rows={12} spellCheck={false} /></label> : null}
      {failure ? <div className="form-error" role="alert">{failure.kind === 'validation' ? 'Không thể xem trước: ' : ''}{failure.message}{failure.kind === 'error' ? <> <Button type="button" onClick={() => void submit()}>Thử lại</Button></> : null}</div> : null}
      <div className="form-actions"><Button tone="primary" type="submit" disabled={previewing}>{previewing ? 'Đang tính kết quả xem trước…' : 'Xem trước'}</Button></div>
    </form>
    {previewing ? <p role="status" aria-busy="true">Đang tính kết quả xem trước…</p> : null}
    {result ? <PreviewResult result={result} /> : null}
  </section>;
}

function DeltaSummary({ result }: { result: ScorePreview }) {
  const modules = result.delta.modules;
  const added = Object.keys(modules?.add ?? {});
  const updated = Object.entries(modules?.update ?? {});
  const removed = modules?.remove ?? [];
  const shared = result.delta.shared ?? [];
  if (!added.length && !updated.length && !removed.length && !shared.length) {
    return <div className="form-success" role="status">Không có thay đổi Workload: Score tạo ra đúng Deployment Set hiện tại. Chi tiết lập kế hoạch bên dưới vẫn mô tả đồ thị của môi trường.</div>;
  }
  return <ul className="delta-summary">
    {added.map((id) => <li key={`add-${id}`}><strong>thêm</strong> module {id}</li>)}
    {updated.map(([id, patches]) => <li key={`update-${id}`}><strong>cập nhật</strong> module {id} · {patches.length} thay đổi: {patches.map((patch) => `${patch.op} ${patch.path}`).join(', ')}</li>)}
    {removed.map((id) => <li key={`remove-${id}`}><strong>xóa</strong> module {id}</li>)}
    {shared.length ? <li><strong>dùng chung</strong> · {shared.length} thay đổi: {shared.map((patch) => `${patch.op} ${patch.path}`).join(', ')}</li> : null}
  </ul>;
}

function List({ items, empty }: { items: string[]; empty: string }) {
  return items.length ? <ul>{items.map((item) => <li key={item}>{item}</li>)}</ul> : <p>{empty}</p>;
}

function PreviewResult({ result }: { result: ScorePreview }) {
  const renderer = result.rendering?.[result.workloadId];
  return <section className="preview-result" aria-label="Kết quả xem trước Score">
    <section className="content-panel"><div className="section-header"><div><h2>Xem trước · {result.workloadId} · {actionLabel(result.action)}</h2><p>Kết quả chỉ đọc. Không có gì được lưu hay triển khai; hãy triển khai qua luồng Workload thông thường.</p></div></div>
      <dl className="detail-grid"><div><dt>Môi trường</dt><dd>{result.applicationKey} / {result.environmentKey}</dd></div><div><dt>Deployment Set nền</dt><dd>{result.baseSetId || 'trống'}</dd></div><div><dt>Phiên bản môi trường</dt><dd>{result.baseVersion}</dd></div><div><dt>ID lần chạy</dt><dd>{result.runId}</dd></div><div><dt>Trình dựng Workload</dt><dd>{rendererText(renderer)}</dd></div><div><dt>Mã băm kế hoạch</dt><dd><code className="plan-hash" title={result.planHash}>{result.planHash}</code></dd></div></dl></section>
    <section className="content-panel"><h2>Delta</h2><DeltaSummary result={result} /></section>
    <section className="content-panel"><h2>Thay đổi tài nguyên</h2><div className="detail-grid">
      <div><h3>Mới ({result.classification.new.length})</h3><List items={result.classification.new} empty="Không có" /></div>
      <div><h3>Đã có ({result.classification.existing.length})</h3><List items={result.classification.existing} empty="Không có" /></div>
      <div><h3>Không còn được tham chiếu ({result.classification.unreferenced.length})</h3><List items={result.classification.unreferenced} empty="Không có" /></div></div></section>
    <section className="content-panel"><h2>Tài nguyên hạ tầng</h2><Placements placements={result.placements} />{result.binding?.connectionKey ? <p aria-label="Đích đã ghim của xem trước">Đích đã ghim: Kết nối {result.binding.connectionKey} · phạm vi {result.binding.executionProfile}{result.binding.region ? ` · vùng ${result.binding.region}` : ''}{result.accountId ? ` · tài khoản AWS ${result.accountId}` : ''}</p> : null}<ResourceIntents intents={result.resourceIntents} /></section>
    <section className="content-panel"><h2>Thứ tự tạo tài nguyên</h2>{result.batches.length ? <ol className="batch-list">{result.batches.map((batch, index) => <li key={index}>Lô {index + 1}: {batch.join(', ')}</li>)}</ol> : <p>Không có tài nguyên cần tạo.</p>}<p><small>Bên cung cấp được tạo trước bên sử dụng.</small></p></section>
    <section className="content-panel"><h2>Liên kết thực thi tài nguyên</h2>{result.matches.length ? <table className="plain-table"><thead><tr><th>Tài nguyên</th><th>Liên kết</th><th>Driver</th></tr></thead><tbody>{result.matches.map((match) => <tr key={match.descriptor}><td>{match.descriptor}</td><td>{match.binding === 'environment-connection' ? `Kết nối của môi trường ${match.connectionKey ?? ''}`.trim() : match.definitionKey}</td><td>{match.driverType}</td></tr>)}</tbody></table> : <p>Không có liên kết tài nguyên.</p>}</section>
    <section className="content-panel"><h2>Đồ thị tài nguyên</h2>
      <DependencyGraph nodes={result.graph.nodes} edges={result.graph.edges} /></section>
    <section className="content-panel"><h2>Tài liệu (bản đã lọc dữ liệu nhạy cảm)</h2><p><small>Bản đã lọc dữ liệu nhạy cảm: giá trị biến container nguyên văn của các Workload khác hiển thị là <code>***redacted***</code>, và giá trị tham số trong đồ thị tài nguyên bị lược bỏ (chỉ liệt kê tên tham số). Đây không phải Deployment Set có thể thực thi.</small></p>
      <details><summary>Tài liệu Delta</summary><pre className="score-preview">{JSON.stringify(result.delta, null, 2)}</pre></details>
      <details><summary>Candidate Deployment Set — bản đã lọc dữ liệu nhạy cảm, không thể thực thi</summary><pre className="score-preview">{JSON.stringify(result.candidateSet, null, 2)}</pre></details></section>
  </section>;
}
