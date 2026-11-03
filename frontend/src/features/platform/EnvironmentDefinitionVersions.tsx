import { useEffect, useRef, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { navigate } from '../../app/routes';
import { t } from '../../shared/i18n';
import type { EnvironmentKey } from '../../shared/types/application';
type Version = { id: string; number: number; active: boolean; archived: boolean; changes: string[] };
type Row = { key: string; resourceType: string; desiredRevisionId: string; appliedRevisionId: string; versions: Version[]; inUse?: boolean; actualRevisionIds?: string[] };
type Data = { revision: number; environmentVersion: number; pending: boolean; appliedVerified: boolean; definitions: Row[] };
export function EnvironmentDefinitionVersions({ applicationId, environment, busy, onSaved }: { applicationId: string; environment: EnvironmentKey; busy: boolean; onSaved?(): void }) {
  const [open, setOpen] = useState(false);
  const [data, setData] = useState<Data>();
  const [choices, setChoices] = useState<Record<string, string>>({});
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [uncertain, setUncertain] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [notice, setNotice] = useState(false);
  const generation = useRef(0);
  const base = `/applications/${encodeURIComponent(applicationId)}/environments/${environment}/definition-versions`;
  useEffect(() => {
    if (!open) return;
    const n = ++generation.current;
    setData(undefined); setChoices({}); setError(''); setSaving(false); setUncertain(false);
    api<Data>(base).then(d => { if (n === generation.current) setData({ ...d, definitions: d.definitions ?? [] }); }).catch(e => { if (n === generation.current) setError(describeError(e, t('versions.failed'))); });
    const tracker = generation;
    return () => { tracker.current++; };
  }, [base, open, attempt]);
  async function save(row?: Row) {
    if (!data) return;
    const n = generation.current;
    setSaving(true); setError(''); setNotice(false);
    try {
      await api(`${base}${row ? '' : '/discard'}`, { method: row ? 'PUT' : 'POST', body: JSON.stringify({ ...(row ? { definitionKey: row.key, revisionId: choices[row.key] } : {}), expectedRevision: data.revision, expectedEnvironmentVersion: data.environmentVersion }) });
      if (n === generation.current) { setNotice(true); setAttempt(v => v + 1); onSaved?.(); }
    } catch (e) { if (n === generation.current) { setError(describeError(e, t('versions.failed'))); setUncertain(true); } }
    finally { if (n === generation.current) setSaving(false); }
  }
  const label = (r: Row, id: string) => { const v = r.versions.find(v => v.id === id); return v ? `r${v.number}` : t('versions.frozen'); };
  return <details className="content-panel" open={open} onToggle={e => setOpen(e.currentTarget.open)}>
    <summary><strong>{t('versions.environmentTitle')}</strong></summary>
    {open ? <>
      {error ? <div className="form-error" role="alert">{error} <Button disabled={saving} onClick={() => setAttempt(v => v + 1)}>{t('versions.reload')}</Button></div> : null}
      {!data && !error ? <p role="status">{t('versions.loading')}</p> : null}
      {notice ? <p className="form-success" role="status">{t('versions.saved')}</p> : null}
      {data ? <>
        <p className="feature-note">{t('versions.environmentNote')}</p>
        {!data.appliedVerified ? <p className="feature-note">{t('versions.unverifiedBaseline')}</p> : null}
        {data.definitions.length === 0 ? <p>{t('versions.empty')}</p> : data.definitions.map(row => {
          const selectedId = choices[row.key] ?? row.desiredRevisionId;
          const selected = row.versions.find(v => v.id === selectedId);
          const latest = row.versions.filter(v => !v.archived).at(-1);
          const changes = selectedId !== row.desiredRevisionId ? selected?.changes ?? [] : [];
          return <section className="catalog-fields" key={row.key}>
            <div className="section-header"><h3>{row.key}</h3><span>{row.resourceType}{latest && latest.id !== row.desiredRevisionId ? ` · ${t('versions.available')}` : ''}</span></div>
            <p>{t('versions.applied')}: {row.inUse === false ? t('versions.notUsed') : row.actualRevisionIds?.length ? row.actualRevisionIds.map(id => label(row, id)).join(', ') : t('versions.unknown')} · {t('versions.desired')}: {label(row, row.desiredRevisionId)}</p>
            {row.versions.length ? <div className="catalog-field-row"><label>{t('versions.choose', { key: row.key })}<select disabled={busy || saving || uncertain || row.versions.length === 0} value={selectedId} onChange={e => setChoices(c => ({ ...c, [row.key]: e.target.value }))}>
              {!row.versions.some(v => v.id === row.desiredRevisionId) ? <option value={row.desiredRevisionId}>{t('versions.frozen')}</option> : null}
              {row.versions.map(v => <option key={v.id} value={v.id} disabled={v.archived}>r{v.number} · {v.archived ? t('versions.archived') : v.active ? t('versions.active') : t('versions.proposed')}</option>)}
            </select></label>{selectedId !== row.desiredRevisionId ? <Button disabled={busy || saving || uncertain || !selected || selected.archived} onClick={() => void save(row)}>{t('versions.select')}</Button> : null}</div> : null}
            {changes.length ? <details open><summary>{t('versions.changes')}</summary><ul>{changes.map(c => <li key={c}><code>{c}</code></li>)}</ul></details> : null}
            {changes.length && ['postgres', 'mysql', 'redis', 'persistent-storage'].includes(row.resourceType) ? <p className="form-info">{t('versions.dataGuard')}</p> : null}
          </section>;
        })}
        <div className="form-actions"><Button disabled={busy || saving} onClick={() => navigate({ name: 'score-preview', applicationId, environment })}>{t('versions.preview')}</Button>{data.pending ? <Button disabled={busy || saving || uncertain} onClick={() => void save()}>{t('versions.discard')}</Button> : null}</div>
      </> : null}
    </> : null}
  </details>;
}
