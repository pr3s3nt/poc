import { api } from '../../shared/api/client';

// UC-17 / OC-20. Every number comes from the server; the Console only formats it.

export type MetricStatus = 'COLLECTED' | 'NO_DATA' | 'NOT_COLLECTED';
export type MetricUnit = 'COUNT' | 'RATIO' | 'SECONDS';
export type MetricGroup = 'DORA' | 'USAGE' | 'SELF_SERVICE' | 'VALUE';
export type FrequencyBand = 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'RARE' | 'NONE';

export type Metric = {
  id: string; group: MetricGroup; unit: MetricUnit; status: MetricStatus;
  value?: number; numerator?: number; denominator?: number;
  coverage?: { covered: number; total: number };
  details?: Record<string, number>;
  requiredSource?: string;
};

export type Baseline = {
  id: string; metric: string; applicationKey?: string; version: number; retired: boolean; statistic?: string;
  valueSeconds?: number; source: string; sampleSize?: number; measuredFrom?: string; measuredTo?: string;
  effectiveFrom: string; createdBy: string; createdAt: string;
};

export type BreakdownRow = {
  key: string; name: string; productionDeployments: number; frequencyBand: FrequencyBand; completedWork: number; openWork: number;
  attempts: number; failedAttempts: number; medianDurationSeconds: number | null; activeUsers: number;
};

export type Report = {
  scope: { level: 'ORGANIZATION' | 'APPLICATION' | 'WORKLOAD'; applicationKey?: string; applicationName?: string; workloadId?: string };
  period: { from: string; to: string; timezone: string; days: number };
  dataOrigin: 'PRODUCTION' | 'TEST_INSTALLATION';
  generatedAt: string;
  sources: { id: string; available: boolean }[];
  metrics: Metric[];
  breakdown: { level: 'APPLICATION' | 'WORKLOAD'; rows: BreakdownRow[] } | null;
  trend: { bucket: 'DAY' | 'WEEK'; points: { start: string; productionDeployments: number; completedWork: number }[] };
  baseline: Baseline | null;
};

export type WorkRecord = {
  id: string; kind: 'DEPLOYMENT' | 'RESOURCE_CLEANUP'; applicationKey: string; applicationName: string; environmentKey: string;
  workloadId?: string; resourceId?: string; action: string; state: 'COMPLETED' | 'RUNNING' | 'FAILED_OPEN';
  attempts: number; failedAttempts: number; attemptsInPeriod: number; firstStartedAt: string; completedAt?: string; durationSeconds?: number;
  actors: { name: string; kind: 'USER' | 'AUTOMATION' }[]; lastRunId: string;
};

export type Scope = { application?: string; workload?: string; from: string; to: string };

function query(scope: Scope, extra: Record<string, string> = {}) {
  const params = new URLSearchParams({ from: scope.from, to: scope.to });
  if (scope.application) params.set('application', scope.application);
  if (scope.application && scope.workload) params.set('workload', scope.workload);
  for (const [key, value] of Object.entries(extra)) params.set(key, value);
  return params.toString();
}

export function getReport(scope: Scope) {
  return api<Report>(`/effectiveness/report?${query(scope)}`);
}

export function listRecords(scope: Scope, metric?: string) {
  return api<{ records: WorkRecord[] }>(`/effectiveness/records?${query(scope, metric ? { metric } : {})}`);
}

export function listBaselines(application?: string) {
  const params = application ? `?${new URLSearchParams({ application })}` : '';
  return api<{ currentVersion: number; baselines: Baseline[] }>(`/effectiveness/baselines${params}`);
}

export type BaselineDraft = {
  applicationKey?: string; expectedVersion: number; retired?: boolean; valueSeconds?: number; source: string;
  effectiveFrom: string; sampleSize?: number; measuredFrom?: string; measuredTo?: string;
};

export function addBaseline(draft: BaselineDraft) {
  return api<Baseline>('/effectiveness/baselines', { method: 'POST', body: JSON.stringify({ metric: 'work_duration', ...draft }) });
}

const reportZone = 'Asia/Ho_Chi_Minh';

/** Today's date in the report timezone, YYYY-MM-DD. */
export function today(now = new Date()): string {
  return new Intl.DateTimeFormat('en-CA', { timeZone: reportZone, year: 'numeric', month: '2-digit', day: '2-digit' }).format(now);
}

/** The date `days` calendar days after `date` (before when negative). */
export function addDays(date: string, days: number): string {
  const [y, m, d] = date.split('-').map(Number);
  return new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, (d ?? 1) + days)).toISOString().slice(0, 10);
}

/** The inclusive period of the last `days` days ending today. */
export function periodOf(days: number, now = new Date()): { from: string; to: string } {
  const to = today(now);
  return { from: addDays(to, -(days - 1)), to };
}
