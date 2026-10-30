import { useEffect, useId, useRef, useState } from 'react';
import { navigate } from '../../app/routes';
import { ApiError, describeError } from '../../shared/api/client';
import { t } from '../../shared/i18n';
import type { EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { getDelivery, listDeliveryBindings, saveDelivery, type DeliveryBinding, type DeliverySelection, type DeliveryView } from './api';

const equivalent = (a: DeliverySelection, b: DeliverySelection) => a.mode === b.mode && (a.mode === 'DIRECT' || (a.bindingId === b.bindingId && a.bindingVersion === b.bindingVersion && a.selfHeal === b.selfHeal));
const bindingKey = (id?: string, version?: number) => id ? `${id}:${version}` : '';

// Mounted with an Application/Environment key. Requests from an old scope never
// update the new Environment; uncertain saves require a GET before another write.
export function DeliverySelectionPanel({ applicationId, environment, canEdit, busy, onDirtyChange, onSaved }: {
  applicationId: string; environment: EnvironmentKey; canEdit: boolean; busy: boolean;
  onDirtyChange(dirty: boolean): void; onSaved(): void;
}) {
	const providerId = useId();
  const [view, setView] = useState<DeliveryView>();
  const [bindings, setBindings] = useState<DeliveryBinding[]>([]);
  const [draft, setDraft] = useState<DeliverySelection>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState(false);
  const [needsReload, setNeedsReload] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  useEffect(() => {
    let current = true;
    setLoading(true);
    Promise.all([getDelivery(applicationId, environment), listDeliveryBindings()]).then(([next, choices]) => {
      if (!current) return;
      setView(next); setBindings(choices); setDraft(next.desired); setNeedsReload(false); setError('');
    }).catch(() => { if (current) setError(t('delivery.loadFailed')); }).finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [applicationId, environment, attempt]);
  const dirty = Boolean(view && draft && !equivalent(view.desired, draft));
  useEffect(() => { onDirtyChange(dirty); return () => onDirtyChange(false); }, [dirty, onDirtyChange]);
  useEffect(() => {
    if (!dirty) return;
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault(); };
    window.addEventListener('beforeunload', warn);
    return () => window.removeEventListener('beforeunload', warn);
  }, [dirty]);
  const eligible = bindings.filter(b => b.status === 'READY');
  const selected = bindings.find(b => bindingKey(b.id, b.version) === bindingKey(draft?.bindingId, draft?.bindingVersion));
  const enabled = draft?.mode === 'CD';
  const locked = !canEdit || busy || saving || loading || needsReload;
  function edit(next: DeliverySelection) { setDraft(next); setNotice(false); setError(''); }
  async function save() {
    if (!view || !draft || locked || !dirty) return;
    const selection: DeliverySelection = draft.mode === 'DIRECT' ? { mode: 'DIRECT', selfHeal: false } : draft;
    setSaving(true); setError(''); setNotice(false);
    try {
      const next = await saveDelivery(applicationId, environment, selection, view);
      if (!alive.current) return;
      setView(next); setDraft(next.desired); setNotice(true); onSaved();
    } catch (err) {
      if (!alive.current) return;
      // Never automatically replay a stale or unknown mutation. Keep the draft
      // visible and require an explicit authoritative reload/review.
      setError(describeError(err, t('delivery.saveUnknown')));
      if (!(err instanceof ApiError) || err.status === 409 || err.status >= 500) setNeedsReload(true);
    } finally { if (alive.current) setSaving(false); }
  }
  const appliedBinding = bindings.find(b => bindingKey(b.id, b.version) === bindingKey(view?.applied.bindingId, view?.applied.bindingVersion));
  return <section className="content-panel" aria-label={t('delivery.title')}>
    <div className="section-header"><h2>{t('delivery.title')}</h2>{view?.pending ? <span className="pending-pill">{t('delivery.pending')}</span> : null}</div>
    {loading ? <p role="status">{t('delivery.loading')}</p> : null}
    {error ? <div className="form-error" role="alert">{error}{needsReload || !view ? <Button onClick={() => setAttempt(v => v + 1)} disabled={saving || loading}>{t('delivery.reload')}</Button> : null}</div> : null}
    {view && draft ? <div className="inline-editor editor-grid">
      <p className="muted">{t('delivery.applied', { provider: view.applied.mode === 'DIRECT' ? t('delivery.direct') : appliedBinding?.name ?? t('delivery.unavailable') })}</p>
      <label className="checkbox-row"><input type="checkbox" checked={enabled} disabled={locked} onChange={e => edit({ ...draft, mode: e.target.checked ? 'CD' : 'DIRECT' })} />{t('delivery.enable')}</label>
      {enabled ? <>
        <label htmlFor={providerId}>{t('delivery.provider')}</label><select id={providerId} value={bindingKey(draft.bindingId, draft.bindingVersion)} disabled={locked} onChange={e => {
          const b = eligible.find(item => bindingKey(item.id, item.version) === e.target.value);
          edit({ mode: 'CD', bindingId: b?.id, bindingVersion: b?.version, selfHeal: Boolean(draft.selfHeal && b?.capabilities.selfHeal) });
        }}><option value="">{t('delivery.choose')}</option>{eligible.map(b => <option key={bindingKey(b.id, b.version)} value={bindingKey(b.id, b.version)}>{b.provider} · {b.name}</option>)}{selected && selected.status !== 'READY' ? <option value={bindingKey(selected.id, selected.version)}>{selected.name} · {t('delivery.unavailable')}</option> : null}</select>
        {eligible.length === 0 ? <p className="form-info">{t('delivery.noProvider')}{canEdit ? <Button onClick={() => navigate({ name: 'cd-providers' })}>{t('cd.title')}</Button> : null}</p> : null}
        {selected?.capabilities.selfHeal ? <label className="checkbox-row"><input type="checkbox" checked={draft.selfHeal} disabled={locked} onChange={e => edit({ ...draft, selfHeal: e.target.checked })} />{t('delivery.selfHeal')}</label> : null}
        <details><summary>{t('delivery.help')}</summary><p>{t('delivery.helpText')}</p></details>
      </> : null}
      {canEdit ? <div className="form-actions"><Button disabled={locked || !dirty} onClick={() => { setDraft(view.desired); setNotice(false); }}>{t('delivery.cancel')}</Button><Button tone="primary" disabled={locked || !dirty || (enabled && selected?.status !== 'READY')} onClick={() => void save()}>{t('delivery.save')}</Button></div> : null}
    </div> : null}
    {notice ? <div className="form-success" role="status">{t('delivery.saved')} <Button onClick={() => navigate({ name: 'application', applicationId, environment })}>{t('delivery.preview')}</Button></div> : null}
  </section>;
}
