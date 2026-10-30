import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { Dialog } from '../../shared/ui/Dialog';
import { PasswordInput } from '../../shared/ui/PasswordInput';
import { t } from '../../shared/i18n';
import { createUser, listUsers, resetUserPassword, type UserAccount, type UserRole } from './api';

const usernamePattern = /^[a-z0-9][a-z0-9._-]{1,62}$/;
const roles: readonly UserRole[] = ['DEVELOPER', 'PLATFORM_ENGINEER', 'ADMIN'];

type FieldErrors = { username?: string; password?: string };
type ResetFieldErrors = { newPassword?: string; confirmPassword?: string };

export function roleLabel(role: string): string {
  if (role === 'ADMIN') return t('users.page.roleAdmin');
  if (role === 'PLATFORM_ENGINEER') return t('users.page.rolePlatformEngineer');
  return t('users.page.roleDeveloper');
}

function statusLabel(status: string): string {
  return status === 'DISABLED' ? t('users.page.statusDisabled') : t('users.page.statusActive');
}

// Admin-only account management of the Organization (UC-00 BR-09). The backend
// re-checks the role on every call; this page only renders for ADMIN sessions.
export function UsersPage() {
  const [users, setUsers] = useState<readonly UserAccount[]>([]);
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState<UserRole>('DEVELOPER');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [saving, setSaving] = useState(false);
  const submitting = useRef(false);
  const [resetTarget, setResetTarget] = useState<UserAccount | null>(null);
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [resetFieldErrors, setResetFieldErrors] = useState<ResetFieldErrors>({});
  const [resetError, setResetError] = useState('');
  const [pageError, setPageError] = useState('');
  const [resetting, setResetting] = useState(false);
  const resettingRef = useRef(false);
  const load = useCallback(async () => {
    setState('loading');
    try { const response = await listUsers(); setUsers(response.users ?? []); setState('ready'); } catch { setState('error'); }
  }, []);
  useEffect(() => { void load(); }, [load]);

  function openReset(user: UserAccount) {
    setResetTarget(user);
    setNewPassword(''); setConfirmPassword('');
    setResetFieldErrors({}); setResetError('');
    setSuccess(''); setPageError('');
  }

  // Admin password reset (UC-00 BR-11): the Admin never enters the current
  // password of the account; the backend revokes every session of the target.
  function submitReset(event: FormEvent) {
    event.preventDefault();
    if (resettingRef.current || !resetTarget) return;
    const errors: ResetFieldErrors = {};
    if (!newPassword) errors.newPassword = t('users.page.enterNewPassword');
    else if (newPassword.length < 8) errors.newPassword = t('users.page.passwordTooShort');
    if (confirmPassword !== newPassword) errors.confirmPassword = t('users.page.confirmMismatch');
    setResetFieldErrors(errors);
    if (errors.newPassword || errors.confirmPassword) return;
    resettingRef.current = true;
    setResetting(true); setResetError('');
    const target = resetTarget;
    resetUserPassword(target.id, newPassword, confirmPassword).then(() => {
      setResetTarget(null);
      setSuccess(t('users.page.resetSuccess', { username: target.username }));
      resettingRef.current = false; setResetting(false);
    }).catch((err: unknown) => {
      if (err instanceof ApiError && err.status === 404) {
        setResetTarget(null);
        setPageError(t('users.page.resetNotFound'));
        void load();
      } else if (err instanceof ApiError && err.field === 'newPassword') setResetFieldErrors({ newPassword: t('users.page.passwordTooShort') });
      else if (err instanceof ApiError && err.field === 'confirmPassword') setResetFieldErrors({ confirmPassword: t('users.page.confirmMismatch') });
      else setResetError(t('users.page.couldNotReset'));
      resettingRef.current = false; setResetting(false);
    });
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting.current) return;
    const errors: FieldErrors = {};
    const name = username.trim();
    if (!name) errors.username = t('users.page.enterUsername');
    else if (!usernamePattern.test(name)) errors.username = t('users.page.usernameInvalid');
    if (!password) errors.password = t('users.page.enterPassword');
    else if (password.length < 8) errors.password = t('users.page.passwordTooShort');
    setFieldErrors(errors);
    if (errors.username || errors.password) return;
    submitting.current = true;
    setSaving(true); setError(''); setSuccess('');
    const submitted = password;
    setPassword('');
    createUser(name, submitted, role).then(async (response) => {
      setUsername(''); setRole('DEVELOPER');
      setSuccess(t('users.page.created', { username: response.user.username }));
      submitting.current = false; setSaving(false);
      await load();
    }).catch((err: unknown) => {
      if (err instanceof ApiError && err.code === 'DUPLICATE') setFieldErrors({ username: t('users.page.usernameTaken') });
      else if (err instanceof ApiError && err.field === 'username') setFieldErrors({ username: t('users.page.usernameInvalid') });
      else if (err instanceof ApiError && err.field === 'password') setFieldErrors({ password: t('users.page.passwordTooShort') });
      else setError(t('users.page.couldNotCreate'));
      submitting.current = false; setSaving(false);
    });
  }

  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('users.page.eyebrow')}</p><h1>{t('users.page.title')}</h1><p>{t('users.page.description')}</p></div></header>
    {success ? <div className="form-success" role="status">{success}</div> : null}
    {pageError ? <div className="form-error" role="alert">{pageError}</div> : null}
    <section className="content-panel" aria-label={t('users.page.accounts')}>
      <div className="section-header"><div><h2>{t('users.page.accounts')}</h2></div></div>
      {state === 'loading' ? <p role="status" aria-busy="true">{t('users.page.loading')}</p> : null}
      {state === 'error' ? <div className="form-error" role="alert">{t('users.page.loadError')}<Button type="button" onClick={() => void load()}>{t('users.page.retry')}</Button></div> : null}
      {users.length > 0 ? <div className="settings-table"><div className="settings-table-head"><span>{t('users.page.username')}</span><span>{t('users.page.role')}</span><span>{t('users.page.status')}</span><span /></div>
        {users.map((user) => <div className="settings-table-row" key={user.id}><strong>{user.username}</strong><span>{roleLabel(user.role)}</span><span>{statusLabel(user.status)}</span><span className="row-actions"><Button type="button" onClick={() => openReset(user)}>{t('users.page.resetPassword')}</Button></span></div>)}</div> : null}
    </section>
    <form className="create-form" aria-label={t('users.page.createTitle')} noValidate onSubmit={submit}>
      <h2>{t('users.page.createTitle')}</h2>
      {error ? <div className="form-error" role="alert">{error}</div> : null}
      <fieldset className="form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="editor-grid">
          <div className="field"><label>{t('users.page.username')}<input autoComplete="off" value={username} aria-invalid={Boolean(fieldErrors.username)} onChange={(event) => setUsername(event.target.value)} /></label>{fieldErrors.username ? <small className="field-error" role="alert">{fieldErrors.username}</small> : null}</div>
          <div className="field"><label>{t('users.page.password')}<PasswordInput autoComplete="new-password" value={password} aria-invalid={Boolean(fieldErrors.password)} onChange={(event) => setPassword(event.target.value)} /></label>{fieldErrors.password ? <small className="field-error" role="alert">{fieldErrors.password}</small> : null}</div>
          <div className="field"><label>{t('users.page.role')}<select value={role} onChange={(event) => setRole(event.target.value as UserRole)}>{roles.map((item) => <option key={item} value={item}>{roleLabel(item)}</option>)}</select></label></div>
        </div>
        <div className="form-actions"><Button tone="primary" type="submit" disabled={saving}>{saving ? t('users.page.creating') : t('users.page.create')}</Button></div>
      </fieldset>
    </form>
    {resetTarget ? <Dialog label={t('users.page.resetTitle', { username: resetTarget.username })} busy={resetting} onClose={() => setResetTarget(null)}>
      <form className="create-form" aria-label={t('users.page.resetTitle', { username: resetTarget.username })} noValidate onSubmit={submitReset}>
        <h2>{t('users.page.resetTitle', { username: resetTarget.username })}</h2>
        <p>{t('users.page.resetDescription')}</p>
        {resetError ? <div className="form-error" role="alert">{resetError}</div> : null}
        <fieldset className="form-fieldset" disabled={resetting} aria-busy={resetting}>
          <div className="editor-grid">
            <div className="field"><label>{t('users.page.newPassword')}<PasswordInput autoComplete="new-password" value={newPassword} aria-invalid={Boolean(resetFieldErrors.newPassword)} onChange={(event) => setNewPassword(event.target.value)} /></label>{resetFieldErrors.newPassword ? <small className="field-error" role="alert">{resetFieldErrors.newPassword}</small> : null}</div>
            <div className="field"><label>{t('users.page.confirmNewPassword')}<PasswordInput autoComplete="new-password" value={confirmPassword} aria-invalid={Boolean(resetFieldErrors.confirmPassword)} onChange={(event) => setConfirmPassword(event.target.value)} /></label>{resetFieldErrors.confirmPassword ? <small className="field-error" role="alert">{resetFieldErrors.confirmPassword}</small> : null}</div>
          </div>
          <div className="form-actions">
            <Button type="button" disabled={resetting} onClick={() => setResetTarget(null)}>{t('users.page.resetCancel')}</Button>
            <Button tone="primary" type="submit" disabled={resetting}>{resetting ? t('users.page.resetting') : t('users.page.reset')}</Button>
          </div>
        </fieldset>
      </form>
    </Dialog> : null}
  </section>;
}
