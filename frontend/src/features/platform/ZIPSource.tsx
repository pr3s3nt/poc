import { useEffect, useRef, useState } from 'react';
import { api } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';

type Props<T> = {
  endpoint: string;
  resourceType?: string;
  disabled: boolean;
  parseResult: (response: unknown) => T;
  onResult: (value: T | null) => void;
};

// Upload shares inspection semantics with Git: edits invalidate the prepared
// pin, and stale responses cannot restore an old file/path/Type selection.
export function ZIPSource<T>({ endpoint, resourceType, disabled, parseResult, onResult }: Props<T>) {
  const [file, setFile] = useState<File | null>(null);
  const [path, setPath] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [verified, setVerified] = useState(false);
  const version = useRef(0);
  const callback = useRef(onResult);
  callback.current = onResult;
  useEffect(() => { const tracker = version; return () => { tracker.current++; }; }, []);
  function invalidate() {
    version.current++; setBusy(false); setError(''); setVerified(false); callback.current(null);
  }
  async function inspect() {
    if (!file) return;
    invalidate();
    if (!file.name.toLowerCase().endsWith('.zip') || file.size > 32 * 1024 * 1024 || file.size === 0) { setError(t('platform.zipSource.invalidFile')); return; }
    const current = ++version.current;
    const body = new FormData();
    body.append('file', file); body.append('path', path.trim() || '.');
    if (resourceType !== undefined) body.append('resourceType', resourceType);
    setBusy(true);
    try {
      const result = parseResult(await api<unknown>(endpoint, { method: 'POST', body }));
      if (current !== version.current) return;
      callback.current(result); setVerified(true);
    } catch {
      if (current === version.current) setError(t('platform.zipSource.failed'));
    } finally { if (current === version.current) setBusy(false); }
  }
  return <div className="catalog-fields" role="group" aria-label={t('platform.zipSource.uploadZIP')}>
    <div className="field-grid">
      <label>{t('platform.zipSource.file')}<input type="file" accept=".zip,application/zip" disabled={disabled} onChange={(event) => { invalidate(); setFile(event.target.files?.[0] ?? null); }} /></label>
      <label>{t('platform.zipSource.directory')}<input value={path} placeholder="." disabled={disabled} spellCheck={false} onChange={(event) => { invalidate(); setPath(event.target.value); }} /></label>
    </div>
    <details className="feature-note"><summary>{t('platform.zipSource.help')}</summary><p>{t('platform.zipSource.helpText')}</p></details>
    <p><Button type="button" disabled={disabled || busy || !file || resourceType === ''} onClick={() => { void inspect(); }}>{busy ? t('platform.zipSource.uploading') : t('platform.zipSource.inspect')}</Button></p>
    {busy ? <p role="status">{t('platform.zipSource.uploading')}</p> : null}
    {error ? <p role="alert" className="form-error">{error}</p> : null}
    {verified ? <p role="status" className="form-success">{t('platform.zipSource.verified')}</p> : null}
  </div>;
}
