import { useEffect, useRef, useState } from 'react';
import { ApiError } from '../../shared/api/client';
import { toTarget } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { fetchEnvironment, listStoreChoices, setSecretStore, type StoreChoice } from './api';

type State = { status: 'loading' | 'error' } | { status: 'ready'; stores: readonly StoreChoice[] };

// Editable Secret Store Connection of ONE Environment (UC-01/UC-12, ADR-012).
// Changing it copies the desired Secrets first and commits only when every copy
// verified; running workloads move after the next Preview and Deploy.
export function SecretStoreSelection({ application, environment, configVersion, onTargetChange, onConfigurationChanged }: {
  application: Application; environment: EnvironmentKey; configVersion: number;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onConfigurationChanged(): void;
}) {
  const target = application.environments[environment];
  const busy = Boolean(target.activeOperation);
  const [choices, setChoices] = useState<State>({ status: 'loading' });
  const [selected, setSelected] = useState(target.secretStoreKey);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  const generation = useRef(0);
  useEffect(() => () => { generation.current += 1; }, []);
  useEffect(() => { generation.current += 1; setSelected(application.environments[environment].secretStoreKey); setError(''); setNotice(''); setSaving(false); }, [application.id, environment]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const current = ++generation.current;
    setChoices({ status: 'loading' });
    listStoreChoices().then((stores) => { if (current === generation.current) setChoices({ status: 'ready', stores }); }).catch(() => { if (current === generation.current) setChoices({ status: 'error' }); });
  }, [application.id, environment, attempt]);

  async function save() {
    if (!selected || saving || busy) return;
    const current = ++generation.current;
    setSaving(true); setError(''); setNotice('');
    try {
      const result = await setSecretStore(application.id, environment, selected, target.version, configVersion);
      onTargetChange(application.id, environment, toTarget(result.environment));
      onConfigurationChanged();
      if (current === generation.current) setNotice(!result.changed ? 'Already selected; nothing was copied.' : `Secret store saved. ${result.copiedSecrets} secret(s) were copied and verified. Preview and Deploy now to move running workloads.`);
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 409) {
        setError(err.message);
        try { const latest = await fetchEnvironment(application.id, environment); if (latest && current === generation.current) { onTargetChange(application.id, environment, latest); onConfigurationChanged(); setSelected(latest.secretStoreKey); } } catch { /* reload hint below */ }
      } else if (err instanceof ApiError && err.status === 502) {
        setError('The secrets could not be copied to the new store. The previous store is still selected and nothing changed.');
      } else if (err instanceof ApiError && err.status === 422) {
        setError('The selected secret store is not available. Choose another store.');
        setAttempt((value) => value + 1);
      } else {
        setError('Could not save the secret store. Try again.');
      }
    } finally {
      if (current === generation.current) setSaving(false);
    }
  }

  const title = environment === 'staging' ? 'Staging' : 'Production';
  const stores = choices.status === 'ready' ? choices.stores : [];
  return <section className="content-panel secret-store-selection" aria-label="Secret store">
    <div className="section-header"><div><h2>Secret store</h2><p>{target.secretStoreKey ? `${title} stores Secrets in this store. Variables stay in Orchestrator.` : `${title} has no secret store. Variables work without one; selecting a store is required to add a Secret.`}</p></div><span className="pending-pill">{target.secretStoreKey ? (target.secretStoreName || target.secretStoreKey) : 'Not selected'}</span></div>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {busy ? <div className="form-info" role="status">An operation is running on this environment. Settings are read-only until it finishes.</div> : null}
    {choices.status === 'loading' ? <p role="status" aria-busy="true">Loading secret stores…</p> : null}
    {choices.status === 'error' ? <div className="form-error" role="alert">Could not load secret stores.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
    {choices.status === 'ready' && stores.length === 0 ? <div className="form-info" role="status">No READY secret store is available. Ask a Platform Engineer to register one, then retry.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
    {stores.length > 0 ? <>
      <label htmlFor="environment-secret-store">Secret store for {title}</label>
      <select id="environment-secret-store" value={selected} disabled={saving || busy} onChange={(event) => { setSelected(event.target.value); setNotice(''); }}>
        {!target.secretStoreKey ? <option value="" disabled>Choose a secret store</option> : null}
        {stores.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}){item.legacy ? ' · legacy platform store' : ''}</option>)}
      </select>
      <small>Changing the store copies the current Secrets into the new store first and verifies every copy. If any copy fails the previous store stays selected. Running workloads keep their old values until the next Deploy.</small>
      <div className="form-actions"><Button tone="primary" disabled={!selected || saving || busy || selected === target.secretStoreKey} onClick={() => void save()}>{saving ? 'Copying secrets…' : 'Save secret store'}</Button></div>
    </> : null}
  </section>;
}
