import { useState, type FormEvent } from 'react';
import { Button } from '../../shared/ui/Button';

export function SignInPage({ onSuccess }: { onSuccess(username: string, password: string): Promise<void> }) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!username.trim() || !password) { setError('Enter your username and password.'); return; }
    setError(null);
	setSubmitting(true);
	onSuccess(username, password).catch(() => { setError('Invalid username or password.'); setSubmitting(false); });
  }

  return <div className="sign-in-page">
    <section className="sign-in-brand">
      <a className="wordmark wordmark-light" href="#sign-in"><span className="wordmark-mark">◆</span> Orchestrator</a>
      <p className="eyebrow eyebrow-light">Internal developer platform</p>
      <div className="brand-message"><h1>Build.<br />Deploy.<br />Move faster.</h1><p>Infrastructure that stays out of your way.</p></div>
      <div className="network-art" aria-hidden="true"><i /><i /><i /><i /><i /></div>
    </section>
    <section className="sign-in-form-wrap">
      <form className="sign-in-form" onSubmit={submit}>
        <span className="eyebrow">Welcome back</span>
        <h2>Sign in to your workspace</h2>
        <p>Use your local test account to create and deploy applications.</p>
        {error ? <div className="form-error" role="alert">{error}</div> : null}
        <label>Username<input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} /></label>
        <label>Password<input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} /></label>
        <Button tone="primary" type="submit" disabled={submitting}>{submitting ? 'Signing in…' : 'Sign in'}</Button>
        <small>Use the fixed local test account configured for this environment.</small>
      </form>
    </section>
  </div>;
}
