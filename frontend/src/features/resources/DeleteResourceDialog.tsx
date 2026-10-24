import { useCallback, useEffect, useId, useRef, useState, type KeyboardEvent } from 'react';
import { ApiError, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import type { EnvironmentKey } from '../../shared/types/application';
import { previewDeletion, requestDeletion, responseLost, type Admission, type DeletionPreview } from './api';
import { consumerKindLabel, refusalLabel, resourceStatusLabel } from './labels';
import { availability, chosen, initialSelection, tick, untick, validSelection } from './selection';
import { t } from '../../shared/i18n';

type Phase = 'loading' | 'error' | 'ready' | 'submitting' | 'stale' | 'lost';

/** Dialog of the confirmed deletion (UC-07 DEL-MS-02..04, OC-19). Nothing is sent until the exact name is typed. */
export function DeleteResourceDialog({ applicationId, environment, resourceId, onClose, onAccepted, onLost, now = () => Date.now() }: {
  applicationId: string; environment: EnvironmentKey; resourceId: string; onClose(): void; onAccepted(admission: Admission): void; onLost(): void; now?(): number;
}) {
  const [phase, setPhase] = useState<Phase>('loading');
  const [preview, setPreview] = useState<DeletionPreview>();
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set());
  const [typed, setTyped] = useState('');
  const [message, setMessage] = useState('');
  const [explanation, setExplanation] = useState('');
  const requestId = useRef(0);
  const dialog = useRef<HTMLDivElement>(null);
  const heading = useId();
  const warning = useId();
  const reviewed = useRef<ReadonlySet<string>>(new Set());

  // One preview read at a time: a reply that arrives for an older call, another resource or another
  // Environment is dropped. Nothing is ever auto-retried.
  const load = useCallback((keep: ReadonlySet<string>) => {
    const current = ++requestId.current;
    setPhase('loading'); setMessage(''); setExplanation(''); setTyped('');
    previewDeletion(applicationId, environment, resourceId).then((value) => {
      if (current !== requestId.current) return;
      const plan = { root: value.root, candidates: value.candidates };
      // Ticks the person made before stay only while they are still valid; nothing new is ticked.
      const next = new Set(initialSelection(plan));
      for (const candidate of value.candidates) if (keep.has(candidate.id) && validSelection(plan, new Set([...next, candidate.id]))) next.add(candidate.id);
      setPreview(value); setSelected(validSelection(plan, next) ? next : initialSelection(plan)); setPhase('ready');
    }).catch((err: unknown) => {
      if (current !== requestId.current) return;
      setPreview(undefined); setSelected(new Set()); setPhase('error');
      setMessage(describeError(err, t('resources.deleteResourceDialog.couldNotLoadTheDeletion')));
    });
  }, [applicationId, environment, resourceId]);

  useEffect(() => { load(new Set()); return () => { requestId.current += 1; }; }, [load]);
  useEffect(() => { dialog.current?.focus(); }, []);

  // A preview is only valid for a short time; an expired one has to be reviewed again.
  useEffect(() => {
    if (!preview || phase !== 'ready') return;
    const left = Date.parse(preview.expiresAt) - now();
    const expire = () => { reviewed.current = selected; setPhase('stale'); setMessage(t('resources.deleteResourceDialog.theDeletionScopePreviewHas')); setTyped(''); };
    if (!Number.isFinite(left) || left <= 0) { expire(); return; }
    const timer = setTimeout(expire, Math.min(left, 2 ** 31 - 1));
    return () => clearTimeout(timer);
  }, [preview, phase, selected, now]);

  const plan = preview ? { root: preview.root, candidates: preview.candidates } : undefined;
  const exact = preview !== undefined && typed === preview.confirmationName;
  const canConfirm = phase === 'ready' && plan !== undefined && validSelection(plan, selected) && exact;

  function change(id: string, on: boolean) {
    if (!plan || phase !== 'ready') return;
    setTyped(''); setMessage('');
    if (on) { setExplanation(''); setSelected(tick(plan, selected, id).selected); return; }
    const result = untick(plan, selected, id);
    setSelected(result.selected);
    const names = result.removed.map((removedId) => plan.candidates.find((c) => c.id === removedId)?.descriptor).filter((name): name is string => Boolean(name));
    setExplanation(names.length ? t('resources.deleteResourceDialog.automaticallyDeselectedNamesBecauseThe', { names: names.join(', ') }) : '');
  }

  async function confirm() {
    if (!canConfirm || !preview || !plan) return;
    const body = { token: preview.token, confirmationName: typed, selectedResourceIds: chosen(plan, selected).map((c) => c.id) };
    const current = ++requestId.current;
    setPhase('submitting'); setMessage('');
    try {
      const admission = await requestDeletion(applicationId, environment, resourceId, body);
      if (current === requestId.current) onAccepted(admission);
    } catch (err) {
      if (current !== requestId.current) return;
      setTyped('');
      if (responseLost(err)) {
        // The request may have been admitted: never send it again. The person reloads the persisted state.
        setPhase('lost');
        setMessage(t('resources.deleteResourceDialog.noResponseSoItIs'));
        return;
      }
      const apiError = err as ApiError;
      if (apiError.code === 'RESOURCE_CONFIRMATION_MISMATCH') { setPhase('ready'); setMessage(apiError.message); return; }
      if (apiError.status === 409 || apiError.status === 404) {
        reviewed.current = selected; setPhase('stale');
        setMessage(t('resources.deleteResourceDialog.messageReviewTheNewScope', { message: apiError.message }));
        return;
      }
      setPhase('ready'); setMessage(apiError.message);
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape' && phase !== 'submitting') { event.stopPropagation(); onClose(); return; }
    if (event.key !== 'Tab' || !dialog.current) return;
    const focusable = [...dialog.current.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)')];
    const first = focusable[0], last = focusable[focusable.length - 1];
    if (!first || !last) { event.preventDefault(); dialog.current.focus(); return; }
    if (event.shiftKey && (document.activeElement === first || document.activeElement === dialog.current)) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
  }

  const rows = plan ? plan.candidates : [];
  const picked = plan ? chosen(plan, selected) : [];
  return <div className="dialog-backdrop">
    <div className="dialog" role="dialog" aria-modal="true" aria-labelledby={heading} aria-describedby={warning} tabIndex={-1} ref={dialog} onKeyDown={onKeyDown}>
      <h2 id={heading}>{t('resources.deleteResourceDialog.deleteResource')}</h2>
      {preview ? <p className="muted">{t('resources.deleteResourceDialog.application')}{' '} <strong>{preview.applicationId}</strong> {' '}{t('resources.deleteResourceDialog.environment')}{' '} <strong>{preview.environment}</strong> {' '}{t('resources.deleteResourceDialog.targetGeneration')}{' '} {preview.generation}</p> : null}
      <p id={warning} className="warning-text"><strong>{t('resources.deleteResourceDialog.permanentlyDeletesDataCannotBe')}</strong> {' '}{t('resources.deleteResourceDialog.theResourceAndItsData')}</p>
      {phase === 'loading' ? <p role="status" aria-busy="true">{t('resources.deleteResourceDialog.loadingDeletionScope')}</p> : null}
      {message ? <div className={phase === 'lost' || phase === 'stale' || phase === 'error' || phase === 'ready' ? 'form-error' : 'form-info'} role="alert">{message}</div> : null}
      {phase === 'error' ? <div className="form-actions"><Button onClick={() => load(new Set())}>{t('resources.deleteResourceDialog.retry')}</Button><Button onClick={onClose}>{t('resources.deleteResourceDialog.close')}</Button></div> : null}
      {phase === 'stale' ? <div className="form-actions"><Button tone="primary" onClick={() => load(reviewed.current)}>{t('resources.deleteResourceDialog.reviewScope')}</Button><Button onClick={onClose}>{t('resources.deleteResourceDialog.close')}</Button></div> : null}
      {phase === 'lost' ? <div className="form-actions"><Button tone="primary" onClick={onLost}>{t('resources.deleteResourceDialog.reloadResourceStatus')}</Button><Button onClick={onClose}>{t('resources.deleteResourceDialog.close')}</Button></div> : null}
      {preview && plan && (phase === 'ready' || phase === 'submitting') ? <>
        <p>{t('resources.deleteResourceDialog.technicalNameOfTheResource')}{' '} <code data-testid="root-name">{preview.confirmationName}</code></p>
        <fieldset className="form-fieldset" disabled={phase === 'submitting'}>
          <legend>{t('resources.deleteResourceDialog.deletionScope')}</legend>
          <ul className="selection-list">
            <li><label className="checkbox"><input type="checkbox" checked disabled aria-describedby={`${heading}-root`} /><span><code>{plan.root.descriptor}</code> {' '}{t('resources.deleteResourceDialog.rootResource')}</span></label><small id={`${heading}-root`}>{t('resources.deleteResourceDialog.alwaysInTheDeletionSet')}</small></li>
            {rows.map((candidate) => {
              const state = availability(candidate, selected);
              const isOn = selected.has(candidate.id);
              const names = state.available ? [] : state.waitingFor.map((id) => (id === plan.root.id ? plan.root : plan.candidates.find((c) => c.id === id))?.descriptor ?? t('resources.deleteResourceDialog.otherResources'));
              const reasonId = `${heading}-${candidate.id}`;
              return <li key={candidate.id}>
                <label className="checkbox"><input type="checkbox" checked={isOn} disabled={!isOn && !state.available} aria-describedby={isOn ? undefined : reasonId} onChange={(event) => change(candidate.id, event.target.checked)} /><span><code>{candidate.descriptor}</code> · {resourceStatusLabel(candidate.status)}</span></label>
                {!isOn && !state.available ? <small id={reasonId} className="muted">{candidate.selectable
                  ? t('resources.deleteResourceDialog.cannotSelectYetSelectNames', { names: names.join(', ') })
                  : t('resources.deleteResourceDialog.cannotSelectValueValue2', { value: state.reasons.map(refusalLabel).join(' '), value2: candidate.consumers.length ? t('resources.deleteResourceDialog.usedByValue', { value: candidate.consumers.map((c) => `${consumerKindLabel(c.kind)} ${c.name}`).join(', ') }) : '' })}</small> : null}
              </li>;
            })}
          </ul>
          {rows.length === 0 ? <p className="muted">{t('resources.deleteResourceDialog.thisResourceHasNoDependencies')}</p> : null}
        </fieldset>
        {explanation ? <div className="form-info" role="status">{explanation}</div> : null}
        <section aria-label={t('resources.deleteResourceDialog.resourcesToBeDeleted')}><h3>{t('resources.deleteResourceDialog.pickedCountResourcesWillBeDeleted', { pickedCount: picked.length })}</h3><ul>{picked.map((c) => <li key={c.id}><code>{c.descriptor}</code></li>)}</ul></section>
        <label className="field">{t('resources.deleteResourceDialog.reEnterTheResourceName')}<input value={typed} onChange={(event) => { setTyped(event.target.value); setMessage(''); }} disabled={phase === 'submitting'} autoComplete="off" spellCheck={false} aria-describedby={warning} /></label>
        <div className="form-actions"><Button tone="danger" disabled={!canConfirm} onClick={() => void confirm()}>{phase === 'submitting' ? t('resources.deleteResourceDialog.sendingDeletionRequest') : t('resources.deleteResourceDialog.confirmPermanentDeletion')}</Button><Button disabled={phase === 'submitting'} onClick={onClose}>{t('resources.deleteResourceDialog.cancel')}</Button></div>
      </> : null}
    </div>
  </div>;
}
