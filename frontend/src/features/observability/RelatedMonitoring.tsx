import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { Chart, type Series } from './MonitoringChart';
export type RelatedObservation = { source?: string; id: string; name: string; kind: string; type?: string; provenance?: string; collectorWorkload?: string; state: string; configuration: string; profileVersion?: string; metricsAt?: string; from?: string; to?: string; metrics: Record<string, { unit: string; state: string; series: Series[] }>; alerts: { name: string; state: string; labels: Record<string,string> }[]; grafanaUrl?: string };
const label = (key: string) => t(`monitoring.${key}`);
export function RelatedPanel({ item, applicationId, environment }: { item: RelatedObservation; applicationId: string; environment: EnvironmentKey }) {
 const query = { workload: item.collectorWorkload, from: item.from, to: item.to };
 return <section aria-label={item.name} className="content-panel"><div className="section-header"><h4>{item.kind === 'resource' ? <button className="back-link" onClick={() => navigate({ name:'resource', applicationId, environment, resourceId:item.id })}>{item.name}</button> : item.name}</h4><span>{label(item.state)}</span></div>
  {item.collectorWorkload ? <p>{item.source ? `${item.source} · ` : ''}{label('collector')}: {item.collectorWorkload} · {label(item.configuration)}{item.profileVersion ? ` · ${item.profileVersion}` : ''}{item.metricsAt ? ` · ${new Date(item.metricsAt).toLocaleString()}` : ''}</p> : null}
  {item.kind === 'dependency' ? <small>{label('declaredDependency')}</small> : null}
  <div className="obs-chart-grid">{Object.entries(item.metrics).map(([key,metric]) => <Chart key={key} title={`${key} (${metric.unit}) · ${label(metric.state)}`} series={metric.series ?? []} />)}</div>
  {item.alerts.map(alert => <p key={JSON.stringify(alert.labels)}>{alert.name} · {alert.labels.metric_key} · {label(alert.state)}</p>)}
  {item.collectorWorkload ? <div className="section-header"><Button onClick={() => navigate({name:'observability',applicationId,environment,tab:'metrics',query})}>{label('metrics')}</Button><Button onClick={() => navigate({name:'observability',applicationId,environment,tab:'logs',query})}>{label('logs')}</Button><Button onClick={() => navigate({name:'deployments',applicationId,environment})}>{label('deployments')}</Button>{item.grafanaUrl ? <a href={item.grafanaUrl} target="_blank" rel="noreferrer">{label('grafana')}</a> : null}</div> : null}
 </section>;
}
export function ResourceMonitoring({ applicationId, environment, resourceId }: { applicationId:string; environment:EnvironmentKey; resourceId:string }) {
 const [result,setResult]=useState<RelatedObservation>();const [error,setError]=useState('');const [minutes,setMinutes]=useState(60);const [reload,setReload]=useState(0);
 useEffect(() => {let active=true;setResult(undefined);setError('');const load=async()=>{const to=new Date(),from=new Date(to.getTime()-minutes*60000),q=new URLSearchParams({from:from.toISOString(),to:to.toISOString()});try{const value=await api<RelatedObservation>(`/applications/${encodeURIComponent(applicationId)}/environments/${environment}/monitoring/resources/${encodeURIComponent(resourceId)}?${q}`);if(value.id!==resourceId||!value.metrics||!Array.isArray(value.alerts))throw new Error();if(active){setResult(value);setError('')}}catch(e){if(active)setError(describeError(e,label('failed')))}};void load();const timer=window.setInterval(()=>void load(),30000);return()=>{active=false;clearInterval(timer)}},[applicationId,environment,resourceId,minutes,reload]);
 return <div><div className="section-header"><label>{label('range')} <select value={minutes} onChange={e=>setMinutes(Number(e.target.value))}>{[15,60,360,1440].map(n=><option key={n} value={n}>{n} {label('minutes')}</option>)}</select></label><Button onClick={()=>setReload(n=>n+1)}>{label('reload')}</Button></div>{error?<p role="alert" className="form-error">{error}</p>:null}{result?<RelatedPanel item={result} applicationId={applicationId} environment={environment}/>:!error?<p role="status">{label('loading')}</p>:null}</div>;
}
