import { useState, type InputHTMLAttributes } from 'react';
import { t } from '../i18n';

// Password input with a show/hide plain-text toggle (UC-00 UI design): the
// value is masked by default and the user reveals it on demand. The toggle is
// a plain button with its own accessible name, so it never submits the form.
export function PasswordInput(props: InputHTMLAttributes<HTMLInputElement>) {
  const [visible, setVisible] = useState(false);
  const label = visible ? t('shared.passwordInput.hide') : t('shared.passwordInput.show');
  return <span className="password-input">
    <input {...props} type={visible ? 'text' : 'password'} />
    <button type="button" className="password-toggle" aria-pressed={visible} aria-label={label} onClick={() => setVisible((current) => !current)}>{label}</button>
  </span>;
}
