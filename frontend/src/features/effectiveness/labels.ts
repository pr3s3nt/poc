import type { FrequencyBand, Metric } from './api';
import { intlLocale, t } from '../../shared/i18n';

// Vietnamese copy of the UC-17 catalog. Unknown IDs from a newer server still render with their ID.
type MetricCopy = { name: string; term?: string; formula: string };

export const metricCopy: Readonly<Record<string, MetricCopy>> = {
  deployment_frequency: { name: t('effectiveness.labels.deploymentFrequency'), term: 'Deployment frequency', formula: t('effectiveness.labels.numberOfDeploymentsOrUpdates') },
  change_lead_time: { name: t('effectiveness.labels.commitToProduction'), term: 'Change lead time', formula: t('effectiveness.labels.timeFromCommitUntilThe') },
  change_fail_rate: { name: t('effectiveness.labels.changeFailureRate'), term: 'Change fail rate', formula: t('effectiveness.labels.shareOfProductionDeploymentsNeeding') },
  failed_deployment_recovery_time: { name: t('effectiveness.labels.timeToRestore'), term: 'Failed deployment recovery time', formula: t('effectiveness.labels.timeFromDetectingAFailed') },
  deployment_rework_rate: { name: t('effectiveness.labels.remediationDeployments'), term: 'Deployment rework rate', formula: t('effectiveness.labels.shareOfUnplannedDeploymentsMade') },
  active_applications: { name: t('effectiveness.labels.activeApplications'), formula: t('effectiveness.labels.applicationsWithAtLeastOne') },
  active_users: { name: t('effectiveness.labels.activeUsers'), formula: t('effectiveness.labels.numberOfDistinctAccountsThat') },
  completed_work: { name: t('effectiveness.labels.completedWork'), formula: t('effectiveness.labels.numberOfDeployUpdateRemove') },
  execution_attempts: { name: t('effectiveness.labels.runs'), formula: t('effectiveness.labels.numberOfRunsStartedIn') },
  work_duration: { name: t('effectiveness.labels.leadTime'), formula: t('effectiveness.labels.medianTimeFromTheFirst') },
  work_duration_vs_baseline: { name: t('effectiveness.labels.timeSaved'), formula: t('effectiveness.labels.baselineMinusTheMedianLead') },
  request_lead_time: { name: t('effectiveness.labels.requestToCompletion'), formula: t('effectiveness.labels.timeFromTheRequestUntil') },
  self_service_rate: { name: t('effectiveness.labels.selfService'), formula: t('effectiveness.labels.shareOfCompletedWorkItems') },
  ops_effort: { name: t('effectiveness.labels.opsEffort'), formula: t('effectiveness.labels.opsPersonHoursSpentOn') },
  onboarding_time: { name: 'Onboarding', formula: t('effectiveness.labels.timeFromTheStartOf') },
  labor_hours_saved: { name: t('effectiveness.labels.personHoursSaved'), formula: t('effectiveness.labels.baselineEffortMinusEffortThrough') },
};

export const requiredSourceLabels: Readonly<Record<string, string>> = {
  COMMIT_LINK: t('effectiveness.labels.requiresLinkingDeploymentsToCommits'),
  INCIDENTS: t('effectiveness.labels.requiresPostDeploymentIncidentData'),
  REQUESTS: t('effectiveness.labels.requiresTheRequestCreationTime'),
  SUPPORT_RECORDS: t('effectiveness.labels.requiresASupportRecordFor'),
  EFFORT_RECORDS: t('effectiveness.labels.requiresEffortRecordsBothFor'),
  ONBOARDING_START: t('effectiveness.labels.requiresTheOnboardingStartTime'),
};

const own = <T,>(table: Readonly<Record<string, T>>, key: string): T | undefined => (Object.hasOwn(table, key) ? table[key] : undefined);

export const metricName = (id: string) => own(metricCopy, id)?.name ?? id;
export const metricFormula = (id: string) => own(metricCopy, id)?.formula ?? '';
export const metricTerm = (id: string) => own(metricCopy, id)?.term;
export const requiredSourceLabel = (source?: string) => (source ? own(requiredSourceLabels, source) ?? '' : '');

export const bandLabels: Readonly<Record<FrequencyBand, string>> = { DAILY: t('effectiveness.labels.daily'), WEEKLY: t('effectiveness.labels.weekly'), MONTHLY: t('effectiveness.labels.monthly'), RARE: t('effectiveness.labels.lessOften'), NONE: t('effectiveness.labels.none') };
export const bandDetailKeys: Readonly<Record<FrequencyBand, string>> = { DAILY: 'bandDaily', WEEKLY: 'bandWeekly', MONTHLY: 'bandMonthly', RARE: 'bandRare', NONE: 'bandNone' };

