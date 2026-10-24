import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { effectivenessDays, navigate, replaceRoute, type EffectivenessDays } from '../../app/routes';
import { ApiError, describeError } from '../../shared/api/client';
import type { Application } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { getReport, listRecords, periodOf, type Metric, type Report, type Scope } from './api';
import { BaselineDialog } from './BaselineDialog';
import { buildCsv, csvFileName } from './csv';
import { bandDetailKeys, bandLabels, formatDate, formatDuration, formatNumber, metricName, metricNote, metricValue } from './labels';
import { DoraHelpDialog, MetricDialog } from './MetricDialog';
import { t } from '../../shared/i18n';

type Load = { phase: 'loading' } | { phase: 'ready'; report: Report } | { phase: 'error'; message: string; missing: boolean };
type Open = { kind: 'metric'; id: string } | { kind: 'dora' } | { kind: 'baseline' };

const usageIds = ['active_applications', 'active_users', 'completed_work', 'execution_attempts', 'work_duration'];
const shownElsewhere = new Set([...usageIds, 'work_duration_vs_baseline']);
const bands = ['DAILY', 'WEEKLY', 'MONTHLY', 'RARE', 'NONE'] as const;

function MetricCard({ metric, onOpen }: { metric: Metric; onOpen(): void }) {
  const value = metricValue(metric);
  const note = metricNote(metric);
  const muted = metric.status !== 'COLLECTED';
  return <button className={`eff-metric${muted ? ' eff-metric-muted' : ''}`} onClick={onOpen} aria-label={t('effectiveness.effectiveness.metricNameValueViewDetails', { metricName: metricName(metric.id), value })}>
    <span className="eff-metric-title">{metricName(metric.id)}<span aria-hidden="true">↗</span></span>
    <strong>{value}</strong>
    <small className={note?.warning ? 'eff-warning' : undefined}>{note?.text ?? ''}</small>
  </button>;
}

function TrendChart({ report }: { report: Report }) {
  const points = report.trend.points;
  const max = Math.max(1, ...points.map((point) => point.productionDeployments));
  const total = points.reduce((sum, point) => sum + point.productionDeployments, 0);
  return <section className="content-panel">
    <div className="section-header"><h2>{t('effectiveness.effectiveness.productionDeployments')}</h2><span className="muted">{report.trend.bucket === 'DAY' ? t('effectiveness.effectiveness.daily') : t('effectiveness.effectiveness.weekly')}</span></div>
    <div className="eff-chart" role="img" aria-label={t('effectiveness.effectiveness.productionDeploymentsValueTotalTotal', { value: report.trend.bucket === 'DAY' ? t('effectiveness.effectiveness.daily2') : t('effectiveness.effectiveness.weekly2'), total })}>
      {points.map((point) => <div className="eff-bar-group" key={point.start}>
        <strong>{point.productionDeployments}</strong>
        <div className="eff-bar" style={{ height: `${Math.max(3, (point.productionDeployments / max) * 120)}px` }} />
        <small>{formatDate(point.start).slice(0, 5)}</small>
      </div>)}
    </div>
  </section>;
}

function Comparison({ report, onOpen }: { report: Report; onOpen(): void }) {
  const metric = report.metrics.find((m) => m.id === 'work_duration_vs_baseline');
  const workload = report.scope.level === 'WORKLOAD';
  const d = metric?.details ?? {};
  return <section className="content-panel eff-comparison">
    <div className="section-header"><h2>{t('effectiveness.effectiveness.timeSaved')}</h2></div>
    {metric?.status === 'COLLECTED' && metric.value !== undefined ? <>
      <strong className="eff-comparison-value">{formatDuration(metric.value)}</strong>
      <p className="muted">{t('effectiveness.effectiveness.formatDurationFormatDuration2WorkItemMedian', { formatDuration: formatDuration(d.baselineSeconds ?? 0), formatDuration2: formatDuration(d.currentSeconds ?? 0) })}</p>
      <small>{t('effectiveness.effectiveness.processTimeNotPersonHours', { version: report.baseline?.version })}</small>
    </> : <>
      <strong className="eff-comparison-value">—</strong>
      <p className="muted">{workload ? t('effectiveness.effectiveness.theBaselineAppliesToThe') : report.baseline ? t('effectiveness.effectiveness.noDataInThisPeriod') : t('effectiveness.effectiveness.noBaselineYet')}</p>
    </>}
    {workload ? null : <Button onClick={onOpen}>{report.baseline ? t('effectiveness.effectiveness.viewBaseline') : t('effectiveness.effectiveness.addBaseline')}</Button>}
  </section>;
}

