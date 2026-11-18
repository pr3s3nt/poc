import { useState, type FormEvent } from 'react';
import { api, describeError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { PasswordInput } from '../../shared/ui/PasswordInput';
import { LanguageSelect } from '../../shared/ui/LanguageSelect';
import { t } from '../../shared/i18n';

export function InitializePage({ onComplete }: { onComplete(): void }) {
  const [draft, setDraft] = useState({ token: '', organizationKey: '', organizationName: '', username: '', password: '', confirmPassword: '' });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (draft.password !== draft.confirmPassword) { setError(t('setup.passwordMismatch')); return; }
    setBusy(true); setError('');
    const request = { ...draft };
    setDraft(current => ({ ...current, token: '', password: '', confirmPassword: '' }));
    try { await api('/setup', { method: 'POST', body: JSON.stringify(request) }); onComplete(); }
    catch (err) { setError(describeError(err, t('setup.failed'))); }
    finally { setBusy(false); }
  }
  return <div className="sign-in-page"><section className="sign-in-brand">
    <a className="wordmark wordmark-light" href="/ui/"><span className="wordmark-mark">◆</span> Orchestrator</a>
    <div className="brand-message"><h1>{t('setup.welcome')}</h1></div>
  </section><section className="sign-in-form-wrap"><form className="sign-in-form" onSubmit={event => void submit(event)}>
    <h2>{t('setup.initialize')}</h2>
    {error ? <p role="alert" className="form-error">{error}</p> : null}
    {(['token', 'organizationKey', 'organizationName', 'username', 'password', 'confirmPassword'] as const).map(field => <label key={field}>{t(`setup.${field}`)}
      {field === 'token' || field === 'password' || field === 'confirmPassword' ? <PasswordInput aria-label={t(`setup.${field}`)} value={draft[field]} autoComplete="new-password" required minLength={field === 'token' ? 32 : 8} maxLength={1024} disabled={busy} onChange={event => setDraft({ ...draft, [field]: event.target.value })} /> : <input value={draft[field]} autoComplete={field === 'username' ? 'username' : 'off'} required maxLength={field === 'organizationName' ? 200 : 63} disabled={busy} onChange={event => setDraft({ ...draft, [field]: event.target.value })} />}
    </label>)}
    <Button type="submit" disabled={busy}>{t(busy ? 'setup.working' : 'setup.initialize')}</Button>
    <details><summary>{t('setup.help')}</summary><p>{t('setup.tokenHelp')}</p></details><LanguageSelect />
  </form></section></div>;
}
