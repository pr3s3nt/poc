import { useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { platformDomain } from '../../shared/types/application';
import { t } from '../../shared/i18n';

// Mirrors the backend DNS-label rule (UC-01 BR-03); the backend stays authoritative.
const subdomainPattern = /^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/;

type FieldErrors = { name?: string; subdomain?: string };

const nameRequired = t('applications.createApplication.enterAnApplicationName');
const subdomainRequired = t('applications.createApplication.enterASubdomain');
const subdomainInvalid = t('applications.createApplication.useOnlyLowercaseLettersWithout');

// Input aid only: the backend stays authoritative for the ID and for uniqueness.
export function suggestSubdomain(name: string): string {
  return name
    .normalize('NFD')
    .replace(/[\u0300-\u036f]/g, '')
    .replace(/[đĐ]/g, 'd')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+/, '')
    .slice(0, 63)
    .replace(/-+$/, '');
}

function describeCreateError(err: unknown): { fields: FieldErrors; form?: string } {
  if (err instanceof ApiError && (err.field === 'name' || err.field === 'subdomain')) {
    if (err.code === 'DUPLICATE') return { fields: err.field === 'name' ? { name: t('applications.createApplication.thisApplicationNameAlreadyExists') } : { subdomain: t('applications.createApplication.thisSubdomainIsAlreadyIn') } };
    if (err.code === 'INVALID_REQUEST') return { fields: err.field === 'name' ? { name: nameRequired } : { subdomain: subdomainInvalid } };
  }
  return { fields: {}, form: err instanceof ApiError && err.status !== 400 && err.status !== 409 ? t('applications.createApplication.couldNotCreateTheApplication', { message: err.message }) : t('applications.createApplication.couldNotCreateTheApplication2') };
}

// UC-01: only Name and Subdomain. Each Environment chooses its own execution
// connection later, in Environment Settings (ADR-011).
export function CreateApplicationPage({ onCreate }: { onCreate(name: string, subdomain: string): Promise<string> }) {
  const [name, setName] = useState('');
  const [subdomain, setSubdomain] = useState('');
  const [subdomainEdited, setSubdomainEdited] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const normalized = subdomain.trim().toLowerCase();
  function changeName(value: string) {
    setName(value);
    if (!subdomainEdited) setSubdomain(suggestSubdomain(value));
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting) return;
    const errors: FieldErrors = {
      name: name.trim() ? undefined : nameRequired,
      subdomain: !normalized ? subdomainRequired : subdomainPattern.test(normalized) ? undefined : subdomainInvalid,
    };
    setFieldErrors(errors);
    setError(null);
    if (errors.name || errors.subdomain) return;
    setSubmitting(true);
    onCreate(name.trim(), normalized)
      .then((id) => navigate({ name: 'application', applicationId: id }))
      .catch((err: unknown) => { const described = describeCreateError(err); setFieldErrors(described.fields); setError(described.form ?? null); setSubmitting(false); });
  }
  const host = normalized || 'ten-ung-dung';
  return <section className="page narrow-page"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>{t('applications.createApplication.applications')}</button><header className="form-header"><p className="eyebrow">{t('applications.createApplication.newApplication')}</p><h1>{t('applications.createApplication.createApplication')}</h1><p>{t('applications.createApplication.theSystemCreatesTheStaging')}</p></header>
    <form className="create-form" aria-label={t('applications.createApplication.createApplication')} onSubmit={submit} noValidate>{error ? <div className="form-error" role="alert">{error}</div> : null}
      <label htmlFor="application-name">{t('applications.createApplication.applicationName')}</label><input id="application-name" placeholder={t('applications.createApplication.examplePayments')} value={name} disabled={submitting} aria-invalid={Boolean(fieldErrors.name)} onChange={(event) => changeName(event.target.value)} />{fieldErrors.name ? <span className="field-error">{fieldErrors.name}</span> : null}
      <label htmlFor="application-subdomain">Subdomain</label><input id="application-subdomain" placeholder={t('applications.createApplication.examplePayments2')} value={subdomain} disabled={submitting} aria-invalid={Boolean(fieldErrors.subdomain)} onChange={(event) => { setSubdomainEdited(true); setSubdomain(event.target.value); }} />{fieldErrors.subdomain ? <span className="field-error">{fieldErrors.subdomain}</span> : null}<small>{t('applications.createApplication.suggestedFromTheApplicationName')}</small>
      <div className="url-preview" role="group" aria-label={t('applications.createApplication.endpointToBeCreated')}><span>{t('applications.createApplication.endpointToBeCreated')}</span><strong>staging.{host}.{platformDomain}</strong><strong>{host}.{platformDomain}</strong></div>
      <div className="form-actions"><Button type="button" onClick={() => navigate({ name: 'applications' })}>{t('applications.createApplication.cancel')}</Button><Button tone="primary" type="submit" disabled={submitting}>{submitting ? t('applications.createApplication.creatingApplication') : t('applications.createApplication.createApplication')}</Button></div>
    </form>
  </section>;
}
