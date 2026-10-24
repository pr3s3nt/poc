import type { Report, WorkRecord } from './api';
import { actionLabel, metricName, requiredSourceLabel, stateLabel } from './labels';
import { t } from '../../shared/i18n';

// Formats the server report and its records; nothing is recomputed here.

const cell = (value: string | number | undefined | null) => `"${String(value ?? '').replaceAll('"', '""')}"`;
const row = (...values: (string | number | undefined | null)[]) => values.map(cell).join(',');

const statusText: Readonly<Record<string, string>> = { COLLECTED: t('effectiveness.csv.hasData'), NO_DATA: t('effectiveness.csv.noDataYet'), NOT_COLLECTED: t('effectiveness.csv.notCollected') };
const unitText: Readonly<Record<string, string>> = { COUNT: t('effectiveness.csv.count'), RATIO: t('effectiveness.csv.ratio01'), SECONDS: t('effectiveness.csv.seconds') };
const sourceText: Readonly<Record<string, string>> = { deployments: t('effectiveness.csv.deploymentHistory'), resource_cleanups: t('effectiveness.csv.resourceDeletion') };
const lookup = (table: Readonly<Record<string, string>>, key: string) => (Object.hasOwn(table, key) ? table[key] : key);

export function scopeLabel(report: Report): string {
  if (report.scope.level === 'ORGANIZATION') return t('effectiveness.csv.wholeOrganization');
  const app = report.scope.applicationName || report.scope.applicationKey || '';
  return report.scope.level === 'WORKLOAD' ? `${app} › ${report.scope.workloadId ?? ''}` : app;
}

export function buildCsv(report: Report, records: WorkRecord[]): string {
  const lines = [
    row(t('effectiveness.csv.platformEffectivenessReport')),
    row(t('effectiveness.csv.scope'), scopeLabel(report)),
    row(t('effectiveness.csv.period'), `${report.period.from} — ${report.period.to}`, t('effectiveness.csv.timeZone'), report.period.timezone),
    row(t('effectiveness.csv.dataSource'), report.dataOrigin === 'PRODUCTION' ? 'Production' : t('effectiveness.csv.testDataNonProductionInstallation')),
    row(t('effectiveness.csv.generatedAt'), report.generatedAt),
    row(t('effectiveness.csv.source'), ...report.sources.map((source) => `${lookup(sourceText, source.id)}: ${source.available ? t('effectiveness.csv.available') : t('effectiveness.csv.unavailable')}`)),
    '',
    row(t('effectiveness.csv.metricID'), t('effectiveness.csv.metric'), t('effectiveness.csv.status'), t('effectiveness.csv.value'), t('effectiveness.csv.unit'), t('effectiveness.csv.numerator'), t('effectiveness.csv.denominator'), t('effectiveness.csv.coverage'), t('effectiveness.csv.requiredSource')),
    ...report.metrics.map((m) => row(m.id, metricName(m.id), lookup(statusText, m.status), m.value, lookup(unitText, m.unit), m.numerator, m.denominator,
      m.coverage ? `${m.coverage.covered}/${m.coverage.total}` : '', requiredSourceLabel(m.requiredSource))),
  ];
  if (report.baseline) {
    const b = report.baseline;
    lines.push('', row(t('effectiveness.csv.baseline'), t('effectiveness.csv.versionVersion', { version: b.version }), t('effectiveness.csv.valueSecondsMedian', { value: b.valueSeconds ?? '' }), t('effectiveness.csv.effectiveFromEffectiveFrom', { effectiveFrom: b.effectiveFrom }), b.source));
  }
  lines.push('', row(t('effectiveness.csv.workID'), t('effectiveness.csv.application'), t('effectiveness.csv.environment'), 'Workload', t('effectiveness.csv.resource'), t('effectiveness.csv.type'), t('effectiveness.csv.status'), t('effectiveness.csv.attempts'), t('effectiveness.csv.failedAttempts'), t('effectiveness.csv.attemptsInPeriod'),
    t('effectiveness.csv.started'), t('effectiveness.csv.completed'), t('effectiveness.csv.leadTimeSeconds'), t('effectiveness.csv.performedBy'), t('effectiveness.csv.lastRun')));
  for (const r of records) {
    lines.push(row(r.id, r.applicationName || r.applicationKey, r.environmentKey, r.workloadId, r.resourceId, actionLabel(r.action), stateLabel(r.state),
      r.attempts, r.failedAttempts, r.attemptsInPeriod, r.firstStartedAt, r.completedAt, r.durationSeconds,
      r.actors.map((a) => (a.kind === 'USER' ? a.name : t('effectiveness.csv.nameAutomated', { name: a.name }))).join('; '), r.lastRunId));
  }
  return `\uFEFF${lines.join('\r\n')}\r\n`;
}

export function csvFileName(report: Report): string {
  const scope = report.scope.level === 'ORGANIZATION' ? 'to-chuc' : [report.scope.applicationKey, report.scope.workloadId].filter(Boolean).join('-');
  return `hieu-qua-platform-${scope.replace(/[^A-Za-z0-9._-]+/g, '-')}-${report.period.from}-${report.period.to}.csv`;
}
