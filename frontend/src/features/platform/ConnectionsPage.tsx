import { useId, useRef, useState, type ChangeEvent, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { useCatalogList } from './useCatalogList';

type Connection = { key: string; name?: string; kind: string; authenticationType?: string; status: string; config?: { cluster?: string; kubeContext?: string; endpoint?: string; region?: string; accountId?: string } };
type KubeconfigContext = { name: string; cluster: string; endpoint: string };
type Source = 'upload' | 'paste';

// Same bound as the backend document limit (UC-04 API mapping).
const maxKubeconfigBytes = 1 << 20;

function readText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ''));
    reader.onerror = () => reject(new Error('read failed'));
    reader.readAsText(file);
  });
}

const loadConnections = async () => (await api<{ connections: Connection[] }>('/connections')).connections ?? [];

function describeFailure(reason: unknown): string {
  if (reason instanceof ApiError) return reason.message;
  // No HTTP status: the request may or may not have reached the backend.
  return 'The outcome is unknown: the connection may have been saved. Reload the list before trying again.';
}

export function ConnectionsPage() {
  const { items, loading, loadError, reload } = useCatalogList(loadConnections);
  // The Console shows READY Connections only (UC-04 UI states).
  const connections = items.filter((connection) => connection.status === 'READY');
  const [name, setName] = useState('');
  const [source, setSource] = useState<Source>('upload');
  const [kubeconfig, setKubeconfig] = useState('');
  const [fileLabel, setFileLabel] = useState('');
  const [hidden, setHidden] = useState(true);
  const [contexts, setContexts] = useState<KubeconfigContext[]>([]);
  const [selected, setSelected] = useState('');
  const [inspecting, setInspecting] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const fileInput = useRef<HTMLInputElement>(null);
  const formId = useId();
  // Every document change bumps the revision so a late inspection response
  // can never restore an older document's contexts or summary.
  const revision = useRef(0);

  function replaceDocument(next: string) {
    revision.current += 1;
    setKubeconfig(next); setContexts([]); setSelected(''); setInspecting(false); setError('');
  }

  function chooseSource(next: Source) {
    if (next === source) return;
    setSource(next); setFileLabel('');
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
  }

  async function chooseFile(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    replaceDocument('');
    setFileLabel('');
    if (!file) return;
    if (file.size > maxKubeconfigBytes) { setError('The kubeconfig file is larger than 1 MiB.'); return; }
    const pending = revision.current;
    let text: string;
    try { text = await readText(file); } catch { if (pending === revision.current) setError('The kubeconfig file could not be read.'); return; }
    if (pending !== revision.current) return;
    setKubeconfig(text); setFileLabel(file.name);
  }

  async function inspect() {
    if (inspecting || saving || !kubeconfig.trim()) return;
    if (new Blob([kubeconfig]).size > maxKubeconfigBytes) { setError('The kubeconfig is larger than 1 MiB.'); return; }
    const pending = revision.current;
    setInspecting(true); setError(''); setNotice('');
    try {
      const result = await api<{ contexts: KubeconfigContext[] }>('/connections/kubernetes/inspect', { method: 'POST', body: JSON.stringify({ kubeconfig }) });
      if (pending !== revision.current) return;
      const found = result.contexts ?? [];
      setContexts(found);
      setSelected(found.length === 1 && found[0] ? found[0].name : '');
    } catch (reason) {
      if (pending === revision.current) setError(reason instanceof ApiError ? reason.message : 'The kubeconfig could not be inspected. Try again.');
    } finally {
      if (pending === revision.current) setInspecting(false);
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving || inspecting) return;
    if (!name.trim()) { setError('Enter a connection name.'); return; }
    if (!contexts.some((context) => context.name === selected)) { setError('Inspect the kubeconfig and select a context before checking and saving.'); return; }
    setError(''); setNotice(''); setSaving(true);
    let created: Connection;
    try {
      created = await api<Connection>('/connections/kubernetes', { method: 'POST', body: JSON.stringify({ name, kubeconfig, context: selected }) });
    } catch (reason) {
      setError(describeFailure(reason)); setSaving(false);
      // An unknown outcome is checked by reloading the list, never by resubmitting.
      if (!(reason instanceof ApiError)) void reload();
      return;
    }
    // Committed: report it, clear every credential and the form, then reload.
    setNotice(`Registered connection ${created.name ?? name.trim()} (${created.key}).`);
    setName(''); setFileLabel(''); setHidden(true);
    if (fileInput.current) fileInput.current.value = '';
    replaceDocument('');
    setSaving(false);
    await reload();
  }

  const chosen = contexts.find((context) => context.name === selected);
  const busy = saving || inspecting;
  return <section className="page"><header className="page-header"><div><p className="eyebrow">Platform</p><h1>Connections</h1><p>Register existing Kubernetes clusters from a kubeconfig. Orchestrator stores the credential in its credential store, not on the backend host.</p></div></header>
    <section className="content-panel"><div className="section-header"><h2>Registered connections</h2></div>{loadError ? <div className="form-error" role="alert" aria-label="List error">{loadError} <Button type="button" onClick={() => void reload()}>Retry</Button></div> : loading ? <p>Loading connections…</p> : connections.length === 0 ? <p>No connections registered yet.</p> : <ConnectionTable connections={connections} />}</section>
    <section className="content-panel"><div className="connection-register">
      <div className="section-header"><div><h2>Register Kubernetes cluster</h2><p>Upload or paste a kubeconfig with an embedded token or client certificate. Check and save verifies API access and permissions without creating resources.</p></div></div>
      <form onSubmit={submit}><fieldset className="form-fieldset connection-form" disabled={saving} aria-busy={busy}>
        <label className="connection-field">Connection name<input value={name} required maxLength={100} onChange={(event) => setName(event.target.value)} /></label>
        <div className="connection-source">
          <div className="source-choices" role="radiogroup" aria-label="Kubeconfig source">
            <label className="source-choice"><input type="radio" name="kubeconfig-source" checked={source === 'upload'} onChange={() => chooseSource('upload')} /> Upload kubeconfig file</label>
            <label className="source-choice"><input type="radio" name="kubeconfig-source" checked={source === 'paste'} onChange={() => chooseSource('paste')} /> Paste kubeconfig</label>
          </div>
          {source === 'upload'
            ? <div className="connection-field"><span id={`${formId}-file`}>Kubeconfig file</span><div className="file-picker">
                <span className="button file-picker-button">Choose file<input ref={fileInput} type="file" aria-labelledby={`${formId}-file`} aria-describedby={`${formId}-file-name`} onChange={(event) => void chooseFile(event)} /></span>
                <small id={`${formId}-file-name`} className="file-picker-name">{fileLabel ? `Selected file: ${fileLabel}. Its content is not displayed.` : 'No file chosen'}</small>
              </div></div>
            : <label className="connection-field">Kubeconfig content<textarea className={hidden ? 'masked-text' : undefined} rows={8} spellCheck={false} autoComplete="off" value={kubeconfig} onChange={(event) => replaceDocument(event.target.value)} /></label>}
          <div className="source-actions"><Button type="button" disabled={busy || !kubeconfig.trim()} onClick={() => void inspect()}>{inspecting ? 'Inspecting…' : 'Inspect kubeconfig'}</Button>
            {source === 'paste' ? <label className="inline-check"><input type="checkbox" checked={!hidden} onChange={(event) => setHidden(!event.target.checked)} /> Show content</label> : null}</div>
        </div>
        {contexts.length > 0 ? <div className="connection-destination">
          {contexts.length > 1 ? <label className="connection-field">Context<select value={selected} required onChange={(event) => setSelected(event.target.value)}><option value="">Select a context</option>{contexts.map((context) => <option key={context.name} value={context.name}>{context.name}</option>)}</select></label> : null}
          {chosen ? <dl className="destination-summary" aria-label="Selected destination">
            <div><dt>Context</dt> <dd>{chosen.name}</dd></div>
            <div><dt>Cluster</dt> <dd>{chosen.cluster}</dd></div>
            <div><dt>Endpoint</dt> <dd>{chosen.endpoint}</dd></div>
          </dl> : null}
        </div> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}{notice ? <div className="form-success" role="status">{notice}</div> : null}
        <div className="connection-footer"><Button type="submit" tone="primary" disabled={busy}>{saving ? 'Checking…' : 'Check and save'}</Button></div>
      </fieldset></form>
    </div></section>
  </section>;
}

