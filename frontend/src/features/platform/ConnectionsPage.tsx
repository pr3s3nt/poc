import { useEffect, useState, type FormEvent } from 'react';
import { api } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';

type Connection = { key: string; kind: string; status: string; config: { cluster?: string; kubeContext?: string; endpoint?: string } };

export function ConnectionsPage() {
  const [connections, setConnections] = useState<Connection[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [key, setKey] = useState('');
  const [clusterId, setClusterId] = useState('');
  const [kubeContext, setKubeContext] = useState('');
  useEffect(() => { api<{ connections: Connection[] }>('/connections').then((result) => setConnections(result.connections)).catch((reason: Error) => setError(reason.message)).finally(() => setLoading(false)); }, []);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(''); setSaving(true);
    try {
      await api('/connections/kubernetes', { method: 'POST', body: JSON.stringify({ key: key.trim(), clusterId: clusterId.trim(), kubeContext: kubeContext.trim() }) });
      const result = await api<{ connections: Connection[] }>('/connections');
      setConnections(result.connections); setKey(''); setClusterId(''); setKubeContext('');
    } catch (reason) { setError((reason as Error).message); }
    finally { setSaving(false); }
  }
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Platform</p><h1>Connections</h1><p>Register clusters that Orchestrator can reach from the backend host.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Registered connections</h2></div>{loading ? <p>Loading connections…</p> : connections.length === 0 ? <p>No connections registered yet.</p> : <div className="catalog-list">{connections.map((connection) => <div className="catalog-entry" key={connection.key}><strong>{connection.key}</strong><span>{connection.kind} · {connection.config?.cluster ?? connection.config?.endpoint ?? '—'} · {connection.status}</span></div>)}</div>}</section>
    <section className="content-panel"><div className="section-header"><div><h2>Register Kubernetes cluster</h2><p>The kube context must already exist on the backend host. Registration checks API access and RBAC without creating resources.</p></div></div>
      <form className="editor-grid" onSubmit={submit}><div className="field-grid"><label>Connection ID<input value={key} required onChange={(event) => setKey(event.target.value)} /></label><label>Cluster ID<input value={clusterId} required onChange={(event) => setClusterId(event.target.value)} /></label></div><label>Kube context<input value={kubeContext} required onChange={(event) => setKubeContext(event.target.value)} /></label>
        {error ? <div className="form-error" role="alert">{error}</div> : null}<div className="form-actions"><Button type="submit" tone="primary" disabled={saving}>{saving ? 'Verifying…' : 'Register cluster'}</Button></div></form>
    </section>
  </section>;
}
