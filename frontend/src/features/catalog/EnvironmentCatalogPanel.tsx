import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import type { EnvironmentKey } from '../../shared/types/application';
type Selection = { sourceId: string; tag: string; changes?: { id: string; change: string }[]; blockers?: string[] };
type View = { catalog: { desired?: Selection; applied?: Selection; revision: number; status: string }; environmentVersion: number; releases: Selection[] };
export function EnvironmentCatalogPanel({ applicationId, environment }: { applicationId: string; environment: EnvironmentKey; busy: boolean; onSaved(): void }) {
  const [view, setView] = useState<View>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  const endpoint = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}/catalog`;
  useEffect(() => {
    let active = true; setView(undefined); setLoading(true); setError('');
    api<View>(endpoint).then(result => { if (!result?.catalog) throw new Error(); if (active) setView(result); })
      .catch(cause => { if (active) setError(describeError(cause, t('catalog.failed'))); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [endpoint, attempt]);
  const title = (value?: Selection) => value ? `${value.sourceId} · ${value.tag}` : t('catalog.unselected');
  if (loading || (!error && !view?.catalog.desired && !view?.catalog.applied)) return null;
  return <section className="content-panel" aria-label={t('catalog.title')}><h2>{t('catalog.title')}</h2>
    {view ? <><p>{t('catalog.applied')}: <strong>{title(view.catalog.applied)}</strong></p><p>{t('catalog.desired')}: <strong>{title(view.catalog.desired)}</strong> · {t(`catalog.code.${view.catalog.status}`)}</p></> : null}
    {error ? <div className="form-error" role="alert">{error} <Button onClick={() => setAttempt(value => value + 1)}>{t('catalog.reload')}</Button></div> : null}
  </section>;
}
