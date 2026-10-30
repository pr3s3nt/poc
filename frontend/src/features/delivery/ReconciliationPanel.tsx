import { useEffect, useState } from 'react';
import { api } from '../../shared/api/client';
import { t, intlLocale } from '../../shared/i18n';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';

type Observation = { scope: { unitKey: string }; intendedRevision: string; observedRevision?: string; sync: string; health: string; drift: string; complete: boolean; observedAt: string };
type Unit = { unitId: string; workloadId: string; sequence: number; lastKnown: Observation; effective: Observation; fresh: boolean; reasonCode?: string };
type View = { units: Unit[]; nextCursor: string; intervalSeconds: number };
const label = (value: string) => t(`reconciliation.status.${value}`);

export function ReconciliationPanel({ applicationId, environment, refreshKey }: { applicationId: string; environment: EnvironmentKey; refreshKey?: string }) {
  const [data, setData] = useState<View>();
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    let current = true; let timer: ReturnType<typeof setTimeout>;
    const refresh = async () => {
      try {
        const view = await api<View>(`/applications/${encodeURIComponent(applicationId)}/environments/${environment}/reconciliation?limit=200`);
        if (!Array.isArray(view.units) || view.units.some(unit => !unit.lastKnown || !unit.effective) || !Number.isFinite(view.intervalSeconds)) throw new Error('Invalid reconciliation projection');
        if (current) { setData(view); setError(false); }
      } catch { if (current) setError(true); }
      finally { if (current) { setLoading(false); timer = setTimeout(() => void refresh(), 30_000); } }
    };
    void refresh(); return () => { current = false; clearTimeout(timer); };
  }, [applicationId, environment, refreshKey, attempt]);
  return <section className="content-panel" aria-label={t('reconciliation.title')}>
    <div className="section-header"><h2>{t('reconciliation.title')}</h2><Button disabled={loading} onClick={() => setAttempt(value => value + 1)}>{t('reconciliation.refresh')}</Button></div>
    {loading ? <p role="status">{t('reconciliation.loading')}</p> : null}
    {error ? <p className="form-error" role="alert">{t('reconciliation.unavailable')}</p> : null}
    {data?.units.length === 0 ? <p>{t('reconciliation.waiting')}</p> : null}
    {data?.units.length ? <table className="plain-table"><thead><tr>{['unit', 'sync', 'health', 'drift', 'observed'].map(key => <th key={key}>{t(`reconciliation.${key}`)}</th>)}</tr></thead><tbody>{data.units.map(unit => {
      const timestamp = Date.parse(unit.lastKnown.observedAt);
      const fresh = !error && unit.fresh && timestamp <= Date.now() && Date.now() - timestamp <= data.intervalSeconds * 3000;
      const value = fresh ? unit.effective : { sync: 'UNKNOWN', health: 'UNKNOWN', drift: 'UNKNOWN' };
      return <tr key={unit.unitId}><td><details><summary>{unit.workloadId || t('reconciliation.routes')} · {t(`delivery.phase.${unit.lastKnown.scope.unitKey.split('/').at(-1)}`)}</summary><dl><dt>{t('reconciliation.intended')}</dt><dd>{unit.lastKnown.intendedRevision}</dd><dt>{t('reconciliation.observedRevision')}</dt><dd>{unit.lastKnown.observedRevision || t('reconciliation.status.UNKNOWN')}</dd></dl></details></td><td>{label(value.sync)}</td><td>{label(value.health)}</td><td>{label(value.drift)}</td><td>{Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString(intlLocale()) : '—'}{fresh ? '' : ` · ${t('reconciliation.stale')}`}</td></tr>;
    })}</tbody></table> : null}
    {data?.nextCursor ? <p className="form-info">{t('reconciliation.more')}</p> : null}
  </section>;
}
