import { useCallback, useEffect, useRef, useState } from 'react';
import { ApiError, describeError } from '../../../shared/api/client';
import { Button } from '../../../shared/ui/Button';
import { Dialog } from '../../../shared/ui/Dialog';
import { t } from '../../../shared/i18n';
import { deleteApplication, previewApplicationDeletion, type Admission, type DeletionPreview } from './api';
import { codeLabel } from './labels';

type Phase = 'loading' | 'error' | 'ready' | 'blocked' | 'submitting' | 'stale' | 'lost';

/** Confirmation of UC-18 MS-01..04: the server computes the scope; nothing is sent until the exact name is typed. */
export function DeleteApplicationDialog({ applicationId, onClose, onAccepted, onLost, now = () => Date.now() }: {
  applicationId: string; onClose(): void; onAccepted(admission: Admission): void; onLost(): void; now?(): number;
}) {
  const [phase, setPhase] = useState<Phase>('loading');
  const [preview, setPreview] = useState<DeletionPreview>();
  const [typed, setTyped] = useState('');
  const [message, setMessage] = useState('');
  const request = useRef(0);

  const load = useCallback(() => {
    const current = ++request.current;
    setPhase('loading'); setMessage(''); setTyped('');
    previewApplicationDeletion(applicationId).then((value) => {
      if (current !== request.current) return;
      setPreview(value);
      setPhase(value.blocked || !value.token ? 'blocked' : 'ready');
    }).catch((err: unknown) => {
      if (current !== request.current) return;
      setPreview(undefined); setPhase('error');
      setMessage(describeError(err, t('applications.deletion.couldNotLoadScope')));
    });
  }, [applicationId]);

  useEffect(() => { load(); return () => { request.current += 1; }; }, [load]);

  // An expired preview has to be reviewed again; the typed name is not reused.
  useEffect(() => {
    if (!preview?.expiresAt || phase !== 'ready') return;
    const left = Date.parse(preview.expiresAt) - now();
    const expire = () => { setPhase('stale'); setMessage(t('applications.deletion.previewExpired')); setTyped(''); };
    if (!Number.isFinite(left) || left <= 0) { expire(); return; }
    const timer = setTimeout(expire, Math.min(left, 2 ** 31 - 1));
    return () => clearTimeout(timer);
  }, [preview, phase, now]);

  const canConfirm = phase === 'ready' && preview?.token !== undefined && typed === preview.confirmationName;

  async function confirm() {
    if (!canConfirm || !preview?.token) return;
    const current = ++request.current;
    setPhase('submitting'); setMessage('');
    try {
      const admission = await deleteApplication(applicationId, preview.token, typed);
      if (current === request.current) onAccepted(admission);
    } catch (err) {
      if (current !== request.current) return;
      setTyped('');
      if (!(err instanceof ApiError)) {
        // The request may have been admitted: it is never sent again automatically.
        setPhase('lost'); setMessage(t('applications.deletion.noResponse'));
        return;
      }
      if (err.code === 'APPLICATION_CONFIRMATION_MISMATCH') { setPhase('ready'); setMessage(err.message); return; }
      if (err.status === 409 || err.status === 404) { setPhase('stale'); setMessage(t('applications.deletion.reviewAgain', { message: err.message })); return; }
      setPhase('ready'); setMessage(err.message);
    }
  }

  const busy = phase === 'submitting';
  return <Dialog label={t('applications.deletion.title')} busy={busy} onClose={onClose}>
    <h2>{t('applications.deletion.title')}</h2>
    <p className="warning-text"><strong>{t('applications.deletion.permanent')}</strong> {t('applications.deletion.permanentDetail')}</p>
    {phase === 'loading' ? <p role="status" aria-busy="true">{t('applications.deletion.loadingScope')}</p> : null}
    {message ? <div className="form-error" role="alert">{message}</div> : null}
    {preview && phase !== 'loading' && phase !== 'error' ? <>
      {preview.environments.map((env) => <section key={env.key} className="deletion-scope" aria-label={env.key}>
        <h3>{env.key}</h3>
        <ul>
          <li>{t('applications.deletion.workloadsCount', { count: env.workloads.length + env.pendingDrafts.length })}{env.workloads.length ? `: ${env.workloads.join(', ')}` : ''}</li>
          <li>{t('applications.deletion.resourcesCount', { count: env.resources.length })}</li>
          <li>{t('applications.deletion.secretsCount', { count: env.secretValues })}</li>
        </ul>
        {env.resources.length ? <details><summary>{t('applications.deletion.showResources')}</summary><ul>{env.resources.map((r) => <li key={r.id}><code>{r.descriptor}</code> · {r.status}{r.generation ? ` · gen ${r.generation}` : ''}</li>)}</ul></details> : null}
        {env.blockers.length ? <ul className="form-error" role="alert">{env.blockers.map((b, i) => <li key={`${b.code}-${b.resourceId ?? b.operationId ?? i}`}>{codeLabel(b.code)}{b.descriptor ? <> · <code>{b.descriptor}</code></> : null}</li>)}</ul> : null}
      </section>)}
      <section aria-label={t('applications.deletion.kept')}>
        <h3>{t('applications.deletion.kept')}</h3>
        <p className="muted">{t('applications.deletion.keptDetail', { stores: preview.kept.secretStores.join(', ') || '—', platform: preview.kept.platformResources })}</p>
        {preview.warnings.map((w) => <p key={w} className="form-info">{codeLabel(w)}{preview.kept.definitions.length ? `: ${preview.kept.definitions.join(', ')}` : ''}</p>)}
      </section>
    </> : null}
    {phase === 'error' ? <div className="form-actions"><Button onClick={load}>{t('applications.deletion.retryLoad')}</Button><Button onClick={onClose}>{t('applications.deletion.close')}</Button></div> : null}
    {phase === 'blocked' ? <div className="form-actions"><Button onClick={load}>{t('applications.deletion.checkAgain')}</Button><Button onClick={onClose}>{t('applications.deletion.close')}</Button></div> : null}
    {phase === 'stale' ? <div className="form-actions"><Button tone="primary" onClick={load}>{t('applications.deletion.reviewScope')}</Button><Button onClick={onClose}>{t('applications.deletion.close')}</Button></div> : null}
    {phase === 'lost' ? <div className="form-actions"><Button tone="primary" onClick={onLost}>{t('applications.deletion.reloadStatus')}</Button><Button onClick={onClose}>{t('applications.deletion.close')}</Button></div> : null}
    {preview && (phase === 'ready' || phase === 'submitting') ? <>
      <label className="field">{t('applications.deletion.typeName', { name: preview.confirmationName })}<input value={typed} onChange={(event) => { setTyped(event.target.value); setMessage(''); }} disabled={busy} autoComplete="off" spellCheck={false} /></label>
      <div className="form-actions"><Button tone="danger" disabled={!canConfirm} onClick={() => void confirm()}>{busy ? t('applications.deletion.sending') : t('applications.deletion.confirm')}</Button><Button disabled={busy} onClick={onClose}>{t('applications.deletion.cancel')}</Button></div>
    </> : null}
  </Dialog>;
}
