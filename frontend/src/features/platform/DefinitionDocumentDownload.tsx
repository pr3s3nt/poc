import { useEffect, useRef, useState } from 'react';
import { api, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import { Button } from '../../shared/ui/Button';
import { downloadDocument } from './catalogDocument';

export function DefinitionDocumentDownload({ definitionKey, label, revisionId, version, disabled = false }: {
  definitionKey: string; label: string; revisionId?: string; version?: number; disabled?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const generation = useRef(0);
  const pending = useRef(false);
  useEffect(() => {
    generation.current++; setBusy(false); setError(''); pending.current = false;
    const current = generation;
    return () => { current.current++; };
  }, [definitionKey, revisionId, version]);
  async function download() {
    if (pending.current) return;
    const current = generation.current;
    pending.current = true; setBusy(true); setError('');
    try {
      const query = revisionId ? `?revision=${encodeURIComponent(revisionId)}` : version ? `?version=${version}` : '';
      const doc = await api<Record<string, unknown>>(`/resource-definitions/${encodeURIComponent(definitionKey)}/document${query}`);
      if (current === generation.current) downloadDocument('resource-definitions', doc, label);
    } catch (reason) {
      if (current === generation.current) setError(describeError(reason, t('catalogDocument.downloadFailed')));
    } finally {
      if (current === generation.current) { setBusy(false); pending.current = false; }
    }
  }
  return <span>
    <Button type="button" disabled={disabled || busy} onClick={() => void download()}>{t('catalogDocument.download')} · {label}</Button>
    {error ? <span role="alert" className="form-error">{error}</span> : null}
  </span>;
}
