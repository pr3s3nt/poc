import { useEffect, useRef, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { DocumentProblem, documentLimit, parseCatalogDocument, type DocumentKind } from './catalogDocument';
import { SavedSourceSummary } from './SavedSourceSummary';

type Prepared = { definition: Record<string, unknown>; source?: { url: string; path: string; revision: string } };

export function CatalogDocumentImport({ kind, onRegistered, onBusyChange }: { kind: DocumentKind; onRegistered(): Promise<unknown>; onBusyChange?(busy: boolean): void }) {
  const [text, setText] = useState('');
  const [working, setWorking] = useState<'upload' | 'check' | 'register'>();
  const [prepared, setPrepared] = useState<Prepared>();
  const [error, setError] = useState('');
  const [status, setStatus] = useState('');
  const readGeneration = useRef(0);
  const pending = useRef(false);
  const scope = useRef(0);
  const busyCallback = useRef(onBusyChange);
  busyCallback.current = onBusyChange;
  useEffect(() => {
    scope.current++;
    setPrepared(undefined); setError(''); setStatus(''); setWorking(undefined); pending.current = false;
    busyCallback.current?.(false);
    const currentScope = scope;
    const currentRead = readGeneration;
    return () => { currentScope.current++; currentRead.current++; };
  }, [kind]);
  const busy = working !== undefined;
  const locked = working === 'upload' || working === 'register';
  function work(value: typeof working) { pending.current = value !== undefined; setWorking(value); onBusyChange?.(value !== undefined); }
  function invalidate() { readGeneration.current++; setPrepared(undefined); setError(''); setStatus(''); }
  function fail(reason: unknown, fallback = 'catalogDocument.failed') {
    setError(reason instanceof DocumentProblem ? t(`catalogDocument.${reason.code}`) : describeError(reason, t(fallback)));
  }
  async function upload(file?: File) {
    if (!file) return;
    invalidate();
    const generation = ++readGeneration.current;
    const currentScope = scope.current;
    work('upload');
    try {
      if (!/\.(yaml|yml|json)$/i.test(file.name)) throw new DocumentProblem('extension');
      if (file.size > documentLimit) throw new DocumentProblem('size');
      const content = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer());
      if (currentScope === scope.current && generation === readGeneration.current) setText(content);
    } catch (reason) { if (currentScope === scope.current && generation === readGeneration.current) fail(reason instanceof TypeError ? new DocumentProblem('utf8') : reason); }
    finally { if (currentScope === scope.current && generation === readGeneration.current) work(undefined); }
  }
  async function check() {
    if (pending.current) return;
    invalidate();
    const generation = readGeneration.current;
    const currentScope = scope.current;
    work('check');
    try {
      const doc = parseCatalogDocument(text);
      const result = await api<Prepared>('/resource-definitions/import/prepare', { method: 'POST', body: JSON.stringify(doc) });
      if (currentScope === scope.current && generation === readGeneration.current) {
        if (!result.definition || typeof result.definition !== 'object' || Array.isArray(result.definition)) throw new DocumentProblem('checkFailed');
        setPrepared(result); setStatus(t('catalogDocument.checked'));
      }
    } catch (reason) { if (currentScope === scope.current && generation === readGeneration.current) fail(reason, 'catalogDocument.checkFailed'); }
    finally { if (currentScope === scope.current && generation === readGeneration.current) work(undefined); }
  }
  async function submit() {
    if (pending.current || (kind === 'resource-definitions' && !prepared)) return;
    const currentScope = scope.current;
    setError(''); setStatus(''); work('register');
    try {
      const doc = kind === 'resource-definitions' ? prepared!.definition : parseCatalogDocument(text);
      const result = await api<{ status: string; fields?: string[] }>(`/${kind}/import`, { method: 'POST', body: JSON.stringify(doc) });
      if (currentScope !== scope.current) return;
      setStatus(result.status + (result.fields?.length ? `: ${result.fields.join(', ')}` : ''));
      if (result.status === 'CREATED') await onRegistered();
    } catch (reason) { if (currentScope === scope.current) fail(reason); }
    finally { if (currentScope === scope.current) work(undefined); }
  }
  return <section className="content-panel" aria-label={t('catalogDocument.title')}>
    <fieldset className="editor-grid form-fieldset" disabled={locked} aria-busy={busy}>
      <label>{t('catalogDocument.file')}<input aria-label={t('catalogDocument.file')} type="file" accept=".yaml,.yml,.json" onChange={event => { void upload(event.target.files?.[0]); event.target.value = ''; }} /></label>
      <label>{t('catalogDocument.content')}<textarea aria-label={t('catalogDocument.content')} rows={14} spellCheck={false} value={text} onChange={event => { invalidate(); setText(event.target.value); if (working === 'check') work(undefined); }} /></label>
      <details><summary>{t('catalogDocument.help')}</summary><p>{t(kind === 'resource-definitions' ? 'catalogDocument.definitionFormat' : 'catalogDocument.format')}</p></details>
      {error ? <p role="alert" className="form-error">{error}</p> : null}
      {status ? <p role="status" className="form-success">{status}</p> : null}
      {working === 'check' ? <p role="status">{t('catalogDocument.checking')}</p> : null}
      {prepared?.source ? <SavedSourceSummary source={{ kind: 'git', ...prepared.source }} /> : null}
      <div className="form-actions">
        {kind === 'resource-definitions' ? <Button type="button" disabled={busy || !text.trim()} onClick={() => void check()}>{t('catalogDocument.check')}</Button> : null}
        <Button type="button" tone="primary" disabled={busy || !text.trim() || (kind === 'resource-definitions' && !prepared)} onClick={() => void submit()}>{t('catalogDocument.register')}</Button>
      </div>
    </fieldset>
  </section>;
}
