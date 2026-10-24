import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import { Button } from '../../shared/ui/Button';
import { listRecords, type Metric, type Report, type Scope, type WorkRecord } from './api';
import { scopeLabel } from './csv';
import { Dialog } from './Dialog';
import { actionLabel, formatDateTime, formatDuration, formatNumber, metricFormula, metricName, metricTerm, metricValue, requiredSourceLabel, stateLabel } from './labels';
import { t } from '../../shared/i18n';

const doraGuide = 'https://dora.dev/guides/dora-metrics/';
const platformGuide = 'https://humanitec.com/blog/impact-of-internal-developer-platforms';

function openRecord(record: WorkRecord) {
  if (record.environmentKey !== 'staging' && record.environmentKey !== 'production') return;
  if (record.kind === 'DEPLOYMENT') navigate({ name: 'deployment', applicationId: record.applicationKey, environment: record.environmentKey, deploymentId: record.lastRunId });
  else if (record.resourceId) navigate({ name: 'resource', applicationId: record.applicationKey, environment: record.environmentKey, resourceId: record.resourceId });
}

function fraction(metric: Metric): string | undefined {
  if (metric.numerator === undefined || metric.denominator === undefined) return undefined;
  if (metric.id === 'deployment_frequency') return t('effectiveness.metricDialog.formatNumberRunsInFormatNumber2Days', { formatNumber: formatNumber(metric.numerator), formatNumber2: formatNumber(metric.denominator) });
  if (metric.id === 'execution_attempts') return t('effectiveness.metricDialog.formatNumberRunsFormatNumber2CompletedWork', { formatNumber: formatNumber(metric.numerator), formatNumber2: formatNumber(metric.denominator) });
  return `${formatNumber(metric.numerator)}/${formatNumber(metric.denominator)}`;
}

