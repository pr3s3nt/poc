import type { EnvironmentKey } from '../../shared/types/application';
import { t } from '../../shared/i18n';

// Vietnamese Console labels (glossary VI-01). Enum values and technical IDs stay
// as the API sends them; only the label shown next to them is translated.
const statusLabels: Record<string, string> = {
  READY: t('environment.labels.ready'), PENDING: t('environment.labels.pending'), FAILED: t('environment.labels.failed'), SUCCEEDED: t('environment.labels.succeeded'),
  UNCONFIGURED: t('environment.labels.notConfigured'), RUNNING: t('environment.labels.running'), INTERRUPTED: t('environment.labels.interrupted'), ACTIVE: t('environment.labels.active'),
};

export const statusLabel = (status: string): string => statusLabels[status] ? `${statusLabels[status]} (${status})` : status;
export const environmentTitle = (environment: EnvironmentKey): string => environment === 'staging' ? 'Staging' : 'Production';
