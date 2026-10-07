import { useEffect, useRef, useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { platformDomain } from '../../shared/types/application';

// Mirrors the backend DNS-label rule (UC-01 BR-03); the backend stays authoritative.
const subdomainPattern = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

type FieldErrors = { name?: string; subdomain?: string; connection?: string };

type ConnectionChoice = { key: string; name: string; kind: string; status: string };
type ChoicesResponse = { connections: ConnectionChoice[]; defaultConnectionKey?: string };
type ChoicesState = { status: 'loading' | 'error' } | { status: 'ready'; connections: readonly ConnectionChoice[]; defaultKey: string };

const connectionUnavailable = 'The selected connection is no longer available. Choose another connection.';

function describeCreateError(err: unknown): { fields: FieldErrors; form?: string } {
  if (err instanceof ApiError && (err.field === 'name' || err.field === 'subdomain')) {
    if (err.status === 409) return { fields: err.field === 'name' ? { name: 'An application with this name already exists.' } : { subdomain: 'This subdomain is already in use.' } };
    if (err.status === 400) return { fields: err.field === 'name' ? { name: 'Enter an application name.' } : { subdomain: 'Use lowercase letters, numbers and hyphens; start and end with a letter or number.' } };
  }
  if (err instanceof ApiError && err.field === 'connectionKey' && (err.status === 422 || err.status === 400)) return { fields: { connection: connectionUnavailable } };
  if (err instanceof ApiError && err.status === 422) return { fields: {}, form: 'The selected connection is not available. Choose another connection.' };
  return { fields: {}, form: 'Could not create the application. Try again.' };
}

export function CreateApplicationPage({ onCreate }: { onCreate(name: string, subdomain: string, connectionKey: string): Promise<string> }) {
  const [name, setName] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [choices, setChoices] = useState<ChoicesState>({ status: 'loading' });
  const [selected, setSelected] = useState('');
  const [attempt, setAttempt] = useState(0);
  // The default is preselected once. After that a choice that disappears is
  // cleared, never replaced by the default (UC-01 UI states).
  const selectedRef = useRef('');
  const defaultApplied = useRef(false);
  selectedRef.current = selected;
  // Each load owns its result; an older or unmounted reply is ignored. Name,
  // Subdomain and a still-valid selection survive retries and reloads.
  useEffect(() => {
    let current = true;
    setChoices({ status: 'loading' });
    api<ChoicesResponse>('/application-connections')
      .then((response) => {
        if (!current) return;
        const connections = response.connections ?? [];
        const defaultKey = connections.some((item) => item.key === response.defaultConnectionKey) ? response.defaultConnectionKey ?? '' : '';
        setChoices({ status: 'ready', connections, defaultKey });
        const kept = connections.some((item) => item.key === selectedRef.current) ? selectedRef.current : '';
        setSelected(kept || (defaultApplied.current ? '' : defaultKey));
        defaultApplied.current = true;
      })
      .catch(() => { if (current) setChoices({ status: 'error' }); });
    return () => { current = false; };
  }, [attempt]);
  const ready = choices.status === 'ready' && choices.connections.length > 0;
  const normalized = subdomain.trim().toLowerCase();
  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting || !ready) return;
    const errors: FieldErrors = {
      name: name.trim() ? undefined : 'Enter an application name.',
      connection: selected ? undefined : 'Choose a connection.',
      subdomain: !normalized ? 'Enter a subdomain.' : subdomainPattern.test(normalized) ? undefined : 'Use lowercase letters, numbers and hyphens; start and end with a letter or number.',
    };
    setFieldErrors(errors);
    setError(null);
    if (errors.name || errors.subdomain || errors.connection) return;
    setSubmitting(true);
    onCreate(name.trim(), normalized, selected)
      .then((id) => navigate({ name: 'application', applicationId: id }))
      .catch((err: unknown) => { const described = describeCreateError(err); setFieldErrors(described.fields); setError(described.form ?? null); setSubmitting(false); if (described.fields.connection || (err instanceof ApiError && err.status === 422)) setAttempt((value) => value + 1); });
  }
  return <section className="page narrow-page"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>← Applications</button><header className="form-header"><p className="eyebrow">New application</p><h1>Create an application</h1><p>We will create staging and production environments for you.</p></header>
    <form className="create-form" onSubmit={submit} noValidate>{error ? <div className="form-error" role="alert">{error}</div> : null}
      <label htmlFor="application-name">Application name</label><input id="application-name" placeholder="e.g. Payment" value={name} disabled={submitting} aria-invalid={Boolean(fieldErrors.name)} onChange={(event) => setName(event.target.value)} />{fieldErrors.name ? <span className="field-error">{fieldErrors.name}</span> : null}
      <label htmlFor="application-subdomain">Subdomain</label><input id="application-subdomain" placeholder="e.g. payment" value={subdomain} disabled={submitting} aria-invalid={Boolean(fieldErrors.subdomain)} onChange={(event) => setSubdomain(event.target.value)} />{fieldErrors.subdomain ? <span className="field-error">{fieldErrors.subdomain}</span> : null}<small>Lowercase letters, numbers and hyphens only.</small>
      <label htmlFor="application-connection">Connection</label>
      {choices.status === 'loading' ? <p role="status" aria-busy="true">Loading connections…</p> : null}
      {choices.status === 'error' ? <div className="form-error" role="alert">Could not load connections.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
      {choices.status === 'ready' && choices.connections.length === 0 ? <div className="form-info" role="status">No READY connection is available. Ask a Platform Engineer to register one, then retry.<Button type="button" onClick={() => setAttempt((value) => value + 1)}>Retry</Button></div> : null}
      {ready ? <select id="application-connection" value={selected} disabled={submitting} aria-invalid={Boolean(fieldErrors.connection)} onChange={(event) => setSelected(event.target.value)}>{selected ? null : <option value="" disabled>Choose a connection</option>}{choices.connections.map((item) => <option key={item.key} value={item.key}>{item.name} ({item.key}) · {item.kind === 'AWS' ? 'AWS' : 'Kubernetes'}{item.key === choices.defaultKey ? ' · default' : ''}</option>)}</select> : null}{fieldErrors.connection ? <span className="field-error">{fieldErrors.connection}</span> : null}<small>Applies to staging and production and cannot be changed after creation.</small>
      <div className="url-preview"><span>Endpoints created for you</span><strong>staging.{normalized || 'your-app'}.{platformDomain}</strong><strong>{normalized || 'your-app'}.{platformDomain}</strong></div>
      <div className="form-actions"><Button type="button" onClick={() => navigate({ name: 'applications' })}>Cancel</Button><Button tone="primary" type="submit" disabled={submitting || !ready || !selected}>{submitting ? 'Creating application…' : 'Create application'}</Button></div>
    </form>
  </section>;
}
