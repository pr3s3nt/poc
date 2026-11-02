import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
type Component = { id: string; kind: string; path: string; sha256: string; binding?: string; snapshot?: Record<string, unknown> };
type Detail = { components: Component[]; templates: { id: string; version: string; kind: string }[] };
export function ReleaseDetails({ source, tag }: { source: string; tag: string }) {
 const [detail, setDetail] = useState<Detail>();
 const [error, setError] = useState('');
 const [attempt, setAttempt] = useState(0);
 useEffect(() => {
  let current = true; setDetail(undefined); setError('');
  api<{ release: Detail }>(`/platform/catalog/sources/${encodeURIComponent(source)}/releases/${encodeURIComponent(tag)}`)
   .then(result => { if (current) setDetail(result.release); })
   .catch(cause => { if (current) setError(describeError(cause, t('catalog.failed'))); });
  return () => { current = false; };
 }, [source, tag, attempt]);
 if (error) return <div className="form-error" role="alert">{error} <Button onClick={() => setAttempt(value => value + 1)}>{t('catalog.reload')}</Button></div>;
 if (!detail) return <p role="status">{t('catalog.loading')}</p>;
 return <section className="definition-review" aria-label={`${t('catalog.components')} ${tag}`}>
  <p className="feature-note">{t('catalog.gitOwned')}</p>
  {detail.components.length ? detail.components.map(component => <details key={component.id} className="catalog-entry catalog-entry-rich">
   <summary><strong>{component.id}</strong> · {component.kind}</summary>
   <p>{t('catalog.path')}: <code>{component.path}</code></p><p>SHA-256: <code>{component.sha256}</code></p>
   {component.binding ? <p>{t('catalog.binding')}: <code>{component.binding}</code></p> : null}
   {component.snapshot ? <><h3>{t('catalog.configuration')}</h3><pre>{JSON.stringify(component.snapshot, null, 2)}</pre></> : null}
  </details>) : <p>{t('catalog.noComponents')}</p>}
  {detail.templates.length ? <><h3>{t('catalog.templates')}</h3>{detail.templates.map(template => <details key={`${template.id}/${template.version}`}><summary>{template.id} · {template.version} · {template.kind}</summary><pre>{JSON.stringify(template, null, 2)}</pre></details>)}</> : null}
 </section>;
}
