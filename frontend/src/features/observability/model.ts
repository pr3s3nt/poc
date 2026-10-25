import { intlLocale, t } from '../../shared/i18n';
import type { Event, Focus, Metric, Peak, Sample, Snapshot, UsagePeak } from './api';

export const text = (key: string, params?: Record<string, unknown>) => t(`observability.native.${key}`, params);
export const time = (raw: string | number, seconds = false) => new Intl.DateTimeFormat(intlLocale(), { hour: '2-digit', minute: '2-digit', ...(seconds ? { second: '2-digit' } : {}) }).format(new Date(raw));
export const dateTime = (raw: string | number) => new Date(raw).toLocaleString(intlLocale());
export const number = (value: number, digits = 0) => new Intl.NumberFormat(intlLocale(), { maximumFractionDigits: digits }).format(value);
export const timezone = () => Intl.DateTimeFormat().resolvedOptions().timeZone;
export const valueOf = (sample: Sample, metric: Metric) => metric === 'cpu' ? sample.cpuMilli : sample.memoryBytes;
export const limitOf = (sample: Sample, metric: Metric) => metric === 'cpu' ? sample.cpuLimitMilli : sample.memoryLimitBytes;
export const displayValue = (metric: Metric, value: number) => number(metric === 'memory' ? value / 1048576 : value, 1);
export const unit = (metric: Metric) => text(metric === 'cpu' ? 'unitCPU' : 'unitMemory');
export const fresh = (sample: Sample) => Date.parse(sample.metricsAt ?? sample.timestamp) >= Date.now() - 90_000;
export const percentage = (value: number | null | undefined, limit: number | null | undefined) => value !== null && value !== undefined && limit && limit > 0 ? value / limit * 100 : null;
const palette = ['#0969da', '#8250df', '#1a7f64', '#bf8700', '#cf222e', '#0550ae'];
export const color = (workload: string, workloads: readonly string[]) => palette[Math.max(0, workloads.indexOf(workload)) % palette.length]!;

export function peakOf(data: Snapshot, metric: Metric): Peak | UsagePeak | undefined {
  const relative = data.peaks.filter(p => p.metric === metric).sort((a, b) => b.percent - a.percent)[0];
  return relative ?? data.usagePeaks.filter(p => p.metric === metric).sort((a, b) => b.value - a.value)[0];
}
export const isRelativePeak = (peak: Peak | UsagePeak): peak is Peak => 'limit' in peak && peak.limit > 0;

export function filledPoints(data: Snapshot, workload: string, metric: Metric): { at: number; value: number | null }[] {
  const step = Math.max(30, data.stepSeconds) * 1000, start = Math.floor(Date.parse(data.from) / step) * step, end = Date.parse(data.to);
  const samples = new Map(data.points.filter(p => p.workloadId === workload).map(p => [Math.floor(Date.parse(p.timestamp) / step) * step, valueOf(p, metric)]));
  const points: { at: number; value: number | null }[] = [];
  for (let at = start; at < end && points.length <= 242; at += step) points.push({ at, value: samples.get(at) ?? null });
  return points;
}

export type Signal = { id: string; name: string; detail: string; timestamp: string; workload: string; view: 'logs' | 'metrics'; focus?: Focus; level?: string; state: string; tone: string };
function eventName(event: Event): string {
  if (event.kind === 'RESTART') return text('recentRestart');
  if (event.reason === 'OOMKilled') return text('outOfMemory');
  if (event.reason === 'COLLECTION_GAP') return text('collectionGap');
  if (event.reason === 'LOG_TRUNCATED') return text('logTruncated');
  return text('workloadIssue');
}
export function signalsOf(data: Snapshot): Signal[] {
  const signals: Signal[] = data.peaks.filter(p => p.percent >= 80).map(peak => {
    const latest = data.latest.find(s => s.workloadId === peak.workloadId);
    const current = latest && fresh(latest) ? percentage(valueOf(latest, peak.metric), limitOf(latest, peak.metric)) : null;
    return { id: `peak-${peak.workloadId}-${peak.metric}`, name: text(peak.metric === 'cpu' ? 'highCPU' : 'highMemory'), detail: `${peak.workloadId} · ${text('limitDetail', { value: displayValue(peak.metric, peak.value), limit: displayValue(peak.metric, peak.limit), unit: unit(peak.metric) })} (${number(peak.percent)}%)`, timestamp: peak.timestamp, workload: peak.workloadId, view: 'metrics', focus: peak.metric, state: current === null ? text('currentUnavailable') : text(current >= 80 ? 'highNow' : 'recovered'), tone: 'amber' };
  });
  for (const event of data.events) signals.push({ id: event.id, name: eventName(event), detail: `${event.workloadId} · ${event.reason} · ${number(event.occurrences)}`, timestamp: event.timestamp, workload: event.workloadId, view: 'metrics', focus: 'signals', state: text('observed'), tone: event.kind === 'RESTART' ? 'violet' : 'amber' });
  return signals.sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp));
}
export function noteworthy(data: Snapshot): Signal[] {
  const signals = signalsOf(data);
  const error = data.logs.find(log => log.level === 'ERROR');
  if (error) signals.unshift({ id: error.id, name: text('logErrors'), detail: `${error.workloadId} · ${error.message}`, timestamp: error.timestamp, workload: error.workloadId, view: 'logs', level: 'ERROR', state: 'ERROR', tone: 'red' });
  return signals.sort((a, b) => Date.parse(b.timestamp) - Date.parse(a.timestamp)).slice(0, 4);
}

const stateLabels: Record<string, string> = { RECEIVING: 'healthy', NOT_DEPLOYED: 'empty', WAITING: 'waiting', STALE: 'gap', PARTIAL: 'gap', METRICS_UNAVAILABLE: 'missing', STORAGE_LIMIT: 'storageLimit', UNAVAILABLE: 'unavailable', INACTIVE: 'inactive' };
export const stateLabel = (state: string) => text(Object.hasOwn(stateLabels, state) ? stateLabels[state]! : 'absent');
