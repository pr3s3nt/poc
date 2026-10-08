import { useCallback, useEffect, useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';

type Store = { key: string; name: string; provider: string; backendAddress: string; workloadAddress: string; mount: string; authMount: string; tlsCustomCa: boolean; status: string; legacy?: boolean; verification: Record<string, unknown> };
type Form = { name: string; backendAddress: string; workloadAddress: string; mount: string; authMount: string; tlsCaPem: string; token: string };
const empty: Form = { name: '', backendAddress: '', workloadAddress: '', mount: 'kv', authMount: 'kubernetes', tlsCaPem: '', token: '' };

// Platform Engineer registration of Organization-scoped Vault KV v2 stores
// (UC-04 SS-01..06). The token is write-only: it is cleared after submit and
// never shown again; the list shows only safe metadata.
export function SecretStoresPage() {
  const [stores, setStores] = useState<readonly Store[]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [form, setForm] = useState<Form>(empty);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const load = useCallback(async () => {
    setState('loading');
    try { const response = await api<{ secretStores: Store[] }>('/secret-stores'); setStores(response.secretStores ?? []); setState('ready'); } catch { setState('error'); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  async function register() {
    if (saving) return;
    setSaving(true); setError(''); setSuccess('');
    try {
      const created = await api<Store>('/secret-stores', { method: 'POST', body: JSON.stringify({ ...form, tlsCaPem: form.tlsCaPem || undefined }) });
      setForm(empty);
      setSuccess(`Secret store "${created.name}" is READY. Developers can now select it in Environment Settings.`);
      await load();
    } catch (err) {
      // The token field is cleared on every failure so it never lingers in the page.
      setForm((value) => ({ ...value, token: '' }));
      setError(err instanceof ApiError ? err.message : 'Could not register the secret store. Try again.');
    } finally { setSaving(false); }
  }
  const set = (key: keyof Form) => (event: { target: { value: string } }) => setForm({ ...form, [key]: event.target.value });
  const valid = form.name.trim() && form.backendAddress.trim() && form.workloadAddress.trim() && form.token;
  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">Platform</p><h1>Secret stores</h1><p>Register Vault KV v2 stores that Developers can select per environment. The token stays in the platform credential store and is never shown again.</p></div></header>
    {success ? <div className="form-success" role="status">{success}</div> : null}
    <section className="content-panel" aria-label="Registered secret stores">
      <div className="section-header"><div><h2>Registered stores</h2></div></div>
      {state === 'loading' ? <p role="status" aria-busy="true">Loading secret stores…</p> : null}
      {state === 'error' ? <div className="form-error" role="alert">Could not load secret stores.<Button onClick={() => void load()}>Retry</Button></div> : null}
      {state === 'ready' && stores.length === 0 ? <div className="section-empty">No secret store is registered yet.</div> : null}
      {stores.length > 0 ? <div className="settings-table"><div className="settings-table-head"><span>Name</span><span>Backend</span><span>Mounts</span><span>Status</span></div>
        {stores.map((store) => <div className="settings-table-row" key={store.key}><strong>{store.name}{store.legacy ? ' (legacy)' : ''}<small className="muted"> {store.key}</small></strong><span>{store.backendAddress}</span><span>kv: {store.mount} · auth: {store.authMount}{store.tlsCustomCa ? ' · custom CA' : ''}</span><span>{store.status}{store.verification.kubernetesAuth ? ` · k8s auth ${String(store.verification.kubernetesAuth).toLowerCase()}` : ''}</span></div>)}</div> : null}
    </section>
    <form className="create-form" aria-label="Register secret store" onSubmit={(event) => { event.preventDefault(); void register(); }}>
      <h2>Register a Vault KV v2 store</h2>
      {error ? <div className="form-error" role="alert">{error}</div> : null}
      <label>Name<input value={form.name} onChange={set('name')} placeholder="Team Vault" maxLength={100} /></label>
      <label>Backend address<input value={form.backendAddress} onChange={set('backendAddress')} placeholder="https://vault.example.com:8200" /><small>Used by the orchestrator to read and write values.</small></label>
      <label>Workload address<input value={form.workloadAddress} onChange={set('workloadAddress')} placeholder="http://vault.vault.svc:8200" /><small>Address workload Pods and the Vault Secrets Operator use inside the cluster.</small></label>
      <label>KV v2 mount<input value={form.mount} onChange={set('mount')} /></label>
      <label>Kubernetes auth mount<input value={form.authMount} onChange={set('authMount')} /></label>
      <label>CA certificate (optional PEM)<textarea value={form.tlsCaPem} onChange={set('tlsCaPem')} rows={3} /><small>TLS verification is always on.</small></label>
      <label>Token<input type="password" autoComplete="off" value={form.token} onChange={set('token')} /><small>Needs KV v2 value access plus workload policy and Kubernetes-auth role management. Verified before the store becomes READY.</small></label>
      <div className="form-actions"><Button tone="primary" type="submit" disabled={saving || !valid}>{saving ? 'Verifying…' : 'Verify and register'}</Button></div>
    </form>
  </section>;
}
