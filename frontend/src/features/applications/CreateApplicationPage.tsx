import { useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { platformDomain } from '../../shared/types/application';

// Mirrors the backend DNS-label rule (UC-01 BR-03); the backend stays authoritative.
const subdomainPattern = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

type FieldErrors = { name?: string; subdomain?: string };

function describeCreateError(err: unknown): { fields: FieldErrors; form?: string } {
  if (err instanceof ApiError && (err.field === 'name' || err.field === 'subdomain')) {
    if (err.status === 409) return { fields: err.field === 'name' ? { name: 'An application with this name already exists.' } : { subdomain: 'This subdomain is already in use.' } };
    if (err.status === 400) return { fields: err.field === 'name' ? { name: 'Enter an application name.' } : { subdomain: 'Use lowercase letters, numbers and hyphens; start and end with a letter or number.' } };
  }
  if (err instanceof ApiError && err.status === 422) return { fields: {}, form: 'The platform default target is not ready. Contact your platform engineer.' };
  return { fields: {}, form: 'Could not create the application. Try again.' };
}

export function CreateApplicationPage({ onCreate }: { onCreate(name: string, subdomain: string): Promise<string> }) {
  const [name, setName] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const normalized = subdomain.trim().toLowerCase();
  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting) return;
    const errors: FieldErrors = {
      name: name.trim() ? undefined : 'Enter an application name.',
      subdomain: !normalized ? 'Enter a subdomain.' : subdomainPattern.test(normalized) ? undefined : 'Use lowercase letters, numbers and hyphens; start and end with a letter or number.',
    };
    setFieldErrors(errors);
    setError(null);
    if (errors.name || errors.subdomain) return;
    setSubmitting(true);
    onCreate(name.trim(), normalized)
      .then((id) => navigate({ name: 'application', applicationId: id }))
      .catch((err: unknown) => { const described = describeCreateError(err); setFieldErrors(described.fields); setError(described.form ?? null); setSubmitting(false); });
  }
  return <section className="page narrow-page"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>← Applications</button><header className="form-header"><p className="eyebrow">New application</p><h1>Create an application</h1><p>We will create staging and production environments for you.</p></header>
    <form className="create-form" onSubmit={submit} noValidate>{error ? <div className="form-error" role="alert">{error}</div> : null}
      <label htmlFor="application-name">Application name</label><input id="application-name" placeholder="e.g. Payment" value={name} disabled={submitting} aria-invalid={Boolean(fieldErrors.name)} onChange={(event) => setName(event.target.value)} />{fieldErrors.name ? <span className="field-error">{fieldErrors.name}</span> : null}
      <label htmlFor="application-subdomain">Subdomain</label><input id="application-subdomain" placeholder="e.g. payment" value={subdomain} disabled={submitting} aria-invalid={Boolean(fieldErrors.subdomain)} onChange={(event) => setSubdomain(event.target.value)} />{fieldErrors.subdomain ? <span className="field-error">{fieldErrors.subdomain}</span> : null}<small>Lowercase letters, numbers and hyphens only.</small>
      <div className="url-preview"><span>Endpoints created for you</span><strong>staging.{normalized || 'your-app'}.{platformDomain}</strong><strong>{normalized || 'your-app'}.{platformDomain}</strong></div>
      <div className="form-actions"><Button type="button" onClick={() => navigate({ name: 'applications' })}>Cancel</Button><Button tone="primary" type="submit" disabled={submitting}>{submitting ? 'Creating application…' : 'Create application'}</Button></div>
    </form>
  </section>;
}
