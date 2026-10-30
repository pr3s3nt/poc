import { useEffect, useRef, useState } from 'react';
import { api, ApiError, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { t, intlLocale } from '../../shared/i18n';
import { fetchEnvironment } from '../environment/api';
import type { EnvironmentKey } from '../../shared/types/application';
import type { DriftScan, InfrastructureReport } from './InfrastructureDriftPanel';

type RepairReview = { id: string; status: string; expiresAt: string; blockers: string[]; instances: { instanceId: string; report: InfrastructureReport; blockers: string[] }[]; consumedOperationId?: string };
type View = { review: RepairReview; reviewToken?: string };
type Phase = 'select' | 'preparing' | 'review' | 'confirming' | 'unknown' | 'finished';

export function InfrastructureRepairDialog({ applicationId, environment, scan, onClose, onComplete }: { applicationId: string; environment: EnvironmentKey; scan: DriftScan; onClose(): void; onComplete(): void }) {
  const path = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}`;
  const candidates = scan.findings.filter(finding => finding.result === 'DRIFTED' && Boolean(finding.report.proposedActions?.length));
  const [selected, setSelected] = useState<string[]>(candidates.map(finding => finding.instanceId));
  const [phase, setPhase] = useState<Phase>('select');
  const [view, setView] = useState<View>();
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState('');
  const [expired, setExpired] = useState(false);
  const [operationId, setOperationId] = useState('');
  const prepareRequest = useRef<{ scanId: string; instanceIds: string[]; requestId: string; expectedEnvironmentVersion: number } | undefined>(undefined);
  const confirmRequest = useRef<{ reviewId: string; reviewToken: string; requestId: string } | undefined>(undefined);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
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
        const result = await api<{ operations: { id: string; status: string }[] }>(`${path}/operations`);
        const operation = result.operations.find(item => item.id === operationId);
        if (!current) return;
        if (operation?.status === 'SUCCEEDED') { setPhase('finished'); return; }
        if (operation && ['FAILED', 'INTERRUPTED', 'RECOVERED'].includes(operation.status)) { setPhase('unknown'); setError(t('infrastructureDrift.repairIncomplete')); return; }
      } catch { if (current) setError(t('infrastructureDrift.repairUnconfirmed')); }
      if (current) timer = setTimeout(() => void refresh(), 2000);
    };
    void refresh(); return () => { current = false; clearTimeout(timer); };
  }, [operationId, path]);

  async function prepare() {
    if (!selected.length) return;
    setPhase('preparing'); setError(''); setConfirmed(false); setExpired(false);
    try {
      let command = prepareRequest.current;
      if (!command) {
        const target = await fetchEnvironment(applicationId, environment);
        if (!target || target.activeOperation) throw new Error('Environment busy');
        command = { scanId: scan.id, instanceIds: selected, requestId: crypto.randomUUID(), expectedEnvironmentVersion: target.version };
        prepareRequest.current = command;
      }
      const reply = await api<View>(`${path}/drift-repairs/preview`, { method: 'POST', body: JSON.stringify(command) });
      if (alive.current) { setView(reply); setPhase('review'); }
    } catch (error) { if (alive.current) { setError(describeError(error, t('infrastructureDrift.repairUnconfirmed'))); setPhase('select'); } }
  }
  async function confirm() {
    if (!view?.reviewToken || expired || view.review.status !== 'READY' || !confirmed) return;
    setPhase('confirming'); setError('');
    const command = confirmRequest.current ?? { reviewId: view.review.id, reviewToken: view.reviewToken, requestId: crypto.randomUUID() };
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
  const busy = phase === 'preparing' || phase === 'confirming';
  const ready = phase === 'review' && view?.review.status === 'READY' && Boolean(view.reviewToken) && !expired;
  return <Dialog label={t('infrastructureDrift.repairPreview')} busy={busy} onClose={onClose}>
    <h2>{t('infrastructureDrift.repairPreview')}</h2>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    {phase === 'select' || phase === 'preparing' ? <>
      <fieldset disabled={busy || Boolean(prepareRequest.current)}><legend>{t('infrastructureDrift.selectedInstances')}</legend>{candidates.map(candidate => <label className="checkbox" key={candidate.instanceId}><input type="checkbox" checked={selected.includes(candidate.instanceId)} onChange={event => setSelected(current => event.target.checked ? [...current, candidate.instanceId] : current.filter(id => id !== candidate.instanceId))} />{candidate.descriptor || candidate.instanceId}</label>)}</fieldset>
      <footer className="form-actions"><Button disabled={busy} onClick={onClose}>{t('infrastructureDrift.cancel')}</Button><Button tone="primary" disabled={busy || !selected.length} onClick={() => void prepare()}>{busy ? t('infrastructureDrift.preparing') : t('infrastructureDrift.prepare')}</Button></footer>
    </> : phase === 'finished' ? <><p role="status">{t('infrastructureDrift.repairSucceeded')}</p><Button tone="primary" onClick={onComplete}>{t('infrastructureDrift.done')}</Button></> : <>
      {view?.review.instances.map(instance => <div key={instance.instanceId}><h3>{candidates.find(candidate => candidate.instanceId === instance.instanceId)?.descriptor || instance.instanceId}</h3><table className="plain-table"><thead><tr><th>{t('infrastructureDrift.resource')}</th><th>{t('infrastructureDrift.action')}</th><th>{t('infrastructureDrift.fields')}</th></tr></thead><tbody>{instance.report.proposedActions?.map(item => <tr key={item.address}><td>{item.address}</td><td>{item.actions.join(', ')}</td><td>{item.paths.join(', ')}</td></tr>)}</tbody></table>{instance.blockers.map(blocker => <p className="form-error" key={blocker}>{t(`infrastructureDrift.blocker.${blocker}`)}</p>)}</div>)}
      {view?.review.blockers.map(blocker => <p className="form-error" key={blocker}>{t(`infrastructureDrift.blocker.${blocker}`)}</p>)}
      {view ? <p>{t('infrastructureDrift.expires')}: {new Date(view.review.expiresAt).toLocaleString(intlLocale())}{expired ? ` · ${t('infrastructureDrift.expired')}` : ''}</p> : null}
      {ready ? <label className="checkbox"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />{t('infrastructureDrift.confirmReview')}</label> : null}
      <footer className="form-actions"><Button disabled={busy} onClick={onClose}>{t('infrastructureDrift.close')}</Button>{ready ? <Button tone="primary" disabled={!confirmed || busy} onClick={() => void confirm()}>{t('infrastructureDrift.confirmRepair')}</Button> : null}{phase === 'unknown' && !operationId ? <Button onClick={() => void confirm()}>{t('infrastructureDrift.retryConfirmation')}</Button> : null}{busy ? <span role="status">{t('infrastructureDrift.repairing')}</span> : null}</footer>
    </>}
  </Dialog>;
}
