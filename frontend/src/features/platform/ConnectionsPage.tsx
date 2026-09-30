import { useState, type FormEvent } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { useCatalogList } from './useCatalogList';

type Connection = { key: string; kind: string; status: string; config: { cluster?: string; kubeContext?: string; endpoint?: string } };

const loadConnections = async () => (await api<{ connections: Connection[] }>('/connections')).connections ?? [];

export function ConnectionsPage() {
  const { items, loading, loadError, reload } = useCatalogList(loadConnections);
  // The Console shows READY Connections only (UC-04 UI states).
  const connections = items.filter((connection) => connection.status === 'READY');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [key, setKey] = useState('');
  const [clusterId, setClusterId] = useState('');
  const [kubeContext, setKubeContext] = useState('');
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setError(''); setNotice(''); setSaving(true);
    const submitted = key.trim();
    try {
      await api('/connections/kubernetes', { method: 'POST', body: JSON.stringify({ key: submitted, clusterId: clusterId.trim(), kubeContext: kubeContext.trim() }) });
    } catch (reason) { setError((reason as Error).message); setSaving(false); return; }
    // Committed: report it and reset the submitted form before reloading.
    setNotice(`Registered connection ${submitted}.`); setKey(''); setClusterId(''); setKubeContext(''); setSaving(false);
    await reload();
  }
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Platform</p><h1>Connections</h1><p>Register clusters that Orchestrator can reach from the backend host.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Registered connections</h2></div>{loadError ? <div className="form-error" role="alert" aria-label="List error">{loadError} <Button type="button" onClick={() => void reload()}>Retry</Button></div> : loading ? <p>Loading connections…</p> : connections.length === 0 ? <p>No connections registered yet.</p> : <div className="catalog-list">{connections.map((connection) => <div className="catalog-entry" key={connection.key}><strong>{connection.key}</strong><span>{connection.kind} · cluster {connection.config?.cluster ?? '—'} · context {connection.config?.kubeContext ?? '—'} · {connection.status}</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Register Kubernetes cluster</h2><p>The kube context must already exist on the backend host. Registration checks API access and RBAC without creating resources.</p></div></div>
      <form onSubmit={submit}><fieldset className="editor-grid form-fieldset" disabled={saving} aria-busy={saving}><div className="field-grid"><label>Connection ID<input value={key} required onChange={(event) => setKey(event.target.value)} /></label><label>Cluster ID<input value={clusterId} required onChange={(event) => setClusterId(event.target.value)} /></label></div><label>Kube context<input value={kubeContext} required onChange={(event) => setKubeContext(event.target.value)} /></label>
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving}>{saving ? 'Verifying…' : 'Register cluster'}</Button></div></fieldset></form>
    </section>
  </section>;
}
