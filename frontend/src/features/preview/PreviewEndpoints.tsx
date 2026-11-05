import { t } from '../../shared/i18n';
import type { PendingChange } from '../workloads/api';

// Prospective public addresses (EP-03): shown once, where Deploy is decided.
export function PreviewEndpoints({ changes }: { changes: PendingChange[] }) {
  const endpoints = changes.filter((change) => change.endpoint);
  if (endpoints.length === 0) return null;
  return <div className="form-info" role="status" aria-label={t('preview.endpoint')}>
    <strong>{t('preview.endpoint')}</strong>
    <ul>{endpoints.map((change) => <li key={change.workloadId}>{change.workloadId} · <code>{change.endpoint?.url}</code></li>)}</ul>
  </div>;
}
