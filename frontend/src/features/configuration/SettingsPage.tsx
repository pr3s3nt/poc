import { useEffect, useState } from 'react';
import { navigate } from '../../app/routes';
import type { Application, EnvironmentKey } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { deleteKey, getConfiguration, putKey, renameKey, type ConfigKey, type Configuration, type KeyKind } from './api';

type Form = { kind: KeyKind; name: string; value: string; editing?: string };
type Action = { kind: 'rename' | 'delete'; key: ConfigKey; newName: string };

export function SettingsPage({ application }: { application: Application }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>('staging');
  const [data, setData] = useState<Configuration>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [form, setForm] = useState<Form>();
  const [action, setAction] = useState<Action>();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setData(undefined);
    setForm(undefined);
    setAction(undefined);
    setError('');
    getConfiguration(application.id, environment).then((result) => { if (!cancelled) setData(result); }).catch((err: Error) => { if (!cancelled) setError(err.message); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment]);

  async function saveForm() {
    if (!form || !data) return;
    setSaving(true); setError('');
    try {
      const result = await putKey(application.id, environment, form.name.trim(), form.kind, form.value, data.version);
      setData(result); setForm(undefined);
    } catch (err) { setError((err as Error).message); }
    finally { setSaving(false); }
  }

  async function confirmAction() {
    if (!action || !data) return;
    setSaving(true); setError('');
    try {
      const result = action.kind === 'rename'
        ? await renameKey(application.id, environment, action.key.name, action.newName.trim(), data.version)
        : await deleteKey(application.id, environment, action.key.name, data.version);
      setData(result); setAction(undefined);
    } catch (err) { setError((err as Error).message); }
    finally { setSaving(false); }
  }

  function section(kind: KeyKind, title: string) {
    const keys = data?.keys.filter((key) => key.kind === kind) ?? [];
    return <section className="content-panel" aria-label={title}>
      <div className="section-header"><div><h2>{title}</h2><p>{kind === 'SECRET' ? 'Values are hidden after saving.' : 'Reusable across workloads in this environment.'}</p></div><Button onClick={() => { setAction(undefined); setForm({ kind, name: '', value: '' }); }}>+ Add {kind === 'SECRET' ? 'secret' : 'variable'}</Button></div>
      {keys.length === 0 ? <div className="section-empty">No {title.toLowerCase()} configured in {environment}.</div> : <div className="settings-table"><div className="settings-table-head"><span>Name</span><span>{kind === 'SECRET' ? 'Status' : 'Value'}</span><span>Used by</span><span>Actions</span></div>
        {keys.map((key) => <div className="settings-table-row" key={key.name}><strong>{key.name}</strong><span className={kind === 'SECRET' ? 'muted' : ''}>{kind === 'SECRET' ? 'Configured' : key.value}</span><span>{key.usedBy.length ? key.usedBy.join(', ') : '—'}</span><span className="row-actions"><Button tone="quiet" onClick={() => { setAction(undefined); setForm({ kind, name: key.name, value: kind === 'SECRET' ? '' : key.value ?? '', editing: key.name }); }}>{kind === 'SECRET' ? 'Update' : 'Edit'}</Button><Button tone="quiet" onClick={() => { setForm(undefined); setAction({ kind: 'rename', key, newName: key.name }); }}>Rename</Button><Button tone="danger" onClick={() => { setForm(undefined); setAction({ kind: 'delete', key, newName: '' }); }}>Delete</Button></span></div>)}</div>}
    </section>;
  }

  return <section className="page settings-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header application-header"><div><p className="eyebrow">Application settings</p><h1>Variables &amp; Secrets</h1><p>Configure values separately for staging and production. Use Preview changes on the Application page to see what still needs deployment.</p></div>{data && data.version > 0 ? <span className="pending-pill">Desired revision v{data.version}</span> : null}</header>
    <div className="tabs" role="tablist" aria-label="Environment">{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={env === environment} className={env === environment ? 'tab tab-active' : 'tab'} onClick={() => setEnvironment(env)}>{env === 'staging' ? 'Staging' : 'Production'}</button>)}</div>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {loading ? <p>Loading configuration…</p> : null}
    {!loading && data ? <>{section('VARIABLE', 'Environment variables')}{section('SECRET', 'Secrets')}</> : null}
    {form ? <div className="content-panel inline-editor"><div className="section-header"><h2>{form.editing ? (form.kind === 'SECRET' ? 'Replace secret' : 'Edit variable') : (form.kind === 'SECRET' ? 'Add secret' : 'Add variable')}</h2></div><label>Key name<input value={form.name} disabled={Boolean(form.editing)} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="API_URL" /></label><label>{form.kind === 'SECRET' ? 'New secret value' : 'Value'}<input type={form.kind === 'SECRET' ? 'password' : 'text'} autoComplete="off" value={form.value} onChange={(event) => setForm({ ...form, value: event.target.value })} /></label><div className="form-actions"><Button onClick={() => setForm(undefined)}>Cancel</Button><Button tone="primary" disabled={saving || !form.name.trim() || !form.value} onClick={saveForm}>Save pending change</Button></div></div> : null}
    {action ? <div className="content-panel inline-editor" role="dialog" aria-label={`${action.kind} ${action.key.name}`}><h2>{action.kind === 'rename' ? 'Rename' : 'Delete'} {action.key.name}?</h2>{action.key.usedBy.length ? <p className="warning-text">Referenced by {action.key.usedBy.join(', ')}. References will not be updated automatically; Preview will reject missing keys until you edit those workloads.</p> : <p>This changes {environment} only. Running workloads stay unchanged until Deploy.</p>}{action.kind === 'rename' ? <label>New key name<input value={action.newName} onChange={(event) => setAction({ ...action, newName: event.target.value })} /></label> : null}<div className="form-actions"><Button onClick={() => setAction(undefined)}>Cancel</Button><Button tone={action.kind === 'delete' ? 'danger' : 'primary'} disabled={saving || (action.kind === 'rename' && !action.newName.trim())} onClick={confirmAction}>Continue</Button></div></div> : null}
  </section>;
}
