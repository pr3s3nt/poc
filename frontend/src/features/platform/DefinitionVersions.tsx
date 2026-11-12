import { DefinitionDocumentDownload } from './DefinitionDocumentDownload';
import { useEffect, useRef, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

type Revision = { id: string; number: number; definition: Record<string, unknown>; archived: boolean; createdAt: string };
type History = { generation: number; activeId: string; versions: Revision[] };
type Usage = { applicationKey: string; applicationName: string; environment: string; desiredRevisionId: string; appliedRevisionId: string; appliedVerified: boolean; pending: boolean; inUse?: boolean; actualRevisionIds?: string[] };
type Data = { history: History; usage: Usage[] };
function changedFields(a: Record<string, unknown>, b: Record<string, unknown>) {
  return [...new Set([...Object.keys(a), ...Object.keys(b)])].filter(k => !['version', 'revisionId', 'sourceFingerprint'].includes(k) && JSON.stringify(a[k]) !== JSON.stringify(b[k]));
}

export function DefinitionVersions({ definitionKey, onClose }: { definitionKey: string; onClose(): void }) {
  const [data, setData] = useState<Data>();
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [compare, setCompare] = useState('');
  const generation = useRef(0);
  const base = `/resource-definitions/${encodeURIComponent(definitionKey)}/versions`;
  useEffect(() => {
    const n = ++generation.current;
    setData(undefined); setError(''); setBusy(false); setUncertain(false); setCompare('');
    api<Data>(base).then(d => { if (n === generation.current) setData(d); }).catch(e => { if (n === generation.current) setError(describeError(e, t('versions.failed'))); });
    const tracker = generation;
    return () => { tracker.current++; };
  }, [base, attempt]);
  async function act(id: string, action: 'promote' | 'archive') {
    if (!data) return;
    const n = generation.current;
    setBusy(true); setError('');
    try {
      await api(`${base}/${encodeURIComponent(id)}/${action}`, { method: 'POST', body: JSON.stringify({ expectedGeneration: data.history.generation }) });
      if (n === generation.current) setAttempt(v => v + 1);
    } catch (e) { if (n === generation.current) { setError(describeError(e, t('versions.failed'))); setUncertain(true); } }
    finally { if (n === generation.current) setBusy(false); }
  }
  const revisions = data?.history.versions ?? [];
  const active = revisions.find(v => v.id === data?.history.activeId);
  const chosen = revisions.find(v => v.id === compare);
  const label = (id: string) => { if (!id) return t('versions.frozen'); const v = revisions.find(v => v.id === id); return v ? `r${v.number}` : t('versions.unknown'); };
  return <section className="content-panel" aria-label={t('versions.title')}>
    <div className="section-header"><h2>{t('versions.title')} · {definitionKey}</h2><Button disabled={busy} onClick={onClose}>{t('versions.close')}</Button></div>
    {error ? <div className="form-error" role="alert">{error} <Button disabled={busy} onClick={() => setAttempt(v => v + 1)}>{t('versions.reload')}</Button></div> : null}
    {!data && !error ? <p role="status">{t('versions.loading')}</p> : null}
    {data ? <>
      <p className="feature-note">{t('versions.policyNote')}</p>
      {revisions.length === 0 ? <p>{t('versions.empty')}</p> : <div className="catalog-list">{revisions.slice().reverse().map(v => <div className="catalog-entry" key={v.id}>
        <div><strong>r{v.number}</strong> · {v.archived ? t('versions.archived') : v.id === data.history.activeId ? t('versions.active') : t('versions.proposed')}</div>
        <div className="form-actions">
          <DefinitionDocumentDownload definitionKey={definitionKey} revisionId={v.id} label={`r${v.number}`} disabled={busy} /><Button disabled={busy} onClick={() => setCompare(v.id)}>{t('versions.compare')}</Button>
          <Button disabled={busy || uncertain || v.archived || v.id === data.history.activeId} onClick={() => void act(v.id, 'promote')}>{t('versions.promote')}</Button>
          <Button disabled={busy || uncertain || v.archived || v.id === data.history.activeId} onClick={() => void act(v.id, 'archive')}>{t('versions.archive')}</Button>
        </div>
      </div>)}</div>}
      {chosen ? <section className="definition-review"><h3>{t('versions.compare')} · {active ? `r${active.number}` : '—'} → r{chosen.number}</h3><p>{active ? changedFields(active.definition, chosen.definition).join(', ') || t('versions.same') : t('versions.unknown')}</p><details><summary>{t('versions.configuration')}</summary><div className="field-grid"><pre>{JSON.stringify(active?.definition ?? {}, null, 2)}</pre><pre>{JSON.stringify(chosen.definition, null, 2)}</pre></div></details></section> : null}
      <h3>{t('versions.usage')}</h3>
      {data.usage.length === 0 ? <p>{t('versions.noUsage')}</p> : <div className="catalog-list">{data.usage.map(u => <div className="catalog-entry" key={`${u.applicationKey}/${u.environment}`}><strong>{u.applicationName} · {u.environment}</strong><span>{t('versions.applied')}: {u.inUse === false ? t('versions.notUsed') : u.actualRevisionIds?.length ? u.actualRevisionIds.map(label).join(', ') : t('versions.unknown')} · {t('versions.desired')}: {label(u.desiredRevisionId)}{u.pending ? ` · ${t('versions.pending')}` : ''}</span></div>)}</div>}
    </> : null}
  </section>;
}