export const actionLabels: Readonly<Record<string, string>> = { DEPLOY: t('effectiveness.labels.deploy'), UPDATE: t('effectiveness.labels.update'), REMOVE: t('effectiveness.labels.remove'), RESOURCE_CLEANUP: t('effectiveness.labels.resourceDeletion') };
export const stateLabels: Readonly<Record<string, string>> = { COMPLETED: t('effectiveness.labels.completed'), RUNNING: t('effectiveness.labels.running'), FAILED_OPEN: t('effectiveness.labels.failedUnresolved') };
export const actionLabel = (action: string) => own(actionLabels, action) ?? action;
export const stateLabel = (state: string) => own(stateLabels, state) ?? state;

const number = new Intl.NumberFormat(intlLocale(), { maximumFractionDigits: 1 });
export const formatNumber = (value: number) => number.format(value);

/** Elapsed time in the largest readable unit; the sign is kept. */
export function formatDuration(seconds: number): string {
  const sign = seconds < 0 ? '−' : '';
  const abs = Math.abs(seconds);
  if (abs < 60) return t('effectiveness.labels.signFormatNumberSeconds', { sign, formatNumber: formatNumber(abs) });
  if (abs < 3600) return t('effectiveness.labels.signFormatNumberMinutes', { sign, formatNumber: formatNumber(abs / 60) });
  if (abs < 86400 * 2) return t('effectiveness.labels.signFormatNumberHours', { sign, formatNumber: formatNumber(abs / 3600) });
  return t('effectiveness.labels.signFormatNumberDays', { sign, formatNumber: formatNumber(abs / 86400) });
}

export const formatDate = (date: string) => {
  const [y, m, d] = date.split('-');
  return y && m && d ? `${d}/${m}/${y}` : date;
};

export const formatDateTime = (value?: string) => {
  if (!value) return '—';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '—' : parsed.toLocaleString(intlLocale(), { timeZone: 'Asia/Ho_Chi_Minh' });
};

/** The value shown on a metric card; missing data is never shown as zero. */
export function metricValue(metric: Metric): string {
  if (metric.status === 'NOT_COLLECTED') return t('effectiveness.labels.notCollected');
  if (metric.status === 'NO_DATA' || metric.value === undefined) return t('effectiveness.labels.noDataYet');
  if (metric.unit === 'SECONDS') return formatDuration(metric.value);
  if (metric.unit === 'RATIO') return `${formatNumber(metric.value * 100)}%`;
  if (metric.numerator !== undefined && metric.denominator !== undefined && metric.id === 'active_applications') return `${formatNumber(metric.numerator)}/${formatNumber(metric.denominator)}`;
  return formatNumber(metric.value);
}

/** The short secondary line of a metric card, if any. */
export function metricNote(metric: Metric): { text: string; warning?: boolean } | undefined {
  if (metric.status !== 'COLLECTED') return undefined;
  const d = metric.details ?? {};
  switch (metric.id) {
    case 'deployment_frequency': return { text: t('effectiveness.labels.formatNumberPerWeekProduction', { formatNumber: formatNumber(d.perWeek ?? 0) }) };
    case 'active_users': return d.automationRuns ? { text: t('effectiveness.labels.formatNumberAutomatedRuns', { formatNumber: formatNumber(d.automationRuns) }) } : undefined;
    case 'completed_work': return d.openWork ? { text: t('effectiveness.labels.openFormatNumber', { formatNumber: formatNumber(d.openWork) }) } : undefined;
    case 'execution_attempts': return { text: t('effectiveness.labels.valueFormatNumberFailed', { value: d.attemptsPerItem !== undefined ? t('effectiveness.labels.formatNumberPerWorkItem', { formatNumber: formatNumber(d.attemptsPerItem) }) : '', formatNumber: formatNumber(d.failedRuns ?? 0) }) };
    case 'work_duration': {
      const coverage = metric.coverage;
      if (coverage && coverage.covered < coverage.total) return { text: t('effectiveness.labels.coverageCoveredTotal', { covered: coverage.covered, total: coverage.total }), warning: true };
      return { text: t('effectiveness.labels.medianP90FormatDuration', { formatDuration: formatDuration(d.p90Seconds ?? 0) }) };
    }
    default: return undefined;
  }
}
