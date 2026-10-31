import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';

type Source = { id: string; repository: string; branch: string; tagPattern: string; manifestPath: string; includePrereleases: boolean; accountBindings?: Record<string, string>; version: number; code: string; lastChecked: string };
type Release = { tag: string; commit: string; observedAt: string; usage?: { applicationKey:string; environmentKey:string; desired:boolean; applied:boolean }[] };
const endpoint = '/platform/catalog/sources';
const initial = { id: '', repository: '', branch: 'main', tagPattern: 'v*', manifestPath: 'catalog.yaml', includePrereleases: false, version: 0, code: '', lastChecked: '' };

export function CatalogPage() {
  const [sources, setSources] = useState<Source[]>([]);
  const [draft, setDraft] = useState<Source>(initial);
  const [bindings, setBindings] = useState('');
  const [releases, setReleases] = useState<Release[]>();
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState('');
  const [reloadRequired, setReloadRequired] = useState(false);
  const [notice, setNotice] = useState('');
  const alive = useRef(false);
  const busy = useRef(false);
  const generation = useRef(0);
  useEffect(() => { alive.current = true; void load(); return () => { alive.current = false; }; }, []);
  async function load() {
    const current = ++generation.current; setLoading(true);
    try { const result = await api<{ sources: Source[] }>(endpoint); if (alive.current && current === generation.current) { setSources(result.sources); setError(''); setReloadRequired(false); } }
    catch (cause) { if (alive.current && current === generation.current) setError(describeError(cause, t('catalog.failed'))); }
    finally { if (alive.current && current === generation.current) setLoading(false); }
  }
  function edit(source: Source) {
    generation.current++; setDraft(source); setBindings(Object.entries(source.accountBindings ?? {}).map(([slot, account]) => `${slot}=${account}`).join('\n')); setReleases(undefined); setNotice(''); setError('');
  }
  async function act(operation: () => Promise<void>) {
    if (busy.current) return; busy.current = true; setPending(true); setError(''); setNotice('');
    try { await operation(); }
    catch (cause) { if (alive.current) { setError(describeError(cause, t('catalog.failed'))); setReloadRequired(true); } }
    finally { busy.current = false; if (alive.current) setPending(false); }
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    void act(async () => {
      const accountBindings: Record<string, string> = {};
      for (const line of bindings.split('\n').filter(value => value.trim())) {
        const [slot, account, extra] = line.split('='); if (!slot?.trim() || !account?.trim() || extra !== undefined || Object.hasOwn(accountBindings, slot.trim())) throw new Error();
        accountBindings[slot.trim()] = account.trim();
      }
      const { id, repository, branch, tagPattern, manifestPath, includePrereleases, version } = draft;
      const result = await api<{ source: Source }>(`${endpoint}/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify({ repository, branch, tagPattern, manifestPath, includePrereleases, accountBindings, expectedVersion: version }) });
      if (!alive.current) return; setDraft(result.source); setNotice(t('catalog.saved')); await load();
    });
  }
  async function refresh(source: Source) {
    await act(async () => {
      const result = await api<{ source: Source }>(`${endpoint}/${encodeURIComponent(source.id)}/refresh`, { method: 'POST' });
      if (!alive.current) return; if (draft.id === source.id) setDraft(result.source); setNotice(t(`catalog.code.${result.source.code}`)); await load();
    });
  }
  async function showReleases(source: Source) {
    edit(source); const current = generation.current;
    await act(async () => { const result = await api<{ releases: Release[] }>(`${endpoint}/${encodeURIComponent(source.id)}/releases`); if (alive.current && current === generation.current) setReleases(result.releases); });
  }
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('platform.git.platform')}</p><h1>{t('catalog.title')}</h1></div><Button disabled={pending} onClick={() => edit(initial)}>{t('catalog.add')}</Button></header>
    {loading ? <p role="status">{t('catalog.loading')}</p> : <section className="content-panel">
      {sources.length === 0 ? <p>{t('catalog.empty')}</p> : sources.map(source => <div className="section-header" key={source.id}><div><strong>{source.id}</strong><p>{source.branch} · {t(`catalog.code.${source.code || 'NOT_CHECKED'}`)}{source.lastChecked && !source.lastChecked.startsWith('0001') ? ` · ${new Date(source.lastChecked).toLocaleString()}` : ''}</p></div><div className="row-actions"><Button disabled={pending} onClick={() => void showReleases(source)}>{t('catalog.versions')}</Button><Button disabled={pending} onClick={() => void refresh(source)}>{t('catalog.refresh')}</Button></div></div>)}
    </section>}
    {error ? <div className="form-error" role="alert">{error} <Button disabled={pending} onClick={() => { edit(initial); void load(); }}>{t('catalog.reload')}</Button></div> : null}
    {notice ? <p className="form-info" role="status">{notice}</p> : null}
    {releases ? <section className="content-panel"><h2>{t('catalog.versions')}</h2>{releases.length ? releases.map(release => <div key={release.tag}><p><strong>{release.tag}</strong> <code>{release.commit}</code></p>{release.usage?.length ? <ul>{release.usage.map(usage => <li key={`${usage.applicationKey}/${usage.environmentKey}`}>{usage.applicationKey} · {usage.environmentKey} · {[usage.applied ? t('catalog.applied') : '', usage.desired ? t('catalog.desired') : ''].filter(Boolean).join(' / ')}</li>)}</ul> : null}</div>) : <p>{t('catalog.noReleases')}</p>}</section> : null}
    <form className="create-form" onSubmit={submit}><h2>{t(draft.version ? 'catalog.edit' : 'catalog.add')}</h2>
      <fieldset className="form-fieldset editor-grid" disabled={pending || loading || reloadRequired}>
        <div className="field"><label htmlFor="catalog-id">{t('catalog.id')}</label><input id="catalog-id" required pattern="[a-z0-9]+(-[a-z0-9]+)*" maxLength={63} disabled={draft.version > 0} value={draft.id} onChange={event => setDraft({ ...draft, id: event.target.value })} /></div>
        <div className="field"><label htmlFor="catalog-repository">{t('catalog.repository')}</label><input id="catalog-repository" required type="url" maxLength={2048} value={draft.repository} onChange={event => setDraft({ ...draft, repository: event.target.value })} /></div>
        <div className="field"><label htmlFor="catalog-branch">{t('catalog.branch')}</label><input id="catalog-branch" required maxLength={200} value={draft.branch} onChange={event => setDraft({ ...draft, branch: event.target.value })} /></div>
        <details><summary>{t('platform.git.advanced')}</summary><div className="editor-grid">
          <div className="field"><label htmlFor="catalog-tags">{t('catalog.tags')}</label><input id="catalog-tags" required maxLength={100} value={draft.tagPattern} onChange={event => setDraft({ ...draft, tagPattern: event.target.value })} /></div>
          <div className="field"><label htmlFor="catalog-manifest">{t('catalog.manifest')}</label><input id="catalog-manifest" required value={draft.manifestPath} onChange={event => setDraft({ ...draft, manifestPath: event.target.value })} /></div>
          <label><input type="checkbox" checked={draft.includePrereleases} onChange={event => setDraft({ ...draft, includePrereleases: event.target.checked })} /> {t('catalog.prereleases')}</label>
          <div className="field"><label htmlFor="catalog-bindings">{t('catalog.bindings')}</label><textarea id="catalog-bindings" rows={3} value={bindings} placeholder="internal=cluster-account" onChange={event => setBindings(event.target.value)} /><small>{t('catalog.bindingsHelp')}</small></div>
        </div></details>
      </fieldset><div className="form-actions"><Button type="submit" tone="primary" disabled={pending || loading || reloadRequired}>{t(pending ? 'platform.git.saving' : 'platform.git.save')}</Button></div>
    </form>
  </section>;
}
