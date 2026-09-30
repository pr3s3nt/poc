import { useState, type FormEvent } from 'react';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';

type FieldErrors = { username?: string; password?: string };

export function SignInPage({ onSuccess, notice }: { onSuccess(username: string, password: string): Promise<void>; notice?: string }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function submit(event: FormEvent) {
    event.preventDefault();
    const errors: FieldErrors = { username: username.trim() ? undefined : 'Enter your username.', password: password ? undefined : 'Enter your password.' };
    setFieldErrors(errors);
    if (errors.username || errors.password) return;
    setError(null);
    setSubmitting(true);
    // The password is handed to the request only and never kept after submit.
    const submitted = password;
    setPassword('');
    onSuccess(username, submitted).catch((err: unknown) => {
      setError(err instanceof ApiError && err.status === 401 ? 'Username or password is incorrect.' : 'Could not sign in. Check your connection and try again.');
      setSubmitting(false);
    });
  }

  return <div className="sign-in-page">
    <section className="sign-in-brand">
      <a className="wordmark wordmark-light" href="#sign-in"><span className="wordmark-mark">◆</span> Orchestrator</a>
      <p className="eyebrow eyebrow-light">Internal developer platform</p>
      <div className="brand-message"><h1>Build.<br />Deploy.<br />Move faster.</h1><p>Infrastructure that stays out of your way.</p></div>
      <div className="network-art" aria-hidden="true"><i /><i /><i /><i /><i /></div>
    </section>
    <section className="sign-in-form-wrap">
      <form className="sign-in-form" onSubmit={submit} noValidate>
        <span className="eyebrow">Welcome back</span>
        <h2>Sign in to your workspace</h2>
        <p>Sign in to create and deploy applications.</p>
        {notice && !error ? <div className="form-info" role="status">{notice}</div> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}
        <label>Username<input autoComplete="username" value={username} disabled={submitting} aria-invalid={Boolean(fieldErrors.username)} onChange={(event) => setUsername(event.target.value)} />{fieldErrors.username ? <span className="field-error">{fieldErrors.username}</span> : null}</label>
        <label>Password<input type="password" autoComplete="current-password" value={password} disabled={submitting} aria-invalid={Boolean(fieldErrors.password)} onChange={(event) => setPassword(event.target.value)} />{fieldErrors.password ? <span className="field-error">{fieldErrors.password}</span> : null}</label>
        <Button tone="primary" type="submit" disabled={submitting}>{submitting ? 'Signing in…' : 'Sign in'}</Button>
        <small>Use your local test account.</small>
      </form>
    </section>
  </div>;
}
