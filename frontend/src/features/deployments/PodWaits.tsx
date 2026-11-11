import { formatDateTime, t } from '../../shared/i18n';

// UC-08 BR-11 / UC-06 CD-11 (issue #68): Pods the deployment waits on and the
// stuck Pods the platform replaced. Reason codes, images and names are shown
// as-is (technical identifiers); the backend sends only sanitized values.
export type PodWait = { object: string; pod: string; container?: string; code: string; image?: string; restarts?: number; stuckSeconds: number };
export type PodWaitGroup = { scope: 'readiness' | 'rollout'; key: string; waiting: PodWait[]; replaced?: { wait: PodWait; at: string }[] };

const imageReasons = new Set(['ImagePullBackOff', 'ErrImagePull', 'InvalidImageName']);

function detail(w: PodWait): string {
  const parts = [`${w.pod}: ${w.code}`];
  if (w.image && imageReasons.has(w.code)) parts.push(t('deployments.podWaits.image', { image: w.image }));
  else if (w.restarts) parts.push(t('deployments.podWaits.restarts', { count: w.restarts }));
  parts.push(t('deployments.podWaits.stuckFor', { seconds: w.stuckSeconds }));
  return parts.join(' · ');
}

export function PodWaits({ groups }: { groups?: PodWaitGroup[] }) {
  const waiting = groups?.flatMap(g => g.waiting) ?? [];
  const replaced = groups?.flatMap(g => g.replaced ?? []) ?? [];
  if (!waiting.length && !replaced.length) return null;
  return <div className="form-info" role="status" aria-label={t('deployments.podWaits.label')}>
    {waiting.length ? <><strong>{t('deployments.podWaits.waiting')}</strong><ul>{waiting.map(w => <li key={`w:${w.object}/${w.pod}`}>{detail(w)}</li>)}</ul></> : null}
    {replaced.length ? <><strong>{t('deployments.podWaits.replaced')}</strong><ul>{replaced.map(r => <li key={`r:${r.wait.object}/${r.wait.pod}/${r.at}`}>{t('deployments.podWaits.replacedItem', { pod: r.wait.pod, code: r.wait.code, seconds: r.wait.stuckSeconds, at: formatDateTime(new Date(r.at)) })}</li>)}</ul></> : null}
  </div>;
}