/** Metric detail: formula, scope, coverage and the evidence work items (UC-17 MS-07). */
export function MetricDialog({ metric, report, scope, onClose }: { metric: Metric; report: Report; scope: Scope; onClose(): void }) {
  const [records, setRecords] = useState<WorkRecord[]>();
  const [error, setError] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const collected = metric.status !== 'NOT_COLLECTED';
  useEffect(() => {
    if (!collected) return;
    let cancelled = false;
    setRecords(undefined); setError(false);
    listRecords(scope, metric.id).then((response) => { if (!cancelled) setRecords(response.records); }).catch(() => { if (!cancelled) setError(true); });
    return () => { cancelled = true; };
  }, [collected, metric.id, scope, attempt]);
  const cleanupsUnavailable = metric.group === 'USAGE' && report.sources.some((source) => source.id === 'resource_cleanups' && !source.available);
  const term = metricTerm(metric.id);
  const parts = fraction(metric);
  return <Dialog title={metricName(metric.id)} subtitle={`${scopeLabel(report)} · ${report.period.from} — ${report.period.to}${metric.group === 'DORA' ? ' · production' : ''}`} onClose={onClose}>
    <p>{metricFormula(metric.id)}</p>
    {!collected ? <div className="form-info">{t('effectiveness.metricDialog.notCollectedRequiredSourceLabel', { requiredSourceLabel: requiredSourceLabel(metric.requiredSource) })}</div> : <>
      <p className="eff-detail-value"><strong>{metricValue(metric)}</strong>{parts ? <span className="muted"> · {parts}</span> : null}</p>
      {metric.coverage && metric.coverage.covered < metric.coverage.total ? <p className="eff-warning">{t('effectiveness.metricDialog.coverageCoveredTotalWorkItems', { covered: metric.coverage.covered, total: metric.coverage.total })}</p> : null}
      {cleanupsUnavailable ? <p className="eff-warning">{t('effectiveness.metricDialog.resourceDeletionsNotIncludedSource')}</p> : null}
      {error ? <div className="form-error" role="alert">{t('effectiveness.metricDialog.couldNotLoadTheRecords')}<Button onClick={() => setAttempt((value) => value + 1)}>{t('effectiveness.metricDialog.retry')}</Button></div>
        : !records ? <p role="status" aria-busy="true">{t('effectiveness.metricDialog.loadingRecords')}</p>
          : records.length ? <div className="eff-table-wrap"><table className="plain-table">
            <thead><tr><th>{t('effectiveness.metricDialog.completedStarted')}</th><th>{t('effectiveness.metricDialog.applicationEnvironment')}</th><th>{t('effectiveness.metricDialog.workloadResource')}</th><th>{t('effectiveness.metricDialog.type')}</th><th>{t('effectiveness.metricDialog.attempts')}</th><th>{t('effectiveness.metricDialog.duration')}</th><th>{t('effectiveness.metricDialog.performedBy')}</th><th><span className="eff-sr-only">{t('effectiveness.metricDialog.open')}</span></th></tr></thead>
            <tbody>{records.map((record) => <tr key={record.id}>
              <td>{formatDateTime(record.completedAt ?? record.firstStartedAt)}{record.state !== 'COMPLETED' ? <small className="eff-warning">{stateLabel(record.state)}</small> : null}</td>
              <td>{record.applicationName || record.applicationKey}<small>{record.environmentKey}</small></td>
              <td><code>{record.workloadId || record.resourceId || '—'}</code></td>
              <td>{actionLabel(record.action)}</td>
              <td>{record.attempts}{record.failedAttempts ? <small>{t('effectiveness.metricDialog.failedAttemptsFailed', { failedAttempts: record.failedAttempts })}</small> : null}</td>
              <td>{record.durationSeconds !== undefined ? formatDuration(record.durationSeconds) : '—'}</td>
              <td>{record.actors.length ? record.actors.map((actor) => actor.kind === 'USER' ? actor.name : t('effectiveness.metricDialog.nameAutomated', { name: actor.name })).join(', ') : '—'}</td>
              <td><Button tone="quiet" aria-label={t('effectiveness.metricDialog.openValue', { value: record.workloadId || record.resourceId || record.id })} onClick={() => openRecord(record)}>{t('effectiveness.metricDialog.open')}</Button></td>
            </tr>)}</tbody>
          </table></div> : <p className="muted">{t('effectiveness.metricDialog.noRecordsInThisPeriod')}</p>}
    </>}
    <details className="eff-help">
      <summary>{t('effectiveness.metricDialog.definition')}</summary>
      {term ? <p>{t('effectiveness.metricDialog.originalTermTerm', { term })}</p> : null}
      <p>{t('effectiveness.metricDialog.retriesOfTheSameWork')}</p>
      <p><a href={metric.group === 'DORA' ? doraGuide : platformGuide} target="_blank" rel="noreferrer">{metric.group === 'DORA' ? t('effectiveness.metricDialog.dORAMetricDefinitions') : t('effectiveness.metricDialog.humanitecPlatformMetrics')} ↗</a></p>
    </details>
  </Dialog>;
}

/** “Cách đọc chỉ số”: the DORA reading guide. */
export function DoraHelpDialog({ report, onClose }: { report: Report; onClose(): void }) {
  const dora = report.metrics.filter((metric) => metric.group === 'DORA');
  return <Dialog title={t('effectiveness.metricDialog.howToReadTheMetrics')} onClose={onClose}>
    <p>{t('effectiveness.metricDialog.dORAMeasuresTheSpeedAnd')}</p>
    <ul className="eff-help-list">{dora.map((metric) => <li key={metric.id}><b>{metricName(metric.id)}</b>{metric.status === 'NOT_COLLECTED' ? <span className="muted">{' '}{t('effectiveness.metricDialog.notCollected')}</span> : null}<p>{metricFormula(metric.id)}</p></li>)}</ul>
    <p><a href={doraGuide} target="_blank" rel="noreferrer">{t('effectiveness.metricDialog.dORAMetricDefinitions2')}</a></p>
  </Dialog>;
}
