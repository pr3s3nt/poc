import { useState, type FormEvent } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';
import { parseCatalogDocument } from '../platform/catalogDocument';

type Source = { url: string; ref: string; path: string };
type Found = { file: string; kind: string; content: Record<string, unknown> | null; problem?: string };
type Discovery = { url: string; revision: string; path: string; documents: Found[]; skipped: number };
type Draft = Found & { selected: boolean; text: string; template: Source };
type Document = { file: string; type?: Record<string, unknown>; definition?: Record<string, unknown>; template?: Source };
type Review = Document & { status: string; fields?: string[] };
const placeholders = (value: unknown): string[] => {
  if (typeof value === 'string') return value.match(/<[^<>\n]+>/g) ?? [];
  if (Array.isArray(value)) return value.flatMap(placeholders);
  if (value && typeof value === 'object') return Object.values(value).flatMap(placeholders);
  return [];
};
function substitute(value: unknown, values: Record<string, string>): unknown {
  if (typeof value === 'string') return value.replace(/<[^<>\n]+>/g, key => values[key] || key);
  if (Array.isArray(value)) return value.map(item => substitute(item, values));
  if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, substitute(item, values)]));
  return value;
}

export function GitInit({ onImported }: { onImported(): void }) {
  const [source, setSource] = useState<Source>({ url: '', ref: '', path: '' });
  const [discovery, setDiscovery] = useState<Discovery>();
  const [drafts, setDrafts] = useState<Draft[]>([]);
  const [values, setValues] = useState<Record<string, string>>({});
  const [review, setReview] = useState<Review[]>();
  const [results, setResults] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  function invalidate() { setReview(undefined); setResults({}); }
  function change(index: number, update: Partial<Draft>) { invalidate(); setDrafts(current => current.map((item, i) => i === index ? { ...item, ...update } : item)); }
  async function discover(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(''); invalidate(); setDiscovery(undefined); setDrafts([]); setValues({});
    try {
      const found = await api<Discovery>('/platform/setup/discover', { method: 'POST', body: JSON.stringify(source) });
      setDiscovery(found);
      setDrafts(found.documents.map(item => ({ ...item, selected: item.kind !== 'invalid', text: JSON.stringify(item.content, null, 2), template: { url: found.url, ref: found.revision, path: '' } })));
    } catch (err) { setError(describeError(err, t('setup.failed'))); }
    finally { setBusy(false); }
  }
  const tokens = [...new Set(drafts.filter(d => d.selected).flatMap(d => {
    try {
      const content = parseCatalogDocument(d.text);
      // Template identity is prepared separately from its Git source.
      if (content.driverType === 'template' && d.template.path) {
        const inputs = content.driverInputs as { values?: { variables?: Record<string, unknown> } } | undefined;
        if (inputs?.values?.variables) delete inputs.values.variables.template_source;
      }
      return placeholders(content);
    } catch { return []; }
  }))];
  async function prepare() {
    setBusy(true); setError(''); invalidate();
    try {
      const documents: Document[] = drafts.filter(d => d.selected).map(d => {
        const content = substitute(parseCatalogDocument(d.text), values) as Record<string, unknown>;
        if (content.driverType === 'template' && d.template.path) {
          const inputs = content.driverInputs as { values?: { variables?: Record<string, unknown> } } | undefined;
          if (inputs?.values?.variables) inputs.values.variables.template_source = '';
        }
        if (placeholders(content).length) throw new Error('placeholders');
        return { file: d.file, ...(d.kind === 'resource-types' ? { type: content } : { definition: content }), ...(content.driverType === 'template' && d.template.path ? { template: d.template } : {}) };
      });
      const response = await api<{ documents: Review[] }>('/platform/setup/preview', { method: 'POST', body: JSON.stringify({ documents }) });
      setReview(response.documents);
    } catch (err) { setError(describeError(err, t('setup.reviewError'))); }
    finally { setBusy(false); }
  }
  async function importDocuments() {
    if (!review) return;
    setBusy(true); setError(''); setResults({});
    try {
      for (const doc of [...review].sort((a, b) => Number(Boolean(b.type)) - Number(Boolean(a.type)))) {
        const kind = doc.type ? 'resource-types' : 'resource-definitions';
        try {
          const result = await api<{ status: string; fields?: string[] }>(`/${kind}/import`, { method: 'POST', body: JSON.stringify(doc.type ?? doc.definition) });
          setResults(current => ({ ...current, [doc.file]: result.status }));
          if (result.status === 'CONFLICT') { setError(t('setup.conflict')); break; }
        } catch (err) { setResults(current => ({ ...current, [doc.file]: 'FAILED' })); setError(describeError(err, t('setup.failed'))); break; }
      }
    } finally { setBusy(false); onImported(); }
  }
  return <section className="content-panel editor-grid"><h2>{t('setup.gitInit')}</h2>
    <form onSubmit={event => void discover(event)} className="setup-fields">
      {(['url', 'ref', 'path'] as const).map(field => <label key={field}>{t(`setup.${field}`)}<input type={field === 'url' ? 'url' : 'text'} required={field !== 'path'} disabled={busy} value={source[field]} onChange={event => { setSource({ ...source, [field]: event.target.value }); invalidate(); setDiscovery(undefined); setDrafts([]); }} /></label>)}
      <Button type="submit" disabled={busy}>{t(busy ? 'setup.working' : 'setup.discover')}</Button>
    </form>
    {error ? <p role="alert" className="form-error">{error}</p> : null}
    {discovery ? <>
      <p>{t('setup.commit')}: <code>{discovery.revision}</code></p>
      {!drafts.length ? <p role="status">{t('setup.noDocuments')}</p> : null}
      {drafts.map((d, index) => <div className="setup-document" key={d.file}>
        <label><input type="checkbox" checked={d.selected} disabled={busy || d.kind === 'invalid'} onChange={event => change(index, { selected: event.target.checked })} /> {d.file} {d.kind === 'invalid' ? t('setup.INVALID') : ''}</label>
        {d.selected ? <details><summary>{t('setup.document')}</summary><label>{t('setup.content')}<textarea rows={12} value={d.text} disabled={busy} onChange={event => change(index, { text: event.target.value })} /></label></details> : null}
        {d.selected && d.content?.driverType === 'template' ? <fieldset disabled={busy}><legend>{t('setup.template')}</legend>{(['url', 'ref', 'path'] as const).map(field => <label key={field}>{t(`setup.template${field}`)}<input value={d.template[field]} onChange={event => change(index, { template: { ...d.template, [field]: event.target.value } })} /></label>)}</fieldset> : null}
      </div>)}
      {tokens.length ? <fieldset disabled={busy}><legend>{t('setup.environmentValues')}</legend>{tokens.map(token => <label key={token}>{token}<input value={values[token] ?? ''} onChange={event => { setValues({ ...values, [token]: event.target.value }); invalidate(); }} /></label>)}</fieldset> : null}
      <Button disabled={busy || !drafts.some(d => d.selected)} onClick={() => void prepare()}>{t(busy ? 'setup.working' : 'setup.preview')}</Button>
    </> : null}
    {review ? <><h3>{t('setup.review')}</h3><table><thead><tr><th>{t('setup.document')}</th><th>{t('setup.status')}</th><th>{t('setup.fields')}</th></tr></thead><tbody>{review.map(doc => <tr key={doc.file}><td>{doc.file}</td><td>{t(`setup.${results[doc.file] ?? doc.status}`)}</td><td>{doc.fields?.join(', ')}</td></tr>)}</tbody></table>
      <details><summary>{t('setup.prepared')}</summary><pre>{JSON.stringify(review.map(({ file, type, definition }) => ({ file, type, definition })), null, 2)}</pre></details>
      <Button tone="primary" disabled={busy || review.some(d => d.status === 'INVALID' || d.status === 'CONFLICT')} onClick={() => void importDocuments()}>{t(busy ? 'setup.working' : 'setup.confirmImport')}</Button>
      <p>{t('setup.retained')}</p>
    </> : null}
  </section>;
}
