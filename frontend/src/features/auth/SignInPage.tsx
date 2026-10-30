import { useState, type FormEvent } from 'react';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { LanguageSelect } from '../../shared/ui/LanguageSelect';
import { PasswordInput } from '../../shared/ui/PasswordInput';
import { t } from '../../shared/i18n';

type FieldErrors = { username?: string; password?: string };

export function SignInPage({ onSuccess, notice }: { onSuccess(username: string, password: string): Promise<void>; notice?: string }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function submit(event: FormEvent) {
    event.preventDefault();
    const errors: FieldErrors = { username: username.trim() ? undefined : t('auth.signIn.enterYourUsername'), password: password ? undefined : t('auth.signIn.enterYourPassword') };
    setFieldErrors(errors);
    if (errors.username || errors.password) return;
    setError(null);
    setSubmitting(true);
    // The password is handed to the request only and never kept after submit.
    const submitted = password;
    setPassword('');
    onSuccess(username, submitted).catch((err: unknown) => {
      setError(err instanceof ApiError && err.code === 'INVALID_CREDENTIALS' ? t('auth.signIn.incorrectUsernameOrPassword') : t('auth.signIn.couldNotSignInCheck'));
      setSubmitting(false);
    });
  }

  return <div className="sign-in-page">
    <section className="sign-in-brand">
      <a className="wordmark wordmark-light" href="#sign-in"><span className="wordmark-mark">◆</span> Orchestrator</a>
      <p className="eyebrow eyebrow-light">{t('auth.signIn.internalDeveloperPlatform')}</p>
      <div className="brand-message"><h1>{t('auth.signIn.build')}<br />{t('auth.signIn.deploy')}<br />{t('auth.signIn.workFaster')}</h1><p>{t('auth.signIn.infrastructureThatLetsYouFocus')}</p></div>
      <div className="network-art" aria-hidden="true"><i /><i /><i /><i /><i /></div>
    </section>
    <section className="sign-in-form-wrap">
      <form className="sign-in-form" onSubmit={submit} noValidate>
        <span className="eyebrow">{t('auth.signIn.welcomeBack')}</span>
        <h2>{t('auth.signIn.signInToYourWorkspace')}</h2>
        <p>{t('auth.signIn.signInToCreateAnd')}</p>
        {notice && !error ? <div className="form-info" role="status">{notice}</div> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}
        <label>{t('auth.signIn.username')}<input autoComplete="username" value={username} disabled={submitting} aria-invalid={Boolean(fieldErrors.username)} onChange={(event) => setUsername(event.target.value)} />{fieldErrors.username ? <span className="field-error">{fieldErrors.username}</span> : null}</label>
        <label>{t('auth.signIn.password')}<PasswordInput autoComplete="current-password" value={password} disabled={submitting} aria-invalid={Boolean(fieldErrors.password)} onChange={(event) => setPassword(event.target.value)} />{fieldErrors.password ? <span className="field-error">{fieldErrors.password}</span> : null}</label>
        <Button tone="primary" type="submit" disabled={submitting}>{submitting ? t('auth.signIn.signingIn') : t('auth.signIn.signIn')}</Button>
        <small>{t('auth.signIn.useYourLocalTestAccount')}</small>
        <LanguageSelect />
      </form>
    </section>
  </div>;
}
