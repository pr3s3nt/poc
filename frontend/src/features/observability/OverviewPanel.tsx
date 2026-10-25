import { useState } from 'react';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import type { Focus, Metric, Peak, UsagePeak } from './api';
import { useTelemetry } from './useTelemetry';
import { CollectionNotice, EmptyTelemetry, MiniTrend, QueryFailure } from './Common';
import { displayValue, fresh, filledPoints, isRelativePeak, limitOf, noteworthy, number, peakOf, percentage, stateLabel, text, time, unit, valueOf, type Signal } from './model';
import './style.css';

export function OverviewPanel({ applicationId, environment, refreshKey = '' }: { applicationId: string; environment: EnvironmentKey; refreshKey?: string }) {
  const [minutes, setMinutes] = useState(60);
  const { data, error, loading, reload } = useTelemetry(applicationId, environment, 'overview', {}, 30_000, minutes, refreshKey);
  const received = (data?.latest.length ?? 0) > 0 || (data?.eventCount ?? 0) > 0;
  const lastObserved = data?.sources.flatMap(source => source.observedAt ? [source.observedAt] : []).sort().at(-1);
  const logsKnown = (data?.sources.some(source => !['WAITING', 'FAILED', 'FORBIDDEN', 'UNAVAILABLE'].includes(source.logsCode)) ?? false);
  const points = data ? noteworthy(data) : [];
  const open = (view: 'logs' | 'metrics', workload?: string, level?: string, focus?: Focus) => navigate({ name: 'observability', applicationId, environment, tab: view, query: { from: data?.from, to: data?.to, workload, level, focus } });
  const openSignal = (signal: Signal) => open(signal.view, signal.workload, signal.level, signal.focus);
  const peakCard = (metric: Metric, peak?: Peak | UsagePeak) => {
    const relative = peak && isRelativePeak(peak) ? peak : undefined;
    const latest = data?.latest.find(sample => sample.workloadId === peak?.workloadId);
    const current = latest && fresh(latest) ? percentage(valueOf(latest, metric), limitOf(latest, metric)) : null;
    return <button className={`obs-health-tile${relative && relative.percent >= 80 ? ' obs-health-warning' : ''}`} disabled={!peak} onClick={() => open('metrics', peak?.workloadId, undefined, metric)}>
      <span className="obs-health-label">{text(metric === 'cpu' ? 'peakCPU' : 'peakMemory')}<span aria-hidden="true">↗</span></span>
      <strong>{peak ? relative ? number(relative.percent) : displayValue(metric, peak.value) : '—'}{peak ? <small>{relative ? '%' : unit(metric)}</small> : null}</strong>
      <span className="obs-health-detail">{peak ? `${peak.workloadId} · ${text('atTime', { time: time(peak.timestamp) })}` : text('metricsUnknown')}</span>
      {data && peak ? <MiniTrend values={filledPoints(data, peak.workloadId, metric).map(point => point.value)} tone={relative && relative.percent >= 80 ? 'amber' : 'blue'} /> : <div className="obs-health-no-chart" />}
      <span className="obs-health-foot"><span>{relative ? text('ofLimit', { value: `${displayValue(metric, relative.limit)} ${unit(metric)}` }) : peak ? text('noLimit') : text('absent')}</span><b>{current !== null ? text('latestUsage', { value: number(current) }) : text('currentUnavailable')}</b></span>
    </button>;
  };
  return <section className="content-panel obs-home-card" aria-label={text('title')}>
    <div className="section-header"><div><h2>{text('title')}</h2><p>{text('source')} · {environment}</p></div><Status tone={data?.state === 'RECEIVING' ? 'good' : 'neutral'}>{data ? stateLabel(data.state) : text('loadingText')}</Status></div>
    {error ? <QueryFailure message={error} retry={reload} /> : null}
    {data ? <CollectionNotice data={data} /> : null}
    <div className="obs-health-toolbar"><label>{text('time')}<select aria-label={text('time')} value={minutes} onChange={event => setMinutes(Number(event.target.value))}>{[15, 60, 360, 1440].map(value => <option key={value} value={value}>{text(`last${value}`)}</option>)}</select></label><span>{data && lastObserved ? text('scopeData', { count: data.sources.filter(source => source.observedAt).length, time: time(lastObserved, true) }) : loading ? text('loadingText') : text('absent')}</span></div>
    {data && !data.hasHistory && ['NOT_DEPLOYED', 'WAITING', 'UNAVAILABLE'].includes(data.state) ? <EmptyTelemetry state={data.state} /> : <>
      <div className="obs-health-grid" aria-busy={loading && !data}>
        <button className={`obs-health-tile${data && data.errorCount ? ' obs-health-danger' : ''}`} disabled={!logsKnown && !data?.logCount} onClick={() => open('logs', undefined, 'ERROR')}>
          <span className="obs-health-label">{text('logErrors')}<span aria-hidden="true">↗</span></span><strong>{logsKnown || data?.logCount ? number(data?.errorCount ?? 0) : '—'}</strong><span className="obs-health-detail">{data ? text('errorCount', { count: data.errorCount, total: data.logCount }) : text('absent')}</span>
          <div className="obs-error-histogram" aria-hidden="true">{(data?.histogram ?? Array.from({ length: 16 }, () => ({ count: 0 }))).map((bucket, index) => <i key={index} className={bucket.count ? 'has-errors' : ''} style={{ height: `${Math.max(2, bucket.count / Math.max(1, ...(data?.histogram.map(point => point.count) ?? [])) * 28)}px` }} />)}</div><span className="obs-health-foot">{text('receivedRecords')}</span>
        </button>
        <button className={`obs-health-tile${data?.signalCount ? ' obs-health-warning' : ''}`} disabled={!received && !data?.hasHistory} onClick={() => open('metrics', undefined, undefined, 'signals')}>
          <span className="obs-health-label">{text('signals')}<span aria-hidden="true">↗</span></span><strong>{received || data?.hasHistory ? number(data?.signalCount ?? 0) : '—'}</strong><span className="obs-health-detail">CPU / RAM · Restart</span><div className="obs-signal-dots" aria-hidden="true">{['⌁', '▥', '↻'].map((icon, index) => <span className={data?.signalCount ? 'has-signal' : ''} key={index}>{icon}</span>)}</div><span className="obs-health-foot">{data ? text('signalCount', { count: data.signalCount }) : text('absent')}</span>
        </button>
        {peakCard('cpu', data ? peakOf(data, 'cpu') : undefined)}{peakCard('memory', data ? peakOf(data, 'memory') : undefined)}
      </div>
      {data ? <div className="obs-highlights"><div className="obs-highlights-heading"><h3>{text('highlights')}</h3><details><summary>{text('readSummary')}</summary><p>{text('countsHelp')}</p></details></div>{points.length ? points.map(signal => <button className="obs-highlight-row" key={signal.id} onClick={() => openSignal(signal)} aria-label={text('openSignal', { name: signal.name })}><span className={`obs-event-icon obs-tone-${signal.tone}`} aria-hidden="true">{signal.view === 'logs' ? '!' : signal.focus === 'signals' ? '↻' : '↗'}</span><span className="obs-event-copy"><strong>{signal.name}</strong><small>{signal.detail}</small></span><span className="obs-event-state">{signal.state}</span><time>{time(signal.timestamp)}</time><span className="obs-event-arrow" aria-hidden="true">→</span></button>) : <p className="obs-stamp">{received ? text('noSignals') : text('absent')}</p>}</div> : null}
    </>}
    <div className="obs-home-footer"><small>{text('retention')}</small><div><Button onClick={() => open('logs')}>{text('viewLogs')} →</Button><Button onClick={() => open('metrics')}>{text('viewMetrics')} →</Button></div></div>
  </section>;
}
