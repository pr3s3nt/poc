import { api } from '../../../../shared/api/client';
import { t } from '../../../../shared/i18n';

// Infrastructure Account diagnostics (ADR-022, UC-04 DIAG-01..14). The DTOs are
// the backend's safe projection; this file adds no fixture data and the Console
// never reads provider text: every sentence is chosen from a stable code.

export type Verdict = 'PASS' | 'FAIL' | 'BLOCKED' | 'UNKNOWN' | 'NA';
export type Purpose = 'GENERAL' | 'USE_CASE';
export type Mode = 'CONDITIONS' | 'EXECUTION';
export type RunStatus = 'QUEUED' | 'RUNNING' | 'CANCELLING' | 'COMPLETED' | 'CANCELLED' | 'INTERRUPTED';
export const verdicts: Verdict[] = ['PASS', 'FAIL', 'BLOCKED', 'UNKNOWN', 'NA'];
export const useCaseKeys = ['preview', 'deploy', 'database', 'cache', 'secrets', 'https', 'lifecycle'] as const;
export type UseCase = (typeof useCaseKeys)[number];

export type Counts = { pass: number; fail: number; blocked: number; unknown: number; na: number; pending: number };
export type Scope = {
  purpose: Purpose; mode: Mode; region?: string; useCases: string[];
  context?: { applicationKey: string; applicationName?: string; environmentKey?: string; environmentType?: string };
  accountKey: string; accountName: string; accountType: 'AWS' | 'INTERNAL'; accountVersion: number; identity?: Record<string, string>;
};
export type Finding = {
  id: string; group: string; verdict: Verdict; reason: string; facts: Record<string, string>; remediation?: string;
  affectedUseCases: string[]; dependsOn: string[]; observedAt: string; durationMs: number;
};
export type Summary = {
  reportId: string; scope: Scope; status: RunStatus; counts: Counts; ready: boolean; cleanupStatus: string; stale: boolean; createdAt: string; finishedAt?: string;
};
export type Report = {
  id: string; scope: Scope; status: RunStatus; progress: { completed: number; total: number }; counts: Counts; ready: boolean;
  findings: Finding[]; cleanup: { status: string; resources: { kind: string; namespace?: string; name: string; uid?: string; state: string }[] };
  limits: string[]; stale: boolean; staleReasons: string[]; createdAt: string; startedAt?: string; finishedAt?: string; updatedAt: string;
};
export type StartBody = { version: number; purpose: Purpose; mode: Mode; region?: string; useCases?: string[]; application?: string; environment?: string };

const base = (account: string) => `/driver-accounts/${encodeURIComponent(account)}/diagnostics`;
export const startDiagnostic = (account: string, body: StartBody) => api<Report>(base(account), { method: 'POST', body: JSON.stringify(body) });
export const getDiagnostic = (account: string, id: string) => api<Report>(`${base(account)}/${encodeURIComponent(id)}`);
export const cancelDiagnostic = (account: string, id: string) => api<Report>(`${base(account)}/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: '{}' });
export const listDiagnostics = (account: string, purpose: Purpose, cursor = '') =>
  api<{ reports: Summary[]; nextCursor: string }>(`${base(account)}?purpose=${purpose}&limit=20${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`);

export const active = (status: RunStatus) => status === 'QUEUED' || status === 'RUNNING' || status === 'CANCELLING';

/** Verdict of the whole report for badges: never PASS while anything is unresolved. */
export function overall(counts: Counts, status: RunStatus): Verdict {
  if (counts.fail) return 'FAIL';
  if (counts.blocked) return 'BLOCKED';
  if (counts.unknown || counts.pending || status !== 'COMPLETED') return 'UNKNOWN';
  return counts.pass ? 'PASS' : 'NA';
}

// Safe-code catalogs. A missing key shows the generic text, never the code's
// raw detail, so a new backend code degrades to a neutral sentence.
function catalog(prefix: string, code: string, fallback: string): string {
  const key = `platform.diagnostics.${prefix}.${code}`;
  const text = t(key);
  return text === key ? t(`platform.diagnostics.${fallback}`) : text;
}
export const checkTitle = (id: string) => catalog('check', id, 'unknownCheck');
export const checkExpected = (id: string) => catalog('expected', id, 'unknownExpected');
export const reasonText = (reason: string) => catalog('reason', reason, 'unknownReason');
export const fixSteps = (key: string | undefined): string[] => {
  if (!key) return [];
  const text = catalog('fix', key, 'unknownFix');
  return text.split('\n').filter(Boolean);
};
export const factLabel = (key: string) => catalog('fact', key, 'unknownFact');
export const limitText = (code: string) => catalog('limit', code, 'unknownLimit');
export const groupText = (group: string) => catalog('group', group, 'unknownGroup');
export const staleText = (code: string) => catalog('stale', code, 'unknownStale');
export const resourceStateText = (state: string) => catalog('resourceState', state, 'unknownReason');

export function scopeLabel(scope: Scope): string {
  return [
    scope.purpose === 'GENERAL' ? t('platform.diagnostics.general') : t('platform.diagnostics.useCaseScope'),
    scope.region ?? '',
    scope.context ? `${scope.context.applicationName ?? scope.context.applicationKey}${scope.context.environmentKey ? ` / ${scope.context.environmentKey}` : ''}` : '',
  ].filter(Boolean).join(' · ');
}

export const statusKey = (status: RunStatus) => `platform.diagnostics.status.${status}`;
