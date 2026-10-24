import { useCallback, useEffect, useRef, useState } from 'react';
import { Button } from '../../shared/ui/Button';
import type { EnvironmentKey } from '../../shared/types/application';
import { getCleanup, isTerminalStatus, provenSuccess, type CleanupOperation } from './api';
import { failureLabel, operationStatusLabel, outcomeLabel, reasonCodeLabel, stageLabel } from './labels';
import { t } from '../../shared/i18n';

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
      if (current === latest.current) setError(t('resources.cleanupProgress.couldNotReadTheDeletion'));
    });
    return () => { latest.current += 1; if (timer) clearTimeout(timer); };
  }, [applicationId, environment, operationId, intervalMs, tick]);

  const reload = useCallback(() => { setError(''); setTick((n) => n + 1); }, []);
  return <section className="content-panel" aria-label={t('resources.cleanupProgress.resourceDeletionProgress')} data-testid="cleanup-progress">
    <h2>{t('resources.cleanupProgress.resourceDeletionProgress')}</h2>
    {error ? <div className="form-error" role="alert">{error} <Button onClick={reload}>{t('resources.cleanupProgress.reloadProgress')}</Button></div> : null}
    {!operation ? (error ? null : <p role="status" aria-busy="true">{t('resources.cleanupProgress.loadingDeletionProgress')}</p>) : <>
      <ol className="stage-list" aria-label={t('resources.cleanupProgress.phases')}>{stages.map((stage) => <li key={stage} data-state={stage === operation.stage && running(operation.status) ? 'current' : undefined} aria-current={stage === operation.stage && running(operation.status) ? 'step' : undefined}>{stageLabel(stage)}</li>)}</ol>
      <p role="status">{t('resources.cleanupProgress.status')}{' '} <strong>{operationStatusLabel(operation.status)}</strong>{running(operation.status) ? ` · ${stageLabel(operation.stage)}` : ''}</p>
      {provenSuccess(operation) ? <div className="form-success" role="status">{t('resources.cleanupProgress.allSelectedResourcesWereVerified')}</div> : null}
      {operation.status === 'SUCCEEDED' && !provenSuccess(operation) ? <div className="form-error" role="alert">{t('resources.cleanupProgress.theOperationReportedCompletionBut')}</div> : null}
      {!isTerminalStatus(operation.status) && !running(operation.status) && operation.status !== 'INTERRUPTED' ? <div className="form-error" role="alert">{t('resources.cleanupProgress.theDeletionStatusIsNot')}</div> : null}
      {operation.status === 'FAILED' ? <div className="form-error" role="alert">{t('resources.cleanupProgress.failureLabelResourcesNotVerifiedAs', { failureLabel: failureLabel(operation.failureCode ?? 'CLEANUP_FAILED') })}</div> : null}
      {operation.status === 'INTERRUPTED' ? <div className="form-error" role="alert">{t('resources.cleanupProgress.theOperationWasInterruptedOr')}</div> : null}
      <table className="plain-table"><thead><tr><th>{t('resources.cleanupProgress.resource')}</th><th>{t('resources.cleanupProgress.result')}</th><th>{t('resources.cleanupProgress.verifiedGone')}</th><th>{t('resources.cleanupProgress.remaining')}</th></tr></thead><tbody>{operation.resources.map((item) => <tr key={item.id}>
        <td>{item.descriptor}</td>
        <td>{outcomeLabel(item.outcome)}{item.reasonCode ? <><br /><small>{reasonCodeLabel(item.reasonCode)}</small></> : null}</td>
        <td>{item.verified ? t('resources.cleanupProgress.yes') : t('resources.cleanupProgress.no')}</td>
        <td>{item.verified ? '—' : item.remaining.length ? <ul>{item.remaining.map((id) => <li key={id}><code>{id}</code></li>)}</ul> : item.remainingUnknown ? t('resources.cleanupProgress.unknownCannotBeTreatedAs') : '—'}</td></tr>)}</tbody></table>
      {!operation.allVerified && operation.remainingIds.length ? <p>{t('resources.cleanupProgress.countPhysicalObjectsNotYet', { count: operation.remainingIds.length })}</p> : null}
    </>}
  </section>;
}
