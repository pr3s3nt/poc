import { useEffect, useMemo, useRef, useState } from 'react';
import { navigate, type TelemetryRouteQuery } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { endpointFor } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Status } from '../../shared/ui/Status';
import { describeError } from '../../shared/api/client';
import { interval, readTelemetry, type Filters, type Focus, type Log, type Metric } from './api';
import { CollectionNotice, EmptyTelemetry, QueryFailure } from './Common';
import { MetricChart } from './MetricCharts';
import { color, dateTime, displayValue, number, signalsOf, stateLabel, text, time, timezone } from './model';
import { useTelemetry } from './useTelemetry';
import './style.css';

const localDateInput = (raw: string) => { const date = new Date(raw); if (!Number.isFinite(date.getTime())) return ''; return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16); };
const knownRanges = [15, 60, 360, 1440, 10080, 21600];
function preset(filters: Filters): string { if (!filters.from || !filters.to) return '60'; const duration = (Date.parse(filters.to) - Date.parse(filters.from)) / 60000; return String(knownRanges.find(minutes => Math.abs(minutes - duration) < 0.05) ?? 'custom'); }

export function HistoryPage({ application, environment, tab, query = {} }: { application: Application; environment: EnvironmentKey; tab: 'logs' | 'metrics'; query?: TelemetryRouteQuery }) {
  const routeKey = JSON.stringify(query);
  const initial = useMemo(() => { const input = JSON.parse(routeKey) as TelemetryRouteQuery; return { ...interval(60), ...Object.fromEntries(Object.entries(input).filter(([key, value]) => key !== 'focus' && value !== undefined)), limit: 100 } as Filters; }, [routeKey]);
  const [filters, setFilters] = useState<Filters>(initial);
  const [focus, setFocus] = useState<Focus | undefined>(query.focus);
  const [range, setRange] = useState(preset(initial));
  const [startInput, setStartInput] = useState(localDateInput(initial.from!));
  const [endInput, setEndInput] = useState(localDateInput(initial.to!));
  const [invalidRange, setInvalidRange] = useState(false);
  const [follow, setFollow] = useState(false);
  const [search, setSearch] = useState(initial.q ?? '');
  const [older, setOlder] = useState<Log[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [olderBusy, setOlderBusy] = useState(false);
  const [olderError, setOlderError] = useState('');
  const [detail, setDetail] = useState<Log | null>(null);
  const [copyState, setCopyState] = useState('');
  const dialog = useRef<HTMLDialogElement>(null), logArea = useRef<HTMLDivElement>(null), opener = useRef<HTMLElement | null>(null);
  const scope = `${application.id}/${environment}/${tab}/${JSON.stringify(filters)}`;
  const scopeRef = useRef(scope); scopeRef.current = scope;
  const history = useTelemetry(application.id, environment, tab, filters, follow && tab === 'logs' ? 5000 : 0, follow && range !== 'custom' ? Number(range) : undefined);
  const { data, error, loading, reload } = history;
  useEffect(() => { setFilters(initial); setRange(preset(initial)); setSearch(initial.q ?? ''); setFocus(query.focus); setFollow(false); setDetail(null); setStartInput(localDateInput(initial.from!)); setEndInput(localDateInput(initial.to!)); setInvalidRange(false); }, [initial, application.id, environment, tab, query.focus]);
  useEffect(() => { setOlder([]); setNextCursor(undefined); setOlderError(''); setOlderBusy(false); }, [scope]);
  useEffect(() => { const timer = window.setTimeout(() => { setFilters(value => value.q === search ? value : { ...value, q: search }); }, 250); return () => window.clearTimeout(timer); }, [search]);
  useEffect(() => { if (detail) dialog.current?.showModal(); else if (dialog.current?.open) { dialog.current.close(); opener.current?.focus(); } }, [detail]);
  useEffect(() => { if (follow && logArea.current) logArea.current.scrollTop = logArea.current.scrollHeight; }, [follow, data]);
  const changeFilter = (patch: Partial<Filters>) => { setFocus(undefined); setFollow(false); setFilters(previous => ({ ...previous, ...patch, cursor: undefined })); };
  const logFrom = (from?: string) => from && Date.parse(from) < Date.now() - 7 * 86400000 ? new Date(Date.now() - 7 * 86400000).toISOString() : from;
  const openTab = (next: 'logs' | 'metrics') => navigate({ name: 'observability', applicationId: application.id, environment, tab: next, query: { from: next === 'logs' ? logFrom(filters.from) : filters.from, to: filters.to, workload: filters.workload, level: filters.level, q: search, focus } });
  async function loadOlder() {
    const cursor = nextCursor ?? data?.nextCursor;
    if (!data || !cursor || olderBusy) return;
    const selected = scope; setOlderBusy(true); setOlderError('');
    try { const page = await readTelemetry(application.id, environment, 'logs', { ...filters, from: data.from, to: data.to, cursor }); if (scopeRef.current === selected) { setOlder(previous => [...previous, ...page.logs]); setNextCursor(page.nextCursor ?? ''); } }
    catch (error) { if (scopeRef.current === selected) setOlderError(describeError(error, text('errorText'))); }
    finally { if (scopeRef.current === selected) setOlderBusy(false); }
  }
  function applyRange() {
    const start = new Date(startInput), end = new Date(endInput), now = Date.now(), retention = (tab === 'logs' ? 7 : 15) * 86400000;
    const valid = Number.isFinite(start.getTime()) && Number.isFinite(end.getTime()) && start < end && start.getTime() >= now - retention - 60000 && end.getTime() <= now + 60000;
    setInvalidRange(!valid); if (valid) changeFilter({ from: start.toISOString(), to: end.toISOString() });
  }
  function inspectLog(at: number) {
    const start = Math.max(Date.now() - 7 * 86400000, at - 300000), end = Math.min(Date.now(), at + 300000);
    navigate({ name: 'observability', applicationId: application.id, environment, tab: 'logs', query: { from: new Date(start).toISOString(), to: new Date(end).toISOString(), workload: filters.workload } });
  }
  const ids = new Set<string>();
  const logs = [...(data?.logs ?? []), ...older].filter(row => { if (ids.has(row.id)) return false; ids.add(row.id); return true; }).reverse();
  const signals = data ? signalsOf(data) : [];
  const workloads = [...new Set([...(data?.workloads ?? []), ...(filters.workload ? [filters.workload] : [])])];
  const unseen = data && !data.hasHistory && ['NOT_DEPLOYED', 'WAITING', 'UNAVAILABLE'].includes(data.state);
  return <section className="page application-home obs-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id, environment })}>← {text('back')}</button>
    <header className="page-header application-header"><div><p className="eyebrow">{text('app')}</p><h1>{application.name}</h1><p>{endpointFor(application, environment)}</p></div><Button onClick={() => navigate({ name: 'settings', applicationId: application.id, environment })}>{text('settings')}</Button></header>
    <div className="tabs" role="tablist" aria-label="Environment">{(['staging', 'production'] as const).map(env => <button key={env} role="tab" aria-selected={env === environment} className={`tab${env === environment ? ' tab-active' : ''}`} onClick={() => navigate({ name: 'observability', applicationId: application.id, environment: env, tab, query: { from: filters.from, to: filters.to } })}>{env}<span>{endpointFor(application, env)}</span></button>)}</div>
    <section className="content-panel obs-explorer">
      <div className="section-header"><div><h2>{text('title')}</h2><p>{environment} · {text('source')}</p></div><Status tone={data?.state === 'RECEIVING' ? 'good' : 'neutral'}>{data ? stateLabel(data.state) : text('loadingText')}</Status></div>
      <div className="obs-view-tabs" role="tablist" aria-label={text('title')}>{(['logs', 'metrics'] as const).map(next => <button key={next} role="tab" aria-selected={tab === next} className={tab === next ? 'is-active' : ''} onClick={() => openTab(next)}>{text(next)}</button>)}</div>
      <div className="obs-filters"><label>{text('workload')}<select aria-label={text('workload')} value={filters.workload ?? ''} onChange={event => changeFilter({ workload: event.target.value })}><option value="">{text('all')}</option>{workloads.map(workload => <option key={workload}>{workload}</option>)}</select></label><label>{text('time')}<select aria-label={text('time')} value={range} onChange={event => { const value = event.target.value; setRange(value); setInvalidRange(false); if (value !== 'custom') { const bounds = interval(Number(value)); setStartInput(localDateInput(bounds.from!)); setEndInput(localDateInput(bounds.to!)); changeFilter(bounds); } else setFollow(false); }}>{knownRanges.filter(minutes => tab === 'metrics' || minutes <= 10080).map(minutes => <option key={minutes} value={minutes}>{text(`last${minutes}`)}</option>)}<option value="custom">{text('custom')}</option></select></label><span className="obs-filter-spacer" /><span className="obs-stamp">{data?.sources.some(source => source.observedAt) ? text('lastObservation', { time: time(data.sources.flatMap(source => source.observedAt ? [source.observedAt] : []).sort().at(-1)!, true) }) : ''}</span><Button onClick={reload} disabled={loading}>{text('refresh')}</Button></div>
      {range === 'custom' ? <div className="obs-custom-range"><label>{text('from')}<input type="datetime-local" aria-label={text('from')} value={startInput} onChange={event => setStartInput(event.target.value)} /></label><label>{text('to')}<input type="datetime-local" aria-label={text('to')} value={endInput} onChange={event => setEndInput(event.target.value)} /></label><Button onClick={applyRange}>{text('apply')}</Button>{invalidRange ? <span className="field-error" role="alert">{text('invalidRange')}</span> : null}</div> : null}
      {focus ? <div className="obs-drill-context" role="status"><span>{focus === 'signals' ? text('signalsContext') : `${text(focus === 'cpu' ? 'cpu' : 'memory')} · ${filters.workload || text('all')}`}</span><button aria-label={text('clearContext')} onClick={() => setFocus(undefined)}>×</button></div> : null}
      {error ? <QueryFailure message={error} retry={reload} /> : null}{data ? <CollectionNotice data={data} /> : null}
      {!data ? loading ? <div className="obs-empty" role="status" aria-busy="true">{text('loadingText')}</div> : null : unseen ? <EmptyTelemetry state={data.state} /> : tab === 'logs' ? <>
        <div className="obs-log-controls"><label className="obs-search"><svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="8" cy="8" r="5" /><path d="m12 12 5 5" /></svg><input maxLength={200} aria-label={text('search')} placeholder={text('searchHint')} value={search} onChange={event => { setSearch(event.target.value); setFocus(undefined); setFollow(false); }} /></label><label className="obs-stream"><span>{text('level')}</span><select aria-label={text('level')} value={filters.level ?? 'ALL'} onChange={event => changeFilter({ level: event.target.value })}>{['ALL', 'ERROR', 'WARN', 'INFO', 'DEBUG', 'UNKNOWN'].map(level => <option key={level} value={level}>{level === 'ALL' ? text('allLevels') : level === 'UNKNOWN' ? text('logLevelUnknown') : level}</option>)}</select></label><Button aria-pressed={follow} title={text('followHelp')} disabled={range === 'custom'} onClick={() => { setOlder([]); setNextCursor(undefined); setFollow(value => !value); }}><span className={`obs-live-dot${follow ? ' is-live' : ''}`} />{text(follow ? 'pause' : 'follow')}</Button></div>
        <div className="obs-log-table" role="region" aria-label={text('logs')} tabIndex={0} ref={logArea}><table><thead><tr><th>{text('timeColumn')}</th><th>{text('workload')}</th><th>{text('content')}</th></tr></thead><tbody>{logs.map(log => <tr key={log.id}><td><span>{time(log.timestamp, true)}</span><small>{new Date(log.timestamp).toLocaleDateString()}</small></td><td><span className="obs-workload-label"><i style={{ background: color(log.workloadId, workloads) }} />{log.workloadId}</span><small>{log.level}</small></td><td><button className="obs-log-message" onClick={event => { opener.current = event.currentTarget; setCopyState(''); setDetail(log); }}>{log.message}</button></td></tr>)}</tbody></table>{!logs.length ? <div className="obs-empty"><p>{text('noResults')}</p><Button onClick={() => { setSearch(''); changeFilter({ q: '', workload: '', level: 'ALL' }); }}>{text('clear')}</Button></div> : null}</div>
        {olderError ? <QueryFailure message={olderError} retry={() => void loadOlder()} /> : null}
        <div className="obs-log-footer"><span>{text('recordsOf', { shown: logs.length, total: data.logCount })}{follow ? ` · ${text('live')}` : ''}</span>{(nextCursor ?? data.nextCursor) ? <Button tone="quiet" disabled={olderBusy || follow} onClick={() => void loadOlder()}>{olderBusy ? text('loadingText') : text('older')} ↑</Button> : <small>{text('removed')}</small>}</div>
      </> : <>
        {focus === 'signals' ? <section className="obs-drill-events" aria-label={text('eventsInRange')}><h3>{text('eventsInRange')}</h3>{signals.length ? signals.map(signal => <button key={signal.id} className="obs-highlight-row" onClick={() => { setFocus(signal.focus); setFilters(previous => ({ ...previous, workload: signal.workload })); }}><span className={`obs-event-icon obs-tone-${signal.tone}`} aria-hidden="true">↗</span><span className="obs-event-copy"><strong>{signal.name}</strong><small>{signal.detail}</small></span><span className="obs-event-state">{signal.state}</span><time>{time(signal.timestamp)}</time><span aria-hidden="true">→</span></button>) : <p className="obs-stamp">{text('noSignals')}</p>}{signals.length < data.signalCount ? <small>{text('partialEvents', { shown: signals.length, total: data.signalCount })}</small> : null}</section> : null}
        <div className="obs-chart-grid">{(['cpu', 'memory'] as Metric[]).map(metric => <MetricChart key={`${metric}/${filters.workload ?? ''}/${data.from}/${data.to}`} data={data} metric={metric} selected={filters.workload} focused={focus === metric} onLogs={inspectLog} />)}</div>
        <div className="obs-metric-table-heading"><h3>{text('latestValues')}</h3><span>{text('metricInterval', { seconds: data.stepSeconds })}</span></div>
        <div className="obs-table-scroll"><table className="obs-metric-table"><thead><tr><th>{text('workload')}</th><th>{text('cpu')} <small>(mCPU)</small></th><th>{text('memory')} <small>(MiB)</small></th><th>{text('restarts')}</th><th>{text('pods')}</th><th /></tr></thead><tbody>{data.latest.map(sample => <tr key={sample.workloadId}><td><span className="obs-workload-label"><i style={{ background: color(sample.workloadId, workloads) }} />{sample.workloadId}</span><small className="obs-sample-time">{text('sourceTimestamp', { time: dateTime(sample.metricsAt ?? sample.timestamp) })}</small></td><td>{sample.cpuMilli === null ? '—' : displayValue('cpu', sample.cpuMilli)}</td><td>{sample.memoryBytes === null ? '—' : displayValue('memory', sample.memoryBytes)}</td><td>{number(sample.restarts)}</td><td><Status tone={data.state === 'RECEIVING' && sample.readyPods === sample.totalPods && sample.totalPods > 0 ? 'good' : 'neutral'}>{`${sample.readyPods} / ${sample.totalPods}`}</Status></td><td><Button tone="quiet" onClick={() => navigate({ name: 'observability', applicationId: application.id, environment, tab: 'logs', query: { from: logFrom(filters.from), to: filters.to, workload: sample.workloadId } })}>{text('viewLogs')} →</Button></td></tr>)}</tbody></table>{!data.latest.length ? <p className="obs-stamp">{text('emptyPeriod')}</p> : null}</div>
      </>}
      <div className="obs-explorer-footer"><small>{text('retention')}</small><small>{text('timezoneName', { zone: timezone() })}</small></div>
    </section>
    <dialog ref={dialog} className="obs-detail-dialog" aria-labelledby="native-log-title" onCancel={() => setDetail(null)} onClick={event => { if (event.target === event.currentTarget) setDetail(null); }}>
      {detail ? <><header><h2 id="native-log-title">{text('detail')}</h2><Button tone="quiet" aria-label={text('close')} onClick={() => setDetail(null)}>×</Button></header><dl><div><dt>{text('timeColumn')}</dt><dd>{dateTime(detail.timestamp)}</dd></div><div><dt>Environment</dt><dd>{environment}</dd></div><div><dt>{text('workload')}</dt><dd>{detail.workloadId}</dd></div><div><dt>{text('container')}</dt><dd>{detail.container}</dd></div><div className="obs-full-row"><dt>{text('pod')}</dt><dd>{detail.podName}<small className="obs-sample-time">UID: {detail.podUid}</small></dd></div><div><dt>{text('level')}</dt><dd>{detail.level}</dd></div><div><dt>{text('restarts')}</dt><dd>{detail.restartIndex}</dd></div></dl><pre>{detail.message}</pre><footer><span role="status">{copyState}</span><Button onClick={() => { if (!navigator.clipboard) { setCopyState(text('copyFail')); return; } void navigator.clipboard.writeText(detail.message).then(() => setCopyState(text('copied'))).catch(() => setCopyState(text('copyFail'))); }}>{text('copy')}</Button></footer></> : null}
    </dialog>
  </section>;
}
