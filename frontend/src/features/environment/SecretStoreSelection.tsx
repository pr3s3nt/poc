import { useEffect, useRef, useState } from 'react';
import { ApiError, isBusy, isStaleConflict } from '../../shared/api/client';
import { toTarget } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { environmentTitle, statusLabel } from './labels';
import { fetchEnvironment, listStoreChoices, setSecretStore, type StoreChoice } from './api';
import { t } from '../../shared/i18n';

type State = { status: 'loading' | 'error' } | { status: 'ready'; stores: readonly StoreChoice[] };

// Editable Secret Store Connection of ONE Environment (UC-01/UC-12, ADR-012).
// Changing it copies the desired Secrets first and commits only when every copy
// verified; running workloads move after the next Preview and Deploy.
// With no binding and exactly one READY store, that store is only SUGGESTED in
// the draft (not an Organization default or fallback); nothing persists until
// Save. The draft is derived from `picked`, so a reload or a late response never
// overwrites the user's choice, and a bound store is never retargeted.
export function SecretStoreSelection({ application, environment, configVersion, onTargetChange, onConfigurationChanged }: {
  application: Application; environment: EnvironmentKey; configVersion: number;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onConfigurationChanged(): void;
}) {
  const target = application.environments[environment];
  const busy = Boolean(target.activeOperation);
  const [choices, setChoices] = useState<State>({ status: 'loading' });
  const [picked, setPicked] = useState<string>();
  const [saving, setSaving] = useState(false);
  // A stale conflict whose reload failed: saving stays off until a reload succeeds.
  const [needsReload, setNeedsReload] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  const generation = useRef(0);
  // `scope` changes only with the Application/Environment or unmount (not with a
  // choices reload), so a saved store refreshes the configuration of the page the
  // user is still on and never of one they have left.
  const scope = useRef(0);
  useEffect(() => () => { generation.current += 1; scope.current += 1; }, []);
  useEffect(() => { scope.current += 1; generation.current += 1; setPicked(undefined); setError(''); setNotice(''); setSaving(false); setNeedsReload(false); }, [application.id, environment]);
  useEffect(() => {
    const current = ++generation.current;
    setChoices({ status: 'loading' });
    listStoreChoices().then((stores) => { if (current === generation.current) setChoices({ status: 'ready', stores }); }).catch(() => { if (current === generation.current) setChoices({ status: 'error' }); });
  }, [application.id, environment, attempt]);

  const title = environmentTitle(environment);
  const eligible = choices.status === 'ready' ? choices.stores.filter((item) => item.status === 'READY') : [];
  // The bound store stays listed even if it is no longer READY.
  const bound = choices.status === 'ready' && target.secretStoreKey && !eligible.some((item) => item.key === target.secretStoreKey)
    ? [choices.stores.find((item) => item.key === target.secretStoreKey) ?? { key: target.secretStoreKey, name: target.secretStoreName || target.secretStoreKey, provider: '', status: '' }] : [];
  const stores = [...bound, ...eligible];
  const suggestion = !target.secretStoreKey && eligible.length === 1 ? eligible[0] : undefined;
  const candidate = picked ?? (target.secretStoreKey || suggestion?.key || '');
  const selected = stores.some((item) => item.key === candidate) ? candidate : '';
  const suggested = picked === undefined && Boolean(suggestion) && selected === suggestion?.key;
  const lostPick = picked !== undefined && picked !== '' && selected === '' && choices.status === 'ready';

  // True only when the authoritative GET succeeded and returned this Environment.
  async function reloadLatest(current: number): Promise<boolean> {
    try {
      const latest = await fetchEnvironment(application.id, environment);
      if (!latest) return false;
      if (current === generation.current) { onTargetChange(application.id, environment, latest); onConfigurationChanged(); }
      return true;
    } catch { return false; }
  }

  async function retryReload() {
    const current = ++generation.current;
    if (await reloadLatest(current) && current === generation.current) { setNeedsReload(false); setError(''); setNotice(t('environment.secretStoreSelection.theLatestSelectionHasBeen')); }
    else if (current === generation.current) setError(t('environment.secretStoreSelection.stillCouldNotReloadThe'));
  }

  async function save() {
    if (!selected || saving || busy || needsReload) return;
    const current = ++generation.current;
    const mine = scope.current;
    setSaving(true); setError(''); setNotice('');
    try {
      const result = await setSecretStore(application.id, environment, selected, target.version, configVersion);
      // The confirmed result belongs to the Environment it was saved for; only the refresh is scope-bound.
      onTargetChange(application.id, environment, toTarget(result.environment));
      if (mine === scope.current) onConfigurationChanged();
      if (current === generation.current) { setPicked(undefined); setNotice(!result.changed ? t('environment.secretStoreSelection.thisStoreIsSelectedNothing') : t('environment.secretStoreSelection.secretStoreSavedCopiedSecretsSecrets', { copiedSecrets: result.copiedSecrets })); }
    } catch (err) {
      if (current !== generation.current) return;
      if (isBusy(err)) {
        setError((err as ApiError).message);
      } else if (isStaleConflict(err)) {
        const base = (err as ApiError).message;
        const reloaded = await reloadLatest(current);
        if (current !== generation.current) return;
        setNeedsReload(!reloaded);
        setError(reloaded ? t('environment.secretStoreSelection.baseTheLatestSelectionHas', { base }) : t('environment.secretStoreSelection.baseCouldNotReloadThe', { base }));
      } else if (err instanceof ApiError && err.code === 'SECRET_COPY_FAILED') {
        setError(t('environment.secretStoreSelection.couldNotCopyTheSecrets'));
      } else if (err instanceof ApiError && err.status === 422) {
        setError(err.code === 'SECRET_STORE_UNAVAILABLE' ? t('environment.secretStoreSelection.theSelectedSecretStoreIs') : err.message);
        setAttempt((value) => value + 1);
      } else {
        // A network failure does not prove the selection was not saved.
        setError(err instanceof ApiError ? t('environment.secretStoreSelection.couldNotSaveTheSecret', { message: err.message }) : t('environment.secretStoreSelection.couldNotSaveTheSecret2'));
      }
    } finally {
      if (current === generation.current) setSaving(false);
    }
  }

  return <section className="content-panel secret-store-selection" aria-label={t('environment.secretStoreSelection.secretStore')}>
    <div className="section-header"><div><h2>{t('environment.secretStoreSelection.secretStore')}</h2><p>{target.secretStoreKey ? t('environment.secretStoreSelection.titleStoresSecretsInThis', { title }) : t('environment.secretStoreSelection.titleHasNoSecretStore', { title })}</p></div><span className="pending-pill">{target.secretStoreKey ? (target.secretStoreName || target.secretStoreKey) : t('environment.secretStoreSelection.notSelected')}</span></div>
    {error ? <div className="form-error" role="alert">{error}{needsReload ? <Button type="button" onClick={() => void retryReload()}>{t('environment.secretStoreSelection.reloadStatus')}</Button> : null}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {busy ? <div className="form-info" role="status">{t('environment.secretStoreSelection.anOperationIsRunningOn')}</div> : null}
    {choices.status === 'loading' ? <p role="status" aria-busy="true">{t('environment.secretStoreSelection.loadingSecretStores')}</p> : null}
    {choices.status === 'error' ? <div className="form-error" role="alert">{t('environment.secretStoreSelection.couldNotLoadTheSecret')}<Button type="button" onClick={() => setAttempt((value) => value + 1)}>{t('environment.secretStoreSelection.retry')}</Button></div> : null}
    {choices.status === 'ready' && stores.length === 0 ? <div className="form-info" role="status">{t('environment.secretStoreSelection.noReadyREADYSecretStore')}<Button type="button" onClick={() => setAttempt((value) => value + 1)}>{t('environment.secretStoreSelection.retry')}</Button></div> : null}
    {stores.length > 0 ? <>
      <label htmlFor="environment-secret-store">{t('environment.secretStoreSelection.secretStoreForTitle', { title })}</label>
      <select id="environment-secret-store" value={selected} disabled={saving || busy} onChange={(event) => { setPicked(event.target.value); setNotice(''); }}>
        {!target.secretStoreKey || !selected ? <option value="" disabled>{t('environment.secretStoreSelection.chooseASecretStore')}</option> : null}
        {stores.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}){item.status && item.status !== 'READY' ? ` · ${statusLabel(item.status)}` : ''}</option>)}
      </select>
      {suggested ? <div className="form-info" role="status">{t('environment.secretStoreSelection.unsavedSuggestion')}{' '} <strong>{suggestion?.name}</strong> {' '}{t('environment.secretStoreSelection.isTheOnlyReadySecret')}</div> : null}
      {lostPick ? <div className="form-info" role="status">{t('environment.secretStoreSelection.theStoreYouChoseEarlier')}</div> : null}
      {!target.secretStoreKey && stores.length > 1 && !selected ? <small>{t('environment.secretStoreSelection.severalStoresAreReadyChoose')}</small> : null}
      <small>{t('environment.secretStoreSelection.changingStoresFirstCopiesThe')}</small>
      <div className="form-actions"><Button tone="primary" disabled={!selected || saving || busy || needsReload || selected === target.secretStoreKey} onClick={() => void save()}>{saving ? t('environment.secretStoreSelection.copyingSecrets') : t('environment.secretStoreSelection.saveSecretStore')}</Button></div>
    </> : null}
  </section>;
}
