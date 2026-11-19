import { useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { t } from '../../shared/i18n';

export function CatalogDeleteButton({ collection, itemKey, disabled, onDeleted }: {
  collection: 'resource-types' | 'resource-definitions';
  itemKey: string;
  disabled?: boolean;
  onDeleted(): void;
}) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [references, setReferences] = useState<string[]>([]);
  const [uncertain, setUncertain] = useState(false);

  async function remove() {
    if (busy || uncertain) return;
    setBusy(true);
    setMessage('');
    setReferences([]);
    try {
      await api(`/${collection}/${encodeURIComponent(itemKey)}`, { method: 'DELETE' });
    } catch (error) {
      setBusy(false);
      if (error instanceof ApiError && error.status < 500) {
        setMessage(error.message);
        setReferences(error.references ?? []);
      } else {
        setMessage(t('catalogDeletion.failed'));
        setUncertain(true);
      }
      return;
    }
    setBusy(false);
    setOpen(false);
    onDeleted();
  }

  return <>
    <Button type="button" tone="danger" disabled={disabled} aria-label={t('catalogDeletion.action', { key: itemKey })} onClick={() => {
      setMessage(''); setReferences([]); setUncertain(false); setOpen(true);
    }}>{t('catalogDeletion.delete')}</Button>
    {open ? <Dialog label={t('catalogDeletion.title', { key: itemKey })} busy={busy} onClose={() => setOpen(false)}>
      <h2>{t('catalogDeletion.title', { key: itemKey })}</h2>
      <p>{t('catalogDeletion.prompt')}</p>
      {message ? <div className="form-error" role="alert">{message}
        {references.length ? <ul>{references.map((reference) => <li key={reference}>{reference === 'builtin' ? t('catalogDeletion.builtin') : <code>{reference}</code>}</li>)}</ul> : null}
      </div> : null}
      <div className="form-actions">
        <Button type="button" disabled={busy} onClick={() => setOpen(false)}>{t('catalogDeletion.cancel')}</Button>
        <Button type="button" tone="danger" disabled={busy || uncertain} onClick={() => void remove()}>{t('catalogDeletion.delete')}</Button>
      </div>
    </Dialog> : null}
  </>;
}
