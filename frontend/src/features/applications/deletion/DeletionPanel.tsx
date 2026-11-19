import { useEffect, useRef, useState } from 'react';
import { describeError } from '../../../shared/api/client';
import { Button } from '../../../shared/ui/Button';
import { t } from '../../../shared/i18n';
import { getApplicationDeletion, type DeletionProgress } from './api';
import { DeleteApplicationDialog } from './DeleteApplicationDialog';
import { codeLabel, stageLabel, statusLabel } from './labels';

/** Persisted UC-18 progress. Polling reads state only and never resends the deletion. */
export function DeletionPanel({ applicationId, operationId, onDeleted, onOperation, pollMs = 3000 }: {
  applicationId: string; operationId: string; onDeleted(): void; onOperation(operationId: string): void; pollMs?: number;
}) {
  const [progress, setProgress] = useState<DeletionProgress>();
  const [error, setError] = useState('');
  const [confirmation, setConfirmation] = useState<'retry' | 'force'>();
  const [tick, setTick] = useState(0);
  const done = useRef(false);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const read = () => {
      getApplicationDeletion(applicationId, operationId).then((value) => {
        if (cancelled) return;
        setProgress(value); setError('');
        if (value.applicationStatus === 'DELETED') { if (!done.current) { done.current = true; onDeleted(); } return; }
        if (value.running) timer = setTimeout(read, pollMs);
      }).catch((err: unknown) => {
        if (cancelled) return;
        // A failed read keeps the last known state and tries again later.
        setError(describeError(err, t('applications.deletion.couldNotReadProgress')));
        timer = setTimeout(read, pollMs);
      });
    };
    read();
    return () => { cancelled = true; if (timer) clearTimeout(timer); };
  }, [applicationId, operationId, pollMs, onDeleted, tick]);

  const elapsed = (started: string, updated: string) => {
    const seconds = Math.max(0, Math.floor((Date.parse(updated) - Date.parse(started)) / 1000));
    return t('applications.deletion.elapsed', { minutes: Math.floor(seconds / 60), seconds: seconds % 60 });
  };
  const failed = progress !== undefined && !progress.running && progress.status === 'FAILED';
  return <section className="content-panel deletion-panel" aria-label={t('applications.deletion.progressTitle')}>
    <div className="section-header"><div><h2>{t('applications.deletion.progressTitle')}</h2><p>{progress ? statusLabel(progress.status) : t('applications.deletion.loadingProgress')}{progress?.failureCode ? ` · ${codeLabel(progress.failureCode)}` : ''}</p></div>
      {failed ? <div className="form-actions"><Button onClick={() => setConfirmation('retry')}>{t('applications.deletion.retry')}</Button><Button tone="danger" onClick={() => setConfirmation('force')}>{t('applications.deletion.forceTitle')}</Button></div> : null}</div>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {progress ? <p className="muted">{t('applications.deletion.started', { time: new Date(progress.requestedAt).toLocaleString() })} · {elapsed(progress.requestedAt, progress.finishedAt ?? progress.updatedAt)}</p> : null}
    {progress ? <ol className="deletion-environments">{progress.environments.map((env) => <li key={env.key}>
      <strong>{env.key}</strong> · {stageLabel(env.stage)} · {statusLabel(env.status)}{env.failureCode ? ` · ${codeLabel(env.failureCode)}` : ''}
      {env.resources.length ? <ul>{env.resources.map((r) => <li key={r.id}><code>{r.descriptor}</code> · {t(`applications.deletion.resource.${r.outcome}`)}{r.reasonCode ? ` · ${r.reasonCode}` : ''}{r.remaining?.length ? ` · ${r.remaining.join(', ')}` : ''}{r.remainingUnknown ? ` · ${t('applications.deletion.remainingUnknown')}` : ''}</li>)}</ul> : null}
    </li>)}</ol> : null}
    {confirmation ? <DeleteApplicationDialog applicationId={applicationId} force={confirmation === 'force'} onClose={() => setConfirmation(undefined)} onLost={() => { setConfirmation(undefined); setTick((v) => v + 1); }}
      onAccepted={(admission) => { setConfirmation(undefined); onOperation(admission.operationId); setTick((v) => v + 1); }} /> : null}
  </section>;
}
