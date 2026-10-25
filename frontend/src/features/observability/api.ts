import { api } from '../../shared/api/client';
import type { EnvironmentKey } from '../../shared/types/application';

export type Metric = 'cpu' | 'memory';
export type Focus = Metric | 'signals';
export type Filters = { from?: string; to?: string; workload?: string; q?: string; level?: string; cursor?: string; limit?: number };
export type Health = { workloadId: string; logsCode: string; metricsCode: string; observedAt: string | null; lastLogAt: string | null; lastMetricAt: string | null; storageLimited: boolean };
export type Log = { id: string; workloadId: string; timestamp: string; podUid: string; podName: string; container: string; restartIndex: number; level: string; message: string };
export type Sample = { workloadId: string; timestamp: string; metricsAt: string | null; cpuMilli: number | null; memoryBytes: number | null; cpuLimitMilli: number | null; memoryLimitBytes: number | null; readyPods: number; totalPods: number; restarts: number };
export type Peak = { workloadId: string; metric: Metric; timestamp: string; value: number; limit: number; percent: number };
export type UsagePeak = { workloadId: string; metric: Metric; timestamp: string; value: number };
export type Event = { id: string; workloadId: string; timestamp: string; kind: string; reason: string; occurrences: number; podName: string; container: string };
export type Snapshot = {
  state: string; from: string; to: string; generatedAt: string; stepSeconds: number;
  workloads: string[]; sources: Health[]; signalCount: number; logsRetentionDays: number; metricsRetentionDays: number; hasHistory: boolean;
  logs: Log[]; logCount: number; errorCount: number; histogram: { timestamp: string; count: number }[]; nextCursor?: string;
  points: Sample[]; latest: Sample[]; peaks: Peak[]; usagePeaks: UsagePeak[]; events: Event[]; eventCount: number;
};

export function interval(minutes: number): Filters {
  const now = Date.now();
  return { from: new Date(now - minutes * 60_000).toISOString(), to: new Date(now).toISOString() };
}

export async function readTelemetry(app: string, environment: EnvironmentKey, kind: 'overview' | 'logs' | 'metrics', filters: Filters = {}, signal?: AbortSignal): Promise<Snapshot> {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) if (value !== undefined && value !== '') query.set(key, String(value));
  const result = await api<Snapshot>(`/applications/${encodeURIComponent(app)}/environments/${environment}/observability/${kind}?${query}`, { signal });
  if (!result || typeof result.state !== 'string' || !Array.isArray(result.sources) || !Array.isArray(result.logs) || !Array.isArray(result.points) || !Array.isArray(result.peaks) || !Array.isArray(result.usagePeaks) || !Array.isArray(result.latest) || !Array.isArray(result.events) || !Array.isArray(result.workloads) || !Array.isArray(result.histogram)) throw new Error('Telemetry response unavailable');
  const stamp = (value: unknown): value is string => typeof value === 'string' && Number.isFinite(Date.parse(value));
  const count = (value: unknown) => typeof value === 'number' && Number.isFinite(value) && value >= 0;
  const optionalNumber = (value: unknown) => value === null || count(value);
  const sample = (value: Sample) => typeof value.workloadId === 'string' && stamp(value.timestamp) && (value.metricsAt === null || stamp(value.metricsAt)) && [value.cpuMilli, value.memoryBytes, value.cpuLimitMilli, value.memoryLimitBytes].every(optionalNumber) && [value.readyPods, value.totalPods, value.restarts].every(count);
  if (![result.from, result.to, result.generatedAt].every(stamp) || !count(result.stepSeconds) || result.stepSeconds < 30 || ![result.logCount, result.errorCount, result.eventCount, result.signalCount].every(count)
    || !result.points.every(sample) || !result.latest.every(sample)
    || !result.logs.every(row => stamp(row.timestamp) && typeof row.message === 'string' && typeof row.id === 'string' && typeof row.workloadId === 'string')
    || !result.sources.every(source => [source.observedAt, source.lastLogAt, source.lastMetricAt].every(value => value === null || stamp(value)))
    || !result.peaks.every(peak => stamp(peak.timestamp) && count(peak.value) && count(peak.percent) && count(peak.limit) && peak.limit > 0)
    || !result.usagePeaks.every(peak => stamp(peak.timestamp) && count(peak.value))
    || !result.events.every(event => stamp(event.timestamp) && count(event.occurrences))
    || !result.histogram.every(bucket => stamp(bucket.timestamp) && count(bucket.count))) throw new Error('Telemetry response unavailable');
  return result;
}
