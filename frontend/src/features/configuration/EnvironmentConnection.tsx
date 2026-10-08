import { useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { toTarget, type APIEnvironment } from '../../shared/api/environment';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';

type Choice = { key: string; name: string; kind: string; status: string };
type ChoicesState = { status: 'loading' | 'error' } | { status: 'ready'; connections: readonly Choice[] };

export { toTarget };
const kindLabel = (kind?: string) => kind === 'AWS' ? 'AWS' : 'Kubernetes';

// Editable execution Connection of ONE Environment (UC-01 ES-01..06, ADR-012).
// Before runtime exists Save is a versioned metadata change; once resources
// exist a different destination is an explicit transition, never a silent Save.
export function EnvironmentConnection({ application, environment, onTargetChange, onStartTransition }: {
  application: Application; environment: EnvironmentKey;
  onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void;
  onStartTransition?(destinationKey: string): void;
}) {
  const target = application.environments[environment];
  const busy = Boolean(target.activeOperation);
  const [choices, setChoices] = useState<ChoicesState>({ status: 'loading' });
  const [selected, setSelected] = useState(target.connectionKey);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [attempt, setAttempt] = useState(0);
  // Every Environment switch, every choices (re)load and unmount bumps a
  // generation: replies started for an earlier one are ignored instead of
  // changing the current selection, list or message. A server-confirmed save
  // updates the Environment it was made for even after this view is gone.
  const generation = useRef(0);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; generation.current += 1; }; }, []);
  useEffect(() => { generation.current += 1; setSelected(application.environments[environment].connectionKey); setError(''); setNotice(''); setSaving(false); }, [application.id, environment]); // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const current = ++generation.current;
    setChoices({ status: 'loading' });
    api<{ connections: Choice[] }>('/application-connections')
      .then((response) => { if (current === generation.current) setChoices({ status: 'ready', connections: response.connections ?? [] }); })
      .catch(() => { if (current === generation.current) setChoices({ status: 'error' }); });
  }, [application.id, environment, attempt]);

  async function reloadEnvironment(current: number) {
    try {
      const response = await api<{ application: { environments?: APIEnvironment[] } }>(`/applications/${encodeURIComponent(application.id)}`);
      const item = response.application.environments?.find((env) => env.key === environment);
      if (item && (current === generation.current || !mounted.current)) {
        const latest = toTarget(item);
        onTargetChange(application.id, environment, latest);
        if (current === generation.current) setSelected((value) => value === target.connectionKey ? latest.connectionKey : value);
      }
    } catch { /* the notice already tells the user to reload */ }
  }

  async function save() {
    if (!selected || saving || busy) return;
    const current = ++generation.current;
    setSaving(true); setError(''); setNotice('');
    try {
      const response = await api<{ environment: APIEnvironment }>(`/applications/${encodeURIComponent(application.id)}/environments/${environment}/connection`, { method: 'PUT', body: JSON.stringify({ connectionKey: selected, expectedVersion: target.version }) });
      onTargetChange(application.id, environment, toTarget(response.environment));
      if (current === generation.current) setNotice(selected === target.connectionKey ? 'Already selected; nothing changed.' : 'Connection saved. Preview and Deploy now use it.');
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 409) {
        setError(err.message);
        await reloadEnvironment(current);
      } else if (err instanceof ApiError && err.status === 422) {
        setError('The selected connection is no longer available. Choose another connection.');
        setAttempt((value) => value + 1);
      } else {
        setError('Could not save the connection. Try again.');
      }
    } finally {
      if (current === generation.current) setSaving(false);
    }
  }

  const title = environment === 'staging' ? 'Staging' : 'Production';
  const ready = choices.status === 'ready' && choices.connections.length > 0;
  const changed = selected !== target.connectionKey;
  const needsTransition = target.configured && target.runtimeExists && changed;
  return <section className="content-panel environment-connection" aria-label="Execution connection">
    <div className="section-header"><div><h2>Deployment connection</h2><p>{target.configured ? `${title} deploys to this connection. You can change it; the change is versioned.` : `${title} has no connection yet, so it cannot be previewed or deployed. Variables and workload drafts can still be prepared.`}</p></div><span className="pending-pill">{target.configured ? `Generation ${target.targetGeneration}` : 'Not configured'}</span></div>
    {target.configured ? <dl className="connection-facts">
      <dt>Connection</dt><dd><strong>{target.connectionName || target.connectionKey}</strong> ({target.connectionKey})</dd>
      <dt>Kind</dt><dd>{kindLabel(target.connectionKind)}</dd>
      <dt>Execution profile</dt><dd>{target.profile}</dd>
      {target.region ? <><dt>Region</dt><dd>{target.region}</dd></> : null}
      <dt>Runtime status</dt><dd>{target.runtimeStatus}</dd>
      {target.infrastructureScope === 'LEGACY_APPLICATION' ? <><dt>Infrastructure</dt><dd>Application-scoped (migrated binding)</dd></> : null}
    </dl> : null}
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {notice ? <div className="form-success" role="status">{notice}</div> : null}
    {busy ? <div className="form-info" role="status">An operation is running on this environment. Settings are read-only until it finishes.</div> : null}
    {choices.status === 'loading' ? <p role="status" aria-busy="true">Loading connections…</p> : null}
    {choices.status === 'error' ? <div className="form-error" role="alert">Could not load connections.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
    {choices.status === 'ready' && choices.connections.length === 0 ? <div className="form-info" role="status">No READY connection is available. Ask a Platform Engineer to register one, then retry.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
    {ready ? <>
      <label htmlFor="environment-connection">Connection for {title}</label>
      <select id="environment-connection" value={selected} disabled={saving || busy} onChange={(event) => { setSelected(event.target.value); setNotice(''); }}>
        {!target.configured ? <option value="" disabled>Choose a connection</option> : null}
        {choices.connections.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}) · {kindLabel(item.kind)}</option>)}
      </select>
      {needsTransition
        ? <><small>This environment already has runtime resources. Changing its destination deploys a new generation, with an optional PostgreSQL data transfer, and keeps the old one for review.</small>
          <div className="form-actions"><Button tone="primary" disabled={busy} onClick={() => onStartTransition?.(selected)}>Review transition…</Button></div></>
        : <><small>{target.configured ? 'Saving a different connection is allowed only while no runtime resources exist.' : 'Choose a connection and save it. You can change it later.'}</small>
          <div className="form-actions"><Button tone="primary" disabled={!selected || saving || busy || (!changed && target.configured)} onClick={() => void save()}>{saving ? 'Saving connection…' : 'Save connection'}</Button></div></>}
    </> : null}
  </section>;
}
