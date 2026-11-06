import { OverviewPanel } from './OverviewPanel';
import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';

type Point = { timestamp: string; value: number | null };
type Series = { metric: Record<string, string>; values?: Point[]; value?: Point };
type Observation = { workload: string; state: string; metrics: Record<string, Series[]> };
type Alert = { name: string; state: string; labels: Record<string, string>; activeAt?: string; value: string; annotations: Record<string, string> };
type View = { source: string; state: string; from: string; to: string; workloads: Observation[]; alerts: Alert[]; grafanaUrl?: string };
const valueOf = (w: Observation, key: string) => { const series = w.metrics[key]?.[0]; return series?.value?.value ?? series?.values?.at(-1)?.value ?? null; };
const display = (n: number | null, divisor = 1) => n === null ? '—' : new Intl.NumberFormat(undefined, { maximumFractionDigits: 2 }).format(n / divisor);
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
export function ExternalMonitoring({ applicationId, environment }: { applicationId: string; environment: EnvironmentKey }) {
  const [data, setData] = useState<View>(); const [error, setError] = useState(''); const [reload, setReload] = useState(0); const [minutes, setMinutes] = useState(60); const [workload, setWorkload] = useState(''); const [selected, setSelected] = useState<Alert>();
  useEffect(() => { let active = true; setData(undefined); setError(''); setSelected(undefined);
    const load = async () => { const to = new Date(), from = new Date(to.getTime() - minutes * 60000); const q = new URLSearchParams({ from: from.toISOString(), to: to.toISOString() });
      try { const result = await api<View>(`/applications/${encodeURIComponent(applicationId)}/environments/${environment}/monitoring?${q}`); if (!Array.isArray(result.workloads)) throw new Error('invalid monitoring response'); if (active) { setData(result); setError(''); } } catch (e) { if (active) setError(describeError(e, label('failed'))); }
    }; void load(); const timer = window.setInterval(() => void load(), 30000); return () => { active = false; clearInterval(timer); };
  }, [applicationId, environment, minutes, reload]);
  if (data?.state === 'NOT_CONFIGURED') return <OverviewPanel applicationId={applicationId} environment={environment} />;
  const shown = data?.workloads.filter(w => !workload || w.workload === workload) ?? [];
  const drill = (tab: 'logs' | 'metrics', name?: string) => navigate({ name: 'observability', applicationId, environment, tab, query: { from: data?.from, to: data?.to, workload: name } });
  return <section className="content-panel" aria-label={label('title')}><div className="section-header"><div><h2>{label('title')}</h2><small>Prometheus · {environment} · {data ? label(data.state) : label('loading')}</small></div><Button onClick={() => setReload(n => n + 1)}>{label('reload')}</Button></div>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    <div className="obs-health-toolbar"><label>{label('range')} <select value={minutes} onChange={e => setMinutes(Number(e.target.value))}>{[15, 60, 360, 1440].map(n => <option key={n} value={n}>{n} {label('minutes')}</option>)}</select></label><label>{label('workload')} <select value={workload} onChange={e => setWorkload(e.target.value)}><option value="">{label('all')}</option>{data?.workloads.map(w => <option key={w.workload}>{w.workload}</option>)}</select></label></div>
    <table className="obs-metric-table"><thead><tr><th>{label('workload')}</th><th>{label('readiness')}</th><th>CPU (mCPU)</th><th>RAM (MiB)</th><th>{label('restarts')}</th><th>OOM</th><th>{label('freshness')}</th></tr></thead><tbody>{shown.map(w => <tr key={w.workload}><td>{w.workload}</td><td>{display(valueOf(w, 'ready'))} / {display(valueOf(w, 'desired'))}</td><td>{display(valueOf(w, 'cpu'))}</td><td>{display(valueOf(w, 'memory'), 1048576)}</td><td>{display(valueOf(w, 'restarts'))}</td><td>{display(valueOf(w, 'oom'))}</td><td>{label(w.state)}</td></tr>)}</tbody></table>
    {shown.map(w => <div key={w.workload}><h3>{w.workload}</h3><div className="obs-chart-grid"><Chart title="CPU (mCPU)" series={w.metrics.cpu ?? []} /><Chart title="RAM (MiB)" series={w.metrics.memory ?? []} divisor={1048576} /></div></div>)}
    <h3>{label('alerts')}</h3>{data?.alerts?.length ? data.alerts.filter(a => !workload || a.labels.orchestrator_workload === workload).map(a => <Button key={`${a.name}/${a.labels.orchestrator_workload}`} onClick={() => setSelected(a)}>{a.name} · {label(a.labels.severity ?? 'unknown')} · {label(a.state)}</Button>) : <p>{label('noAlerts')}</p>}
    {selected ? <section aria-label={label('alertDetail')}><h4>{selected.name}</h4><p>{selected.labels.orchestrator_workload} · {label(selected.labels.severity ?? 'unknown')} · {label(selected.state)}</p><p>{selected.activeAt ? new Date(selected.activeAt).toLocaleString() : '—'} · {selected.value}</p><p>{selected.annotations.summary}</p><p>{label('deliveryUnknown')}</p><Button onClick={() => drill('logs', selected.labels.orchestrator_workload)}>{label('nativeLogs')}</Button><Button onClick={() => { setWorkload(selected.labels.orchestrator_workload ?? ''); }}>{label('metrics')}</Button><Button onClick={() => navigate({ name: 'deployments', applicationId, environment })}>{label('deployments')}</Button></section> : null}
    {data?.grafanaUrl ? <a href={data.grafanaUrl} target="_blank" rel="noreferrer">{label('grafana')}</a> : null}
    <details><summary>{label('coverage')}</summary><p>{label('coverageHelp')}</p></details>
  </section>;
}
