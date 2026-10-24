import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiError, describeError } from '../../shared/api/client';
import { fieldNote } from '../../shared/api/errors';
import { Button } from '../../shared/ui/Button';
import { addBaseline, listBaselines, today, type Baseline, type BaselineDraft } from './api';
import { Dialog } from './Dialog';
import { formatDate, formatDuration } from './labels';
import { t } from '../../shared/i18n';

type Phase = 'loading' | 'error' | 'ready' | 'saving';

function versionLine(b: Baseline): string {
  const value = b.retired ? t('effectiveness.baselineDialog.retire') : t('effectiveness.baselineDialog.formatDurationMedian', { formatDuration: formatDuration(b.valueSeconds ?? 0) });
  return t('effectiveness.baselineDialog.versionVersionValueFromFormatDate', { version: b.version, value, formatDate: formatDate(b.effectiveFrom), value2: b.createdBy ? ` · ${b.createdBy}` : '' });
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
      setMessage(describeError(err, t('effectiveness.baselineDialog.couldNotSaveTheBaseline')) + (err instanceof ApiError ? fieldNote(err.field) : ''));
    }
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const text = (key: string) => String(form.get(key) ?? '').trim();
    const minutes = Number(text('minutes'));
    const sample = text('sampleSize');
    const from = text('measuredFrom'), to = text('measuredTo');
    if ((from === '') !== (to === '')) { setMessage(t('effectiveness.baselineDialog.enterBothDatesOfThe')); return; }
    void save({
      valueSeconds: Math.round(minutes * 60), source: text('source'), effectiveFrom: text('effectiveFrom'),
      ...(sample ? { sampleSize: Number(sample) } : {}), ...(from ? { measuredFrom: from, measuredTo: to } : {}),
    }, t('effectiveness.baselineDialog.baselineSaved'));
  }

  function retire() {
    void save({ retired: true, source: t('effectiveness.baselineDialog.retiredFromTheConsole'), effectiveFrom: today() }, t('effectiveness.baselineDialog.baselineRetired'));
  }

  return <Dialog title={t('effectiveness.baselineDialog.baseline')} subtitle={t('effectiveness.baselineDialog.scopeNameLeadTime', { scopeName })} onClose={onClose} busy={phase === 'saving'}>
    {locked ? <p className="form-info">{t('effectiveness.baselineDialog.theBaselineAppliesToThe')}</p>
      : phase === 'loading' ? <p role="status" aria-busy="true">{t('effectiveness.baselineDialog.loadingBaseline')}</p>
        : phase === 'error' ? <div className="form-error" role="alert">{t('effectiveness.baselineDialog.couldNotLoadTheBaseline')}<Button onClick={load}>{t('effectiveness.baselineDialog.retry')}</Button></div>
          : <>
            {active ? <p className="eff-baseline-current"><strong>{versionLine(active)}</strong><small>{active.source}</small></p> : <p className="muted">{t('effectiveness.baselineDialog.noBaselineYet')}</p>}
            {stale ? <div className="form-error" role="alert">{t('effectiveness.baselineDialog.theBaselineHasJustChanged')}<Button onClick={load}>{t('effectiveness.baselineDialog.reload')}</Button></div> : null}
            {message ? <div className="form-error" role="alert">{message}</div> : null}
            <form className="editor-grid eff-baseline-form" onSubmit={submit}>
              <label className="field">{t('effectiveness.baselineDialog.medianLeadTimeMinutes')}<input name="minutes" type="number" min="1" max="525600" step="any" required defaultValue={active?.valueSeconds ? active.valueSeconds / 60 : undefined} /></label>
              <label className="field">{t('effectiveness.baselineDialog.effectiveDate')}<input name="effectiveFrom" type="date" required defaultValue={today()} /></label>
              <label className="field eff-span">{t('effectiveness.baselineDialog.measurementSource')}<textarea name="source" required maxLength={2000} placeholder={t('effectiveness.baselineDialog.methodWorkTypeAndMeasurement')} /></label>
              <label className="field">{t('effectiveness.baselineDialog.sampleSize')}<input name="sampleSize" type="number" min="1" step="1" /></label>
              <fieldset className="eff-range"><legend>{t('effectiveness.baselineDialog.measurementPeriod')}</legend>
                <label>{t('effectiveness.baselineDialog.from')}<input name="measuredFrom" type="date" aria-label={t('effectiveness.baselineDialog.measurementPeriodFrom')} /></label>
                <label>{t('effectiveness.baselineDialog.to')}<input name="measuredTo" type="date" aria-label={t('effectiveness.baselineDialog.measurementPeriodTo')} /></label>
              </fieldset>
              <div className="form-actions eff-span">
                {active ? <Button type="button" onClick={retire} disabled={phase === 'saving'}>{t('effectiveness.baselineDialog.retire')}</Button> : null}
                <Button tone="primary" type="submit" disabled={phase === 'saving'}>{phase === 'saving' ? t('effectiveness.baselineDialog.saving') : t('effectiveness.baselineDialog.saveNewVersion')}</Button>
              </div>
            </form>
            <details className="eff-help">
              <summary>{t('effectiveness.baselineDialog.howTheComparisonWorks')}</summary>
              <p>{t('effectiveness.baselineDialog.comparedWithTheMedianLead')}</p>
            </details>
            {versions.length ? <details className="eff-help"><summary>{t('effectiveness.baselineDialog.versionHistory')}</summary>
              <ul className="eff-help-list">{versions.map((b) => <li key={b.id}>{versionLine(b)}<p>{b.source}</p></li>)}</ul>
            </details> : null}
          </>}
  </Dialog>;
}
