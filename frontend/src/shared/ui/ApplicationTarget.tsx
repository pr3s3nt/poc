import type { Application, EnvironmentKey } from '../types/application';
import { t } from '../i18n';

export function ApplicationTarget({ environment }: { application: Application; environment: EnvironmentKey }) {
  return <p className="application-target" aria-label={t('shared.applicationTarget.deploymentTarget')}>{t('shared.applicationTarget.environmentTheTargetIsDetermined', { environment })}</p>;
}
