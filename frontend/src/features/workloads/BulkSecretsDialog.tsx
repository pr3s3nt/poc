import { useEffect, useRef, useState } from 'react';
import { ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { toTarget } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { createKey, getConfiguration, type Configuration } from '../configuration/api';
import { listStoreChoices, setSecretStore, type StoreChoice } from '../environment/api';
import { changeSource, type Binding, type KeyCatalog } from './bindings';
import { t } from '../../shared/i18n';

type Status = 'pending' | 'created' | 'reused' | 'failed' | 'unknown';
export function BulkSecretsDialog({ application, environment, rows, catalog, onConfiguration, onReplace, onTargetChange, onBusy, onUncertain, onClose }: {
  application: Application; environment: EnvironmentKey; rows: Binding[]; catalog: Extract<KeyCatalog, { status: 'ready' }>;
  onConfiguration(value: Configuration): void; onReplace(id: string, binding: Binding): void;
  onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onUncertain(id: string, value: boolean): void;
  onBusy(value: boolean): void; onClose(): void;
}) {
  const [items, setItems] = useState(() => rows.map(row => ({ row, name: row.name, status: (row.secretUncertain ? 'unknown' : 'pending') as Status, reuse: false })));
  const [busy, setBusy] = useState(false), [error, setError] = useState(''), [reviewed, setReviewed] = useState(false);
  const [stores, setStores] = useState<StoreChoice[]>([]), [storeStatus, setStoreStatus] = useState('loading'), [picked, setPicked] = useState('');
  const alive = useRef(true);
  const mutations = useRef(false);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  function markBusy(value: boolean) { mutations.current = value; setBusy(value); onBusy(value); }
  async function loadStores() {
    setStoreStatus('loading');
    try { const choices = await listStoreChoices(); if (alive.current) { setStores(choices.filter(store => store.status === 'READY')); setStoreStatus('ready'); } }
    catch { if (alive.current) setStoreStatus('error'); }
  }
  useEffect(() => {
    if (catalog.secretStoreKey) return;
    let cancelled = false;
    listStoreChoices().then(choices => { if (!cancelled) { setStores(choices.filter(store => store.status === 'READY')); setStoreStatus('ready'); } }).catch(() => { if (!cancelled) setStoreStatus('error'); });
    return () => { cancelled = true; };
  }, [catalog.secretStoreKey]);
  async function saveStore() {
    if (mutations.current || !picked) return;
    markBusy(true); setError('');
    try {
      const result = await setSecretStore(application.id, environment, picked, application.environments[environment].version, catalog.version);
      if (!alive.current) return;
      onTargetChange?.(application.id, environment, toTarget(result.environment));
      const config = await getConfiguration(application.id, environment);
      if (alive.current) onConfiguration(config);
    } catch (err) { if (alive.current) setError(err instanceof ApiError ? err.message : t('workloads.createKey.storeUnknown')); }
    finally { if (alive.current) markBusy(false); }
  }
  async function review() {
    if (mutations.current) return;
    markBusy(true); setError('');
    try {
      const config = await getConfiguration(application.id, environment);
      if (!alive.current) return;
      onConfiguration(config); setReviewed(true);
      items.filter(item => item.status === 'unknown' && !config.keys.some(key => key.name === item.name)).forEach(item => onUncertain(item.row.id, false));
      // A missing key after a successful GET permits an explicit new attempt.
      setItems(prev => prev.map(item => item.status === 'unknown' && !config.keys.some(key => key.name === item.name) ? { ...item, status: 'pending' } : item));
    } catch { if (alive.current) setError(t('workloads.createKey.reviewFailed')); }
    finally { if (alive.current) markBusy(false); }
  }
  const pending = items.filter(item => item.status !== 'created' && item.status !== 'reused');
  const invalid = pending.some(item => !/^[A-Za-z_][A-Za-z0-9_]*$/.test(item.name) || !item.row.value || item.row.value.includes('\u0000') || items.some(other => other.row.id !== item.row.id && other.name === item.name) || item.status === 'unknown' && !item.reuse || catalog.keys.some(key => key.name === item.name && (key.kind !== 'SECRET' || !item.reuse)));
  async function submit() {
    if (mutations.current || invalid || !catalog.secretStoreKey) return;
    markBusy(true); setError('');
    let version = catalog.version;
    for (const item of pending) {
      if (!alive.current) return;
      try {
        if (!item.reuse) {
          const config = await createKey(application.id, environment, item.name, 'SECRET', item.row.value, version);
          if (!alive.current) return;
          version = config.version; onConfiguration(config);
        }
        const binding = { ...changeSource(item.row, 'SECRET'), key: item.name };
        onReplace(item.row.id, binding);
        setItems(prev => prev.map(current => current.row.id === item.row.id ? { ...current, row: binding, status: item.reuse ? 'reused' : 'created' } : current));
      } catch (err) {
        if (!alive.current) return;
        const known = err instanceof ApiError && (isBusy(err) || isStaleConflict(err) || err.status < 500 && err.code !== 'COMMIT_UNCERTAIN');
        setItems(prev => prev.map(current => current.row.id === item.row.id ? { ...current, status: known ? 'failed' : 'unknown' } : current));
        setError(known ? (err as ApiError).message : t('workloads.createKey.unknownOutcome'));
        if (!known) onUncertain(item.row.id, true);
        setReviewed(false); break;
      }
    }
    if (alive.current) markBusy(false);
  }
  const title = t('workloads.envfile.confirmSecrets');
  return <Dialog label={title} busy={busy} onClose={onClose}>
    <h2>{title}</h2><p>{t('workloads.createKey.scope', { application: application.name, environment })}</p>
    {!catalog.secretStoreKey ? <fieldset disabled={busy}><legend>{t('workloads.createKey.storeLegend')}</legend>
      {storeStatus !== 'ready' ? <p role="status">{t(`workloads.createKey.${storeStatus === 'loading' ? 'storesLoading' : 'storesError'}`)}</p> : null}
      {storeStatus === 'ready' && !stores.length ? <p>{t('workloads.createKey.noStore')}</p> : null}
      <label className="field">{t('workloads.createKey.storeLabel')}<select value={picked} onChange={event => setPicked(event.target.value)}><option value="">{t('workloads.createKey.storeChoose')}</option>{stores.map(store => <option key={store.key} value={store.key}>{store.name}</option>)}</select></label>
      <Button disabled={!picked} onClick={() => void saveStore()}>{t('workloads.createKey.storeSave')}</Button><Button onClick={() => void loadStores()}>{t('workloads.createKey.reload')}</Button>
    </fieldset> : null}
    <div className="envfile-secret-list">{items.map(item => {
      const done = item.status === 'created' || item.status === 'reused';
      const existing = catalog.keys.find(key => key.name === item.name);
      return <div key={item.row.id}><label className="field">{item.row.name}<input aria-label={t('workloads.envfile.secretName', { name: item.row.name })} value={item.name} disabled={busy || done || item.status === 'unknown'} onChange={event => { const name = event.target.value; setItems(prev => prev.map(current => current.row.id === item.row.id ? { ...current, name, reuse: false, status: 'pending' } : current)); }} /></label>
        <span role="status">{t(`workloads.envfile.status.${item.status}`)}</span>
        {!done && existing ? existing.kind === 'SECRET' ? <label><input type="checkbox" disabled={busy || item.status === 'unknown' && !reviewed} checked={item.reuse} onChange={event => { const reuse = event.target.checked; setItems(prev => prev.map(current => current.row.id === item.row.id ? { ...current, reuse } : current)); }} />{t('workloads.createKey.useExisting')}</label> : <p>{t('workloads.createKey.otherKind', { name: item.name })}</p> : null}
      </div>;
    })}</div>
    <p>{t('workloads.envfile.secretBoundary', { environment })}</p>
    {error ? <p className="form-error" role="alert">{error}</p> : null}
    <div className="form-actions"><Button disabled={busy} onClick={onClose}>{t('workloads.envEntry.close')}</Button><Button disabled={busy} onClick={() => void review()}>{t('workloads.createKey.reviewConfig')}</Button><Button tone="primary" disabled={busy || invalid || !catalog.secretStoreKey || !pending.length} onClick={() => void submit()}>{t('workloads.envfile.createSecrets')}</Button></div>
  </Dialog>;
}
