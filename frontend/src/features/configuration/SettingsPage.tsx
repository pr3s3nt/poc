import { useEffect, useState } from 'react';
import { navigate, replaceRoute } from '../../app/routes';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';
import { EnvironmentConnection } from './EnvironmentConnection';
import { OperationBanner } from '../environment/OperationBanner';
import { SecretStoreSelection } from '../environment/SecretStoreSelection';
import { TransitionPanel } from '../environment/TransitionPanel';
import { deleteKey, getConfiguration, putKey, renameKey, type ConfigKey, type Configuration, type KeyKind } from './api';

type Form = { kind: KeyKind; name: string; value: string; editing?: string };
type Action = { kind: 'rename' | 'delete'; key: ConfigKey; newName: string };

// 409 conflicts are shown with the server sentence; the page then reloads the
// authoritative configuration so the user resubmits against the latest version.
function describe(err: unknown): string {
  const message = (err as Error).message;
  return (err as { status?: number }).status === 409 ? `${message} The latest values were reloaded; review them and try again.` : message;
}

export function SettingsPage({ application, initialEnvironment = 'staging', onTargetChange }: { application: Application; initialEnvironment?: EnvironmentKey; onTargetChange?(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  const [environment, setEnvironment] = useState<EnvironmentKey>(initialEnvironment);
  const [data, setData] = useState<Configuration>();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [form, setForm] = useState<Form>();
  const [action, setAction] = useState<Action>();
  const [saving, setSaving] = useState(false);
  const [reloadCount, setReloadCount] = useState(0);
  const [transitionTo, setTransitionTo] = useState<string>();
  const [needStore, setNeedStore] = useState(false);
  // An active or interrupted Environment operation holds every configuration write
  // (the server answers 409 ENVIRONMENT_BUSY). Open editors keep their text and
  // simply cannot submit until the target no longer reports an operation.
  const busy = Boolean(application.environments[environment].activeOperation);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setData(undefined);
    setForm(undefined);
    setAction(undefined);
    setError('');
    setTransitionTo(undefined);
    setNeedStore(false);
    getConfiguration(application.id, environment).then((result) => { if (!cancelled) setData(result); }).catch((err: Error) => { if (!cancelled) setError(err.message); }).finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [application.id, environment, reloadCount]);

  async function saveForm() {
    if (!form || !data || busy) return;
    setSaving(true); setError('');
    try {
      const result = await putKey(application.id, environment, form.name.trim(), form.kind, form.value, data.version);
      setData(result); setForm(undefined);
    } catch (err) { setError(describe(err)); await refreshOn409(err); }
    finally { setSaving(false); }
  }

  // After a conflict the authoritative configuration replaces the stale copy
  // without clearing the message; the user reviews it and resubmits explicitly.
  async function refreshOn409(err: unknown) {
    if ((err as { status?: number }).status !== 409) return;
    try { setData(await getConfiguration(application.id, environment)); } catch { /* the message already says to retry */ }
  }

  async function confirmAction() {
    if (!action || !data || busy) return;
    setSaving(true); setError('');
    try {
      const result = action.kind === 'rename'
        ? await renameKey(application.id, environment, action.key.name, action.newName.trim(), data.version)
        : await deleteKey(application.id, environment, action.key.name, data.version);
      setData(result); setAction(undefined);
    } catch (err) { setError(describe(err)); await refreshOn409(err); }
    finally { setSaving(false); }
  }

  function section(kind: KeyKind, title: string) {
    const keys = data?.keys.filter((key) => key.kind === kind) ?? [];
    return <section className="content-panel" aria-label={title}>
      <div className="section-header"><div><h2>{title}</h2><p>{kind === 'SECRET' ? 'Values are hidden after saving.' : 'Reusable across workloads in this environment.'}</p></div><Button disabled={busy} onClick={() => { setAction(undefined); if (kind === 'SECRET' && !application.environments[environment].secretStoreKey) { setForm(undefined); setNeedStore(true); return; } setNeedStore(false); setForm({ kind, name: '', value: '' }); }}>+ Add {kind === 'SECRET' ? 'secret' : 'variable'}</Button></div>
      {keys.length === 0 ? <div className="section-empty">No {title.toLowerCase()} configured in {environment}.</div> : <div className="settings-table"><div className="settings-table-head"><span>Name</span><span>{kind === 'SECRET' ? 'Status' : 'Value'}</span><span>Used by</span><span>Actions</span></div>
        {keys.map((key) => <div className="settings-table-row" key={key.name}><strong>{key.name}</strong><span className={kind === 'SECRET' ? 'muted' : ''}>{kind === 'SECRET' ? 'Configured' : key.value}</span><span>{key.usedBy.length ? key.usedBy.join(', ') : '—'}</span><span className="row-actions"><Button tone="quiet" disabled={busy} onClick={() => { setAction(undefined); setForm({ kind, name: key.name, value: kind === 'SECRET' ? '' : key.value ?? '', editing: key.name }); }}>{kind === 'SECRET' ? 'Update' : 'Edit'}</Button><Button tone="quiet" disabled={busy} onClick={() => { setForm(undefined); setAction({ kind: 'rename', key, newName: key.name }); }}>Rename</Button><Button tone="danger" disabled={busy} onClick={() => { setForm(undefined); setAction({ kind: 'delete', key, newName: '' }); }}>Delete</Button></span></div>)}</div>}
    </section>;
  }

  const change = onTargetChange ?? (() => undefined);
  return <section className="page settings-page">
    <button className="back-link" onClick={() => navigate({ name: 'application', applicationId: application.id })}>← {application.name}</button>
    <header className="page-header application-header"><div><p className="eyebrow">Application settings</p><h1>Environment settings</h1><p>Choose and change each environment's deployment connection and secret store, and configure values separately for staging and production. Use Preview changes on the Application page to see what still needs deployment.</p></div>{data && data.version > 0 ? <span className="pending-pill">Desired revision v{data.version}</span> : null}</header>
    <div className="tabs" role="tablist" aria-label="Environment">{(['staging', 'production'] as const).map((env) => <button key={env} role="tab" aria-selected={env === environment} className={env === environment ? 'tab tab-active' : 'tab'} onClick={() => { setEnvironment(env); replaceRoute({ name: 'settings', applicationId: application.id, environment: env }); }}>{env === 'staging' ? 'Staging' : 'Production'}</button>)}</div>
    <OperationBanner application={application} environment={environment} onTargetChange={change} />
    <EnvironmentConnection application={application} environment={environment} onTargetChange={change} onStartTransition={(destination) => setTransitionTo(destination)} />
    {transitionTo !== undefined || application.environments[environment].runtimeExists ? <TransitionPanel application={application} environment={environment} initialDestination={transitionTo ?? ''} onTargetChange={change} /> : null}
    <SecretStoreSelection application={application} environment={environment} configVersion={data?.version ?? 0} onTargetChange={change} onConfigurationChanged={() => setReloadCount((value) => value + 1)} />
    {needStore ? <div className="form-info" role="status">Select a secret store above before adding a Secret. Environment variables can be added without one.</div> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {loading ? <p>Loading configuration…</p> : null}
    {!loading && data ? <>{section('VARIABLE', 'Environment variables')}{section('SECRET', 'Secrets')}</> : null}
    {form ? <div className="content-panel inline-editor"><div className="section-header"><h2>{form.editing ? (form.kind === 'SECRET' ? 'Replace secret' : 'Edit variable') : (form.kind === 'SECRET' ? 'Add secret' : 'Add variable')}</h2></div><label>Key name<input value={form.name} disabled={Boolean(form.editing)} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="API_URL" /></label><label>{form.kind === 'SECRET' ? 'New secret value' : 'Value'}<input type={form.kind === 'SECRET' ? 'password' : 'text'} autoComplete="off" value={form.value} onChange={(event) => setForm({ ...form, value: event.target.value })} /></label>{busy ? <p className="form-info" role="status">An environment operation is running. Your text is kept; submit again when it ends.</p> : null}<div className="form-actions"><Button onClick={() => setForm(undefined)}>Cancel</Button><Button tone="primary" disabled={busy || saving || !form.name.trim() || !form.value} onClick={saveForm}>Save pending change</Button></div></div> : null}
    {action ? <div className="content-panel inline-editor" role="dialog" aria-label={`${action.kind} ${action.key.name}`}><h2>{action.kind === 'rename' ? 'Rename' : 'Delete'} {action.key.name}?</h2>{action.key.usedBy.length ? <p className="warning-text">Referenced by {action.key.usedBy.join(', ')}. References will not be updated automatically; Preview will reject missing keys until you edit those workloads.</p> : <p>This changes {environment} only. Running workloads stay unchanged until Deploy.</p>}{action.kind === 'rename' ? <label>New key name<input value={action.newName} onChange={(event) => setAction({ ...action, newName: event.target.value })} /></label> : null}{busy ? <p className="form-info" role="status">An environment operation is running. Confirm again when it ends.</p> : null}<div className="form-actions"><Button onClick={() => setAction(undefined)}>Cancel</Button><Button tone={action.kind === 'delete' ? 'danger' : 'primary'} disabled={busy || saving || (action.kind === 'rename' && !action.newName.trim())} onClick={confirmAction}>Continue</Button></div></div> : null}
  </section>;
}
