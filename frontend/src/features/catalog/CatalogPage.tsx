import { useEffect, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { ReleaseDetails } from './ReleaseDetails';
import { Button } from '../../shared/ui/Button';

type Source = { id: string; repository: string; branch: string; tagPattern: string; manifestPath: string; includePrereleases: boolean; accountBindings?: Record<string, string>; version: number; code: string; lastChecked: string };
type Release = { tag: string; commit: string; observedAt: string; usage?: { applicationKey:string; environmentKey:string; desired:boolean; applied:boolean }[] };
const endpoint = '/platform/catalog/sources';

export function CatalogPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [selected, setSelected] = useState('');
  const [openedTag, setOpenedTag] = useState('');
  const [releases, setReleases] = useState<Release[]>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    let active = true;
    setLoading(true); setError('');
    api<{ sources: Source[] }>(endpoint).then(result => { if (active) setSources(result.sources); })
      .catch(cause => { if (active) setError(describeError(cause, t('catalog.failed'))); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [attempt]);
  useEffect(() => {
    let active = true;
    setReleases(undefined); setOpenedTag('');
    if (selected) api<{ releases: Release[] }>(`${endpoint}/${encodeURIComponent(selected)}/releases`)
      .then(result => { if (active) setReleases(result.releases); })
      .catch(cause => { if (active) setError(describeError(cause, t('catalog.failed'))); });
    return () => { active = false; };
  }, [selected, attempt]);
  return <section className="page">
    <header className="page-header"><h1>{t('catalog.title')}</h1></header>
    {loading ? <p role="status">{t('catalog.loading')}</p> : <section className="content-panel">
      {sources.length === 0 ? <p>{t('catalog.empty')}</p> : sources.map(source => <div className="section-header" key={source.id}><div><strong>{source.id}</strong><p>{source.branch} · {t(`catalog.code.${source.code || 'NOT_CHECKED'}`)}</p></div><Button onClick={() => setSelected(source.id)}>{t('catalog.versions')}</Button></div>)}
    </section>}
    {error ? <div className="form-error" role="alert">{error} <Button onClick={() => setAttempt(value => value + 1)}>{t('catalog.reload')}</Button></div> : null}
    {selected ? <section className="content-panel"><h2>{t('catalog.versions')}</h2>{!releases ? <p role="status">{t('catalog.loading')}</p> : releases.length ? releases.map(release => <div key={release.tag}><p><strong>{release.tag}</strong> <code>{release.commit}</code> <Button aria-expanded={openedTag === release.tag} onClick={() => setOpenedTag(openedTag === release.tag ? '' : release.tag)}>{t('catalog.details')}</Button></p>{openedTag === release.tag ? <ReleaseDetails key={`${selected}/${release.tag}`} source={selected} tag={release.tag} /> : null}{release.usage?.length ? <ul>{release.usage.map(usage => <li key={`${usage.applicationKey}/${usage.environmentKey}`}>{usage.applicationKey} · {usage.environmentKey} · {[usage.applied ? t('catalog.applied') : '', usage.desired ? t('catalog.desired') : ''].filter(Boolean).join(' / ')}</li>)}</ul> : null}</div>) : <p>{t('catalog.noReleases')}</p>}</section> : null}
  </section>;
}
