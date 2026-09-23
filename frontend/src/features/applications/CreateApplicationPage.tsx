import { useMemo, useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { Button } from '../../shared/ui/Button';
import { platformDomain } from '../../shared/types/application';

export function CreateApplicationPage({ onCreate }: { onCreate(name: string, subdomain: string): Promise<string> }) {
  const [name, setName] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [error, setError] = useState<string | null>(null);
  const normalized = useMemo(() => subdomain.toLowerCase().replace(/[^a-z0-9-]/g, ''), [subdomain]);
  function submit(event: FormEvent) {
    event.preventDefault();
    if (!name.trim() || !normalized) { setError('Application name and a valid subdomain are required.'); return; }
	onCreate(name.trim(), normalized).then((id) => navigate({ name: 'application', applicationId: id })).catch(() => setError('Could not create the application.'));
  }
  return <section className="page narrow-page"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>← Applications</button><header className="form-header"><p className="eyebrow">New application</p><h1>Create an application</h1><p>We will create staging and production environments for you.</p></header>
    <form className="create-form" onSubmit={submit}>{error ? <div className="form-error" role="alert">{error}</div> : null}
      <label htmlFor="application-name">Application name</label><input id="application-name" placeholder="e.g. Payment" value={name} onChange={(event) => setName(event.target.value)} />
      <label htmlFor="application-subdomain">Subdomain</label><input id="application-subdomain" placeholder="e.g. payment" value={subdomain} onChange={(event) => setSubdomain(event.target.value)} /><small>Lowercase letters, numbers and hyphens only.</small>
      <div className="url-preview"><span>Endpoints created for you</span><strong>staging.{normalized || 'your-app'}.{platformDomain}</strong><strong>{normalized || 'your-app'}.{platformDomain}</strong></div>
      <div className="form-actions"><Button type="button" onClick={() => navigate({ name: 'applications' })}>Cancel</Button><Button tone="primary" type="submit">Create application</Button></div>
    </form>
  </section>;
}
