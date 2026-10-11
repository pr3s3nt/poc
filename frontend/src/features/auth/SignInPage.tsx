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
    const errors: FieldErrors = { username: username.trim() ? undefined : 'Nhập tên đăng nhập.', password: password ? undefined : 'Nhập mật khẩu.' };
    setFieldErrors(errors);
    if (errors.username || errors.password) return;
    setError(null);
    setSubmitting(true);
    // The password is handed to the request only and never kept after submit.
    const submitted = password;
    setPassword('');
    onSuccess(username, submitted).catch((err: unknown) => {
      setError(err instanceof ApiError && err.status === 401 ? 'Tên đăng nhập hoặc mật khẩu không đúng.' : 'Không thể đăng nhập. Kiểm tra kết nối và thử lại.');
      setSubmitting(false);
    });
  }

  return <div className="sign-in-page">
    <section className="sign-in-brand">
      <a className="wordmark wordmark-light" href="#sign-in"><span className="wordmark-mark">◆</span> Orchestrator</a>
      <p className="eyebrow eyebrow-light">Nền tảng nội bộ cho nhà phát triển</p>
      <div className="brand-message"><h1>Xây dựng.<br />Triển khai.<br />Làm việc nhanh hơn.</h1><p>Hạ tầng giúp bạn tập trung vào ứng dụng.</p></div>
      <div className="network-art" aria-hidden="true"><i /><i /><i /><i /><i /></div>
    </section>
    <section className="sign-in-form-wrap">
      <form className="sign-in-form" onSubmit={submit} noValidate>
        <span className="eyebrow">Chào mừng trở lại</span>
        <h2>Đăng nhập vào không gian làm việc</h2>
        <p>Đăng nhập để tạo và triển khai ứng dụng.</p>
        {notice && !error ? <div className="form-info" role="status">{notice}</div> : null}
        {error ? <div className="form-error" role="alert">{error}</div> : null}
        <label>Tên đăng nhập<input autoComplete="username" value={username} disabled={submitting} aria-invalid={Boolean(fieldErrors.username)} onChange={(event) => setUsername(event.target.value)} />{fieldErrors.username ? <span className="field-error">{fieldErrors.username}</span> : null}</label>
        <label>Mật khẩu<input type="password" autoComplete="current-password" value={password} disabled={submitting} aria-invalid={Boolean(fieldErrors.password)} onChange={(event) => setPassword(event.target.value)} />{fieldErrors.password ? <span className="field-error">{fieldErrors.password}</span> : null}</label>
        <Button tone="primary" type="submit" disabled={submitting}>{submitting ? 'Đang đăng nhập…' : 'Đăng nhập'}</Button>
        <small>Dùng tài khoản kiểm thử local của bạn.</small>
      </form>
    </section>
  </div>;
}
