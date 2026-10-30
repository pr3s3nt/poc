import { useRef, useState, type FormEvent } from 'react';
import { api, ApiError } from '../../shared/api/client';
import { Button } from '../../shared/ui/Button';
import { PasswordInput } from '../../shared/ui/PasswordInput';
import { t } from '../../shared/i18n';

type FieldErrors = { currentPassword?: string; newPassword?: string; confirmPassword?: string };

// Self-service password change (UC-00 BR-08). A successful change revokes
// every session of the account on the backend; onChanged ends the local
// session so the user signs in again with the new password.
export function ChangePasswordPage({ onChanged }: { onChanged(): void }) {
  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const submitting = useRef(false);

  function submit(event: FormEvent) {
    event.preventDefault();
    if (submitting.current) return;
    const errors: FieldErrors = {};
    if (!currentPassword) errors.currentPassword = t('auth.changePassword.enterCurrentPassword');
    if (!newPassword) errors.newPassword = t('auth.changePassword.enterNewPassword');
    else if (newPassword.length < 8) errors.newPassword = t('auth.changePassword.passwordTooShort');
    if (confirmPassword !== newPassword) errors.confirmPassword = t('auth.changePassword.confirmMismatch');
    setFieldErrors(errors);
    if (errors.currentPassword || errors.newPassword || errors.confirmPassword) return;
    submitting.current = true;
    setSaving(true); setError('');
    api('/auth/change-password', { method: 'POST', body: JSON.stringify({ currentPassword, newPassword, confirmPassword }) })
      .then(() => onChanged())
      .catch((err: unknown) => {
        if (err instanceof ApiError && err.code === 'INVALID_CREDENTIALS') setFieldErrors({ currentPassword: t('auth.changePassword.incorrectCurrentPassword') });
        else if (err instanceof ApiError && err.field === 'newPassword') setFieldErrors({ newPassword: t('auth.changePassword.passwordTooShort') });
        else if (err instanceof ApiError && err.field === 'confirmPassword') setFieldErrors({ confirmPassword: t('auth.changePassword.confirmMismatch') });
        else setError(t('auth.changePassword.couldNotChange'));
        submitting.current = false; setSaving(false);
      });
  }

  return <section className="page">
    <header className="page-header"><div><p className="eyebrow">{t('auth.changePassword.eyebrow')}</p><h1>{t('auth.changePassword.pageTitle')}</h1><p>{t('auth.changePassword.pageDescription')}</p></div></header>
    <form className="create-form" aria-label={t('auth.changePassword.pageTitle')} noValidate onSubmit={submit}>
      {error ? <div className="form-error" role="alert">{error}</div> : null}
      <fieldset className="form-fieldset" disabled={saving} aria-busy={saving}>
        <div className="editor-grid">
          <div className="field"><label>{t('auth.changePassword.currentPassword')}<PasswordInput autoComplete="current-password" value={currentPassword} aria-invalid={Boolean(fieldErrors.currentPassword)} onChange={(event) => setCurrentPassword(event.target.value)} /></label>{fieldErrors.currentPassword ? <small className="field-error" role="alert">{fieldErrors.currentPassword}</small> : null}</div>
          <div className="field"><label>{t('auth.changePassword.newPassword')}<PasswordInput autoComplete="new-password" value={newPassword} aria-invalid={Boolean(fieldErrors.newPassword)} onChange={(event) => setNewPassword(event.target.value)} /></label>{fieldErrors.newPassword ? <small className="field-error" role="alert">{fieldErrors.newPassword}</small> : null}</div>
          <div className="field"><label>{t('auth.changePassword.confirmPassword')}<PasswordInput autoComplete="new-password" value={confirmPassword} aria-invalid={Boolean(fieldErrors.confirmPassword)} onChange={(event) => setConfirmPassword(event.target.value)} /></label>{fieldErrors.confirmPassword ? <small className="field-error" role="alert">{fieldErrors.confirmPassword}</small> : null}</div>
        </div>
        <div className="form-actions"><Button tone="primary" type="submit" disabled={saving}>{saving ? t('auth.changePassword.submitting') : t('auth.changePassword.submit')}</Button></div>
      </fieldset>
    </form>
  </section>;
}
