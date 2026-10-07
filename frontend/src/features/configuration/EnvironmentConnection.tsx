import { useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../../shared/api/client';
import type { Application, EnvironmentKey, EnvironmentTarget } from '../../shared/types/application';
import { Button } from '../../shared/ui/Button';

type Choice = { key: string; name: string; kind: string; status: string };
type ChoicesState = { status: 'loading' | 'error' } | { status: 'ready'; connections: readonly Choice[]; defaultKey: string };
type APIEnvironment = { key: string; version: number; configured?: boolean; connectionKey?: string; connectionName?: string; connectionKind?: string; executionProfile?: string; region?: string; runtimeStatus?: string; infrastructureScope?: string };

export function toTarget(item: APIEnvironment): EnvironmentTarget {
  return { configured: Boolean(item.configured), connectionKey: item.connectionKey ?? '', connectionName: item.connectionName, connectionKind: item.connectionKind, profile: item.executionProfile ?? '', region: item.region, runtimeStatus: item.runtimeStatus ?? 'UNCONFIGURED', infrastructureScope: item.infrastructureScope ?? 'ENVIRONMENT', version: item.version };
}

const kindLabel = (kind?: string) => kind === 'AWS' ? 'AWS' : 'Kubernetes';

// Set-once execution Connection of ONE Environment (UC-01 ES-01..06, ADR-011).
// Unconfigured: an explicit choice and `Set connection`. Configured: read-only.
export function EnvironmentConnection({ application, environment, onTargetChange }: { application: Application; environment: EnvironmentKey; onTargetChange(applicationId: string, environment: EnvironmentKey, target: EnvironmentTarget): void }) {
  const target = application.environments[environment];
  const [choices, setChoices] = useState<ChoicesState>({ status: 'loading' });
  const [selected, setSelected] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const [attempt, setAttempt] = useState(0);
  // Every Environment switch, every choices (re)load and unmount bumps a
  // generation: replies started for an earlier one are ignored instead of
  // changing the current selection, list or message. A server-confirmed set is
  // the exception for the cache only: it updates the Environment it was made
  // for (never the one currently shown) even after this view is gone.
  const generation = useRef(0);
  const mounted = useRef(true);
  useEffect(() => { mounted.current = true; return () => { mounted.current = false; generation.current += 1; }; }, []);

  useEffect(() => { generation.current += 1; setSelected(''); setError(''); setSaving(false); }, [application.id, environment]);
  useEffect(() => {
    if (target.configured) return;
    const current = ++generation.current;
    setChoices({ status: 'loading' });
    api<{ connections: Choice[]; defaultConnectionKey?: string }>('/application-connections')
      .then((response) => {
        if (current !== generation.current) return;
        const connections = response.connections ?? [];
        setChoices({ status: 'ready', connections, defaultKey: connections.some((item) => item.key === response.defaultConnectionKey) ? response.defaultConnectionKey ?? '' : '' });
        setSelected((value) => connections.some((item) => item.key === value) ? value : '');
      })
      .catch(() => { if (current === generation.current) setChoices({ status: 'error' }); });
  }, [application.id, environment, target.configured, attempt]);

  // The authoritative Environment replaces the cached one (stale UI or a set that raced elsewhere).
  async function reloadEnvironment(current: number) {
    try {
      const response = await api<{ application: { environments?: APIEnvironment[] } }>(`/applications/${encodeURIComponent(application.id)}`);
      const item = response.application.environments?.find((env) => env.key === environment);
      if (item && (current === generation.current || !mounted.current)) onTargetChange(application.id, environment, toTarget(item));
    } catch { /* the notice below already tells the user to reload */ }
  }

  async function setConnection() {
    if (!selected || saving) return;
    const current = ++generation.current;
    setSaving(true); setError('');
    try {
      const response = await api<{ environment: APIEnvironment }>(`/applications/${encodeURIComponent(application.id)}/environments/${environment}/connection`, { method: 'PUT', body: JSON.stringify({ connectionKey: selected, expectedVersion: target.version }) });
      // Applied to the Environment this request was made for, whatever is shown now.
      onTargetChange(application.id, environment, toTarget(response.environment));
    } catch (err) {
      if (current !== generation.current) return;
      if (err instanceof ApiError && err.status === 409) {
        setError(err.message);
        await reloadEnvironment(current);
      } else if (err instanceof ApiError && err.status === 422) {
        setError('The selected connection is no longer available. Choose another connection.');
        setAttempt((value) => value + 1);
      } else {
        setError('Could not set the connection. Try again.');
      }
    } finally {
      if (current === generation.current) setSaving(false);
    }
  }

  const title = environment === 'staging' ? 'Staging' : 'Production';
  if (target.configured) {
    return <section className="content-panel environment-connection" aria-label="Execution connection">
      <div className="section-header"><div><h2>Execution connection</h2><p>{title} deploys to this connection. It was set once and is locked.</p></div><span className="pending-pill">Locked</span></div>
      <dl className="connection-facts">
        <dt>Connection</dt><dd><strong>{target.connectionName || target.connectionKey}</strong> ({target.connectionKey})</dd>
        <dt>Kind</dt><dd>{kindLabel(target.connectionKind)}</dd>
        <dt>Execution profile</dt><dd>{target.profile}</dd>
        {target.region ? <><dt>Region</dt><dd>{target.region}</dd></> : null}
        <dt>Runtime status</dt><dd>{target.runtimeStatus}</dd>
        {target.infrastructureScope === 'LEGACY_APPLICATION' ? <><dt>Infrastructure</dt><dd>Application-scoped (migrated binding)</dd></> : null}
      </dl>
    </section>;
  }
  const ready = choices.status === 'ready' && choices.connections.length > 0;
  return <section className="content-panel environment-connection" aria-label="Execution connection">
    <div className="section-header"><div><h2>Execution connection</h2><p>{title} has no connection yet, so it cannot be previewed or deployed. Variables, secrets and workload drafts can still be prepared.</p></div><span className="pending-pill">Not configured</span></div>
    {error ? <div className="form-error" role="alert">{error}</div> : null}
    {choices.status === 'loading' ? <p role="status" aria-busy="true">Loading connections…</p> : null}
    {choices.status === 'error' ? <div className="form-error" role="alert">Could not load connections.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
    {choices.status === 'ready' && choices.connections.length === 0 ? <div className="form-info" role="status">No READY connection is available. Ask a Platform Engineer to register one, then retry.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
    {ready ? <>
      <label htmlFor="environment-connection">Connection for {title}</label>
      <select id="environment-connection" value={selected} disabled={saving} onChange={(event) => setSelected(event.target.value)}><option value="" disabled>Choose a connection</option>{choices.connections.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}) · {kindLabel(item.kind)}{item.key === choices.defaultKey ? ' · default' : ''}</option>)}</select>
      <small>You can set this once. It cannot be changed afterwards, even before the first deployment.</small>
      <div className="form-actions"><Button tone="primary" disabled={!selected || saving} onClick={() => void setConnection()}>{saving ? 'Setting connection…' : 'Set connection'}</Button></div>
    </> : null}
  </section>;
}