/** UC-17 “Hiệu quả platform”: organization overview with drill-down (desktop Console). */
export function EffectivenessPage({ applications, application, workload, days = 30 }: {
  applications: readonly Application[]; application?: string; workload?: string; days?: EffectivenessDays;
}) {
  const [load, setLoad] = useState<Load>({ phase: 'loading' });
  const [open, setOpen] = useState<Open>();
  const [notice, setNotice] = useState('');
  const [exporting, setExporting] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [workloads, setWorkloads] = useState<Record<string, string[]>>({});
  const request = useRef(0);
  const scope = useMemo<Scope>(() => ({ application, workload, ...periodOf(days) }), [application, workload, days]);

  const reload = useCallback(() => setAttempt((value) => value + 1), []);
  useEffect(() => {
    const id = ++request.current;
    setLoad({ phase: 'loading' });
    getReport(scope).then((report) => {
      if (id !== request.current) return;
      setLoad({ phase: 'ready', report });
      if (report.scope.level === 'APPLICATION' && report.breakdown && scope.application) {
        const key = scope.application;
        setWorkloads((current) => ({ ...current, [key]: report.breakdown?.rows.map((row) => row.key) ?? [] }));
      }
    }).catch((err: unknown) => {
      if (id !== request.current) return;
      const missing = err instanceof ApiError && err.status === 404;
      const message = err instanceof ApiError && err.status === 403 ? t('effectiveness.effectiveness.youDoNotHavePermission') : describeError(err, t('effectiveness.effectiveness.couldNotLoadTheReport'));
      setLoad({ phase: 'error', message: missing ? t('effectiveness.effectiveness.applicationNotFound') : message, missing });
    });
  }, [scope, attempt]);

  const go = (next: { application?: string; workload?: string; days?: EffectivenessDays }, replace = false) => {
    const route = { name: 'effectiveness' as const, ...(next.application ? { application: next.application } : {}), ...(next.application && next.workload ? { workload: next.workload } : {}), ...(next.days && next.days !== 30 ? { days: next.days } : {}) };
    setOpen(undefined);
    if (replace) replaceRoute(route); else navigate(route);
  };

  async function exportCsv(report: Report) {
    setExporting(true); setNotice('');
    try {
      const { records } = await listRecords(scope);
      const url = URL.createObjectURL(new Blob([buildCsv(report, records)], { type: 'text/csv;charset=utf-8' }));
      const link = document.createElement('a');
      link.href = url; link.download = csvFileName(report); link.click();
      URL.revokeObjectURL(url);
      setNotice(t('effectiveness.effectiveness.cSVExported'));
    } catch (err) {
      setNotice(describeError(err, t('effectiveness.effectiveness.couldNotExportCSV')));
    } finally {
      setExporting(false);
    }
  }

  const report = load.phase === 'ready' ? load.report : undefined;
  const appName = applications.find((item) => item.id === application)?.name ?? report?.scope.applicationName ?? application;
  const appWorkloads = application ? workloads[application] ?? (workload ? [workload] : []) : [];
  const metric = (id: string) => report?.metrics.find((m) => m.id === id);
  const opened = open?.kind === 'metric' ? metric(open.id) : undefined;
  const frequency = metric('deployment_frequency');

  return <section className="page eff-page">
    <header className="page-header">
      <div><h1>{t('effectiveness.effectiveness.platformEffectiveness')}</h1>{report?.dataOrigin === 'TEST_INSTALLATION' ? <span className="eff-badge">{t('effectiveness.effectiveness.testData')}</span> : null}</div>
      <div className="eff-actions">
        <Button onClick={() => setOpen({ kind: 'baseline' })} disabled={!report}>{t('effectiveness.effectiveness.baseline')}</Button>
        <Button tone="primary" onClick={() => report && void exportCsv(report)} disabled={!report || exporting}>{exporting ? t('effectiveness.effectiveness.exporting') : t('effectiveness.effectiveness.exportCSV')}</Button>
      </div>
    </header>
    <div className="eff-filters">
      <nav className="eff-crumbs" aria-label={t('effectiveness.effectiveness.scope')}>
        {application ? <button className="back-link" onClick={() => go({ days })}>{t('effectiveness.effectiveness.wholeOrganization')}</button> : <strong>{t('effectiveness.effectiveness.wholeOrganization')}</strong>}
        {application ? <><span aria-hidden="true">›</span>{workload ? <button className="back-link" onClick={() => go({ application, days })}>{appName}</button> : <strong>{appName}</strong>}</> : null}
        {workload ? <><span aria-hidden="true">›</span><strong>{workload}</strong></> : null}
      </nav>
      <label>{t('effectiveness.effectiveness.application')}<select value={application ?? ''} onChange={(event) => go({ application: event.target.value || undefined, days })}>
        <option value="">{t('effectiveness.effectiveness.allApplications')}</option>
        {applications.map((item) => <option key={item.id} value={item.id}>{item.name}</option>)}
      </select></label>
      {application ? <label>Workload<select value={workload ?? ''} onChange={(event) => go({ application, workload: event.target.value || undefined, days })}>
        <option value="">{t('effectiveness.effectiveness.allWorkloads')}</option>
        {appWorkloads.map((id) => <option key={id} value={id}>{id}</option>)}
      </select></label> : null}
      <label>{t('effectiveness.effectiveness.period')}<select value={days} onChange={(event) => go({ application, workload, days: Number(event.target.value) as EffectivenessDays }, true)}>
        {effectivenessDays.map((value) => <option key={value} value={value}>{t('effectiveness.effectiveness.valueDays', { value })}</option>)}
      </select></label>
      <small className="eff-period">{t('effectiveness.effectiveness.formatDateFormatDate2VietnamTime', { formatDate: formatDate(scope.from), formatDate2: formatDate(scope.to) })}</small>
    </div>
    {notice ? <div className="form-info eff-notice" role="status">{notice}<Button tone="quiet" aria-label={t('effectiveness.effectiveness.dismissNotification')} onClick={() => setNotice('')}>×</Button></div> : null}
    {load.phase === 'loading' ? <p role="status" aria-busy="true">{t('effectiveness.effectiveness.loadingReport')}</p> : null}
    {load.phase === 'error' ? load.missing
      ? <div className="empty-state"><h2>{t('effectiveness.effectiveness.applicationNotFound')}</h2><Button onClick={() => go({ days })}>{t('effectiveness.effectiveness.backToWholeOrganization')}</Button></div>
      : <div className="form-error" role="alert">{load.message}<Button onClick={reload}>{t('effectiveness.effectiveness.retry')}</Button></div> : null}
    {report ? <>
      <div className="section-header eff-section"><h2>{t('effectiveness.effectiveness.softwareDeliveryDORA')}</h2><Button tone="quiet" onClick={() => setOpen({ kind: 'dora' })}>{t('effectiveness.effectiveness.howToReadTheMetrics')}</Button></div>
      <div className="eff-metrics">{report.metrics.filter((m) => m.group === 'DORA').map((m) => <MetricCard key={m.id} metric={m} onOpen={() => setOpen({ kind: 'metric', id: m.id })} />)}</div>
      <div className="section-header eff-section"><h2>{t('effectiveness.effectiveness.usage')}</h2></div>
      <div className="eff-metrics">{usageIds.map((id) => metric(id)).filter((m): m is Metric => !!m).map((m) => <MetricCard key={m.id} metric={m} onOpen={() => setOpen({ kind: 'metric', id: m.id })} />)}</div>
      <div className="eff-two-col"><TrendChart report={report} /><Comparison report={report} onOpen={() => setOpen({ kind: 'baseline' })} /></div>
      {report.breakdown ? <section className="content-panel">
        <div className="section-header"><h2>{report.breakdown.level === 'APPLICATION' ? t('effectiveness.effectiveness.byApplication') : 'Theo workload'}</h2></div>
        {report.breakdown.level === 'APPLICATION' && frequency?.details ? <p className="eff-bands">{bands.map((band) => <span key={band}>{bandLabels[band]} <b>{formatNumber(frequency.details?.[bandDetailKeys[band]] ?? 0)}</b></span>)}</p> : null}
        {report.breakdown.rows.length ? <div className="eff-table-wrap"><table className="plain-table eff-breakdown">
          <thead><tr><th>{report.breakdown.level === 'APPLICATION' ? t('effectiveness.effectiveness.application') : 'Workload'}</th><th>{t('effectiveness.effectiveness.productionDeployments')}</th><th>{t('effectiveness.effectiveness.frequency')}</th><th>{t('effectiveness.effectiveness.completedWork')}</th><th>{t('effectiveness.effectiveness.runs')}</th><th>{t('effectiveness.effectiveness.leadTime')}</th><th>{t('effectiveness.effectiveness.users')}</th></tr></thead>
          <tbody>{report.breakdown.rows.map((row) => <tr key={row.key}>
            <td><button className="back-link" onClick={() => report.breakdown?.level === 'APPLICATION' ? go({ application: row.key, days }) : go({ application, workload: row.key, days })}>{row.name}</button></td>
            <td>{row.productionDeployments}</td>
            <td>{bandLabels[row.frequencyBand]}</td>
            <td>{row.completedWork}{row.openWork ? <small>{t('effectiveness.effectiveness.openOpenWork', { openWork: row.openWork })}</small> : null}</td>
            <td>{row.attempts}{row.failedAttempts ? <small>{t('effectiveness.effectiveness.failedAttemptsFailed', { failedAttempts: row.failedAttempts })}</small> : null}</td>
            <td>{row.medianDurationSeconds !== null ? formatDuration(row.medianDurationSeconds) : t('effectiveness.effectiveness.noDataYet')}</td>
            <td>{row.activeUsers}</td>
          </tr>)}</tbody>
        </table></div> : <p className="muted">{t('effectiveness.effectiveness.noWorkInThisPeriod')}</p>}
      </section> : null}
      <section className="content-panel eff-pending">
        <h2>{t('effectiveness.effectiveness.notCollected')}</h2>
        {report.metrics.filter((m) => m.group !== 'DORA' && m.status === 'NOT_COLLECTED' && !shownElsewhere.has(m.id)).map((m) =>
          <button key={m.id} className="eff-pending-item" onClick={() => setOpen({ kind: 'metric', id: m.id })} aria-label={t('effectiveness.effectiveness.metricNameNotCollectedViewDetails', { metricName: metricName(m.id) })}>
            <span>{metricName(m.id)}</span><span className="muted">{t('effectiveness.effectiveness.notCollected')}</span>
          </button>)}
      </section>
    </> : null}
    {report && opened ? <MetricDialog metric={opened} report={report} scope={scope} onClose={() => setOpen(undefined)} /> : null}
    {report && open?.kind === 'dora' ? <DoraHelpDialog report={report} onClose={() => setOpen(undefined)} /> : null}
    {report && open?.kind === 'baseline' ? <BaselineDialog applicationKey={application} scopeName={application ? appName ?? application : t('effectiveness.effectiveness.wholeOrganization')} locked={!!workload}
      onClose={() => setOpen(undefined)} onSaved={(message) => { setOpen(undefined); setNotice(message); reload(); }} /> : null}
  </section>;
}
