import { useEffect, useState } from 'react';
import { api } from '../../shared/api/client';
import { fetchEnvironment } from '../environment/api';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { t, intlLocale } from '../../shared/i18n';
import { InfrastructureRepairDialog } from './InfrastructureRepairDialog';

type Finding = { address: string; type: string; actions: string[]; paths: string[] };
export type InfrastructureReport = { externalDrift: Finding[] | null; proposedActions: Finding[] | null; outputNames: string[] | null; complete: boolean; outputsUnknown: boolean };
type InstanceResult = { instanceId: string; descriptor: string; result: string; report: InfrastructureReport; reasonCode?: string };
export type DriftScan = { id: string; status: string; result: string; findings: InstanceResult[]; createdAt: string; finishedAt?: string; reasonCode?: string; fresh?: boolean; effectiveResult?: string };
const pending = (scan?: DriftScan) => scan?.status === 'QUEUED' || scan?.status === 'RUNNING';

export function InfrastructureDriftPanel({ applicationId, environment, canManage, busy, refreshKey }: { applicationId: string; environment: EnvironmentKey; canManage: boolean; busy: boolean; refreshKey?: string }) {
  const path = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}/drift-scans`;
  const [scans, setScans] = useState<DriftScan[]>();
  const [error, setError] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [retry, setRetry] = useState<{ requestId: string; expectedEnvironmentVersion: number }>();
  const [repair, setRepair] = useState(false);
  useEffect(() => {
    let current = true; let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const reply = await api<{ scans: DriftScan[] }>(`${path}?limit=10`);
        if (!Array.isArray(reply.scans)) throw new Error('Invalid stored scan projection');
        if (current) { setScans(reply.scans); timer = setTimeout(() => void refresh(), reply.scans.some(pending) ? 2000 : 30_000); }
      } catch { if (current) { setError(t('infrastructureDrift.unavailable')); timer = setTimeout(() => void refresh(), 30_000); } }
    };
    void refresh(); return () => { current = false; clearTimeout(timer); };
  }, [path, refreshKey, attempt]);

  async function scan() {
    setSubmitting(true); setError('');
    try {
      let command = retry;
      if (!command) {
        const target = await fetchEnvironment(applicationId, environment);
        if (!target || target.activeOperation) throw new Error('Environment busy');
        command = { requestId: crypto.randomUUID(), expectedEnvironmentVersion: target.version };
      }
      setRetry(command);
      await api(path, { method: 'POST', body: JSON.stringify(command) });
      setRetry(undefined); setAttempt(value => value + 1);
    } catch { setError(t('infrastructureDrift.scanUnconfirmed')); }
    finally { setSubmitting(false); }
  }
  const latest = scans?.[0];
  const timestamp = latest ? Date.parse(latest.finishedAt ?? latest.createdAt) : Number.NaN;
  const fresh = !error && (latest?.fresh ?? (Number.isFinite(timestamp) && timestamp <= Date.now() && Date.now() - timestamp <= 45 * 60_000));
  const result = fresh ? latest?.effectiveResult ?? latest?.result : 'UNKNOWN';
  return <section className="content-panel" aria-label={t('infrastructureDrift.title')}>
    <div className="section-header"><h2>{t('infrastructureDrift.title')}</h2><div><Button disabled={submitting} onClick={() => { setError(''); setAttempt(value => value + 1); }}>{t('infrastructureDrift.reload')}</Button>{canManage ? <Button disabled={submitting || busy || pending(latest)} onClick={() => void scan()}>{submitting ? t('infrastructureDrift.queuing') : retry ? t('infrastructureDrift.retry') : t('infrastructureDrift.scan')}</Button> : null}</div></div>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    {!scans ? <p role="status">{t('infrastructureDrift.loading')}</p> : !latest ? <p>{t('infrastructureDrift.noScan')}</p> : <>
      <p role="status"><strong>{t(`infrastructureDrift.status.${latest.status}`)}</strong> · {t(`infrastructureDrift.result.${result}`)} · {new Date(latest.finishedAt ?? latest.createdAt).toLocaleString(intlLocale())}{fresh ? '' : ` · ${t('infrastructureDrift.stale')}`}</p>
      {canManage && latest.status === 'SUCCEEDED' && latest.findings.some(finding => finding.result === 'DRIFTED' && Boolean(finding.report.proposedActions?.length)) ? <Button disabled={busy || submitting} onClick={() => setRepair(true)}>{t('infrastructureDrift.repairPreview')}</Button> : null}
      <details><summary>{t('infrastructureDrift.details')}</summary>{latest.reasonCode ? <p>{t(`infrastructureDrift.reason.${latest.reasonCode}`)}</p> : null}
        {latest.findings.map(finding => <div key={finding.instanceId}><h3>{finding.descriptor || finding.instanceId}</h3><p>{t(`infrastructureDrift.result.${finding.result}`)}{finding.reasonCode ? ` · ${t(`infrastructureDrift.reason.${finding.reasonCode}`)}` : ''}</p>
          {(['externalDrift', 'proposedActions'] as const).map(kind => finding.report[kind]?.length ? <div key={kind}><h4>{t(`infrastructureDrift.${kind}`)}</h4><table className="plain-table"><thead><tr><th>{t('infrastructureDrift.resource')}</th><th>{t('infrastructureDrift.action')}</th><th>{t('infrastructureDrift.fields')}</th></tr></thead><tbody>{finding.report[kind]?.map(item => <tr key={item.address}><td>{item.address}</td><td>{item.actions.join(', ')}</td><td>{item.paths.join(', ') || '—'}</td></tr>)}</tbody></table></div> : null)}
          {finding.report.outputNames?.length ? <p>{t('infrastructureDrift.outputs')}: {finding.report.outputNames.join(', ')}</p> : null}
        </div>)}
      </details>
    </>}
    {repair && latest ? <InfrastructureRepairDialog applicationId={applicationId} environment={environment} scan={latest} onClose={() => setRepair(false)} onComplete={() => { setRepair(false); setAttempt(value => value + 1); }} /> : null}
  </section>;
}
