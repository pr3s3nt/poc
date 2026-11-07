import { OverviewPanel } from './OverviewPanel';
import { useEffect, useState, type ReactNode } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { navigate, type TelemetryRouteQuery } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';

type Point = { timestamp: string; value: number | null };
type Series = { metric: Record<string, string>; values?: Point[]; value?: Point };
type Observation = { limits?: { cpu?: number | null; memory?: number | null }; configuration?: string; profileVersion?: string; applicationMetrics?: Record<string, { unit: string; state: string; series: Series[] }>; workload: string; state: string; metrics: Record<string, Series[]> };
type Alert = { name: string; state: string; labels: Record<string, string>; activeAt?: string; value: string; annotations: Record<string, string> };
type View = { logsSource?: string; alertmanagerState?: string; notifications?: { name: string; state: string; receivers: { name: string }[]; labels: Record<string, string> }[]; source: string; state: string; from: string; to: string; workloads: Observation[]; alerts: Alert[]; grafanaUrl?: string };
const valueOf = (w: Observation, key: string) => { const series = w.metrics[key]?.[0]; return series?.value?.value ?? series?.values?.at(-1)?.value ?? null; };
const display = (n: number | null, divisor = 1) => n === null ? '—' : new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(n / divisor);
const alertKey = (name: string, labels: Record<string, string>) => name + JSON.stringify(Object.entries(labels).sort(([a], [b]) => a.localeCompare(b)));
const label = (key: string) => t(`monitoring.${key}`);

