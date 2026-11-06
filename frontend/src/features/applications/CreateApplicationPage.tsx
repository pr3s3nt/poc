import { useState, type FormEvent } from 'react';
import { navigate } from '../../app/routes';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { t } from '../../shared/i18n';

type FieldErrors = { name?: string };

const nameRequired = t('applications.createApplication.enterAnApplicationName');

function describeCreateError(err: unknown): { fields: FieldErrors; form?: string } {
  if (err instanceof ApiError && err.field === 'name') {
    if (err.code === 'DUPLICATE') return { fields: { name: t('applications.createApplication.thisApplicationNameAlreadyExists') } };
    if (err.code === 'INVALID_REQUEST') return { fields: { name: nameRequired } };
  }
  return { fields: {}, form: err instanceof ApiError && err.status !== 400 && err.status !== 409 ? t('applications.createApplication.couldNotCreateTheApplication', { message: err.message }) : t('applications.createApplication.couldNotCreateTheApplication2') };
}

// UC-01: Name only (ADR-037). Public addresses come from workload endpoints.
export function CreateApplicationPage({ onCreate }: { onCreate(name: string): Promise<string> }) {
  const [name, setName] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  function changeName(value: string) {
    setName(value);
  }
  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting) return;
    const errors: FieldErrors = {
      name: name.trim() ? undefined : nameRequired,
    };
    setFieldErrors(errors);
    setError(null);
    if (errors.name) return;
    setSubmitting(true);
    onCreate(name.trim())
      .then((id) => navigate({ name: 'application', applicationId: id }))
      .catch((err: unknown) => { const described = describeCreateError(err); setFieldErrors(described.fields); setError(described.form ?? null); setSubmitting(false); });
  }
  return <section className="page narrow-page"><button className="back-link" onClick={() => navigate({ name: 'applications' })}>{t('applications.createApplication.applications')}</button><header className="form-header"><p className="eyebrow">{t('applications.createApplication.newApplication')}</p><h1>{t('applications.createApplication.createApplication')}</h1><p>{t('applications.createApplication.theSystemCreatesTheStaging')}</p></header>
    <form className="create-form" aria-label={t('applications.createApplication.createApplication')} onSubmit={submit} noValidate>{error ? <div className="form-error" role="alert">{error}</div> : null}
      <label htmlFor="application-name">{t('applications.createApplication.applicationName')}</label><input id="application-name" placeholder={t('applications.createApplication.examplePayments')} value={name} disabled={submitting} aria-invalid={Boolean(fieldErrors.name)} onChange={(event) => changeName(event.target.value)} />{fieldErrors.name ? <span className="field-error">{fieldErrors.name}</span> : null}
      <div className="form-actions"><Button type="button" onClick={() => navigate({ name: 'applications' })}>{t('applications.createApplication.cancel')}</Button><Button tone="primary" type="submit" disabled={submitting}>{submitting ? t('applications.createApplication.creatingApplication') : t('applications.createApplication.createApplication')}</Button></div>
    </form>
  </section>;
}
