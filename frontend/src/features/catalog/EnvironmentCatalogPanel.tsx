import { useEffect, useRef, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { navigate } from '../../app/routes';
import type { EnvironmentKey } from '../../shared/types/application';
type Selection = { sourceId: string; tag: string; changes?: { id: string; change: string }[]; blockers?: string[] };
type View = { catalog: { desired?: Selection; applied?: Selection; revision: number; status: string }; environmentVersion: number; releases: Selection[] };
const key = (value?: Selection) => value ? JSON.stringify([value.sourceId, value.tag]) : '';
export function EnvironmentCatalogPanel({ applicationId, environment, busy, onSaved }: { applicationId: string; environment: EnvironmentKey; busy: boolean; onSaved(): void }) {
  const [view, setView] = useState<View>();
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState(false);
  const [reloadRequired, setReloadRequired] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const alive = useRef(false);
  const posting = useRef(false);
  const generation = useRef(0);
  const endpoint = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}/catalog`;
  useEffect(() => {
    let current = true; alive.current = true; generation.current++; setLoading(true);
    api<View>(endpoint).then(result => { if (!result?.catalog || !Array.isArray(result.releases)) throw new Error(); if (current) { setView(result); setDraft(key(result.catalog.desired)); setLoading(false); setError(''); setReloadRequired(false); } }).catch(cause => { if (current) { setError(describeError(cause, t('catalog.failed'))); setLoading(false); } });
    return () => { current = false; alive.current = false; };
  }, [endpoint, attempt]);
  async function save() {
    const selection = view?.releases.find(value => key(value) === draft);
    if (!view || !selection || posting.current || busy || reloadRequired) return;
    const started=generation.current;
    posting.current = true; setSaving(true); setError(''); setNotice(false);
    try {
      await api(endpoint, { method: 'PUT', body: JSON.stringify({ sourceId:selection.sourceId,tag:selection.tag, expectedEnvironmentVersion: view.environmentVersion, expectedRevision: view.catalog.revision }) });
      if (alive.current && started===generation.current) { setNotice(true); setAttempt(value => value + 1); onSaved(); }
    } catch (cause) { if (alive.current && started===generation.current) { setError(describeError(cause, t('catalog.failed'))); setReloadRequired(true); } }
    finally { posting.current = false; if (alive.current && started===generation.current) setSaving(false); }
  }
  const selected = view?.releases.find(value => key(value) === draft);
  const title = (value?: Selection) => value ? `${value.sourceId} · ${value.tag}` : t('catalog.legacy');
  return <section className="content-panel" aria-label={t('catalog.title')}><h2>{t('catalog.title')}</h2>
    {loading ? <p role="status">{t('catalog.loading')}</p> : view ? <>
      <p>{t('catalog.applied')}: <strong>{title(view.catalog.applied)}</strong></p>
      <p>{t('catalog.desired')}: <strong>{title(view.catalog.desired)}</strong> · {t(`catalog.code.${view.catalog.status}`)}</p>
      <div className="inline-editor editor-grid"><label htmlFor={`catalog-selection-${environment}`}>{t('catalog.version')}</label><select id={`catalog-selection-${environment}`} value={draft} disabled={busy || saving || reloadRequired} onChange={event => { setDraft(event.target.value); setNotice(false); }}><option value="">{t('catalog.choose')}</option>{view.releases.map(value => <option key={key(value)} value={key(value)} disabled={Boolean(value.blockers?.length)}>{title(value)}</option>)}</select></div>
      {selected?.changes?.length ? <details><summary>{t('catalog.differences')}</summary>{selected.changes.map(change => <p key={change.id}><strong>{change.id}</strong> · {t(`catalog.change.${change.change}`)}</p>)}</details> : null}
      {selected?.blockers?.map(blocker => <p className="form-error" key={blocker}>{t(`catalog.blocker.${blocker}`)}</p>)}
      {view.releases.length === 0 ? <p>{t('catalog.noReleases')}</p> : null}
      <div className="form-actions"><Button disabled={busy || saving || reloadRequired || !draft || Boolean(selected?.blockers?.length) || draft === key(view.catalog.desired)} onClick={() => void save()}>{t('catalog.saveDesired')}</Button></div>
    </> : null}
    {error ? <div className="form-error" role="alert">{error} <Button disabled={saving} onClick={() => { setLoading(true); setAttempt(value => value + 1); }}>{t('catalog.reload')}</Button></div> : null}
    {notice ? <p className="form-info" role="status">{t('catalog.desiredSaved')} <Button onClick={() => navigate({ name: 'application', applicationId, environment })}>{t('catalog.preview')}</Button></p> : null}
  </section>;
}
