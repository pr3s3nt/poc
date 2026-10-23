import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiError, describeError } from '../../shared/api/client';
import { fieldNote } from '../../shared/api/errors';
import { Button } from '../../shared/ui/Button';
import { addBaseline, listBaselines, today, type Baseline, type BaselineDraft } from './api';
import { Dialog } from './Dialog';
import { formatDate, formatDuration } from './labels';

type Phase = 'loading' | 'error' | 'ready' | 'saving';

function versionLine(b: Baseline): string {
  const value = b.retired ? 'Ngừng áp dụng' : `${formatDuration(b.valueSeconds ?? 0)} (trung vị)`;
  return `Phiên bản ${b.version} · ${value} · từ ${formatDate(b.effectiveFrom)}${b.createdBy ? ` · ${b.createdBy}` : ''}`;
}

/** “Mốc so sánh”: append-only versions of the work-duration baseline (UC-17 ALT-01). */
export function BaselineDialog({ applicationKey, scopeName, locked, onClose, onSaved }: {
  applicationKey?: string; scopeName: string; locked: boolean; onClose(): void; onSaved(message: string): void;
}) {
  const [phase, setPhase] = useState<Phase>('loading');
  const [versions, setVersions] = useState<Baseline[]>([]);
  const [current, setCurrent] = useState(0);
  const [message, setMessage] = useState('');
  const [stale, setStale] = useState(false);
  const request = useRef(0);
  const load = useCallback(() => {
    const id = ++request.current;
    setPhase('loading'); setMessage(''); setStale(false);
    listBaselines(applicationKey).then((response) => {
      if (id !== request.current) return;
      setVersions(response.baselines); setCurrent(response.currentVersion); setPhase('ready');
    }).catch(() => { if (id === request.current) setPhase('error'); });
  }, [applicationKey]);
  useEffect(() => { if (!locked) load(); }, [load, locked]);
  const latest = versions[0];
  const active = latest && !latest.retired ? latest : undefined;

  async function save(draft: Omit<BaselineDraft, 'applicationKey' | 'expectedVersion'>, done: string) {
    setPhase('saving'); setMessage(''); setStale(false);
    try {
      await addBaseline({ ...draft, ...(applicationKey ? { applicationKey } : {}), expectedVersion: current });
      onSaved(done);
    } catch (err) {
      setPhase('ready');
      if (err instanceof ApiError && err.code === 'STALE_VERSION') { setStale(true); return; }
      setMessage(describeError(err, 'Không thể lưu mốc so sánh. Nội dung đang nhập được giữ nguyên.') + (err instanceof ApiError ? fieldNote(err.field) : ''));
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const text = (key: string) => String(form.get(key) ?? '').trim();
    const minutes = Number(text('minutes'));
    const sample = text('sampleSize');
    const from = text('measuredFrom'), to = text('measuredTo');
    if ((from === '') !== (to === '')) { setMessage('Nhập đủ cả hai ngày của kỳ đo, hoặc để trống cả hai.'); return; }
    void save({
      valueSeconds: Math.round(minutes * 60), source: text('source'), effectiveFrom: text('effectiveFrom'),
      ...(sample ? { sampleSize: Number(sample) } : {}), ...(from ? { measuredFrom: from, measuredTo: to } : {}),
    }, 'Đã lưu mốc so sánh.');
  }

  function retire() {
    void save({ retired: true, source: 'Ngừng áp dụng từ Console', effectiveFrom: today() }, 'Đã ngừng áp dụng mốc so sánh.');
  }

  return <Dialog title="Mốc so sánh" subtitle={`${scopeName} · Thời gian thực hiện`} onClose={onClose} busy={phase === 'saving'}>
    {locked ? <p className="form-info">Mốc so sánh áp dụng cho tổ chức hoặc ứng dụng.</p>
      : phase === 'loading' ? <p role="status" aria-busy="true">Đang tải mốc so sánh…</p>
        : phase === 'error' ? <div className="form-error" role="alert">Không thể tải mốc so sánh.<Button onClick={load}>Thử lại</Button></div>
          : <>
            {active ? <p className="eff-baseline-current"><strong>{versionLine(active)}</strong><small>{active.source}</small></p> : <p className="muted">Chưa có mốc so sánh.</p>}
            {stale ? <div className="form-error" role="alert">Mốc so sánh vừa được thay đổi. Tải lại để xem phiên bản mới.<Button onClick={load}>Tải lại</Button></div> : null}
            {message ? <div className="form-error" role="alert">{message}</div> : null}
            <form className="editor-grid eff-baseline-form" onSubmit={submit}>
              <label className="field">Thời gian thực hiện trung vị (phút)<input name="minutes" type="number" min="1" max="525600" step="any" required defaultValue={active?.valueSeconds ? active.valueSeconds / 60 : undefined} /></label>
              <label className="field">Ngày áp dụng<input name="effectiveFrom" type="date" required defaultValue={today()} /></label>
              <label className="field eff-span">Nguồn đo<textarea name="source" required maxLength={2000} placeholder="Phương pháp, loại công việc và phạm vi đo" /></label>
              <label className="field">Số mẫu<input name="sampleSize" type="number" min="1" step="1" /></label>
              <fieldset className="eff-range"><legend>Kỳ đo</legend>
                <label>Từ<input name="measuredFrom" type="date" aria-label="Kỳ đo từ" /></label>
                <label>Đến<input name="measuredTo" type="date" aria-label="Kỳ đo đến" /></label>
              </fieldset>
              <div className="form-actions eff-span">
                {active ? <Button type="button" onClick={retire} disabled={phase === 'saving'}>Ngừng áp dụng</Button> : null}
                <Button tone="primary" type="submit" disabled={phase === 'saving'}>{phase === 'saving' ? 'Đang lưu…' : 'Lưu phiên bản mới'}</Button>
              </div>
            </form>
            <details className="eff-help">
              <summary>Cách so sánh</summary>
              <p>So với trung vị thời gian thực hiện của cùng phạm vi. Mỗi lần lưu tạo phiên bản mới; kỳ trước ngày áp dụng giữ phiên bản cũ. Đây là thời gian quy trình, không phải giờ công.</p>
            </details>
            {versions.length ? <details className="eff-help"><summary>Lịch sử phiên bản</summary>
              <ul className="eff-help-list">{versions.map((b) => <li key={b.id}>{versionLine(b)}<p>{b.source}</p></li>)}</ul>
            </details> : null}
          </>}
  </Dialog>;
}
