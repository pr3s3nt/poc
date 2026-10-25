import { intlLocale, t } from '../../../../shared/i18n';
import type { Account } from '../accountsApi';
import { overall, scopeLabel, type Purpose } from './diagnosticsApi';

/**
 * Latest diagnostic of one account, shown apart from the connection status.
 * Its scope and time are always shown, and an unreadable history is not
 * presented as "never checked".
 */
export function LatestDiagnostic({ account, onOpen }: { account: Account; onOpen(purpose: Purpose, reportId: string): void }) {
  if (account.diagnosticsUnavailable) return <span className="ac-not-checked" role="status">{t('platform.diagnostics.latestUnavailable')}</span>;
  const latest = account.latestDiagnostic;
  if (!latest) return <span className="ac-not-checked">{t('platform.diagnostics.neverChecked')}</span>;
  const verdict = overall(latest.counts, latest.status);
  const running = latest.status === 'QUEUED' || latest.status === 'RUNNING' || latest.status === 'CANCELLING';
  const label = running ? t('platform.diagnostics.running')
    : latest.counts.fail ? t('platform.diagnostics.summaryFailure', { count: latest.counts.fail })
      : latest.counts.blocked ? t('platform.diagnostics.summaryBlocked', { count: latest.counts.blocked })
        : latest.counts.unknown || latest.status !== 'COMPLETED' ? t('platform.diagnostics.summaryUnknown', { count: latest.counts.unknown })
          : t('platform.diagnostics.summaryPass');
  return <button type="button" className="ac-latest-result" aria-label={t('platform.diagnostics.openLatest', { name: account.name })} onClick={() => onOpen(latest.scope.purpose, latest.reportId)}>
    <span className={`ac-result-label dg-${verdict.toLowerCase()}`}>{label}</span>
    <small>{scopeLabel(latest.scope)}</small>
    <small>{new Date(latest.createdAt).toLocaleString(intlLocale(), { hour: '2-digit', minute: '2-digit', day: '2-digit', month: '2-digit' })}{latest.stale ? ` · ${t('platform.diagnostics.stale')}` : ''}</small>
  </button>;
}
