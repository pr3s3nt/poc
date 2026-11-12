import { useRef, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { DocumentProblem, documentLimit, parseCatalogDocument, type DocumentKind } from './catalogDocument';

export function CatalogDocumentImport({ kind, onRegistered, onBusyChange }: { kind: DocumentKind; onRegistered(): Promise<unknown>; onBusyChange?(busy: boolean): void }) {
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [status, setStatus] = useState('');
  const readGeneration = useRef(0);
  function setWorking(value: boolean) { setBusy(value); onBusyChange?.(value); }
  function fail(reason: unknown) {
    setError(reason instanceof DocumentProblem ? t(`catalogDocument.${reason.code}`) : describeError(reason, t('catalogDocument.failed')));
  }
  async function upload(file?: File) {
    if (!file) return;
    const generation = ++readGeneration.current;
    setError(''); setStatus(''); setWorking(true);
    try {
      if (!/\.(yaml|yml|json)$/i.test(file.name)) throw new DocumentProblem('extension');
      if (file.size > documentLimit) throw new DocumentProblem('size');
      const content = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer());
      if (generation === readGeneration.current) setText(content);
    } catch (reason) { if (generation === readGeneration.current) fail(reason instanceof TypeError ? new DocumentProblem('utf8') : reason); }
    finally { if (generation === readGeneration.current) setWorking(false); }
  }
  async function submit() {
    if (busy) return;
    setError(''); setStatus(''); setWorking(true);
    try {
      const doc = parseCatalogDocument(text);
      const result = await api<{ status: string; fields?: string[] }>(`/${kind}/import`, { method: 'POST', body: JSON.stringify(doc) });
      setStatus(result.status + (result.fields?.length ? `: ${result.fields.join(', ')}` : ''));
      if (result.status === 'CREATED') await onRegistered();
    } catch (reason) { fail(reason); }
    finally { setWorking(false); }
  }
  return <section className="content-panel" aria-label={t('catalogDocument.title')}>
    <fieldset className="editor-grid form-fieldset" disabled={busy} aria-busy={busy}>
      <label>{t('catalogDocument.file')}<input aria-label={t('catalogDocument.file')} type="file" accept=".yaml,.yml,.json" onChange={event => { void upload(event.target.files?.[0]); event.target.value = ''; }} /></label>
      <label>{t('catalogDocument.content')}<textarea aria-label={t('catalogDocument.content')} rows={14} spellCheck={false} value={text} onChange={event => { setText(event.target.value); setError(''); setStatus(''); }} /></label>
      <details><summary>{t('catalogDocument.help')}</summary><p>{t('catalogDocument.format')}</p></details>
      {error ? <p role="alert" className="form-error">{error}</p> : null}
      {status ? <p role="status" className="form-success">{status}</p> : null}
      <div className="form-actions"><Button type="button" tone="primary" disabled={busy || !text.trim()} onClick={() => void submit()}>{t('catalogDocument.register')}</Button></div>
    </fieldset>
  </section>;
}