// Destination shows only the public fields that apply to the kind, so AWS
// records never carry empty Kubernetes columns.
function destination(connection: Connection): [string, string][] {
  const config = connection.config ?? {};
  const fields: [string, string | undefined][] = connection.kind === 'AWS'
    ? [['Region', config.region], ['Account', config.accountId]]
    : [['Cluster', config.cluster], ['Endpoint', config.endpoint]];
  return fields.filter((field): field is [string, string] => Boolean(field[1]));
}

function ConnectionTable({ connections }: { connections: Connection[] }) {
  return <table className="connection-table" aria-label="Registered connections">
    <thead><tr><th scope="col">Name</th><th scope="col">Type</th><th scope="col">Destination</th><th scope="col">Status</th></tr></thead>
    <tbody>{connections.map((connection) => {
      const label = connection.name || connection.key;
      const fields = destination(connection);
      return <tr key={connection.key}>
        <td data-label="Name"><strong>{label}</strong>{label !== connection.key ? <small className="connection-key">{connection.key}</small> : null}</td>
        <td data-label="Type">{connection.kind}</td>
        <td data-label="Destination">{fields.length ? fields.map(([field, value]) => <span className="destination-line" key={field}><span className="muted">{field}</span> {value}</span>) : <span className="muted">—</span>}</td>
        <td data-label="Status"><span className={`status-badge${connection.status === 'READY' ? ' status-badge-ready' : ''}`}>{connection.status}</span></td>
      </tr>;
    })}</tbody>
  </table>;
}
