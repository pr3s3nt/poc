import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import { Button } from '../../shared/ui/Button';
import { listRecords, type Metric, type Report, type Scope, type WorkRecord } from './api';
import { scopeLabel } from './csv';
import { Dialog } from './Dialog';
import { actionLabel, formatDateTime, formatDuration, formatNumber, metricFormula, metricName, metricTerm, metricValue, requiredSourceLabel, stateLabel } from './labels';

const doraGuide = 'https://dora.dev/guides/dora-metrics/';
const platformGuide = 'https://humanitec.com/blog/impact-of-internal-developer-platforms';

function openRecord(record: WorkRecord) {
  if (record.environmentKey !== 'staging' && record.environmentKey !== 'production') return;
  if (record.kind === 'DEPLOYMENT') navigate({ name: 'deployment', applicationId: record.applicationKey, environment: record.environmentKey, deploymentId: record.lastRunId });
  else if (record.resourceId) navigate({ name: 'resource', applicationId: record.applicationKey, environment: record.environmentKey, resourceId: record.resourceId });
}

function fraction(metric: Metric): string | undefined {
  if (metric.numerator === undefined || metric.denominator === undefined) return undefined;
  if (metric.id === 'deployment_frequency') return `${formatNumber(metric.numerator)} lượt trong ${formatNumber(metric.denominator)} ngày`;
  if (metric.id === 'execution_attempts') return `${formatNumber(metric.numerator)} lượt ÷ ${formatNumber(metric.denominator)} công việc hoàn thành`;
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
    {!collected ? <div className="form-info">Chưa thu thập. {requiredSourceLabel(metric.requiredSource)}</div> : <>
      <p className="eff-detail-value"><strong>{metricValue(metric)}</strong>{parts ? <span className="muted"> · {parts}</span> : null}</p>
      {metric.coverage && metric.coverage.covered < metric.coverage.total ? <p className="eff-warning">Độ phủ {metric.coverage.covered}/{metric.coverage.total} công việc.</p> : null}
      {cleanupsUnavailable ? <p className="eff-warning">Chưa gồm xóa tài nguyên (nguồn không khả dụng).</p> : null}
      {error ? <div className="form-error" role="alert">Không thể tải bản ghi.<Button onClick={() => setAttempt((value) => value + 1)}>Thử lại</Button></div>
        : !records ? <p role="status" aria-busy="true">Đang tải bản ghi…</p>
          : records.length ? <div className="eff-table-wrap"><table className="plain-table">
            <thead><tr><th>Hoàn thành / bắt đầu</th><th>Ứng dụng · môi trường</th><th>Workload / tài nguyên</th><th>Loại</th><th>Lần thử</th><th>Thời gian</th><th>Người thực hiện</th><th><span className="eff-sr-only">Mở</span></th></tr></thead>
            <tbody>{records.map((record) => <tr key={record.id}>
              <td>{formatDateTime(record.completedAt ?? record.firstStartedAt)}{record.state !== 'COMPLETED' ? <small className="eff-warning">{stateLabel(record.state)}</small> : null}</td>
              <td>{record.applicationName || record.applicationKey}<small>{record.environmentKey}</small></td>
              <td><code>{record.workloadId || record.resourceId || '—'}</code></td>
              <td>{actionLabel(record.action)}</td>
              <td>{record.attempts}{record.failedAttempts ? <small>{record.failedAttempts} thất bại</small> : null}</td>
              <td>{record.durationSeconds !== undefined ? formatDuration(record.durationSeconds) : '—'}</td>
              <td>{record.actors.length ? record.actors.map((actor) => actor.kind === 'USER' ? actor.name : `${actor.name} (tự động)`).join(', ') : '—'}</td>
              <td><Button tone="quiet" aria-label={`Mở ${record.workloadId || record.resourceId || record.id}`} onClick={() => openRecord(record)}>Mở</Button></td>
            </tr>)}</tbody>
          </table></div> : <p className="muted">Không có bản ghi trong kỳ này.</p>}
    </>}
    <details className="eff-help">
      <summary>Định nghĩa</summary>
      {term ? <p>Thuật ngữ gốc: {term}.</p> : null}
      <p>Retry của cùng công việc gộp thành một công việc; mọi lượt chạy vẫn được đếm ở “Lượt chạy”. Kỳ tính theo giờ Việt Nam.</p>
      <p><a href={metric.group === 'DORA' ? doraGuide : platformGuide} target="_blank" rel="noreferrer">{metric.group === 'DORA' ? 'DORA · Định nghĩa chỉ số' : 'Humanitec · Chỉ số platform'} ↗</a></p>
    </details>
  </Dialog>;
}

/** “Cách đọc chỉ số”: the DORA reading guide. */
export function DoraHelpDialog({ report, onClose }: { report: Report; onClose(): void }) {
  const dora = report.metrics.filter((metric) => metric.group === 'DORA');
  return <Dialog title="Cách đọc chỉ số" onClose={onClose}>
    <p>DORA đo tốc độ và độ ổn định khi giao phần mềm, chỉ tính môi trường production. Ở cấp tổ chức, xem phân bố theo ứng dụng; không cộng hay lấy trung bình giữa các ứng dụng.</p>
    <ul className="eff-help-list">{dora.map((metric) => <li key={metric.id}><b>{metricName(metric.id)}</b>{metric.status === 'NOT_COLLECTED' ? <span className="muted"> · Chưa thu thập</span> : null}<p>{metricFormula(metric.id)}</p></li>)}</ul>
    <p><a href={doraGuide} target="_blank" rel="noreferrer">DORA · Định nghĩa chỉ số ↗</a></p>
  </Dialog>;
}