function Chart({ title, series, divisor = 1 }: { title: string; series: Series[]; divisor?: number }) {
  const points = series[0]?.values ?? [];
  const max = Math.max(1, ...points.flatMap(p => p.value === null ? [] : [p.value / divisor]));
  const from = Date.parse(points[0]?.timestamp ?? ''), to = Date.parse(points.at(-1)?.timestamp ?? '');
  const x = (p: Point) => 10 + (Date.parse(p.timestamp) - from) / Math.max(1, to - from) * 580;
  const y = (p: Point) => 120 - (p.value ?? 0) / divisor / max * 100;
  return <section><h4>{title}</h4>{points.some(p => p.value !== null) ? <svg viewBox="0 0 600 150" role="img" aria-label={title} style={{ width: '100%', maxHeight: 180 }}>
    {points.slice(1).map((p, i) => { const prior = points[i]; return prior && p.value !== null && prior.value !== null && Date.parse(p.timestamp) - Date.parse(prior.timestamp) <= 1.5 * (to - from) / Math.max(1, points.length - 1) ? <line key={p.timestamp} x1={x(prior)} x2={x(p)} y1={y(prior)} y2={y(p)} stroke="#0969da" strokeWidth="2" /> : null; })}
    {points.filter(p => p.value !== null).map(p => <circle key={p.timestamp} cx={x(p)} cy={y(p)} r="3" tabIndex={0}><title>{new Date(p.timestamp).toLocaleString()} · {display(p.value, divisor)}</title></circle>)}
    <text x="10" y="145" fontSize="10">{new Date(from).toLocaleTimeString()}</text><text x="500" y="145" fontSize="10">{new Date(to).toLocaleTimeString()}</text>
  </svg> : <p>{label('noData')}</p>}</section>;
}
export function ExternalMonitoring({ applicationId, environment, initialQuery, nativeFallback, historyName }: { applicationId: string; environment: EnvironmentKey; initialQuery?: TelemetryRouteQuery; nativeFallback?: ReactNode; historyName?: string }) {
  const [data, setData] = useState<View>(); const [error, setError] = useState(''); const [reload, setReload] = useState(0); const [minutes, setMinutes] = useState(initialQuery?.from && initialQuery?.to ? 0 : 60); const [workload, setWorkload] = useState(initialQuery?.workload ?? ''); const [selected, setSelected] = useState<Alert>();
  useEffect(() => { let active = true; setData(undefined); setError(''); setSelected(undefined);
    const load = async () => { const to = minutes === 0 && initialQuery?.to ? new Date(initialQuery.to) : new Date(), from = minutes === 0 && initialQuery?.from ? new Date(initialQuery.from) : new Date(to.getTime() - minutes * 60000); const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
      try { const result = await api<View>(`/applications/${encodeURIComponent(applicationId)}/environments/${environment}/monitoring?${q}`); if (!Array.isArray(result.workloads)) throw new Error('invalid monitoring response'); if (active) { setData(result); setError(''); } } catch (e) { if (active) setError(describeError(e, label('failed'))); }
    }; void load(); const timer = window.setInterval(() => void load(), 30000); return () => { active = false; clearInterval(timer); };
  }, [applicationId, environment, minutes, reload, initialQuery?.from, initialQuery?.to]);
  if (data?.state === 'NOT_CONFIGURED' || data?.state === 'METRICS_NATIVE') return nativeFallback ?? <OverviewPanel applicationId={applicationId} environment={environment} />;
  let dashboardLink = data?.grafanaUrl;
  if (dashboardLink && workload && data?.workloads.some(w => w.workload === workload)) { const url = new URL(dashboardLink); url.searchParams.set('var-workload', workload); dashboardLink = url.toString(); }
  const shown = data?.workloads.filter(w => !workload || w.workload === workload) ?? [];
  const drill = (tab: 'logs' | 'metrics', name?: string) => navigate({ name: 'observability', applicationId, environment, tab, query: { from: data?.from, to: data?.to, workload: name } });
  const panel = <section className="content-panel" aria-label={label('title')}><div className="section-header"><div><h2>{label('title')}</h2><small>Prometheus · {environment} · {data ? label(data.state) : label('loading')}</small></div><Button onClick={() => setReload(n => n + 1)}>{label('reload')}</Button></div>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    <div className="obs-health-toolbar"><label>{label('range')} <select value={minutes} onChange={e => setMinutes(Number(e.target.value))}>{initialQuery?.from && initialQuery?.to ? <option value={0}>{label('customRange')}</option> : null}{[15, 60, 360, 1440].map(n => <option key={n} value={n}>{n} {label('minutes')}</option>)}</select></label><label>{label('workload')} <select value={workload} onChange={e => setWorkload(e.target.value)}><option value="">{label('all')}</option>{data?.workloads.map(w => <option key={w.workload}>{w.workload}</option>)}</select></label></div>
    <table className="obs-metric-table"><thead><tr><th>{label('workload')}</th><th>{label('readiness')}</th><th>CPU (mCPU)</th><th>RAM (MiB)</th><th>{label('restarts')}</th><th>OOM</th><th>{label('freshness')}</th></tr></thead><tbody>{shown.map(w => <tr key={w.workload}><td>{w.workload}</td><td>{display(valueOf(w, 'ready'))} / {display(valueOf(w, 'desired'))}</td><td>{display(valueOf(w, 'cpu'))}{w.limits?.cpu != null ? <small> / {display(w.limits.cpu)}</small> : null}</td><td>{display(valueOf(w, 'memory'), 1048576)}{w.limits?.memory != null ? <small> / {display(w.limits.memory, 1048576)}</small> : null}</td><td>{display(valueOf(w, 'restarts'))}</td><td>{display(valueOf(w, 'oom'))}</td><td>{label(w.state)}</td></tr>)}</tbody></table>
    {shown.map(w => <div key={w.workload}><h3>{w.workload}</h3><p>{label('configSync')}: {label(w.configuration ?? 'UNVERIFIED')}{w.profileVersion ? ` · ${w.profileVersion}` : ''}</p><div className="obs-chart-grid"><Chart title="CPU (mCPU)" series={w.metrics.cpu ?? []} /><Chart title="RAM (MiB)" series={w.metrics.memory ?? []} divisor={1048576} />{Object.entries(w.applicationMetrics ?? {}).map(([key, metric]) => <Chart key={key} title={`${key} (${metric.unit}) · ${label(metric.state)}`} series={metric.series} />)}</div></div>)}
    <h3>{label('alerts')}</h3><p>{label('alertmanager')}: {label(data?.alertmanagerState || 'UNVERIFIED')}</p>{data?.notifications?.map(n => <p key={alertKey(n.name, n.labels)}>{n.name} · {label(n.state)} · {n.receivers.map(r => r.name).join(', ')}</p>)}{data?.alerts?.length ? data.alerts.filter(a => !workload || a.labels.orchestrator_workload === workload).map(a => <Button key={alertKey(a.name, a.labels)} onClick={() => setSelected(a)}>{a.name}{a.labels.metric_key ? ` · ${a.labels.metric_key}` : ''} · {label(a.labels.severity ?? 'unknown')} · {label(a.state)}</Button>) : <p>{label('noAlerts')}</p>}
    {selected ? <section aria-label={label('alertDetail')}><h4>{selected.name}</h4><p>{selected.labels.orchestrator_workload} · {label(selected.labels.severity ?? 'unknown')} · {label(selected.state)}</p><p>{selected.activeAt ? new Date(selected.activeAt).toLocaleString() : '—'} · {selected.value}</p><p>{selected.annotations.summary}</p><p>{label('deliveryUnknown')}</p><Button onClick={() => drill('logs', selected.labels.orchestrator_workload)}>{data?.logsSource === 'loki' ? 'Logs Loki' : label('nativeLogs')}</Button><Button onClick={() => { setWorkload(selected.labels.orchestrator_workload ?? ''); }}>{label('metrics')}</Button><Button onClick={() => navigate({ name: 'deployments', applicationId, environment })}>{label('deployments')}</Button></section> : null}
    {dashboardLink ? <a href={dashboardLink} target="_blank" rel="noreferrer">{label('grafana')}</a> : null}
    <Button onClick={() => navigate({ name: 'observability', applicationId, environment, tab: 'metrics', query: { from: data?.from, to: data?.to, workload, source: 'native' } })}>{label('nativeMetricHistory')}</Button><details><summary>{label('coverage')}</summary><p>{label('coverageHelp')}</p></details>
  </section>;
  if (!historyName) return panel;
  return <section className="page application-home obs-page"><button className="back-link" onClick={() => navigate({ name: 'application', applicationId, environment })}>← {historyName}</button><header className="page-header application-header"><h1>{historyName}</h1><Button onClick={() => navigate({ name: 'settings', applicationId, environment })}>{label('settings')}</Button></header><div className="tabs" role="tablist" aria-label="Environment">{(['staging', 'production'] as const).map(env => <button key={env} role="tab" aria-selected={env === environment} className={`tab${env === environment ? ' tab-active' : ''}`} onClick={() => navigate({ name: 'observability', applicationId, environment: env, tab: 'metrics', query: { from: initialQuery?.from, to: initialQuery?.to, workload: initialQuery?.workload } })}>{env}</button>)}</div>{panel}</section>;

}
