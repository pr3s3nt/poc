import type { ReactNode } from 'react';
import { t } from '../../shared/i18n';

export function GuideSteps({ prefix, count }: { prefix: string; count: number }) {
  return <ol>{Array.from({ length: count }, (_, i) => <li key={i}>{t(`${prefix}.step${i + 1}`)}</li>)}</ol>;
}

export function GuideSection({ id, titleKey = `guide.${id}.title`, children }: { id: string; titleKey?: string; children: ReactNode }) {
  return <section className="guide-section" aria-labelledby={`guide-${id}`}>
    <h3 id={`guide-${id}`} tabIndex={-1}>{t(titleKey)}</h3>{children}
  </section>;
}
