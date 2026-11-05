import { useEffect, useRef, useState } from 'react';
import { api, ApiError, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { t, intlLocale } from '../../shared/i18n';
import { fetchEnvironment } from '../environment/api';
import type { EnvironmentKey } from '../../shared/types/application';
import { InfrastructureComparisons } from './InfrastructureComparisons';
import type { DriftScan, InfrastructureReport } from './InfrastructureDriftPanel';

type RepairReview = { rootIds?: string[]; scanId?: string; consumers?: { descriptor: string; workloadId?: string }[]; id: string; status: string; expiresAt: string; blockers: string[]; instances: { descriptor?: string; deferred?: boolean; completed?: boolean; instanceId: string; report: InfrastructureReport; blockers: string[] }[]; consumedOperationId?: string };
type View = { review: RepairReview; reviewToken?: string };
type Phase = 'select' | 'preparing' | 'review' | 'confirming' | 'unknown' | 'finished' | 'next';

export function InfrastructureRepairDialog({ applicationId, environment, scan, previousReviewId, onClose, onComplete }: { applicationId: string; environment: EnvironmentKey; scan: DriftScan; previousReviewId?: string; onClose(): void; onComplete(): void }) {
  const path = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}`;
  const candidates = scan.findings.filter(finding => finding.result === 'DRIFTED');
  const [selected, setSelected] = useState<string[]>(candidates.map(finding => finding.instanceId));
  const [phase, setPhase] = useState<Phase>(previousReviewId ? 'preparing' : 'select');
  const [view, setView] = useState<View>();
  const [confirmed, setConfirmed] = useState(false);
  const [destructiveConfirmed, setDestructiveConfirmed] = useState(false);
  const [error, setError] = useState('');
  const [expired, setExpired] = useState(false);
  const [operationId, setOperationId] = useState('');
  const prepareRequest = useRef<{ scanId: string; instanceIds: string[]; requestId: string; expectedEnvironmentVersion: number; previousReviewId?: string } | undefined>(undefined);
  const confirmRequest = useRef<{ reviewId: string; reviewToken: string; requestId: string; confirmDestructive?: boolean } | undefined>(undefined);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => {
    if (!previousReviewId) return;
    let current = true;
    void api<View>(`${path}/drift-repairs/${encodeURIComponent(previousReviewId)}`).then(reply => {
      if (current) { setView(reply); setPhase('next'); }
    }).catch(() => { if (current) { setError(t('infrastructureDrift.repairUnconfirmed')); setPhase('unknown'); } });
    return () => { current = false; };
  }, [path, previousReviewId]);
  useEffect(() => {
    if (!view || view.review.status !== 'READY') return;
    const expire = () => setExpired(!Number.isFinite(Date.parse(view.review.expiresAt)) || Date.parse(view.review.expiresAt) <= Date.now());
    expire(); const timer = setInterval(expire, 1000); return () => clearInterval(timer);
  }, [view]);
  useEffect(() => {
    if (!operationId) return;
    let current = true; let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const result = await api<{ operations: { id: string; status: string; stage?: string }[] }>(`${path}/operations`);
        const operation = result.operations.find(item => item.id === operationId);
        if (!current) return;
        if (operation?.status === 'FAILED' && operation.stage === 'DRIFT_REPAIR_REVIEW_REQUIRED') { setPhase('next'); setError(''); return; }
        if (operation?.status === 'SUCCEEDED') { setPhase('finished'); return; }
        if (operation && ['FAILED', 'INTERRUPTED', 'RECOVERED'].includes(operation.status)) { setPhase('unknown'); setError(t('infrastructureDrift.repairIncomplete')); return; }
      } catch { if (current) setError(t('infrastructureDrift.repairUnconfirmed')); }
      if (current) timer = setTimeout(() => void refresh(), 2000);
    };
    void refresh(); return () => { current = false; clearTimeout(timer); };
  }, [operationId, path]);

  async function prepare(continuation = false) {
    const selection = continuation ? view?.review.rootIds ?? selected : selected;
    if (!selection.length) return;
    if (continuation) { prepareRequest.current = undefined; confirmRequest.current = undefined; setOperationId(''); }
    setPhase('preparing'); setError(''); setConfirmed(false); setDestructiveConfirmed(false); setExpired(false);
    try {
      let command = prepareRequest.current;
      if (!command) {
        const target = await fetchEnvironment(applicationId, environment);
        if (!target || target.activeOperation) throw new Error('Environment busy');
        command = { scanId: continuation ? view?.review.scanId ?? scan.id : scan.id, instanceIds: selection, ...(continuation && view ? { previousReviewId: view.review.id } : {}), requestId: crypto.randomUUID(), expectedEnvironmentVersion: target.version };
        prepareRequest.current = command;
      }
      const reply = await api<View>(`${path}/drift-repairs/preview`, { method: 'POST', body: JSON.stringify(command) });
      if (alive.current) { setView(reply); setPhase('review'); }
    } catch (error) { if (alive.current) { setError(describeError(error, t('infrastructureDrift.repairUnconfirmed'))); setPhase(continuation ? 'next' : 'select'); } }
  }
  async function confirm() {
    if (!view?.reviewToken || expired || view.review.status !== 'READY' || !confirmed || (destructive && !destructiveConfirmed)) return;
    setPhase('confirming'); setError('');
    const command = confirmRequest.current ?? { reviewId: view.review.id, reviewToken: view.reviewToken, requestId: crypto.randomUUID(), ...(destructive ? { confirmDestructive: true } : {}) };
    confirmRequest.current = command;
    try {
      const reply = await api<{ operationId: string }>(`${path}/drift-repairs`, { method: 'POST', body: JSON.stringify(command) });
      if (alive.current) setOperationId(reply.operationId);
    } catch (error) {
      if (!alive.current) return;
      setError(describeError(error, t('infrastructureDrift.repairUnconfirmed')));
      if (error instanceof ApiError && error.status >= 400 && error.status < 500) { setExpired(true); setPhase('review'); }
      else setPhase('unknown');
    }
  }
  const destructive = Boolean(view?.review.instances.some(instance => !instance.completed && !instance.deferred && instance.report.proposedActions?.some(item => item.actions.includes('delete'))));
  const actionLabel = (actions: string[]) => actions.includes('create') && actions.includes('delete') ? t('infrastructureDrift.actionLabel.replace') : actions.map(action => ['create', 'update', 'delete', 'read', 'no-op'].includes(action) ? t(`infrastructureDrift.actionLabel.${action}`) : action).join(', ');
  const busy = phase === 'preparing' || phase === 'confirming';
  const ready = phase === 'review' && view?.review.status === 'READY' && Boolean(view.reviewToken) && !expired;
  return <Dialog label={t('infrastructureDrift.repairPreview')} busy={busy} onClose={onClose}>
    <h2>{t('infrastructureDrift.repairPreview')}</h2>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    {phase === 'select' || phase === 'preparing' ? <>
      <fieldset disabled={busy || Boolean(prepareRequest.current)}><legend>{t('infrastructureDrift.selectedInstances')}</legend>{candidates.map(candidate => <label className="checkbox" key={candidate.instanceId}><input type="checkbox" checked={selected.includes(candidate.instanceId)} onChange={event => setSelected(current => event.target.checked ? [...current, candidate.instanceId] : current.filter(id => id !== candidate.instanceId))} />{candidate.descriptor || candidate.instanceId}</label>)}</fieldset>
      <footer className="form-actions"><Button disabled={busy} onClick={onClose}>{t('infrastructureDrift.cancel')}</Button><Button tone="primary" disabled={busy || !selected.length} onClick={() => void prepare()}>{busy ? t('infrastructureDrift.preparing') : t('infrastructureDrift.prepare')}</Button></footer>
    </> : phase === 'next' ? <><p role="status">{t('infrastructureDrift.nextReviewRequired')}</p><footer className="form-actions"><Button onClick={onClose}>{t('infrastructureDrift.close')}</Button><Button tone="primary" onClick={() => void prepare(true)}>{t('infrastructureDrift.nextPreview')}</Button></footer></> : phase === 'finished' ? <><p role="status">{t('infrastructureDrift.repairSucceeded')}</p><Button tone="primary" onClick={onComplete}>{t('infrastructureDrift.done')}</Button></> : <>
      {view?.review.instances.map(instance => <div key={instance.instanceId}><h3>{instance.descriptor || candidates.find(candidate => candidate.instanceId === instance.instanceId)?.descriptor || instance.instanceId}</h3>{instance.completed ? <p>{t('infrastructureDrift.memberCompleted')}</p> : instance.deferred ? <p>{t('infrastructureDrift.memberDeferred')}</p> : null}{instance.report.proposedActions?.length ? <table className="plain-table"><thead><tr><th>{t('infrastructureDrift.resource')}</th><th>{t('infrastructureDrift.action')}</th><th>{t('infrastructureDrift.fields')}</th></tr></thead><tbody>{instance.report.proposedActions?.map(item => <tr key={item.address}><td>{item.address}</td><td>{actionLabel(item.actions)}</td><td>{item.paths.join(', ')}</td></tr>)}</tbody></table> : null}<InfrastructureComparisons comparisons={instance.report.comparisons} />{instance.report.outputNames?.length ? <p>{t('infrastructureDrift.outputs')}: {instance.report.outputNames.join(', ')}</p> : null}{instance.report.outputsUnknown ? <p className="warning-text">{t('infrastructureDrift.outputsUnknown')}</p> : null}{instance.blockers.map(blocker => <p className="form-error" key={blocker}>{t(`infrastructureDrift.blocker.${blocker}`)}</p>)}</div>)}
      {view?.review.consumers?.length ? <details open><summary>{t('infrastructureDrift.affectedConsumers')}</summary><ul>{view.review.consumers.map(consumer => <li key={consumer.descriptor}>{consumer.workloadId || consumer.descriptor}</li>)}</ul></details> : null}
      {destructive ? <p className="warning-text" role="alert">{t('infrastructureDrift.destructiveWarning')}</p> : null}
      {view?.review.blockers.map(blocker => <p className="form-error" key={blocker}>{t(`infrastructureDrift.blocker.${blocker}`)}</p>)}
      {view ? <p>{t('infrastructureDrift.expires')}: {new Date(view.review.expiresAt).toLocaleString(intlLocale())}{expired ? ` · ${t('infrastructureDrift.expired')}` : ''}</p> : null}
      {ready ? <label className="checkbox"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />{t('infrastructureDrift.confirmReview')}</label> : null}
      {ready && destructive ? <label className="checkbox"><input type="checkbox" checked={destructiveConfirmed} onChange={event => setDestructiveConfirmed(event.target.checked)} />{t('infrastructureDrift.confirmDestructive')}</label> : null}
      <footer className="form-actions"><Button disabled={busy} onClick={onClose}>{t('infrastructureDrift.close')}</Button>{ready ? <Button tone="primary" disabled={!confirmed || busy || (destructive && !destructiveConfirmed)} onClick={() => void confirm()}>{t('infrastructureDrift.confirmRepair')}</Button> : null}{phase === 'unknown' && !operationId ? <Button onClick={() => void confirm()}>{t('infrastructureDrift.retryConfirmation')}</Button> : null}{busy ? <span role="status">{t('infrastructureDrift.repairing')}</span> : null}</footer>
    </>}
  </Dialog>;
}
