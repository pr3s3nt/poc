import { useEffect, useRef, useState } from 'react';
import { ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { toTarget } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { createKey, getConfiguration, type Configuration, type KeyKind } from '../configuration/api';
import { listStoreChoices, setSecretStore, type StoreChoice } from '../environment/api';
import { t } from '../../shared/i18n';
import type { KeyCatalog } from './bindings';

const NAME = /^[A-Za-z_][A-Za-z0-9_]*$/;
type Ready = Extract<KeyCatalog, { status: 'ready' }>;
type Stores = { status: 'loading' | 'error' } | { status: 'ready'; stores: StoreChoice[] };

// Creates one Application variable or secret without leaving the workload editor
// (UC-16 BR-21, UC-12 BR-24..28). The key is committed by the create-only API, on its
// own: cancelling or failing the workload Save later keeps it. The secret exists only
// in this masked input and the request. Nothing here retries a create on its own.
export function CreateKeyDialog({ application, environment, kind, suggestedName, catalog, onConfiguration, onCreated, onTargetChange, onClose }: {
  application: Application; environment: EnvironmentKey; kind: KeyKind; suggestedName: string; catalog: Ready;
  onConfiguration(configuration: Configuration): void;
  onCreated(name: string): void;
  onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onClose(): void;
}) {
  const secret = kind === 'SECRET';
  const [name, setName] = useState(NAME.test(suggestedName) ? suggestedName : '');
  const [value, setValue] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  // reviewed: a GET after a duplicate / unknown outcome has shown the current keys.
  const [reviewed, setReviewed] = useState(false);
  const [unknown, setUnknown] = useState(false);
  const [stores, setStores] = useState<Stores>({ status: 'loading' });
  const [picked, setPicked] = useState('');
  const [storeNotice, setStoreNotice] = useState('');
  const [storeAttempt, setStoreAttempt] = useState(0);
  const alive = useRef(true);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);

  const needsStore = secret && !catalog.secretStoreKey;
  useEffect(() => {
    if (!needsStore) return;
    let cancelled = false;
    setStores({ status: 'loading' });
    listStoreChoices().then((list) => { if (!cancelled) setStores({ status: 'ready', stores: list }); }).catch(() => { if (!cancelled) setStores({ status: 'error' }); });
    return () => { cancelled = true; };
  }, [needsStore, storeAttempt]);
  const ready = stores.status === 'ready' ? stores.stores.filter((item) => item.status === 'READY') : [];

  const trimmedProblem = !NAME.test(name) ? t('workloads.createKey.nameInvalid') : !value ? t('workloads.createKey.valueRequired') : value.includes('\u0000') ? t('workloads.createKey.valueNul') : '';
  const existing = catalog.keys.find((key) => key.name === name);

  async function review(): Promise<boolean> {
    try {
      const latest = await getConfiguration(application.id, environment);
      if (!alive.current) return false;
      onConfiguration(latest);
      if (alive.current) setReviewed(true);
      return true;
    } catch {
      if (alive.current) setError(t('workloads.createKey.reviewFailed'));
      return false;
    }
  }

  async function submit() {
    if (busy || needsStore || trimmedProblem) return;
    setBusy(true); setError(''); setUnknown(false); setReviewed(false);
    try {
      const result = await createKey(application.id, environment, name, kind, value, catalog.version);
      if (!alive.current) return;
      onConfiguration(result);
      onCreated(name);
    } catch (err) {
      if (!alive.current) return;
      if (err instanceof ApiError && err.code === 'DUPLICATE') { setError(err.message); await review(); }
      else if (isStaleConflict(err)) { setError(`${(err as ApiError).message} ${t('workloads.createKey.reloadedNotice')}`); await review(); }
      else if (isBusy(err)) setError((err as ApiError).message);
      else if (err instanceof ApiError && err.status < 500 && err.code !== 'COMMIT_UNCERTAIN') setError(err.message);
      else { setUnknown(true); setError(t('workloads.createKey.unknownOutcome')); }
    } finally {
      if (alive.current) setBusy(false);
    }
  }

  async function saveStore() {
    if (busy || !picked) return;
    setBusy(true); setError(''); setStoreNotice('');
    try {
      const result = await setSecretStore(application.id, environment, picked, application.environments[environment].version, catalog.version);
      if (!alive.current) return;
      onTargetChange?.(application.id, environment, toTarget(result.environment));
      const chosen = ready.find((item) => item.key === picked);
      if (alive.current) setStoreNotice(t('workloads.createKey.storeSaved', { store: chosen?.name ?? picked }));
      const latest = await getConfiguration(application.id, environment);
      if (alive.current) onConfiguration(latest);
    } catch (err) {
      if (!alive.current) return;
      setError(err instanceof ApiError ? err.message : t('workloads.createKey.storeUnknown'));
    } finally {
      if (alive.current) setBusy(false);
    }
  }

  const title = secret ? t('workloads.createKey.titleSecret') : t('workloads.createKey.titleVariable');
  const canUseExisting = reviewed && existing?.kind === kind;
  return <Dialog label={title} busy={busy} onClose={onClose}>
    <h2>{title}</h2>
    <p className="muted">{t('workloads.createKey.scope', { application: application.name, environment })}</p>
    {needsStore ? <fieldset className="form-fieldset" disabled={busy}>
      <legend>{t('workloads.createKey.storeLegend')}</legend>
      {stores.status === 'loading' ? <p role="status" aria-busy="true">{t('workloads.createKey.storesLoading')}</p> : null}
      {stores.status === 'error' ? <div className="form-error" role="alert">{t('workloads.createKey.storesError')} <Button type="button" onClick={() => setStoreAttempt((value) => value + 1)}>{t('workloads.createKey.retry')}</Button></div> : null}
      {stores.status === 'ready' && ready.length === 0 ? <div className="form-info" role="status">{t('workloads.createKey.noStore')} <Button type="button" onClick={() => setStoreAttempt((value) => value + 1)}>{t('workloads.createKey.reload')}</Button></div> : null}
      {ready.length > 0 ? <>
        <label className="field">{t('workloads.createKey.storeLabel')}<select aria-label={t('workloads.createKey.storeLabel')} value={picked} onChange={(event) => setPicked(event.target.value)}><option value="">{t('workloads.createKey.storeChoose')}</option>{ready.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key})</option>)}</select></label>
        <Button tone="primary" type="button" disabled={!picked || busy} onClick={() => void saveStore()}>{t('workloads.createKey.storeSave')}</Button>
      </> : null}
    </fieldset> : null}
    {secret && catalog.secretStoreKey ? <p className="muted" role="note">{storeNotice || t('workloads.createKey.storeSelected', { store: catalog.secretStoreKey })}</p> : null}
    <label className="field">{t('workloads.createKey.nameLabel')}<input value={name} disabled={busy} autoComplete="off" spellCheck={false} onChange={(event) => { setName(event.target.value); setReviewed(false); }} /></label>
    <label className="field">{secret ? t('workloads.createKey.secretValueLabel') : t('workloads.createKey.valueLabel')}<input type={secret ? 'password' : 'text'} value={value} disabled={busy} autoComplete={secret ? 'new-password' : 'off'} spellCheck={false} onChange={(event) => setValue(event.target.value)} /></label>
    <p className="muted">{t('workloads.createKey.boundary', { environment })}</p>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {unknown ? <div className="form-actions"><Button type="button" disabled={busy} onClick={() => void review()}>{t('workloads.createKey.reviewConfig')}</Button></div> : null}
    {reviewed && existing && existing.kind !== kind ? <div className="form-info" role="status">{t('workloads.createKey.otherKind', { name })}</div> : null}
    <div className="form-actions">
      <Button type="button" disabled={busy} onClick={onClose}>{t('workloads.createKey.cancel')}</Button>
      {canUseExisting ? <Button type="button" disabled={busy} onClick={() => onCreated(name)}>{t('workloads.createKey.useExisting')}</Button> : null}
      <Button tone="primary" type="button" disabled={busy || needsStore || Boolean(trimmedProblem) || unknown && !reviewed} aria-describedby={trimmedProblem ? 'create-key-problem' : undefined} onClick={() => void submit()}>{busy ? t('workloads.createKey.creating') : t('workloads.createKey.submit')}</Button>
    </div>
    {trimmedProblem && (name || value) ? <small id="create-key-problem" className="muted">{trimmedProblem}</small> : null}
  </Dialog>;
}
