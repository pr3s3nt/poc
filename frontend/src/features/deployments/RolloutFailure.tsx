import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
import { t } from '../../shared/i18n';

// UC-06 CD-10: a delivered workload whose rollout failed. The backend sends a
// code for its fixed safe summary; the Console localizes it and links to the
// workload logs. Unknown codes return null so callers keep their generic text.
const messages: Record<string, string> = {
  ROLLOUT_DEGRADED: 'deployments.rolloutFailure.degraded',
  ROLLOUT_NOT_READY: 'deployments.rolloutFailure.notReady',
};

export function isRolloutFailure(code?: string): boolean {
  return code !== undefined && code in messages;
}

export function RolloutFailure({ code, applicationId, environment, workloadId }: { code?: string; applicationId: string; environment: EnvironmentKey; workloadId: string }) {
  if (!code || !isRolloutFailure(code)) return null;
  return <>{t(messages[code]!)}{' '}<a href="#" onClick={(event) => { event.preventDefault(); navigate({ name: 'observability', applicationId, environment, tab: 'logs', query: { workload: workloadId } }); }}>{t('deployments.rolloutFailure.viewLogs')}</a></>;
}
